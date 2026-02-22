// idp.go — сервис статуса Identity Provider (Keycloak).
// GetStatus — проверка подключения, RealmInfo, подсчёт users/clients.
// SyncSA — принудительная синхронизация SA через SASyncService.
package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/arturkryukov/artsore/admin-module/internal/domain/model"
	"github.com/arturkryukov/artsore/admin-module/internal/keycloak"
	"github.com/arturkryukov/artsore/admin-module/internal/repository"
)

// IDPService — сервис статуса Identity Provider.
type IDPService struct {
	kcClient      *keycloak.Client
	saRepo        repository.ServiceAccountRepository
	syncStateRepo repository.SyncStateRepository
	saSyncSvc     *SASyncService
	keycloakURL   string
	realm         string
	saPrefix      string
	logger        *slog.Logger
}

// IDPStatus — статус подключения к Keycloak.
type IDPStatus struct {
	Connected    bool
	Realm        string
	KeycloakURL  string
	UsersCount   *int
	ClientsCount *int
	LastSASyncAt *time.Time
	Error        *string
}

// NewIDPService создаёт сервис статуса IdP.
func NewIDPService(
	kcClient *keycloak.Client,
	saRepo repository.ServiceAccountRepository,
	syncStateRepo repository.SyncStateRepository,
	keycloakURL, realm, saPrefix string,
	logger *slog.Logger,
) *IDPService {
	return &IDPService{
		kcClient:      kcClient,
		saRepo:        saRepo,
		syncStateRepo: syncStateRepo,
		keycloakURL:   keycloakURL,
		realm:         realm,
		saPrefix:      saPrefix,
		logger:        logger.With(slog.String("component", "idp_service")),
	}
}

// SetSASyncService устанавливает ссылку на SASyncService.
// Вызывается после создания обоих сервисов для избежания циклической зависимости.
func (s *IDPService) SetSASyncService(saSyncSvc *SASyncService) {
	s.saSyncSvc = saSyncSvc
}

// GetStatus возвращает статус подключения к Keycloak.
func (s *IDPService) GetStatus(ctx context.Context) *IDPStatus {
	status := &IDPStatus{
		Realm:       s.realm,
		KeycloakURL: s.keycloakURL,
	}

	// Проверяем доступность Keycloak
	_, err := s.kcClient.RealmInfo(ctx)
	if err != nil {
		errMsg := fmt.Sprintf("Keycloak недоступен: %v", err)
		status.Error = &errMsg
		status.Connected = false
		return status
	}

	status.Connected = true

	// Подсчёт пользователей
	usersCount, err := s.kcClient.CountUsers(ctx)
	if err != nil {
		s.logger.Warn("Ошибка подсчёта пользователей", slog.String("error", err.Error()))
	} else {
		status.UsersCount = &usersCount
	}

	// Подсчёт SA clients (с префиксом sa_*)
	clients, err := s.kcClient.ListClients(ctx, s.saPrefix, 0, 1000)
	if err != nil {
		s.logger.Warn("Ошибка получения списка SA clients", slog.String("error", err.Error()))
	} else {
		count := len(clients)
		status.ClientsCount = &count
	}

	// Время последней синхронизации SA
	syncState, err := s.syncStateRepo.Get(ctx)
	if err != nil {
		s.logger.Warn("Ошибка получения sync state", slog.String("error", err.Error()))
	} else {
		status.LastSASyncAt = syncState.LastSASyncAt
	}

	return status
}

// SyncSA выполняет принудительную синхронизацию SA с Keycloak.
// Делегирует SASyncService.SyncNow для полной reconciliation.
func (s *IDPService) SyncSA(ctx context.Context) (*model.SASyncResult, error) {
	s.logger.Info("Принудительная синхронизация SA запущена")

	if s.saSyncSvc == nil {
		return nil, fmt.Errorf("сервис синхронизации SA не инициализирован")
	}

	result, err := s.saSyncSvc.SyncNow(ctx)
	if err != nil {
		return nil, fmt.Errorf("синхронизация SA: %w", err)
	}

	s.logger.Info("Принудительная синхронизация SA завершена",
		slog.Int("total_local", result.TotalLocal),
		slog.Int("total_keycloak", result.TotalKeycloak),
		slog.Int("created_local", result.CreatedLocal),
		slog.Int("created_keycloak", result.CreatedKeycloak),
		slog.Int("updated", result.Updated),
	)

	return result, nil
}
