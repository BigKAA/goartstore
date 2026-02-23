// handler.go — основной обработчик API, реализующий generated.ServerInterface.
// Объединяет все доменные обработчики и делегирует запросы в сервисный слой.
package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/arturkryukov/artstore/admin-module/internal/service"
)

// APIHandler — основной обработчик API Admin Module.
// Реализует generated.ServerInterface, делегируя запросы в сервисный слой.
type APIHandler struct {
	health       *HealthHandler
	adminUsers   *service.AdminUserService
	serviceAccts *service.ServiceAccountService
	storageElems *service.StorageElementService
	files        *service.FileRegistryService
	idp          *service.IDPService
	logger       *slog.Logger
}

// NewAPIHandler создаёт основной обработчик API.
func NewAPIHandler(
	health *HealthHandler,
	adminUsers *service.AdminUserService,
	serviceAccts *service.ServiceAccountService,
	storageElems *service.StorageElementService,
	files *service.FileRegistryService,
	idp *service.IDPService,
	logger *slog.Logger,
) *APIHandler {
	return &APIHandler{
		health:       health,
		adminUsers:   adminUsers,
		serviceAccts: serviceAccts,
		storageElems: storageElems,
		files:        files,
		idp:          idp,
		logger:       logger.With(slog.String("component", "api_handler")),
	}
}

// HealthLive — liveness probe (делегируется в HealthHandler).
func (h *APIHandler) HealthLive(w http.ResponseWriter, r *http.Request) {
	h.health.HealthLive(w, r)
}

// HealthReady — readiness probe (делегируется в HealthHandler).
func (h *APIHandler) HealthReady(w http.ResponseWriter, r *http.Request) {
	h.health.HealthReady(w, r)
}

// GetMetrics — Prometheus метрики (делегируется в HealthHandler).
func (h *APIHandler) GetMetrics(w http.ResponseWriter, r *http.Request) {
	h.health.GetMetrics(w, r)
}

// --- Вспомогательные функции ---

// writeJSON записывает JSON-ответ с указанным статусом.
func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

// paginationDefaults нормализует параметры пагинации.
// Возвращает корректные limit и offset.
func paginationDefaults(limit *int, offset *int) (int, int) {
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
