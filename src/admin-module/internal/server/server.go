// Пакет server — HTTP-сервер Admin Module с graceful shutdown.
// Без TLS — HTTP внутри кластера, TLS termination на API Gateway.
package server

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/arturkryukov/artsore/admin-module/internal/api/generated"
	"github.com/arturkryukov/artsore/admin-module/internal/api/middleware"
	"github.com/arturkryukov/artsore/admin-module/internal/config"
)

// Server — HTTP-сервер Admin Module.
type Server struct {
	httpServer *http.Server
	logger     *slog.Logger
	cfg        *config.Config
}

// New создаёт новый HTTP-сервер с настроенными routes и middleware.
// handler — реализация generated.ServerInterface (StubHandler на Phase 1,
// с HealthLive/HealthReady/GetMetrics переопределёнными через HealthHandler).
func New(cfg *config.Config, logger *slog.Logger, handler generated.ServerInterface) *Server {
	router := chi.NewRouter()

	// Глобальные middleware
	router.Use(middleware.MetricsMiddleware())
	router.Use(middleware.RequestLogger(logger))

	// Все маршруты через HandlerFromMux (без JWT на Phase 1).
	// Health/metrics обрабатываются через переопределённые методы в StubHandler.
	// JWT middleware будет добавлен в Phase 3.
	generated.HandlerFromMux(handler, router)

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Port),
		Handler:      router,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	return &Server{
		httpServer: srv,
		logger:     logger,
		cfg:        cfg,
	}
}

// Run запускает сервер и ожидает сигнала завершения (SIGINT, SIGTERM).
// При получении сигнала выполняется graceful shutdown.
func (s *Server) Run() error {
	// Канал для ошибок сервера
	errCh := make(chan error, 1)

	go func() {
		s.logger.Info("HTTP-сервер запущен",
			slog.String("addr", s.httpServer.Addr),
		)

		err := s.httpServer.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
		close(errCh)
	}()

	// Ожидание сигнала завершения
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-quit:
		s.logger.Info("Получен сигнал завершения", slog.String("signal", sig.String()))
	case err := <-errCh:
		if err != nil {
			return fmt.Errorf("ошибка HTTP-сервера: %w", err)
		}
	}

	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), s.cfg.ShutdownTimeout)
	defer cancel()

	s.logger.Info("Выполняется graceful shutdown...")
	if err := s.httpServer.Shutdown(ctx); err != nil {
		return fmt.Errorf("ошибка при graceful shutdown: %w", err)
	}

	s.logger.Info("HTTP-сервер остановлен")
	return nil
}
