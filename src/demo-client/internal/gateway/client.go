package gateway

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Prometheus метрики для Gateway запросов.
var (
	gatewayRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "dc_gateway_requests_total",
			Help: "Общее количество запросов к API Gateway",
		},
		[]string{"method", "path", "status"},
	)

	gatewayRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "dc_gateway_request_duration_seconds",
			Help:    "Время выполнения запросов к API Gateway",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "path"},
	)
)

// TokenProvider — функция для получения текущего токена авторизации.
type TokenProvider func() string

// Client — HTTP-клиент для взаимодействия с API Gateway artstore.
// Автоматически добавляет Authorization: Bearer заголовок ко всем запросам.
type Client struct {
	// httpClient — HTTP-клиент для обычных запросов (search, metadata, health).
	httpClient *http.Client
	// uploadClient — HTTP-клиент с увеличенным таймаутом для загрузки файлов.
	uploadClient *http.Client
	// baseURL — базовый URL API Gateway (например, https://artstore.kryukov.lan).
	baseURL string
	// tokenProvider — функция для получения текущего JWT токена.
	tokenProvider TokenProvider
	// logger — структурированный логгер.
	logger *slog.Logger
}

// ClientConfig — конфигурация для создания Gateway Client.
type ClientConfig struct {
	// BaseURL — базовый URL API Gateway (обязательный).
	BaseURL string
	// RequestTimeout — таймаут для обычных запросов.
	RequestTimeout time.Duration
	// UploadTimeout — таймаут для загрузки файлов.
	UploadTimeout time.Duration
	// HTTPClient — базовый HTTP-клиент (с TLS конфигурацией).
	// Если nil, используется http.DefaultClient.
	HTTPClient *http.Client
}

// NewClient создаёт новый Gateway Client.
func NewClient(cfg ClientConfig, tokenProvider TokenProvider, logger *slog.Logger) *Client {
	baseClient := cfg.HTTPClient
	if baseClient == nil {
		baseClient = http.DefaultClient
	}

	// Клиент для обычных запросов — с RequestTimeout.
	requestClient := &http.Client{
		Transport: baseClient.Transport,
		Timeout:   cfg.RequestTimeout,
	}

	// Клиент для загрузки — с увеличенным UploadTimeout.
	uploadClient := &http.Client{
		Transport: baseClient.Transport,
		Timeout:   cfg.UploadTimeout,
	}

	return &Client{
		httpClient:    requestClient,
		uploadClient:  uploadClient,
		baseURL:       cfg.BaseURL,
		tokenProvider: tokenProvider,
		logger:        logger,
	}
}

// doRequest выполняет HTTP-запрос с авто-инъекцией Bearer токена и сбором метрик.
// Возвращает *http.Response (вызывающий обязан закрыть Body) или ошибку.
func (c *Client) doRequest(req *http.Request, useUploadClient bool) (*http.Response, error) {
	// Инъекция Authorization header.
	token := c.tokenProvider()
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	// Выбор клиента в зависимости от типа операции.
	client := c.httpClient
	if useUploadClient {
		client = c.uploadClient
	}

	// Метрики: замер времени выполнения.
	start := time.Now()
	method := req.Method
	path := normalizePath(req.URL.Path)

	c.logger.Debug("отправка запроса к Gateway",
		"method", method,
		"url", req.URL.String(),
	)

	resp, err := client.Do(req) //nolint:gosec // G704: URL is constructed from validated config
	duration := time.Since(start).Seconds()

	if err != nil {
		gatewayRequestsTotal.WithLabelValues(method, path, "error").Inc()
		gatewayRequestDuration.WithLabelValues(method, path).Observe(duration)

		c.logger.Error("ошибка запроса к Gateway",
			"method", method,
			"url", req.URL.String(),
			"error", err,
			"duration_ms", time.Since(start).Milliseconds(),
		)
		return nil, fmt.Errorf("gateway request failed: %w", err)
	}

	statusStr := fmt.Sprintf("%d", resp.StatusCode)
	gatewayRequestsTotal.WithLabelValues(method, path, statusStr).Inc()
	gatewayRequestDuration.WithLabelValues(method, path).Observe(duration)

	c.logger.Debug("ответ от Gateway",
		"method", method,
		"url", req.URL.String(),
		"status", resp.StatusCode,
		"duration_ms", time.Since(start).Milliseconds(),
	)

	return resp, nil
}

// handleErrorResponse обрабатывает HTTP-ответ с кодом ошибки (>= 400).
// Парсит JSON тело ответа и маппит в типизированную Error.
func (c *Client) handleErrorResponse(resp *http.Response) error {
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // Ограничение 1 MB
	if err != nil {
		return mapHTTPError(resp.StatusCode, nil)
	}

	var apiErr ErrorResponse
	if err := json.Unmarshal(body, &apiErr); err != nil {
		// Не удалось распарсить JSON — возвращаем ошибку только по HTTP коду.
		return mapHTTPError(resp.StatusCode, nil)
	}

	return mapHTTPError(resp.StatusCode, &apiErr)
}

// normalizePath заменяет UUID и числовые ID в пути на ":id" для снижения кардинальности метрик.
func normalizePath(path string) string {
	// Простая нормализация: заменяем сегменты, похожие на UUID или числовые ID.
	parts := splitPath(path)
	for i, part := range parts {
		if isUUIDLike(part) || isNumericID(part) {
			parts[i] = ":id"
		}
	}
	return joinPath(parts)
}

// splitPath разбивает путь на сегменты.
func splitPath(path string) []string {
	var parts []string
	current := ""
	for _, c := range path {
		if c == '/' {
			if current != "" {
				parts = append(parts, current)
				current = ""
			}
		} else {
			current += string(c)
		}
	}
	if current != "" {
		parts = append(parts, current)
	}
	return parts
}

// joinPath собирает сегменты обратно в путь.
func joinPath(parts []string) string {
	result := "/"
	for i, p := range parts {
		result += p
		if i < len(parts)-1 {
			result += "/"
		}
	}
	return result
}

// isUUIDLike проверяет, похож ли сегмент на UUID (8-4-4-4-12 hex).
func isUUIDLike(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, c := range s {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if c != '-' {
				return false
			}
		} else if (c < '0' || c > '9') && (c < 'a' || c > 'f') && (c < 'A' || c > 'F') {
			return false
		}
	}
	return true
}

// isNumericID проверяет, является ли сегмент числовым идентификатором.
func isNumericID(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
