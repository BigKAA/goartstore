// Пакет handlers — HTTP-обработчики Admin UI.
// Файл storage_elements.go — обработчики страниц управления Storage Elements:
// список SE (с фильтрацией, поиском, пагинацией), discover, регистрация,
// редактирование, удаление, синхронизация, детальная страница.
package handlers

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/bigkaa/goartstore/admin-module/internal/repository"
	"github.com/bigkaa/goartstore/admin-module/internal/service"
	uimiddleware "github.com/bigkaa/goartstore/admin-module/internal/ui/middleware"
	"github.com/bigkaa/goartstore/admin-module/internal/ui/pages"
	"github.com/bigkaa/goartstore/admin-module/internal/ui/pages/partials"
)

// Размер страницы по умолчанию для таблицы SE
const sePageSize = 20

// Константа статуса "active" для фильтрации файлов.
const statusActive = "active"

// StorageElementsHandler — обработчик страниц Storage Elements.
type StorageElementsHandler struct {
	storageElemsSvc *service.StorageElementService
	filesSvc        *service.FileRegistryService
	logger          *slog.Logger
}

// NewStorageElementsHandler создаёт новый StorageElementsHandler.
func NewStorageElementsHandler(
	storageElemsSvc *service.StorageElementService,
	filesSvc *service.FileRegistryService,
	logger *slog.Logger,
) *StorageElementsHandler {
	return &StorageElementsHandler{
		storageElemsSvc: storageElemsSvc,
		filesSvc:        filesSvc,
		logger:          logger.With(slog.String("component", "ui.storage_elements")),
	}
}

// HandleList обрабатывает GET /admin/storage-elements — страница списка SE.
func (h *StorageElementsHandler) HandleList(w http.ResponseWriter, r *http.Request) {
	session := uimiddleware.SessionFromContext(r.Context())
	if session == nil {
		http.Redirect(w, r, "/admin/login", http.StatusFound)
		return
	}

	ctx := r.Context()

	// Извлекаем параметры фильтрации из query string
	mode := r.URL.Query().Get("mode")
	status := r.URL.Query().Get("status")
	search := r.URL.Query().Get("q")
	pageStr := r.URL.Query().Get("page")
	sortKey := r.URL.Query().Get("sort")
	sortDir := r.URL.Query().Get("order")

	// Парсинг номера страницы
	page := 1
	if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
		page = p
	}

	// Значения сортировки по умолчанию
	if sortKey == "" {
		sortKey = "name"
	}
	if sortDir == "" {
		sortDir = "asc"
	}

	// Подготовка фильтров для сервиса
	var modePtr, statusPtr *string
	if mode != "" {
		modePtr = &mode
	}
	if status != "" {
		statusPtr = &status
	}

	// Получаем список SE
	offset := (page - 1) * sePageSize
	ses, total, err := h.storageElemsSvc.List(ctx, modePtr, statusPtr, sePageSize, offset)
	if err != nil {
		h.logger.Error("Ошибка получения списка SE",
			slog.String("error", err.Error()),
		)
	}

	// Преобразуем в отображаемые элементы
	items := make([]pages.SEListItem, 0, len(ses))
	for _, se := range ses {
		item := pages.SEListItem{
			ID:            se.ID,
			Name:          se.Name,
			URL:           se.URL,
			Mode:          se.Mode,
			Status:        se.Status,
			CapacityBytes: se.CapacityBytes,
			UsedBytes:     se.UsedBytes,
			LastSyncAt:    se.LastSyncAt,
		}

		// Подсчитываем файлы SE
		activeStatus := statusActive
		filters := repository.FileListFilters{
			StorageElementID: &se.ID,
			Status:           &activeStatus,
		}
		_, fileCount, fErr := h.filesSvc.List(ctx, filters, 0, 0)
		if fErr != nil {
			h.logger.Warn("Ошибка подсчёта файлов SE",
				slog.String("se_id", se.ID),
				slog.String("error", fErr.Error()),
			)
		}
		item.FileCount = fileCount

		// Фильтрация по поиску (имя или URL)
		if search != "" && !matchSearch(item, search) {
			continue
		}

		items = append(items, item)
	}

	// При поиске корректируем общее количество
	totalFiltered := total
	if search != "" {
		totalFiltered = len(items)
	}

	// Пагинация
	totalPages := (totalFiltered + sePageSize - 1) / sePageSize
	if totalPages < 1 {
		totalPages = 1
	}

	data := pages.SEListData{
		Username: session.Username,
		Role:     session.Role,
		Items:    items,
		Filters: pages.SEListFilters{
			Mode:   mode,
			Status: status,
			Search: search,
		},
		SortKey:    sortKey,
		SortDir:    sortDir,
		Page:       page,
		TotalPages: totalPages,
		TotalItems: totalFiltered,
		PageSize:   sePageSize,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.SEList(data).Render(ctx, w); err != nil {
		h.logger.Error("Ошибка рендеринга SE list",
			slog.String("error", err.Error()),
		)
		http.Error(w, "Ошибка рендеринга страницы", http.StatusInternalServerError)
	}
}

