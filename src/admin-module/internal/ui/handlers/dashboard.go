// Пакет handlers — HTTP-обработчики Admin UI.
package handlers

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/bigkaa/goartstore/admin-module/internal/repository"
	"github.com/bigkaa/goartstore/admin-module/internal/service"
	uimiddleware "github.com/bigkaa/goartstore/admin-module/internal/ui/middleware"
	"github.com/bigkaa/goartstore/admin-module/internal/ui/pages"
)

// DashboardHandler — обработчик страницы Dashboard.
type DashboardHandler struct {
	storageElemsSvc  *service.StorageElementService
	filesSvc         *service.FileRegistryService
	serviceAcctsSvc  *service.ServiceAccountService
	dephealthSvc     *service.DephealthService // может быть nil
	logger           *slog.Logger
}

// NewDashboardHandler создаёт новый DashboardHandler.
func NewDashboardHandler(
	storageElemsSvc *service.StorageElementService,
	filesSvc *service.FileRegistryService,
	serviceAcctsSvc *service.ServiceAccountService,
	dephealthSvc *service.DephealthService,
	logger *slog.Logger,
) *DashboardHandler {
	return &DashboardHandler{
		storageElemsSvc: storageElemsSvc,
		filesSvc:        filesSvc,
		serviceAcctsSvc: serviceAcctsSvc,
		dephealthSvc:    dephealthSvc,
		logger:          logger.With(slog.String("component", "ui.dashboard")),
	}
}

// HandleDashboard обрабатывает GET /admin/ — отображает страницу Dashboard.
// Собирает агрегированные метрики из сервисов для отображения карточек,
// списка SE и графиков.
func (h *DashboardHandler) HandleDashboard(w http.ResponseWriter, r *http.Request) {
	session := uimiddleware.SessionFromContext(r.Context())
	if session == nil {
		http.Redirect(w, r, "/admin/login", http.StatusFound)
		return
	}

	ctx := r.Context()
	data := h.collectDashboardData(ctx, session.Username, session.Role)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	if err := pages.Dashboard(data).Render(ctx, w); err != nil {
		h.logger.Error("Ошибка рендеринга Dashboard",
			slog.String("error", err.Error()),
			slog.String("username", session.Username),
		)
		http.Error(w, "Ошибка рендеринга страницы", http.StatusInternalServerError)
	}
}

// collectDashboardData собирает агрегированные данные из всех сервисов.
// При ошибках отдельных сервисов — логирует и отображает нулевые значения.
func (h *DashboardHandler) collectDashboardData(ctx context.Context, username, role string) pages.DashboardData {
	data := pages.DashboardData{
		Username: username,
		Role:     role,
	}

	// --- Storage Elements ---
	h.collectSEMetrics(ctx, &data)

	// --- Файлы ---
	h.collectFileMetrics(ctx, &data)

	// --- Service Accounts ---
	h.collectSAMetrics(ctx, &data)

	// --- Статусы зависимостей ---
	h.collectDepHealth(&data)

	return data
}

// collectSEMetrics собирает метрики Storage Elements: список SE, счётчики по статусам.
func (h *DashboardHandler) collectSEMetrics(ctx context.Context, data *pages.DashboardData) {
	// Получаем все SE (без фильтрации, лимит 1000 — достаточно для Dashboard)
	ses, total, err := h.storageElemsSvc.List(ctx, nil, nil, 1000, 0)
	if err != nil {
		h.logger.Error("Ошибка получения SE для Dashboard",
			slog.String("error", err.Error()),
		)
		return
	}

	data.SETotal = total

	// Агрегируем метрики по каждому SE
	for _, se := range ses {
		// Счётчики по статусам
		switch se.Status {
		case "online":
			data.SEOnline++
		case "offline":
			data.SEOffline++
		case "degraded":
			data.SEDegraded++
		case "maintenance":
			data.SEMaintenance++
		}

		// Суммарное хранилище
		data.StorageTotalBytes += se.CapacityBytes
		data.StorageUsedBytes += se.UsedBytes

		// Добавляем SE в список для таблицы и графиков
		data.StorageElements = append(data.StorageElements, pages.SEItem{
			ID:            se.ID,
			Name:          se.Name,
			Mode:          se.Mode,
			Status:        se.Status,
			CapacityBytes: se.CapacityBytes,
			UsedBytes:     se.UsedBytes,
		})
	}

	// Подсчитываем файлы для каждого SE
	h.collectFilesPerSE(ctx, data)
}

