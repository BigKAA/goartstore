// Пакет server — HTTP-сервер Storage Element с TLS и graceful shutdown.
package server

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	apierrors "github.com/arturkryukov/artsore/storage-element/internal/api/errors"
	"github.com/arturkryukov/artsore/storage-element/internal/api/generated"
	"github.com/arturkryukov/artsore/storage-element/internal/api/middleware"
	"github.com/arturkryukov/artsore/storage-element/internal/config"
)

// ProxyMiddleware — интерфейс для middleware проксирования запросов (replicated mode).
type ProxyMiddleware interface {
	Middleware(next http.Handler) http.Handler
}

// Server — HTTP-сервер Storage Element.
type Server struct {
	httpServer *http.Server
	logger     *slog.Logger
	cfg        *config.Config
	jwtAuth    JWTAuthProvider
	proxy      ProxyMiddleware
}

// JWTAuthProvider — интерфейс для JWT middleware.
type JWTAuthProvider interface {
	Middleware() func(http.Handler) http.Handler
}

// New создаёт новый HTTP-сервер с настроенными routes и middleware.
// handler — реализация generated.ServerInterface с реальными handlers.
// jwtAuth — JWT middleware (nil для режима без аутентификации).
// proxy — middleware проксирования (nil для standalone mode).
func New(cfg *config.Config, logger *slog.Logger, handler generated.ServerInterface, jwtAuth JWTAuthProvider, proxy ProxyMiddleware) *Server {
	router := chi.NewRouter()

	// Глобальные middleware
	router.Use(middleware.MetricsMiddleware())
	router.Use(middleware.RequestLogger(logger))

	// Proxy middleware (replicated mode: follower → leader для write-запросов).
	// Ставится после logging/metrics, перед маршрутами.
	if proxy != nil {
		router.Use(proxy.Middleware)
	}

	// Определяем маршруты с JWT-аутентификацией.
	// Публичные endpoints (без auth): health, metrics, info.
	// Защищённые endpoints: files, mode, maintenance.
	if jwtAuth != nil {
		// Публичные маршруты — монтируем напрямую без JWT
		router.Get("/health/live", handler.HealthLive)
		router.Get("/health/ready", handler.HealthReady)
		router.Get("/metrics", handler.GetMetrics)
		router.Get("/api/v1/info", handler.GetStorageInfo)

		// Защищённые маршруты — JWT middleware
		router.Group(func(r chi.Router) {
			r.Use(jwtAuth.Middleware())

			// Files — files:read
			r.Group(func(rr chi.Router) {
				rr.Use(middleware.RequireScope("files:read"))
				// ListFiles и GetFileMetadata через wrapper
				rr.Get("/api/v1/files", func(w http.ResponseWriter, r *http.Request) {
					// Парсим параметры вручную для ListFiles
					siw := &generated.ServerInterfaceWrapper{Handler: handler, ErrorHandlerFunc: defaultErrorHandler}
					siw.ListFiles(w, r)
				})
				rr.Get("/api/v1/files/{file_id}", func(w http.ResponseWriter, r *http.Request) {
					siw := &generated.ServerInterfaceWrapper{Handler: handler, ErrorHandlerFunc: defaultErrorHandler}
					siw.GetFileMetadata(w, r)
				})
				rr.Get("/api/v1/files/{file_id}/download", func(w http.ResponseWriter, r *http.Request) {
					siw := &generated.ServerInterfaceWrapper{Handler: handler, ErrorHandlerFunc: defaultErrorHandler}
					siw.DownloadFile(w, r)
				})
			})

			// Files — files:write
			r.Group(func(rr chi.Router) {
				rr.Use(middleware.RequireScope("files:write"))
				rr.Post("/api/v1/files/upload", handler.UploadFile)
				rr.Patch("/api/v1/files/{file_id}", func(w http.ResponseWriter, r *http.Request) {
					siw := &generated.ServerInterfaceWrapper{Handler: handler, ErrorHandlerFunc: defaultErrorHandler}
					siw.UpdateFileMetadata(w, r)
				})
				rr.Delete("/api/v1/files/{file_id}", func(w http.ResponseWriter, r *http.Request) {
					siw := &generated.ServerInterfaceWrapper{Handler: handler, ErrorHandlerFunc: defaultErrorHandler}
					siw.DeleteFile(w, r)
				})
			})

			// Storage — storage:write
			r.Group(func(rr chi.Router) {
				rr.Use(middleware.RequireScope("storage:write"))
				rr.Post("/api/v1/mode/transition", handler.TransitionMode)
				rr.Post("/api/v1/maintenance/reconcile", handler.Reconcile)
			})
		})
	} else {
		// Без JWT — все маршруты открыты (для разработки/тестирования)
		generated.HandlerFromMux(handler, router)
	}

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Port),
		Handler:      router,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Настройка TLS
	if cfg.TLSCert != "" && cfg.TLSKey != "" {
		srv.TLSConfig = &tls.Config{
			MinVersion: tls.VersionTLS12,
		}
	}

	return &Server{
		httpServer: srv,
		logger:     logger,
		cfg:        cfg,
		jwtAuth:    jwtAuth,
		proxy:      proxy,
	}
}

// defaultErrorHandler — обработчик ошибок парсинга параметров из сгенерированного кода.
func defaultErrorHandler(w http.ResponseWriter, r *http.Request, err error) {
	apierrors.ValidationError(w, err.Error())
}

// MetricsHandler — обработчик для /metrics, делегирующий в Prometheus.
type MetricsHandler struct {
	promHandler http.Handler
}

// NewMetricsHandler создаёт обработчик Prometheus метрик.
func NewMetricsHandler() *MetricsHandler {
	return &MetricsHandler{
		promHandler: promhttp.Handler(),
	}
}

// ServeHTTP реализует http.Handler.
func (m *MetricsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	m.promHandler.ServeHTTP(w, r)
}

// Run запускает сервер и ожидает сигнала завершения (SIGINT, SIGTERM).
// При получении сигнала выполняется graceful shutdown с таймаутом 30 секунд.
func (s *Server) Run() error {
	// Канал для ошибок сервера
	errCh := make(chan error, 1)

	go func() {
		s.logger.Info("HTTP-сервер запущен",
			slog.String("addr", s.httpServer.Addr),
			slog.Bool("tls", s.cfg.TLSCert != ""),
			slog.Bool("jwt_auth", s.jwtAuth != nil),
		)

		var err error
		if s.cfg.TLSCert != "" && s.cfg.TLSKey != "" {
			err = s.httpServer.ListenAndServeTLS(s.cfg.TLSCert, s.cfg.TLSKey)
		} else {
			err = s.httpServer.ListenAndServe()
		}

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

	// Graceful shutdown с таймаутом из конфига (SE_SHUTDOWN_TIMEOUT, по умолчанию 5s).
	// Важно: таймаут должен быть меньше K8s terminationGracePeriodSeconds,
	// чтобы после остановки HTTP-сервера оставалось время для election.Stop()
	// (освобождение NFS flock) до SIGKILL.
	ctx, cancel := context.WithTimeout(context.Background(), s.cfg.ShutdownTimeout)
	defer cancel()

	s.logger.Info("Выполняется graceful shutdown...")
	if err := s.httpServer.Shutdown(ctx); err != nil {
		return fmt.Errorf("ошибка при graceful shutdown: %w", err)
	}

	s.logger.Info("HTTP-сервер остановлен")
	return nil
}
