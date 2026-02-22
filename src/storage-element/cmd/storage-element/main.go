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
	"github.com/arturkryukov/artsore/storage-element/internal/replica"
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

	// Предупреждения о параметрах topologymetrics с дефолтными значениями
	if os.Getenv("SE_DEPHEALTH_GROUP") == "" {
		logger.Warn("SE_DEPHEALTH_GROUP не задана, используется значение по умолчанию",
			slog.String("default", cfg.DephealthGroup),
		)
	}
	if os.Getenv("SE_DEPHEALTH_DEP_NAME") == "" {
		logger.Warn("SE_DEPHEALTH_DEP_NAME не задана, используется значение по умолчанию",
			slog.String("default", cfg.DephealthDepName),
		)
	}

	// --- Инициализация компонентов ---

	// 1. Определение начального режима: mode.json приоритетнее SE_MODE
	initialMode := cfg.Mode
	modeFilePath := replica.ModeFilePath(cfg.DataDir)
	if loadedMode, loadErr := replica.LoadMode(modeFilePath); loadErr == nil {
		initialMode = string(loadedMode)
		logger.Info("Режим загружен из mode.json",
			slog.String("mode", initialMode),
			slog.String("path", modeFilePath),
		)
	} else {
		logger.Debug("mode.json не найден, используется SE_MODE",
			slog.String("mode", initialMode),
			slog.String("error", loadErr.Error()),
		)
	}

	// 2. Конечный автомат режимов
	sm, err := mode.NewStateMachine(mode.StorageMode(initialMode))
	if err != nil {
		logger.Error("Ошибка инициализации state machine", slog.String("error", err.Error()))
		os.Exit(1)
	}
	logger.Info("Режим работы установлен", slog.String("mode", initialMode))

	// 3. WAL-движок
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

	// 4. Файловое хранилище
	store, err := filestore.New(cfg.DataDir)
	if err != nil {
		logger.Error("Ошибка инициализации FileStore", slog.String("error", err.Error()))
		os.Exit(1)
	}

	// 5. In-memory индекс метаданных
	idx := index.New(logger)
	if err := idx.BuildFromDir(cfg.DataDir); err != nil {
		logger.Error("Ошибка построения индекса", slog.String("error", err.Error()))
		os.Exit(1)
	}

	// Обновляем Prometheus метрики файлов
	updateFileMetrics(idx)

	// 6. Сервисы
	uploadSvc := service.NewUploadService(cfg, walEngine, store, idx, sm, logger)
	downloadSvc := service.NewDownloadService(store, idx, sm, logger)

	ctx := context.Background()

	// 7. Фоновые процессы и leader election
	gcSvc := service.NewGCService(store, idx, cfg.GCInterval, logger)
	reconcileSvc := service.NewReconcileService(store, idx, cfg.DataDir, cfg.ReconcileInterval, logger)

	// RoleProvider и proxy middleware — зависят от replica mode
	var roleProvider handlers.RoleProvider
	var proxyMiddleware server.ProxyMiddleware
	var election *replica.Election
	var refreshSvc *replica.FollowerRefreshService

	if cfg.ReplicaMode == "replicated" {
		// --- Replicated mode: leader election ---
		election = replica.NewElection(
			cfg.DataDir,
			cfg.Port,
			cfg.ElectionRetryInterval,
			// onBecomeLeader: запустить GC и Reconcile
			func() {
				logger.Info("Стал leader — запуск GC и Reconcile")
				gcSvc.Start(ctx)
				reconcileSvc.Start(ctx)
				// Загрузить mode из mode.json (если есть)
				if loadedMode, loadErr := replica.LoadMode(modeFilePath); loadErr == nil {
					sm.ForceMode(loadedMode)
				}
			},
			// onBecomeFollower: остановить GC и Reconcile, запустить refresh
			func() {
				logger.Info("Стал follower — остановка GC и Reconcile")
				gcSvc.Stop()
				reconcileSvc.Stop()
			},
			logger,
		)

		if err := election.Start(); err != nil {
			logger.Error("Ошибка запуска leader election", slog.String("error", err.Error()))
			os.Exit(1)
		}

		// Если follower — запустить FollowerRefreshService
		if !election.IsLeader() {
			refreshSvc = replica.NewFollowerRefreshService(
				idx, sm, cfg.DataDir, modeFilePath,
				cfg.IndexRefreshInterval, logger,
			)
			refreshSvc.Start(ctx)
		}

		roleProvider = &roleProviderAdapter{election: election}
		proxyMiddleware = replica.NewLeaderProxy(election, logger)
	} else {
		// --- Standalone mode: GC/Reconcile стартуют безусловно ---
		gcSvc.Start(ctx)
		reconcileSvc.Start(ctx)
		roleProvider = &standaloneRoleAdapter{}
	}

	// 7.1 topologymetrics — мониторинг зависимостей
	dephealthSvc, dephealthErr := service.NewDephealthService(
		cfg.StorageID,
		cfg.DephealthGroup,
		cfg.DephealthDepName,
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
				slog.String("group", cfg.DephealthGroup),
				slog.String("dep_name", cfg.DephealthDepName),
				slog.String("jwks_url", cfg.JWKSUrl),
				slog.String("check_interval", cfg.DephealthCheckInterval.String()),
			)
		}
	}

	// 8. ModePersister — сохранение mode.json при смене режима (все режимы)
	var updatedByFn func() string
	if cfg.ReplicaMode == "replicated" && election != nil {
		updatedByFn = election.LeaderAddr
	} else {
		updatedByFn = func() string {
			return fmt.Sprintf("%s:%d", cfg.StorageID, cfg.Port)
		}
	}
	modePersister := &modePersisterAdapter{
		path:        modeFilePath,
		updatedByFn: updatedByFn,
	}

	// 9. Handlers
	filesHandler := handlers.NewFilesHandler(uploadSvc, downloadSvc, store, idx, sm)
	systemHandler := handlers.NewSystemHandler(cfg, sm, idx, diskUsageFn(cfg.DataDir), roleProvider)
	modeHandler := handlers.NewModeHandler(sm, logger, modePersister)
	maintenanceHandler := handlers.NewMaintenanceHandler(reconcileSvc)
	healthHandler := handlers.NewHealthHandlerFull(cfg.DataDir, cfg.WALDir, idx, roleProvider)
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

	// 10. JWT middleware
	var jwtAuth server.JWTAuthProvider
	jwtMiddleware, err := middleware.NewJWTAuth(cfg.JWKSUrl, cfg.JWKSCACert, logger)
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

	// 11. Создание и запуск HTTP-сервера
	srv := server.New(cfg, logger, apiHandler, jwtAuth, proxyMiddleware)

	if err := srv.Run(); err != nil {
		logger.Error("Ошибка сервера", slog.String("error", err.Error()))
		os.Exit(1)
	}

	// --- Graceful shutdown фоновых процессов ---
	// Порядок важен: election.Stop() первым — освобождаем NFS flock
	// до истечения K8s terminationGracePeriodSeconds (SIGKILL).
	// Без своевременного освобождения flock follower не сможет стать leader
	// до истечения NFS v4 lease timeout (~90s по умолчанию).
	logger.Info("Остановка фоновых процессов...")

	if election != nil {
		election.Stop()
		logger.Info("Leader election остановлен, NFS flock освобождён")
	}

	gcSvc.Stop()
	reconcileSvc.Stop()
	if dephealthSvc != nil {
		dephealthSvc.Stop()
	}
	if refreshSvc != nil {
		refreshSvc.Stop()
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

// --- Адаптеры для интерфейсов handlers ---

// roleProviderAdapter адаптирует replica.Election к handlers.RoleProvider.
type roleProviderAdapter struct {
	election *replica.Election
}

func (a *roleProviderAdapter) CurrentRole() string {
	return string(a.election.CurrentRole())
}

func (a *roleProviderAdapter) IsLeader() bool {
	return a.election.IsLeader()
}

func (a *roleProviderAdapter) LeaderAddr() string {
	return a.election.LeaderAddr()
}

// standaloneRoleAdapter — адаптер RoleProvider для standalone mode.
type standaloneRoleAdapter struct{}

func (a *standaloneRoleAdapter) CurrentRole() string {
	return string(replica.RoleStandalone)
}

func (a *standaloneRoleAdapter) IsLeader() bool {
	return true
}

func (a *standaloneRoleAdapter) LeaderAddr() string {
	return ""
}

// modePersisterAdapter адаптирует SaveMode к handlers.ModePersister.
type modePersisterAdapter struct {
	path        string
	updatedByFn func() string
}

func (a *modePersisterAdapter) SaveMode(m mode.StorageMode) error {
	updatedBy := a.updatedByFn()
	return replica.SaveMode(a.path, m, updatedBy)
}
