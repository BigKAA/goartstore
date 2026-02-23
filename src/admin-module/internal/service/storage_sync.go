// storage_sync.go — сервис периодической синхронизации файлового реестра с Storage Elements.
//
// StorageSyncService запускает фоновую горутину с ticker (AM_SYNC_INTERVAL),
// которая обходит все SE со статусом online и синхронизирует файловый реестр.
//
// SyncOne выполняет полную синхронизацию одного SE:
//  1. GET /api/v1/info → обновить mode/status/capacity в БД
//  2. Постраничный GET /api/v1/files → batch upsert в file_registry
//  3. Пометить отсутствующие файлы как deleted
//  4. Обновить last_sync_at, last_file_sync_at
//
// Prometheus-метрики:
//   - admin_module_sync_duration_seconds — длительность синхронизации
//   - admin_module_sync_files_total — количество обработанных файлов (по операциям)
package service

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"github.com/bigkaa/goartstore/admin-module/internal/domain/model"
	"github.com/bigkaa/goartstore/admin-module/internal/repository"
	"github.com/bigkaa/goartstore/admin-module/internal/seclient"
)

// Prometheus-метрики для синхронизации файлового реестра.
var (
	syncDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "admin_module_sync_duration_seconds",
		Help:    "Длительность синхронизации файлового реестра с SE",
		Buckets: prometheus.ExponentialBuckets(0.1, 2, 12), // 0.1s … ~204s
	}, []string{"se_id"})

	syncFilesTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "admin_module_sync_files_total",
		Help: "Количество обработанных файлов при синхронизации",
	}, []string{"se_id", "operation"}) // operation: added, updated, deleted
)

// StorageSyncService — фоновый сервис синхронизации файлового реестра.
type StorageSyncService struct {
	seClient      *seclient.Client
	seRepo        repository.StorageElementRepository
	fileRepo      repository.FileRegistryRepository
	syncStateRepo repository.SyncStateRepository
	pageSize      int
	interval      time.Duration
	logger        *slog.Logger

	cancel context.CancelFunc
	done   chan struct{}
}

// NewStorageSyncService создаёт сервис синхронизации файлового реестра.
func NewStorageSyncService(
	seClient *seclient.Client,
	seRepo repository.StorageElementRepository,
	fileRepo repository.FileRegistryRepository,
	syncStateRepo repository.SyncStateRepository,
	pageSize int,
	interval time.Duration,
	logger *slog.Logger,
) *StorageSyncService {
	return &StorageSyncService{
		seClient:      seClient,
		seRepo:        seRepo,
		fileRepo:      fileRepo,
		syncStateRepo: syncStateRepo,
		pageSize:      pageSize,
		interval:      interval,
		logger:        logger.With(slog.String("component", "storage_sync")),
	}
}

// Start запускает фоновую горутину с периодической синхронизацией.
// Вызывается один раз при старте приложения.
func (s *StorageSyncService) Start(ctx context.Context) {
	ctx, s.cancel = context.WithCancel(ctx)
	s.done = make(chan struct{})

	go func() {
		defer close(s.done)

		s.logger.Info("Периодическая синхронизация файлового реестра запущена",
			slog.String("interval", s.interval.String()),
			slog.Int("page_size", s.pageSize),
		)

		ticker := time.NewTicker(s.interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				s.logger.Info("Периодическая синхронизация файлового реестра остановлена")
				return
			case <-ticker.C:
				s.logger.Info("Запуск периодической синхронизации всех SE")
				results, err := s.SyncAll(ctx)
				if err != nil {
					s.logger.Error("Ошибка периодической синхронизации", slog.String("error", err.Error()))
				} else {
					s.logger.Info("Периодическая синхронизация завершена",
						slog.Int("se_count", len(results)),
					)
				}
			}
		}
	}()
}

// Stop останавливает фоновую горутину и ждёт завершения.
func (s *StorageSyncService) Stop() {
	if s.cancel != nil {
		s.cancel()
	}
	if s.done != nil {
		<-s.done
	}
}

