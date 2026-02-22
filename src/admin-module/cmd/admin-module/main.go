// Точка входа Admin Module — управляющий модуль системы Artsore v2.0.0.
// Загружает конфигурацию, подключается к PostgreSQL, применяет миграции,
// запускает HTTP-сервер с health endpoints.
package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/arturkryukov/artsore/admin-module/internal/api/handlers"
	"github.com/arturkryukov/artsore/admin-module/internal/config"
	"github.com/arturkryukov/artsore/admin-module/internal/database"
	"github.com/arturkryukov/artsore/admin-module/internal/server"
)

func main() {
	// 1. Загрузка конфигурации из переменных окружения
	cfg, err := config.Load()
	if err != nil {
		slog.Error("Ошибка загрузки конфигурации", slog.String("error", err.Error()))
		os.Exit(1)
	}

	// 2. Настройка логирования
	logger := config.SetupLogger(cfg)
	logger.Info("Admin Module запускается",
		slog.String("version", config.Version),
		slog.Int("port", cfg.Port),
	)

	// 3. Применение миграций БД
	logger.Info("Применение миграций БД...")
	if err := database.Migrate(cfg, logger); err != nil {
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

	// 5. Создание обработчиков
	// ReadinessChecker для PostgreSQL (Keycloak checker — Phase 3)
	pgChecker := database.NewReadinessChecker(pool)
	healthHandler := handlers.NewHealthHandler(pgChecker, nil)

	// Phase 2: StubHandler — все endpoints кроме health/metrics возвращают 501
	// (handlers будут заменены в Phase 4)
	stubHandler := handlers.NewStubHandler(healthHandler)

	// 6. Создание и запуск HTTP-сервера
	srv := server.New(cfg, logger, stubHandler)
	if err := srv.Run(); err != nil {
		logger.Error("Ошибка сервера", slog.String("error", err.Error()))
		os.Exit(1)
	}

	logger.Info("Admin Module остановлен")
}
