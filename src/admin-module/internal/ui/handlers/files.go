// Пакет handlers — HTTP-обработчики Admin UI.
// Файл files.go — обработчики страниц файлового реестра:
// список файлов (с фильтрацией, поиском, пагинацией, сортировкой),
// детальный просмотр, редактирование, soft delete.
package handlers

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/bigkaa/goartstore/admin-module/internal/repository"
	"github.com/bigkaa/goartstore/admin-module/internal/service"
	uimiddleware "github.com/bigkaa/goartstore/admin-module/internal/ui/middleware"
	"github.com/bigkaa/goartstore/admin-module/internal/ui/pages"
	"github.com/bigkaa/goartstore/admin-module/internal/ui/pages/partials"
)

// Размер страницы по умолчанию для таблицы файлов
const filePageSize = 20

// FilesHandler — обработчик страниц файлового реестра.
type FilesHandler struct {
	filesSvc        *service.FileRegistryService
	storageElemsSvc *service.StorageElementService
	logger          *slog.Logger
}

// NewFilesHandler создаёт новый FilesHandler.
func NewFilesHandler(
	filesSvc *service.FileRegistryService,
	storageElemsSvc *service.StorageElementService,
	logger *slog.Logger,
) *FilesHandler {
	return &FilesHandler{
		filesSvc:        filesSvc,
		storageElemsSvc: storageElemsSvc,
		logger:          logger.With(slog.String("component", "ui.files")),
	}
}

// HandleList обрабатывает GET /admin/files — страница списка файлов.
func (h *FilesHandler) HandleList(w http.ResponseWriter, r *http.Request) {
	session := uimiddleware.SessionFromContext(r.Context())
	if session == nil {
		http.Redirect(w, r, "/admin/login", http.StatusFound)
		return
	}

	ctx := r.Context()

	// Извлекаем параметры фильтрации из query string
	status := r.URL.Query().Get("status")
	retention := r.URL.Query().Get("retention")
	seID := r.URL.Query().Get("se")
	contentType := r.URL.Query().Get("content_type")
	search := r.URL.Query().Get("q")
	pageStr := r.URL.Query().Get("page")
	sortKey := r.URL.Query().Get("sort")
	sortDir := r.URL.Query().Get("order")
	showDeleted := r.URL.Query().Get("show_deleted")

	// Парсинг номера страницы
	page := 1
	if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
		page = p
	}

	// Значения сортировки по умолчанию
	if sortKey == "" {
		sortKey = "uploaded_at"
	}
	if sortDir == "" {
		sortDir = "desc"
	}

	// Подготовка фильтров для сервиса
	filters := h.buildFilters(status, retention, seID, showDeleted, session.Role)

	// Получаем список файлов
	offset := (page - 1) * filePageSize
	files, total, err := h.filesSvc.List(ctx, filters, filePageSize, offset)
	if err != nil {
		h.logger.Error("Ошибка получения списка файлов",
			slog.String("error", err.Error()),
		)
	}

	// Получаем список SE для фильтра (имена)
	seList := h.getSENames(ctx)

	// Преобразуем в отображаемые элементы с фильтрацией по поиску и content_type
	items := make([]pages.FileListItem, 0, len(files))
	for _, f := range files {
		item := pages.FileListItem{
			ID:               f.FileID,
			OriginalFilename: f.OriginalFilename,
			ContentType:      f.ContentType,
			SizeBytes:        f.Size,
			StorageElementID: f.StorageElementID,
			UploadedBy:       f.UploadedBy,
			UploadedAt:       f.UploadedAt,
			Status:           f.Status,
			RetentionPolicy:  f.RetentionPolicy,
		}

		// Находим имя SE
		item.SEName = h.findSEName(seList, f.StorageElementID)

		// Фильтрация по content_type (client-side, т.к. нет фильтра в репозитории)
		if contentType != "" && !matchContentType(f.ContentType, contentType) {
			continue
		}

		// Фильтрация по поиску (имя файла)
		if search != "" && !fileMatchSearch(f.OriginalFilename, search) {
			continue
		}

		items = append(items, item)
	}

	// При поиске/content_type фильтре корректируем общее количество
	totalFiltered := total
	if search != "" || contentType != "" {
		totalFiltered = len(items)
	}

	// Пагинация
	totalPages := (totalFiltered + filePageSize - 1) / filePageSize
	if totalPages < 1 {
		totalPages = 1
	}

	data := pages.FileListData{
		Username: session.Username,
		Role:     session.Role,
		Items:    items,
		SEList:   seList,
		Filters: pages.FileListFilters{
			Status:      status,
			Retention:   retention,
			SEID:        seID,
			ContentType: contentType,
			Search:      search,
			ShowDeleted: showDeleted == "true",
		},
		SortKey:    sortKey,
		SortDir:    sortDir,
		Page:       page,
		TotalPages: totalPages,
		TotalItems: totalFiltered,
		PageSize:   filePageSize,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.FileList(data).Render(ctx, w); err != nil {
		h.logger.Error("Ошибка рендеринга file list",
			slog.String("error", err.Error()),
		)
		http.Error(w, "Ошибка рендеринга страницы", http.StatusInternalServerError)
	}
}

