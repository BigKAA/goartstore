// storage_elements.go — обработчики /api/v1/storage-elements endpoints.
// Discover, CRUD SE, sync.
package handlers

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/google/uuid"

	apierrors "github.com/arturkryukov/artsore/admin-module/internal/api/errors"
	"github.com/arturkryukov/artsore/admin-module/internal/api/generated"
	"github.com/arturkryukov/artsore/admin-module/internal/api/middleware"
	"github.com/arturkryukov/artsore/admin-module/internal/domain/model"
	"github.com/arturkryukov/artsore/admin-module/internal/service"
)

// DiscoverStorageElement — POST /api/v1/storage-elements/discover.
// Предпросмотр SE: запрос GET /api/v1/info к SE.
// Доступ: admin или readonly.
func (h *APIHandler) DiscoverStorageElement(w http.ResponseWriter, r *http.Request) {
	claims := middleware.ClaimsFromContext(r.Context())
	if claims == nil || claims.SubjectType != middleware.SubjectTypeUser || !claims.HasAnyRole("admin", "readonly") {
		apierrors.Forbidden(w, "Недостаточно прав: требуется роль admin или readonly")
		return
	}

	var req generated.DiscoverRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierrors.ValidationError(w, "Некорректный JSON: "+err.Error())
		return
	}

	if req.Url == "" {
		apierrors.ValidationError(w, "URL Storage Element обязателен")
		return
	}

	result, err := h.storageElems.Discover(r.Context(), req.Url)
	if err != nil {
		if errors.Is(err, service.ErrSEUnavailable) {
			apierrors.SEUnavailable(w, err.Error())
			return
		}
		h.logger.Error("Ошибка discover SE", "url", req.Url, "error", err)
		apierrors.InternalError(w, "Ошибка обнаружения Storage Element")
		return
	}

	resp := generated.DiscoverResponse{
		StorageId: result.StorageID,
		Mode:      generated.DiscoverResponseMode(result.Mode),
		Status:    generated.DiscoverResponseStatus(result.Status),
		Version:   result.Version,
		Capacity: struct {
			AvailableBytes *int64 `json:"available_bytes,omitempty"`
			TotalBytes     *int64 `json:"total_bytes,omitempty"`
			UsedBytes      *int64 `json:"used_bytes,omitempty"`
		}{
			TotalBytes:     &result.TotalBytes,
			UsedBytes:      &result.UsedBytes,
			AvailableBytes: &result.AvailableBytes,
		},
	}

	writeJSON(w, http.StatusOK, resp)
}

// CreateStorageElement — POST /api/v1/storage-elements.
// Регистрация SE: discover + сохранение в БД + полная синхронизация файлов (Phase 5).
// Доступ: admin.
func (h *APIHandler) CreateStorageElement(w http.ResponseWriter, r *http.Request) {
	claims := middleware.ClaimsFromContext(r.Context())
	if claims == nil || claims.SubjectType != middleware.SubjectTypeUser || !claims.HasRole("admin") {
		apierrors.Forbidden(w, "Недостаточно прав: требуется роль admin")
		return
	}

	var req generated.StorageElementCreate
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierrors.ValidationError(w, "Некорректный JSON: "+err.Error())
		return
	}

	// Валидация
	if req.Name == "" || len(req.Name) > 100 {
		apierrors.ValidationError(w, "Имя SE должно быть от 1 до 100 символов")
		return
	}
	if req.Url == "" {
		apierrors.ValidationError(w, "URL Storage Element обязателен")
		return
	}

	se, err := h.storageElems.Create(r.Context(), req.Name, req.Url)
	if err != nil {
		if errors.Is(err, service.ErrConflict) {
			apierrors.Conflict(w, err.Error())
			return
		}
		if errors.Is(err, service.ErrSEUnavailable) {
			apierrors.SEUnavailable(w, err.Error())
			return
		}
		h.logger.Error("Ошибка создания SE", "error", err)
		apierrors.InternalError(w, "Ошибка регистрации Storage Element")
		return
	}

	writeJSON(w, http.StatusCreated, mapStorageElement(se))
}

