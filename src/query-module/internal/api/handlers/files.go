// files.go — обработчики файлов:
// GET /api/v1/files/{file_id} — метаданные файла
// GET /api/v1/files/{file_id}/download — proxy download из Storage Element
package handlers

import (
	"errors"
	"log/slog"
	"net/http"

	apierrors "github.com/bigkaa/goartstore/query-module/internal/api/errors"
	"github.com/bigkaa/goartstore/query-module/internal/api/generated"
	"github.com/bigkaa/goartstore/query-module/internal/service"
)

// handleGetFileMetadata — реализация GET /api/v1/files/{file_id}.
// Авторизация: RequireRoleOrScope (admin, readonly / files:read) — на уровне middleware.
func (h *APIHandler) handleGetFileMetadata(w http.ResponseWriter, r *http.Request, fileID generated.FileId) {
	record, err := h.searchService.GetFileMetadata(r.Context(), fileID.String())
	if err != nil {
		if errors.Is(err, service.ErrNotFound) {
			apierrors.NotFound(w, "Файл не найден")
			return
		}
		h.logger.Error("Ошибка получения метаданных файла",
			slog.String("file_id", fileID.String()),
			slog.String("error", err.Error()),
		)
		apierrors.InternalError(w, "Внутренняя ошибка при получении метаданных файла")
		return
	}

	// Конвертация domain модели в API-тип FileMetadata
	resp := generated.FileMetadata{
		FileId:           parseUUID(record.FileID),
		OriginalFilename: record.OriginalFilename,
		ContentType:      record.ContentType,
		Size:             record.Size,
		Checksum:         record.Checksum,
		UploadedBy:       record.UploadedBy,
		UploadedAt:       record.UploadedAt,
		Description:      record.Description,
		Tags:             tagsToPtr(record.Tags),
		Status:           generated.FileMetadataStatus(record.Status),
		RetentionPolicy:  generated.FileMetadataRetentionPolicy(record.RetentionPolicy),
		TtlDays:          record.TTLDays,
		ExpiresAt:        record.ExpiresAt,
	}

	writeJSON(w, http.StatusOK, resp)
}

// handleDownloadFile — реализация GET /api/v1/files/{file_id}/download.
// Проксирует скачивание файла из Storage Element через DownloadService.
// Поддерживает HTTP Range requests (частичная загрузка).
func (h *APIHandler) handleDownloadFile(w http.ResponseWriter, r *http.Request, fileID generated.FileId, params generated.DownloadFileParams) {
	// Извлекаем Range header (если передан через oapi-codegen params)
	rangeHeader := ""
	if params.Range != nil {
		rangeHeader = *params.Range
	}

	// Вызываем download service — полный pipeline (кэш → AM → SE → streaming)
	err := h.downloadService.Download(r.Context(), w, fileID.String(), rangeHeader)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrNotFound):
			apierrors.NotFound(w, "Файл не найден")
		case errors.Is(err, service.ErrFileDeleted):
			apierrors.NotFound(w, "Файл не найден на Storage Element (удалён)")
		default:
			h.logger.Error("Ошибка скачивания файла",
				slog.String("file_id", fileID.String()),
				slog.String("error", err.Error()),
			)

			// Определяем тип ошибки для правильного HTTP-кода
			errMsg := err.Error()
			switch {
			case contains(errMsg, "получение информации о SE", "AM вернул статус", "запрос GetStorageElement"):
				apierrors.AMUnavailable(w, "Admin Module недоступен")
			case contains(errMsg, "запрос Download к", "получение токена для SE"):
				apierrors.SEUnavailable(w, "Storage Element недоступен")
			default:
				apierrors.InternalError(w, "Внутренняя ошибка при скачивании файла")
			}
		}
	}
}

// contains проверяет, содержит ли строка хотя бы одну из подстрок.
func contains(s string, substrs ...string) bool {
	for _, sub := range substrs {
		if len(s) >= len(sub) {
			for i := 0; i <= len(s)-len(sub); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
		}
	}
	return false
}
