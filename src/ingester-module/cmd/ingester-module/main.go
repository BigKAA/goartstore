// main.go — точка входа Ingester Module.
// Phase 2: JWT, logging, metrics middleware.
// Phase 3: полный upload pipeline (Admin Module + SE клиенты).
package main

import (
	"log/slog"
	"os"

	"github.com/bigkaa/goartstore/ingester-module/internal/api/handlers"
	"github.com/bigkaa/goartstore/ingester-module/internal/api/middleware"
	"github.com/bigkaa/goartstore/ingester-module/internal/config"
	"github.com/bigkaa/goartstore/ingester-module/internal/server"
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

	// 4. Health handler (stub readiness — полная реализация в Phase 3.7)
	healthHandler := handlers.NewHealthHandler()

	// 5. API handler (uploadService = nil — полная реализация в Phase 3)
	apiHandler := handlers.NewAPIHandler(healthHandler, logger)

	// 6. HTTP-сервер с middleware:
	//    Порядок: metrics -> logging -> JWT (с exclusions для health/metrics)
	//    metrics первым = считает все запросы включая 401
	srv := server.New(cfg, logger, apiHandler,
		middleware.MetricsMiddleware(),
		middleware.RequestLogger(logger),
		server.JWTAuthWithExclusions(jwtAuth.Middleware(), "/health/", "/metrics"),
	)

	// 7. Запуск сервера (блокирующий вызов с graceful shutdown)
	if err = srv.Run(); err != nil {
		logger.Error("Ошибка сервера", slog.String("error", err.Error()))
		os.Exit(1) //nolint:gocritic // exitAfterDefer: defer выполняется при нормальном завершении через graceful shutdown
	}

	logger.Info("Ingester Module остановлен")
}
