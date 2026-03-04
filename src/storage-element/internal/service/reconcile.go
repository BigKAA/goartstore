// reconcile.go — сервис фоновой сверки (Reconciliation) файлового хранилища.
//
// Reconciliation сравнивает:
//   - Файлы на диске с записями в attr.json
//   - attr.json с физическими файлами
//   - Контрольные суммы и размеры файлов
//
// Обнаруживает проблемы:
//   - orphaned_file: файл на диске без attr.json
//   - orphaned_attr: attr.json без файла на диске
//   - missing_file: запись в индексе, но файла нет
//   - checksum_mismatch: не совпадает checksum
//   - size_mismatch: не совпадает размер
//
// Idempotent: каждый pod запускает свой reconcile без singleton-координации.
// При обнаружении orphaned_file/orphaned_attr проверяется lock-файл —
// если файл загружается (lock активен), проблема пропускается.
//
// Запускается как горутина с периодическим тикером (SE_RECONCILE_INTERVAL).
package service

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"github.com/bigkaa/goartstore/storage-element/internal/api/generated"
	"github.com/bigkaa/goartstore/storage-element/internal/backend"
	"github.com/bigkaa/goartstore/storage-element/internal/storage/index"
)

// Prometheus метрики Reconciliation
var (
	// reconcileRunsTotal — количество запусков reconciliation.
	reconcileRunsTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "se_reconcile_runs_total",
		Help: "Общее количество запусков reconciliation",
	})

	// reconcileIssuesTotal — количество обнаруженных проблем по типу.
	reconcileIssuesTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "se_reconcile_issues_total",
		Help: "Общее количество проблем, обнаруженных reconciliation",
	}, []string{"type"})

	// reconcileDurationSeconds — длительность выполнения reconciliation.
	reconcileDurationSeconds = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "se_reconcile_duration_seconds",
		Help:    "Длительность выполнения reconciliation в секундах",
		Buckets: []float64{0.1, 0.5, 1, 5, 10, 30, 60, 120, 300},
	})
)

// ReconcileService — сервис фоновой сверки хранилища.
//
// Idempotent: каждый pod запускает reconcile без mutex-координации.
type ReconcileService struct {
	files    backend.FileStore
	attrs    backend.AttrStore
	idx      *index.Index
	locks    backend.LockStore
	interval time.Duration
	logger   *slog.Logger

	cancel context.CancelFunc
}

// NewReconcileService создаёт сервис reconciliation.
func NewReconcileService(
	files backend.FileStore,
	attrs backend.AttrStore,
	idx *index.Index,
	locks backend.LockStore,
	interval time.Duration,
	logger *slog.Logger,
) *ReconcileService {
	return &ReconcileService{
		files:    files,
		attrs:    attrs,
		idx:      idx,
		locks:    locks,
		interval: interval,
		logger:   logger.With(slog.String("component", "reconcile")),
	}
}

// Start запускает фоновую горутину reconciliation с периодическим тикером.
func (rs *ReconcileService) Start(ctx context.Context) {
	rsCtx, cancel := context.WithCancel(ctx)
	rs.cancel = cancel

	go rs.run(rsCtx)

	rs.logger.Info("Reconciliation запущена",
		slog.String("interval", rs.interval.String()),
	)
}

// Stop останавливает фоновой процесс reconciliation.
func (rs *ReconcileService) Stop() {
	if rs.cancel != nil {
		rs.cancel()
	}
	rs.logger.Info("Reconciliation остановлена")
}

// run — основной цикл фоновой горутины.
func (rs *ReconcileService) run(ctx context.Context) {
	ticker := time.NewTicker(rs.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			rs.RunOnce()
		}
	}
}

// RunOnce выполняет один цикл reconciliation.
//
// Idempotent: несколько pod-ов могут запускать reconcile одновременно.
// Lock-aware: orphaned файлы с активным lock пропускаются (upload в процессе).
//
// Возвращает:
//   - *generated.ReconcileResponse — результат сверки
func (rs *ReconcileService) RunOnce() *generated.ReconcileResponse {
	ctx := context.Background()
	startedAt := time.Now().UTC()
	rs.logger.Info("Reconciliation начата")

	issues := rs.reconcile(ctx)

	// Пересобираем индекс из attr.json через AttrStore
	metadatas, err := rs.attrs.ScanAll(ctx)
	if err != nil {
		rs.logger.Error("Ошибка сканирования attr.json для пересборки индекса",
			slog.String("error", err.Error()),
		)
	} else {
		rs.idx.RebuildFromMetadata(metadatas)
	}

	completedAt := time.Now().UTC()
	duration := completedAt.Sub(startedAt)

	// Подсчитываем summary
	summary := generated.ReconcileSummary{}
	for _, issue := range issues {
		switch issue.Type {
		case generated.OrphanedFile:
			summary.OrphanedFiles++
		case generated.OrphanedAttr:
			// orphaned_attr считаем как missing_files
			summary.MissingFiles++
		case generated.MissingFile:
			summary.MissingFiles++
		case generated.ChecksumMismatch:
			summary.ChecksumMismatches++
		case generated.SizeMismatch:
			summary.SizeMismatches++
		}
	}

	// Общее количество проверенных файлов
	filesChecked := rs.idx.Count()
	summary.Ok = filesChecked - len(issues)
	if summary.Ok < 0 {
		summary.Ok = 0
	}

	// Обновляем Prometheus метрики
	reconcileRunsTotal.Inc()
	reconcileDurationSeconds.Observe(duration.Seconds())
	for _, issue := range issues {
		reconcileIssuesTotal.WithLabelValues(string(issue.Type)).Inc()
	}

	rs.logger.Info("Reconciliation завершена",
		slog.Int("files_checked", filesChecked),
		slog.Int("issues", len(issues)),
		slog.Int("ok", summary.Ok),
		slog.Duration("duration", duration),
	)

	return &generated.ReconcileResponse{
		StartedAt:    startedAt,
		CompletedAt:  completedAt,
		FilesChecked: filesChecked,
		Issues:       issues,
		Summary:      summary,
	}
}

