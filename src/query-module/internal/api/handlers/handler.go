// handler.go — основной обработчик API, реализующий generated.ServerInterface.
// Объединяет health и бизнес-обработчики.
package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/google/uuid"

	"github.com/bigkaa/goartstore/query-module/internal/api/errors"
	"github.com/bigkaa/goartstore/query-module/internal/api/generated"
	"github.com/bigkaa/goartstore/query-module/internal/api/middleware"
	"github.com/bigkaa/goartstore/query-module/internal/service"
)

// APIHandler — основной обработчик API Query Module.
// Реализует generated.ServerInterface, делегируя запросы в сервисный слой.
type APIHandler struct {
	health        *HealthHandler
	searchService *service.SearchService
	logger        *slog.Logger
}

// NewAPIHandler создаёт основной обработчик API.
func NewAPIHandler(
	health *HealthHandler,
	searchService *service.SearchService,
	logger *slog.Logger,
) *APIHandler {
	return &APIHandler{
		health:        health,
		searchService: searchService,
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

// SearchFiles — поиск файлов (POST /api/v1/search).
// Авторизация: admin, readonly / files:read.
func (h *APIHandler) SearchFiles(w http.ResponseWriter, r *http.Request) {
	// Проверка авторизации
	if !h.checkAuth(w, r) {
		return
	}
	h.handleSearchFiles(w, r)
}

// GetFileMetadata — метаданные файла (GET /api/v1/files/{file_id}).
// Авторизация: admin, readonly / files:read.
func (h *APIHandler) GetFileMetadata(w http.ResponseWriter, r *http.Request, fileID generated.FileId) {
	// Проверка авторизации
	if !h.checkAuth(w, r) {
		return
	}
	h.handleGetFileMetadata(w, r, fileID)
}

// DownloadFile — скачивание файла (stub, будет реализован в Phase 4).
func (h *APIHandler) DownloadFile(w http.ResponseWriter, _ *http.Request, _ generated.FileId, _ generated.DownloadFileParams) {
	errors.WriteError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "Скачивание файла ещё не реализовано")
}

// --- Авторизация ---

// checkAuth проверяет наличие роли admin/readonly или scope files:read.
// Возвращает true, если авторизация пройдена.
func (h *APIHandler) checkAuth(w http.ResponseWriter, r *http.Request) bool {
	claims := middleware.ClaimsFromContext(r.Context())
	if claims == nil {
		errors.Unauthorized(w, "Требуется аутентификация")
		return false
	}

	// Проверяем роли (для пользователей) или scopes (для service accounts)
	if claims.HasAnyRole("admin", "readonly") || claims.HasAnyScope("files:read") {
		return true
	}

	errors.Forbidden(w, "Недостаточно прав для доступа к файлам")
	return false
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
func paginationDefaults(limit, offset *int) (limitVal, offsetVal int) {
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

// parseUUID парсит строку UUID. При ошибке парсинга возвращает нулевой UUID.
func parseUUID(s string) uuid.UUID {
	u, err := uuid.Parse(s)
	if err != nil {
		return uuid.UUID{}
	}
	return u
}

// tagsToPtr конвертирует []string в *[]string для API-ответов.
// nil или пустой срез → nil (omitempty в JSON).
func tagsToPtr(tags []string) *[]string {
	if len(tags) == 0 {
		return nil
	}
	return &tags
}
