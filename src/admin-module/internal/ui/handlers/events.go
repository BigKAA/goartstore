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
	"strings"
	"time"

	"github.com/bigkaa/goartstore/admin-module/internal/service"
	uimiddleware "github.com/bigkaa/goartstore/admin-module/internal/ui/middleware"
)

// EventsHandler — обработчик SSE endpoints для real-time обновлений.
type EventsHandler struct {
	storageElemsSvc *service.StorageElementService
	dephealthSvc    *service.DephealthService // может быть nil
	sseInterval     time.Duration
	logger          *slog.Logger
}

// NewEventsHandler создаёт новый EventsHandler.
// sseInterval — интервал отправки SSE-обновлений (AM_SSE_INTERVAL).
func NewEventsHandler(
	storageElemsSvc *service.StorageElementService,
	dephealthSvc *service.DephealthService,
	sseInterval time.Duration,
	logger *slog.Logger,
) *EventsHandler {
	return &EventsHandler{
		storageElemsSvc: storageElemsSvc,
		dephealthSvc:    dephealthSvc,
		sseInterval:     sseInterval,
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

	// Используем http.ResponseController для корректной работы Flush()
	// через обёрнутый ResponseWriter (logging middleware и др.).
	// ResponseController вызывает Unwrap() и находит оригинальный http.Flusher.
	rc := http.NewResponseController(w)
	if err := rc.Flush(); err != nil {
		http.Error(w, "SSE не поддерживается", http.StatusInternalServerError)
		return
	}

	ctx := r.Context()

	h.logger.Debug("SSE клиент подключён",
		slog.String("username", session.Username),
		slog.String("remote_addr", r.RemoteAddr),
	)

	// Отправляем начальные данные сразу при подключении
	h.sendDepStatus(ctx, w, rc)
	h.sendSEStatus(ctx, w, rc)

	// Периодическая отправка
	ticker := time.NewTicker(h.sseInterval)
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
			h.sendDepStatus(ctx, w, rc)
			h.sendSEStatus(ctx, w, rc)
		}
	}
}

// sendDepStatus отправляет SSE-событие со статусами зависимостей.
func (h *EventsHandler) sendDepStatus(_ context.Context, w http.ResponseWriter, rc *http.ResponseController) {
	event := depStatusEvent{}

	if h.dephealthSvc == nil {
		event.Dependencies = []depStatusItem{
			{Name: "PostgreSQL", Status: "unavailable"},
			{Name: "Keycloak", Status: "unavailable"},
		}
	} else {
		health := h.dephealthSvc.Health()
		// Health() возвращает ключи формата "dependency:host:port"
		// (например "postgresql:postgresql:5432").
		// Ищем статус по префиксу имени зависимости.
		event.Dependencies = []depStatusItem{
			{Name: "PostgreSQL", Status: depHealthStatus(findHealthByPrefix(health, "postgresql"))},
			{Name: "Keycloak", Status: depHealthStatus(findHealthByPrefix(health, "keycloak-jwks"))},
		}
	}

	data, err := json.Marshal(event)
	if err != nil {
		h.logger.Error("Ошибка сериализации dep-status", slog.String("error", err.Error()))
		return
	}

	// Формат SSE: event: dep-status\ndata: {json}\n\n
	fmt.Fprintf(w, "event: dep-status\ndata: %s\n\n", data)
	_ = rc.Flush()
}

// sendSEStatus отправляет SSE-событие со статусами Storage Elements.
func (h *EventsHandler) sendSEStatus(ctx context.Context, w http.ResponseWriter, rc *http.ResponseController) {
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
		case seStatusOnline:
			event.Online++
		case seStatusOffline:
			event.Offline++
		case seStatusDegraded:
			event.Degraded++
		}
	}

	data, err := json.Marshal(event)
	if err != nil {
		h.logger.Error("Ошибка сериализации se-status", slog.String("error", err.Error()))
		return
	}

	fmt.Fprintf(w, "event: se-status\ndata: %s\n\n", data)
	_ = rc.Flush()
}

// findHealthByPrefix ищет статус зависимости по префиксу имени.
// Health() из topologymetrics SDK возвращает ключи формата "dependency:host:port",
// поэтому ищем ключ, начинающийся с имени зависимости + ":".
// Если найдено несколько — возвращает true только если все healthy.
func findHealthByPrefix(health map[string]bool, prefix string) bool {
	found := false
	for key, ok := range health {
		if strings.HasPrefix(key, prefix+":") || key == prefix {
			if !ok {
				return false
			}
			found = true
		}
	}
	return found
}