// reconcile выполняет сверку данных через FileStore и AttrStore.
// Использует FileStore.ListDataPaths() и AttrStore.ScanAll() для cross-reference.
//
//nolint:gocognit // reconcile — комплексная сверка с несколькими фазами
func (rs *ReconcileService) reconcile(ctx context.Context) []generated.ReconcileIssue {
	var issues []generated.ReconcileIssue

	// Получаем список data-файлов через FileStore
	dataPaths, err := rs.files.ListDataPaths(ctx)
	if err != nil {
		rs.logger.Error("Ошибка получения списка data-файлов",
			slog.String("error", err.Error()),
		)
		return issues
	}

	// Получаем все метаданные через AttrStore
	allMetas, err := rs.attrs.ScanAll(ctx)
	if err != nil {
		rs.logger.Error("Ошибка сканирования attr.json",
			slog.String("error", err.Error()),
		)
		return issues
	}

	// Строим maps для cross-reference
	dataFilesSet := make(map[string]bool, len(dataPaths))
	for _, p := range dataPaths {
		dataFilesSet[p] = true
	}

	// storagePath → metadata
	attrByPath := make(map[string]bool, len(allMetas))
	metaByPath := make(map[string]*struct {
		fileID   string
		size     int64
		checksum string
	}, len(allMetas))

	for _, m := range allMetas {
		attrByPath[m.StoragePath] = true
		metaByPath[m.StoragePath] = &struct {
			fileID   string
			size     int64
			checksum string
		}{
			fileID:   m.FileID,
			size:     m.Size,
			checksum: m.Checksum,
		}
	}

	// Определяем порог «свежести» файла: если файл создан в пределах lock TTL,
	// он может быть in-flight upload-ом (данные записаны, attr.json ещё нет).
	lockTTL := rs.locks.TTL()
	now := time.Now()

	// 1. Проверяем: data-файл без attr.json (orphaned_file)
	// Lock-aware: если файл свежий (mtime в пределах lockTTL) — пропускаем,
	// т.к. upload мог записать данные, но ещё не записал attr.json.
	for _, dataFile := range dataPaths {
		if attrByPath[dataFile] {
			continue
		}

		// Проверяем mtime файла — если свежий, upload может быть в процессе
		fullPath := rs.files.FullPath(dataFile)
		if info, statErr := os.Stat(fullPath); statErr == nil {
			if now.Sub(info.ModTime()) < lockTTL {
				rs.logger.Debug("Reconcile: orphaned файл свежий, пропуск (возможный in-flight upload)",
					slog.String("file", dataFile),
				)
				continue
			}
		}

		path := dataFile
		issues = append(issues, generated.ReconcileIssue{
			Type:        generated.OrphanedFile,
			Path:        &path,
			Description: "Файл на диске без attr.json",
		})
	}

	// 2. Проверяем: attr.json без data-файла (orphaned_attr / missing_file)
	for _, m := range allMetas {
		if dataFilesSet[m.StoragePath] {
			continue
		}

		path := m.StoragePath
		issue := generated.ReconcileIssue{
			Type:        generated.MissingFile,
			Path:        &path,
			Description: "attr.json без соответствующего файла на диске",
		}

		if parsedUUID, parseErr := uuid.Parse(m.FileID); parseErr == nil {
			issue.FileId = &parsedUUID
		}

		issues = append(issues, issue)
	}

	// 3. Проверяем целостность: size и checksum
	for _, dataFile := range dataPaths {
		info, ok := metaByPath[dataFile]
		if !ok {
			continue // Уже обработан как orphaned
		}

		// Проверяем размер
		actualSize, sizeErr := rs.files.FileSize(ctx, dataFile)
		if sizeErr != nil {
			rs.logger.Warn("Ошибка получения размера файла",
				slog.String("file", dataFile),
				slog.String("error", sizeErr.Error()),
			)
			continue
		}

		if actualSize != info.size {
			path := dataFile
			parsedUUID, _ := uuid.Parse(info.fileID)
			issues = append(issues, generated.ReconcileIssue{
				Type:        generated.SizeMismatch,
				FileId:      &parsedUUID,
				Path:        &path,
				Description: "Размер файла на диске не совпадает с attr.json",
			})
			continue // Если размер не совпадает, checksum точно не совпадёт
		}

		// Проверяем checksum
		actualChecksum, csErr := rs.files.ComputeChecksum(ctx, dataFile)
		if csErr != nil {
			rs.logger.Warn("Ошибка вычисления checksum",
				slog.String("file", dataFile),
				slog.String("error", csErr.Error()),
			)
			continue
		}

		if actualChecksum != info.checksum {
			path := dataFile
			parsedUUID, _ := uuid.Parse(info.fileID)
			issues = append(issues, generated.ReconcileIssue{
				Type:        generated.ChecksumMismatch,
				FileId:      &parsedUUID,
				Path:        &path,
				Description: "Checksum файла на диске не совпадает с attr.json",
			})
		}
	}

	return issues
}

