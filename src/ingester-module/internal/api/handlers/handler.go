// handler.go — основной обработчик API, реализующий generated.ServerInterface.
// Объединяет health и бизнес-обработчики.
package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"

	apierrors "github.com/bigkaa/goartstore/ingester-module/internal/api/errors"
	"github.com/bigkaa/goartstore/ingester-module/internal/api/middleware"
	"github.com/bigkaa/goartstore/ingester-module/internal/service"
)

// APIHandler — основной обработчик API Ingester Module.
// Реализует generated.ServerInterface, делегируя запросы в сервисный слой.
type APIHandler struct {
	health        *HealthHandler
	uploadService *service.UploadService
	logger        *slog.Logger
}

// NewAPIHandler создаёт основной обработчик API.
// uploadService может быть nil (тогда upload возвращает 501).
func NewAPIHandler(
	health *HealthHandler,
	uploadService *service.UploadService,
	logger *slog.Logger,
) *APIHandler {
	return &APIHandler{
		health:        health,
		uploadService: uploadService,
		logger:        logger.With(slog.String("component", "api_handler")),
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
func (h *APIHandler) UploadFile(w http.ResponseWriter, r *http.Request) {
	if h.uploadService == nil {
		apierrors.WriteError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED",
			"Upload endpoint ещё не реализован")
		return
	}
	h.handleUploadFile(w, r)
}

// --- Авторизация ---

// checkAuth проверяет наличие роли admin или scope files:write.
// Возвращает true, если авторизация пройдена.
// Upload авторизация: role admin ИЛИ scope files:write.
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
func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}
