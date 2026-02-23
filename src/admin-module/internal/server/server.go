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
	"strings"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/arturkryukov/artstore/admin-module/internal/api/generated"
	"github.com/arturkryukov/artstore/admin-module/internal/api/middleware"
	"github.com/arturkryukov/artstore/admin-module/internal/config"
	uihandlers "github.com/arturkryukov/artstore/admin-module/internal/ui/handlers"
	uimiddleware "github.com/arturkryukov/artstore/admin-module/internal/ui/middleware"
	"github.com/arturkryukov/artstore/admin-module/internal/ui/static"
)

// Server — HTTP-сервер Admin Module.
type Server struct {
	httpServer *http.Server
	logger     *slog.Logger
	cfg        *config.Config
}

// UIComponents — компоненты Admin UI для регистрации маршрутов.
// Nil означает, что UI отключён (AM_UI_ENABLED=false).
type UIComponents struct {
	// AuthHandler — обработчики login/callback/logout.
	AuthHandler *uihandlers.AuthHandler
	// AuthMiddleware — middleware проверки UI-сессии.
	AuthMiddleware *uimiddleware.UIAuth
	// DashboardHandler — обработчик страницы Dashboard.
	DashboardHandler *uihandlers.DashboardHandler
	// StorageElementsHandler — обработчик страниц Storage Elements.
	StorageElementsHandler *uihandlers.StorageElementsHandler
}

// New создаёт новый HTTP-сервер с настроенными routes и middleware.
// handler — реализация generated.ServerInterface (APIHandler).
// jwtAuth — JWT middleware (может быть nil для тестирования без auth).
// uiComponents — компоненты Admin UI (nil если UI отключён).
func New(cfg *config.Config, logger *slog.Logger, handler generated.ServerInterface, jwtAuth *middleware.JWTAuth, uiComponents *UIComponents) *Server {
	router := chi.NewRouter()

	// Глобальные middleware (применяются ко ВСЕМ маршрутам)
	router.Use(middleware.MetricsMiddleware())
	router.Use(middleware.RequestLogger(logger))

	// JWT middleware с исключениями для публичных endpoints и UI.
	// Health и metrics проверяются Kubernetes напрямую, без API Gateway.
	// /admin/* и /static/* используют cookie-based auth (не JWT Bearer).
	if jwtAuth != nil {
		router.Use(jwtAuthWithExclusions(jwtAuth, "/health/", "/metrics", "/admin", "/static/"))
	}

	// Все API маршруты через HandlerFromMux (oapi-codegen chi-server).
	generated.HandlerFromMux(handler, router)

	// --- Admin UI маршруты ---
	if uiComponents != nil {
		registerUIRoutes(router, uiComponents, logger)
	}

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

// registerUIRoutes регистрирует маршруты Admin UI.
func registerUIRoutes(router chi.Router, ui *UIComponents, logger *slog.Logger) {
	// Статические файлы (CSS, JS) — без аутентификации
	staticFS := static.FileSystem()
	router.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(staticFS)))

	// Auth endpoints — без UI auth middleware (публичные)
	router.Get("/admin/login", ui.AuthHandler.HandleLogin)
	router.Get("/admin/callback", ui.AuthHandler.HandleCallback)
	router.Post("/admin/logout", ui.AuthHandler.HandleLogout)

	// Защищённые UI маршруты — с UI auth middleware
	router.Route("/admin", func(r chi.Router) {
		r.Use(ui.AuthMiddleware.Middleware())

		// Dashboard (главная страница Admin UI)
		r.Get("/", ui.DashboardHandler.HandleDashboard)

		// --- Storage Elements ---
		if ui.StorageElementsHandler != nil {
			se := ui.StorageElementsHandler
			r.Get("/storage-elements", se.HandleList)
			r.Get("/storage-elements/{id}", se.HandleDetail)

			// HTMX partials для SE
			r.Get("/partials/se-table", se.HandleTablePartial)
			r.Post("/partials/se-discover", se.HandleDiscover)
			r.Post("/partials/se-register", se.HandleRegister)
			r.Get("/partials/se-edit-form/{id}", se.HandleEditForm)
			r.Put("/partials/se-edit/{id}", se.HandleEdit)
			r.Delete("/partials/se-delete/{id}", se.HandleDelete)
			r.Post("/partials/se-sync/{id}", se.HandleSync)
			r.Post("/partials/se-sync-all", se.HandleSyncAll)
			r.Get("/partials/se-files/{id}", se.HandleFilesPartial)
		}
	})

	logger.Info("Admin UI маршруты зарегистрированы",
		slog.String("static", "/static/*"),
		slog.String("auth", "/admin/login, /admin/callback, /admin/logout"),
		slog.String("protected", "/admin/*"),
	)
}

// jwtAuthWithExclusions оборачивает JWTAuth.Middleware(), пропуская указанные пути.
// Запросы к путям, начинающимся с любого из excludePrefixes, проходят без JWT.
func jwtAuthWithExclusions(jwtAuth *middleware.JWTAuth, excludePrefixes ...string) func(http.Handler) http.Handler {
	jwtMiddleware := jwtAuth.Middleware()

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Проверяем, начинается ли путь с исключённого префикса
			for _, prefix := range excludePrefixes {
				if strings.HasPrefix(r.URL.Path, prefix) {
					next.ServeHTTP(w, r)
					return
				}
			}

			// Применяем JWT middleware
			jwtMiddleware(next).ServeHTTP(w, r)
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
