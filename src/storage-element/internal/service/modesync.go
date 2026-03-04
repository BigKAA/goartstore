// modesync.go — сервис периодической синхронизации mode.json между pod-ами.
//
// В stateless архитектуре все pod-ы равноправны.
// При смене режима через API один pod записывает mode.json,
// остальные подхватывают изменение через периодическое чтение.
//
// ModeSyncService читает mode.json каждые SE_MODE_SYNC_INTERVAL (default 10s)
// и вызывает sm.ForceMode() при обнаружении расхождения.
//
// Запускается как горутина с периодическим тикером.
package service

import (
	"context"
	"log/slog"
	"time"

	"github.com/bigkaa/goartstore/storage-element/internal/domain/mode"
	"github.com/bigkaa/goartstore/storage-element/internal/modefile"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Prometheus метрики ModeSyncService
var (
	// modeSyncRunsTotal — количество запусков синхронизации mode.json.
	modeSyncRunsTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "se_mode_sync_runs_total",
		Help: "Общее количество запусков синхронизации mode.json",
	})

	// modeSyncChangesTotal — количество обнаруженных изменений режима.
	modeSyncChangesTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "se_mode_sync_changes_total",
		Help: "Общее количество обнаруженных изменений режима через mode.json",
	})

	// modeSyncErrorsTotal — количество ошибок при чтении mode.json.
	modeSyncErrorsTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "se_mode_sync_errors_total",
		Help: "Общее количество ошибок при чтении mode.json",
	})
)

// ModeSyncService — сервис периодической синхронизации режима из mode.json.
//
// Все pod-ы периодически читают mode.json с общей файловой системы.
// При обнаружении расхождения текущего режима с mode.json —
// вызывается sm.ForceMode() для принудительной смены.
type ModeSyncService struct {
	modeFilePath string               // полный путь к mode.json
	sm           *mode.StateMachine   // конечный автомат режимов
	interval     time.Duration        // интервал синхронизации
	logger       *slog.Logger

	cancel context.CancelFunc
}

// NewModeSyncService создаёт сервис синхронизации mode.json.
//
// Параметры:
//   - modeFilePath: полный путь к mode.json (обычно {dataDir}/mode.json)
//   - sm: конечный автомат режимов для ForceMode()
//   - interval: интервал проверки (SE_MODE_SYNC_INTERVAL)
//   - logger: логгер
func NewModeSyncService(
	modeFilePath string,
	sm *mode.StateMachine,
	interval time.Duration,
	logger *slog.Logger,
) *ModeSyncService {
	return &ModeSyncService{
		modeFilePath: modeFilePath,
		sm:           sm,
		interval:     interval,
		logger:       logger.With(slog.String("component", "modesync")),
	}
}

// Start запускает фоновую горутину синхронизации mode.json.
// Вызывается один раз при старте приложения.
func (mss *ModeSyncService) Start(ctx context.Context) {
	msCtx, cancel := context.WithCancel(ctx)
	mss.cancel = cancel

	go mss.run(msCtx)

	mss.logger.Info("Синхронизация mode.json запущена",
		slog.String("interval", mss.interval.String()),
		slog.String("path", mss.modeFilePath),
	)
}

// Stop останавливает фоновый процесс синхронизации.
func (mss *ModeSyncService) Stop() {
	if mss.cancel != nil {
		mss.cancel()
	}
	mss.logger.Info("Синхронизация mode.json остановлена")
}

// run — основной цикл фоновой горутины.
func (mss *ModeSyncService) run(ctx context.Context) {
	// Первый запуск — сразу после старта
	mss.SyncOnce()

	ticker := time.NewTicker(mss.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			mss.SyncOnce()
		}
	}
}

// SyncOnce выполняет одну итерацию синхронизации:
//  1. Читает mode.json через modefile.LoadMode()
//  2. Сравнивает с текущим режимом sm.CurrentMode()
//  3. При расхождении — вызывает sm.ForceMode() и логирует смену
//
// Ошибки чтения mode.json не являются фатальными — логируются и пропускаются
// (mode.json может не существовать при первом запуске).
func (mss *ModeSyncService) SyncOnce() {
	modeSyncRunsTotal.Inc()

	// Читаем режим из mode.json
	fileMode, err := modefile.LoadMode(mss.modeFilePath)
	if err != nil {
		// mode.json может не существовать — это нормально при первом запуске
		mss.logger.Debug("Не удалось прочитать mode.json, пропуск синхронизации",
			slog.String("error", err.Error()),
		)
		modeSyncErrorsTotal.Inc()
		return
	}

	// Сравниваем с текущим режимом
	currentMode := mss.sm.CurrentMode()
	if currentMode == fileMode {
		return
	}

	// Обнаружено расхождение — принудительно обновляем режим
	mss.logger.Info("Обнаружено изменение режима в mode.json, применяю",
		slog.String("current_mode", string(currentMode)),
		slog.String("file_mode", string(fileMode)),
	)

	mss.sm.ForceMode(fileMode)
	modeSyncChangesTotal.Inc()

	mss.logger.Info("Режим обновлён из mode.json",
		slog.String("mode", string(fileMode)),
	)
}