// collectFilesPerSE подсчитывает количество файлов для каждого SE в списке.
func (h *DashboardHandler) collectFilesPerSE(ctx context.Context, data *pages.DashboardData) {
	for i := range data.StorageElements {
		seID := data.StorageElements[i].ID
		activeStatus := "active"
		filters := repository.FileListFilters{
			StorageElementID: &seID,
			Status:           &activeStatus,
		}
		_, count, err := h.filesSvc.List(ctx, filters, 0, 0)
		if err != nil {
			h.logger.Warn("Ошибка подсчёта файлов для SE",
				slog.String("se_id", seID),
				slog.String("error", err.Error()),
			)
			continue
		}
		data.StorageElements[i].FileCount = count
	}
}

// collectFileMetrics собирает метрики файлов: всего, по статусу, по retention.
func (h *DashboardHandler) collectFileMetrics(ctx context.Context, data *pages.DashboardData) {
	// Общее число активных файлов
	activeStatus := "active"
	_, totalActive, err := h.filesSvc.List(ctx, repository.FileListFilters{Status: &activeStatus}, 0, 0)
	if err != nil {
		h.logger.Error("Ошибка подсчёта активных файлов",
			slog.String("error", err.Error()),
		)
		return
	}
	data.FilesTotal = totalActive

	// Файлы permanent
	permanentPolicy := "permanent"
	_, permCount, err := h.filesSvc.List(ctx, repository.FileListFilters{
		Status:          &activeStatus,
		RetentionPolicy: &permanentPolicy,
	}, 0, 0)
	if err != nil {
		h.logger.Warn("Ошибка подсчёта permanent файлов",
			slog.String("error", err.Error()),
		)
	} else {
		data.FilesPermanent = permCount
	}

	// Файлы temporary = total - permanent
	data.FilesTemporary = data.FilesTotal - data.FilesPermanent
}

// collectSAMetrics собирает метрики Service Accounts: всего, active, suspended.
func (h *DashboardHandler) collectSAMetrics(ctx context.Context, data *pages.DashboardData) {
	// Общее число SA
	_, total, err := h.serviceAcctsSvc.List(ctx, nil, 0, 0)
	if err != nil {
		h.logger.Error("Ошибка подсчёта SA",
			slog.String("error", err.Error()),
		)
		return
	}
	data.SATotal = total

	// Active SA
	activeStatus := "active"
	_, activeCount, err := h.serviceAcctsSvc.List(ctx, &activeStatus, 0, 0)
	if err != nil {
		h.logger.Warn("Ошибка подсчёта active SA",
			slog.String("error", err.Error()),
		)
	} else {
		data.SAActive = activeCount
	}

	// Suspended = total - active
	data.SASuspended = data.SATotal - data.SAActive
}

// collectDepHealth собирает статусы зависимостей (PostgreSQL, Keycloak).
func (h *DashboardHandler) collectDepHealth(data *pages.DashboardData) {
	if h.dephealthSvc == nil {
		// DephealthService не инициализирован — показываем как unavailable
		data.Dependencies = []pages.DependencyStatus{
			{Name: "PostgreSQL", Status: "unavailable"},
			{Name: "Keycloak", Status: "unavailable"},
		}
		return
	}

	health := h.dephealthSvc.Health()

	data.Dependencies = []pages.DependencyStatus{
		{
			Name:   "PostgreSQL",
			Status: depHealthStatus(health["postgresql"]),
		},
		{
			Name:   "Keycloak",
			Status: depHealthStatus(health["keycloak-jwks"]),
		},
	}
}

// depHealthStatus преобразует bool в строку статуса для UI.
func depHealthStatus(ok bool) string {
	if ok {
		return "online"
	}
	return "offline"
}

// FormatBytes форматирует байты для использования в шаблонах.
// Экспортируемая обёртка для переиспользования.
func FormatBytes(bytes int64) string {
	if bytes < 0 {
		bytes = 0
	}

	const (
		KB = 1024
		MB = 1024 * KB
		GB = 1024 * MB
		TB = 1024 * GB
	)

	switch {
	case bytes >= TB:
		return fmt.Sprintf("%.1f TB", float64(bytes)/float64(TB))
	case bytes >= GB:
		return fmt.Sprintf("%.1f GB", float64(bytes)/float64(GB))
	case bytes >= MB:
		return fmt.Sprintf("%.1f MB", float64(bytes)/float64(MB))
	case bytes >= KB:
		return fmt.Sprintf("%.1f KB", float64(bytes)/float64(KB))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}