// HandleTablePartial обрабатывает GET /admin/partials/file-table — partial для HTMX.
// Возвращает только тело таблицы + пагинацию (без layout).
func (h *FilesHandler) HandleTablePartial(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Роль пользователя для RBAC
	session := uimiddleware.SessionFromContext(ctx)
	role := ""
	if session != nil {
		role = session.Role
	}

	// Извлекаем параметры
	status := r.URL.Query().Get("status")
	retention := r.URL.Query().Get("retention")
	seID := r.URL.Query().Get("se")
	contentType := r.URL.Query().Get("content_type")
	search := r.URL.Query().Get("q")
	pageStr := r.URL.Query().Get("page")
	sortKey := r.URL.Query().Get("sort")
	sortDir := r.URL.Query().Get("order")
	showDeleted := r.URL.Query().Get("show_deleted")

	page := 1
	if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
		page = p
	}

	if sortKey == "" {
		sortKey = "uploaded_at"
	}
	if sortDir == "" {
		sortDir = "desc"
	}

	filters := h.buildFilters(status, retention, seID, showDeleted, role)

	offset := (page - 1) * filePageSize
	files, total, err := h.filesSvc.List(ctx, filters, filePageSize, offset)
	if err != nil {
		h.logger.Error("Ошибка получения списка файлов (partial)",
			slog.String("error", err.Error()),
		)
	}

	seList := h.getSENames(ctx)

	items := make([]pages.FileListItem, 0, len(files))
	for _, f := range files {
		item := pages.FileListItem{
			ID:               f.FileID,
			OriginalFilename: f.OriginalFilename,
			ContentType:      f.ContentType,
			SizeBytes:        f.Size,
			StorageElementID: f.StorageElementID,
			UploadedBy:       f.UploadedBy,
			UploadedAt:       f.UploadedAt,
			Status:           f.Status,
			RetentionPolicy:  f.RetentionPolicy,
		}

		item.SEName = h.findSEName(seList, f.StorageElementID)

		if contentType != "" && !matchContentType(f.ContentType, contentType) {
			continue
		}

		if search != "" && !fileMatchSearch(f.OriginalFilename, search) {
			continue
		}

		items = append(items, item)
	}

	totalFiltered := total
	if search != "" || contentType != "" {
		totalFiltered = len(items)
	}

	totalPages := (totalFiltered + filePageSize - 1) / filePageSize
	if totalPages < 1 {
		totalPages = 1
	}

	tableData := partials.FileTableData{
		Items:      items,
		Role:       role,
		SortKey:    sortKey,
		SortDir:    sortDir,
		Page:       page,
		TotalPages: totalPages,
		TotalItems: totalFiltered,
		PageSize:   filePageSize,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := partials.FileTableBody(tableData).Render(ctx, w); err != nil {
		h.logger.Error("Ошибка рендеринга file table partial",
			slog.String("error", err.Error()),
		)
		http.Error(w, "Ошибка рендеринга", http.StatusInternalServerError)
	}
}

