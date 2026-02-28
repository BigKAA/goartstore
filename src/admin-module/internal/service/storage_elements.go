// storage_elements.go — сервис управления Storage Elements.
// CRUD SE: discover, регистрация, обновление, удаление.
// Sync делегирует синхронизацию StorageSyncService (Phase 5).
package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/bigkaa/goartstore/admin-module/internal/domain/model"
	"github.com/bigkaa/goartstore/admin-module/internal/repository"
	"github.com/bigkaa/goartstore/admin-module/internal/seclient"
)

// StorageElementService — сервис управления Storage Elements.
type StorageElementService struct {
	seClient     *seclient.Client
	seRepo       repository.StorageElementRepository
	fileRepo     repository.FileRegistryRepository
	syncSvc      *StorageSyncService
	dephealthSvc *DephealthService
	logger       *slog.Logger
}

// NewStorageElementService создаёт сервис Storage Elements.
func NewStorageElementService(
	seClient *seclient.Client,
	seRepo repository.StorageElementRepository,
	fileRepo repository.FileRegistryRepository,
	logger *slog.Logger,
) *StorageElementService {
	return &StorageElementService{
		seClient: seClient,
		seRepo:   seRepo,
		fileRepo: fileRepo,
		logger:   logger.With(slog.String("component", "se_service")),
	}
}

// SetSyncService устанавливает ссылку на StorageSyncService.
// Вызывается после создания обоих сервисов для избежания циклической зависимости.
func (s *StorageElementService) SetSyncService(syncSvc *StorageSyncService) {
	s.syncSvc = syncSvc
}

// SetDephealthService устанавливает ссылку на DephealthService для мониторинга SE.
// Вызывается после создания dephealth, аналогично SetSyncService.
// Если nil — мониторинг SE через dephealth отключён.
func (s *StorageElementService) SetDephealthService(dephealthSvc *DephealthService) {
	s.dephealthSvc = dephealthSvc
}

// DiscoverResult — результат предпросмотра SE.
type DiscoverResult struct {
	StorageID      string
	Mode           string
	Status         string
	Version        string
	TotalBytes     int64
	UsedBytes      int64
	AvailableBytes int64
}

// Discover запрашивает информацию о SE (GET /api/v1/info).
// Не регистрирует SE в БД — только предпросмотр.
func (s *StorageElementService) Discover(ctx context.Context, url string) (*DiscoverResult, error) {
	info, err := s.seClient.Info(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrSEUnavailable, err) //nolint:errorlint // намеренный двойной wrap
	}

	result := &DiscoverResult{
		StorageID: info.StorageID,
		Mode:      info.Mode,
		Status:    info.Status,
		Version:   info.Version,
	}

	if info.Capacity != nil {
		result.TotalBytes = info.Capacity.TotalBytes
		result.UsedBytes = info.Capacity.UsedBytes
		result.AvailableBytes = info.Capacity.AvailableBytes
	}

	return result, nil
}

// Create регистрирует новый SE: discover + сохранение в БД + полная синхронизация файлов.
func (s *StorageElementService) Create(ctx context.Context, name, url string) (*model.StorageElement, error) {
	// Предпросмотр SE
	info, err := s.seClient.Info(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrSEUnavailable, err) //nolint:errorlint // намеренный двойной wrap
	}

	seID := uuid.New().String()
	now := time.Now().UTC()

	se := &model.StorageElement{
		ID:         seID,
		Name:       name,
		URL:        url,
		StorageID:  info.StorageID,
		Mode:       info.Mode,
		Status:     info.Status,
		LastSyncAt: &now,
	}

	if info.Capacity != nil {
		se.CapacityBytes = info.Capacity.TotalBytes
		se.UsedBytes = info.Capacity.UsedBytes
		avail := info.Capacity.AvailableBytes
		se.AvailableBytes = &avail
	}

	// Сохраняем в БД
	if err := s.seRepo.Create(ctx, se); err != nil {
		if errors.Is(err, repository.ErrConflict) {
			return nil, fmt.Errorf("%w: SE с URL '%s' уже зарегистрирован", ErrConflict, url)
		}
		return nil, fmt.Errorf("сохранение SE в БД: %w", err)
	}

	s.logger.Info("SE зарегистрирован",
		slog.String("se_id", seID),
		slog.String("name", name),
		slog.String("url", url),
		slog.String("storage_id", info.StorageID),
	)

	// Регистрация SE endpoint в dephealth (non-blocking, ошибки → Warn)
	if s.dephealthSvc != nil {
		if dhErr := s.dephealthSvc.RegisterSEEndpoint(name, url); dhErr != nil {
			s.logger.Warn("Не удалось зарегистрировать SE в dephealth",
				slog.String("se_id", seID),
				slog.String("name", name),
				slog.String("error", dhErr.Error()),
			)
		}
	}

	// Запуск полной синхронизации файлов в фоне (не блокируем Create)
	if s.syncSvc != nil {
		go func() {
			syncCtx := context.Background()
			result, syncErr := s.syncSvc.SyncOne(syncCtx, seID)
			if syncErr != nil {
				s.logger.Warn("Ошибка полной синхронизации при создании SE",
					slog.String("se_id", seID),
					slog.String("error", syncErr.Error()),
				)
			} else {
				s.logger.Info("Полная синхронизация при создании SE завершена",
					slog.String("se_id", seID),
					slog.Int("files_added", result.FilesAdded),
				)
			}
		}()
	}

	return se, nil
}

