// handler.go — основной обработчик API, реализующий generated.ServerInterface.
// Объединяет health и бизнес-обработчики.
package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"

	apierrors "github.com/bigkaa/goartstore/ingester-module/internal/api/errors"
	"github.com/bigkaa/goartstore/ingester-module/internal/api/middleware"
)

// APIHandler — основной обработчик API Ingester Module.
// Реализует generated.ServerInterface, делегируя запросы в сервисный слой.
type APIHandler struct {
	health *HealthHandler
	logger *slog.Logger
	// uploadService будет добавлен в Phase 3
}

// NewAPIHandler создаёт основной обработчик API.
// uploadService = nil в Phase 2, будет установлен в Phase 3.
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
	apierrors.WriteError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED",
		"Upload endpoint ещё не реализован (Phase 3)")
}

// --- Авторизация ---

// checkAuth проверяет наличие роли admin или scope files:write.
// Возвращает true, если авторизация пройдена.
// Upload авторизация: role admin ИЛИ scope files:write.
//
//nolint:unused // используется в Phase 3.5
func (h *APIHandler) checkAuth(w http.ResponseWriter, r *http.Request) bool {
	claims := middleware.ClaimsFromContext(r.Context())
	if claims == nil {
		apierrors.Unauthorized(w, "Отсутствуют claims в контексте")
		return false
	}

	// User: role admin. SA: scope files:write.
	switch claims.SubjectType {
	case middleware.SubjectTypeUser:
		if claims.HasAnyRole(middleware.RoleAdmin) {
			return true
		}
		apierrors.Forbidden(w, "Недостаточно прав: требуется роль admin")
		return false

	case middleware.SubjectTypeSA:
		if claims.HasAnyScope("files:write") {
			return true
		}
		apierrors.Forbidden(w, "Недостаточно прав: требуется scope files:write")
		return false

	default:
		apierrors.Forbidden(w, "Неизвестный тип субъекта")
		return false
	}
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
