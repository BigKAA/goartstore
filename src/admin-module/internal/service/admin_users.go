// Пакет service — бизнес-логика Admin Module.
// admin_users.go — сервис управления пользователями (Keycloak + локальные role overrides).
package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/arturkryukov/artstore/admin-module/internal/api/middleware"
	"github.com/arturkryukov/artstore/admin-module/internal/domain/model"
	"github.com/arturkryukov/artstore/admin-module/internal/domain/rbac"
	"github.com/arturkryukov/artstore/admin-module/internal/keycloak"
	"github.com/arturkryukov/artstore/admin-module/internal/repository"
)

// AdminUserService — сервис управления пользователями.
// Объединяет данные из Keycloak (основной источник) с локальными role overrides.
type AdminUserService struct {
	kcClient       *keycloak.Client
	roleRepo       repository.RoleOverrideRepository
	adminGroups    []string
	readonlyGroups []string
	logger         *slog.Logger
}

// NewAdminUserService создаёт сервис управления пользователями.
func NewAdminUserService(
	kcClient *keycloak.Client,
	roleRepo repository.RoleOverrideRepository,
	adminGroups, readonlyGroups []string,
	logger *slog.Logger,
) *AdminUserService {
	return &AdminUserService{
		kcClient:       kcClient,
		roleRepo:       roleRepo,
		adminGroups:    adminGroups,
		readonlyGroups: readonlyGroups,
		logger:         logger.With(slog.String("component", "admin_users_service")),
	}
}

// GetCurrentUser возвращает данные текущего пользователя из JWT claims.
func (s *AdminUserService) GetCurrentUser(claims *middleware.AuthClaims) *model.AdminUser {
	return &model.AdminUser{
		ID:            claims.Subject,
		Username:      claims.PreferredUsername,
		Email:         claims.Email,
		Groups:        claims.Groups,
		IdpRole:       claims.IdpRole,
		RoleOverride:  claims.RoleOverride,
		EffectiveRole: claims.EffectiveRole,
	}
}

// ListUsers возвращает список пользователей из Keycloak с role overrides.
func (s *AdminUserService) ListUsers(ctx context.Context, limit, offset int) ([]*model.AdminUser, int, error) {
	// Получаем пользователей из Keycloak
	kcUsers, err := s.kcClient.ListUsers(ctx, "", offset, limit)
	if err != nil {
		return nil, 0, fmt.Errorf("получение пользователей из Keycloak: %w", err)
	}

	// Получаем общее количество пользователей
	total, err := s.kcClient.CountUsers(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("подсчёт пользователей в Keycloak: %w", err)
	}

	// Формируем AdminUser для каждого пользователя
	users := make([]*model.AdminUser, 0, len(kcUsers))
	for _, kcUser := range kcUsers {
		user, err := s.enrichUser(ctx, &kcUser)
		if err != nil {
			s.logger.Warn("Ошибка обогащения пользователя",
				slog.String("user_id", kcUser.ID),
				slog.String("error", err.Error()),
			)
			// Добавляем пользователя без обогащения
			users = append(users, s.basicUser(&kcUser))
			continue
		}
		users = append(users, user)
	}

	return users, total, nil
}

// GetUser возвращает пользователя по Keycloak ID.
func (s *AdminUserService) GetUser(ctx context.Context, id string) (*model.AdminUser, error) {
	kcUser, err := s.kcClient.GetUser(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("получение пользователя из Keycloak: %w", err)
	}

	user, err := s.enrichUser(ctx, kcUser)
	if err != nil {
		s.logger.Warn("Ошибка обогащения пользователя, используем базовые данные",
			slog.String("user_id", id),
			slog.String("error", err.Error()),
		)
		return s.basicUser(kcUser), nil
	}

	return user, nil
}

