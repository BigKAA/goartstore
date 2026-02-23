// admin_auth.go — обработчик /api/v1/admin-auth endpoints.
// GET /api/v1/admin-auth/me — текущий пользователь из JWT claims.
package handlers

import (
	"net/http"

	openapi_types "github.com/oapi-codegen/runtime/types"

	apierrors "github.com/bigkaa/goartstore/admin-module/internal/api/errors"
	"github.com/bigkaa/goartstore/admin-module/internal/api/generated"
	"github.com/bigkaa/goartstore/admin-module/internal/api/middleware"
)

// GetCurrentUser — GET /api/v1/admin-auth/me.
// Возвращает данные текущего пользователя из JWT claims.
// Доступ: любой аутентифицированный пользователь (admin или readonly).
func (h *APIHandler) GetCurrentUser(w http.ResponseWriter, r *http.Request) {
	claims := middleware.ClaimsFromContext(r.Context())
	if claims == nil {
		apierrors.Unauthorized(w, "Отсутствуют claims в контексте")
		return
	}

	// Только для Admin Users
	if claims.SubjectType != middleware.SubjectTypeUser {
		apierrors.Forbidden(w, "Endpoint доступен только для пользователей")
		return
	}

	user := h.adminUsers.GetCurrentUser(claims)

	// Маппинг domain model → generated API type
	resp := generated.CurrentUser{
		Id:            user.ID,
		Username:      user.Username,
		EffectiveRole: generated.CurrentUserEffectiveRole(user.EffectiveRole),
		IdpRole:       generated.CurrentUserIdpRole(user.IdpRole),
	}

	if user.Email != "" {
		email := openapi_types.Email(user.Email)
		resp.Email = &email
	}

	if len(user.Groups) > 0 {
		groups := user.Groups
		resp.Groups = &groups
	}

	if user.RoleOverride != nil {
		override := generated.CurrentUserRoleOverride(*user.RoleOverride)
		resp.RoleOverride = &override
	}

	writeJSON(w, http.StatusOK, resp)
}
