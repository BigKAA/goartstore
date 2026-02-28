// files.go — обработчики /api/v1/files endpoints.
// Файловый реестр: регистрация, список, получение, обновление, soft delete.
package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"

	apierrors "github.com/bigkaa/goartstore/admin-module/internal/api/errors"
	"github.com/bigkaa/goartstore/admin-module/internal/api/generated"
	"github.com/bigkaa/goartstore/admin-module/internal/api/middleware"
	"github.com/bigkaa/goartstore/admin-module/internal/domain/model"
	"github.com/bigkaa/goartstore/admin-module/internal/repository"
	"github.com/bigkaa/goartstore/admin-module/internal/service"
)

// RegisterFile — POST /api/v1/files.
// Регистрация файла после загрузки на SE.
// Доступ: SA с scope files:write.
//
//nolint:cyclop // TODO: упростить RegisterFile
func (h *APIHandler) RegisterFile(w http.ResponseWriter, r *http.Request) {
	claims := middleware.ClaimsFromContext(r.Context())
	if claims == nil {
		apierrors.Unauthorized(w, "Отсутствуют claims")
		return
	}

	// Доступ: SA с files:write или admin user
	switch claims.SubjectType {
	case middleware.SubjectTypeSA:
		if !claims.HasScope("files:write") {
			apierrors.Forbidden(w, "Недостаточно прав: требуется scope files:write")
			return
		}
	case middleware.SubjectTypeUser:
		if !claims.HasRole("admin") {
			apierrors.Forbidden(w, "Недостаточно прав: требуется роль admin")
			return
		}
	default:
		apierrors.Forbidden(w, "Неизвестный тип субъекта")
		return
	}

	var req generated.FileRegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierrors.ValidationError(w, "Некорректный JSON: "+err.Error())
		return
	}

	// Валидация
	if req.OriginalFilename == "" {
		apierrors.ValidationError(w, "Имя файла (original_filename) обязательно")
		return
	}
	if req.ContentType == "" {
		apierrors.ValidationError(w, "Тип содержимого (content_type) обязателен")
		return
	}
	if req.Size <= 0 {
		apierrors.ValidationError(w, "Размер файла должен быть положительным")
		return
	}
	if req.Checksum == "" {
		apierrors.ValidationError(w, "Контрольная сумма (checksum) обязательна")
		return
	}
	if req.RetentionPolicy == generated.FileRegisterRequestRetentionPolicyTemporary {
		if req.TtlDays == nil || *req.TtlDays <= 0 || *req.TtlDays > 365 {
			apierrors.ValidationError(w, "Для temporary файлов ttl_days должен быть от 1 до 365")
			return
		}
	}

	// Определяем uploaded_by
	uploadedBy := claims.PreferredUsername
	if claims.SubjectType == middleware.SubjectTypeSA {
		uploadedBy = claims.ClientID
	}

	// Маппинг в domain model
	f := &model.FileRecord{
		FileID:           req.FileId.String(),
		OriginalFilename: req.OriginalFilename,
		ContentType:      req.ContentType,
		Size:             req.Size,
		Checksum:         req.Checksum,
		StorageElementID: req.StorageElementId.String(),
		UploadedBy:       uploadedBy,
		UploadedAt:       time.Now().UTC(),
		Status:           "active",
		RetentionPolicy:  string(req.RetentionPolicy),
		TTLDays:          req.TtlDays,
	}

	if req.Description != nil {
		f.Description = req.Description
	}
	if req.Tags != nil {
		f.Tags = *req.Tags
	}

	if err := h.files.Register(r.Context(), f); err != nil {
		if errors.Is(err, service.ErrConflict) {
			apierrors.Conflict(w, err.Error())
			return
		}
		if errors.Is(err, service.ErrValidation) {
			apierrors.ValidationError(w, err.Error())
			return
		}
		h.logger.Error("Ошибка регистрации файла", "file_id", req.FileId, "error", err)
		apierrors.InternalError(w, "Ошибка регистрации файла")
		return
	}

	writeJSON(w, http.StatusCreated, mapFileRecord(f))
}

// ListFiles — GET /api/v1/files.
// Возвращает список файлов с фильтрацией и пагинацией.
// Доступ: admin, readonly или SA с scope files:read.
func (h *APIHandler) ListFiles(w http.ResponseWriter, r *http.Request, params generated.ListFilesParams) {
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
		if !claims.HasScope("files:read") {
			apierrors.Forbidden(w, "Недостаточно прав: требуется scope files:read")
			return
		}
	default:
		apierrors.Forbidden(w, "Неизвестный тип субъекта")
		return
	}

	limit, offset := paginationDefaults(params.Limit, params.Offset)

	// Формируем фильтры
	filters := repository.FileListFilters{}
	if params.Status != nil {
		s := string(*params.Status)
		filters.Status = &s
	}
	if params.RetentionPolicy != nil {
		s := string(*params.RetentionPolicy)
		filters.RetentionPolicy = &s
	}
	if params.StorageElementId != nil {
		s := params.StorageElementId.String()
		filters.StorageElementID = &s
	}
	if params.UploadedBy != nil {
		filters.UploadedBy = params.UploadedBy
	}

	files, total, err := h.files.List(r.Context(), filters, limit, offset)
	if err != nil {
		h.logger.Error("Ошибка получения списка файлов", "error", err)
		apierrors.InternalError(w, "Ошибка получения списка файлов")
		return
	}

	items := make([]generated.FileRecord, len(files))
	for i, f := range files {
		items[i] = mapFileRecord(f)
	}

	resp := generated.FileRecordListResponse{
		Items:   items,
		Total:   total,
		Limit:   limit,
		Offset:  offset,
		HasMore: offset+limit < total,
	}

	writeJSON(w, http.StatusOK, resp)
}

