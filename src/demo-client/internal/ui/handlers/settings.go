package handlers

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/bigkaa/goartstore/demo-client/internal/config"
	"github.com/bigkaa/goartstore/demo-client/internal/service"
	"github.com/bigkaa/goartstore/demo-client/internal/ui/pages"
)

// SettingsHandler — обработчик страницы Settings.
type SettingsHandler struct {
	cfg    *config.Config
	svc    *service.DashboardService
	logger *slog.Logger
}

// NewSettingsHandler создаёт обработчик Settings.
func NewSettingsHandler(cfg *config.Config, svc *service.DashboardService, logger *slog.Logger) *SettingsHandler {
	return &SettingsHandler{
		cfg:    cfg,
		svc:    svc,
		logger: logger,
	}
}

// Page — GET /settings — рендер страницы Settings.
func (h *SettingsHandler) Page(w http.ResponseWriter, r *http.Request) {
	health := h.svc.GetHealth()

	err := pages.Settings(pages.SettingsPageData{
		GatewayURL: h.cfg.GatewayURL,
		TokenURL:   h.cfg.TokenURL,
		ClientID:   h.cfg.ClientID,
		Scopes:     strings.Join(h.cfg.Scopes, " "),
		Health:     health,
		Version:    config.Version,
	}).Render(r.Context(), w)
	if err != nil {
		h.logger.Error("ошибка рендера Settings", "error", err)
		http.Error(w, "render error", http.StatusInternalServerError)
		return
	}
}

// HealthPartial — GET /partials/settings/health — partial обновление health (HTMX).
func (h *SettingsHandler) HealthPartial(w http.ResponseWriter, r *http.Request) {
	health := h.svc.GetHealth()

	err := pages.SettingsHealthPartial(health).Render(r.Context(), w)
	if err != nil {
		h.logger.Error("ошибка рендера settings health partial", "error", err)
		http.Error(w, "render error", http.StatusInternalServerError)
		return
	}
}
