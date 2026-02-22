// Точка входа Admin Module — управляющий модуль системы Artsore v2.0.0.
// Загружает конфигурацию, настраивает логирование, запускает HTTP-сервер.
package main

import (
	"log/slog"
	"os"

	"github.com/arturkryukov/artsore/admin-module/internal/api/handlers"
	"github.com/arturkryukov/artsore/admin-module/internal/config"
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

	// 3. Создание обработчиков
	// Phase 1: HealthHandler — без реальных проверок PostgreSQL и Keycloak
	// (checkers будут добавлены в Phase 2+)
	healthHandler := handlers.NewHealthHandler(nil, nil)

	// Phase 1: StubHandler — все endpoints кроме health/metrics возвращают 501
	stubHandler := handlers.NewStubHandler(healthHandler)

	// 4. Создание и запуск HTTP-сервера
	srv := server.New(cfg, logger, stubHandler)
	if err := srv.Run(); err != nil {
		logger.Error("Ошибка сервера", slog.String("error", err.Error()))
		os.Exit(1)
	}

	logger.Info("Admin Module остановлен")
}
