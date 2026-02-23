// Пакет handlers — HTTP-обработчики Admin UI.
// Файл settings.go — обработчик страницы настроек (admin only):
// конфигурация Prometheus (URL, enabled, timeout), проверка подключения.
package handlers

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/bigkaa/goartstore/admin-module/internal/service"
	uimiddleware "github.com/bigkaa/goartstore/admin-module/internal/ui/middleware"
	"github.com/bigkaa/goartstore/admin-module/internal/ui/pages"
	"github.com/bigkaa/goartstore/admin-module/internal/ui/pages/partials"
	"github.com/bigkaa/goartstore/admin-module/internal/ui/prometheus"
)

// SettingsHandler — обработчик страницы настроек.
type SettingsHandler struct {
	settingsSvc *service.UISettingsService
	promClient  *prometheus.Client
	logger      *slog.Logger
}

// NewSettingsHandler создаёт новый SettingsHandler.
func NewSettingsHandler(
	settingsSvc *service.UISettingsService,
	promClient *prometheus.Client,
	logger *slog.Logger,
) *SettingsHandler {
	return &SettingsHandler{
		settingsSvc: settingsSvc,
		promClient:  promClient,
		logger:      logger.With(slog.String("component", "ui.settings")),
	}
}

// HandleSettings обрабатывает GET /admin/settings — страница настроек (admin only).
func (h *SettingsHandler) HandleSettings(w http.ResponseWriter, r *http.Request) {
	session := uimiddleware.SessionFromContext(r.Context())
	if session == nil {
		http.Redirect(w, r, "/admin/login", http.StatusFound)
		return
	}

	// Только admin может просматривать настройки
	if session.Role != "admin" {
		http.Redirect(w, r, "/admin/", http.StatusFound)
		return
	}

	ctx := r.Context()

	data := pages.SettingsData{
		Username: session.Username,
		Role:     session.Role,
		Prometheus: pages.PrometheusSettings{
			URL:             h.settingsSvc.GetPrometheusURL(ctx),
			Enabled:         h.settingsSvc.IsPrometheusEnabled(ctx),
			Timeout:         h.settingsSvc.GetPrometheusTimeout(ctx).String(),
			RetentionPeriod: h.getRetentionPeriod(ctx),
		},
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	if err := pages.Settings(data).Render(ctx, w); err != nil {
		h.logger.Error("Ошибка рендеринга Settings",
			slog.String("error", err.Error()),
			slog.String("username", session.Username),
		)
		http.Error(w, "Ошибка рендеринга страницы", http.StatusInternalServerError)
	}
}

// HandlePrometheusUpdate обрабатывает PUT /admin/partials/settings-prometheus.
// Сохраняет настройки Prometheus в БД.
func (h *SettingsHandler) HandlePrometheusUpdate(w http.ResponseWriter, r *http.Request) {
	session := uimiddleware.SessionFromContext(r.Context())
	if session == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	if session.Role != "admin" {
		h.renderAlert(w, r, "error", "Нет прав для изменения настроек")
		return
	}

	ctx := r.Context()

	if err := r.ParseForm(); err != nil {
		h.renderAlert(w, r, "error", "Ошибка разбора формы: "+err.Error())
		return
	}

	promURL := r.FormValue("prometheus_url")
	promEnabled := r.FormValue("prometheus_enabled")
	promTimeout := r.FormValue("prometheus_timeout")
	promRetention := r.FormValue("prometheus_retention_period")

	// Сохраняем каждую настройку
	settings := map[string]string{
		"prometheus.url":              promURL,
		"prometheus.enabled":          promEnabled,
		"prometheus.timeout":          promTimeout,
		"prometheus.retention_period": promRetention,
	}

	for key, value := range settings {
		if err := h.settingsSvc.Set(ctx, key, value, session.Username); err != nil {
			h.renderAlert(w, r, "error", "Ошибка сохранения "+key+": "+err.Error())
			return
		}
	}

	h.logger.Info("Настройки Prometheus обновлены",
		slog.String("updated_by", session.Username),
		slog.String("url", promURL),
		slog.String("enabled", promEnabled),
	)

	h.renderAlert(w, r, "success", "Настройки Prometheus сохранены")
}

// HandlePrometheusTest обрабатывает POST /admin/partials/settings-prometheus-test.
// Проверяет доступность Prometheus.
func (h *SettingsHandler) HandlePrometheusTest(w http.ResponseWriter, r *http.Request) {
	session := uimiddleware.SessionFromContext(r.Context())
	if session == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	if session.Role != "admin" {
		h.renderAlert(w, r, "error", "Нет прав для этого действия")
		return
	}

	ctx := r.Context()

	if h.promClient == nil {
		h.renderAlert(w, r, "error", "Prometheus-клиент не инициализирован")
		return
	}

	if h.promClient.IsAvailable(ctx) {
		h.renderAlert(w, r, "success", "Подключение к Prometheus успешно")
	} else {
		h.renderAlert(w, r, "error", "Prometheus недоступен. Проверьте URL и доступность сервера.")
	}
}

// getRetentionPeriod возвращает настроенный период хранения Prometheus.
func (h *SettingsHandler) getRetentionPeriod(ctx context.Context) string {
	setting, err := h.settingsSvc.Get(ctx, "prometheus.retention_period")
	if err != nil {
		return "7d"
	}
	return setting.Value
}

// renderAlert отрисовывает alert-компонент для HTMX-ответов.
func (h *SettingsHandler) renderAlert(w http.ResponseWriter, r *http.Request, variant, msg string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := partials.SettingsAlert(variant, msg).Render(r.Context(), w); err != nil {
		h.logger.Error("Ошибка рендеринга alert",
			slog.String("error", err.Error()),
		)
	}
}
