// Пакет handlers — HTTP-обработчики Admin UI.
package handlers

import (
	"log/slog"
	"net/http"

	uimiddleware "github.com/arturkryukov/artstore/admin-module/internal/ui/middleware"
	"github.com/arturkryukov/artstore/admin-module/internal/ui/pages"
)

// DashboardHandler — обработчик страницы Dashboard.
type DashboardHandler struct {
	logger *slog.Logger
}

// NewDashboardHandler создаёт новый DashboardHandler.
func NewDashboardHandler(logger *slog.Logger) *DashboardHandler {
	return &DashboardHandler{
		logger: logger.With(slog.String("component", "ui.dashboard")),
	}
}

// HandleDashboard обрабатывает GET /admin/ — отображает страницу Dashboard.
func (h *DashboardHandler) HandleDashboard(w http.ResponseWriter, r *http.Request) {
	session := uimiddleware.SessionFromContext(r.Context())
	if session == nil {
		http.Redirect(w, r, "/admin/login", http.StatusFound)
		return
	}

	data := pages.DashboardData{
		Username: session.Username,
		Role:     session.Role,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	if err := pages.Dashboard(data).Render(r.Context(), w); err != nil {
		h.logger.Error("Ошибка рендеринга Dashboard",
			slog.String("error", err.Error()),
			slog.String("username", session.Username),
		)
		http.Error(w, "Ошибка рендеринга страницы", http.StatusInternalServerError)
	}
}
