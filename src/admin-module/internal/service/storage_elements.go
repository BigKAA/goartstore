// storage_elements.go — сервис управления Storage Elements.
// CRUD SE: discover, регистрация, обновление, удаление.
// Sync (полная синхронизация) — заглушка для Phase 5.
package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/arturkryukov/artsore/admin-module/internal/domain/model"
	"github.com/arturkryukov/artsore/admin-module/internal/repository"
	"github.com/arturkryukov/artsore/admin-module/internal/seclient"
)

// StorageElementService — сервис управления Storage Elements.
type StorageElementService struct {
	seClient *seclient.Client
	seRepo   repository.StorageElementRepository
	fileRepo repository.FileRegistryRepository
	logger   *slog.Logger
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
		return nil, fmt.Errorf("%w: %v", ErrSEUnavailable, err)
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

// Create регистрирует новый SE: discover + сохранение в БД.
// Полная синхронизация файлов — Phase 5 (заглушка).
func (s *StorageElementService) Create(ctx context.Context, name, url string) (*model.StorageElement, error) {
	// Предпросмотр SE
	info, err := s.seClient.Info(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrSEUnavailable, err)
	}

	seID := uuid.New().String()
	now := time.Now().UTC()

	se := &model.StorageElement{
		ID:        seID,
		Name:      name,
		URL:       url,
		StorageID: info.StorageID,
		Mode:      info.Mode,
		Status:    info.Status,
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

	// TODO Phase 5: запустить полную синхронизацию файлов

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

	return se, nil
}

// Delete удаляет SE из реестра. Физические файлы не удаляются.
func (s *StorageElementService) Delete(ctx context.Context, id string) error {
	if err := s.seRepo.Delete(ctx, id); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return ErrNotFound
		}
		return fmt.Errorf("удаление SE: %w", err)
	}

	s.logger.Info("SE удалён из реестра",
		slog.String("se_id", id),
	)

	return nil
}

// Sync выполняет полную синхронизацию SE (info + файлы).
// В Phase 4 — заглушка, реализация в Phase 5.
func (s *StorageElementService) Sync(ctx context.Context, id string) (*model.SyncResult, error) {
	// Проверяем существование SE
	se, err := s.seRepo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("получение SE для sync: %w", err)
	}

	startedAt := time.Now().UTC()

	// Запрашиваем актуальную информацию о SE
	info, err := s.seClient.Info(ctx, se.URL)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrSEUnavailable, err)
	}

	// Обновляем данные SE из info
	se.Mode = info.Mode
	se.Status = info.Status
	if info.Capacity != nil {
		se.CapacityBytes = info.Capacity.TotalBytes
		se.UsedBytes = info.Capacity.UsedBytes
		avail := info.Capacity.AvailableBytes
		se.AvailableBytes = &avail
	}
	now := time.Now().UTC()
	se.LastSyncAt = &now

	// Сохраняем обновлённые данные SE
	if err := s.seRepo.Update(ctx, se); err != nil {
		return nil, fmt.Errorf("обновление SE после sync: %w", err)
	}

	completedAt := time.Now().UTC()

	// TODO Phase 5: полная синхронизация файлов (пагинированный ListFiles + BatchUpsert)
	result := &model.SyncResult{
		StorageElementID:   id,
		FilesOnSE:          0,
		FilesAdded:         0,
		FilesUpdated:       0,
		FilesMarkedDeleted: 0,
		StartedAt:          startedAt,
		CompletedAt:        completedAt,
	}

	s.logger.Info("SE sync выполнен",
		slog.String("se_id", id),
		slog.String("mode", info.Mode),
		slog.String("status", info.Status),
	)

	return result, nil
}
