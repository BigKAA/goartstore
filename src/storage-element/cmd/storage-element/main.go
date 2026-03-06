// Точка входа Storage Element — модуля физического хранения файлов.
//
// Stateless архитектура: все экземпляры SE равноправны.
// Leader election, proxy, WAL — удалены.
// Координация через per-file lock-файлы с TTL.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"regexp"

	"github.com/bigkaa/goartstore/storage-element/internal/api/handlers"
	"github.com/bigkaa/goartstore/storage-element/internal/api/middleware"
	"github.com/bigkaa/goartstore/storage-element/internal/config"
	"github.com/bigkaa/goartstore/storage-element/internal/domain/mode"
	"github.com/bigkaa/goartstore/storage-element/internal/lockfile"
	"github.com/bigkaa/goartstore/storage-element/internal/modefile"
	"github.com/bigkaa/goartstore/storage-element/internal/server"
	"github.com/bigkaa/goartstore/storage-element/internal/service"
	"github.com/bigkaa/goartstore/storage-element/internal/storage/attr"
	"github.com/bigkaa/goartstore/storage-element/internal/storage/filestore"
	"github.com/bigkaa/goartstore/storage-element/internal/storage/index"
)

//nolint:cyclop,gocognit // TODO: разбить main на подфункции
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
		slog.Int64("max_capacity", cfg.MaxCapacity),
		slog.String("storage_backend", cfg.StorageBackend),
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
	modeFilePath := modefile.ModeFilePath(cfg.DataDir)
	if loadedMode, loadErr := modefile.LoadMode(modeFilePath); loadErr == nil {
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

	// 3. Storage Backend (через абстракцию)
	hostname, _ := os.Hostname()

	// Инициализация backend на основе cfg.StorageBackend
	store, err := filestore.New(cfg.DataDir)
	if err != nil {
		logger.Error("Ошибка инициализации FileStore", slog.String("error", err.Error()))
		os.Exit(1)
	}

	attrStore := attr.NewStore(cfg.DataDir)

	lockMgr := lockfile.NewLockManager(cfg.DataDir, cfg.UploadLockTTL, hostname)
	if err := lockMgr.EnsureDir(); err != nil {
		logger.Error("Ошибка создания директории lock-файлов", slog.String("error", err.Error()))
		os.Exit(1)
	}
	logger.Info("Storage Backend инициализирован",
		slog.String("backend", cfg.StorageBackend),
		slog.String("data_dir", cfg.DataDir),
		slog.String("lock_dir", cfg.DataDir+"/.locks"),
		slog.String("ttl", cfg.UploadLockTTL.String()),
		slog.String("holder", hostname),
	)

	// 4. In-memory индекс метаданных — построение из AttrStore
	idx := index.New(logger)
	metadatas, err := attrStore.ScanAll(context.Background())
	if err != nil {
		logger.Error("Ошибка сканирования attr.json для построения индекса", slog.String("error", err.Error()))
		os.Exit(1)
	}
	idx.RebuildFromMetadata(metadatas)
	logger.Info("Индекс построен из AttrStore",
		slog.Int("files", idx.Count()),
	)

	// Обновляем Prometheus метрики файлов
	updateFileMetrics(idx)

	// 5. Сервисы (используют backend интерфейсы)
	uploadSvc := service.NewUploadService(cfg, store, attrStore, idx, sm, lockMgr, logger)
	downloadSvc := service.NewDownloadService(store, idx, sm, logger)

	ctx := context.Background()

	// 6. Фоновые процессы — запускаются безусловно на каждом pod-е (stateless)
	gcSvc := service.NewGCService(store, attrStore, idx, lockMgr, cfg.GCInterval, logger)
	reconcileSvc := service.NewReconcileService(store, attrStore, idx, lockMgr, cfg.ReconcileInterval, logger)
	modeSyncSvc := service.NewModeSyncService(modeFilePath, sm, cfg.ModeSyncInterval, logger)
	indexSyncSvc := service.NewIndexSyncService(idx, attrStore, cfg.IndexSyncInterval, logger)
	gcSvc.Start(ctx)
	reconcileSvc.Start(ctx)
	modeSyncSvc.Start(ctx)
	indexSyncSvc.Start(ctx)

	// 7. topologymetrics — мониторинг зависимостей
	//
	// Определение имени владельца пода для метки name:
	// 1. DEPHEALTH_NAME (env) → использовать как есть
	// 2. Парсинг os.Hostname() → извлечь имя владельца (Deployment/StatefulSet)
	// 3. Fallback → cfg.StorageID
	dephealthName := cfg.DephealthName
	if dephealthName == "" {
		dephealthName = resolveOwnerFromHostname(logger, cfg.StorageID)
	}

	dephealthSvc, dephealthErr := service.NewDephealthService(
		dephealthName,
		cfg.DephealthGroup,
		cfg.DephealthDepName,
		cfg.JWKSUrl,
		cfg.DephealthCheckInterval,
		cfg.TLSSkipVerify,
		cfg.DephealthIsEntry,
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
				slog.String("name", dephealthName),
				slog.String("group", cfg.DephealthGroup),
				slog.String("dep_name", cfg.DephealthDepName),
				slog.String("jwks_url", cfg.JWKSUrl),
				slog.String("check_interval", cfg.DephealthCheckInterval.String()),
				slog.Bool("is_entry", cfg.DephealthIsEntry),
			)
		}
	}

	// 8. ModePersister — сохранение mode.json при смене режима
	modePersister := &modePersisterAdapter{
		path: modeFilePath,
		updatedByFn: func() string {
			return fmt.Sprintf("%s:%d", cfg.StorageID, cfg.Port)
		},
	}

	// 9. Handlers (используют backend интерфейсы)
	filesHandler := handlers.NewFilesHandler(uploadSvc, downloadSvc, store, attrStore, idx, sm, lockMgr)
	systemHandler := handlers.NewSystemHandler(cfg, sm, idx, store)
	modeHandler := handlers.NewModeHandler(sm, logger, modePersister)
	maintenanceHandler := handlers.NewMaintenanceHandler(reconcileSvc)
	locksHandler := handlers.NewLocksHandler(lockMgr)
	healthHandler := handlers.NewHealthHandlerFull(cfg.DataDir, idx)
	metricsHandler := server.NewMetricsHandler()

	// Единый API handler
	apiHandler := handlers.NewAPIHandler(
		filesHandler,
		systemHandler,
		modeHandler,
		maintenanceHandler,
		locksHandler,
		healthHandler,
		metricsHandler,
	)

	// 10. JWT middleware
	var jwtAuth server.JWTAuthProvider
	jwtMiddleware, err := middleware.NewJWTAuth(middleware.JWTAuthConfig{
		JWKSURL:         cfg.JWKSUrl,
		CACertPath:      cfg.CACertPath,
		TLSSkipVerify:   cfg.TLSSkipVerify,
		ClientTimeout:   cfg.JWKSClientTimeout,
		RefreshInterval: cfg.JWKSRefreshInterval,
		JWTLeeway:       cfg.JWTLeeway,
	}, logger)
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
	srv := server.New(cfg, logger, apiHandler, jwtAuth)

	if err := srv.Run(); err != nil {
		logger.Error("Ошибка сервера", slog.String("error", err.Error()))
		os.Exit(1)
	}

	// --- Graceful shutdown фоновых процессов ---
	logger.Info("Остановка фоновых процессов...")

	gcSvc.Stop()
	reconcileSvc.Stop()
	modeSyncSvc.Stop()
	indexSyncSvc.Stop()
	if dephealthSvc != nil {
		dephealthSvc.Stop()
	}

	logger.Info("Storage Element остановлен")
}

