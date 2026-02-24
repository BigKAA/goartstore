// Пакет prometheus — HTTP-клиент для Prometheus Query API.
// Используется для получения исторических метрик latency и storage usage
// для отображения графиков на странице мониторинга.
//
// Конфигурация (URL, enabled, timeout) загружается из UISettingsService.
// Если Prometheus не настроен — все методы возвращают пустые результаты.
package prometheus

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/bigkaa/goartstore/admin-module/internal/service"
)

// Client — HTTP-клиент для Prometheus Query API.
type Client struct {
	httpClient  *http.Client
	settingsSvc *service.UISettingsService
	logger      *slog.Logger
}

// New создаёт новый Prometheus-клиент.
// settingsSvc используется для получения URL, enabled, timeout из БД.
// timeout — таймаут HTTP-запросов (AM_PROMETHEUS_CLIENT_TIMEOUT).
func New(settingsSvc *service.UISettingsService, timeout time.Duration, logger *slog.Logger) *Client {
	return &Client{
		httpClient:  &http.Client{Timeout: timeout},
		settingsSvc: settingsSvc,
		logger:      logger.With(slog.String("component", "ui.prometheus")),
	}
}

// --- Типы ответов Prometheus API --- //

// queryResponse — ответ Prometheus /api/v1/query и /api/v1/query_range.
type queryResponse struct {
	Status string    `json:"status"` // "success" или "error"
	Data   queryData `json:"data"`
}

// queryData — данные из ответа Prometheus.
type queryData struct {
	ResultType string        `json:"resultType"` // "matrix", "vector", "scalar", "string"
	Result     []queryResult `json:"result"`
}

// queryResult — один результат (серия).
type queryResult struct {
	Metric map[string]string `json:"metric"`
	Values [][]interface{}   `json:"values"` // [[timestamp, value], ...]  — для range query
	Value  []interface{}     `json:"value"`  // [timestamp, value]          — для instant query
}

// TimeSeriesPoint — точка на временном графике.
type TimeSeriesPoint struct {
	Timestamp time.Time `json:"timestamp"`
	Value     float64   `json:"value"`
}

// LatencyResult — результат запроса latency для одной зависимости.
type LatencyResult struct {
	Target string            `json:"target"` // Имя зависимости (postgresql, keycloak-jwks, SE name)
	Points []TimeSeriesPoint `json:"points"` // Точки графика
}

// IsAvailable проверяет доступность Prometheus.
// Возвращает true если Prometheus настроен, включён и отвечает на запросы.
func (c *Client) IsAvailable(ctx context.Context) bool {
	baseURL := c.settingsSvc.GetPrometheusURL(ctx)
	if baseURL == "" {
		return false
	}

	if !c.settingsSvc.IsPrometheusEnabled(ctx) {
		return false
	}

	timeout := c.settingsSvc.GetPrometheusTimeout(ctx)
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Проверяем /api/v1/status/build
	reqURL := baseURL + "/api/v1/status/build"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		c.logger.Debug("Ошибка создания запроса к Prometheus", slog.String("error", err.Error()))
		return false
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.logger.Debug("Prometheus недоступен", slog.String("error", err.Error()))
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK
}

// IsConfigured возвращает true если Prometheus URL настроен и включён.
// Не выполняет сетевой запрос — проверяет только настройки.
func (c *Client) IsConfigured(ctx context.Context) bool {
	return c.settingsSvc.GetPrometheusURL(ctx) != "" && c.settingsSvc.IsPrometheusEnabled(ctx)
}

// QueryLatency запрашивает метрики latency для указанной зависимости за период.
// Использует метрику app_dependency_latency_seconds из topologymetrics.
// target — имя зависимости (e.g. "postgresql", "keycloak-jwks").
// period — период запроса (e.g. 1h, 6h, 24h, 7d).
func (c *Client) QueryLatency(ctx context.Context, target string, period time.Duration) (*LatencyResult, error) {
	if !c.IsConfigured(ctx) {
		return &LatencyResult{Target: target}, nil
	}

	// Определяем шаг на основе периода
	step := c.calculateStep(period)

	query := fmt.Sprintf(`rate(app_dependency_latency_seconds_sum{target="%s"}[5m]) / rate(app_dependency_latency_seconds_count{target="%s"}[5m])`, target, target)

	results, err := c.queryRange(ctx, query, period, step)
	if err != nil {
		return nil, fmt.Errorf("запрос latency для %s: %w", target, err)
	}

	result := &LatencyResult{Target: target}
	if len(results) > 0 {
		result.Points = c.extractPoints(results[0])
	}

	return result, nil
}

