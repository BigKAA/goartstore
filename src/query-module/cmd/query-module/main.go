// main.go — точка входа Query Module.
// Загружает конфигурацию, подключается к PostgreSQL, применяет миграции (индексы),
// инициализирует JWT middleware, создаёт repository/cache/service/handlers и HTTP-сервер.
package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/bigkaa/goartstore/query-module/internal/adminclient"
	"github.com/bigkaa/goartstore/query-module/internal/api/handlers"
	"github.com/bigkaa/goartstore/query-module/internal/api/middleware"
	"github.com/bigkaa/goartstore/query-module/internal/config"
	"github.com/bigkaa/goartstore/query-module/internal/database"
	"github.com/bigkaa/goartstore/query-module/internal/repository"
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

	// 5. JWT middleware
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

	// 6. Admin Module HTTP-клиент
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
	_ = adminClient // будет использован в Phase 4 (download service)
	logger.Info("Admin Module клиент инициализирован",
		slog.String("admin_url", cfg.AdminURL),
	)

	// 7. Repository слой
	fileRepo := repository.NewFileRepository(pool)

	// 8. LRU-кэш метаданных файлов
	cacheService := service.NewCacheService(cfg.CacheMaxSize, cfg.CacheTTL)
	logger.Info("LRU-кэш инициализирован",
		slog.Int("max_size", cfg.CacheMaxSize),
		slog.Duration("ttl", cfg.CacheTTL),
	)

	// 9. Search service (repository + cache)
	searchService := service.NewSearchService(fileRepo, cacheService, logger)

	// 10. Readiness checkers
	pgChecker := database.NewReadinessChecker(pool)

	// 11. Health handler (с PostgreSQL checker)
	healthHandler := handlers.NewHealthHandler(pgChecker)

	// 12. API handler (search, file metadata + stubs для download)
	apiHandler := handlers.NewAPIHandler(healthHandler, searchService, logger)

	// 13. HTTP-сервер с middleware (metrics → logging → JWT с exclusions)
	srv := server.New(cfg, logger, apiHandler,
		middleware.MetricsMiddleware(),
		middleware.RequestLogger(logger),
		server.JWTAuthWithExclusions(
			jwtAuth.Middleware(),
			"/health/", "/metrics",
		),
	)

	// 14. Запуск сервера (блокирующий вызов с graceful shutdown)
	if err = srv.Run(); err != nil {
		logger.Error("Ошибка сервера", slog.String("error", err.Error()))
		os.Exit(1)
	}

	logger.Info("Query Module остановлен")
}
