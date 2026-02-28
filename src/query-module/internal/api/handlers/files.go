// files.go — обработчик GET /api/v1/files/{file_id}.
// Получение метаданных одного файла по UUID.
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
