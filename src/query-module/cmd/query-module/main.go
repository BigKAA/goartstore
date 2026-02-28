// main.go — точка входа Query Module.
// Phase 1: минимальная версия — config, logger, health endpoints, stub handlers.
// БД, auth, кэш и бизнес-логика добавляются в Phase 2-4.
package main

import (
	"log"
	"log/slog"

	"github.com/bigkaa/goartstore/query-module/internal/api/handlers"
	"github.com/bigkaa/goartstore/query-module/internal/config"
	"github.com/bigkaa/goartstore/query-module/internal/server"
)

func main() {
	// 1. Загрузка конфигурации из переменных окружения
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Ошибка загрузки конфигурации: %v", err)
	}

	// 2. Настройка логгера
	logger := config.SetupLogger(cfg)
	logger.Info("Query Module запускается",
		slog.String("version", config.Version),
		slog.Int("port", cfg.Port),
	)

	// 3. Health handler (без PostgreSQL checker — будет в Phase 2)
	healthHandler := handlers.NewHealthHandler(nil)

	// 4. API handler (stubs для бизнес-endpoints)
	apiHandler := handlers.NewAPIHandler(healthHandler, logger)

	// 5. HTTP-сервер (без JWT middleware — будет в Phase 2)
	srv := server.New(cfg, logger, apiHandler)

	// 6. Запуск сервера (блокирующий вызов с graceful shutdown)
	if err := srv.Run(); err != nil {
		logger.Error("Ошибка сервера", slog.String("error", err.Error()))
		log.Fatalf("Сервер завершился с ошибкой: %v", err)
	}

	logger.Info("Query Module остановлен")
}