// updateFileMetrics обновляет Prometheus метрики файлов из индекса.
func updateFileMetrics(idx *index.Index) {
	middleware.FilesTotal.Set(float64(idx.Count()))
}

// --- Адаптеры для интерфейсов handlers ---

// modePersisterAdapter адаптирует SaveMode к handlers.ModePersister.
type modePersisterAdapter struct {
	path        string
	updatedByFn func() string
}

func (a *modePersisterAdapter) SaveMode(m mode.StorageMode) error {
	updatedBy := a.updatedByFn()
	return modefile.SaveMode(a.path, m, updatedBy)
}

// --- Вспомогательные функции для DEPHEALTH_NAME ---

// reDeployment — паттерн имени пода Deployment: {name}-{rs-hash 8-10}-{pod-hash 4-5}
var reDeployment = regexp.MustCompile(`^(.+)-[a-z0-9]{8,10}-[a-z0-9]{4,5}$`)

// reStatefulSet — паттерн имени пода StatefulSet: {name}-{ordinal}
var reStatefulSet = regexp.MustCompile(`^(.+)-(\d+)$`)

// parseOwnerName извлекает имя владельца (Deployment/StatefulSet) из hostname пода.
// Порядок проверки: Deployment → StatefulSet → hostname целиком (fallback).
func parseOwnerName(hostname string) string {
	// Deployment: {name}-{rs-hash}-{pod-hash}
	if m := reDeployment.FindStringSubmatch(hostname); m != nil {
		return m[1]
	}
	// StatefulSet: {name}-{ordinal}
	if m := reStatefulSet.FindStringSubmatch(hostname); m != nil {
		return m[1]
	}
	// Fallback — hostname целиком
	return hostname
}

// resolveOwnerFromHostname определяет имя владельца пода из os.Hostname()
// и логирует предупреждение о том, что DEPHEALTH_NAME не была задана.
func resolveOwnerFromHostname(logger *slog.Logger, fallback string) string {
	hostname, err := os.Hostname()
	if err != nil {
		logger.Warn("Не удалось получить hostname, используется fallback для DEPHEALTH_NAME",
			slog.String("fallback", fallback),
			slog.String("error", err.Error()),
		)
		return fallback
	}

	resolved := parseOwnerName(hostname)
	logger.Warn("DEPHEALTH_NAME не задана, имя определено из hostname",
		slog.String("hostname", hostname),
		slog.String("resolved_name", resolved),
	)
	return resolved
}
