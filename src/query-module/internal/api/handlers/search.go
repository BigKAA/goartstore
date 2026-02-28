// search.go — обработчик POST /api/v1/search.
// Десериализация SearchRequest, валидация, вызов service, сериализация SearchResponse.
package handlers

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	apierrors "github.com/bigkaa/goartstore/query-module/internal/api/errors"
	"github.com/bigkaa/goartstore/query-module/internal/api/generated"
	"github.com/bigkaa/goartstore/query-module/internal/domain/model"
	"github.com/bigkaa/goartstore/query-module/internal/repository"
)

// handleSearchFiles — реализация POST /api/v1/search.
// Авторизация: RequireRoleOrScope (admin, readonly / files:read) — на уровне middleware.
func (h *APIHandler) handleSearchFiles(w http.ResponseWriter, r *http.Request) {
	// Десериализация запроса
	var req generated.SearchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierrors.ValidationError(w, "Некорректный JSON в теле запроса")
		return
	}

	// Валидация параметров
	if err := validateSearchRequest(&req); err != nil {
		apierrors.ValidationError(w, err.Error())
		return
	}

	// Нормализация пагинации
	limit, offset := paginationDefaults(req.Limit, req.Offset)

	// Определение режима поиска (по умолчанию partial)
	mode := "partial"
	if req.Mode != nil {
		mode = string(*req.Mode)
	}

	// Определение сортировки (по умолчанию uploaded_at DESC)
	sortBy := "uploaded_at"
	if req.SortBy != nil {
		sortBy = string(*req.SortBy)
	}
	sortOrder := "desc"
	if req.SortOrder != nil {
		sortOrder = string(*req.SortOrder)
	}

	// По умолчанию — только active файлы
	var status *string
	if req.Status != nil {
		s := string(*req.Status)
		status = &s
	} else {
		s := "active"
		status = &s
	}

	// Конвертация retention_policy из typed enum в string
	var retentionPolicy *string
	if req.RetentionPolicy != nil {
		s := string(*req.RetentionPolicy)
		retentionPolicy = &s
	}

	// Построение SearchParams из запроса
	params := repository.SearchParams{
		Query:           req.Query,
		Filename:        req.Filename,
		FileExtension:   req.FileExtension,
		Tags:            req.Tags,
		UploadedBy:      req.UploadedBy,
		RetentionPolicy: retentionPolicy,
		Status:          status,
		MinSize:         req.MinSize,
		MaxSize:         req.MaxSize,
		UploadedAfter:   req.UploadedAfter,
		UploadedBefore:  req.UploadedBefore,
		Mode:            mode,
		SortBy:          sortBy,
		SortOrder:       sortOrder,
		Limit:           limit,
		Offset:          offset,
	}

	// Вызов service
	result, err := h.searchService.Search(r.Context(), params)
	if err != nil {
		h.logger.Error("Ошибка поиска файлов",
			slog.String("error", err.Error()),
		)
		apierrors.InternalError(w, "Внутренняя ошибка при поиске файлов")
		return
	}

	// Сериализация ответа
	resp := generated.SearchResponse{
		Items:   fileRecordsToSearchItems(result.Items),
		Total:   result.Total,
		Limit:   result.Limit,
		Offset:  result.Offset,
		HasMore: result.HasMore,
	}

	writeJSON(w, http.StatusOK, resp)
}

// validateSearchRequest проверяет корректность параметров поиска.
func validateSearchRequest(req *generated.SearchRequest) error {
	// Валидация дат: uploaded_after не может быть позже uploaded_before
	if req.UploadedAfter != nil && req.UploadedBefore != nil {
		if req.UploadedAfter.After(*req.UploadedBefore) {
			return errors.New("uploaded_after не может быть позже uploaded_before")
		}
	}

	// Валидация размеров: min_size не может быть больше max_size
	if req.MinSize != nil && req.MaxSize != nil {
		if *req.MinSize > *req.MaxSize {
			return errors.New("min_size не может быть больше max_size")
		}
	}

	// Валидация размеров: отрицательные значения
	if req.MinSize != nil && *req.MinSize < 0 {
		return errors.New("min_size не может быть отрицательным")
	}
	if req.MaxSize != nil && *req.MaxSize < 0 {
		return errors.New("max_size не может быть отрицательным")
	}

	return nil
}

// fileRecordsToSearchItems конвертирует domain модели в API-тип SearchResultItem.
func fileRecordsToSearchItems(records []*model.FileRecord) []generated.SearchResultItem {
	items := make([]generated.SearchResultItem, 0, len(records))
	for _, r := range records {
		items = append(items, fileRecordToSearchItem(r))
	}
	return items
}

// fileRecordToSearchItem конвертирует одну domain-запись в API SearchResultItem.
func fileRecordToSearchItem(r *model.FileRecord) generated.SearchResultItem {
	return generated.SearchResultItem{
		FileId:           parseUUID(r.FileID),
		OriginalFilename: r.OriginalFilename,
		ContentType:      r.ContentType,
		Size:             r.Size,
		Checksum:         r.Checksum,
		UploadedBy:       r.UploadedBy,
		UploadedAt:       r.UploadedAt,
		Description:      r.Description,
		Tags:             tagsToPtr(r.Tags),
		Status:           generated.SearchResultItemStatus(r.Status),
		RetentionPolicy:  generated.SearchResultItemRetentionPolicy(r.RetentionPolicy),
		TtlDays:          r.TTLDays,
		ExpiresAt:        r.ExpiresAt,
	}
}