// List возвращает список SE с фильтрацией и пагинацией.
func (s *StorageElementService) List(ctx context.Context, mode, status *string, limit, offset int) ([]*model.StorageElement, int, error) {
	ses, err := s.seRepo.List(ctx, mode, status, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("получение списка SE: %w", err)
	}

	total, err := s.seRepo.Count(ctx, mode, status)
	if err != nil {
		return nil, 0, fmt.Errorf("подсчёт SE: %w", err)
	}

	return ses, total, nil
}

// Get возвращает SE по ID.
func (s *StorageElementService) Get(ctx context.Context, id string) (*model.StorageElement, error) {
	se, err := s.seRepo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("получение SE: %w", err)
	}
	return se, nil
}

// Update обновляет SE (name, url). Mode/status/capacity обновляются через sync.
func (s *StorageElementService) Update(ctx context.Context, id string, name, url *string) (*model.StorageElement, error) {
	// Получаем текущий SE
	se, err := s.seRepo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("получение SE для обновления: %w", err)
	}

	// Сохраняем старые значения для dephealth update
	oldName := se.Name
	oldURL := se.URL

	// Применяем обновления
	if name != nil {
		se.Name = *name
	}
	if url != nil {
		se.URL = *url
	}

	// Обновляем в БД
	if err := s.seRepo.Update(ctx, se); err != nil {
		if errors.Is(err, repository.ErrConflict) {
			return nil, fmt.Errorf("%w: URL или storage_id уже зарегистрирован", ErrConflict)
		}
		return nil, fmt.Errorf("обновление SE в БД: %w", err)
	}

	s.logger.Info("SE обновлён",
		slog.String("se_id", id),
	)

	// Обновление SE endpoint в dephealth (non-blocking, ошибки → Warn)
	if s.dephealthSvc != nil && (oldName != se.Name || oldURL != se.URL) {
		if dhErr := s.dephealthSvc.UpdateSEEndpoint(oldName, oldURL, se.Name, se.URL); dhErr != nil {
			s.logger.Warn("Не удалось обновить SE в dephealth",
				slog.String("se_id", id),
				slog.String("error", dhErr.Error()),
			)
		}
	}

	return se, nil
}

// Delete удаляет SE из реестра. Физические файлы не удаляются.
func (s *StorageElementService) Delete(ctx context.Context, id string) error {
	// Получаем SE для dephealth (name, URL) перед удалением
	var seName, seURL string
	if s.dephealthSvc != nil {
		if se, getErr := s.seRepo.GetByID(ctx, id); getErr == nil {
			seName = se.Name
			seURL = se.URL
		}
	}

	if err := s.seRepo.Delete(ctx, id); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return ErrNotFound
		}
		return fmt.Errorf("удаление SE: %w", err)
	}

	s.logger.Info("SE удалён из реестра",
		slog.String("se_id", id),
	)

	// Удаление SE endpoint из dephealth (non-blocking, ошибки → Warn)
	if s.dephealthSvc != nil && seName != "" && seURL != "" {
		if dhErr := s.dephealthSvc.UnregisterSEEndpoint(seName, seURL); dhErr != nil {
			s.logger.Warn("Не удалось удалить SE из dephealth",
				slog.String("se_id", id),
				slog.String("name", seName),
				slog.String("error", dhErr.Error()),
			)
		}
	}

	return nil
}

// Sync выполняет полную синхронизацию SE (info + файлы).
// Делегирует StorageSyncService.SyncOne для полной синхронизации.
func (s *StorageElementService) Sync(ctx context.Context, id string) (*model.SyncResult, error) {
	// Проверяем существование SE
	_, err := s.seRepo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("получение SE для sync: %w", err)
	}

	// Делегируем полную синхронизацию StorageSyncService
	if s.syncSvc == nil {
		return nil, fmt.Errorf("сервис синхронизации не инициализирован")
	}

	result, err := s.syncSvc.SyncOne(ctx, id)
	if err != nil {
		// Оборачиваем ошибку SE unavailable
		return nil, fmt.Errorf("синхронизация SE: %w", err)
	}

	return result, nil
}
