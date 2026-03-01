// gc.go — сервис фоновой очистки (Garbage Collection) файлов.
//
// GC выполняет две задачи:
//  1. Помечает active файлы с истёкшим TTL как expired (обновляет attr.json + индекс)
//  2. Физически удаляет файлы со статусом deleted (файл + attr.json + запись в индексе)
//
// GC идемпотентен: каждый pod запускает свой GC без singleton-координации.
// Перед удалением проверяется lock-файл (per-file lock с TTL),
// чтобы не удалить файл во время upload-а.
//
// Запускается как горутина с периодическим тикером (SE_GC_INTERVAL).
package service

import (
	"context"
	"log/slog"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"github.com/bigkaa/goartstore/storage-element/internal/domain/model"
	"github.com/bigkaa/goartstore/storage-element/internal/lockfile"
	"github.com/bigkaa/goartstore/storage-element/internal/storage/attr"
	"github.com/bigkaa/goartstore/storage-element/internal/storage/filestore"
	"github.com/bigkaa/goartstore/storage-element/internal/storage/index"
)

// Prometheus метрики GC
var (
	// gcRunsTotal — количество запусков GC.
	gcRunsTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "se_gc_runs_total",
		Help: "Общее количество запусков GC",
	})

	// gcFilesDeletedTotal — количество физически удалённых файлов.
	gcFilesDeletedTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "se_gc_files_deleted_total",
		Help: "Общее количество файлов, удалённых GC",
	})

	// gcFilesExpiredTotal — количество файлов, помеченных как expired.
	gcFilesExpiredTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "se_gc_files_expired_total",
		Help: "Общее количество файлов, помеченных как expired",
	})

	// gcDurationSeconds — длительность выполнения GC.
	gcDurationSeconds = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "se_gc_duration_seconds",
		Help:    "Длительность выполнения GC в секундах",
		Buckets: []float64{0.01, 0.05, 0.1, 0.5, 1, 5, 10, 30, 60},
	})
)

// GCResult — результат одного запуска GC.
type GCResult struct {
	// ExpiredCount — количество файлов, помеченных как expired
	ExpiredCount int
	// DeletedCount — количество физически удалённых файлов
	DeletedCount int
	// SkippedLocked — количество пропущенных файлов (активный lock)
	SkippedLocked int
	// Errors — количество ошибок при обработке файлов
	Errors int
	// Duration — длительность выполнения
	Duration time.Duration
}

// GCService — сервис фоновой очистки файлов.
//
// Idempotent: каждый pod запускает свой GC без mutex-координации.
// os.Remove() идемпотентен: два GC могут удалять один и тот же файл одновременно.
type GCService struct {
	store       *filestore.FileStore
	idx         *index.Index
	lockManager *lockfile.LockManager
	interval    time.Duration
	logger      *slog.Logger

	cancel context.CancelFunc
}

// NewGCService создаёт сервис GC.
func NewGCService(
	store *filestore.FileStore,
	idx *index.Index,
	lockManager *lockfile.LockManager,
	interval time.Duration,
	logger *slog.Logger,
) *GCService {
	return &GCService{
		store:       store,
		idx:         idx,
		lockManager: lockManager,
		interval:    interval,
		logger:      logger.With(slog.String("component", "gc")),
	}
}

// Start запускает фоновую горутину GC с периодическим тикером.
// Вызывается один раз при старте приложения.
func (gc *GCService) Start(ctx context.Context) {
	gcCtx, cancel := context.WithCancel(ctx)
	gc.cancel = cancel

	go gc.run(gcCtx)

	gc.logger.Info("GC запущен",
		slog.String("interval", gc.interval.String()),
	)
}

// Stop останавливает фоновый процесс GC.
func (gc *GCService) Stop() {
	if gc.cancel != nil {
		gc.cancel()
	}
	gc.logger.Info("GC остановлен")
}

// run — основной цикл фоновой горутины.
func (gc *GCService) run(ctx context.Context) {
	// Первый запуск — сразу после старта
	gc.RunOnce()

	ticker := time.NewTicker(gc.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			gc.RunOnce()
		}
	}
}

