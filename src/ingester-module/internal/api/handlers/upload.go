// upload.go — обработчик POST /api/v1/files/upload.
// Парсинг multipart, авторизация, вызов upload pipeline, error mapping.
package handlers

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	openapi_types "github.com/oapi-codegen/runtime/types"

	apierrors "github.com/bigkaa/goartstore/ingester-module/internal/api/errors"
	"github.com/bigkaa/goartstore/ingester-module/internal/api/generated"
	"github.com/bigkaa/goartstore/ingester-module/internal/api/middleware"
	"github.com/bigkaa/goartstore/ingester-module/internal/service"
)

// handleUploadFile — реализация POST /api/v1/files/upload.
//
// Pipeline:
//  1. Авторизация: role admin ИЛИ scope files:write
//  2. Извлечение uploaded_by из JWT sub
//  3. Парсинг multipart form
//  4. Извлечение файла и form-полей
//  5. Вызов uploadService.Upload
//  6. Ответ: 201 Created + UploadResponse
func (h *APIHandler) handleUploadFile(w http.ResponseWriter, r *http.Request) {
	// 1. Авторизация
	if !h.checkAuth(w, r) {
		return
	}

	// 2. Извлечение uploaded_by из JWT claims
	claims := middleware.ClaimsFromContext(r.Context())
	uploadedBy := claims.Subject

	// 3. Парсинг multipart form (32 MB буфер для заголовков, файл сохраняется на диск)
	if parseErr := r.ParseMultipartForm(32 << 20); parseErr != nil {
		apierrors.ValidationError(w, "Некорректный multipart запрос: "+parseErr.Error())
		return
	}

	// 4a. Извлечение файла
	file, header, fileErr := r.FormFile("file")
	if fileErr != nil {
		apierrors.ValidationError(w, "Поле 'file' обязательно: "+fileErr.Error())
		return
	}
	defer file.Close()

	// 4b-4e. Извлечение form-полей
	params, ok := h.parseUploadParams(w, r, uploadedBy)
	if !ok {
		return
	}

	// 5. Вызов upload pipeline
	resp, uploadErr := h.uploadService.Upload(r.Context(), file, header, params)
	if uploadErr != nil {
		h.handleUploadError(w, uploadErr)
		return
	}

	// 6. Ответ 201 Created
	apiResp := mapUploadResponse(resp)
	writeJSON(w, http.StatusCreated, apiResp)
}

// parseUploadParams извлекает параметры upload из form-полей.
// Возвращает false и пишет ошибку в w при невалидных данных.
func (h *APIHandler) parseUploadParams(w http.ResponseWriter, r *http.Request, uploadedBy string) (service.UploadParams, bool) {
	// retention_policy
	retentionPolicy := r.FormValue("retention_policy")
	if retentionPolicy == "" {
		retentionPolicy = service.RetentionTemporary
	}

	// ttl_days (опционально)
	var ttlDays *int
	if ttlStr := r.FormValue("ttl_days"); ttlStr != "" {
		ttl, parseErr := strconv.Atoi(ttlStr)
		if parseErr != nil {
			apierrors.ValidationError(w, "Некорректное значение ttl_days: "+ttlStr)
			return service.UploadParams{}, false
		}
		ttlDays = &ttl
	}

	// description (опционально)
	var description *string
	if desc := r.FormValue("description"); desc != "" {
		description = &desc
	}

	// tags (JSON string → []string)
	var tags []string
	if tagsStr := r.FormValue("tags"); tagsStr != "" {
		if unmarshalErr := json.Unmarshal([]byte(tagsStr), &tags); unmarshalErr != nil {
			apierrors.ValidationError(w, "Поле 'tags' должно быть JSON-массивом строк: "+unmarshalErr.Error())
			return service.UploadParams{}, false
		}
	}

	return service.UploadParams{
		UploadedBy:      uploadedBy,
		Description:     description,
		Tags:            tags,
		RetentionPolicy: retentionPolicy,
		TTLDays:         ttlDays,
	}, true
}

// handleUploadError маппит ошибки upload service на HTTP-ответы.
func (h *APIHandler) handleUploadError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, service.ErrValidation):
		apierrors.ValidationError(w, err.Error())
	case errors.Is(err, service.ErrFileTooLarge):
		apierrors.FileTooLarge(w, err.Error())
	case errors.Is(err, service.ErrNoStorageAvailable):
		apierrors.NoStorageAvailable(w, "Нет доступных Storage Elements с достаточным местом")
	case errors.Is(err, service.ErrStorageFull):
		apierrors.StorageFull(w, "Все Storage Elements заполнены (retry исчерпаны)")
	case errors.Is(err, service.ErrSEUploadFailed):
		apierrors.SEUploadFailed(w, err.Error())
	case errors.Is(err, service.ErrAMUnavailable):
		apierrors.AdminUnavailable(w, err.Error())
	default:
		h.logger.Error("Неожиданная ошибка upload",
			slog.String("error", err.Error()),
		)
		apierrors.InternalError(w, "Внутренняя ошибка при загрузке файла")
	}
}

// mapUploadResponse конвертирует service.UploadResponse в generated.UploadResponse.
func mapUploadResponse(resp *service.UploadResponse) generated.UploadResponse {
	seID := openapi_types.UUID{}
	_ = seID.Scan(resp.StorageElementID)

	apiResp := generated.UploadResponse{
		FileId:           openapi_types.UUID{},
		OriginalFilename: resp.OriginalFilename,
		ContentType:      resp.ContentType,
		Size:             resp.Size,
		Checksum:         resp.Checksum,
		UploadedBy:       resp.UploadedBy,
		UploadedAt:       resp.UploadedAt,
		Description:     resp.Description,
		RetentionPolicy: generated.UploadResponseRetentionPolicy(resp.RetentionPolicy),
		TtlDays:          resp.TTLDays,
		ExpiresAt:        resp.ExpiresAt,
		StorageElementId: &seID,
	}
	_ = apiResp.FileId.Scan(resp.FileID)

	if len(resp.Tags) > 0 {
		apiResp.Tags = &resp.Tags
	}

	return apiResp
}
