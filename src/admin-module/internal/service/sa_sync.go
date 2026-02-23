// sa_sync.go — сервис периодической синхронизации Service Accounts с Keycloak.
//
// SASyncService запускает фоновую горутину с ticker (AM_SA_SYNC_INTERVAL),
// которая выполняет reconciliation SA между локальной БД и Keycloak.
//
// Reconciliation:
//  1. Получить SA clients из Keycloak (с префиксом sa_*)
//  2. Получить SA из локальной БД
//  3. В Keycloak, но не локально → создать в локальной БД (source=keycloak)
//  4. Локально, но не в Keycloak → создать в Keycloak (source=local)
//  5. В обоих → сравнить scopes, обновить при расхождении (local wins)
//
// Prometheus-метрики:
//   - admin_module_sa_sync_duration_seconds — длительность синхронизации SA
package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"github.com/arturkryukov/artstore/admin-module/internal/domain/model"
	"github.com/arturkryukov/artstore/admin-module/internal/keycloak"
	"github.com/arturkryukov/artstore/admin-module/internal/repository"
)

// Prometheus-метрики для синхронизации SA.
var saSyncDuration = promauto.NewHistogram(prometheus.HistogramOpts{
	Name:    "admin_module_sa_sync_duration_seconds",
	Help:    "Длительность синхронизации Service Accounts с Keycloak",
	Buckets: prometheus.ExponentialBuckets(0.1, 2, 10), // 0.1s … ~51s
})

// SASyncService — фоновый сервис синхронизации SA с Keycloak.
type SASyncService struct {
	kcClient      *keycloak.Client
	saRepo        repository.ServiceAccountRepository
	syncStateRepo repository.SyncStateRepository
	saPrefix      string
	interval      time.Duration
	logger        *slog.Logger

	cancel context.CancelFunc
	done   chan struct{}
}

// NewSASyncService создаёт сервис синхронизации SA.
func NewSASyncService(
	kcClient *keycloak.Client,
	saRepo repository.ServiceAccountRepository,
	syncStateRepo repository.SyncStateRepository,
	saPrefix string,
	interval time.Duration,
	logger *slog.Logger,
) *SASyncService {
	return &SASyncService{
		kcClient:      kcClient,
		saRepo:        saRepo,
		syncStateRepo: syncStateRepo,
		saPrefix:      saPrefix,
		interval:      interval,
		logger:        logger.With(slog.String("component", "sa_sync")),
	}
}

// Start запускает фоновую горутину с периодической синхронизацией SA.
func (s *SASyncService) Start(ctx context.Context) {
	ctx, s.cancel = context.WithCancel(ctx)
	s.done = make(chan struct{})

	go func() {
		defer close(s.done)

		s.logger.Info("Периодическая синхронизация SA запущена",
			slog.String("interval", s.interval.String()),
			slog.String("sa_prefix", s.saPrefix),
		)

		ticker := time.NewTicker(s.interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				s.logger.Info("Периодическая синхронизация SA остановлена")
				return
			case <-ticker.C:
				s.logger.Info("Запуск периодической синхронизации SA")
				result, err := s.SyncNow(ctx)
				if err != nil {
					s.logger.Error("Ошибка периодической синхронизации SA",
						slog.String("error", err.Error()),
					)
				} else {
					s.logger.Info("Периодическая синхронизация SA завершена",
						slog.Int("total_local", result.TotalLocal),
						slog.Int("total_keycloak", result.TotalKeycloak),
						slog.Int("created_local", result.CreatedLocal),
						slog.Int("created_keycloak", result.CreatedKeycloak),
						slog.Int("updated", result.Updated),
					)
				}
			}
		}
	}()
}

// Stop останавливает фоновую горутину и ждёт завершения.
func (s *SASyncService) Stop() {
	if s.cancel != nil {
		s.cancel()
	}
	if s.done != nil {
		<-s.done
	}
}

