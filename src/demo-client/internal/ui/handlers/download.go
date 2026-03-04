// Package handlers — HTTP-обработчики UI страниц Demo Client.
// download.go — обработчик скачивания файлов и обработка FILE_ARCHIVED (410).
package handlers

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/bigkaa/goartstore/demo-client/internal/gateway"
	"github.com/bigkaa/goartstore/demo-client/internal/service"
	"github.com/bigkaa/goartstore/demo-client/internal/ui/components"
	"github.com/bigkaa/goartstore/demo-client/internal/ui/i18n"
	"github.com/bigkaa/goartstore/demo-client/internal/ui/pages"
)

// DownloadHandler — обработчик скачивания файлов.
type DownloadHandler struct {
	svc    *service.DownloadService
	logger *slog.Logger
}

// NewDownloadHandler создаёт новый обработчик скачивания.
func NewDownloadHandler(svc *service.DownloadService, logger *slog.Logger) *DownloadHandler {
	return &DownloadHandler{
		svc:    svc,
		logger: logger,
	}
}

// Download — GET /files/{fileID}/download — скачивание файла.
// При 410 (FILE_ARCHIVED) возвращает HTML-partial с модалкой.
// При других ошибках — toast с сообщением.
func (h *DownloadHandler) Download(w http.ResponseWriter, r *http.Request) {
	fileID := chi.URLParam(r, "fileID")
	if fileID == "" {
		http.Error(w, "file ID required", http.StatusBadRequest)
		return
	}

	result, err := h.svc.Download(fileID)
	if err != nil {
		h.handleDownloadError(w, r, fileID, err)
		return
	}

	// Файл в архиве — рендерим модалку вместо файла
	if result.Archived != nil {
		h.renderArchivedModal(w, r, result.Archived)
		return
	}

	// Успешное скачивание — streaming файла клиенту
	resp := result.Response
	defer resp.Body.Close()

	// Устанавливаем заголовки для скачивания
	if resp.Filename != "" {
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", resp.Filename))
	}
	if resp.ContentType != "" {
		w.Header().Set("Content-Type", resp.ContentType)
	}
	if resp.ContentLength > 0 {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", resp.ContentLength))
	}
	if resp.ETag != "" {
		w.Header().Set("ETag", resp.ETag)
	}

	// Stream файла
	if _, err := io.Copy(w, resp.Body); err != nil {
		h.logger.Error("ошибка streaming файла",
			"file_id", fileID,
			"error", err,
		)
	}
}

// handleDownloadError — обработка ошибок скачивания с i18n-сообщениями.
func (h *DownloadHandler) handleDownloadError(w http.ResponseWriter, r *http.Request, fileID string, err error) {
	ctx := r.Context()
	var msg string

	switch {
	case errors.Is(err, gateway.ErrNotFound):
		msg = i18n.T(ctx, "download.error.not_found")
	case errors.Is(err, gateway.ErrServiceUnavailable):
		msg = i18n.T(ctx, "download.error.unavailable")
	default:
		msg = i18n.T(ctx, "download.error.unavailable")
	}

	h.logger.Error("ошибка скачивания файла",
		"file_id", fileID,
		"error", err,
	)

	_ = components.Toast(components.ToastParams{
		Variant: components.ToastError,
		Message: msg,
	}).Render(ctx, w)
}

// renderArchivedModal — рендер модалки FILE_ARCHIVED.
func (h *DownloadHandler) renderArchivedModal(w http.ResponseWriter, r *http.Request, archived *service.ArchivedFileInfo) {
	err := pages.ArchivedModalPartial(archived).Render(r.Context(), w)
	if err != nil {
		h.logger.Error("ошибка рендера archived modal", "error", err)
		http.Error(w, "render error", http.StatusInternalServerError)
		return
	}
}
