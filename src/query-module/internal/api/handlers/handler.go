// handler.go — основной обработчик API, реализующий generated.ServerInterface.
// Объединяет health и бизнес-обработчики. Пока все бизнес-методы — stubs (501).
package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/bigkaa/goartstore/query-module/internal/api/errors"
	"github.com/bigkaa/goartstore/query-module/internal/api/generated"
)

// APIHandler — основной обработчик API Query Module.
// Реализует generated.ServerInterface, делегируя запросы в сервисный слой.
type APIHandler struct {
	health *HealthHandler
	logger *slog.Logger
}

// NewAPIHandler создаёт основной обработчик API.
func NewAPIHandler(
	health *HealthHandler,
	logger *slog.Logger,
) *APIHandler {
	return &APIHandler{
		health: health,
		logger: logger.With(slog.String("component", "api_handler")),
	}
}

// --- Health endpoints (делегируются в HealthHandler) ---

// HealthLive — liveness probe.
func (h *APIHandler) HealthLive(w http.ResponseWriter, r *http.Request) {
	h.health.HealthLive(w, r)
}

// HealthReady — readiness probe.
func (h *APIHandler) HealthReady(w http.ResponseWriter, r *http.Request) {
	h.health.HealthReady(w, r)
}

// GetMetrics — Prometheus метрики.
func (h *APIHandler) GetMetrics(w http.ResponseWriter, r *http.Request) {
	h.health.GetMetrics(w, r)
}

// --- Stub-обработчики (501 Not Implemented) ---
// Будут реализованы в Phase 3-4.

// SearchFiles — поиск файлов (stub).
func (h *APIHandler) SearchFiles(w http.ResponseWriter, _ *http.Request) {
	errors.WriteError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "Поиск файлов ещё не реализован")
}

// GetFileMetadata — метаданные файла (stub).
func (h *APIHandler) GetFileMetadata(w http.ResponseWriter, _ *http.Request, _ generated.FileId) {
	errors.WriteError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "Получение метаданных файла ещё не реализовано")
}

// DownloadFile — скачивание файла (stub).
func (h *APIHandler) DownloadFile(w http.ResponseWriter, _ *http.Request, _ generated.FileId, _ generated.DownloadFileParams) {
	errors.WriteError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "Скачивание файла ещё не реализовано")
}

// --- Вспомогательные функции ---

// writeJSON записывает JSON-ответ с указанным статусом.
func writeJSON(w http.ResponseWriter, status int, data any) { //nolint:unused // используется в Phase 3+
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

// paginationDefaults нормализует параметры пагинации.
// Возвращает корректные limit и offset.
func paginationDefaults(limit, offset *int) (limitVal, offsetVal int) { //nolint:unused // используется в Phase 3+
	l := 100
	o := 0

	if limit != nil {
		l = *limit
		if l < 1 {
			l = 1
		}
		if l > 1000 {
			l = 1000
		}
	}

	if offset != nil {
		o = *offset
		if o < 0 {
			o = 0
		}
	}

	return l, o
}
