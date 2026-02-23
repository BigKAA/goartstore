// service_accounts.go — сервис управления Service Accounts.
// CRUD SA: создание в Keycloak (Client Credentials) + локальная БД,
// обновление, удаление, ротация секрета.
package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/bigkaa/goartstore/admin-module/internal/domain/model"
	"github.com/bigkaa/goartstore/admin-module/internal/keycloak"
	"github.com/bigkaa/goartstore/admin-module/internal/repository"
)

// ServiceAccountService — сервис управления Service Accounts.
type ServiceAccountService struct {
	kcClient *keycloak.Client
	saRepo   repository.ServiceAccountRepository
	saPrefix string // Префикс client_id (по умолчанию "sa_")
	logger   *slog.Logger
}

// NewServiceAccountService создаёт сервис Service Accounts.
func NewServiceAccountService(
	kcClient *keycloak.Client,
	saRepo repository.ServiceAccountRepository,
	saPrefix string,
	logger *slog.Logger,
) *ServiceAccountService {
	return &ServiceAccountService{
		kcClient: kcClient,
		saRepo:   saRepo,
		saPrefix: saPrefix,
		logger:   logger.With(slog.String("component", "sa_service")),
	}
}

// ServiceAccountWithSecret — SA с секретом (возвращается только при создании).
type ServiceAccountWithSecret struct {
	*model.ServiceAccount
	ClientSecret string
}

// Create создаёт новый Service Account: регистрирует в Keycloak + сохраняет в БД.
// Возвращает SA с client_secret (показывается только один раз).
func (s *ServiceAccountService) Create(ctx context.Context, name, description string, scopes []string) (*ServiceAccountWithSecret, error) {
	// Генерируем client_id: sa_<name>_<random>
	clientID := s.generateClientID(name)
	saID := uuid.New().String()

	// Создаём клиент в Keycloak
	kcInternalID, err := s.kcClient.CreateClient(ctx, clientID, name, description, scopes)
	if err != nil {
		return nil, fmt.Errorf("создание клиента в Keycloak: %w", err)
	}

	// Получаем секрет от Keycloak
	clientSecret, err := s.kcClient.GetClientSecret(ctx, kcInternalID)
	if err != nil {
		// Попытка очистки: удаляем созданного клиента в Keycloak
		_ = s.kcClient.DeleteClient(ctx, kcInternalID)
		return nil, fmt.Errorf("получение секрета из Keycloak: %w", err)
	}

	// Сохраняем в локальную БД
	var desc *string
	if description != "" {
		desc = &description
	}

	now := time.Now().UTC()
	sa := &model.ServiceAccount{
		ID:               saID,
		KeycloakClientID: &kcInternalID,
		ClientID:         clientID,
		Name:             name,
		Description:      desc,
		Scopes:           scopes,
		Status:           "active",
		Source:            "local",
		LastSyncedAt:     &now,
	}

	if err := s.saRepo.Create(ctx, sa); err != nil {
		// Попытка очистки: удаляем клиента из Keycloak
		_ = s.kcClient.DeleteClient(ctx, kcInternalID)
		if errors.Is(err, repository.ErrConflict) {
			return nil, fmt.Errorf("%w: SA с именем '%s' уже существует", ErrConflict, name)
		}
		return nil, fmt.Errorf("сохранение SA в БД: %w", err)
	}

	s.logger.Info("SA создан",
		slog.String("sa_id", saID),
		slog.String("client_id", clientID),
		slog.String("name", name),
	)

	return &ServiceAccountWithSecret{
		ServiceAccount: sa,
		ClientSecret:   clientSecret,
	}, nil
}

// List возвращает список SA с фильтрацией и пагинацией.
func (s *ServiceAccountService) List(ctx context.Context, status *string, limit, offset int) ([]*model.ServiceAccount, int, error) {
	sas, err := s.saRepo.List(ctx, status, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("получение списка SA: %w", err)
	}

	total, err := s.saRepo.Count(ctx, status)
	if err != nil {
		return nil, 0, fmt.Errorf("подсчёт SA: %w", err)
	}

	return sas, total, nil
}

// Get возвращает SA по ID.
func (s *ServiceAccountService) Get(ctx context.Context, id string) (*model.ServiceAccount, error) {
	sa, err := s.saRepo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("получение SA: %w", err)
	}
	return sa, nil
}

