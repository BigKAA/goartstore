// main.go — точка входа Ingester Module.
// Phase 1: минимальный сервер с health endpoints и stub upload.
// Phase 2: JWT, logging, metrics middleware.
// Phase 3: полный upload pipeline (Admin Module + SE клиенты).
package main

import (
	"log/slog"
	"os"

	"github.com/bigkaa/goartstore/ingester-module/internal/api/handlers"
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

	// 3. Health handler (stub readiness — полная реализация в Phase 3.7)
	healthHandler := handlers.NewHealthHandler()

	// 4. API handler (stub upload — полная реализация в Phase 3.5)
	apiHandler := handlers.NewAPIHandler(healthHandler, logger)

	// 5. HTTP-сервер (без middleware в Phase 1 — добавятся в Phase 2)
	srv := server.New(cfg, logger, apiHandler)

	// 6. Запуск сервера (блокирующий вызов с graceful shutdown)
	if err = srv.Run(); err != nil {
		logger.Error("Ошибка сервера", slog.String("error", err.Error()))
		os.Exit(1)
	}

	logger.Info("Ingester Module остановлен")
}