// QueryAllLatencies запрашивает latency для всех зависимостей за период.
// Возвращает map[target][]TimeSeriesPoint.
func (c *Client) QueryAllLatencies(ctx context.Context, period time.Duration) ([]LatencyResult, error) {
	if !c.IsConfigured(ctx) {
		return nil, nil
	}

	step := c.calculateStep(period)

	query := `rate(app_dependency_latency_seconds_sum[5m]) / rate(app_dependency_latency_seconds_count[5m])`

	results, err := c.queryRange(ctx, query, period, step)
	if err != nil {
		return nil, fmt.Errorf("запрос всех latencies: %w", err)
	}

	var latencies []LatencyResult
	for _, r := range results {
		target := r.Metric["target"]
		if target == "" {
			continue
		}
		latencies = append(latencies, LatencyResult{
			Target: target,
			Points: c.extractPoints(r),
		})
	}

	return latencies, nil
}

// QueryStorageUsage запрашивает использование хранилища по SE за период.
// Возвращает map[se_name][]TimeSeriesPoint.
func (c *Client) QueryStorageUsage(ctx context.Context, period time.Duration) ([]LatencyResult, error) {
	if !c.IsConfigured(ctx) {
		return nil, nil
	}

	step := c.calculateStep(period)

	// Метрика использования хранилища SE (если доступна через /metrics)
	query := `admin_module_se_used_bytes`

	results, err := c.queryRange(ctx, query, period, step)
	if err != nil {
		// Метрика может не существовать — не считаем ошибкой
		c.logger.Debug("Метрика storage usage недоступна", slog.String("error", err.Error()))
		return nil, nil
	}

	var usage []LatencyResult
	for _, r := range results {
		seName := r.Metric["se_name"]
		if seName == "" {
			seName = r.Metric["se_id"]
		}
		if seName == "" {
			continue
		}
		usage = append(usage, LatencyResult{
			Target: seName,
			Points: c.extractPoints(r),
		})
	}

	return usage, nil
}

// --- Внутренние методы --- //

// queryRange выполняет запрос /api/v1/query_range к Prometheus.
func (c *Client) queryRange(ctx context.Context, query string, period time.Duration, step string) ([]queryResult, error) {
	baseURL := c.settingsSvc.GetPrometheusURL(ctx)
	if baseURL == "" {
		return nil, fmt.Errorf("Prometheus URL не настроен")
	}

	timeout := c.settingsSvc.GetPrometheusTimeout(ctx)
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	end := time.Now().UTC()
	start := end.Add(-period)

	params := url.Values{}
	params.Set("query", query)
	params.Set("start", start.Format(time.RFC3339))
	params.Set("end", end.Format(time.RFC3339))
	params.Set("step", step)

	reqURL := fmt.Sprintf("%s/api/v1/query_range?%s", baseURL, params.Encode())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("создание запроса: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("выполнение запроса: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("чтение ответа: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Prometheus вернул код %d: %s", resp.StatusCode, string(body))
	}

	var qr queryResponse
	if err := json.Unmarshal(body, &qr); err != nil {
		return nil, fmt.Errorf("парсинг ответа: %w", err)
	}

	if qr.Status != "success" {
		return nil, fmt.Errorf("Prometheus error: status=%s", qr.Status)
	}

	return qr.Data.Result, nil
}

// extractPoints извлекает точки из результата Prometheus range query.
func (c *Client) extractPoints(result queryResult) []TimeSeriesPoint {
	points := make([]TimeSeriesPoint, 0, len(result.Values))

	for _, v := range result.Values {
		if len(v) < 2 {
			continue
		}

		// v[0] — timestamp (float64), v[1] — value (string)
		ts, ok := v[0].(float64)
		if !ok {
			continue
		}

		valStr, ok := v[1].(string)
		if !ok {
			continue
		}

		var val float64
		if _, err := fmt.Sscanf(valStr, "%f", &val); err != nil {
			continue
		}

		points = append(points, TimeSeriesPoint{
			Timestamp: time.Unix(int64(ts), 0),
			Value:     val,
		})
	}

	return points
}

// calculateStep определяет шаг для range query на основе периода.
// Цель — получить ~100-200 точек для графика.
func (c *Client) calculateStep(period time.Duration) string {
	switch {
	case period <= time.Hour:
		return "30s"
	case period <= 6*time.Hour:
		return "2m"
	case period <= 24*time.Hour:
		return "10m"
	case period <= 7*24*time.Hour:
		return "1h"
	default:
		return "1h"
	}
}
