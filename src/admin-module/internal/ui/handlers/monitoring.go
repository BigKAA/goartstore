// Пакет handlers — HTTP-обработчики Admin UI.
// Файл monitoring.go — обработчик страницы мониторинга:
// здоровье зависимостей (real-time через SSE), состояние фоновых задач,
// алерты (проблемы), Prometheus-графики latency (опционально).
package handlers

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/bigkaa/goartstore/admin-module/internal/config"
	"github.com/bigkaa/goartstore/admin-module/internal/repository"
	"github.com/bigkaa/goartstore/admin-module/internal/service"
	uimiddleware "github.com/bigkaa/goartstore/admin-module/internal/ui/middleware"
	"github.com/bigkaa/goartstore/admin-module/internal/ui/pages"
	"github.com/bigkaa/goartstore/admin-module/internal/ui/pages/partials"
	"github.com/bigkaa/goartstore/admin-module/internal/ui/prometheus"
)

// MonitoringHandler — обработчик страницы мониторинга.
type MonitoringHandler struct {
	storageElemsSvc *service.StorageElementService
	filesSvc        *service.FileRegistryService
	dephealthSvc    *service.DephealthService // может быть nil
	syncStateRepo   repository.SyncStateRepository
	promClient      *prometheus.Client
	cfg             *config.Config
	logger          *slog.Logger
}

// NewMonitoringHandler создаёт новый MonitoringHandler.
func NewMonitoringHandler(
	storageElemsSvc *service.StorageElementService,
	filesSvc *service.FileRegistryService,
	dephealthSvc *service.DephealthService,
	syncStateRepo repository.SyncStateRepository,
	promClient *prometheus.Client,
	cfg *config.Config,
	logger *slog.Logger,
) *MonitoringHandler {
	return &MonitoringHandler{
		storageElemsSvc: storageElemsSvc,
		filesSvc:        filesSvc,
		dephealthSvc:    dephealthSvc,
		syncStateRepo:   syncStateRepo,
		promClient:      promClient,
		cfg:             cfg,
		logger:          logger.With(slog.String("component", "ui.monitoring")),
	}
}

// HandleMonitoring обрабатывает GET /admin/monitoring — страница мониторинга.
func (h *MonitoringHandler) HandleMonitoring(w http.ResponseWriter, r *http.Request) {
	session := uimiddleware.SessionFromContext(r.Context())
	if session == nil {
		http.Redirect(w, r, "/admin/login", http.StatusFound)
		return
	}

	ctx := r.Context()
	data := h.collectMonitoringData(ctx, session.Username, session.Role)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	if err := pages.Monitoring(data).Render(ctx, w); err != nil {
		h.logger.Error("Ошибка рендеринга Monitoring",
			slog.String("error", err.Error()),
			slog.String("username", session.Username),
		)
		http.Error(w, "Ошибка рендеринга страницы", http.StatusInternalServerError)
	}
}

// HandleChartsPartial обрабатывает GET /admin/partials/monitoring-charts — обновление графиков.
// Параметры: ?period=1h|6h|24h|7d
func (h *MonitoringHandler) HandleChartsPartial(w http.ResponseWriter, r *http.Request) {
	session := uimiddleware.SessionFromContext(r.Context())
	if session == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	ctx := r.Context()
	period := parsePeriod(r.URL.Query().Get("period"))

	chartData := h.collectChartData(ctx, period)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	if err := partials.MonitoringChartsBody(chartData).Render(ctx, w); err != nil {
		h.logger.Error("Ошибка рендеринга monitoring charts partial",
			slog.String("error", err.Error()),
		)
	}
}