// HandleTablePartial обрабатывает GET /admin/partials/se-table — partial для HTMX.
// Возвращает только тело таблицы + пагинацию (без layout).
func (h *StorageElementsHandler) HandleTablePartial(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Извлекаем параметры
	mode := r.URL.Query().Get("mode")
	status := r.URL.Query().Get("status")
	search := r.URL.Query().Get("q")
	pageStr := r.URL.Query().Get("page")
	sortKey := r.URL.Query().Get("sort")
	sortDir := r.URL.Query().Get("order")

	// Роль пользователя для RBAC
	session := uimiddleware.SessionFromContext(ctx)
	role := ""
	if session != nil {
		role = session.Role
	}

	page := 1
	if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
		page = p
	}

	if sortKey == "" {
		sortKey = "name"
	}
	if sortDir == "" {
		sortDir = "asc"
	}

	var modePtr, statusPtr *string
	if mode != "" {
		modePtr = &mode
	}
	if status != "" {
		statusPtr = &status
	}

	offset := (page - 1) * sePageSize
	ses, total, err := h.storageElemsSvc.List(ctx, modePtr, statusPtr, sePageSize, offset)
	if err != nil {
		h.logger.Error("Ошибка получения списка SE (partial)",
			slog.String("error", err.Error()),
		)
	}

	items := make([]pages.SEListItem, 0, len(ses))
	for _, se := range ses {
		item := pages.SEListItem{
			ID:            se.ID,
			Name:          se.Name,
			URL:           se.URL,
			Mode:          se.Mode,
			Status:        se.Status,
			CapacityBytes: se.CapacityBytes,
			UsedBytes:     se.UsedBytes,
			LastSyncAt:    se.LastSyncAt,
		}

		activeStatus := statusActive
		filters := repository.FileListFilters{
			StorageElementID: &se.ID,
			Status:           &activeStatus,
		}
		_, fileCount, fErr := h.filesSvc.List(ctx, filters, 0, 0)
		if fErr == nil {
			item.FileCount = fileCount
		}

		if search != "" && !matchSearch(item, search) {
			continue
		}

		items = append(items, item)
	}

	totalFiltered := total
	if search != "" {
		totalFiltered = len(items)
	}

	totalPages := (totalFiltered + sePageSize - 1) / sePageSize
	if totalPages < 1 {
		totalPages = 1
	}

	tableData := partials.SETableData{
		Items:      items,
		Role:       role,
		SortKey:    sortKey,
		SortDir:    sortDir,
		Page:       page,
		TotalPages: totalPages,
		TotalItems: totalFiltered,
		PageSize:   sePageSize,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := partials.SETableBody(tableData).Render(ctx, w); err != nil {
		h.logger.Error("Ошибка рендеринга SE table partial",
			slog.String("error", err.Error()),
		)
		http.Error(w, "Ошибка рендеринга", http.StatusInternalServerError)
	}
}

