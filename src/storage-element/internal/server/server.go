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

	"github.com/arturkryukov/artsore/storage-element/internal/api/generated"
	"github.com/arturkryukov/artsore/storage-element/internal/api/middleware"
	"github.com/arturkryukov/artsore/storage-element/internal/config"
)

// Server — HTTP-сервер Storage Element.
type Server struct {
	httpServer *http.Server
	logger     *slog.Logger
	cfg        *config.Config
}

// New создаёт новый HTTP-сервер с настроенными routes и middleware.
// handler — реализация generated.ServerInterface (stub или реальные handlers).
func New(cfg *config.Config, logger *slog.Logger, handler generated.ServerInterface) *Server {
	router := chi.NewRouter()

	// Middleware
	router.Use(middleware.RequestLogger(logger))

	// Prometheus /metrics — монтируем отдельно, чтобы не конфликтовать
	// с сгенерированными маршрутами (GetMetrics из ServerInterface).
	// Сгенерированный wrapper вызовет handler.GetMetrics, поэтому мы
	// перенаправим его в реальный promhttp.Handler через отдельный handler.
	//
	// Монтируем все endpoints из ServerInterface через сгенерированный роутер.
	generated.HandlerFromMux(handler, router)

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
	}
}

// MetricsHandler — обработчик для /metrics, делегирующий в Prometheus.
// Используется для замены stub.GetMetrics на реальный promhttp.Handler.
type MetricsHandler struct {
	promHandler http.Handler
}

// NewMetricsHandler создаёт обработчик Prometheus метрик.
func NewMetricsHandler() *MetricsHandler {
	return &MetricsHandler{
		promHandler: promhttp.Handler(),
	}
}

// GetMetrics реализует endpoint /metrics.
func (m *MetricsHandler) GetMetrics(w http.ResponseWriter, r *http.Request) {
	m.promHandler.ServeHTTP(w, r)
}

// CompositeHandler — составной обработчик, объединяющий несколько
// частичных реализаций ServerInterface.
type CompositeHandler struct {
	stub    generated.ServerInterface
	health  HealthProvider
	metrics *MetricsHandler
}

// HealthProvider — интерфейс для health endpoints.
type HealthProvider interface {
	HealthLive(w http.ResponseWriter, r *http.Request)
	HealthReady(w http.ResponseWriter, r *http.Request)
}

// NewCompositeHandler создаёт составной обработчик, объединяющий
// заглушки с реальными health и metrics handlers.
func NewCompositeHandler(stub generated.ServerInterface, health HealthProvider, metrics *MetricsHandler) *CompositeHandler {
	return &CompositeHandler{
		stub:    stub,
		health:  health,
		metrics: metrics,
	}
}

// --- Делегирование в stub для ещё не реализованных endpoints ---

func (c *CompositeHandler) ListFiles(w http.ResponseWriter, r *http.Request, params generated.ListFilesParams) {
	c.stub.ListFiles(w, r, params)
}

func (c *CompositeHandler) UploadFile(w http.ResponseWriter, r *http.Request) {
	c.stub.UploadFile(w, r)
}

func (c *CompositeHandler) DeleteFile(w http.ResponseWriter, r *http.Request, fileId generated.FileId) {
	c.stub.DeleteFile(w, r, fileId)
}

func (c *CompositeHandler) GetFileMetadata(w http.ResponseWriter, r *http.Request, fileId generated.FileId) {
	c.stub.GetFileMetadata(w, r, fileId)
}

func (c *CompositeHandler) UpdateFileMetadata(w http.ResponseWriter, r *http.Request, fileId generated.FileId) {
	c.stub.UpdateFileMetadata(w, r, fileId)
}

func (c *CompositeHandler) DownloadFile(w http.ResponseWriter, r *http.Request, fileId generated.FileId, params generated.DownloadFileParams) {
	c.stub.DownloadFile(w, r, fileId, params)
}

func (c *CompositeHandler) GetStorageInfo(w http.ResponseWriter, r *http.Request) {
	c.stub.GetStorageInfo(w, r)
}

func (c *CompositeHandler) Reconcile(w http.ResponseWriter, r *http.Request) {
	c.stub.Reconcile(w, r)
}

func (c *CompositeHandler) TransitionMode(w http.ResponseWriter, r *http.Request) {
	c.stub.TransitionMode(w, r)
}

// --- Реальные реализации ---

func (c *CompositeHandler) HealthLive(w http.ResponseWriter, r *http.Request) {
	c.health.HealthLive(w, r)
}

func (c *CompositeHandler) HealthReady(w http.ResponseWriter, r *http.Request) {
	c.health.HealthReady(w, r)
}

func (c *CompositeHandler) GetMetrics(w http.ResponseWriter, r *http.Request) {
	c.metrics.GetMetrics(w, r)
}

// Проверка на этапе компиляции
var _ generated.ServerInterface = (*CompositeHandler)(nil)

// Run запускает сервер и ожидает сигнала завершения (SIGINT, SIGTERM).
// При получении сигнала выполняется graceful shutdown с таймаутом 30 секунд.
func (s *Server) Run() error {
	// Канал для ошибок сервера
	errCh := make(chan error, 1)

	go func() {
		s.logger.Info("HTTP-сервер запущен",
			slog.String("addr", s.httpServer.Addr),
			slog.Bool("tls", s.cfg.TLSCert != ""),
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

	// Graceful shutdown с таймаутом 30 секунд
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	s.logger.Info("Выполняется graceful shutdown...")
	if err := s.httpServer.Shutdown(ctx); err != nil {
		return fmt.Errorf("ошибка при graceful shutdown: %w", err)
	}

	s.logger.Info("HTTP-сервер остановлен")
	return nil
}