// HandleDetailModal обрабатывает GET /admin/partials/file-detail/{id} — modal с метаданными файла.
func (h *FilesHandler) HandleDetailModal(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	session := uimiddleware.SessionFromContext(ctx)
	role := ""
	if session != nil {
		role = session.Role
	}

	f, err := h.filesSvc.Get(ctx, id)
	if err != nil {
		if errors.Is(err, service.ErrNotFound) {
			h.renderAlert(w, r, "Файл не найден")
		} else {
			h.renderAlert(w, r, "Ошибка получения файла: "+err.Error())
		}
		return
	}

	// Получаем имя SE
	seList := h.getSENames(ctx)
	seName := h.findSEName(seList, f.StorageElementID)

	detail := partials.FileDetailData{
		ID:               f.FileID,
		OriginalFilename: f.OriginalFilename,
		ContentType:      f.ContentType,
		SizeBytes:        f.Size,
		Checksum:         f.Checksum,
		StorageElementID: f.StorageElementID,
		SEName:           seName,
		UploadedBy:       f.UploadedBy,
		UploadedAt:       f.UploadedAt,
		Description:      f.Description,
		Tags:             f.Tags,
		Status:           f.Status,
		RetentionPolicy:  f.RetentionPolicy,
		TTLDays:          f.TTLDays,
		ExpiresAt:        f.ExpiresAt,
		CreatedAt:        f.CreatedAt,
		UpdatedAt:        f.UpdatedAt,
		Role:             role,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := partials.FileDetailContent(detail).Render(ctx, w); err != nil {
		h.logger.Error("Ошибка рендеринга file detail modal",
			slog.String("error", err.Error()),
		)
	}
}

// HandleEditForm обрабатывает GET /admin/partials/file-edit-form/{id} — форма редактирования.
func (h *FilesHandler) HandleEditForm(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	f, err := h.filesSvc.Get(ctx, id)
	if err != nil {
		h.renderAlert(w, r, "Файл не найден")
		return
	}

	desc := ""
	if f.Description != nil {
		desc = *f.Description
	}
	tagsStr := strings.Join(f.Tags, ", ")

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := partials.FileEditForm(f.FileID, f.OriginalFilename, desc, tagsStr).Render(ctx, w); err != nil {
		h.logger.Error("Ошибка рендеринга file edit form",
			slog.String("error", err.Error()),
		)
	}
}

// HandleUpdate обрабатывает PUT /admin/partials/file-update/{id} — обновление метаданных файла.
func (h *FilesHandler) HandleUpdate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	if err := r.ParseForm(); err != nil {
		h.renderAlert(w, r, "Ошибка разбора формы")
		return
	}

	description := r.FormValue("description")
	tagsStr := r.FormValue("tags")

	// Парсим теги (разделитель — запятая)
	var tags []string
	if tagsStr != "" {
		for _, t := range strings.Split(tagsStr, ",") {
			t = strings.TrimSpace(t)
			if t != "" {
				tags = append(tags, t)
			}
		}
	}

	_, err := h.filesSvc.Update(ctx, id, &description, &tags, nil)
	if err != nil {
		h.logger.Warn("Ошибка обновления файла",
			slog.String("file_id", id),
			slog.String("error", err.Error()),
		)
		if errors.Is(err, service.ErrNotFound) {
			h.renderAlert(w, r, "Файл не найден")
		} else {
			h.renderAlert(w, r, "Ошибка обновления: "+err.Error())
		}
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := partials.FileEditSuccess().Render(ctx, w); err != nil {
		h.logger.Error("Ошибка рендеринга file edit success",
			slog.String("error", err.Error()),
		)
	}
}

// HandleDelete обрабатывает DELETE /admin/partials/file-delete/{id} — soft delete файла.
func (h *FilesHandler) HandleDelete(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	err := h.filesSvc.Delete(ctx, id)
	if err != nil {
		h.logger.Warn("Ошибка удаления файла",
			slog.String("file_id", id),
			slog.String("error", err.Error()),
		)
		if errors.Is(err, service.ErrNotFound) {
			h.renderAlert(w, r, "Файл не найден")
		} else {
			h.renderAlert(w, r, "Ошибка удаления: "+err.Error())
		}
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := partials.FileDeleteSuccess().Render(ctx, w); err != nil {
		h.logger.Error("Ошибка рендеринга file delete success",
			slog.String("error", err.Error()),
		)
	}
}

// buildFilters формирует фильтры для запроса к сервису.
func (h *FilesHandler) buildFilters(status, retention, seID, showDeleted, role string) repository.FileListFilters {
	var filters repository.FileListFilters

	if status != "" {
		filters.Status = &status
	} else if showDeleted != "true" || role != "admin" {
		// По умолчанию показываем только активные файлы (если не включён showDeleted)
		activeStatus := "active"
		filters.Status = &activeStatus
	}

	if retention != "" {
		filters.RetentionPolicy = &retention
	}

	if seID != "" {
		filters.StorageElementID = &seID
	}

	return filters
}

// getSENames получает список SE с именами для фильтра.
func (h *FilesHandler) getSENames(ctx context.Context) []pages.SEOption {
	ses, _, err := h.storageElemsSvc.List(ctx, nil, nil, 1000, 0)
	if err != nil {
		h.logger.Warn("Ошибка получения списка SE для фильтра",
			slog.String("error", err.Error()),
		)
		return nil
	}

	result := make([]pages.SEOption, 0, len(ses))
	for _, se := range ses {
		result = append(result, pages.SEOption{
			ID:   se.ID,
			Name: se.Name,
		})
	}
	return result
}

// findSEName находит имя SE по ID.
func (h *FilesHandler) findSEName(seList []pages.SEOption, seID string) string {
	for _, se := range seList {
		if se.ID == seID {
			return se.Name
		}
	}
	return seID[:8] + "..." // Сокращённый UUID как fallback
}

// renderAlert рендерит alert-компонент с вариантом "error".
func (h *FilesHandler) renderAlert(w http.ResponseWriter, r *http.Request, msg string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := partials.FileAlert("error", msg).Render(r.Context(), w); err != nil {
		h.logger.Error("Ошибка рендеринга alert",
			slog.String("error", err.Error()),
		)
	}
}

// matchContentType проверяет соответствие content_type фильтру.
// Фильтр может быть группой (image, video, audio, application, text) или точным типом.
func matchContentType(ct, filter string) bool {
	if filter == "" {
		return true
	}
	// Если фильтр — группа (без "/"), проверяем prefix
	if !strings.Contains(filter, "/") {
		return strings.HasPrefix(strings.ToLower(ct), strings.ToLower(filter)+"/")
	}
	return strings.EqualFold(ct, filter)
}

// fileMatchSearch проверяет, содержит ли имя файла поисковый запрос.
func fileMatchSearch(filename, search string) bool {
	return containsLower(filename, toLower(search))
}
