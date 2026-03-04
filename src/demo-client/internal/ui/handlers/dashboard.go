// Package handlers — HTTP-обработчики UI страниц Demo Client.
// Каждый handler рендерит templ-шаблон с данными из Service Layer.
package handlers

import (
	"log/slog"
	"net/http"

	"github.com/bigkaa/goartstore/demo-client/internal/service"
	"github.com/bigkaa/goartstore/demo-client/internal/ui/i18n"
	"github.com/bigkaa/goartstore/demo-client/internal/ui/pages"
)

// DashboardHandler — обработчик страницы Dashboard.
type DashboardHandler struct {
	svc    *service.DashboardService
	logger *slog.Logger
}

// NewDashboardHandler создаёт новый обработчик Dashboard.
func NewDashboardHandler(svc *service.DashboardService, logger *slog.Logger) *DashboardHandler {
	return &DashboardHandler{
		svc:    svc,
		logger: logger,
	}
}

// Page — GET / — рендер полной страницы Dashboard.
func (h *DashboardHandler) Page(w http.ResponseWriter, r *http.Request) {
	data := h.svc.GetDashboardData()

	err := pages.Dashboard(pages.DashboardPageData{
		Health:         data.Health,
		Token:          data.Token,
		Stats:          data.Stats,
		RecentActivity: data.RecentActivity,
		Lang:           i18n.LangFromContext(r.Context()),
	}).Render(r.Context(), w)
	if err != nil {
		h.logger.Error("ошибка рендера Dashboard", "error", err)
		http.Error(w, "render error", http.StatusInternalServerError)
		return
	}
}

// HealthPartial — GET /partials/health — partial обновление health статусов (HTMX).
func (h *DashboardHandler) HealthPartial(w http.ResponseWriter, r *http.Request) {
	health := h.svc.GetHealth()

	err := pages.HealthStatusPartial(health).Render(r.Context(), w)
	if err != nil {
		h.logger.Error("ошибка рендера health partial", "error", err)
		http.Error(w, "render error", http.StatusInternalServerError)
		return
	}
}

// TokenPartial — GET /partials/token — partial обновление токен статуса (HTMX).
func (h *DashboardHandler) TokenPartial(w http.ResponseWriter, r *http.Request) {
	tokenInfo := h.svc.GetTokenInfo()

	err := pages.TokenStatusPartial(tokenInfo).Render(r.Context(), w)
	if err != nil {
		h.logger.Error("ошибка рендера token partial", "error", err)
		http.Error(w, "render error", http.StatusInternalServerError)
		return
	}
}
