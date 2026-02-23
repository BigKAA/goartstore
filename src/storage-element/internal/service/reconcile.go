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
// Запускается как горутина с периодическим тикером (SE_RECONCILE_INTERVAL).
package service

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"github.com/bigkaa/goartstore/storage-element/internal/api/generated"
	"github.com/bigkaa/goartstore/storage-element/internal/storage/attr"
	"github.com/bigkaa/goartstore/storage-element/internal/storage/filestore"
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
type ReconcileService struct {
	store    *filestore.FileStore
	idx      *index.Index
	dataDir  string
	interval time.Duration
	logger   *slog.Logger

	mu        sync.Mutex // защита от параллельного запуска
	inProcess bool       // reconciliation в процессе выполнения
	cancel    context.CancelFunc
}

// NewReconcileService создаёт сервис reconciliation.
func NewReconcileService(
	store *filestore.FileStore,
	idx *index.Index,
	dataDir string,
	interval time.Duration,
	logger *slog.Logger,
) *ReconcileService {
	return &ReconcileService{
		store:    store,
		idx:      idx,
		dataDir:  dataDir,
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

// IsInProgress возвращает true, если reconciliation выполняется.
func (rs *ReconcileService) IsInProgress() bool {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	return rs.inProcess
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
// Потокобезопасен: если reconciliation уже выполняется, возвращает nil, true.
//
// Возвращает:
//   - *generated.ReconcileResponse — результат сверки
//   - bool — true если reconciliation уже выполнялась (skipped)
func (rs *ReconcileService) RunOnce() (*generated.ReconcileResponse, bool) {
	rs.mu.Lock()
	if rs.inProcess {
		rs.mu.Unlock()
		rs.logger.Warn("Reconciliation уже выполняется, пропуск")
		return nil, true
	}
	rs.inProcess = true
	rs.mu.Unlock()

	defer func() {
		rs.mu.Lock()
		rs.inProcess = false
		rs.mu.Unlock()
	}()

	startedAt := time.Now().UTC()
	rs.logger.Info("Reconciliation начата")

	issues := rs.reconcile()

	// Пересобираем индекс из attr.json
	if err := rs.idx.RebuildFromDir(rs.dataDir); err != nil {
		rs.logger.Error("Ошибка пересборки индекса",
			slog.String("error", err.Error()),
		)
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
	}, false
}

// reconcile выполняет сверку данных на диске.
func (rs *ReconcileService) reconcile() []generated.ReconcileIssue {
	var issues []generated.ReconcileIssue

	// Собираем все файлы на диске (не attr.json)
	dataFiles := make(map[string]bool)
	// Собираем все attr.json на диске
	attrFiles := make(map[string]bool)

	entries, err := os.ReadDir(rs.dataDir)
	if err != nil {
		rs.logger.Error("Ошибка чтения директории данных",
			slog.String("error", err.Error()),
		)
		return issues
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()

		// Пропускаем служебные файлы
		if strings.HasPrefix(name, ".") {
			continue
		}
		// Пропускаем temp файлы
		if strings.HasSuffix(name, ".tmp") {
			continue
		}

		if attr.IsAttrFile(name) {
			attrFiles[name] = true
		} else {
			dataFiles[name] = true
		}
	}

	// 1. Проверяем: файл данных без attr.json (orphaned_file)
	for dataFile := range dataFiles {
		expectedAttr := dataFile + attr.AttrSuffix
		if !attrFiles[expectedAttr] {
			path := dataFile
			issues = append(issues, generated.ReconcileIssue{
				Type:        generated.OrphanedFile,
				Path:        &path,
				Description: "Файл на диске без attr.json",
			})
		}
	}

	// 2. Проверяем: attr.json без файла данных (orphaned_attr / missing_file)
	for attrFile := range attrFiles {
		dataFile := strings.TrimSuffix(attrFile, attr.AttrSuffix)
		if !dataFiles[dataFile] {
			// Читаем attr.json для получения file_id
			attrPath := filepath.Join(rs.dataDir, attrFile)
			meta, readErr := attr.Read(attrPath)
			path := dataFile

			issue := generated.ReconcileIssue{
				Type:        generated.MissingFile,
				Path:        &path,
				Description: "attr.json без соответствующего файла на диске",
			}

			if readErr == nil {
				if parsedUUID, parseErr := uuid.Parse(meta.FileID); parseErr == nil {
					issue.FileId = &parsedUUID
				}
			}

			issues = append(issues, issue)
		}
	}

	// 3. Проверяем целостность файлов: size и checksum
	for attrFile := range attrFiles {
		dataFile := strings.TrimSuffix(attrFile, attr.AttrSuffix)
		if !dataFiles[dataFile] {
			// Файл отсутствует — уже обработан выше
			continue
		}

		// Читаем метаданные из attr.json
		attrPath := filepath.Join(rs.dataDir, attrFile)
		meta, readErr := attr.Read(attrPath)
		if readErr != nil {
			rs.logger.Warn("Ошибка чтения attr.json при reconciliation",
				slog.String("attr_file", attrFile),
				slog.String("error", readErr.Error()),
			)
			continue
		}

		parsedUUID, _ := uuid.Parse(meta.FileID)

		// Проверяем размер
		actualSize, sizeErr := rs.store.FileSize(dataFile)
		if sizeErr != nil {
			rs.logger.Warn("Ошибка получения размера файла",
				slog.String("file", dataFile),
				slog.String("error", sizeErr.Error()),
			)
			continue
		}

		if actualSize != meta.Size {
			path := dataFile
			issues = append(issues, generated.ReconcileIssue{
				Type:        generated.SizeMismatch,
				FileId:      &parsedUUID,
				Path:        &path,
				Description: "Размер файла на диске не совпадает с attr.json",
			})
			continue // Если размер не совпадает, checksum точно не совпадёт
		}

		// Проверяем checksum
		actualChecksum, csErr := rs.store.ComputeChecksum(dataFile)
		if csErr != nil {
			rs.logger.Warn("Ошибка вычисления checksum",
				slog.String("file", dataFile),
				slog.String("error", csErr.Error()),
			)
			continue
		}

		if actualChecksum != meta.Checksum {
			path := dataFile
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