// ListStorageElements — GET /api/v1/storage-elements.
// Возвращает список зарегистрированных SE.
// Доступ: admin, readonly или SA с scope storage:read.
func (h *APIHandler) ListStorageElements(w http.ResponseWriter, r *http.Request, params generated.ListStorageElementsParams) {
	claims := middleware.ClaimsFromContext(r.Context())
	if claims == nil {
		apierrors.Unauthorized(w, "Отсутствуют claims")
		return
	}

	// Проверка RBAC: admin/readonly или SA с storage:read
	switch claims.SubjectType {
	case middleware.SubjectTypeUser:
		if !claims.HasAnyRole("admin", "readonly") {
			apierrors.Forbidden(w, "Недостаточно прав: требуется роль admin или readonly")
			return
		}
	case middleware.SubjectTypeSA:
		if !claims.HasScope("storage:read") {
			apierrors.Forbidden(w, "Недостаточно прав: требуется scope storage:read")
			return
		}
	default:
		apierrors.Forbidden(w, "Неизвестный тип субъекта")
		return
	}

	limit, offset := paginationDefaults(params.Limit, params.Offset)

	var mode, status *string
	if params.Mode != nil {
		s := string(*params.Mode)
		mode = &s
	}
	if params.Status != nil {
		s := string(*params.Status)
		status = &s
	}

	ses, total, err := h.storageElems.List(r.Context(), mode, status, limit, offset)
	if err != nil {
		h.logger.Error("Ошибка получения списка SE", "error", err)
		apierrors.InternalError(w, "Ошибка получения списка Storage Elements")
		return
	}

	items := make([]generated.StorageElement, len(ses))
	for i, se := range ses {
		items[i] = mapStorageElement(se)
	}

	resp := generated.StorageElementListResponse{
		Items:   items,
		Total:   total,
		Limit:   limit,
		Offset:  offset,
		HasMore: offset+limit < total,
	}

	writeJSON(w, http.StatusOK, resp)
}

// GetStorageElement — GET /api/v1/storage-elements/{id}.
// Возвращает SE по ID.
// Доступ: admin, readonly или SA с scope storage:read.
func (h *APIHandler) GetStorageElement(w http.ResponseWriter, r *http.Request, id generated.StorageElementId) {
	claims := middleware.ClaimsFromContext(r.Context())
	if claims == nil {
		apierrors.Unauthorized(w, "Отсутствуют claims")
		return
	}

	switch claims.SubjectType {
	case middleware.SubjectTypeUser:
		if !claims.HasAnyRole("admin", "readonly") {
			apierrors.Forbidden(w, "Недостаточно прав: требуется роль admin или readonly")
			return
		}
	case middleware.SubjectTypeSA:
		if !claims.HasScope("storage:read") {
			apierrors.Forbidden(w, "Недостаточно прав: требуется scope storage:read")
			return
		}
	default:
		apierrors.Forbidden(w, "Неизвестный тип субъекта")
		return
	}

	se, err := h.storageElems.Get(r.Context(), id.String())
	if err != nil {
		if errors.Is(err, service.ErrNotFound) {
			apierrors.NotFound(w, "Storage Element не найден")
			return
		}
		h.logger.Error("Ошибка получения SE", "se_id", id, "error", err)
		apierrors.InternalError(w, "Ошибка получения Storage Element")
		return
	}

	writeJSON(w, http.StatusOK, mapStorageElement(se))
}

// UpdateStorageElement — PUT /api/v1/storage-elements/{id}.
// Обновляет SE (только name и url).
// Доступ: admin.
func (h *APIHandler) UpdateStorageElement(w http.ResponseWriter, r *http.Request, id generated.StorageElementId) {
	claims := middleware.ClaimsFromContext(r.Context())
	if claims == nil || claims.SubjectType != middleware.SubjectTypeUser || !claims.HasRole("admin") {
		apierrors.Forbidden(w, "Недостаточно прав: требуется роль admin")
		return
	}

	var req generated.StorageElementUpdate
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierrors.ValidationError(w, "Некорректный JSON: "+err.Error())
		return
	}

	se, err := h.storageElems.Update(r.Context(), id.String(), req.Name, req.Url)
	if err != nil {
		if errors.Is(err, service.ErrNotFound) {
			apierrors.NotFound(w, "Storage Element не найден")
			return
		}
		if errors.Is(err, service.ErrConflict) {
			apierrors.Conflict(w, err.Error())
			return
		}
		h.logger.Error("Ошибка обновления SE", "se_id", id, "error", err)
		apierrors.InternalError(w, "Ошибка обновления Storage Element")
		return
	}

	writeJSON(w, http.StatusOK, mapStorageElement(se))
}