// collectMonitoringData собирает все данные для страницы мониторинга.
func (h *MonitoringHandler) collectMonitoringData(ctx context.Context, username, role string) pages.MonitoringData {
	data := pages.MonitoringData{
		Username: username,
		Role:     role,
	}

	// --- Зависимости ---
	h.collectDepStatus(ctx, &data)

	// --- Storage Elements ---
	h.collectSEStatus(ctx, &data)

	// --- Фоновые задачи ---
	h.collectBackgroundTasks(ctx, &data)

	// --- Алерты ---
	h.collectAlerts(&data)

	// --- Prometheus ---
	data.PrometheusConfigured = h.promClient != nil && h.promClient.IsConfigured(ctx)

	// Начальные данные графиков (период 24h по умолчанию)
	if data.PrometheusConfigured {
		data.Charts = h.collectChartData(ctx, 24*time.Hour)
	}

	return data
}

// collectDepStatus собирает статусы зависимостей.
func (h *MonitoringHandler) collectDepStatus(ctx context.Context, data *pages.MonitoringData) {
	if h.dephealthSvc == nil {
		data.Dependencies = []pages.MonitoringDepStatus{
			{Name: "PostgreSQL", Status: "unavailable", Type: "database"},
			{Name: "Keycloak", Status: "unavailable", Type: "auth"},
		}
		return
	}

	health := h.dephealthSvc.Health()

	// Health() возвращает ключи формата "dependency:host:port"
	data.Dependencies = []pages.MonitoringDepStatus{
		{
			Name:   "PostgreSQL",
			Status: depHealthStatus(findHealthByPrefix(health, "postgresql")),
			Type:   "database",
		},
		{
			Name:   "Keycloak",
			Status: depHealthStatus(findHealthByPrefix(health, "keycloak-jwks")),
			Type:   "auth",
		},
	}
}

// collectSEStatus собирает статусы всех SE.
func (h *MonitoringHandler) collectSEStatus(ctx context.Context, data *pages.MonitoringData) {
	ses, _, err := h.storageElemsSvc.List(ctx, nil, nil, 1000, 0)
	if err != nil {
		h.logger.Error("Ошибка получения SE для мониторинга",
			slog.String("error", err.Error()),
		)
		return
	}

	for _, se := range ses {
		data.StorageElements = append(data.StorageElements, pages.MonitoringSEStatus{
			ID:            se.ID,
			Name:          se.Name,
			Status:        se.Status,
			Mode:          se.Mode,
			CapacityBytes: se.CapacityBytes,
			UsedBytes:     se.UsedBytes,
		})
	}
}

// collectBackgroundTasks собирает информацию о фоновых задачах.
func (h *MonitoringHandler) collectBackgroundTasks(ctx context.Context, data *pages.MonitoringData) {
	// Получаем состояние синхронизации
	syncState, err := h.syncStateRepo.Get(ctx)
	if err != nil {
		h.logger.Warn("Ошибка получения sync_state", slog.String("error", err.Error()))
	}

	// File Sync задача
	fileSync := pages.BackgroundTask{
		Name:     "Синхронизация файлов",
		Interval: h.cfg.SyncInterval.String(),
	}
	if syncState != nil && syncState.LastFileSyncAt != nil {
		fileSync.LastRun = syncState.LastFileSyncAt
		fileSync.Status = "ok"
	} else {
		fileSync.Status = "pending"
	}
	data.BackgroundTasks = append(data.BackgroundTasks, fileSync)

	// SA Sync задача
	saSync := pages.BackgroundTask{
		Name:     "Синхронизация SA",
		Interval: h.cfg.SASyncInterval.String(),
	}
	if syncState != nil && syncState.LastSASyncAt != nil {
		saSync.LastRun = syncState.LastSASyncAt
		saSync.Status = "ok"
	} else {
		saSync.Status = "pending"
	}
	data.BackgroundTasks = append(data.BackgroundTasks, saSync)

	// Dep Health Check задача
	depHealth := pages.BackgroundTask{
		Name:     "Проверка зависимостей",
		Interval: h.cfg.DephealthCheckInterval.String(),
	}
	if h.dephealthSvc != nil {
		depHealth.Status = "ok"
	} else {
		depHealth.Status = "unavailable"
	}
	data.BackgroundTasks = append(data.BackgroundTasks, depHealth)
}

