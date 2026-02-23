// dephealth.go — интеграция с topologymetrics SDK для мониторинга зависимостей.
//
// Admin Module мониторит две зависимости:
//   - PostgreSQL — SQL checker через существующий pgxpool (connection pool mode, critical)
//   - Keycloak — HTTP checker к JWKS endpoint (critical)
//
// Connection pool mode предпочтителен, т.к. отражает реальную способность сервиса
// работать с зависимостью и может обнаружить исчерпание пула соединений.
//
// Метрики доступны на /metrics вместе с остальными Prometheus-метриками:
//   - app_dependency_health — состояние зависимости (1 = ok, 0 = fail)
//   - app_dependency_latency_seconds — задержка проверки
//   - app_dependency_status — категория статуса
//   - app_dependency_status_detail — детальный статус
package service

import (
	"context"
	"database/sql"
	"log/slog"
	"net/url"
	"time"

	"github.com/BigKAA/topologymetrics/sdk-go/dephealth"
	_ "github.com/BigKAA/topologymetrics/sdk-go/dephealth/checks/httpcheck" // HTTP checker для Keycloak
	"github.com/BigKAA/topologymetrics/sdk-go/dephealth/checks/pgcheck"     // PostgreSQL checker (pool mode)
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
// Использует connection pool mode для PostgreSQL: проверка выполняется
// через существующий *sql.DB (адаптер pgxpool), что позволяет обнаружить
// исчерпание пула соединений и отражает реальную способность сервиса
// работать с базой данных.
//
// Параметры:
//   - serviceID — имя вершины графа текущего приложения (e.g. "admin-module")
//   - group — имя группы в метриках (AM_DEPHEALTH_GROUP)
//   - db — *sql.DB, полученный из pgxpool через stdlib.OpenDBFromPool()
//   - pgConnURL — URL подключения к PostgreSQL (для метрик/лейблов, не для подключения)
//   - keycloakJWKSURL — URL JWKS endpoint Keycloak
//   - checkInterval — интервал проверки зависимостей (AM_DEPHEALTH_CHECK_INTERVAL)
func NewDephealthService(
	serviceID string,
	group string,
	db *sql.DB,
	pgConnURL string,
	keycloakJWKSURL string,
	checkInterval time.Duration,
	logger *slog.Logger,
) (*DephealthService, error) {
	return newDephealthService(serviceID, group, db, pgConnURL, keycloakJWKSURL, checkInterval, logger)
}

// NewDephealthServiceWithRegisterer создаёт сервис с указанным Prometheus registerer.
// Используется в тестах для изоляции метрик.
func NewDephealthServiceWithRegisterer(
	serviceID string,
	group string,
	db *sql.DB,
	pgConnURL string,
	keycloakJWKSURL string,
	checkInterval time.Duration,
	logger *slog.Logger,
	registerer prometheus.Registerer,
) (*DephealthService, error) {
	return newDephealthService(serviceID, group, db, pgConnURL, keycloakJWKSURL, checkInterval, logger,
		dephealth.WithRegisterer(registerer))
}

// newDephealthService — внутренний конструктор.
func newDephealthService(
	serviceID string,
	group string,
	db *sql.DB,
	pgConnURL string,
	keycloakJWKSURL string,
	checkInterval time.Duration,
	logger *slog.Logger,
	extraOpts ...dephealth.Option,
) (*DephealthService, error) {
	// Извлекаем path из JWKS URL для health check.
	// По умолчанию dephealth проверяет /health, но у Keycloak этот endpoint
	// доступен только на management порту (9000). Используем path самого JWKS URL —
	// это подтверждает доступность realm и OIDC endpoints.
	kcHealthPath := "/health"
	if parsed, parseErr := url.Parse(keycloakJWKSURL); parseErr == nil && parsed.Path != "" {
		kcHealthPath = parsed.Path
	}

	opts := []dephealth.Option{
		dephealth.WithLogger(logger),
		// PostgreSQL — connection pool mode через существующий pgxpool.
		// Проверка идёт через *sql.DB (адаптер pgxpool), что отражает реальное
		// состояние пула соединений и может обнаружить его исчерпание.
		// Используем pgcheck.New + dephealth.AddDependency напрямую,
		// чтобы не тянуть contrib/sqldb с транзитивной зависимостью на MySQL.
		dephealth.AddDependency("postgresql", dephealth.TypePostgres,
			pgcheck.New(pgcheck.WithDB(db)),
			dephealth.FromURL(pgConnURL),
			dephealth.CheckInterval(checkInterval),
			dephealth.Critical(true),
		),
		// Keycloak — HTTP checker к JWKS endpoint
		dephealth.HTTP("keycloak-jwks",
			dephealth.FromURL(keycloakJWKSURL),
			dephealth.WithHTTPHealthPath(kcHealthPath),
			dephealth.CheckInterval(checkInterval),
			dephealth.Critical(true),
			dephealth.WithHTTPTLSSkipVerify(true), // Dev-среда: self-signed сертификаты
		),
	}
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
	ds.logger.Info("Мониторинг зависимостей запущен (PostgreSQL + Keycloak)")
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
