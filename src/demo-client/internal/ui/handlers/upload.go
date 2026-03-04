// Package handlers — HTTP-обработчики UI страниц Demo Client.
// upload.go — обработчики загрузки файлов (single + batch).
package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/bigkaa/goartstore/demo-client/internal/config"
	"github.com/bigkaa/goartstore/demo-client/internal/gateway"
	"github.com/bigkaa/goartstore/demo-client/internal/service"
	"github.com/bigkaa/goartstore/demo-client/internal/ui/components"
	"github.com/bigkaa/goartstore/demo-client/internal/ui/i18n"
	"github.com/bigkaa/goartstore/demo-client/internal/ui/pages"
)

// UploadHandler — обработчик загрузки файлов (single + batch).
type UploadHandler struct {
	svc           *service.UploadService
	maxUploadSize int64
	logger        *slog.Logger
}

// NewUploadHandler создаёт новый обработчик загрузки.
func NewUploadHandler(svc *service.UploadService, cfg *config.Config, logger *slog.Logger) *UploadHandler {
	return &UploadHandler{
		svc:           svc,
		maxUploadSize: cfg.MaxUploadSize,
		logger:        logger,
	}
}

// Page — GET /upload — рендер страницы загрузки файлов.
// Генерирует CSRF-токен на стороне сервера и устанавливает cookie.
// Это исключает race condition при двойном JS fetch (две формы на одной странице).
func (h *UploadHandler) Page(w http.ResponseWriter, r *http.Request) {
	// Генерируем CSRF-токен
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		h.logger.Error("ошибка генерации CSRF-токена", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	csrfToken := hex.EncodeToString(tokenBytes)

	// Определяем Secure flag: true если запрос пришёл по HTTPS
	isSecure := r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"

	// Устанавливаем CSRF-cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "_csrf_token",
		Value:    csrfToken,
		Path:     "/",
		HttpOnly: true,
		Secure:   isSecure,
		SameSite: http.SameSiteStrictMode,
	})

	err := pages.Upload(pages.UploadPageData{
		MaxUploadSize: h.maxUploadSize,
		CSRFToken:     csrfToken,
	}).Render(r.Context(), w)
	if err != nil {
		h.logger.Error("ошибка рендера Upload", "error", err)
		http.Error(w, "render error", http.StatusInternalServerError)
		return
	}
}

// HandleSingle — POST /upload/single — загрузка одного файла.
func (h *UploadHandler) HandleSingle(w http.ResponseWriter, r *http.Request) {
	// Ограничиваем размер multipart формы
	if err := r.ParseMultipartForm(h.maxUploadSize); err != nil {
		h.logger.Warn("ошибка парсинга multipart формы", "error", err)
		h.renderError(w, r, i18n.Tf(r.Context(), "upload.error.too_large", formatSize(h.maxUploadSize)))
		return
	}

	// Получаем файл
	file, header, err := r.FormFile("file")
	if err != nil {
		h.logger.Warn("файл не найден в форме", "error", err)
		h.renderError(w, r, i18n.T(r.Context(), "upload.error.failed"))
		return
	}
	defer file.Close()

	// Собираем параметры загрузки
	params := h.parseUploadParams(r)

	// Загружаем через сервис
	result, err := h.svc.Upload(header.Filename, file, header.Size, params)
	if err != nil {
		h.handleUploadError(w, r, err)
		return
	}

	// Рендерим результат
	err = pages.UploadResultPartial(result).Render(r.Context(), w)
	if err != nil {
		h.logger.Error("ошибка рендера upload result", "error", err)
		http.Error(w, "render error", http.StatusInternalServerError)
		return
	}
}