// DeleteStorageElement — DELETE /api/v1/storage-elements/{id}.
// Удаляет SE из реестра. Физические файлы не удаляются.
// Доступ: admin.
func (h *APIHandler) DeleteStorageElement(w http.ResponseWriter, r *http.Request, id generated.StorageElementId) {
	claims := middleware.ClaimsFromContext(r.Context())
	if claims == nil || claims.SubjectType != middleware.SubjectTypeUser || !claims.HasRole("admin") {
		apierrors.Forbidden(w, "Недостаточно прав: требуется роль admin")
		return
	}

	if err := h.storageElems.Delete(r.Context(), id.String()); err != nil {
		if errors.Is(err, service.ErrNotFound) {
			apierrors.NotFound(w, "Storage Element не найден")
			return
		}
		h.logger.Error("Ошибка удаления SE", "se_id", id, "error", err)
		apierrors.InternalError(w, "Ошибка удаления Storage Element")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// SyncStorageElement — POST /api/v1/storage-elements/{id}/sync.
// Полная синхронизация SE (info + файлы).
// Доступ: admin или SA с scope storage:write.
func (h *APIHandler) SyncStorageElement(w http.ResponseWriter, r *http.Request, id generated.StorageElementId) {
	claims := middleware.ClaimsFromContext(r.Context())
	if claims == nil {
		apierrors.Unauthorized(w, "Отсутствуют claims")
		return
	}

	switch claims.SubjectType {
	case middleware.SubjectTypeUser:
		if !claims.HasRole("admin") {
			apierrors.Forbidden(w, "Недостаточно прав: требуется роль admin")
			return
		}
	case middleware.SubjectTypeSA:
		if !claims.HasScope("storage:write") {
			apierrors.Forbidden(w, "Недостаточно прав: требуется scope storage:write")
			return
		}
	default:
		apierrors.Forbidden(w, "Неизвестный тип субъекта")
		return
	}

	syncResult, err := h.storageElems.Sync(r.Context(), id.String())
	if err != nil {
		if errors.Is(err, service.ErrNotFound) {
			apierrors.NotFound(w, "Storage Element не найден")
			return
		}
		if errors.Is(err, service.ErrSEUnavailable) {
			apierrors.SEUnavailable(w, err.Error())
			return
		}
		h.logger.Error("Ошибка sync SE", "se_id", id, "error", err)
		apierrors.InternalError(w, "Ошибка синхронизации Storage Element")
		return
	}

	// Получаем обновлённый SE
	se, err := h.storageElems.Get(r.Context(), id.String())
	if err != nil {
		h.logger.Error("Ошибка получения SE после sync", "se_id", id, "error", err)
		apierrors.InternalError(w, "Ошибка получения SE после синхронизации")
		return
	}

	resp := generated.SyncResponse{
		StorageElement: mapStorageElement(se),
	}
	resp.FileSync.FilesOnSe = syncResult.FilesOnSE
	resp.FileSync.FilesAdded = syncResult.FilesAdded
	resp.FileSync.FilesUpdated = syncResult.FilesUpdated
	resp.FileSync.FilesMarkedDeleted = syncResult.FilesMarkedDeleted
	resp.FileSync.StartedAt = syncResult.StartedAt
	resp.FileSync.CompletedAt = syncResult.CompletedAt

	writeJSON(w, http.StatusOK, resp)
}

// --- Маппинг domain → API ---

// mapStorageElement конвертирует domain model в generated API type.
func mapStorageElement(se *model.StorageElement) generated.StorageElement {
	result := generated.StorageElement{
		Id:            uuid.MustParse(se.ID),
		Name:          se.Name,
		Url:           se.URL,
		StorageId:     se.StorageID,
		Mode:          generated.StorageElementMode(se.Mode),
		Status:        generated.StorageElementStatus(se.Status),
		CapacityBytes: se.CapacityBytes,
		UsedBytes:     se.UsedBytes,
		CreatedAt:     se.CreatedAt,
		UpdatedAt:     se.UpdatedAt,
	}

	result.AvailableBytes = se.AvailableBytes
	result.LastSyncAt = se.LastSyncAt
	result.LastFileSyncAt = se.LastFileSyncAt

	return result
}