// collectAlerts формирует список активных алертов на основе текущего состояния.
func (h *MonitoringHandler) collectAlerts(data *pages.MonitoringData) {
	// Проверяем зависимости
	for _, dep := range data.Dependencies {
		if dep.Status == "offline" {
			data.Alerts = append(data.Alerts, pages.MonitoringAlert{
				Severity: "error",
				Title:    dep.Name + " недоступен",
				Message:  "Зависимость " + dep.Name + " не отвечает на проверки здоровья",
			})
		}
	}

	// Проверяем SE
	for _, se := range data.StorageElements {
		if se.Status == "offline" {
			data.Alerts = append(data.Alerts, pages.MonitoringAlert{
				Severity: "error",
				Title:    "SE " + se.Name + " offline",
				Message:  "Storage Element " + se.Name + " недоступен",
			})
		}
		if se.Status == "degraded" {
			data.Alerts = append(data.Alerts, pages.MonitoringAlert{
				Severity: "warning",
				Title:    "SE " + se.Name + " degraded",
				Message:  "Storage Element " + se.Name + " работает в ограниченном режиме",
			})
		}

		// Проверяем заполненность SE (>80%)
		if se.CapacityBytes > 0 {
			usagePct := float64(se.UsedBytes) / float64(se.CapacityBytes) * 100
			if usagePct >= 80 {
				data.Alerts = append(data.Alerts, pages.MonitoringAlert{
					Severity: "warning",
					Title:    "SE " + se.Name + " заполнен",
					Message:  "Использование хранилища превышает 80%",
				})
			}
		}
	}
}

// collectChartData собирает данные для графиков из Prometheus.
func (h *MonitoringHandler) collectChartData(ctx context.Context, period time.Duration) pages.MonitoringChartData {
	chartData := pages.MonitoringChartData{
		Period: formatPeriod(period),
	}

	if h.promClient == nil || !h.promClient.IsConfigured(ctx) {
		return chartData
	}

	// Запрашиваем все latency
	latencies, err := h.promClient.QueryAllLatencies(ctx, period)
	if err != nil {
		h.logger.Warn("Ошибка запроса latencies из Prometheus",
			slog.String("error", err.Error()),
		)
		return chartData
	}

	// Конвертируем для графиков
	for _, lat := range latencies {
		series := pages.ChartSeries{
			Name:   lat.Target,
			Points: make([]pages.ChartPoint, 0, len(lat.Points)),
		}
		for _, p := range lat.Points {
			series.Points = append(series.Points, pages.ChartPoint{
				Timestamp: p.Timestamp.Unix(),
				Value:     p.Value * 1000, // Конвертируем секунды → миллисекунды
			})
		}
		chartData.LatencySeries = append(chartData.LatencySeries, series)
	}

	// Сериализуем для ApexCharts
	if len(chartData.LatencySeries) > 0 {
		b, _ := json.Marshal(chartData.LatencySeries)
		chartData.LatencyJSON = string(b)
	}

	return chartData
}

// parsePeriod парсит строковый период в time.Duration.
func parsePeriod(s string) time.Duration {
	switch s {
	case "1h":
		return time.Hour
	case "6h":
		return 6 * time.Hour
	case "24h":
		return 24 * time.Hour
	case "7d":
		return 7 * 24 * time.Hour
	default:
		return 24 * time.Hour // по умолчанию
	}
}

// formatPeriod форматирует duration в человекочитаемый вид.
func formatPeriod(d time.Duration) string {
	switch {
	case d <= time.Hour:
		return "1h"
	case d <= 6*time.Hour:
		return "6h"
	case d <= 24*time.Hour:
		return "24h"
	case d <= 7*24*time.Hour:
		return "7d"
	default:
		return "24h"
	}
}