// SyncAll синхронизирует все SE со статусом online.
// Каждый SE обрабатывается параллельно (до 5 одновременно).
func (s *StorageSyncService) SyncAll(ctx context.Context) ([]*model.SyncResult, error) {
	// Получаем все SE со статусом online (без лимита)
	onlineStatus := "online"
	ses, err := s.seRepo.List(ctx, nil, &onlineStatus, 1000, 0)
	if err != nil {
		return nil, fmt.Errorf("получение списка SE для sync: %w", err)
	}

	if len(ses) == 0 {
		s.logger.Info("Нет SE со статусом online для синхронизации")
		return nil, nil
	}

	s.logger.Info("Синхронизация файлового реестра",
		slog.Int("se_count", len(ses)),
	)

	// Параллельная синхронизация с ограничением concurrency
	const maxConcurrency = 5
	sem := make(chan struct{}, maxConcurrency)

	var mu sync.Mutex
	var results []*model.SyncResult
	var syncErrors []error

	var wg sync.WaitGroup
	for _, se := range ses {
		wg.Add(1)
		go func(seID string) {
			defer wg.Done()

			// Ограничение concurrency
			sem <- struct{}{}
			defer func() { <-sem }()

			result, err := s.SyncOne(ctx, seID)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				syncErrors = append(syncErrors, fmt.Errorf("SE %s: %w", seID, err))
			} else {
				results = append(results, result)
			}
		}(se.ID)
	}
	wg.Wait()

	// Обновляем глобальное время синхронизации файлов
	now := time.Now().UTC()
	if err := s.syncStateRepo.UpdateFileSyncAt(ctx, now); err != nil {
		s.logger.Warn("Ошибка обновления last_file_sync_at", slog.String("error", err.Error()))
	}

	// Логируем ошибки отдельных SE
	for _, syncErr := range syncErrors {
		s.logger.Warn("Ошибка синхронизации SE", slog.String("error", syncErr.Error()))
	}

	return results, nil
}

