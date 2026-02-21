// Точка входа Storage Element — модуля физического хранения файлов.
package main

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/arturkryukov/artsore/storage-element/internal/api/handlers"
	"github.com/arturkryukov/artsore/storage-element/internal/config"
	"github.com/arturkryukov/artsore/storage-element/internal/server"
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

	// Создание обработчиков
	stubHandler := handlers.NewStubHandler()
	healthHandler := handlers.NewHealthHandler()
	metricsHandler := server.NewMetricsHandler()

	// Составной обработчик: health + metrics — реальные, остальное — заглушки
	compositeHandler := server.NewCompositeHandler(stubHandler, healthHandler, metricsHandler)

	// Создание и запуск HTTP-сервера
	srv := server.New(cfg, logger, compositeHandler)

	if err := srv.Run(); err != nil {
		logger.Error("Ошибка сервера", slog.String("error", err.Error()))
		os.Exit(1)
	}

	logger.Info("Storage Element остановлен")
}