// GetFile — GET /api/v1/files/{file_id}.
// Возвращает метаданные файла.
// Доступ: admin, readonly или SA с scope files:read.
//
//nolint:dupl // TODO: вынести общую логику проверки прав
func (h *APIHandler) GetFile(w http.ResponseWriter, r *http.Request, fileId generated.FileId) { //nolint:revive // имя из сгенерированного интерфейса oapi-codegen
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
		if !claims.HasScope("files:read") {
			apierrors.Forbidden(w, "Недостаточно прав: требуется scope files:read")
			return
		}
	default:
		apierrors.Forbidden(w, "Неизвестный тип субъекта")
		return
	}

	f, err := h.files.Get(r.Context(), fileId.String())
	if err != nil {
		if errors.Is(err, service.ErrNotFound) {
			apierrors.NotFound(w, "Файл не найден")
			return
		}
		h.logger.Error("Ошибка получения файла", "file_id", fileId, "error", err)
		apierrors.InternalError(w, "Ошибка получения файла")
		return
	}

	writeJSON(w, http.StatusOK, mapFileRecord(f))
}

// UpdateFile — PUT /api/v1/files/{file_id}.
// Обновляет метаданные файла (description, tags, status).
// Доступ: admin или SA с scope files:write.
func (h *APIHandler) UpdateFile(w http.ResponseWriter, r *http.Request, fileId generated.FileId) { //nolint:revive // имя из сгенерированного интерфейса oapi-codegen
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
		if !claims.HasScope("files:write") {
			apierrors.Forbidden(w, "Недостаточно прав: требуется scope files:write")
			return
		}
	default:
		apierrors.Forbidden(w, "Неизвестный тип субъекта")
		return
	}

	var req generated.FileRecordUpdate
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierrors.ValidationError(w, "Некорректный JSON: "+err.Error())
		return
	}

	var status *string
	if req.Status != nil {
		s := string(*req.Status)
		status = &s
	}

	f, err := h.files.Update(r.Context(), fileId.String(), req.Description, req.Tags, status)
	if err != nil {
		if errors.Is(err, service.ErrNotFound) {
			apierrors.NotFound(w, "Файл не найден")
			return
		}
		h.logger.Error("Ошибка обновления файла", "file_id", fileId, "error", err)
		apierrors.InternalError(w, "Ошибка обновления файла")
		return
	}

	writeJSON(w, http.StatusOK, mapFileRecord(f))
}

// DeleteFile — DELETE /api/v1/files/{file_id}.
// Soft delete файла (status → deleted).
// Доступ: admin или SA с scope files:write.
func (h *APIHandler) DeleteFile(w http.ResponseWriter, r *http.Request, fileId generated.FileId) { //nolint:revive // имя из сгенерированного интерфейса oapi-codegen
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
		if !claims.HasScope("files:write") {
			apierrors.Forbidden(w, "Недостаточно прав: требуется scope files:write")
			return
		}
	default:
		apierrors.Forbidden(w, "Неизвестный тип субъекта")
		return
	}

	if err := h.files.Delete(r.Context(), fileId.String()); err != nil {
		if errors.Is(err, service.ErrNotFound) {
			apierrors.NotFound(w, "Файл не найден")
			return
		}
		h.logger.Error("Ошибка удаления файла", "file_id", fileId, "error", err)
		apierrors.InternalError(w, "Ошибка удаления файла")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// --- Маппинг domain → API ---

// mapFileRecord конвертирует domain model в generated API type.
func mapFileRecord(f *model.FileRecord) generated.FileRecord {
	result := generated.FileRecord{
		FileId:           uuid.MustParse(f.FileID),
		OriginalFilename: f.OriginalFilename,
		ContentType:      f.ContentType,
		Size:             f.Size,
		Checksum:         f.Checksum,
		StorageElementId: uuid.MustParse(f.StorageElementID),
		UploadedBy:       f.UploadedBy,
		UploadedAt:       f.UploadedAt,
		Status:           generated.FileRecordStatus(f.Status),
		RetentionPolicy:  generated.FileRecordRetentionPolicy(f.RetentionPolicy),
	}

	result.Description = f.Description
	result.TtlDays = f.TTLDays
	result.ExpiresAt = f.ExpiresAt

	if !f.CreatedAt.IsZero() {
		result.CreatedAt = &f.CreatedAt
	}
	if !f.UpdatedAt.IsZero() {
		result.UpdatedAt = &f.UpdatedAt
	}

	if len(f.Tags) > 0 {
		tags := f.Tags
		result.Tags = &tags
	}

	return result
}