// UpdateUser обновляет role override пользователя (через PUT).
// Если roleOverride == nil, удаляет override.
func (s *AdminUserService) UpdateUser(ctx context.Context, id string, roleOverride *string, updatedBy string) (*model.AdminUser, error) {
	// Проверяем, что пользователь существует в Keycloak
	kcUser, err := s.kcClient.GetUser(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("получение пользователя из Keycloak: %w", err)
	}

	if roleOverride == nil {
		// Удаляем override
		err = s.roleRepo.Delete(ctx, id)
		if err != nil && !errors.Is(err, repository.ErrNotFound) {
			return nil, fmt.Errorf("удаление role override: %w", err)
		}
	} else {
		// Валидируем роль
		if !rbac.IsValidRole(*roleOverride) {
			return nil, ErrInvalidRole
		}
		// Upsert override
		ro := &model.RoleOverride{
			KeycloakUserID: id,
			Username:       kcUser.Username,
			AdditionalRole: *roleOverride,
			CreatedBy:      updatedBy,
		}
		if err := s.roleRepo.Upsert(ctx, ro); err != nil {
			return nil, fmt.Errorf("установка role override: %w", err)
		}
	}

	// Возвращаем обновлённого пользователя
	return s.GetUser(ctx, id)
}

// DeleteUser удаляет role override пользователя.
func (s *AdminUserService) DeleteUser(ctx context.Context, id string) error {
	// Проверяем, что override существует
	err := s.roleRepo.Delete(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return ErrNotFound
		}
		return fmt.Errorf("удаление role override: %w", err)
	}
	return nil
}

// SetRoleOverride устанавливает role override для пользователя.
func (s *AdminUserService) SetRoleOverride(ctx context.Context, id, role, createdBy string) (*model.AdminUser, error) {
	// Валидируем роль
	if !rbac.IsValidRole(role) {
		return nil, ErrInvalidRole
	}

	// Проверяем, что пользователь существует в Keycloak
	kcUser, err := s.kcClient.GetUser(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("получение пользователя из Keycloak: %w", err)
	}

	// Upsert override
	ro := &model.RoleOverride{
		KeycloakUserID: id,
		Username:       kcUser.Username,
		AdditionalRole: role,
		CreatedBy:      createdBy,
	}
	if err := s.roleRepo.Upsert(ctx, ro); err != nil {
		return nil, fmt.Errorf("установка role override: %w", err)
	}

	// Возвращаем обновлённого пользователя
	return s.GetUser(ctx, id)
}

// enrichUser обогащает данные пользователя Keycloak группами и role overrides.
func (s *AdminUserService) enrichUser(ctx context.Context, kcUser *keycloak.KeycloakUser) (*model.AdminUser, error) {
	// Получаем группы пользователя
	kcGroups, err := s.kcClient.GetUserGroups(ctx, kcUser.ID)
	if err != nil {
		return nil, fmt.Errorf("получение групп: %w", err)
	}

	groups := make([]string, len(kcGroups))
	for i, g := range kcGroups {
		groups[i] = g.Name
	}

	// Вычисляем роль из групп
	idpRole := rbac.MapGroupsToRole(groups, s.adminGroups, s.readonlyGroups)

	// Получаем role override из БД
	var roleOverride *string
	ro, err := s.roleRepo.GetByKeycloakUserID(ctx, kcUser.ID)
	if err == nil {
		roleOverride = &ro.AdditionalRole
	} else if !errors.Is(err, repository.ErrNotFound) {
		s.logger.Warn("Ошибка получения role override",
			slog.String("user_id", kcUser.ID),
			slog.String("error", err.Error()),
		)
	}

	// Вычисляем effective role
	effectiveRole := rbac.EffectiveRole(idpRole, roleOverride)

	return &model.AdminUser{
		ID:            kcUser.ID,
		Username:      kcUser.Username,
		Email:         kcUser.Email,
		FirstName:     kcUser.FirstName,
		LastName:      kcUser.LastName,
		Enabled:       kcUser.Enabled,
		Groups:        groups,
		IdpRole:       idpRole,
		RoleOverride:  roleOverride,
		EffectiveRole: effectiveRole,
		CreatedAt:     kcUser.CreatedAtTime(),
	}, nil
}

// basicUser создаёт AdminUser без обогащения (fallback при ошибке).
func (s *AdminUserService) basicUser(kcUser *keycloak.KeycloakUser) *model.AdminUser {
	return &model.AdminUser{
		ID:        kcUser.ID,
		Username:  kcUser.Username,
		Email:     kcUser.Email,
		FirstName: kcUser.FirstName,
		LastName:  kcUser.LastName,
		Enabled:   kcUser.Enabled,
		CreatedAt: kcUser.CreatedAtTime(),
	}
}