// Update обновляет SA: локальную БД + Keycloak.
func (s *ServiceAccountService) Update(ctx context.Context, id string, name, description *string, scopes []string, status *string) (*model.ServiceAccount, error) {
	// Получаем текущий SA из БД
	sa, err := s.saRepo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("получение SA для обновления: %w", err)
	}

	// Применяем обновления
	if name != nil {
		sa.Name = *name
	}
	if description != nil {
		sa.Description = description
	}
	if scopes != nil {
		sa.Scopes = scopes
	}
	if status != nil {
		sa.Status = *status
	}

	// Обновляем в Keycloak (если есть keycloak_client_id)
	if sa.KeycloakClientID != nil {
		kcClient := &keycloak.KeycloakClient{
			ID:                     *sa.KeycloakClientID,
			ClientID:               sa.ClientID,
			Name:                   sa.Name,
			Enabled:                sa.Status == "active",
			ServiceAccountsEnabled: true,
			DefaultClientScopes:    sa.Scopes,
		}
		if sa.Description != nil {
			kcClient.Description = *sa.Description
		}

		if err := s.kcClient.UpdateClient(ctx, *sa.KeycloakClientID, kcClient); err != nil {
			s.logger.Warn("Ошибка обновления SA в Keycloak",
				slog.String("sa_id", id),
				slog.String("error", err.Error()),
			)
			// Не прерываем — обновляем локально
		}
	}

	// Обновляем в БД
	if err := s.saRepo.Update(ctx, sa); err != nil {
		return nil, fmt.Errorf("обновление SA в БД: %w", err)
	}

	s.logger.Info("SA обновлён",
		slog.String("sa_id", id),
	)

	return sa, nil
}

// Delete удаляет SA: из БД + из Keycloak.
func (s *ServiceAccountService) Delete(ctx context.Context, id string) error {
	// Получаем SA для удаления из Keycloak
	sa, err := s.saRepo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return ErrNotFound
		}
		return fmt.Errorf("получение SA для удаления: %w", err)
	}

	// Удаляем из Keycloak (если есть keycloak_client_id)
	if sa.KeycloakClientID != nil {
		if err := s.kcClient.DeleteClient(ctx, *sa.KeycloakClientID); err != nil {
			s.logger.Warn("Ошибка удаления SA из Keycloak",
				slog.String("sa_id", id),
				slog.String("kc_client_id", *sa.KeycloakClientID),
				slog.String("error", err.Error()),
			)
			// Не прерываем — удаляем локально
		}
	}

	// Удаляем из БД
	if err := s.saRepo.Delete(ctx, id); err != nil {
		return fmt.Errorf("удаление SA из БД: %w", err)
	}

	s.logger.Info("SA удалён",
		slog.String("sa_id", id),
		slog.String("client_id", sa.ClientID),
	)

	return nil
}

// RotateSecret генерирует новый секрет SA в Keycloak.
// Возвращает client_id и новый секрет.
func (s *ServiceAccountService) RotateSecret(ctx context.Context, id string) (string, string, error) {
	// Получаем SA
	sa, err := s.saRepo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return "", "", ErrNotFound
		}
		return "", "", fmt.Errorf("получение SA для ротации: %w", err)
	}

	if sa.KeycloakClientID == nil {
		return "", "", fmt.Errorf("SA не синхронизирован с Keycloak: отсутствует keycloak_client_id")
	}

	// Регенерируем секрет в Keycloak
	newSecret, err := s.kcClient.RegenerateClientSecret(ctx, *sa.KeycloakClientID)
	if err != nil {
		return "", "", fmt.Errorf("регенерация секрета в Keycloak: %w", err)
	}

	s.logger.Info("Секрет SA ротирован",
		slog.String("sa_id", id),
		slog.String("client_id", sa.ClientID),
	)

	return sa.ClientID, newSecret, nil
}

// generateClientID генерирует уникальный client_id в формате: sa_<name>_<random>.
func (s *ServiceAccountService) generateClientID(name string) string {
	// Нормализуем имя: lowercase, заменяем пробелы на _
	normalized := strings.ToLower(strings.ReplaceAll(name, " ", "_"))

	// Генерируем 4 случайных байта (8 hex символов)
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	random := hex.EncodeToString(b)

	return fmt.Sprintf("%s%s_%s", s.saPrefix, normalized, random)
}
