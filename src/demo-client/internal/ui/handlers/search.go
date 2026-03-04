// Package handlers — HTTP-обработчики UI страниц Demo Client.
// search.go — обработчики поиска файлов и просмотра метаданных.
package handlers

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/bigkaa/goartstore/demo-client/internal/gateway"
	"github.com/bigkaa/goartstore/demo-client/internal/service"
	"github.com/bigkaa/goartstore/demo-client/internal/ui/components"
	"github.com/bigkaa/goartstore/demo-client/internal/ui/i18n"
	"github.com/bigkaa/goartstore/demo-client/internal/ui/pages"
)

// defaultPageSize — количество результатов на странице по умолчанию.
const defaultPageSize = 20

// SearchHandler — обработчик поиска и просмотра файлов.
type SearchHandler struct {
	svc    *service.SearchService
	logger *slog.Logger
}

// NewSearchHandler создаёт новый обработчик поиска.
func NewSearchHandler(svc *service.SearchService, logger *slog.Logger) *SearchHandler {
	return &SearchHandler{
		svc:    svc,
		logger: logger,
	}
}

// Page — GET /search — рендер страницы поиска.
func (h *SearchHandler) Page(w http.ResponseWriter, r *http.Request) {
	err := pages.Search(pages.SearchPageData{}).Render(r.Context(), w)
	if err != nil {
		h.logger.Error("ошибка рендера Search", "error", err)
		http.Error(w, "render error", http.StatusInternalServerError)
	}
}

// HandleSearch — GET /search/results — HTMX partial с результатами поиска.
// Параметры поиска передаются через query string.
func (h *SearchHandler) HandleSearch(w http.ResponseWriter, r *http.Request) {
	// Парсим параметры поиска из query string
	params := h.parseSearchParams(r)

	// Выполняем поиск
	result, err := h.svc.Search(params)
	if err != nil {
		h.logger.Error("ошибка поиска", "error", err)
		h.renderSearchError(w, r, err)
		return
	}

	// Рассчитываем параметры пагинации
	page := 1
	if params.Offset > 0 && params.Limit > 0 {
		page = params.Offset/params.Limit + 1
	}
	totalPages := 0
	if result.Total > 0 && params.Limit > 0 {
		totalPages = (result.Total + params.Limit - 1) / params.Limit
	}

	// Формируем URL для пагинации с сохранением параметров поиска
	paginationURL := h.buildPaginationURL(r)

	// Рендерим partial с результатами
	err = pages.SearchResultsPartial(pages.SearchResultsData{
		Items:      result.Items,
		Total:      result.Total,
		Page:       page,
		TotalPages: totalPages,
		PageSize:   params.Limit,
		SortBy:     params.SortBy,
		SortOrder:  params.SortOrder,
		BaseURL:    paginationURL,
	}).Render(r.Context(), w)
	if err != nil {
		h.logger.Error("ошибка рендера search results", "error", err)
		http.Error(w, "render error", http.StatusInternalServerError)
	}
}

// FileDetail — GET /files/{fileID} — HTMX partial с метаданными файла.
func (h *SearchHandler) FileDetail(w http.ResponseWriter, r *http.Request) {
	fileID := chi.URLParam(r, "fileID")
	if fileID == "" {
		http.Error(w, "file ID required", http.StatusBadRequest)
		return
	}

	info, err := h.svc.GetMetadata(fileID)
	if err != nil {
		h.logger.Error("ошибка получения метаданных",
			"file_id", fileID,
			"error", err,
		)
		ctx := r.Context()
		var msg string
		switch {
		case errors.Is(err, gateway.ErrNotFound):
			msg = i18n.T(ctx, "download.error.not_found")
		default:
			msg = i18n.T(ctx, "download.error.unavailable")
		}
		_ = components.Toast(components.ToastParams{
			Variant: components.ToastError,
			Message: msg,
		}).Render(ctx, w)
		return
	}

	err = pages.FileDetailPartial(info).Render(r.Context(), w)
	if err != nil {
		h.logger.Error("ошибка рендера file detail", "error", err)
		http.Error(w, "render error", http.StatusInternalServerError)
	}
}

// parseSearchParams — извлечение параметров поиска из query string.
func (h *SearchHandler) parseSearchParams(r *http.Request) gateway.SearchRequest {
	q := r.URL.Query()

	params := gateway.SearchRequest{
		Query:           strings.TrimSpace(q.Get("q")),
		Filename:        strings.TrimSpace(q.Get("filename")),
		FileExtension:   strings.TrimSpace(q.Get("extension")),
		RetentionPolicy: q.Get("retention"),
		Status:          q.Get("status"),
		Mode:            q.Get("mode"),
		UploadedAfter:   q.Get("date_from"),
		UploadedBefore:  q.Get("date_to"),
		SortBy:          q.Get("sort"),
		SortOrder:       q.Get("order"),
		Limit:           defaultPageSize,
	}

	// Теги (запятая-разделённая строка)
	if tagsRaw := strings.TrimSpace(q.Get("tags")); tagsRaw != "" {
		for _, tag := range strings.Split(tagsRaw, ",") {
			tag = strings.TrimSpace(tag)
			if tag != "" {
				params.Tags = append(params.Tags, tag)
			}
		}
	}

	// Пагинация
	if pageStr := q.Get("page"); pageStr != "" {
		if page, err := strconv.Atoi(pageStr); err == nil && page > 1 {
			params.Offset = (page - 1) * params.Limit
		}
	}

	// Размер (байты)
	if minStr := q.Get("size_from"); minStr != "" {
		if v, err := strconv.ParseInt(minStr, 10, 64); err == nil && v > 0 {
			params.MinSize = &v
		}
	}
	if maxStr := q.Get("size_to"); maxStr != "" {
		if v, err := strconv.ParseInt(maxStr, 10, 64); err == nil && v > 0 {
			params.MaxSize = &v
		}
	}

	// Defaults
	if params.SortBy == "" {
		params.SortBy = "uploaded_at"
	}
	if params.SortOrder == "" {
		params.SortOrder = "desc"
	}
	if params.Mode == "" {
		params.Mode = "fulltext"
	}

	return params
}

// buildPaginationURL — формирование базового URL для пагинации с сохранением фильтров.
func (h *SearchHandler) buildPaginationURL(r *http.Request) string {
	q := r.URL.Query()
	// Убираем page — он добавляется Pagination компонентом
	q.Del("page")
	encoded := q.Encode()
	if encoded != "" {
		return "/search/results?" + encoded
	}
	return "/search/results"
}

// renderSearchError — рендер ошибки поиска как toast.
func (h *SearchHandler) renderSearchError(w http.ResponseWriter, r *http.Request, err error) {
	ctx := r.Context()
	var msg string

	switch {
	case errors.Is(err, gateway.ErrServiceUnavailable):
		msg = i18n.T(ctx, "download.error.unavailable")
	default:
		msg = i18n.T(ctx, "error.server")
	}

	_ = components.Toast(components.ToastParams{
		Variant: components.ToastError,
		Message: msg,
	}).Render(ctx, w)
}
