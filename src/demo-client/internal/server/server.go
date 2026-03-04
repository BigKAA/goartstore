// Package server — HTTP-сервер Demo Client на chi router
// с graceful shutdown, health endpoints, Prometheus метрики,
// security middleware (CSP, CSRF), UI маршруты.
package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/bigkaa/goartstore/demo-client/internal/activity"
	"github.com/bigkaa/goartstore/demo-client/internal/config"
	"github.com/bigkaa/goartstore/demo-client/internal/service"
	"github.com/bigkaa/goartstore/demo-client/internal/token"
	"github.com/bigkaa/goartstore/demo-client/internal/ui/handlers"
	"github.com/bigkaa/goartstore/demo-client/internal/ui/i18n"
	"github.com/bigkaa/goartstore/demo-client/internal/ui/static"
)

// --- Prometheus метрики ---

var (
	httpRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "dc_http_requests_total",
			Help: "Количество HTTP-запросов",
		},
		[]string{"method", "path", "status"},
	)

	httpRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "dc_http_request_duration_seconds",
			Help:    "Длительность HTTP-запросов",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "path"},
	)
)

// ReadinessChecker — интерфейс для проверки готовности сервиса.
type ReadinessChecker interface {
	IsReady() bool
}

// Deps — зависимости сервера для UI-маршрутов.
type Deps struct {
	DashboardSvc *service.DashboardService
	UploadSvc    *service.UploadService
	SearchSvc    *service.SearchService
	DownloadSvc  *service.DownloadService
	ActivityLog  *activity.Log
}

// Server — HTTP-сервер Demo Client.
type Server struct {
	httpServer *http.Server
	logger     *slog.Logger
	cfg        *config.Config
}

// New — создание нового HTTP-сервера с chi router.
// Регистрирует системные и UI маршруты.
func New(cfg *config.Config, logger *slog.Logger, tokenMgr *token.Manager, deps Deps) *Server {
	router := chi.NewRouter()

	// --- Middleware stack ---
	// 1. Security headers (CSP)
	router.Use(securityHeadersMiddleware())
	// 2. CSRF-защита для POST-форм
	router.Use(csrfMiddleware(logger))
	// 3. Prometheus метрики
	router.Use(metricsMiddleware())
	// 4. Request logging
	router.Use(requestLoggingMiddleware(logger))
	// 5. i18n — определение языка из cookie/Accept-Language
	router.Use(i18n.Middleware())

	// --- System endpoints ---
	router.Get("/health/live", healthLiveHandler())
	router.Get("/health/ready", healthReadyHandler(tokenMgr))
	router.Handle("/metrics", promhttp.Handler())
	router.Get("/csrf-token", csrfTokenHandler())

	// --- Static files ---
	fileServer := http.FileServer(static.FileSystem())
	router.Handle("/static/*", http.StripPrefix("/static/", fileServer))

	// --- UI routes ---
	registerUIRoutes(router, cfg, deps, logger)

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

// registerUIRoutes — регистрация всех UI-маршрутов (Phase 4+).
func registerUIRoutes(router chi.Router, cfg *config.Config, deps Deps, logger *slog.Logger) {
	// Handlers
	dashboardH := handlers.NewDashboardHandler(deps.DashboardSvc, logger.With("handler", "dashboard"))
	activityH := handlers.NewActivityHandler(deps.ActivityLog, logger.With("handler", "activity"))
	settingsH := handlers.NewSettingsHandler(cfg, deps.DashboardSvc, logger.With("handler", "settings"))
	uploadH := handlers.NewUploadHandler(deps.UploadSvc, cfg, logger.With("handler", "upload"))
	searchH := handlers.NewSearchHandler(deps.SearchSvc, logger.With("handler", "search"))
	downloadH := handlers.NewDownloadHandler(deps.DownloadSvc, logger.With("handler", "download"))

	// --- Страницы ---
	router.Get("/", dashboardH.Page)
	router.Get("/upload", uploadH.Page)
	router.Get("/search", searchH.Page)
	router.Get("/settings", settingsH.Page)

	// --- Partials (HTMX) ---
	router.Get("/partials/health", dashboardH.HealthPartial)
	router.Get("/partials/token", dashboardH.TokenPartial)
	router.Get("/partials/settings/health", settingsH.HealthPartial)
	router.Get("/search/results", searchH.HandleSearch)
	router.Get("/files/{fileID}", searchH.FileDetail)

	// --- Downloads ---
	router.Get("/files/{fileID}/download", downloadH.Download)

	// --- SSE ---
	router.Get("/activity/stream", activityH.Stream)

	// --- Actions ---
	router.Post("/set-language", handlers.SetLanguage)
	router.Post("/upload/single", uploadH.HandleSingle)
	router.Post("/upload/batch", uploadH.HandleBatch)
}

// Router — доступ к chi router (для тестов или расширений).
func (s *Server) Router() chi.Router {
	return s.httpServer.Handler.(chi.Router)
}

// Run — запуск HTTP-сервера с graceful shutdown по SIGINT/SIGTERM.
func (s *Server) Run() error {
	errCh := make(chan error, 1)
	go func() {
		s.logger.Info("HTTP-сервер запущен",
			"addr", s.httpServer.Addr,
			"version", config.Version,
		)
		err := s.httpServer.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
		close(errCh)
	}()

	// Ожидание сигнала или ошибки
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-quit:
		s.logger.Info("получен сигнал, завершаем работу", "signal", sig.String())
	case err := <-errCh:
		if err != nil {
			return fmt.Errorf("ошибка HTTP-сервера: %w", err)
		}
	}

	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), s.cfg.ShutdownTimeout)
	defer cancel()

	if err := s.httpServer.Shutdown(ctx); err != nil {
		return fmt.Errorf("ошибка graceful shutdown: %w", err)
	}

	s.logger.Info("HTTP-сервер остановлен")
	return nil
}

