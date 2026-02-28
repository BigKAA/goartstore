// Пакет server — HTTP-сервер Query Module с graceful shutdown.
// Без TLS — HTTP внутри кластера, TLS termination на API Gateway.
package server

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/go-chi/chi/v5"

	"github.com/bigkaa/goartstore/query-module/internal/api/generated"
	"github.com/bigkaa/goartstore/query-module/internal/config"
)

// Server — HTTP-сервер Query Module.
type Server struct {
	httpServer *http.Server
	logger     *slog.Logger
	cfg        *config.Config
}

// New создаёт новый HTTP-сервер с настроенными routes и middleware.
// handler — реализация generated.ServerInterface (APIHandler).
// middlewares — дополнительные middleware (metrics, logging, JWT), добавляются в порядке переданного среза.
func New(cfg *config.Config, logger *slog.Logger, handler generated.ServerInterface, middlewares ...func(http.Handler) http.Handler) *Server {
	router := chi.NewRouter()

	// Применяем переданные middleware
	for _, mw := range middlewares {
		router.Use(mw)
	}

	// Все API маршруты через HandlerFromMux (oapi-codegen chi-server).
	generated.HandlerFromMux(handler, router)

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Port),
		Handler:      router,
		ReadTimeout:  cfg.HTTPReadTimeout,
		WriteTimeout: cfg.HTTPWriteTimeout,
		IdleTimeout:  cfg.HTTPIdleTimeout,
	}

	return &Server{
		httpServer: srv,
		logger:     logger,
		cfg:        cfg,
	}
}

// JWTAuthWithExclusions оборачивает middleware, пропуская указанные пути.
// Запросы к путям, начинающимся с любого из excludePrefixes, проходят без middleware.
func JWTAuthWithExclusions(mw func(http.Handler) http.Handler, excludePrefixes ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Проверяем, начинается ли путь с исключённого префикса
			for _, prefix := range excludePrefixes {
				if strings.HasPrefix(r.URL.Path, prefix) {
					next.ServeHTTP(w, r)
					return
				}
			}

			// Применяем middleware
			mw(next).ServeHTTP(w, r)
		})
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