// SyncNow выполняет немедленную синхронизацию SA с Keycloak.
// Reconciliation: сравниваем локальные SA с Keycloak clients.
func (s *SASyncService) SyncNow(ctx context.Context) (*model.SASyncResult, error) {
	startedAt := time.Now().UTC()

	// 1. Получаем SA clients из Keycloak (с префиксом sa_*)
	kcClients, err := s.kcClient.ListClients(ctx, s.saPrefix, 0, 1000)
	if err != nil {
		return nil, fmt.Errorf("получение SA clients из Keycloak: %w", err)
	}

	// 2. Получаем все SA из локальной БД
	localSAs, err := s.saRepo.List(ctx, nil, 10000, 0)
	if err != nil {
		return nil, fmt.Errorf("получение локальных SA: %w", err)
	}

	// 3. Строим карты по client_id для быстрого поиска
	kcMap := make(map[string]keycloak.KeycloakClient, len(kcClients))
	for _, kc := range kcClients {
		kcMap[kc.ClientID] = kc
	}

	localMap := make(map[string]*model.ServiceAccount, len(localSAs))
	for _, sa := range localSAs {
		localMap[sa.ClientID] = sa
	}

	now := time.Now().UTC()
	var createdLocal, createdKeycloak, updated int

	// 4. В Keycloak, но не локально → создать в локальной БД
	for _, kc := range kcClients {
		if _, exists := localMap[kc.ClientID]; !exists {
			if err := s.createLocalFromKC(ctx, kc, now); err != nil {
				s.logger.Warn("Ошибка создания локального SA из Keycloak",
					slog.String("client_id", kc.ClientID),
					slog.String("error", err.Error()),
				)
				continue
			}
			createdLocal++
			s.logger.Info("SA создан локально из Keycloak",
				slog.String("client_id", kc.ClientID),
			)
		}
	}

	// 5. Локально, но не в Keycloak → создать в Keycloak
	for _, sa := range localSAs {
		if _, exists := kcMap[sa.ClientID]; !exists {
			if err := s.createKCFromLocal(ctx, sa, now); err != nil {
				s.logger.Warn("Ошибка создания SA в Keycloak из локальной БД",
					slog.String("client_id", sa.ClientID),
					slog.String("error", err.Error()),
				)
				continue
			}
			createdKeycloak++
			s.logger.Info("SA создан в Keycloak из локальной БД",
				slog.String("client_id", sa.ClientID),
			)
		}
	}

	// 6. В обоих → сравнить scopes, обновить при расхождении
	for _, sa := range localSAs {
		kc, exists := kcMap[sa.ClientID]
		if !exists {
			continue
		}

		if scopesDiffer(sa.Scopes, kc.DefaultClientScopes) {
			if err := s.reconcileScopes(ctx, sa, kc, now); err != nil {
				s.logger.Warn("Ошибка reconcile scopes SA",
					slog.String("client_id", sa.ClientID),
					slog.String("error", err.Error()),
				)
				continue
			}
			updated++
			s.logger.Info("SA scopes обновлены",
				slog.String("client_id", sa.ClientID),
			)
		}

		// Обновляем keycloak_client_id если не установлен
		if sa.KeycloakClientID == nil || *sa.KeycloakClientID != kc.ID {
			sa.KeycloakClientID = &kc.ID
			sa.LastSyncedAt = &now
			if err := s.saRepo.Update(ctx, sa); err != nil {
				s.logger.Warn("Ошибка обновления keycloak_client_id",
					slog.String("client_id", sa.ClientID),
					slog.String("error", err.Error()),
				)
			}
		}
	}

	// 7. Обновляем timestamp синхронизации
	if err := s.syncStateRepo.UpdateSASyncAt(ctx, now); err != nil {
		s.logger.Warn("Ошибка обновления last_sa_sync_at", slog.String("error", err.Error()))
	}

	completedAt := time.Now().UTC()

	// 8. Prometheus-метрика
	saSyncDuration.Observe(completedAt.Sub(startedAt).Seconds())

	// Подсчёт финальных итогов
	totalLocal, _ := s.saRepo.Count(ctx, nil)

	result := &model.SASyncResult{
		TotalLocal:      totalLocal,
		TotalKeycloak:   len(kcClients),
		CreatedLocal:    createdLocal,
		CreatedKeycloak: createdKeycloak,
		Updated:         updated,
		SyncedAt:        now,
	}

	return result, nil
}

// createLocalFromKC создаёт локальный SA из Keycloak client.
func (s *SASyncService) createLocalFromKC(ctx context.Context, kc keycloak.KeycloakClient, now time.Time) error {
	name := kc.Name
	if name == "" {
		name = kc.ClientID
	}

	var desc *string
	if kc.Description != "" {
		desc = &kc.Description
	}

	status := "active"
	if !kc.Enabled {
		status = "suspended"
	}

	sa := &model.ServiceAccount{
		ID:               uuid.New().String(),
		KeycloakClientID: &kc.ID,
		ClientID:         kc.ClientID,
		Name:             name,
		Description:      desc,
		Scopes:           kc.DefaultClientScopes,
		Status:           status,
		Source:           "keycloak",
		LastSyncedAt:     &now,
	}

	return s.saRepo.Create(ctx, sa)
}

// createKCFromLocal создаёт Keycloak client из локального SA.
func (s *SASyncService) createKCFromLocal(ctx context.Context, sa *model.ServiceAccount, now time.Time) error {
	desc := ""
	if sa.Description != nil {
		desc = *sa.Description
	}

	kcID, err := s.kcClient.CreateClient(ctx, sa.ClientID, sa.Name, desc, sa.Scopes)
	if err != nil {
		return fmt.Errorf("создание client в Keycloak: %w", err)
	}

	// Обновляем локальный SA с Keycloak ID
	sa.KeycloakClientID = &kcID
	sa.LastSyncedAt = &now
	if err := s.saRepo.Update(ctx, sa); err != nil {
		return fmt.Errorf("обновление локального SA после создания в KC: %w", err)
	}

	return nil
}

// reconcileScopes обновляет scopes: локальная БД — источник истины.
// При расхождении scopes обновляем Keycloak.
func (s *SASyncService) reconcileScopes(ctx context.Context, sa *model.ServiceAccount, kc keycloak.KeycloakClient, now time.Time) error {
	// Обновляем Keycloak client с локальными scopes
	updatedKC := &keycloak.KeycloakClient{
		ID:                     kc.ID,
		ClientID:               kc.ClientID,
		Name:                   kc.Name,
		Description:            kc.Description,
		Enabled:                kc.Enabled,
		ServiceAccountsEnabled: kc.ServiceAccountsEnabled,
		DefaultClientScopes:    sa.Scopes,
	}

	if err := s.kcClient.UpdateClient(ctx, kc.ID, updatedKC); err != nil {
		return fmt.Errorf("обновление scopes в Keycloak: %w", err)
	}

	// Обновляем last_synced_at локально
	sa.LastSyncedAt = &now
	if err := s.saRepo.Update(ctx, sa); err != nil {
		return fmt.Errorf("обновление last_synced_at: %w", err)
	}

	return nil
}

// scopesDiffer сравнивает два набора scopes.
// Возвращает true, если наборы различаются.
func scopesDiffer(a, b []string) bool {
	if len(a) != len(b) {
		return true
	}

	setA := make(map[string]struct{}, len(a))
	for _, s := range a {
		setA[s] = struct{}{}
	}

	for _, s := range b {
		if _, ok := setA[s]; !ok {
			return true
		}
	}

	return false
}
