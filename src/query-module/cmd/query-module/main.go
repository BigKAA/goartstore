// main.go — точка входа Query Module.
// Загружает конфигурацию, подключается к PostgreSQL, применяет миграции (индексы),
// инициализирует JWT middleware, создаёт repository/cache/service/handlers и HTTP-сервер.
// Поддерживает proxy download с lazy cleanup и мониторинг зависимостей (topologymetrics).
package main //nolint:cyclop // main — точка входа, высокая сложность обусловлена инициализацией компонентов

import (
	"context"
	"log/slog"
	"os"

	"github.com/jackc/pgx/v5/stdlib"

	"github.com/bigkaa/goartstore/query-module/internal/adminclient"
	"github.com/bigkaa/goartstore/query-module/internal/api/handlers"
	"github.com/bigkaa/goartstore/query-module/internal/api/middleware"
	"github.com/bigkaa/goartstore/query-module/internal/config"
	"github.com/bigkaa/goartstore/query-module/internal/database"
	"github.com/bigkaa/goartstore/query-module/internal/repository"
	"github.com/bigkaa/goartstore/query-module/internal/seclient"
	"github.com/bigkaa/goartstore/query-module/internal/server"
	"github.com/bigkaa/goartstore/query-module/internal/service"
)

func main() {
	// 1. Загрузка конфигурации из переменных окружения
	cfg, err := config.Load()
	if err != nil {
		slog.Error("Ошибка загрузки конфигурации", slog.String("error", err.Error()))
		os.Exit(1)
	}

	// 2. Настройка логгера
	logger := config.SetupLogger(cfg)
	logger.Info("Query Module запускается",
		slog.String("version", config.Version),
		slog.Int("port", cfg.Port),
	)

	// 3. Применение миграций БД (индексы для поиска)
	logger.Info("Применение миграций БД...")
	if err = database.Migrate(cfg, logger); err != nil {
		logger.Error("Ошибка миграций БД", slog.String("error", err.Error()))
		os.Exit(1)
	}

	// 4. Подключение к PostgreSQL (pgxpool)
	ctx := context.Background()
	pool, err := database.Connect(ctx, cfg, logger)
	if err != nil {
		logger.Error("Ошибка подключения к PostgreSQL", slog.String("error", err.Error()))
		os.Exit(1)
	}
	defer pool.Close()

	// 5. *sql.DB адаптер pgxpool (для topologymetrics)
	sqlDB := stdlib.OpenDBFromPool(pool)
	defer sqlDB.Close()

	// 6. JWT middleware
	jwtAuth, err := middleware.NewJWTAuth(
		cfg.JWKSURL,
		cfg.CACertPath,
		cfg.JWTIssuer,
		cfg.RoleAdminGroups,
		cfg.RoleReadonlyGroups,
		cfg.JWKSClientTimeout,
		cfg.JWKSRefreshInterval,
		cfg.JWTLeeway,
		logger,
	)
	if err != nil {
		logger.Error("Ошибка создания JWT middleware", slog.String("error", err.Error()))
		os.Exit(1) //nolint:gocritic // exitAfterDefer: допустимо в main — defer выполняется при нормальном завершении
	}
	defer jwtAuth.Close()
	logger.Info("JWT middleware инициализирован",
		slog.String("jwks_url", cfg.JWKSURL),
	)

	// 7. Admin Module HTTP-клиент
	adminClient, err := adminclient.New(
		cfg.AdminURL,
		cfg.CACertPath,
		cfg.AdminTimeout,
		cfg.ClientID,
		cfg.ClientSecret,
		logger,
	)
	if err != nil {
		logger.Error("Ошибка создания Admin Module клиента", slog.String("error", err.Error()))
		os.Exit(1)
	}
	logger.Info("Admin Module клиент инициализирован",
		slog.String("admin_url", cfg.AdminURL),
	)

	// 8. SE HTTP-клиент (proxy download)
	seClient, err := seclient.New(
		cfg.SECACertPath,
		cfg.SEDownloadTimeout,
		adminClient.GetToken,
		logger,
	)
	if err != nil {
		logger.Error("Ошибка создания SE клиента", slog.String("error", err.Error()))
		os.Exit(1)
	}
	logger.Info("SE клиент инициализирован",
		slog.Duration("timeout", cfg.SEDownloadTimeout),
	)

	// 9. Repository слой
	fileRepo := repository.NewFileRepository(pool)

	// 10. LRU-кэш метаданных файлов
	cacheService := service.NewCacheService(cfg.CacheMaxSize, cfg.CacheTTL)
	logger.Info("LRU-кэш инициализирован",
		slog.Int("max_size", cfg.CacheMaxSize),
		slog.Duration("ttl", cfg.CacheTTL),
	)

	// 11. Search service (repository + cache)
	searchService := service.NewSearchService(fileRepo, cacheService, logger)

	// 12. Download service (repository + cache + admin client + SE client)
	downloadService := service.NewDownloadService(fileRepo, cacheService, adminClient, seClient, logger)

	// 13. Readiness checker
	pgChecker := database.NewReadinessChecker(pool)

	// 14. Health handler (с PostgreSQL checker)
	healthHandler := handlers.NewHealthHandler(pgChecker)

	// 15. API handler (search, file metadata, download)
	apiHandler := handlers.NewAPIHandler(healthHandler, searchService, downloadService, logger)

	// 16. HTTP-сервер с middleware (metrics → logging → JWT с exclusions)
	srv := server.New(cfg, logger, apiHandler,
		middleware.MetricsMiddleware(),
		middleware.RequestLogger(logger),
		server.JWTAuthWithExclusions(
			jwtAuth.Middleware(),
			"/health/", "/metrics",
		),
	)

	// 17. Topologymetrics — мониторинг зависимостей (graceful start)
	serviceID := cfg.DephealthName
	if serviceID == "" {
		serviceID = "query-module"
	}
	dephealthSvc, dephealthErr := service.NewDephealthService(
		serviceID,
		cfg.DephealthGroup,
		sqlDB,
		cfg.DatabaseURL(),
		cfg.AdminURL,
		cfg.DephealthCheckInterval,
		cfg.DephealthIsEntry,
		logger,
	)
	if dephealthErr != nil {
		// Graceful start: логируем предупреждение, не прерываем запуск
		logger.Warn("Не удалось инициализировать мониторинг зависимостей",
			slog.String("error", dephealthErr.Error()),
		)
	} else {
		if startErr := dephealthSvc.Start(ctx); startErr != nil {
			logger.Warn("Не удалось запустить мониторинг зависимостей",
				slog.String("error", startErr.Error()),
			)
		} else {
			defer dephealthSvc.Stop()
		}
	}

	// 18. Запуск сервера (блокирующий вызов с graceful shutdown)
	if err = srv.Run(); err != nil {
		logger.Error("Ошибка сервера", slog.String("error", err.Error()))
		os.Exit(1)
	}

	logger.Info("Query Module остановлен")
}
