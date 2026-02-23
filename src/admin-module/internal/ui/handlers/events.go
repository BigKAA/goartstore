// Пакет handlers — HTTP-обработчики Admin UI.
// Файл events.go — SSE (Server-Sent Events) endpoints для real-time обновлений:
// статусы зависимостей (PostgreSQL, Keycloak), статусы SE, агрегированные метрики.
// Каждый SSE-клиент обслуживается отдельной горутиной.
package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/arturkryukov/artstore/admin-module/internal/service"
	uimiddleware "github.com/arturkryukov/artstore/admin-module/internal/ui/middleware"
)

// SSE event интервал отправки (каждые 15 секунд).
const sseInterval = 15 * time.Second

// EventsHandler — обработчик SSE endpoints для real-time обновлений.
type EventsHandler struct {
	storageElemsSvc *service.StorageElementService
	dephealthSvc    *service.DephealthService // может быть nil
	logger          *slog.Logger
}

// NewEventsHandler создаёт новый EventsHandler.
func NewEventsHandler(
	storageElemsSvc *service.StorageElementService,
	dephealthSvc *service.DephealthService,
	logger *slog.Logger,
) *EventsHandler {
	return &EventsHandler{
		storageElemsSvc: storageElemsSvc,
		dephealthSvc:    dephealthSvc,
		logger:          logger.With(slog.String("component", "ui.events")),
	}
}

// depStatusEvent — SSE-событие статусов зависимостей.
type depStatusEvent struct {
	Dependencies []depStatusItem `json:"dependencies"`
}

// depStatusItem — статус одной зависимости.
type depStatusItem struct {
	Name   string `json:"name"`
	Status string `json:"status"` // online, offline, unavailable
}

// seStatusEvent — SSE-событие статусов Storage Elements.
type seStatusEvent struct {
	Elements []seStatusItem `json:"elements"`
	Total    int            `json:"total"`
	Online   int            `json:"online"`
	Offline  int            `json:"offline"`
	Degraded int            `json:"degraded"`
}

// seStatusItem — статус одного SE.
type seStatusItem struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
	Mode   string `json:"mode"`
}

// HandleSystemStatus обрабатывает GET /admin/events/system-status — SSE endpoint.
// Периодически (каждые 15с) отправляет клиенту статусы зависимостей и SE.
// Формат: event: dep-status\ndata: {json}\n\n, event: se-status\ndata: {json}\n\n
// Graceful disconnect при закрытии клиентом соединения (context cancel).
func (h *EventsHandler) HandleSystemStatus(w http.ResponseWriter, r *http.Request) {
	session := uimiddleware.SessionFromContext(r.Context())
	if session == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Настраиваем заголовки SSE
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // Отключаем буферизацию Nginx

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "SSE не поддерживается", http.StatusInternalServerError)
		return
	}

	ctx := r.Context()

	h.logger.Debug("SSE клиент подключён",
		slog.String("username", session.Username),
		slog.String("remote_addr", r.RemoteAddr),
	)

	// Отправляем начальные данные сразу при подключении
	h.sendDepStatus(ctx, w, flusher)
	h.sendSEStatus(ctx, w, flusher)

	// Периодическая отправка
	ticker := time.NewTicker(sseInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			// Клиент отключился
			h.logger.Debug("SSE клиент отключён",
				slog.String("username", session.Username),
			)
			return
		case <-ticker.C:
			h.sendDepStatus(ctx, w, flusher)
			h.sendSEStatus(ctx, w, flusher)
		}
	}
}

// sendDepStatus отправляет SSE-событие со статусами зависимостей.
func (h *EventsHandler) sendDepStatus(ctx context.Context, w http.ResponseWriter, flusher http.Flusher) {
	event := depStatusEvent{}

	if h.dephealthSvc == nil {
		event.Dependencies = []depStatusItem{
			{Name: "PostgreSQL", Status: "unavailable"},
			{Name: "Keycloak", Status: "unavailable"},
		}
	} else {
		health := h.dephealthSvc.Health()
		event.Dependencies = []depStatusItem{
			{Name: "PostgreSQL", Status: depHealthStatus(health["postgresql"])},
			{Name: "Keycloak", Status: depHealthStatus(health["keycloak-jwks"])},
		}
	}

	data, err := json.Marshal(event)
	if err != nil {
		h.logger.Error("Ошибка сериализации dep-status", slog.String("error", err.Error()))
		return
	}

	// Формат SSE: event: dep-status\ndata: {json}\n\n
	fmt.Fprintf(w, "event: dep-status\ndata: %s\n\n", data)
	flusher.Flush()
}

// sendSEStatus отправляет SSE-событие со статусами Storage Elements.
func (h *EventsHandler) sendSEStatus(ctx context.Context, w http.ResponseWriter, flusher http.Flusher) {
	ses, _, err := h.storageElemsSvc.List(ctx, nil, nil, 1000, 0)
	if err != nil {
		h.logger.Error("Ошибка получения SE для SSE", slog.String("error", err.Error()))
		return
	}

	event := seStatusEvent{
		Elements: make([]seStatusItem, 0, len(ses)),
	}

	for _, se := range ses {
		event.Elements = append(event.Elements, seStatusItem{
			ID:     se.ID,
			Name:   se.Name,
			Status: se.Status,
			Mode:   se.Mode,
		})
		event.Total++
		switch se.Status {
		case "online":
			event.Online++
		case "offline":
			event.Offline++
		case "degraded":
			event.Degraded++
		}
	}

	data, err := json.Marshal(event)
	if err != nil {
		h.logger.Error("Ошибка сериализации se-status", slog.String("error", err.Error()))
		return
	}

	fmt.Fprintf(w, "event: se-status\ndata: %s\n\n", data)
	flusher.Flush()
}
