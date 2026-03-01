// indexsync.go — сервис периодической пересборки in-memory индекса.
//
// В stateless архитектуре несколько pod-ов могут записывать файлы
// на общую файловую систему (NFS). Каждый pod поддерживает свой
// in-memory индекс, который обновляется синхронно при собственных
// операциях. Файлы, записанные другими pod-ами, обнаруживаются
// через периодическую полную пересборку индекса из attr.json.
//
// IndexSyncService вызывает index.RebuildFromDir() каждые
// SE_INDEX_SYNC_INTERVAL (default 30s).
//
// Eventual consistency: собственные операции видны сразу,
// чужие — после rebuild (до IndexSyncInterval).
//
// Запускается как горутина с периодическим тикером.
package service

import (
	"context"
	"log/slog"
	"time"

	"github.com/bigkaa/goartstore/storage-element/internal/storage/index"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Prometheus метрики IndexSyncService
var (
	// indexSyncRunsTotal — количество запусков пересборки индекса.
	indexSyncRunsTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "se_index_sync_runs_total",
		Help: "Общее количество запусков пересборки индекса",
	})

	// indexSyncErrorsTotal — количество ошибок при пересборке индекса.
	indexSyncErrorsTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "se_index_sync_errors_total",
		Help: "Общее количество ошибок при пересборке индекса",
	})

	// indexSyncDurationSeconds — длительность пересборки индекса.
	indexSyncDurationSeconds = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "se_index_sync_duration_seconds",
		Help:    "Длительность пересборки индекса в секундах",
		Buckets: []float64{0.01, 0.05, 0.1, 0.5, 1, 5, 10, 30},
	})

	// indexSyncFilesTotal — количество файлов в индексе после последней пересборки.
	indexSyncFilesTotal = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "se_index_sync_files_total",
		Help: "Количество файлов в индексе после последней пересборки",
	})
)

// IndexSyncService — сервис периодической пересборки in-memory индекса.
//
// Вызывает index.RebuildFromDir() для обнаружения файлов,
// записанных другими pod-ами на общую файловую систему.
//
// Файлы с активным lock-ом могут не иметь attr.json (upload в процессе),
// поэтому не попадают в индекс — это ожидаемое поведение.
type IndexSyncService struct {
	idx      *index.Index   // in-memory индекс для пересборки
	dataDir  string         // базовая директория с файлами и attr.json
	interval time.Duration  // интервал пересборки
	logger   *slog.Logger

	cancel context.CancelFunc
}

// NewIndexSyncService создаёт сервис периодической пересборки индекса.
//
// Параметры:
//   - idx: in-memory индекс
//   - dataDir: базовая директория данных SE
//   - interval: интервал пересборки (SE_INDEX_SYNC_INTERVAL)
//   - logger: логгер
func NewIndexSyncService(
	idx *index.Index,
	dataDir string,
	interval time.Duration,
	logger *slog.Logger,
) *IndexSyncService {
	return &IndexSyncService{
		idx:      idx,
		dataDir:  dataDir,
		interval: interval,
		logger:   logger.With(slog.String("component", "indexsync")),
	}
}

// Start запускает фоновую горутину пересборки индекса.
// Вызывается один раз при старте приложения.
func (iss *IndexSyncService) Start(ctx context.Context) {
	isCtx, cancel := context.WithCancel(ctx)
	iss.cancel = cancel

	go iss.run(isCtx)

	iss.logger.Info("Периодическая пересборка индекса запущена",
		slog.String("interval", iss.interval.String()),
		slog.String("data_dir", iss.dataDir),
	)
}

// Stop останавливает фоновый процесс пересборки.
func (iss *IndexSyncService) Stop() {
	if iss.cancel != nil {
		iss.cancel()
	}
	iss.logger.Info("Периодическая пересборка индекса остановлена")
}

// run — основной цикл фоновой горутины.
// Первая пересборка НЕ выполняется сразу — индекс уже построен при старте
// через index.BuildFromDir() в main.go.
func (iss *IndexSyncService) run(ctx context.Context) {
	ticker := time.NewTicker(iss.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			iss.SyncOnce()
		}
	}
}

// SyncOnce выполняет одну полную пересборку индекса из attr.json файлов.
//
// Файлы с активным lock-ом (upload в процессе) могут не иметь attr.json —
// они не попадут в индекс до завершения upload-а и следующего rebuild.
// Это ожидаемое поведение eventual consistency.
func (iss *IndexSyncService) SyncOnce() {
	start := time.Now()
	indexSyncRunsTotal.Inc()

	// Запоминаем количество файлов до пересборки
	countBefore := iss.idx.Count()

	// Полная пересборка из attr.json
	if err := iss.idx.RebuildFromDir(iss.dataDir); err != nil {
		iss.logger.Error("Ошибка пересборки индекса",
			slog.String("error", err.Error()),
		)
		indexSyncErrorsTotal.Inc()
		return
	}

	duration := time.Since(start)
	countAfter := iss.idx.Count()
	diff := countAfter - countBefore

	// Обновляем Prometheus метрики
	indexSyncDurationSeconds.Observe(duration.Seconds())
	indexSyncFilesTotal.Set(float64(countAfter))

	// Логируем результат: Info если обнаружены изменения, Debug если без изменений
	if diff != 0 {
		iss.logger.Info("Индекс пересобран, обнаружены изменения",
			slog.Int("files_before", countBefore),
			slog.Int("files_after", countAfter),
			slog.Int("diff", diff),
			slog.Duration("duration", duration),
		)
	} else {
		iss.logger.Debug("Индекс пересобран, изменений нет",
			slog.Int("files", countAfter),
			slog.Duration("duration", duration),
		)
	}
}
