// delete.go — обработчик DELETE /api/v1/files/{file_id}.
// Авторизация, парсинг параметров, вызов delete pipeline, error mapping.
package handlers

import (
	"errors"
	"log/slog"
	"net/http"

	apierrors "github.com/bigkaa/goartstore/ingester-module/internal/api/errors"
	"github.com/bigkaa/goartstore/ingester-module/internal/api/generated"
	"github.com/bigkaa/goartstore/ingester-module/internal/service"
)

// DeleteFile — удаление файла (DELETE /api/v1/files/{file_id}).
func (h *APIHandler) DeleteFile(w http.ResponseWriter, r *http.Request, fileID generated.FileId, params generated.DeleteFileParams) {
	if h.deleteService == nil {
		apierrors.WriteError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED",
			"Delete endpoint ещё не реализован")
		return
	}
	h.handleDeleteFile(w, r, fileID, params)
}

// handleDeleteFile — реализация DELETE /api/v1/files/{file_id}.
//
// Pipeline:
//  1. Авторизация: role admin ИЛИ scope files:write
//  2. Парсинг file_id (path) + storage_element_id (query)
//  3. Вызов deleteService.Delete
//  4. Ответ: 204 No Content / error
func (h *APIHandler) handleDeleteFile(w http.ResponseWriter, r *http.Request, fileID generated.FileId, params generated.DeleteFileParams) {
	// 1. Авторизация
	if !h.checkAuth(w, r) {
		return
	}

	// 2. Параметры уже распарсены oapi-codegen
	fileIDStr := fileID.String()
	seIDStr := params.StorageElementId.String()

	if fileIDStr == "" || fileIDStr == "00000000-0000-0000-0000-000000000000" {
		apierrors.ValidationError(w, "Некорректный file_id")
		return
	}
	if seIDStr == "" || seIDStr == "00000000-0000-0000-0000-000000000000" {
		apierrors.ValidationError(w, "Некорректный storage_element_id")
		return
	}

	// 3. Вызов delete pipeline
	if deleteErr := h.deleteService.Delete(r.Context(), fileIDStr, seIDStr); deleteErr != nil {
		h.handleDeleteError(w, deleteErr)
		return
	}

	// 4. Ответ 204 No Content
	w.WriteHeader(http.StatusNoContent)
}

// handleDeleteError маппит ошибки delete service на HTTP-ответы.
func (h *APIHandler) handleDeleteError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, service.ErrDeleteFileNotFound):
		apierrors.FileNotFound(w, "Файл не найден на Storage Element")
	case errors.Is(err, service.ErrDeleteModeNotAllowed):
		apierrors.ModeNotAllowed(w, "Удаление возможно только для SE в режиме edit")
	case errors.Is(err, service.ErrDeleteUploadInProgress):
		apierrors.FileUploadInProgress(w, "Файл находится в процессе загрузки")
	case errors.Is(err, service.ErrSENotFound):
		apierrors.SENotFound(w, "Storage Element не найден в реестре Admin Module")
	case errors.Is(err, service.ErrSEDeleteFailed):
		apierrors.SEDeleteFailed(w, err.Error())
	case errors.Is(err, service.ErrAMUnavailable):
		apierrors.AdminUnavailable(w, err.Error())
	default:
		h.logger.Error("Неожиданная ошибка delete",
			slog.String("error", err.Error()),
		)
		apierrors.InternalError(w, "Внутренняя ошибка при удалении файла")
	}
}
