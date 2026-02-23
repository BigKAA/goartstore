// admin_users.go — обработчики /api/v1/admin-users endpoints.
// Управление пользователями: список, получение, обновление role override, удаление.
package handlers

import (
	"encoding/json"
	"errors"
	"net/http"

	openapi_types "github.com/oapi-codegen/runtime/types"

	apierrors "github.com/arturkryukov/artstore/admin-module/internal/api/errors"
	"github.com/arturkryukov/artstore/admin-module/internal/api/generated"
	"github.com/arturkryukov/artstore/admin-module/internal/api/middleware"
	"github.com/arturkryukov/artstore/admin-module/internal/domain/model"
	"github.com/arturkryukov/artstore/admin-module/internal/service"
)

// ListAdminUsers — GET /api/v1/admin-users.
// Возвращает список пользователей из Keycloak с role overrides.
// Доступ: admin или readonly.
func (h *APIHandler) ListAdminUsers(w http.ResponseWriter, r *http.Request, params generated.ListAdminUsersParams) {
	claims := middleware.ClaimsFromContext(r.Context())
	if claims == nil || claims.SubjectType != middleware.SubjectTypeUser || !claims.HasAnyRole("admin", "readonly") {
		apierrors.Forbidden(w, "Недостаточно прав: требуется роль admin или readonly")
		return
	}

	limit, offset := paginationDefaults(params.Limit, params.Offset)

	users, total, err := h.adminUsers.ListUsers(r.Context(), limit, offset)
	if err != nil {
		h.logger.Error("Ошибка получения списка пользователей", "error", err)
		apierrors.IDPUnavailable(w, "Ошибка получения пользователей из Keycloak")
		return
	}

	items := make([]generated.AdminUser, len(users))
	for i, u := range users {
		items[i] = mapAdminUser(u)
	}

	resp := generated.AdminUserListResponse{
		Items:   items,
		Total:   total,
		Limit:   limit,
		Offset:  offset,
		HasMore: offset+limit < total,
	}

	writeJSON(w, http.StatusOK, resp)
}

// GetAdminUser — GET /api/v1/admin-users/{id}.
// Возвращает пользователя по Keycloak ID.
// Доступ: admin или readonly.
func (h *APIHandler) GetAdminUser(w http.ResponseWriter, r *http.Request, id generated.UserId) {
	claims := middleware.ClaimsFromContext(r.Context())
	if claims == nil || claims.SubjectType != middleware.SubjectTypeUser || !claims.HasAnyRole("admin", "readonly") {
		apierrors.Forbidden(w, "Недостаточно прав: требуется роль admin или readonly")
		return
	}

	user, err := h.adminUsers.GetUser(r.Context(), id)
	if err != nil {
		h.logger.Error("Ошибка получения пользователя", "user_id", id, "error", err)
		apierrors.IDPUnavailable(w, "Ошибка получения пользователя из Keycloak")
		return
	}

	writeJSON(w, http.StatusOK, mapAdminUser(user))
}

// UpdateAdminUser — PUT /api/v1/admin-users/{id}.
// Обновляет role override пользователя. null удаляет override.
// Доступ: admin.
func (h *APIHandler) UpdateAdminUser(w http.ResponseWriter, r *http.Request, id generated.UserId) {
	claims := middleware.ClaimsFromContext(r.Context())
	if claims == nil || claims.SubjectType != middleware.SubjectTypeUser || !claims.HasRole("admin") {
		apierrors.Forbidden(w, "Недостаточно прав: требуется роль admin")
		return
	}

	var req generated.AdminUserUpdate
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierrors.ValidationError(w, "Некорректный JSON: "+err.Error())
		return
	}

	// Маппинг: если RoleOverride nil — удаляем override, иначе устанавливаем
	var roleOverride *string
	if req.RoleOverride != nil {
		s := string(*req.RoleOverride)
		roleOverride = &s
	}

	user, err := h.adminUsers.UpdateUser(r.Context(), id, roleOverride, claims.PreferredUsername)
	if err != nil {
		if errors.Is(err, service.ErrInvalidRole) {
			apierrors.ValidationError(w, err.Error())
			return
		}
		h.logger.Error("Ошибка обновления пользователя", "user_id", id, "error", err)
		apierrors.InternalError(w, "Ошибка обновления пользователя")
		return
	}

	writeJSON(w, http.StatusOK, mapAdminUser(user))
}

// DeleteAdminUser — DELETE /api/v1/admin-users/{id}.
// Удаляет role override пользователя.
// Доступ: admin.
func (h *APIHandler) DeleteAdminUser(w http.ResponseWriter, r *http.Request, id generated.UserId) {
	claims := middleware.ClaimsFromContext(r.Context())
	if claims == nil || claims.SubjectType != middleware.SubjectTypeUser || !claims.HasRole("admin") {
		apierrors.Forbidden(w, "Недостаточно прав: требуется роль admin")
		return
	}

	if err := h.adminUsers.DeleteUser(r.Context(), id); err != nil {
		if errors.Is(err, service.ErrNotFound) {
			apierrors.NotFound(w, "Role override для пользователя не найден")
			return
		}
		h.logger.Error("Ошибка удаления role override", "user_id", id, "error", err)
		apierrors.InternalError(w, "Ошибка удаления role override")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// SetRoleOverride — POST /api/v1/admin-users/{id}/role-override.
// Устанавливает role override для пользователя.
// Доступ: admin.
func (h *APIHandler) SetRoleOverride(w http.ResponseWriter, r *http.Request, id generated.UserId) {
	claims := middleware.ClaimsFromContext(r.Context())
	if claims == nil || claims.SubjectType != middleware.SubjectTypeUser || !claims.HasRole("admin") {
		apierrors.Forbidden(w, "Недостаточно прав: требуется роль admin")
		return
	}

	var req generated.RoleOverrideRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierrors.ValidationError(w, "Некорректный JSON: "+err.Error())
		return
	}

	user, err := h.adminUsers.SetRoleOverride(r.Context(), id, string(req.Role), claims.PreferredUsername)
	if err != nil {
		if errors.Is(err, service.ErrInvalidRole) {
			apierrors.ValidationError(w, err.Error())
			return
		}
		h.logger.Error("Ошибка установки role override", "user_id", id, "error", err)
		apierrors.InternalError(w, "Ошибка установки role override")
		return
	}

	writeJSON(w, http.StatusOK, mapAdminUser(user))
}

// --- Маппинг domain → API ---

// mapAdminUser конвертирует domain model в generated API type.
func mapAdminUser(u *model.AdminUser) generated.AdminUser {
	result := generated.AdminUser{
		Id:            u.ID,
		Username:      u.Username,
		EffectiveRole: generated.AdminUserEffectiveRole(u.EffectiveRole),
		IdpRole:       generated.AdminUserIdpRole(u.IdpRole),
		CreatedAt:     u.CreatedAt,
	}

	if u.Email != "" {
		email := openapi_types.Email(u.Email)
		result.Email = &email
	}

	if u.FirstName != "" {
		result.FirstName = &u.FirstName
	}

	if u.LastName != "" {
		result.LastName = &u.LastName
	}

	enabled := u.Enabled
	result.Enabled = &enabled

	if len(u.Groups) > 0 {
		groups := u.Groups
		result.Groups = &groups
	}

	if u.RoleOverride != nil {
		override := generated.AdminUserRoleOverride(*u.RoleOverride)
		result.RoleOverride = &override
	}

	return result
}