// HandleDiscover обрабатывает POST /admin/partials/se-discover — предпросмотр SE по URL.
func (h *StorageElementsHandler) HandleDiscover(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if err := r.ParseForm(); err != nil {
		h.renderDiscoverError(w, r, "Ошибка разбора формы")
		return
	}

	url := r.FormValue("url")
	if url == "" {
		h.renderDiscoverError(w, r, "URL не указан")
		return
	}

	result, err := h.storageElemsSvc.Discover(ctx, url)
	if err != nil {
		h.logger.Warn("Ошибка discover SE",
			slog.String("url", url),
			slog.String("error", err.Error()),
		)
		h.renderDiscoverError(w, r, "Не удалось подключиться к SE: "+err.Error())
		return
	}

	preview := partials.SEDiscoverPreview{
		URL:            url,
		StorageID:      result.StorageID,
		Mode:           result.Mode,
		Status:         result.Status,
		Version:        result.Version,
		TotalBytes:     result.TotalBytes,
		UsedBytes:      result.UsedBytes,
		AvailableBytes: result.AvailableBytes,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := partials.SEDiscoverResult(preview).Render(ctx, w); err != nil {
		h.logger.Error("Ошибка рендеринга discover preview",
			slog.String("error", err.Error()),
		)
	}
}

// HandleRegister обрабатывает POST /admin/partials/se-register — регистрация нового SE.
func (h *StorageElementsHandler) HandleRegister(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if err := r.ParseForm(); err != nil {
		h.renderAlert(w, r, "Ошибка разбора формы")
		return
	}

	name := r.FormValue("name")
	url := r.FormValue("url")

	if name == "" || url == "" {
		h.renderAlert(w, r, "Имя и URL обязательны")
		return
	}

	_, err := h.storageElemsSvc.Create(ctx, name, url)
	if err != nil {
		h.logger.Warn("Ошибка регистрации SE",
			slog.String("name", name),
			slog.String("url", url),
			slog.String("error", err.Error()),
		)
		if errors.Is(err, service.ErrConflict) {
			h.renderAlert(w, r, "SE с таким URL уже зарегистрирован")
		} else {
			h.renderAlert(w, r, "Ошибка регистрации: "+err.Error())
		}
		return
	}

	// Возвращаем alert об успехе + скрипт перезагрузки таблицы
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := partials.SERegisterSuccess(name).Render(ctx, w); err != nil {
		h.logger.Error("Ошибка рендеринга register success",
			slog.String("error", err.Error()),
		)
	}
}

// HandleEdit обрабатывает PUT /admin/partials/se-edit/{id} — обновление SE.
func (h *StorageElementsHandler) HandleEdit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	if err := r.ParseForm(); err != nil {
		h.renderAlert(w, r, "Ошибка разбора формы")
		return
	}

	name := r.FormValue("name")
	url := r.FormValue("url")

	var namePtr, urlPtr *string
	if name != "" {
		namePtr = &name
	}
	if url != "" {
		urlPtr = &url
	}

	_, err := h.storageElemsSvc.Update(ctx, id, namePtr, urlPtr)
	if err != nil {
		h.logger.Warn("Ошибка обновления SE",
			slog.String("se_id", id),
			slog.String("error", err.Error()),
		)
		switch {
		case errors.Is(err, service.ErrNotFound):
			h.renderAlert(w, r, "SE не найден")
		case errors.Is(err, service.ErrConflict):
			h.renderAlert(w, r, "URL или storage_id уже зарегистрирован")
		default:
			h.renderAlert(w, r, "Ошибка обновления: "+err.Error())
		}
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := partials.SEEditSuccess().Render(ctx, w); err != nil {
		h.logger.Error("Ошибка рендеринга edit success",
			slog.String("error", err.Error()),
		)
	}
}

// HandleDelete обрабатывает DELETE /admin/partials/se-delete/{id} — удаление SE.
func (h *StorageElementsHandler) HandleDelete(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	// Проверяем наличие файлов
	activeStatus := statusActive
	filters := repository.FileListFilters{
		StorageElementID: &id,
		Status:           &activeStatus,
	}
	_, fileCount, fErr := h.filesSvc.List(ctx, filters, 0, 0)
	if fErr == nil && fileCount > 0 {
		h.renderAlert(w, r,
			"Невозможно удалить SE: есть "+strconv.Itoa(fileCount)+" активных файлов. Сначала удалите файлы.")
		return
	}

	err := h.storageElemsSvc.Delete(ctx, id)
	if err != nil {
		h.logger.Warn("Ошибка удаления SE",
			slog.String("se_id", id),
			slog.String("error", err.Error()),
		)
		if errors.Is(err, service.ErrNotFound) {
			h.renderAlert(w, r, "SE не найден")
		} else {
			h.renderAlert(w, r, "Ошибка удаления: "+err.Error())
		}
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := partials.SEDeleteSuccess().Render(ctx, w); err != nil {
		h.logger.Error("Ошибка рендеринга delete success",
			slog.String("error", err.Error()),
		)
	}
}

// HandleSync обрабатывает POST /admin/partials/se-sync/{id} — синхронизация одного SE.
func (h *StorageElementsHandler) HandleSync(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	result, err := h.storageElemsSvc.Sync(ctx, id)
	if err != nil {
		h.logger.Warn("Ошибка синхронизации SE",
			slog.String("se_id", id),
			slog.String("error", err.Error()),
		)
		if errors.Is(err, service.ErrNotFound) {
			h.renderAlert(w, r, "SE не найден")
		} else {
			h.renderAlert(w, r, "Ошибка синхронизации: "+err.Error())
		}
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := partials.SESyncSuccess(result).Render(ctx, w); err != nil {
		h.logger.Error("Ошибка рендеринга sync success",
			slog.String("error", err.Error()),
		)
	}
}

// HandleSyncAll обрабатывает POST /admin/partials/se-sync-all — синхронизация всех SE.
func (h *StorageElementsHandler) HandleSyncAll(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Получаем все SE
	ses, _, err := h.storageElemsSvc.List(ctx, nil, nil, 1000, 0)
	if err != nil {
		h.renderAlert(w, r, "Ошибка получения списка SE: "+err.Error())
		return
	}

	synced := 0
	failed := 0
	for _, se := range ses {
		_, syncErr := h.storageElemsSvc.Sync(ctx, se.ID)
		if syncErr != nil {
			h.logger.Warn("Ошибка синхронизации SE (sync-all)",
				slog.String("se_id", se.ID),
				slog.String("error", syncErr.Error()),
			)
			failed++
		} else {
			synced++
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := partials.SESyncAllSuccess(synced, failed).Render(ctx, w); err != nil {
		h.logger.Error("Ошибка рендеринга sync-all success",
			slog.String("error", err.Error()),
		)
	}
}

// HandleDetail обрабатывает GET /admin/storage-elements/{id} — детальная страница SE.
func (h *StorageElementsHandler) HandleDetail(w http.ResponseWriter, r *http.Request) {
	session := uimiddleware.SessionFromContext(r.Context())
	if session == nil {
		http.Redirect(w, r, "/admin/login", http.StatusFound)
		return
	}

	ctx := r.Context()
	id := chi.URLParam(r, "id")

	se, err := h.storageElemsSvc.Get(ctx, id)
	if err != nil {
		if errors.Is(err, service.ErrNotFound) {
			http.Error(w, "SE не найден", http.StatusNotFound)
		} else {
			h.logger.Error("Ошибка получения SE",
				slog.String("se_id", id),
				slog.String("error", err.Error()),
			)
			http.Error(w, "Ошибка сервера", http.StatusInternalServerError)
		}
		return
	}

	// Подсчёт файлов
	activeStatus := statusActive
	filters := repository.FileListFilters{
		StorageElementID: &id,
		Status:           &activeStatus,
	}
	_, fileCount, _ := h.filesSvc.List(ctx, filters, 0, 0)

	// Получаем список файлов для первой страницы
	fileList, _, _ := h.filesSvc.List(ctx, filters, 20, 0)

	fileItems := make([]pages.SEFileItem, 0, len(fileList))
	for _, f := range fileList {
		fileItems = append(fileItems, pages.SEFileItem{
			ID:               f.FileID,
			OriginalFilename: f.OriginalFilename,
			ContentType:      f.ContentType,
			SizeBytes:        f.Size,
			UploadedBy:       f.UploadedBy,
			UploadedAt:       f.UploadedAt,
			Status:           f.Status,
		})
	}

	data := pages.SEDetailData{
		Username:       session.Username,
		Role:           session.Role,
		ID:             se.ID,
		Name:           se.Name,
		URL:            se.URL,
		StorageID:      se.StorageID,
		Mode:           se.Mode,
		Status:         se.Status,
		CapacityBytes:  se.CapacityBytes,
		UsedBytes:      se.UsedBytes,
		AvailableBytes: se.AvailableBytes,
		LastSyncAt:     se.LastSyncAt,
		LastFileSyncAt: se.LastFileSyncAt,
		CreatedAt:      se.CreatedAt,
		UpdatedAt:      se.UpdatedAt,
		FileCount:      fileCount,
		Files:          fileItems,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.SEDetail(data).Render(ctx, w); err != nil {
		h.logger.Error("Ошибка рендеринга SE detail",
			slog.String("se_id", id),
			slog.String("error", err.Error()),
		)
		http.Error(w, "Ошибка рендеринга страницы", http.StatusInternalServerError)
	}
}

// HandleFilesPartial обрабатывает GET /admin/partials/se-files/{id} — partial файлов SE.
func (h *StorageElementsHandler) HandleFilesPartial(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	pageStr := r.URL.Query().Get("page")
	page := 1
	if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
		page = p
	}

	const filesPageSize = 20

	activeStatus := statusActive
	filters := repository.FileListFilters{
		StorageElementID: &id,
		Status:           &activeStatus,
	}

	offset := (page - 1) * filesPageSize
	fileList, totalFiles, err := h.filesSvc.List(ctx, filters, filesPageSize, offset)
	if err != nil {
		h.logger.Error("Ошибка получения файлов SE (partial)",
			slog.String("se_id", id),
			slog.String("error", err.Error()),
		)
	}

	fileItems := make([]pages.SEFileItem, 0, len(fileList))
	for _, f := range fileList {
		fileItems = append(fileItems, pages.SEFileItem{
			ID:               f.FileID,
			OriginalFilename: f.OriginalFilename,
			ContentType:      f.ContentType,
			SizeBytes:        f.Size,
			UploadedBy:       f.UploadedBy,
			UploadedAt:       f.UploadedAt,
			Status:           f.Status,
		})
	}

	totalPages := (totalFiles + filesPageSize - 1) / filesPageSize
	if totalPages < 1 {
		totalPages = 1
	}

	filesData := partials.SEFilesData{
		SEID:       id,
		Files:      fileItems,
		Page:       page,
		TotalPages: totalPages,
		TotalItems: totalFiles,
		PageSize:   filesPageSize,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := partials.SEFilesTable(filesData).Render(ctx, w); err != nil {
		h.logger.Error("Ошибка рендеринга SE files partial",
			slog.String("error", err.Error()),
		)
	}
}

// HandleEditForm обрабатывает GET /admin/partials/se-edit-form/{id} — форма редактирования SE.
func (h *StorageElementsHandler) HandleEditForm(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	se, err := h.storageElemsSvc.Get(ctx, id)
	if err != nil {
		h.renderAlert(w, r, "SE не найден")
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := partials.SEEditForm(se.ID, se.Name, se.URL).Render(ctx, w); err != nil {
		h.logger.Error("Ошибка рендеринга edit form",
			slog.String("error", err.Error()),
		)
	}
}

// renderDiscoverError рендерит ошибку discover.
func (h *StorageElementsHandler) renderDiscoverError(w http.ResponseWriter, r *http.Request, msg string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := partials.SEDiscoverError(msg).Render(r.Context(), w); err != nil {
		h.logger.Error("Ошибка рендеринга discover error",
			slog.String("error", err.Error()),
		)
	}
}

// renderAlert рендерит alert-компонент с вариантом "error".
func (h *StorageElementsHandler) renderAlert(w http.ResponseWriter, r *http.Request, msg string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := partials.SEAlert("error", msg).Render(r.Context(), w); err != nil {
		h.logger.Error("Ошибка рендеринга alert",
			slog.String("error", err.Error()),
		)
	}
}

// matchSearch проверяет, содержит ли SE-элемент поисковый запрос (имя или URL).
func matchSearch(item pages.SEListItem, search string) bool {
	search = toLower(search)
	return containsLower(item.Name, search) || containsLower(item.URL, search)
}

// toLower — приведение строки к нижнему регистру (ASCII).
func toLower(s string) string {
	b := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		b[i] = c
	}
	return string(b)
}

// containsLower проверяет, содержит ли строка подстроку (без учёта регистра ASCII).
func containsLower(s, substr string) bool {
	s = toLower(s)
	if len(substr) > len(s) {
		return false
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
