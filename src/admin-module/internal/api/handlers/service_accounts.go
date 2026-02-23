// service_accounts.go — обработчики /api/v1/service-accounts endpoints.
// CRUD SA: создание, список, получение, обновление, удаление, ротация секрета.
package handlers

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/google/uuid"

	apierrors "github.com/bigkaa/goartstore/admin-module/internal/api/errors"
	"github.com/bigkaa/goartstore/admin-module/internal/api/generated"
	"github.com/bigkaa/goartstore/admin-module/internal/api/middleware"
	"github.com/bigkaa/goartstore/admin-module/internal/domain/model"
	"github.com/bigkaa/goartstore/admin-module/internal/service"
)

// CreateServiceAccount — POST /api/v1/service-accounts.
// Создаёт SA в Keycloak + локальной БД.
// Доступ: admin.
func (h *APIHandler) CreateServiceAccount(w http.ResponseWriter, r *http.Request) {
	claims := middleware.ClaimsFromContext(r.Context())
	if claims == nil || claims.SubjectType != middleware.SubjectTypeUser || !claims.HasRole("admin") {
		apierrors.Forbidden(w, "Недостаточно прав: требуется роль admin")
		return
	}

	var req generated.ServiceAccountCreate
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierrors.ValidationError(w, "Некорректный JSON: "+err.Error())
		return
	}

	// Валидация
	if req.Name == "" || len(req.Name) < 2 || len(req.Name) > 50 {
		apierrors.ValidationError(w, "Имя SA должно быть от 2 до 50 символов")
		return
	}
	if len(req.Scopes) == 0 {
		apierrors.ValidationError(w, "Необходимо указать хотя бы один scope")
		return
	}

	// Маппинг scopes из generated type в []string
	scopes := make([]string, len(req.Scopes))
	for i, s := range req.Scopes {
		scopes[i] = string(s)
	}

	var description string
	if req.Description != nil {
		description = *req.Description
	}

	result, err := h.serviceAccts.Create(r.Context(), req.Name, description, scopes)
	if err != nil {
		if errors.Is(err, service.ErrConflict) {
			apierrors.Conflict(w, err.Error())
			return
		}
		h.logger.Error("Ошибка создания SA", "error", err)
		apierrors.InternalError(w, "Ошибка создания сервисного аккаунта")
		return
	}

	writeJSON(w, http.StatusCreated, mapServiceAccountWithSecret(result))
}

// ListServiceAccounts — GET /api/v1/service-accounts.
// Возвращает список SA.
// Доступ: admin или readonly.
func (h *APIHandler) ListServiceAccounts(w http.ResponseWriter, r *http.Request, params generated.ListServiceAccountsParams) {
	claims := middleware.ClaimsFromContext(r.Context())
	if claims == nil || claims.SubjectType != middleware.SubjectTypeUser || !claims.HasAnyRole("admin", "readonly") {
		apierrors.Forbidden(w, "Недостаточно прав: требуется роль admin или readonly")
		return
	}

	limit, offset := paginationDefaults(params.Limit, params.Offset)

	var status *string
	if params.Status != nil {
		s := string(*params.Status)
		status = &s
	}

	sas, total, err := h.serviceAccts.List(r.Context(), status, limit, offset)
	if err != nil {
		h.logger.Error("Ошибка получения списка SA", "error", err)
		apierrors.InternalError(w, "Ошибка получения списка сервисных аккаунтов")
		return
	}

	items := make([]generated.ServiceAccount, len(sas))
	for i, sa := range sas {
		items[i] = mapServiceAccount(sa)
	}

	resp := generated.ServiceAccountListResponse{
		Items:   items,
		Total:   total,
		Limit:   limit,
		Offset:  offset,
		HasMore: offset+limit < total,
	}

	writeJSON(w, http.StatusOK, resp)
}

// GetServiceAccount — GET /api/v1/service-accounts/{id}.
// Возвращает SA по ID.
// Доступ: admin или readonly.
func (h *APIHandler) GetServiceAccount(w http.ResponseWriter, r *http.Request, id generated.ServiceAccountId) {
	claims := middleware.ClaimsFromContext(r.Context())
	if claims == nil || claims.SubjectType != middleware.SubjectTypeUser || !claims.HasAnyRole("admin", "readonly") {
		apierrors.Forbidden(w, "Недостаточно прав: требуется роль admin или readonly")
		return
	}

	sa, err := h.serviceAccts.Get(r.Context(), id.String())
	if err != nil {
		if errors.Is(err, service.ErrNotFound) {
			apierrors.NotFound(w, "Сервисный аккаунт не найден")
			return
		}
		h.logger.Error("Ошибка получения SA", "sa_id", id, "error", err)
		apierrors.InternalError(w, "Ошибка получения сервисного аккаунта")
		return
	}

	writeJSON(w, http.StatusOK, mapServiceAccount(sa))
}