// HandleBatch — POST /upload/batch — загрузка нескольких файлов.
func (h *UploadHandler) HandleBatch(w http.ResponseWriter, r *http.Request) {
	// Ограничиваем размер multipart формы
	if err := r.ParseMultipartForm(h.maxUploadSize); err != nil {
		h.logger.Warn("ошибка парсинга batch multipart формы", "error", err)
		h.renderError(w, r, i18n.Tf(r.Context(), "upload.error.too_large", formatSize(h.maxUploadSize)))
		return
	}

	// Получаем файлы
	files := r.MultipartForm.File["files"]
	if len(files) == 0 {
		h.renderError(w, r, i18n.T(r.Context(), "upload.error.failed"))
		return
	}

	// Собираем параметры (общие для всех файлов)
	params := h.parseUploadParams(r)

	// Формируем список файлов для batch-загрузки
	batchFiles := make([]service.BatchUploadFile, 0, len(files))
	for _, fh := range files {
		f, err := fh.Open()
		if err != nil {
			h.logger.Warn("ошибка открытия файла из batch", "filename", fh.Filename, "error", err)
			continue
		}
		batchFiles = append(batchFiles, service.BatchUploadFile{
			Filename: fh.Filename,
			Reader:   f,
			Size:     fh.Size,
		})
	}

	// Загружаем последовательно (без streaming прогресса — итоговый результат)
	summary := h.svc.BatchUpload(batchFiles, params, nil)

	// Закрываем все файлы
	for _, fh := range files {
		if f, err := fh.Open(); err == nil {
			_ = f.Close()
		}
	}

	// Рендерим итоговый результат batch-загрузки
	err := pages.BatchProgressPartial(summary).Render(r.Context(), w)
	if err != nil {
		h.logger.Error("ошибка рендера batch progress", "error", err)
		http.Error(w, "render error", http.StatusInternalServerError)
		return
	}
}

// parseUploadParams — извлечение параметров загрузки из формы.
func (h *UploadHandler) parseUploadParams(r *http.Request) gateway.UploadParams {
	params := gateway.UploadParams{
		Description:     strings.TrimSpace(r.FormValue("description")),
		RetentionPolicy: r.FormValue("retention_policy"),
		TTLDays:         30, // значение по умолчанию
	}

	// Теги из TagInput (запятая-разделённая строка)
	tagsRaw := strings.TrimSpace(r.FormValue("tags"))
	if tagsRaw != "" {
		for _, tag := range strings.Split(tagsRaw, ",") {
			tag = strings.TrimSpace(tag)
			if tag != "" {
				params.Tags = append(params.Tags, tag)
			}
		}
	}

	// Retention policy по умолчанию
	if params.RetentionPolicy == "" {
		params.RetentionPolicy = "temporary"
	}

	// TTL days (только для temporary)
	if params.RetentionPolicy == "temporary" {
		if ttlStr := r.FormValue("ttl_days"); ttlStr != "" {
			if ttl, err := strconv.Atoi(ttlStr); err == nil && ttl >= 1 && ttl <= 365 {
				params.TTLDays = ttl
			}
		}
	}

	return params
}

// handleUploadError — обработка ошибок загрузки с i18n-сообщениями.
func (h *UploadHandler) handleUploadError(w http.ResponseWriter, r *http.Request, err error) {
	ctx := r.Context()
	var msg string

	switch {
	case errors.Is(err, gateway.ErrFileTooLarge):
		msg = i18n.Tf(ctx, "upload.error.too_large", formatSize(h.maxUploadSize))
	case errors.Is(err, gateway.ErrStorageFull):
		msg = i18n.T(ctx, "upload.error.storage_full")
	default:
		msg = i18n.Tf(ctx, "upload.error.failed", err.Error())
	}

	h.renderError(w, r, msg)
}

// renderError — рендер toast с ошибкой в контейнер результата.
func (h *UploadHandler) renderError(w http.ResponseWriter, r *http.Request, msg string) {
	err := components.Toast(components.ToastParams{
		Variant: components.ToastError,
		Message: msg,
	}).Render(r.Context(), w)
	if err != nil {
		h.logger.Error("ошибка рендера toast", "error", err)
		http.Error(w, "render error", http.StatusInternalServerError)
		return
	}
}

// formatSize — форматирование размера файла в человеко-читаемый вид.
func formatSize(size int64) string {
	const (
		kb = 1024
		mb = 1024 * kb
		gb = 1024 * mb
	)

	switch {
	case size >= gb:
		return fmt.Sprintf("%.1f ГБ", float64(size)/float64(gb))
	case size >= mb:
		return fmt.Sprintf("%.1f МБ", float64(size)/float64(mb))
	case size >= kb:
		return fmt.Sprintf("%.1f КБ", float64(size)/float64(kb))
	default:
		return fmt.Sprintf("%d Б", size)
	}
}