// RunOnce выполняет один цикл GC.
//
// Идемпотентен: несколько pod-ов могут запускать GC одновременно.
// os.Remove() на уже удалённый файл не вызывает ошибку.
// Перед удалением проверяется lock-файл — locked файлы пропускаются.
//
// Порядок обработки:
//  1. Сканирование индекса: пометка expired (active + TTL истёк)
//  2. Физическое удаление deleted файлов (файл + attr.json + индекс)
func (gc *GCService) RunOnce() *GCResult {
	start := time.Now()
	result := &GCResult{}

	gc.logger.Debug("GC запуск начат")

	now := time.Now().UTC()

	// Фаза 1: пометка expired файлов
	expired := gc.markExpired(now)
	result.ExpiredCount = expired

	// Фаза 2: удаление deleted файлов (lock-aware)
	deleted, skipped, errors := gc.deleteFiles()
	result.DeletedCount = deleted
	result.SkippedLocked = skipped
	result.Errors = errors

	result.Duration = time.Since(start)

	// Обновляем Prometheus метрики
	gcRunsTotal.Inc()
	gcFilesDeletedTotal.Add(float64(deleted))
	gcFilesExpiredTotal.Add(float64(expired))
	gcDurationSeconds.Observe(result.Duration.Seconds())

	gc.logger.Info("GC завершён",
		slog.Int("expired", result.ExpiredCount),
		slog.Int("deleted", result.DeletedCount),
		slog.Int("skipped_locked", result.SkippedLocked),
		slog.Int("errors", result.Errors),
		slog.Duration("duration", result.Duration),
	)

	return result
}

// markExpired находит active файлы с истёкшим TTL и помечает их как expired.
// Обновляет и attr.json, и индекс.
func (gc *GCService) markExpired(now time.Time) int {
	// Получаем все active файлы из индекса
	files, _ := gc.idx.List(0, 0, model.StatusActive)

	count := 0
	for _, meta := range files {
		if !meta.IsExpired(now) {
			continue
		}

		// Обновляем статус на expired
		meta.Status = model.StatusExpired

		// Обновляем attr.json
		attrPath := attr.AttrFilePath(gc.store.FullPath(meta.StoragePath))
		if err := attr.Write(attrPath, meta); err != nil {
			gc.logger.Error("GC: ошибка обновления attr.json",
				slog.String("file_id", meta.FileID),
				slog.String("error", err.Error()),
			)
			continue
		}

		// Обновляем индекс
		if err := gc.idx.Update(meta); err != nil {
			gc.logger.Error("GC: ошибка обновления индекса",
				slog.String("file_id", meta.FileID),
				slog.String("error", err.Error()),
			)
			continue
		}

		gc.logger.Debug("GC: файл помечен как expired",
			slog.String("file_id", meta.FileID),
			slog.String("filename", meta.OriginalFilename),
		)
		count++
	}

	return count
}

// deleteFiles физически удаляет файлы со статусом deleted.
// Перед удалением проверяет lock-файл: если файл locked (TTL не истёк) — пропускает.
// Удаляет: файл данных, attr.json, запись в индексе.
func (gc *GCService) deleteFiles() (deleted, skipped, errors int) {
	// Получаем все deleted файлы из индекса
	files, _ := gc.idx.List(0, 0, model.StatusDeleted)

	for _, meta := range files {
		// Проверяем lock перед удалением: если файл ещё загружается — пропускаем
		locked, lockInfo, lockErr := gc.lockManager.IsLocked(meta.FileID)
		if lockErr != nil {
			gc.logger.Warn("GC: ошибка проверки lock, пропуск файла",
				slog.String("file_id", meta.FileID),
				slog.String("error", lockErr.Error()),
			)
			errors++
			continue
		}
		if locked {
			gc.logger.Debug("GC: файл locked, пропуск",
				slog.String("file_id", meta.FileID),
				slog.String("holder", lockInfo.Holder),
			)
			skipped++
			continue
		}

		// Удаляем файл данных
		if err := gc.store.DeleteFile(meta.StoragePath); err != nil {
			gc.logger.Error("GC: ошибка удаления файла",
				slog.String("file_id", meta.FileID),
				slog.String("storage_path", meta.StoragePath),
				slog.String("error", err.Error()),
			)
			errors++
			continue
		}

		// Удаляем attr.json
		attrPath := attr.AttrFilePath(gc.store.FullPath(meta.StoragePath))
		if err := attr.Delete(attrPath); err != nil {
			gc.logger.Error("GC: ошибка удаления attr.json",
				slog.String("file_id", meta.FileID),
				slog.String("error", err.Error()),
			)
			// Файл уже удалён, но attr.json нет — не критично, продолжаем
		}

		// Удаляем из индекса
		gc.idx.Remove(meta.FileID)

		gc.logger.Debug("GC: файл удалён",
			slog.String("file_id", meta.FileID),
			slog.String("filename", meta.OriginalFilename),
		)
		deleted++
	}

	return deleted, skipped, errors
}
