// idp.go — обработчики /api/v1/idp endpoints.
// Статус Identity Provider (Keycloak), принудительная синхронизация SA.
package handlers

import (
	"errors"
	"net/http"
	"time"

	apierrors "github.com/bigkaa/goartstore/admin-module/internal/api/errors"
	"github.com/bigkaa/goartstore/admin-module/internal/api/generated"
	"github.com/bigkaa/goartstore/admin-module/internal/api/middleware"
	"github.com/bigkaa/goartstore/admin-module/internal/service"
)

// GetIdpStatus — GET /api/v1/idp/status.
// Статус подключения к Keycloak.
// Доступ: admin.
func (h *APIHandler) GetIdpStatus(w http.ResponseWriter, r *http.Request) {
	claims := middleware.ClaimsFromContext(r.Context())
	if claims == nil || claims.SubjectType != middleware.SubjectTypeUser || !claims.HasRole("admin") {
		apierrors.Forbidden(w, "Недостаточно прав: требуется роль admin")
		return
	}

	status := h.idp.GetStatus(r.Context())

	resp := generated.IdpStatus{
		Connected:    status.Connected,
		Realm:        status.Realm,
		UsersCount:   status.UsersCount,
		ClientsCount: status.ClientsCount,
		LastSaSyncAt: status.LastSASyncAt,
		Error:        status.Error,
	}

	if status.KeycloakURL != "" {
		resp.KeycloakUrl = &status.KeycloakURL
	}

	writeJSON(w, http.StatusOK, resp)
}

// SyncServiceAccounts — POST /api/v1/idp/sync-sa.
// Принудительная синхронизация SA с Keycloak.
// Доступ: admin.
func (h *APIHandler) SyncServiceAccounts(w http.ResponseWriter, r *http.Request) {
	claims := middleware.ClaimsFromContext(r.Context())
	if claims == nil || claims.SubjectType != middleware.SubjectTypeUser || !claims.HasRole("admin") {
		apierrors.Forbidden(w, "Недостаточно прав: требуется роль admin")
		return
	}

	result, err := h.idp.SyncSA(r.Context())
	if err != nil {
		if errors.Is(err, service.ErrIDPUnavailable) {
			apierrors.IDPUnavailable(w, err.Error())
			return
		}
		h.logger.Error("Ошибка синхронизации SA", "error", err)
		apierrors.InternalError(w, "Ошибка синхронизации SA с Keycloak")
		return
	}

	resp := generated.SASyncResult{
		CreatedLocal:    result.CreatedLocal,
		CreatedKeycloak: result.CreatedKeycloak,
		Updated:         result.Updated,
		TotalLocal:      result.TotalLocal,
		TotalKeycloak:   result.TotalKeycloak,
		SyncedAt:        time.Now().UTC(),
	}

	writeJSON(w, http.StatusOK, resp)
}
