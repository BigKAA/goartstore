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
// Используется встроенный HTTP checker из dephealth SDK.
package service

import (
	"context"
	"log/slog"
	"time"

	"github.com/BigKAA/topologymetrics/sdk-go/dephealth"
	_ "github.com/BigKAA/topologymetrics/sdk-go/dephealth/checks" // Регистрация фабрик checker-ов (HTTP и др.)
	"github.com/prometheus/client_golang/prometheus"
)

// DephealthService — сервис мониторинга зависимостей через topologymetrics.
type DephealthService struct {
	dh     *dephealth.DepHealth
	logger *slog.Logger
}

// NewDephealthService создаёт сервис мониторинга зависимостей.
// Метрики регистрируются в глобальном Prometheus registry.
//
// Параметры:
//   - storageID — имя вершины графа текущего приложения (SE_STORAGE_ID)
//   - group — имя группы в метриках (SE_DEPHEALTH_GROUP)
//   - depName — имя зависимости / целевого сервиса (SE_DEPHEALTH_DEP_NAME)
//   - jwksURL — URL зависимости для проверки (SE_JWKS_URL)
//   - checkInterval — интервал проверки (SE_DEPHEALTH_CHECK_INTERVAL)
func NewDephealthService(
	storageID string,
	group string,
	depName string,
	jwksURL string,
	checkInterval time.Duration,
	logger *slog.Logger,
) (*DephealthService, error) {
	return newDephealthService(storageID, group, depName, jwksURL, checkInterval, logger)
}

// NewDephealthServiceWithRegisterer создаёт сервис с указанным Prometheus registerer.
// Используется в тестах для изоляции метрик.
func NewDephealthServiceWithRegisterer(
	storageID string,
	group string,
	depName string,
	jwksURL string,
	checkInterval time.Duration,
	logger *slog.Logger,
	registerer prometheus.Registerer,
) (*DephealthService, error) {
	return newDephealthService(storageID, group, depName, jwksURL, checkInterval, logger, dephealth.WithRegisterer(registerer))
}

// newDephealthService — внутренний конструктор.
func newDephealthService(
	storageID string,
	group string,
	depName string,
	jwksURL string,
	checkInterval time.Duration,
	logger *slog.Logger,
	extraOpts ...dephealth.Option,
) (*DephealthService, error) {
	// Собираем опции: встроенный HTTP checker с per-dependency интервалом
	opts := []dephealth.Option{
		dephealth.WithLogger(logger),
		dephealth.HTTP(depName,
			dephealth.FromURL(jwksURL),
			dephealth.CheckInterval(checkInterval),
			dephealth.Critical(true),
			dephealth.WithHTTPTLSSkipVerify(true), // Dev-среда: self-signed сертификаты
		),
	}
	opts = append(opts, extraOpts...)

	dh, err := dephealth.New(
		storageID,
		group,
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
