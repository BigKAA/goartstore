// files.go — HTTP handlers для файловых операций Storage Element.
// Upload, Download, List, Get metadata, Update metadata, Delete.
package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/bigkaa/goartstore/storage-element/internal/api/errors"
	"github.com/bigkaa/goartstore/storage-element/internal/api/generated"
	"github.com/bigkaa/goartstore/storage-element/internal/api/middleware"
	"github.com/bigkaa/goartstore/storage-element/internal/backend"
	"github.com/bigkaa/goartstore/storage-element/internal/domain/mode"
	"github.com/bigkaa/goartstore/storage-element/internal/domain/model"
	"github.com/bigkaa/goartstore/storage-element/internal/service"
	"github.com/bigkaa/goartstore/storage-element/internal/storage/index"
)

// FilesHandler — обработчик файловых endpoints.
type FilesHandler struct {
	uploadSvc   *service.UploadService
	downloadSvc *service.DownloadService
	files       backend.FileStore
	attrs       backend.AttrStore
	idx         *index.Index
	sm          *mode.StateMachine
	locks       backend.LockStore
}

// NewFilesHandler создаёт обработчик файловых endpoints.
func NewFilesHandler(
	uploadSvc *service.UploadService,
	downloadSvc *service.DownloadService,
	files backend.FileStore,
	attrs backend.AttrStore,
	idx *index.Index,
	sm *mode.StateMachine,
	locks backend.LockStore,
) *FilesHandler {
	return &FilesHandler{
		uploadSvc:   uploadSvc,
		downloadSvc: downloadSvc,
		files:       files,
		attrs:       attrs,
		idx:         idx,
		sm:          sm,
		locks:       locks,
	}
}

// UploadFile обрабатывает POST /api/v1/files/upload.
// Multipart form: file (обязательно), description (опционально), tags (опционально, JSON).
func (h *FilesHandler) UploadFile(w http.ResponseWriter, r *http.Request) {
	// Извлекаем subject из JWT контекста
	subject := middleware.SubjectFromContext(r.Context())

	// Парсим multipart form (ограничение по MaxFileSize + запас на заголовки)
	if err := r.ParseMultipartForm(32 << 20); err != nil { // 32 MB buffer
		errors.ValidationError(w, fmt.Sprintf("Ошибка парсинга multipart: %s", err.Error()))
		return
	}

	// Извлекаем файл
	file, header, err := r.FormFile("file")
	if err != nil {
		errors.ValidationError(w, "Поле 'file' обязательно")
		return
	}
	defer file.Close()

	// Определяем Content-Type
	contentType := header.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	// Извлекаем опциональные поля
	description := r.FormValue("description")
	tagsJSON := r.FormValue("tags")

	// Вызываем сервис загрузки
	result, uploadErr := h.uploadSvc.Upload(service.UploadParams{
		Reader:           file,
		OriginalFilename: header.Filename,
		ContentType:      contentType,
		Size:             header.Size,
		UploadedBy:       subject,
		Description:      description,
		TagsJSON:         tagsJSON,
	})

	if uploadErr != nil {
		errors.WriteError(w, uploadErr.StatusCode, uploadErr.Code, uploadErr.Message)
		return
	}

	// Формируем ответ
	resp := domainToAPIMetadata(result.Metadata)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(resp)
}

// DownloadFile обрабатывает GET /api/v1/files/{file_id}/download.
// Поддерживает Range requests (206) и ETag (If-None-Match → 304).
func (h *FilesHandler) DownloadFile(w http.ResponseWriter, r *http.Request, fileId generated.FileId, _ generated.DownloadFileParams) { //nolint:revive // имя fileId задано сгенерированным интерфейсом
	downloadErr := h.downloadSvc.Serve(w, r, fileId.String())
	if downloadErr != nil {
		errors.WriteError(w, downloadErr.StatusCode, downloadErr.Code, downloadErr.Message)
	}
}

