// dephealth.go — интеграция с topologymetrics SDK для мониторинга зависимостей.
//
// Query Module мониторит:
//   - PostgreSQL — SQL checker через существующий pgxpool (connection pool mode, critical)
//   - Admin Module — HTTP checker к health endpoint (critical)
//
// В отличие от Admin Module, QM не мониторит динамические SE endpoints.
// SE endpoints не нужны — QM обращается к SE через proxy download,
// и ошибки обрабатываются в момент скачивания (lazy cleanup при 404).
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
	_ "github.com/BigKAA/topologymetrics/sdk-go/dephealth/checks/httpcheck" // регистрация HTTP checker factory
	"github.com/BigKAA/topologymetrics/sdk-go/dephealth/checks/pgcheck"
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
//   - serviceID — имя вершины графа текущего приложения (e.g. "query-module")
//   - group — имя группы в метриках (QM_DEPHEALTH_GROUP)
//   - db — *sql.DB, полученный из pgxpool через stdlib.OpenDBFromPool()
//   - pgConnURL — URL подключения к PostgreSQL (для метрик/лейблов, не для подключения)
//   - adminModuleURL — URL Admin Module health endpoint
//   - checkInterval — интервал проверки зависимостей (QM_DEPHEALTH_CHECK_INTERVAL)
//   - isEntry — при true добавляет лейбл isentry=yes ко всем зависимостям (DEPHEALTH_ISENTRY)
func NewDephealthService(
	serviceID string,
	group string,
	db *sql.DB,
	pgConnURL string,
	adminModuleURL string,
	checkInterval time.Duration,
	isEntry bool,
	logger *slog.Logger,
) (*DephealthService, error) {
	return newDephealthService(serviceID, group, db, pgConnURL, adminModuleURL, checkInterval, isEntry, logger)
}

// NewDephealthServiceWithRegisterer создаёт сервис с указанным Prometheus registerer.
// Используется в тестах для изоляции метрик.
func NewDephealthServiceWithRegisterer(
	serviceID string,
	group string,
	db *sql.DB,
	pgConnURL string,
	adminModuleURL string,
	checkInterval time.Duration,
	isEntry bool,
	logger *slog.Logger,
	registerer prometheus.Registerer,
) (*DephealthService, error) {
	return newDephealthService(serviceID, group, db, pgConnURL, adminModuleURL, checkInterval, isEntry,
		logger, dephealth.WithRegisterer(registerer))
}

// newDephealthService — внутренний конструктор.
func newDephealthService(
	serviceID string,
	group string,
	db *sql.DB,
	pgConnURL string,
	adminModuleURL string,
	checkInterval time.Duration,
	isEntry bool,
	logger *slog.Logger,
	extraOpts ...dephealth.Option,
) (*DephealthService, error) {
	// Извлекаем health path из Admin Module URL.
	// Добавляем /health/ready как probe path
	amHealthPath := "/health/ready"

	// Опции зависимости PostgreSQL
	pgDepOpts := []dephealth.DependencyOption{
		dephealth.FromURL(pgConnURL),
		dephealth.CheckInterval(checkInterval),
		dephealth.Critical(true),
	}
	if isEntry {
		pgDepOpts = append(pgDepOpts, dephealth.WithLabel("isentry", "yes"))
	}

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

	opts := make([]dephealth.Option, 0, 3+len(extraOpts))
	opts = append(opts,
		dephealth.WithLogger(logger),
		// PostgreSQL — connection pool mode через существующий pgxpool.
		// Проверка идёт через *sql.DB (адаптер pgxpool), что отражает реальное
		// состояние пула соединений и может обнаружить его исчерпание.
		dephealth.AddDependency("postgresql", dephealth.TypePostgres,
			pgcheck.New(pgcheck.WithDB(db)), pgDepOpts...),
		// Admin Module — HTTP checker к /health/ready
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
	ds.logger.Info("Мониторинг зависимостей запущен (PostgreSQL + Admin Module)")
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
