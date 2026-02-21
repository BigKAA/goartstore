// Точка входа Storage Element — модуля физического хранения файлов.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/arturkryukov/artsore/storage-element/internal/api/handlers"
	"github.com/arturkryukov/artsore/storage-element/internal/api/middleware"
	"github.com/arturkryukov/artsore/storage-element/internal/config"
	"github.com/arturkryukov/artsore/storage-element/internal/domain/mode"
	"github.com/arturkryukov/artsore/storage-element/internal/server"
	"github.com/arturkryukov/artsore/storage-element/internal/service"
	"github.com/arturkryukov/artsore/storage-element/internal/storage/filestore"
	"github.com/arturkryukov/artsore/storage-element/internal/storage/index"
	"github.com/arturkryukov/artsore/storage-element/internal/storage/wal"
)

func main() {
	// Загрузка конфигурации из переменных окружения
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Ошибка конфигурации: %v\n", err)
		os.Exit(1)
	}

	// Настройка логгера
	logger := config.SetupLogger(cfg)
	logger.Info("Storage Element запускается",
		slog.String("storage_id", cfg.StorageID),
		slog.String("version", config.Version),
		slog.String("mode", cfg.Mode),
		slog.Int("port", cfg.Port),
		slog.String("replica_mode", cfg.ReplicaMode),
	)

	// --- Инициализация компонентов ---

	// 1. Конечный автомат режимов
	sm, err := mode.NewStateMachine(mode.StorageMode(cfg.Mode))
	if err != nil {
		logger.Error("Ошибка инициализации state machine", slog.String("error", err.Error()))
		os.Exit(1)
	}
	logger.Info("Режим работы установлен", slog.String("mode", cfg.Mode))

	// 2. WAL-движок
	walEngine, err := wal.New(cfg.WALDir, logger)
	if err != nil {
		logger.Error("Ошибка инициализации WAL", slog.String("error", err.Error()))
		os.Exit(1)
	}

	// WAL recovery: откатываем pending транзакции
	pending, err := walEngine.RecoverPending()
	if err != nil {
		logger.Error("Ошибка восстановления WAL", slog.String("error", err.Error()))
		os.Exit(1)
	}
	if len(pending) > 0 {
		logger.Warn("Обнаружены незавершённые WAL-транзакции, откатываем",
			slog.Int("count", len(pending)),
		)
		for _, entry := range pending {
			if rbErr := walEngine.Rollback(entry.TransactionID); rbErr != nil {
				logger.Error("Ошибка отката WAL-транзакции",
					slog.String("tx_id", entry.TransactionID),
					slog.String("error", rbErr.Error()),
				)
			} else {
				logger.Info("WAL-транзакция откачена",
					slog.String("tx_id", entry.TransactionID),
					slog.String("file_id", entry.FileID),
				)
			}
		}
	}

	// 3. Файловое хранилище
	store, err := filestore.New(cfg.DataDir)
	if err != nil {
		logger.Error("Ошибка инициализации FileStore", slog.String("error", err.Error()))
		os.Exit(1)
	}

	// 4. In-memory индекс метаданных
	idx := index.New(logger)
	if err := idx.BuildFromDir(cfg.DataDir); err != nil {
		logger.Error("Ошибка построения индекса", slog.String("error", err.Error()))
		os.Exit(1)
	}

	// Обновляем Prometheus метрики файлов
	updateFileMetrics(idx)

	// 5. Сервисы
	uploadSvc := service.NewUploadService(cfg, walEngine, store, idx, sm, logger)
	downloadSvc := service.NewDownloadService(store, idx, sm, logger)

	// 6. Фоновые процессы
	ctx := context.Background()

	// 6.1 GC — фоновая очистка файлов
	gcSvc := service.NewGCService(store, idx, cfg.GCInterval, logger)
	gcSvc.Start(ctx)

	// 6.2 Reconciliation — фоновая сверка
	reconcileSvc := service.NewReconcileService(store, idx, cfg.DataDir, cfg.ReconcileInterval, logger)
	reconcileSvc.Start(ctx)

	// 6.3 topologymetrics — мониторинг зависимостей
	dephealthSvc, dephealthErr := service.NewDephealthService(
		cfg.StorageID,
		cfg.JWKSUrl,
		cfg.DephealthCheckInterval,
		logger,
	)
	if dephealthErr != nil {
		logger.Warn("topologymetrics недоступен, запуск без мониторинга зависимостей",
			slog.String("error", dephealthErr.Error()),
		)
	} else {
		if startErr := dephealthSvc.Start(ctx); startErr != nil {
			logger.Warn("Ошибка запуска topologymetrics",
				slog.String("error", startErr.Error()),
			)
		} else {
			logger.Info("topologymetrics запущен",
				slog.String("jwks_url", cfg.JWKSUrl),
				slog.String("check_interval", cfg.DephealthCheckInterval.String()),
			)
		}
	}

	// 7. Handlers
	filesHandler := handlers.NewFilesHandler(uploadSvc, downloadSvc, store, idx, sm)
	systemHandler := handlers.NewSystemHandler(cfg, sm, idx, diskUsageFn(cfg.DataDir))
	modeHandler := handlers.NewModeHandler(sm, logger)
	maintenanceHandler := handlers.NewMaintenanceHandler(reconcileSvc)
	healthHandler := handlers.NewHealthHandlerFull(cfg.DataDir, cfg.WALDir, idx)
	metricsHandler := server.NewMetricsHandler()

	// Единый API handler
	apiHandler := handlers.NewAPIHandler(
		filesHandler,
		systemHandler,
		modeHandler,
		maintenanceHandler,
		healthHandler,
		metricsHandler,
	)

	// 8. JWT middleware
	var jwtAuth server.JWTAuthProvider
	jwtMiddleware, err := middleware.NewJWTAuth(cfg.JWKSUrl, logger)
	if err != nil {
		// JWT недоступен — запускаем без аутентификации (для разработки)
		logger.Warn("JWT JWKS недоступен, запуск без аутентификации",
			slog.String("jwks_url", cfg.JWKSUrl),
			slog.String("error", err.Error()),
		)
	} else {
		jwtAuth = jwtMiddleware
		logger.Info("JWT аутентификация настроена",
			slog.String("jwks_url", cfg.JWKSUrl),
		)
	}

	// 9. Создание и запуск HTTP-сервера
	srv := server.New(cfg, logger, apiHandler, jwtAuth)

	if err := srv.Run(); err != nil {
		logger.Error("Ошибка сервера", slog.String("error", err.Error()))
		os.Exit(1)
	}

	// --- Graceful shutdown фоновых процессов ---
	logger.Info("Остановка фоновых процессов...")

	gcSvc.Stop()
	reconcileSvc.Stop()
	if dephealthSvc != nil {
		dephealthSvc.Stop()
	}

	logger.Info("Storage Element остановлен")
}

// updateFileMetrics обновляет Prometheus метрики файлов из индекса.
func updateFileMetrics(idx *index.Index) {
	middleware.FilesTotal.WithLabelValues("active").Set(float64(idx.CountByStatus("active")))
	middleware.FilesTotal.WithLabelValues("deleted").Set(float64(idx.CountByStatus("deleted")))
	middleware.FilesTotal.WithLabelValues("expired").Set(float64(idx.CountByStatus("expired")))
}

// diskUsageFn возвращает функцию для получения информации об ёмкости диска.
func diskUsageFn(dataDir string) func() (total, used, available int64, err error) {
	return func() (int64, int64, int64, error) {
		return getDiskUsage(dataDir)
	}
}
