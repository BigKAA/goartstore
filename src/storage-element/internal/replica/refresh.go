// refresh.go — периодическое обновление индекса и режима на follower.
//
// FollowerRefreshService выполняет:
//  1. Пересборку in-memory индекса из attr.json на NFS
//  2. Синхронизацию режима из mode.json (leader записывает, follower читает)
//
// Запускается только на follower, интервал задаётся через SE_INDEX_REFRESH_INTERVAL.
package replica

import (
	"context"
	"log/slog"
	"time"

	"github.com/bigkaa/goartstore/storage-element/internal/domain/mode"
	"github.com/bigkaa/goartstore/storage-element/internal/storage/index"
)

// FollowerRefreshService — сервис периодического обновления данных на follower.
type FollowerRefreshService struct {
	idx          *index.Index
	sm           *mode.StateMachine
	dataDir      string
	modeFilePath string
	interval     time.Duration
	logger       *slog.Logger

	cancel context.CancelFunc
}

// NewFollowerRefreshService создаёт сервис обновления для follower.
//
// Параметры:
//   - idx: in-memory индекс метаданных
//   - sm: конечный автомат режимов
//   - dataDir: директория данных (общая NFS)
//   - modeFilePath: путь к mode.json
//   - interval: интервал обновления (SE_INDEX_REFRESH_INTERVAL)
//   - logger: логгер
func NewFollowerRefreshService(
	idx *index.Index,
	sm *mode.StateMachine,
	dataDir string,
	modeFilePath string,
	interval time.Duration,
	logger *slog.Logger,
) *FollowerRefreshService {
	return &FollowerRefreshService{
		idx:          idx,
		sm:           sm,
		dataDir:      dataDir,
		modeFilePath: modeFilePath,
		interval:     interval,
		logger:       logger.With(slog.String("component", "follower_refresh")),
	}
}

// Start запускает фоновую горутину обновления.
func (s *FollowerRefreshService) Start(ctx context.Context) {
	refreshCtx, cancel := context.WithCancel(ctx)
	s.cancel = cancel

	go s.run(refreshCtx)

	s.logger.Info("FollowerRefreshService запущен",
		slog.String("interval", s.interval.String()),
	)
}

// Stop останавливает фоновой процесс обновления.
func (s *FollowerRefreshService) Stop() {
	if s.cancel != nil {
		s.cancel()
	}
	s.logger.Info("FollowerRefreshService остановлен")
}

// run — основной цикл фоновой горутины.
func (s *FollowerRefreshService) run(ctx context.Context) {
	// Первое обновление — сразу при запуске
	s.refresh()

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.refresh()
		}
	}
}

// refresh выполняет одно обновление: пересборка индекса + синхронизация режима.
func (s *FollowerRefreshService) refresh() {
	// 1. Пересборка индекса из attr.json на NFS
	if err := s.idx.RebuildFromDir(s.dataDir); err != nil {
		s.logger.Error("Ошибка пересборки индекса",
			slog.String("error", err.Error()),
		)
	} else {
		s.logger.Debug("Индекс пересобран",
			slog.Int("files", s.idx.Count()),
		)
	}

	// 2. Синхронизация режима из mode.json
	if s.modeFilePath != "" {
		loadedMode, err := LoadMode(s.modeFilePath)
		if err != nil {
			s.logger.Debug("Не удалось загрузить mode.json (возможно, ещё не создан)",
				slog.String("error", err.Error()),
			)
			return
		}

		currentMode := s.sm.CurrentMode()
		if currentMode != loadedMode {
			s.sm.ForceMode(loadedMode)
			s.logger.Info("Режим синхронизирован из mode.json",
				slog.String("from", string(currentMode)),
				slog.String("to", string(loadedMode)),
			)
		}
	}
}
