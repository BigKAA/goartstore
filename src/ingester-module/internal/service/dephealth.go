// dephealth.go — интеграция с topologymetrics SDK для мониторинга зависимостей.
//
// Ingester Module мониторит:
//   - Admin Module — HTTP checker к /health/ready (critical)
//
// В отличие от Query Module, IM stateless (нет PostgreSQL).
// SE endpoints не мониторятся — определяются на лету при каждом upload.
//
// Метрики доступны на /metrics вместе с остальными Prometheus-метриками:
//   - app_dependency_health — состояние зависимости (1 = ok, 0 = fail)
//   - app_dependency_latency_seconds — задержка проверки
//   - app_dependency_status — категория статуса
//   - app_dependency_status_detail — детальный статус
package service

import (
	"context"
	"log/slog"
	"net/url"
	"time"

	"github.com/BigKAA/topologymetrics/sdk-go/dephealth"
	_ "github.com/BigKAA/topologymetrics/sdk-go/dephealth/checks/httpcheck" // регистрация HTTP checker factory
	"github.com/prometheus/client_golang/prometheus"
)

// DephealthService — сервис мониторинга зависимостей через topologymetrics.
type DephealthService struct {
	dh     *dephealth.DepHealth
	logger *slog.Logger
}

// NewDephealthService создаёт сервис мониторинга зависимостей.
// Упрощённая версия QM dephealth: только 1 зависимость (Admin Module, без PostgreSQL).
//
// Параметры:
//   - serviceID — имя вершины графа текущего приложения (e.g. "ingester-module")
//   - group — имя группы в метриках (IM_DEPHEALTH_GROUP)
//   - adminModuleURL — URL Admin Module health endpoint
//   - checkInterval — интервал проверки зависимостей (IM_DEPHEALTH_CHECK_INTERVAL)
//   - isEntry — при true добавляет лейбл isentry=yes ко всем зависимостям (DEPHEALTH_ISENTRY)
func NewDephealthService(
	serviceID string,
	group string,
	adminModuleURL string,
	checkInterval time.Duration,
	isEntry bool,
	logger *slog.Logger,
) (*DephealthService, error) {
	return newDephealthService(serviceID, group, adminModuleURL, checkInterval, isEntry, logger)
}

// NewDephealthServiceWithRegisterer создаёт сервис с указанным Prometheus registerer.
// Используется в тестах для изоляции метрик.
func NewDephealthServiceWithRegisterer(
	serviceID string,
	group string,
	adminModuleURL string,
	checkInterval time.Duration,
	isEntry bool,
	logger *slog.Logger,
	registerer prometheus.Registerer,
) (*DephealthService, error) {
	return newDephealthService(serviceID, group, adminModuleURL, checkInterval, isEntry,
		logger, dephealth.WithRegisterer(registerer))
}

// newDephealthService — внутренний конструктор.
func newDephealthService(
	serviceID string,
	group string,
	adminModuleURL string,
	checkInterval time.Duration,
	isEntry bool,
	logger *slog.Logger,
	extraOpts ...dephealth.Option,
) (*DephealthService, error) {
	// Health path для Admin Module
	amHealthPath := "/health/ready"

	// Опции зависимости Admin Module
	amDepOpts := []dephealth.DependencyOption{
		dephealth.FromURL(adminModuleURL),
		dephealth.WithHTTPHealthPath(amHealthPath),
		dephealth.CheckInterval(checkInterval),
		dephealth.Critical(true),
	}
	if isEntry {
		amDepOpts = append(amDepOpts, dephealth.WithLabel("isentry", "yes"))
	}

	// Для Admin Module определяем TLS из URL
	if parsed, err := url.Parse(adminModuleURL); err == nil && parsed.Scheme == "https" {
		amDepOpts = append(amDepOpts, dephealth.WithHTTPTLSSkipVerify(false))
	}

	opts := make([]dephealth.Option, 0, 2+len(extraOpts))
	opts = append(opts,
		dephealth.WithLogger(logger),
		// Admin Module — HTTP checker к /health/ready (единственная зависимость IM)
		dephealth.HTTP("admin-module", amDepOpts...),
	)
	opts = append(opts, extraOpts...)

	dh, err := dephealth.New(serviceID, group, opts...)
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
	ds.logger.Info("Мониторинг зависимостей запущен (Admin Module)")
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