// --- Health endpoints ---

// healthLiveHandler — GET /health/live — всегда 200 (liveness probe).
func healthLiveHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}
}

// healthReadyHandler — GET /health/ready — 200 если токен валиден.
func healthReadyHandler(tokenMgr ReadinessChecker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if !tokenMgr.IsReady() {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"status":"not_ready","reason":"token not valid"}`))
			return
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}
}

// --- Middleware ---

// securityHeadersMiddleware — CSP и другие security headers (NFR-02).
func securityHeadersMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Content Security Policy — разрешаем только свои ресурсы
			// unsafe-inline нужен для Alpine.js и HTMX inline-атрибутов
			// unsafe-eval нужен для Alpine.js
			w.Header().Set("Content-Security-Policy",
				"default-src 'self'; "+
					"script-src 'self' 'unsafe-inline' 'unsafe-eval'; "+
					"style-src 'self' 'unsafe-inline'; "+
					"img-src 'self' data:; "+
					"font-src 'self' https://fonts.gstatic.com; "+
					"connect-src 'self'; "+
					"frame-ancestors 'none'")
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("X-Frame-Options", "DENY")
			w.Header().Set("X-XSS-Protection", "0") // Отключён в пользу CSP
			w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")

			next.ServeHTTP(w, r)
		})
	}
}

// csrfMiddleware — CSRF-защита для POST/PUT/DELETE форм.
// Проверяет заголовок X-CSRF-Token или скрытое поле _csrf_token.
// Пропускает системные endpoints (/health/, /metrics) и /set-language.
func csrfMiddleware(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Пропускаем safe-методы и системные endpoints
			if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
				next.ServeHTTP(w, r)
				return
			}

			path := r.URL.Path
			if strings.HasPrefix(path, "/health/") || path == "/metrics" {
				next.ServeHTTP(w, r)
				return
			}

			// /set-language — безопасный POST без CSRF (устанавливает только cookie)
			if path == "/set-language" {
				next.ServeHTTP(w, r)
				return
			}

			// Проверяем CSRF-токен
			token := r.Header.Get("X-CSRF-Token")
			if token == "" {
				token = r.FormValue("_csrf_token")
			}

			// Получаем ожидаемый токен из cookie
			cookie, err := r.Cookie("_csrf_token")
			if err != nil || cookie.Value == "" || token == "" || cookie.Value != token {
				logger.Warn("CSRF-проверка не пройдена",
					"path", path,
					"method", r.Method,
					"remote_addr", r.RemoteAddr,
				)
				http.Error(w, "CSRF token invalid", http.StatusForbidden)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// csrfTokenHandler — GET /csrf-token — генерация CSRF-токена и установка cookie.
func csrfTokenHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tokenBytes := make([]byte, 32)
		if _, err := rand.Read(tokenBytes); err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		csrfToken := hex.EncodeToString(tokenBytes)

		http.SetCookie(w, &http.Cookie{
			Name:     "_csrf_token",
			Value:    csrfToken,
			Path:     "/",
			HttpOnly: true,
			Secure:   true,
			SameSite: http.SameSiteStrictMode,
		})

		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"token":"%s"}`, csrfToken)
	}
}

// metricsMiddleware — Prometheus метрики для HTTP-запросов.
func metricsMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

			next.ServeHTTP(ww, r)

			duration := time.Since(start).Seconds()
			path := normalizePath(r.URL.Path)

			httpRequestsTotal.WithLabelValues(
				r.Method,
				path,
				fmt.Sprintf("%d", ww.statusCode),
			).Inc()

			httpRequestDuration.WithLabelValues(
				r.Method,
				path,
			).Observe(duration)
		})
	}
}

// requestLoggingMiddleware — логирование HTTP-запросов через slog.
func requestLoggingMiddleware(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

			next.ServeHTTP(ww, r)

			// Пропускаем health/metrics/static в логах
			path := r.URL.Path
			if strings.HasPrefix(path, "/health/") || path == "/metrics" || strings.HasPrefix(path, "/static/") {
				return
			}

			logger.Info("HTTP-запрос",
				"method", r.Method,
				"path", path,
				"status", ww.statusCode,
				"duration_ms", time.Since(start).Milliseconds(),
				"remote_addr", r.RemoteAddr,
			)
		})
	}
}

// --- Вспомогательные типы ---

// responseWriter — обёртка для перехвата status code.
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// normalizePath — нормализация пути для метрик (снижение кардинальности).
// Заменяет UUID и числовые ID на заполнители.
func normalizePath(path string) string {
	parts := strings.Split(path, "/")
	for i, part := range parts {
		// UUID-паттерн (8-4-4-4-12 hex символов)
		if len(part) == 36 && strings.Count(part, "-") == 4 {
			parts[i] = ":id"
			continue
		}
		// Числовые ID
		if len(part) > 0 {
			allDigits := true
			for _, c := range part {
				if c < '0' || c > '9' {
					allDigits = false
					break
				}
			}
			if allDigits {
				parts[i] = ":id"
			}
		}
	}
	return strings.Join(parts, "/")
}
