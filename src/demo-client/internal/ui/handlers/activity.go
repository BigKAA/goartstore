package handlers

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/bigkaa/goartstore/demo-client/internal/activity"
	"github.com/bigkaa/goartstore/demo-client/internal/ui/pages"
)

// ActivityHandler — обработчик SSE потока Activity Log.
type ActivityHandler struct {
	log    *activity.Log
	logger *slog.Logger
}

// NewActivityHandler создаёт обработчик Activity Log.
func NewActivityHandler(log *activity.Log, logger *slog.Logger) *ActivityHandler {
	return &ActivityHandler{
		log:    log,
		logger: logger,
	}
}

// Stream — GET /activity/stream — SSE поток новых записей Activity Log.
// Отправляет начальные записи, затем стримит новые по мере поступления.
func (h *ActivityHandler) Stream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	// SSE заголовки
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // для nginx/envoy

	ctx := r.Context()

	// Подписываемся на новые записи
	ch := h.log.Subscribe()
	defer h.log.Unsubscribe(ch)

	// Отправляем начальные записи (все текущие)
	entries := h.log.List()
	for _, entry := range entries {
		if err := h.sendEntry(ctx, w, entry); err != nil {
			return
		}
	}
	flusher.Flush()

	// Стримим новые записи
	for {
		select {
		case <-ctx.Done():
			// Клиент отключился
			return
		case entry, ok := <-ch:
			if !ok {
				// Канал закрыт
				return
			}
			if err := h.sendEntry(ctx, w, entry); err != nil {
				h.logger.Warn("ошибка отправки SSE", "error", err)
				return
			}
			flusher.Flush()
		}
	}
}

// sendEntry рендерит activity_row partial и отправляет как SSE event.
func (h *ActivityHandler) sendEntry(ctx context.Context, w http.ResponseWriter, entry activity.Entry) error {
	// Рендерим partial в буфер
	var buf bytes.Buffer
	if err := pages.ActivityRow(entry).Render(ctx, &buf); err != nil {
		return fmt.Errorf("рендер activity row: %w", err)
	}

	// Формат SSE: event: activity\ndata: <html>\n\n
	_, err := fmt.Fprintf(w, "event: activity\ndata: %s\n\n", buf.String())
	return err
}
