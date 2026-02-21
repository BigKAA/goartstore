// dephealth.go — интеграция с topologymetrics SDK для мониторинга зависимостей.
//
// Storage Element мониторит:
//   - Admin Module JWKS endpoint (HTTP GET, critical)
//
// Метрики доступны на /metrics вместе с остальными Prometheus-метриками:
//   - app_dependency_health — состояние зависимости (1 = ok, 0 = fail)
//   - app_dependency_latency_seconds — задержка проверки
//   - app_dependency_status — категория статуса
//   - app_dependency_status_detail — детальный статус
//
// Используется кастомный HTTP checker вместо dephealth/checks,
// чтобы избежать тяжёлых transitive зависимостей (PostgreSQL, Redis, gRPC и др.).
package service

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/BigKAA/topologymetrics/sdk-go/dephealth"
	"github.com/prometheus/client_golang/prometheus"
)

// httpChecker — кастомный HTTP health checker для topologymetrics.
// Выполняет HTTP GET к endpoint и проверяет код ответа (2xx = ok).
type httpChecker struct {
	client *http.Client
	path   string // URL path для проверки
}

// newHTTPChecker создаёт HTTP health checker.
func newHTTPChecker(timeout time.Duration, path string, skipTLSVerify bool) *httpChecker {
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: skipTLSVerify, //nolint:gosec // Dev-среда: self-signed сертификаты
		},
	}

	return &httpChecker{
		client: &http.Client{
			Timeout:   timeout,
			Transport: transport,
		},
		path: path,
	}
}

// Check выполняет HTTP GET к endpoint и проверяет ответ.
func (c *httpChecker) Check(ctx context.Context, ep dephealth.Endpoint) error {
	scheme := "http"
	if ep.Port == "443" || ep.Port == "8443" {
		scheme = "https"
	}

	// Если в labels есть scheme — используем его
	if s, ok := ep.Labels["scheme"]; ok {
		scheme = s
	}

	targetURL := fmt.Sprintf("%s://%s:%s%s", scheme, ep.Host, ep.Port, c.path)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return &dephealth.ClassifiedCheckError{
			Category: dephealth.StatusError,
			Detail:   "request_creation_failed",
			Cause:    err,
		}
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return &dephealth.ClassifiedCheckError{
			Category: dephealth.StatusConnectionError,
			Detail:   "connection_failed",
			Cause:    err,
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &dephealth.ClassifiedCheckError{
			Category: dephealth.StatusUnhealthy,
			Detail:   fmt.Sprintf("http_%d", resp.StatusCode),
			Cause:    fmt.Errorf("HTTP %d", resp.StatusCode),
		}
	}

	return nil
}

// Type возвращает тип зависимости.
func (c *httpChecker) Type() string {
	return "http"
}

// DephealthService — сервис мониторинга зависимостей через topologymetrics.
type DephealthService struct {
	dh     *dephealth.DepHealth
	logger *slog.Logger
}

// NewDephealthService создаёт сервис мониторинга зависимостей.
// Метрики регистрируются в глобальном Prometheus registry.
func NewDephealthService(
	storageID string,
	jwksURL string,
	checkInterval time.Duration,
	logger *slog.Logger,
) (*DephealthService, error) {
	return newDephealthService(storageID, jwksURL, checkInterval, logger)
}

// NewDephealthServiceWithRegisterer создаёт сервис с указанным Prometheus registerer.
// Используется в тестах для изоляции метрик.
func NewDephealthServiceWithRegisterer(
	storageID string,
	jwksURL string,
	checkInterval time.Duration,
	logger *slog.Logger,
	registerer prometheus.Registerer,
) (*DephealthService, error) {
	return newDephealthService(storageID, jwksURL, checkInterval, logger, dephealth.WithRegisterer(registerer))
}

// newDephealthService — внутренний конструктор.
func newDephealthService(
	storageID string,
	jwksURL string,
	checkInterval time.Duration,
	logger *slog.Logger,
	extraOpts ...dephealth.Option,
) (*DephealthService, error) {
	// Парсим JWKS URL для получения host, port, path
	parsedURL, err := url.Parse(jwksURL)
	if err != nil {
		return nil, fmt.Errorf("некорректный JWKS URL %q: %w", jwksURL, err)
	}

	host := parsedURL.Hostname()
	port := parsedURL.Port()
	if port == "" {
		if parsedURL.Scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}
	path := parsedURL.Path
	if path == "" {
		path = "/"
	}

	// Создаём HTTP checker
	checker := newHTTPChecker(5*time.Second, path, true)

	// Собираем опции
	opts := []dephealth.Option{
		dephealth.WithCheckInterval(checkInterval),
		dephealth.WithLogger(logger),
		// Зависимость: Admin Module JWKS (HTTP, critical)
		dephealth.AddDependency("admin-jwks", dephealth.TypeHTTP, checker,
			dephealth.FromParams(host, port),
			dephealth.Critical(true),
			dephealth.WithLabel("scheme", parsedURL.Scheme),
		),
	}
	opts = append(opts, extraOpts...)

	dh, err := dephealth.New(
		storageID,
		"storage-element",
		opts...,
	)
	if err != nil {
		return nil, err
	}

	return &DephealthService{
		dh:     dh,
		logger: logger.With(slog.String("component", "dephealth")),
	}, nil
}

// Start запускает периодическую проверку зависимостей.
func (ds *DephealthService) Start(ctx context.Context) error {
	ds.logger.Info("Мониторинг зависимостей запущен")
	return ds.dh.Start(ctx)
}

// Stop останавливает мониторинг зависимостей.
func (ds *DephealthService) Stop() {
	ds.dh.Stop()
	ds.logger.Info("Мониторинг зависимостей остановлен")
}

// Health возвращает текущее состояние зависимостей.
// Ключ — имя зависимости, значение — true если ok.
func (ds *DephealthService) Health() map[string]bool {
	return ds.dh.Health()
}
