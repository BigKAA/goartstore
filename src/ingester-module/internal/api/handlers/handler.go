// handler.go — основной обработчик API, реализующий generated.ServerInterface.
// Объединяет health и бизнес-обработчики.
package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/bigkaa/goartstore/ingester-module/internal/api/errors"
)

// APIHandler — основной обработчик API Ingester Module.
// Реализует generated.ServerInterface, делегируя запросы в сервисный слой.
type APIHandler struct {
	health *HealthHandler
	logger *slog.Logger
	// uploadService будет добавлен в Phase 3
}

// NewAPIHandler создаёт основной обработчик API.
// uploadService = nil в Phase 1, будет установлен в Phase 3.
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

// --- Бизнес-обработчики ---

// UploadFile — загрузка файла (POST /api/v1/files/upload).
// Stub: возвращает 501 Not Implemented. Полная реализация в Phase 3.
func (h *APIHandler) UploadFile(w http.ResponseWriter, _ *http.Request) {
	errors.WriteError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED",
		"Upload endpoint ещё не реализован (Phase 3)")
}

// --- Авторизация ---

// checkAuth проверяет наличие роли admin или scope files:write.
// Stub: всегда возвращает true. Полная реализация в Phase 3.5.
//
//nolint:unused // используется в Phase 3.5
func (h *APIHandler) checkAuth(_ http.ResponseWriter, _ *http.Request) bool {
	// Stub — полная реализация через ClaimsFromContext в Phase 3.5
	return true
}

// --- Вспомогательные функции ---

// writeJSON записывает JSON-ответ с указанным статусом.
//
//nolint:unused // используется в Phase 3.5
func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}