// UpdateServiceAccount — PUT /api/v1/service-accounts/{id}.
// Обновляет SA.
// Доступ: admin.
func (h *APIHandler) UpdateServiceAccount(w http.ResponseWriter, r *http.Request, id generated.ServiceAccountId) {
	claims := middleware.ClaimsFromContext(r.Context())
	if claims == nil || claims.SubjectType != middleware.SubjectTypeUser || !claims.HasRole("admin") {
		apierrors.Forbidden(w, "Недостаточно прав: требуется роль admin")
		return
	}

	var req generated.ServiceAccountUpdate
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierrors.ValidationError(w, "Некорректный JSON: "+err.Error())
		return
	}

	// Маппинг scopes
	var scopes []string
	if req.Scopes != nil {
		scopes = make([]string, len(*req.Scopes))
		for i, s := range *req.Scopes {
			scopes[i] = string(s)
		}
	}

	var status *string
	if req.Status != nil {
		s := string(*req.Status)
		status = &s
	}

	sa, err := h.serviceAccts.Update(r.Context(), id.String(), req.Name, req.Description, scopes, status)
	if err != nil {
		if errors.Is(err, service.ErrNotFound) {
			apierrors.NotFound(w, "Сервисный аккаунт не найден")
			return
		}
		h.logger.Error("Ошибка обновления SA", "sa_id", id, "error", err)
		apierrors.InternalError(w, "Ошибка обновления сервисного аккаунта")
		return
	}

	writeJSON(w, http.StatusOK, mapServiceAccount(sa))
}

// DeleteServiceAccount — DELETE /api/v1/service-accounts/{id}.
// Удаляет SA из БД и Keycloak.
// Доступ: admin.
func (h *APIHandler) DeleteServiceAccount(w http.ResponseWriter, r *http.Request, id generated.ServiceAccountId) {
	claims := middleware.ClaimsFromContext(r.Context())
	if claims == nil || claims.SubjectType != middleware.SubjectTypeUser || !claims.HasRole("admin") {
		apierrors.Forbidden(w, "Недостаточно прав: требуется роль admin")
		return
	}

	if err := h.serviceAccts.Delete(r.Context(), id.String()); err != nil {
		if errors.Is(err, service.ErrNotFound) {
			apierrors.NotFound(w, "Сервисный аккаунт не найден")
			return
		}
		h.logger.Error("Ошибка удаления SA", "sa_id", id, "error", err)
		apierrors.InternalError(w, "Ошибка удаления сервисного аккаунта")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// RotateSecret — POST /api/v1/service-accounts/{id}/rotate-secret.
// Ротация секрета SA в Keycloak.
// Доступ: admin.
func (h *APIHandler) RotateSecret(w http.ResponseWriter, r *http.Request, id generated.ServiceAccountId) {
	claims := middleware.ClaimsFromContext(r.Context())
	if claims == nil || claims.SubjectType != middleware.SubjectTypeUser || !claims.HasRole("admin") {
		apierrors.Forbidden(w, "Недостаточно прав: требуется роль admin")
		return
	}

	clientID, newSecret, err := h.serviceAccts.RotateSecret(r.Context(), id.String())
	if err != nil {
		if errors.Is(err, service.ErrNotFound) {
			apierrors.NotFound(w, "Сервисный аккаунт не найден")
			return
		}
		h.logger.Error("Ошибка ротации секрета", "sa_id", id, "error", err)
		apierrors.InternalError(w, "Ошибка ротации секрета")
		return
	}

	resp := generated.RotateSecretResponse{
		ClientId:     clientID,
		ClientSecret: newSecret,
	}

	writeJSON(w, http.StatusOK, resp)
}

// --- Маппинг domain → API ---

// mapServiceAccount конвертирует domain model в generated API type.
func mapServiceAccount(sa *model.ServiceAccount) generated.ServiceAccount {
	scopes := make([]generated.ServiceAccountScopes, len(sa.Scopes))
	for i, s := range sa.Scopes {
		scopes[i] = generated.ServiceAccountScopes(s)
	}

	result := generated.ServiceAccount{
		Id:        uuid.MustParse(sa.ID),
		ClientId:  sa.ClientID,
		Name:      sa.Name,
		Scopes:    scopes,
		Status:    generated.ServiceAccountStatus(sa.Status),
		Source:    generated.ServiceAccountSource(sa.Source),
		CreatedAt: sa.CreatedAt,
		UpdatedAt: sa.UpdatedAt,
	}

	result.Description = sa.Description
	result.KeycloakClientId = sa.KeycloakClientID
	result.LastSyncedAt = sa.LastSyncedAt

	return result
}

// mapServiceAccountWithSecret конвертирует SA с секретом в generated API type.
func mapServiceAccountWithSecret(saws *service.ServiceAccountWithSecret) generated.ServiceAccountWithSecret {
	sa := saws.ServiceAccount

	scopes := make([]generated.ServiceAccountWithSecretScopes, len(sa.Scopes))
	for i, s := range sa.Scopes {
		scopes[i] = generated.ServiceAccountWithSecretScopes(s)
	}

	result := generated.ServiceAccountWithSecret{
		Id:           uuid.MustParse(sa.ID),
		ClientId:     sa.ClientID,
		ClientSecret: saws.ClientSecret,
		Name:         sa.Name,
		Scopes:       scopes,
		Status:       generated.ServiceAccountWithSecretStatus(sa.Status),
		Source:       generated.ServiceAccountWithSecretSource(sa.Source),
		CreatedAt:    sa.CreatedAt,
		UpdatedAt:    sa.UpdatedAt,
	}

	result.Description = sa.Description
	result.KeycloakClientId = sa.KeycloakClientID
	result.LastSyncedAt = sa.LastSyncedAt

	return result
}
