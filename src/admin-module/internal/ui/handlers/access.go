// Пакет handlers — HTTP-обработчики Admin UI.
// Файл access.go — обработчики страницы «Управление доступом»:
// два таба (Пользователи / Service Accounts), фильтрация, поиск,
// role overrides для пользователей, CRUD SA, ротация секрета, синхронизация.
package handlers

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/bigkaa/goartstore/admin-module/internal/service"
	uimiddleware "github.com/bigkaa/goartstore/admin-module/internal/ui/middleware"
	"github.com/bigkaa/goartstore/admin-module/internal/ui/pages"
	"github.com/bigkaa/goartstore/admin-module/internal/ui/pages/partials"
)

// Размер страницы для таблиц пользователей и SA
const (
	usersPageSize = 20
	saPageSize    = 20
)

// AccessHandler — обработчик страницы «Управление доступом».
type AccessHandler struct {
	usersSvc *service.AdminUserService
	saSvc    *service.ServiceAccountService
	idpSvc   *service.IDPService
	logger   *slog.Logger
}

// NewAccessHandler создаёт новый AccessHandler.
func NewAccessHandler(
	usersSvc *service.AdminUserService,
	saSvc *service.ServiceAccountService,
	idpSvc *service.IDPService,
	logger *slog.Logger,
) *AccessHandler {
	return &AccessHandler{
		usersSvc: usersSvc,
		saSvc:    saSvc,
		idpSvc:   idpSvc,
		logger:   logger.With(slog.String("component", "ui.access")),
	}
}