// SyncOne выполняет полную синхронизацию одного SE:
// 1. Запрос актуальной информации (mode, status, capacity)
// 2. Постраничная загрузка списка файлов
// 3. Batch upsert в файловый реестр
// 4. Пометка отсутствующих файлов как deleted
// 5. Обновление timestamps
func (s *StorageSyncService) SyncOne(ctx context.Context, seID string) (*model.SyncResult, error) {
	startedAt := time.Now().UTC()

	// 1. Получаем SE из БД
	se, err := s.seRepo.GetByID(ctx, seID)
	if err != nil {
		return nil, fmt.Errorf("получение SE: %w", err)
	}

	// 2. Запрашиваем актуальную информацию
	info, err := s.seClient.Info(ctx, se.URL)
	if err != nil {
		return nil, fmt.Errorf("запрос Info SE %s: %w", se.URL, err)
	}

	// 3. Обновляем данные SE
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

	s.logger.Info("SE info обновлён",
		slog.String("se_id", seID),
		slog.String("mode", info.Mode),
		slog.String("status", info.Status),
	)

	// 4. Постраничная загрузка файлов и batch upsert
	var totalFilesOnSE int
	var totalAdded, totalUpdated int
	var allFileIDs []string

	offset := 0
	for {
		// Проверяем отмену контекста
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		fileResp, err := s.seClient.ListFiles(ctx, se.URL, s.pageSize, offset)
		if err != nil {
			return nil, fmt.Errorf("запрос ListFiles SE %s (offset=%d): %w", se.URL, offset, err)
		}

		totalFilesOnSE = fileResp.Total

		if len(fileResp.Files) == 0 {
			break
		}

		// Конвертируем SEFileMetadata → FileRecord
		records := make([]*model.FileRecord, 0, len(fileResp.Files))
		for _, f := range fileResp.Files {
			record, err := seFileToRecord(f, seID)
			if err != nil {
				s.logger.Warn("Ошибка конвертации файла SE → FileRecord",
					slog.String("file_id", f.FileID),
					slog.String("error", err.Error()),
				)
				continue
			}
			records = append(records, record)
			allFileIDs = append(allFileIDs, f.FileID)
		}

		// Batch upsert
		added, updated, err := s.fileRepo.BatchUpsert(ctx, records)
		if err != nil {
			return nil, fmt.Errorf("batch upsert файлов SE %s (offset=%d): %w", seID, offset, err)
		}
		totalAdded += added
		totalUpdated += updated

		s.logger.Debug("Страница файлов обработана",
			slog.String("se_id", seID),
			slog.Int("offset", offset),
			slog.Int("count", len(fileResp.Files)),
			slog.Int("added", added),
			slog.Int("updated", updated),
		)

		offset += len(fileResp.Files)

		// Если получили меньше файлов, чем pageSize — достигли конца
		if len(fileResp.Files) < s.pageSize {
			break
		}
	}

	// 5. Пометить отсутствующие файлы как deleted
	markedDeleted, err := s.fileRepo.MarkDeletedExcept(ctx, seID, allFileIDs)
	if err != nil {
		return nil, fmt.Errorf("пометка удалённых файлов SE %s: %w", seID, err)
	}

	// 6. Обновляем timestamps SE
	fileSyncAt := time.Now().UTC()
	se.LastFileSyncAt = &fileSyncAt
	if err := s.seRepo.Update(ctx, se); err != nil {
		return nil, fmt.Errorf("обновление SE после sync: %w", err)
	}

	completedAt := time.Now().UTC()

	// 7. Обновляем Prometheus-метрики
	duration := completedAt.Sub(startedAt).Seconds()
	syncDuration.WithLabelValues(seID).Observe(duration)
	syncFilesTotal.WithLabelValues(seID, "added").Add(float64(totalAdded))
	syncFilesTotal.WithLabelValues(seID, "updated").Add(float64(totalUpdated))
	syncFilesTotal.WithLabelValues(seID, "deleted").Add(float64(markedDeleted))

	result := &model.SyncResult{
		StorageElementID:   seID,
		FilesOnSE:          totalFilesOnSE,
		FilesAdded:         totalAdded,
		FilesUpdated:       totalUpdated,
		FilesMarkedDeleted: markedDeleted,
		StartedAt:          startedAt,
		CompletedAt:        completedAt,
	}

	s.logger.Info("Синхронизация SE завершена",
		slog.String("se_id", seID),
		slog.Int("files_on_se", totalFilesOnSE),
		slog.Int("files_added", totalAdded),
		slog.Int("files_updated", totalUpdated),
		slog.Int("files_deleted", markedDeleted),
		slog.String("duration", fmt.Sprintf("%.2fs", duration)),
	)

	return result, nil
}

// seFileToRecord конвертирует метаданные файла SE в FileRecord для реестра.
func seFileToRecord(f seclient.SEFileMetadata, seID string) (*model.FileRecord, error) {
	// Парсинг UploadedAt (ISO 8601 / RFC 3339)
	uploadedAt, err := time.Parse(time.RFC3339, f.UploadedAt)
	if err != nil {
		return nil, fmt.Errorf("парсинг uploaded_at %q: %w", f.UploadedAt, err)
	}

	record := &model.FileRecord{
		FileID:           f.FileID,
		OriginalFilename: f.OriginalFilename,
		ContentType:      f.ContentType,
		Size:             f.Size,
		Checksum:         f.Checksum,
		StorageElementID: seID,
		UploadedBy:       f.UploadedBy,
		UploadedAt:       uploadedAt,
		Description:      f.Description,
		Tags:             f.Tags,
		Status:           f.Status,
		RetentionPolicy:  f.RetentionPolicy,
		TTLDays:          f.TTLDays,
	}

	// Парсинг ExpiresAt (опциональное поле)
	if f.ExpiresAt != nil {
		expiresAt, err := time.Parse(time.RFC3339, *f.ExpiresAt)
		if err != nil {
			return nil, fmt.Errorf("парсинг expires_at %q: %w", *f.ExpiresAt, err)
		}
		record.ExpiresAt = &expiresAt
	}

	return record, nil
}
