// gc.go — сервис фоновой очистки (Garbage Collection) файлов.
//
// GC удаляет temporary файлы с истёкшим TTL:
// сканирует индекс, находит temporary файлы с expires_at < now,
// физически удаляет файл + attr.json + запись в индексе.
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

	"github.com/bigkaa/goartstore/storage-element/internal/backend"
	"github.com/bigkaa/goartstore/storage-element/internal/storage/index"
)

// Prometheus метрики GC
var (
	// gcRunsTotal — количество запусков GC.
	gcRunsTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "se_gc_runs_total",
		Help: "Общее количество запусков GC",
	})

	// gcFilesRemovedTotal — количество физически удалённых файлов.
	gcFilesRemovedTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "se_gc_files_removed_total",
		Help: "Общее количество файлов, удалённых GC (по истечении TTL)",
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
	// DeletedCount — количество физически удалённых файлов (TTL истёк)
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
	files    backend.FileStore
	attrs    backend.AttrStore
	idx      *index.Index
	locks    backend.LockStore
	interval time.Duration
	logger   *slog.Logger

	cancel context.CancelFunc
}

// NewGCService создаёт сервис GC.
func NewGCService(
	files backend.FileStore,
	attrs backend.AttrStore,
	idx *index.Index,
	locks backend.LockStore,
	interval time.Duration,
	logger *slog.Logger,
) *GCService {
	return &GCService{
		files:    files,
		attrs:    attrs,
		idx:      idx,
		locks:    locks,
		interval: interval,
		logger:   logger.With(slog.String("component", "gc")),
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
// Находит temporary файлы с истёкшим TTL и физически удаляет их.
func (gc *GCService) RunOnce() *GCResult {
	start := time.Now()
	result := &GCResult{}

	gc.logger.Debug("GC запуск начат")

	now := time.Now().UTC()

	// Находим и удаляем файлы с истёкшим TTL
	deleted, skipped, errors := gc.deleteExpiredFiles(now)
	result.DeletedCount = deleted
	result.SkippedLocked = skipped
	result.Errors = errors

	result.Duration = time.Since(start)

	// Обновляем Prometheus метрики
	gcRunsTotal.Inc()
	gcFilesRemovedTotal.Add(float64(deleted))
	gcDurationSeconds.Observe(result.Duration.Seconds())

	gc.logger.Info("GC завершён",
		slog.Int("deleted", result.DeletedCount),
		slog.Int("skipped_locked", result.SkippedLocked),
		slog.Int("errors", result.Errors),
		slog.Duration("duration", result.Duration),
	)

	return result
}

// deleteExpiredFiles находит temporary файлы с истёкшим TTL и физически удаляет их.
// Удаляет: файл данных, attr.json, запись в индексе.
// Перед удалением проверяет lock-файл: если файл locked (TTL не истёк) — пропускает.
func (gc *GCService) deleteExpiredFiles(now time.Time) (deleted, skipped, errors int) {
	ctx := context.Background()

	// Получаем все temporary файлы из индекса
	files := gc.idx.ListTemporary()

	for _, meta := range files {
		// Проверяем, истёк ли TTL
		if !meta.IsExpired(now) {
			continue
		}

		// Проверяем lock перед удалением: если файл ещё загружается — пропускаем
		locked, lockInfo, lockErr := gc.locks.IsLocked(ctx, meta.FileID)
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
		if err := gc.files.DeleteFile(ctx, meta.StoragePath); err != nil {
			gc.logger.Error("GC: ошибка удаления файла",
				slog.String("file_id", meta.FileID),
				slog.String("storage_path", meta.StoragePath),
				slog.String("error", err.Error()),
			)
			errors++
			continue
		}

		// Удаляем attr.json через AttrStore
		if err := gc.attrs.Delete(ctx, meta.StoragePath); err != nil {
			gc.logger.Error("GC: ошибка удаления attr.json",
				slog.String("file_id", meta.FileID),
				slog.String("error", err.Error()),
			)
			// Файл уже удалён, но attr.json нет — не критично, продолжаем
		}

		// Удаляем из индекса
		gc.idx.Remove(meta.FileID)

		gc.logger.Debug("GC: файл удалён (TTL истёк)",
			slog.String("file_id", meta.FileID),
			slog.String("filename", meta.OriginalFilename),
		)
		deleted++
	}

	return deleted, skipped, errors
}