// ListFiles обрабатывает GET /api/v1/files.
// Пагинация: limit, offset. Фильтр: retention_policy.
func (h *FilesHandler) ListFiles(w http.ResponseWriter, _ *http.Request, params generated.ListFilesParams) {
	// Значения по умолчанию
	limit := 50
	offset := 0
	var retentionFilter model.RetentionPolicy

	if params.Limit != nil {
		limit = *params.Limit
		if limit <= 0 || limit > 1000 {
			errors.ValidationError(w, "Параметр limit должен быть от 1 до 1000")
			return
		}
	}

	if params.Offset != nil {
		offset = *params.Offset
		if offset < 0 {
			errors.ValidationError(w, "Параметр offset не может быть отрицательным")
			return
		}
	}

	if params.RetentionPolicy != nil {
		retentionFilter = model.RetentionPolicy(string(*params.RetentionPolicy))
		// Валидация retention_policy
		switch retentionFilter {
		case model.RetentionPermanent, model.RetentionTemporary:
			// ok
		default:
			errors.ValidationError(w, fmt.Sprintf("Недопустимая политика хранения: %s", retentionFilter))
			return
		}
	}

	// Получаем данные из индекса
	items, total := h.idx.List(limit, offset, retentionFilter)

	// Преобразуем в API-формат
	apiItems := make([]generated.FileMetadata, 0, len(items))
	for _, item := range items {
		apiItems = append(apiItems, domainToAPIMetadata(item))
	}

	hasMore := offset+limit < total

	resp := generated.FileListResponse{
		Items:   apiItems,
		Total:   total,
		Limit:   limit,
		Offset:  offset,
		HasMore: hasMore,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

// GetFileMetadata обрабатывает GET /api/v1/files/{file_id}.
func (h *FilesHandler) GetFileMetadata(w http.ResponseWriter, _ *http.Request, fileId generated.FileId) { //nolint:revive // имя fileId задано сгенерированным интерфейсом
	meta := h.idx.Get(fileId.String())
	if meta == nil {
		errors.NotFound(w, fmt.Sprintf("Файл %s не найден", fileId.String()))
		return
	}

	resp := domainToAPIMetadata(meta)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

// UpdateFileMetadata обрабатывает PATCH /api/v1/files/{file_id}.
// Обновляет description и/или tags.
func (h *FilesHandler) UpdateFileMetadata(w http.ResponseWriter, r *http.Request, fileId generated.FileId) { //nolint:revive // имя fileId задано сгенерированным интерфейсом
	ctx := r.Context()

	// Проверяем допустимость update
	if !h.sm.CanPerform(mode.OpUpdate) {
		errors.ModeNotAllowed(w, fmt.Sprintf("Обновление метаданных недоступно в режиме %s", h.sm.CurrentMode()))
		return
	}

	// Парсим тело запроса
	var req generated.FileMetadataUpdate
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errors.ValidationError(w, fmt.Sprintf("Некорректный JSON: %s", err.Error()))
		return
	}

	// Проверяем, что хотя бы одно поле указано
	if req.Description == nil && req.Tags == nil {
		errors.ValidationError(w, "Необходимо указать хотя бы одно поле для обновления (description или tags)")
		return
	}

	// Получаем метаданные из индекса
	meta := h.idx.Get(fileId.String())
	if meta == nil {
		errors.NotFound(w, fmt.Sprintf("Файл %s не найден", fileId.String()))
		return
	}

	// Обновляем поля
	if req.Description != nil {
		meta.Description = *req.Description
	}
	if req.Tags != nil {
		meta.Tags = *req.Tags
	}

	// Записываем обновлённый attr.json через AttrStore
	if err := h.attrs.Write(ctx, meta.StoragePath, meta); err != nil {
		errors.InternalError(w, "Ошибка обновления метаданных на диске")
		return
	}

	// Обновляем индекс
	_ = h.idx.Update(meta)

	// Метрики
	middleware.OperationsTotal.WithLabelValues("update", "success").Inc()

	resp := domainToAPIMetadata(meta)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

// DeleteFile обрабатывает DELETE /api/v1/files/{file_id}.
// Hard delete: физически удаляет файл, attr.json и запись из индекса.
// Доступно только в режиме edit.
//
// Если файл в данный момент загружается (lock активен) — возвращает 409 Conflict
// с кодом FILE_UPLOAD_IN_PROGRESS. Клиент должен повторить DELETE после завершения upload-а.
func (h *FilesHandler) DeleteFile(w http.ResponseWriter, r *http.Request, fileId generated.FileId) { //nolint:revive // имя fileId задано сгенерированным интерфейсом
	ctx := r.Context()

	// Проверяем допустимость delete
	if !h.sm.CanPerform(mode.OpDelete) {
		errors.ModeNotAllowed(w, fmt.Sprintf("Удаление файлов недоступно в режиме %s", h.sm.CurrentMode()))
		return
	}

	// Получаем метаданные
	meta := h.idx.Get(fileId.String())
	if meta == nil {
		errors.NotFound(w, fmt.Sprintf("Файл %s не найден", fileId.String()))
		return
	}

	// Проверяем lock: если файл загружается — 409 Conflict
	if locked, lockInfo, _ := h.locks.IsLocked(ctx, meta.FileID); locked {
		errors.WriteError(w, http.StatusConflict,
			errors.CodeFileUploadInProgress,
			fmt.Sprintf("Невозможно удалить файл %s: upload в процессе (holder: %s, expires_at: %s)",
				fileId.String(), lockInfo.Holder, lockInfo.ExpiresAt().Format("2006-01-02T15:04:05Z")),
		)
		return
	}

	// Физическое удаление файла данных
	if err := h.files.DeleteFile(ctx, meta.StoragePath); err != nil {
		errors.InternalError(w, "Ошибка удаления файла с диска")
		return
	}

	// Удаляем attr.json
	if err := h.attrs.Delete(ctx, meta.StoragePath); err != nil {
		// Файл уже удалён, attr.json — не критично
	}

	// Удаляем из индекса
	h.idx.Remove(meta.FileID)

	// Метрики
	middleware.OperationsTotal.WithLabelValues("delete", "success").Inc()
	middleware.FilesTotal.Set(float64(h.idx.Count()))

	w.WriteHeader(http.StatusNoContent)
}

// domainToAPIMetadata преобразует доменную модель в API-формат.
// Исключает поле StoragePath (внутреннее).
func domainToAPIMetadata(m *model.FileMetadata) generated.FileMetadata {
	fileID := openapi_types.UUID{}
	_ = fileID.UnmarshalText([]byte(m.FileID))

	result := generated.FileMetadata{
		FileId:           fileID,
		OriginalFilename: m.OriginalFilename,
		ContentType:      m.ContentType,
		Size:             m.Size,
		Checksum:         m.Checksum,
		UploadedBy:       m.UploadedBy,
		UploadedAt:       m.UploadedAt,
		RetentionPolicy:  generated.FileMetadataRetentionPolicy(m.RetentionPolicy),
		TtlDays:          m.TTLDays,
		ExpiresAt:        m.ExpiresAt,
	}

	// Описание
	if m.Description != "" {
		desc := m.Description
		result.Description = &desc
	}

	// Теги
	if len(m.Tags) > 0 {
		tags := make([]string, len(m.Tags))
		copy(tags, m.Tags)
		result.Tags = &tags
	}

	return result
}
