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

	"github.com/bigkaa/goartstore/admin-module/internal/api/generated"
	"github.com/bigkaa/goartstore/admin-module/internal/api/middleware"
	"github.com/bigkaa/goartstore/admin-module/internal/config"
	uihandlers "github.com/bigkaa/goartstore/admin-module/internal/ui/handlers"
	"github.com/bigkaa/goartstore/admin-module/internal/ui/i18n"
	uimiddleware "github.com/bigkaa/goartstore/admin-module/internal/ui/middleware"
	"github.com/bigkaa/goartstore/admin-module/internal/ui/static"
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
	// FilesHandler — обработчик страниц файлового реестра.
	FilesHandler *uihandlers.FilesHandler
	// AccessHandler — обработчик страницы «Управление доступом».
	AccessHandler *uihandlers.AccessHandler
	// MonitoringHandler — обработчик страницы мониторинга.
	MonitoringHandler *uihandlers.MonitoringHandler
	// SettingsHandler — обработчик страницы настроек (admin only).
	SettingsHandler *uihandlers.SettingsHandler
	// EventsHandler — обработчик SSE endpoints для real-time обновлений.
	EventsHandler *uihandlers.EventsHandler
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

	// Переключение языка (доступно без аутентификации — устанавливает cookie)
	router.Post("/admin/set-language", uihandlers.HandleSetLanguage)

	// Защищённые UI маршруты — с UI auth middleware и i18n
	router.Route("/admin", func(r chi.Router) {
		r.Use(i18n.Middleware())
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

		// --- Файловый реестр ---
		if ui.FilesHandler != nil {
			f := ui.FilesHandler
			r.Get("/files", f.HandleList)

			// HTMX partials для файлов
			r.Get("/partials/file-table", f.HandleTablePartial)
			r.Get("/partials/file-detail/{id}", f.HandleDetailModal)
			r.Get("/partials/file-edit-form/{id}", f.HandleEditForm)
			r.Put("/partials/file-update/{id}", f.HandleUpdate)
			r.Delete("/partials/file-delete/{id}", f.HandleDelete)
		}

		// --- Управление доступом ---
		if ui.AccessHandler != nil {
			a := ui.AccessHandler
			r.Get("/access", a.HandleAccess)

			// HTMX partials для пользователей
			r.Get("/partials/users-table", a.HandleUsersTablePartial)
			r.Get("/partials/user-detail/{id}", a.HandleUserDetail)
			r.Post("/partials/user-role-override/{id}", a.HandleAddRoleOverride)
			r.Delete("/partials/user-role-override/{id}", a.HandleRemoveRoleOverride)

			// HTMX partials для SA
			r.Get("/partials/sa-table", a.HandleSATablePartial)
			r.Post("/partials/sa-create", a.HandleSACreate)
			r.Get("/partials/sa-edit-form/{id}", a.HandleSAEditForm)
			r.Put("/partials/sa-edit/{id}", a.HandleSAEdit)
			r.Delete("/partials/sa-delete/{id}", a.HandleSADelete)
			r.Post("/partials/sa-rotate/{id}", a.HandleSARotateSecret)
			r.Post("/partials/sa-sync", a.HandleSASync)
		}

		// --- Мониторинг ---
		if ui.MonitoringHandler != nil {
			m := ui.MonitoringHandler
			r.Get("/monitoring", m.HandleMonitoring)

			// HTMX partials для графиков мониторинга
			r.Get("/partials/monitoring-charts", m.HandleChartsPartial)
		}

		// --- Настройки (admin only) ---
		if ui.SettingsHandler != nil {
			s := ui.SettingsHandler
			r.Get("/settings", s.HandleSettings)

			// HTMX partials для настроек Prometheus
			r.Put("/partials/settings-prometheus", s.HandlePrometheusUpdate)
			r.Post("/partials/settings-prometheus-test", s.HandlePrometheusTest)
		}

		// --- SSE endpoints для real-time обновлений ---
		if ui.EventsHandler != nil {
			r.Get("/events/system-status", ui.EventsHandler.HandleSystemStatus)
		}
	})

	logger.Info("Admin UI маршруты зарегистрированы",
		slog.String("static", "/static/*"),
		slog.String("auth", "/admin/login, /admin/callback, /admin/logout"),
		slog.String("protected", "/admin/*, /admin/access, /admin/monitoring, /admin/settings, /admin/events/*"),
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
