// main.go — точка входа Ingester Module.
// Phase 3: полный upload pipeline (Admin Module + SE клиенты, selector, dephealth).
package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/bigkaa/goartstore/ingester-module/internal/adminclient"
	"github.com/bigkaa/goartstore/ingester-module/internal/api/handlers"
	"github.com/bigkaa/goartstore/ingester-module/internal/api/middleware"
	"github.com/bigkaa/goartstore/ingester-module/internal/config"
	"github.com/bigkaa/goartstore/ingester-module/internal/seclient"
	"github.com/bigkaa/goartstore/ingester-module/internal/server"
	"github.com/bigkaa/goartstore/ingester-module/internal/service"
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
	logger.Info("Ingester Module запускается",
		slog.String("version", config.Version),
		slog.Int("port", cfg.Port),
	)

	// 3. JWT auth middleware (JWKS из Keycloak)
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
		logger.Error("Ошибка инициализации JWT auth", slog.String("error", err.Error()))
		os.Exit(1) //nolint:gocritic // exitAfterDefer: допустимо в main — defer выполняется при нормальном завершении
	}
	defer jwtAuth.Close()

	// 4. Admin Module HTTP-клиент
	adminClient, err := adminclient.New(
		cfg.AdminURL,
		cfg.TokenURL,
		cfg.CACertPath,
		cfg.AdminTimeout,
		cfg.ClientID,
		cfg.ClientSecret,
		logger,
	)
	if err != nil {
		logger.Error("Ошибка инициализации Admin Module клиента", slog.String("error", err.Error()))
		os.Exit(1) //nolint:gocritic // exitAfterDefer
	}

	// 5. SE HTTP-клиент (TokenProvider = adminclient.GetToken)
	seClient, err := seclient.New(
		cfg.SECACertPath,
		cfg.SEUploadTimeout,
		adminClient.GetToken,
		logger,
	)
	if err != nil {
		logger.Error("Ошибка инициализации SE клиента", slog.String("error", err.Error()))
		os.Exit(1) //nolint:gocritic // exitAfterDefer
	}

	// 6. Sequential Fill selector
	selector := service.NewSelectorService(adminClient)

	// 7. Upload service (pipeline: validation → SE selection → SE upload → AM register)
	uploadService := service.NewUploadService(
		selector, adminClient, seClient,
		cfg.MaxFileSize, cfg.MaxRetries, logger,
	)

	// 8. JWKS readiness checker
	jwksChecker, err := middleware.NewKeycloakReadinessChecker(
		cfg.JWKSURL, cfg.CACertPath, cfg.JWKSClientTimeout,
	)
	if err != nil {
		logger.Warn("Не удалось создать JWKS readiness checker, используется stub",
			slog.String("error", err.Error()),
		)
	}

	// 9. Health handler (Admin Module + JWKS checkers)
	adminChecker := handlers.NewAdminModuleChecker(adminClient)
	healthHandler := handlers.NewHealthHandler(adminChecker, jwksChecker)

	// 10. API handler (полный — с upload service)
	apiHandler := handlers.NewAPIHandler(healthHandler, uploadService, logger)

	// 11. HTTP-сервер с middleware:
	//     Порядок: metrics -> logging -> JWT (с exclusions для health/metrics)
	//     metrics первым = считает все запросы включая 401
	srv := server.New(cfg, logger, apiHandler,
		middleware.MetricsMiddleware(),
		middleware.RequestLogger(logger),
		server.JWTAuthWithExclusions(jwtAuth.Middleware(), "/health/", "/metrics"),
	)

	// 12. Topologymetrics (graceful start — warn при ошибке, не exit)
	dephealthSvc, err := service.NewDephealthService(
		cfg.DephealthName,
		cfg.DephealthGroup,
		cfg.AdminURL,
		cfg.DephealthCheckInterval,
		cfg.DephealthIsEntry,
		logger,
	)
	if err != nil {
		logger.Warn("Ошибка инициализации topologymetrics, продолжаем без мониторинга зависимостей",
			slog.String("error", err.Error()),
		)
	} else {
		if startErr := dephealthSvc.Start(context.Background()); startErr != nil {
			logger.Warn("Ошибка запуска topologymetrics",
				slog.String("error", startErr.Error()),
			)
		} else {
			defer dephealthSvc.Stop()
		}
	}

	// 13. Запуск сервера (блокирующий вызов с graceful shutdown)
	if err = srv.Run(); err != nil {
		logger.Error("Ошибка сервера", slog.String("error", err.Error()))
		os.Exit(1) //nolint:gocritic // exitAfterDefer: defer выполняется при нормальном завершении через graceful shutdown
	}

	logger.Info("Ingester Module остановлен")
}