// HandleAccess обрабатывает GET /admin/access — страница с табами.
//
//nolint:cyclop,gocognit // TODO: упростить HandleAccess
func (h *AccessHandler) HandleAccess(w http.ResponseWriter, r *http.Request) {
	session := uimiddleware.SessionFromContext(r.Context())
	if session == nil {
		http.Redirect(w, r, "/admin/login", http.StatusFound)
		return
	}

	ctx := r.Context()

	// Определяем активный таб
	activeTab := r.URL.Query().Get("tab")
	if activeTab != "sa" {
		activeTab = "users"
	}

	// === Пользователи ===
	usersPage := 1
	if p, err := strconv.Atoi(r.URL.Query().Get("users_page")); err == nil && p > 0 {
		usersPage = p
	}
	roleFilter := r.URL.Query().Get("role")
	statusFilter := r.URL.Query().Get("user_status")
	userSearch := r.URL.Query().Get("user_q")

	offset := (usersPage - 1) * usersPageSize
	users, usersTotal, err := h.usersSvc.ListUsers(ctx, usersPageSize+200, 0) // Запрашиваем больше для клиентской фильтрации
	if err != nil {
		h.logger.Error("Ошибка получения списка пользователей",
			slog.String("error", err.Error()),
		)
	}

	// Преобразуем в UI-модели с клиентской фильтрацией
	userItems := make([]pages.UserListItem, 0, len(users))
	for _, u := range users { //nolint:dupl // TODO: вынести общую логику фильтрации пользователей
		item := pages.UserListItem{
			ID:            u.ID,
			Username:      u.Username,
			Email:         u.Email,
			FirstName:     u.FirstName,
			LastName:      u.LastName,
			Enabled:       u.Enabled,
			Groups:        u.Groups,
			IdpRole:       u.IdpRole,
			RoleOverride:  u.RoleOverride,
			EffectiveRole: u.EffectiveRole,
			CreatedAt:     u.CreatedAt,
		}

		// Фильтрация по роли
		if roleFilter != "" && item.EffectiveRole != roleFilter {
			continue
		}

		// Фильтрация по статусу (enabled/disabled)
		if statusFilter == "enabled" && !item.Enabled {
			continue
		}
		if statusFilter == "disabled" && item.Enabled {
			continue
		}

		// Поиск по username/email
		if userSearch != "" &&
			!containsLower(item.Username, toLower(userSearch)) &&
			!containsLower(item.Email, toLower(userSearch)) {
			continue
		}

		userItems = append(userItems, item)
	}

	// Пагинация после фильтрации
	usersTotalFiltered := len(userItems)
	usersTotalPages := (usersTotalFiltered + usersPageSize - 1) / usersPageSize
	if usersTotalPages < 1 {
		usersTotalPages = 1
	}

	// Применяем пагинацию
	startIdx := offset
	endIdx := offset + usersPageSize
	if startIdx > len(userItems) {
		startIdx = len(userItems)
	}
	if endIdx > len(userItems) {
		endIdx = len(userItems)
	}
	pagedUserItems := userItems[startIdx:endIdx]

	// === Service Accounts ===
	saPage := 1
	if p, saPageErr := strconv.Atoi(r.URL.Query().Get("sa_page")); saPageErr == nil && p > 0 {
		saPage = p
	}
	saStatus := r.URL.Query().Get("sa_status")
	saSearch := r.URL.Query().Get("sa_q")

	var saStatusFilter *string
	if saStatus != "" {
		saStatusFilter = &saStatus
	}

	saOffset := (saPage - 1) * saPageSize
	sas, _, err := h.saSvc.List(ctx, saStatusFilter, saPageSize+200, 0) // Больше для клиентской фильтрации
	if err != nil {
		h.logger.Error("Ошибка получения списка SA",
			slog.String("error", err.Error()),
		)
	}

	// Преобразуем SA в UI-модели с поиском
	saItems := make([]pages.SAListItem, 0, len(sas))
	for _, sa := range sas {
		item := pages.SAListItem{
			ID:           sa.ID,
			ClientID:     sa.ClientID,
			Name:         sa.Name,
			Description:  sa.Description,
			Scopes:       sa.Scopes,
			Status:       sa.Status,
			Source:       sa.Source,
			LastSyncedAt: sa.LastSyncedAt,
			CreatedAt:    sa.CreatedAt,
		}

		// Поиск по name/client_id
		if saSearch != "" &&
			!containsLower(item.Name, toLower(saSearch)) &&
			!containsLower(item.ClientID, toLower(saSearch)) {
			continue
		}

		saItems = append(saItems, item)
	}

	// Пагинация SA
	saTotalFiltered := len(saItems)
	saTotalPages := (saTotalFiltered + saPageSize - 1) / saPageSize
	if saTotalPages < 1 {
		saTotalPages = 1
	}

	saStartIdx := saOffset
	saEndIdx := saOffset + saPageSize
	if saStartIdx > len(saItems) {
		saStartIdx = len(saItems)
	}
	if saEndIdx > len(saItems) {
		saEndIdx = len(saItems)
	}
	pagedSAItems := saItems[saStartIdx:saEndIdx]

	// Статус IdP
	idpStatus := h.idpSvc.GetStatus(ctx)

	data := pages.AccessData{
		Username:  session.Username,
		Role:      session.Role,
		ActiveTab: activeTab,

		// Пользователи
		Users: pagedUserItems,
		UsersFilters: pages.UsersFilters{
			Role:   roleFilter,
			Status: statusFilter,
			Search: userSearch,
		},
		UsersPage:       usersPage,
		UsersTotalPages: usersTotalPages,
		UsersTotalItems: usersTotalFiltered,
		UsersTotal:      usersTotal, // Общее количество в Keycloak

		// Service Accounts
		ServiceAccounts: pagedSAItems,
		SAFilters: pages.SAFilters{
			Status: saStatus,
			Search: saSearch,
		},
		SAPage:       saPage,
		SATotalPages: saTotalPages,
		SATotalItems: saTotalFiltered,

		// IdP статус
		IDPConnected:    idpStatus.Connected,
		IDPRealm:        idpStatus.Realm,
		IDPUsersCount:   idpStatus.UsersCount,
		IDPClientsCount: idpStatus.ClientsCount,
		IDPLastSyncAt:   idpStatus.LastSASyncAt,
		IDPError:        idpStatus.Error,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.Access(data).Render(ctx, w); err != nil {
		h.logger.Error("Ошибка рендеринга access page",
			slog.String("error", err.Error()),
		)
		http.Error(w, "Ошибка рендеринга страницы", http.StatusInternalServerError)
	}
}

// HandleUsersTablePartial обрабатывает GET /admin/partials/users-table — partial для HTMX.
func (h *AccessHandler) HandleUsersTablePartial(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	session := uimiddleware.SessionFromContext(ctx)
	role := ""
	if session != nil {
		role = session.Role
	}

	page := 1
	if p, err := strconv.Atoi(r.URL.Query().Get("page")); err == nil && p > 0 {
		page = p
	}
	roleFilter := r.URL.Query().Get("role")
	statusFilter := r.URL.Query().Get("user_status")
	search := r.URL.Query().Get("user_q")

	users, _, err := h.usersSvc.ListUsers(ctx, usersPageSize+200, 0)
	if err != nil {
		h.logger.Error("Ошибка получения пользователей (partial)",
			slog.String("error", err.Error()),
		)
	}

	// Фильтрация
	items := make([]pages.UserListItem, 0, len(users))
	for _, u := range users { //nolint:dupl // TODO: вынести общую логику фильтрации пользователей
		item := pages.UserListItem{
			ID:            u.ID,
			Username:      u.Username,
			Email:         u.Email,
			FirstName:     u.FirstName,
			LastName:      u.LastName,
			Enabled:       u.Enabled,
			Groups:        u.Groups,
			IdpRole:       u.IdpRole,
			RoleOverride:  u.RoleOverride,
			EffectiveRole: u.EffectiveRole,
			CreatedAt:     u.CreatedAt,
		}

		if roleFilter != "" && item.EffectiveRole != roleFilter {
			continue
		}
		if statusFilter == "enabled" && !item.Enabled {
			continue
		}
		if statusFilter == "disabled" && item.Enabled {
			continue
		}
		if search != "" &&
			!containsLower(item.Username, toLower(search)) &&
			!containsLower(item.Email, toLower(search)) {
			continue
		}

		items = append(items, item)
	}

	totalFiltered := len(items)
	totalPages := (totalFiltered + usersPageSize - 1) / usersPageSize
	if totalPages < 1 {
		totalPages = 1
	}

	offset := (page - 1) * usersPageSize
	end := offset + usersPageSize
	if offset > len(items) {
		offset = len(items)
	}
	if end > len(items) {
		end = len(items)
	}

	tableData := partials.UsersTableData{
		Items:      items[offset:end],
		Role:       role,
		Page:       page,
		TotalPages: totalPages,
		TotalItems: totalFiltered,
		PageSize:   usersPageSize,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := partials.UsersTableBody(tableData).Render(ctx, w); err != nil {
		h.logger.Error("Ошибка рендеринга users table partial",
			slog.String("error", err.Error()),
		)
	}
}

// HandleUserDetail обрабатывает GET /admin/partials/user-detail/{id} — детали пользователя.
func (h *AccessHandler) HandleUserDetail(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	session := uimiddleware.SessionFromContext(ctx)
	role := ""
	if session != nil {
		role = session.Role
	}

	user, err := h.usersSvc.GetUser(ctx, id)
	if err != nil {
		h.renderAlert(w, r, "Ошибка получения пользователя: "+err.Error())
		return
	}

	detail := partials.UserDetailData{
		ID:            user.ID,
		Username:      user.Username,
		Email:         user.Email,
		FirstName:     user.FirstName,
		LastName:      user.LastName,
		Enabled:       user.Enabled,
		Groups:        user.Groups,
		IdpRole:       user.IdpRole,
		RoleOverride:  user.RoleOverride,
		EffectiveRole: user.EffectiveRole,
		CreatedAt:     user.CreatedAt,
		Role:          role, // Роль текущего пользователя (для RBAC)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := partials.UserDetailContent(detail).Render(ctx, w); err != nil {
		h.logger.Error("Ошибка рендеринга user detail",
			slog.String("error", err.Error()),
		)
	}
}

// HandleAddRoleOverride обрабатывает POST /admin/partials/user-role-override/{id} — повышение роли.
func (h *AccessHandler) HandleAddRoleOverride(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	session := uimiddleware.SessionFromContext(ctx)
	if session == nil || session.Role != roleAdmin {
		h.renderAlert(w, r, "Нет прав для этого действия")
		return
	}

	_, err := h.usersSvc.SetRoleOverride(ctx, id, "admin", session.Username)
	if err != nil {
		h.logger.Warn("Ошибка добавления role override",
			slog.String("user_id", id),
			slog.String("error", err.Error()),
		)
		h.renderAlert(w, r, "Ошибка повышения роли: "+err.Error())
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := partials.AccessActionSuccess("Роль пользователя повышена до admin").Render(ctx, w); err != nil {
		h.logger.Error("Ошибка рендеринга success",
			slog.String("error", err.Error()),
		)
	}
}

// HandleRemoveRoleOverride обрабатывает DELETE /admin/partials/user-role-override/{id} — снятие override.
//
//nolint:dupl // TODO: вынести общую логику удаления
func (h *AccessHandler) HandleRemoveRoleOverride(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	session := uimiddleware.SessionFromContext(ctx)
	if session == nil || session.Role != roleAdmin {
		h.renderAlert(w, r, "Нет прав для этого действия")
		return
	}

	err := h.usersSvc.DeleteUser(ctx, id)
	if err != nil {
		h.logger.Warn("Ошибка удаления role override",
			slog.String("user_id", id),
			slog.String("error", err.Error()),
		)
		if errors.Is(err, service.ErrNotFound) {
			h.renderAlert(w, r, "Role override не найден")
		} else {
			h.renderAlert(w, r, "Ошибка удаления дополнения роли: "+err.Error())
		}
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := partials.AccessActionSuccess("Дополнение роли удалено").Render(ctx, w); err != nil {
		h.logger.Error("Ошибка рендеринга success",
			slog.String("error", err.Error()),
		)
	}
}

// HandleSATablePartial обрабатывает GET /admin/partials/sa-table — partial таблицы SA.
func (h *AccessHandler) HandleSATablePartial(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	session := uimiddleware.SessionFromContext(ctx)
	role := ""
	if session != nil {
		role = session.Role
	}

	page := 1
	if p, err := strconv.Atoi(r.URL.Query().Get("page")); err == nil && p > 0 {
		page = p
	}
	statusFilter := r.URL.Query().Get("sa_status")
	search := r.URL.Query().Get("sa_q")

	var statusPtr *string
	if statusFilter != "" {
		statusPtr = &statusFilter
	}

	sas, _, err := h.saSvc.List(ctx, statusPtr, saPageSize+200, 0)
	if err != nil {
		h.logger.Error("Ошибка получения SA (partial)",
			slog.String("error", err.Error()),
		)
	}

	items := make([]pages.SAListItem, 0, len(sas))
	for _, sa := range sas {
		item := pages.SAListItem{
			ID:           sa.ID,
			ClientID:     sa.ClientID,
			Name:         sa.Name,
			Description:  sa.Description,
			Scopes:       sa.Scopes,
			Status:       sa.Status,
			Source:       sa.Source,
			LastSyncedAt: sa.LastSyncedAt,
			CreatedAt:    sa.CreatedAt,
		}

		if search != "" &&
			!containsLower(item.Name, toLower(search)) &&
			!containsLower(item.ClientID, toLower(search)) {
			continue
		}

		items = append(items, item)
	}

	totalFiltered := len(items)
	totalPages := (totalFiltered + saPageSize - 1) / saPageSize
	if totalPages < 1 {
		totalPages = 1
	}

	offset := (page - 1) * saPageSize
	end := offset + saPageSize
	if offset > len(items) {
		offset = len(items)
	}
	if end > len(items) {
		end = len(items)
	}

	tableData := partials.SATableData{
		Items:      items[offset:end],
		Role:       role,
		Page:       page,
		TotalPages: totalPages,
		TotalItems: totalFiltered,
		PageSize:   saPageSize,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := partials.SATableBody(tableData).Render(ctx, w); err != nil {
		h.logger.Error("Ошибка рендеринга SA table partial",
			slog.String("error", err.Error()),
		)
	}
}

// HandleSACreate обрабатывает POST /admin/partials/sa-create — создание SA.
func (h *AccessHandler) HandleSACreate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	session := uimiddleware.SessionFromContext(ctx)
	if session == nil || session.Role != roleAdmin {
		h.renderAlert(w, r, "Нет прав для создания SA")
		return
	}

	if err := r.ParseForm(); err != nil {
		h.renderAlert(w, r, "Ошибка разбора формы")
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	description := strings.TrimSpace(r.FormValue("description"))

	if name == "" {
		h.renderAlert(w, r, "Имя SA обязательно")
		return
	}

	// Парсим scopes из чекбоксов
	scopes := r.Form["scopes"]

	saWithSecret, err := h.saSvc.Create(ctx, name, description, scopes)
	if err != nil {
		h.logger.Warn("Ошибка создания SA",
			slog.String("name", name),
			slog.String("error", err.Error()),
		)
		if errors.Is(err, service.ErrConflict) {
			h.renderAlert(w, r, fmt.Sprintf("SA с именем '%s' уже существует", name))
		} else {
			h.renderAlert(w, r, "Ошибка создания SA: "+err.Error())
		}
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := partials.SACreateSuccess(saWithSecret.ClientID, saWithSecret.ClientSecret).Render(ctx, w); err != nil {
		h.logger.Error("Ошибка рендеринга SA create success",
			slog.String("error", err.Error()),
		)
	}
}

// HandleSAEditForm обрабатывает GET /admin/partials/sa-edit-form/{id} — форма редактирования SA.
func (h *AccessHandler) HandleSAEditForm(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	sa, err := h.saSvc.Get(ctx, id)
	if err != nil {
		if errors.Is(err, service.ErrNotFound) {
			h.renderAlert(w, r, "SA не найден")
		} else {
			h.renderAlert(w, r, "Ошибка получения SA: "+err.Error())
		}
		return
	}

	desc := ""
	if sa.Description != nil {
		desc = *sa.Description
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := partials.SAEditForm(sa.ID, sa.Name, desc, sa.Scopes, sa.Status).Render(ctx, w); err != nil {
		h.logger.Error("Ошибка рендеринга SA edit form",
			slog.String("error", err.Error()),
		)
	}
}

// HandleSAEdit обрабатывает PUT /admin/partials/sa-edit/{id} — обновление SA.
func (h *AccessHandler) HandleSAEdit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	session := uimiddleware.SessionFromContext(ctx)
	if session == nil || session.Role != roleAdmin {
		h.renderAlert(w, r, "Нет прав для редактирования SA")
		return
	}

	if err := r.ParseForm(); err != nil {
		h.renderAlert(w, r, "Ошибка разбора формы")
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	description := strings.TrimSpace(r.FormValue("description"))
	status := r.FormValue("status")
	scopes := r.Form["scopes"]

	var namePtr, descPtr, statusPtr *string
	if name != "" {
		namePtr = &name
	}
	if description != "" {
		descPtr = &description
	}
	if status != "" {
		statusPtr = &status
	}

	_, err := h.saSvc.Update(ctx, id, namePtr, descPtr, scopes, statusPtr)
	if err != nil {
		h.logger.Warn("Ошибка обновления SA",
			slog.String("sa_id", id),
			slog.String("error", err.Error()),
		)
		if errors.Is(err, service.ErrNotFound) {
			h.renderAlert(w, r, "SA не найден")
		} else {
			h.renderAlert(w, r, "Ошибка обновления: "+err.Error())
		}
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := partials.AccessActionSuccess("SA успешно обновлён").Render(ctx, w); err != nil {
		h.logger.Error("Ошибка рендеринга success",
			slog.String("error", err.Error()),
		)
	}
}

// HandleSADelete обрабатывает DELETE /admin/partials/sa-delete/{id} — удаление SA.
//
//nolint:dupl // TODO: вынести общую логику удаления
func (h *AccessHandler) HandleSADelete(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	session := uimiddleware.SessionFromContext(ctx)
	if session == nil || session.Role != roleAdmin {
		h.renderAlert(w, r, "Нет прав для удаления SA")
		return
	}

	err := h.saSvc.Delete(ctx, id)
	if err != nil {
		h.logger.Warn("Ошибка удаления SA",
			slog.String("sa_id", id),
			slog.String("error", err.Error()),
		)
		if errors.Is(err, service.ErrNotFound) {
			h.renderAlert(w, r, "SA не найден")
		} else {
			h.renderAlert(w, r, "Ошибка удаления: "+err.Error())
		}
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := partials.AccessActionSuccess("SA успешно удалён").Render(ctx, w); err != nil {
		h.logger.Error("Ошибка рендеринга success",
			slog.String("error", err.Error()),
		)
	}
}

// HandleSARotateSecret обрабатывает POST /admin/partials/sa-rotate/{id} — ротация секрета.
func (h *AccessHandler) HandleSARotateSecret(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	session := uimiddleware.SessionFromContext(ctx)
	if session == nil || session.Role != roleAdmin {
		h.renderAlert(w, r, "Нет прав для ротации секрета")
		return
	}

	clientID, newSecret, err := h.saSvc.RotateSecret(ctx, id)
	if err != nil {
		h.logger.Warn("Ошибка ротации секрета SA",
			slog.String("sa_id", id),
			slog.String("error", err.Error()),
		)
		if errors.Is(err, service.ErrNotFound) {
			h.renderAlert(w, r, "SA не найден")
		} else {
			h.renderAlert(w, r, "Ошибка ротации секрета: "+err.Error())
		}
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := partials.SASecretDisplay(clientID, newSecret).Render(ctx, w); err != nil {
		h.logger.Error("Ошибка рендеринга SA secret display",
			slog.String("error", err.Error()),
		)
	}
}

// HandleSASync обрабатывает POST /admin/partials/sa-sync — синхронизация SA с Keycloak.
func (h *AccessHandler) HandleSASync(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	session := uimiddleware.SessionFromContext(ctx)
	if session == nil || session.Role != roleAdmin {
		h.renderAlert(w, r, "Нет прав для синхронизации")
		return
	}

	result, err := h.idpSvc.SyncSA(ctx)
	if err != nil {
		h.logger.Warn("Ошибка синхронизации SA",
			slog.String("error", err.Error()),
		)
		h.renderAlert(w, r, "Ошибка синхронизации: "+err.Error())
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := partials.SASyncSuccess(result).Render(ctx, w); err != nil {
		h.logger.Error("Ошибка рендеринга SA sync success",
			slog.String("error", err.Error()),
		)
	}
}

// renderAlert рендерит alert-компонент с вариантом "error".
func (h *AccessHandler) renderAlert(w http.ResponseWriter, r *http.Request, msg string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := partials.AccessAlert("error", msg).Render(r.Context(), w); err != nil {
		h.logger.Error("Ошибка рендеринга alert",
			slog.String("error", err.Error()),
		)
	}
}
