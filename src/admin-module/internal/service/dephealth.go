// dephealth.go — интеграция с topologymetrics SDK для мониторинга зависимостей.
//
// Admin Module мониторит:
//   - PostgreSQL — SQL checker через существующий pgxpool (connection pool mode, critical)
//   - Keycloak — HTTP checker к JWKS endpoint (critical)
//   - Storage Elements — HTTP checker к /health/ready (динамические, non-critical)
//
// SE endpoints добавляются/удаляются динамически при CRUD операциях через
// AddEndpoint/RemoveEndpoint/UpdateEndpoint SDK dephealth v0.8.0.
// При старте AM все SE загружаются из БД.
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
	"fmt"
	"log/slog"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/BigKAA/topologymetrics/sdk-go/dephealth"
	"github.com/BigKAA/topologymetrics/sdk-go/dephealth/checks/httpcheck"
	"github.com/BigKAA/topologymetrics/sdk-go/dephealth/checks/pgcheck" // PostgreSQL checker (pool mode)
	"github.com/prometheus/client_golang/prometheus"
)

// reNonAlphaNum — паттерн для замены спецсимволов при нормализации имени SE.
var reNonAlphaNum = regexp.MustCompile(`[^a-z0-9]+`)

// reMultiDash — коллапс нескольких дефисов подряд.
var reMultiDash = regexp.MustCompile(`-{2,}`)

// seHealthPath — путь readiness probe SE для health check.
const seHealthPath = "/health/ready"

// DephealthService — сервис мониторинга зависимостей через topologymetrics.
type DephealthService struct {
	dh            *dephealth.DepHealth
	tlsSkipVerify bool
	logger        *slog.Logger
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
//   - tlsSkipVerify — пропускать проверку TLS-сертификатов (AM_TLS_SKIP_VERIFY)
//   - isEntry — при true добавляет лейбл isentry=yes ко всем зависимостям (DEPHEALTH_ISENTRY)
func NewDephealthService(
	serviceID string,
	group string,
	db *sql.DB,
	pgConnURL string,
	keycloakJWKSURL string,
	checkInterval time.Duration,
	tlsSkipVerify bool,
	isEntry bool,
	logger *slog.Logger,
) (*DephealthService, error) {
	return newDephealthService(serviceID, group, db, pgConnURL, keycloakJWKSURL, checkInterval, tlsSkipVerify, isEntry, logger)
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
	tlsSkipVerify bool,
	isEntry bool,
	logger *slog.Logger,
	registerer prometheus.Registerer,
) (*DephealthService, error) {
	return newDephealthService(serviceID, group, db, pgConnURL, keycloakJWKSURL, checkInterval, tlsSkipVerify, isEntry,
		logger, dephealth.WithRegisterer(registerer))
}

// newDephealthService — внутренний конструктор.
func newDephealthService(
	serviceID string,
	group string,
	db *sql.DB,
	pgConnURL string,
	keycloakJWKSURL string,
	checkInterval time.Duration,
	tlsSkipVerify bool,
	isEntry bool,
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

	// Опции зависимости PostgreSQL
	pgDepOpts := []dephealth.DependencyOption{
		dephealth.FromURL(pgConnURL),
		dephealth.CheckInterval(checkInterval),
		dephealth.Critical(true),
	}
	if isEntry {
		pgDepOpts = append(pgDepOpts, dephealth.WithLabel("isentry", "yes"))
	}

	// Опции зависимости Keycloak
	kcDepOpts := []dephealth.DependencyOption{
		dephealth.FromURL(keycloakJWKSURL),
		dephealth.WithHTTPHealthPath(kcHealthPath),
		dephealth.CheckInterval(checkInterval),
		dephealth.Critical(true),
		dephealth.WithHTTPTLSSkipVerify(tlsSkipVerify),
	}
	if isEntry {
		kcDepOpts = append(kcDepOpts, dephealth.WithLabel("isentry", "yes"))
	}

	opts := make([]dephealth.Option, 0, 3+len(extraOpts))
	opts = append(opts,
		dephealth.WithLogger(logger),
		// PostgreSQL — connection pool mode через существующий pgxpool.
		// Проверка идёт через *sql.DB (адаптер pgxpool), что отражает реальное
		// состояние пула соединений и может обнаружить его исчерпание.
		dephealth.AddDependency("postgresql", dephealth.TypePostgres,
			pgcheck.New(pgcheck.WithDB(db)), pgDepOpts...),
		// Keycloak — HTTP checker к JWKS endpoint
		dephealth.HTTP("keycloak-jwks", kcDepOpts...),
	)
	opts = append(opts, extraOpts...)

	dh, err := dephealth.New(serviceID, group, opts...)
	if err != nil {
		return nil, err
	}

	return &DephealthService{
		dh:            dh,
		tlsSkipVerify: tlsSkipVerify,
		logger:        logger.With(slog.String("component", "dephealth")),
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

// --- Динамическое управление SE endpoints ---

// NormalizeSEDepName нормализует имя SE для dephealth (regex: ^[a-z][a-z0-9-]*$, 1-63).
//
// Правила:
//   - Перевод в lowercase
//   - Спецсимволы (включая пробелы) → дефис
//   - Коллапс нескольких дефисов подряд → один
//   - Trim дефисов по краям
//   - Обрезка до 63 символов
//   - Если начинается с цифры — префикс "se-"
//   - Пустой результат → "unknown-se"
func NormalizeSEDepName(name string) string {
	// Lowercase
	s := strings.ToLower(name)

	// Спецсимволы → дефис
	s = reNonAlphaNum.ReplaceAllString(s, "-")

	// Коллапс нескольких дефисов
	s = reMultiDash.ReplaceAllString(s, "-")

	// Trim дефисов
	s = strings.Trim(s, "-")

	// Пустой результат
	if s == "" {
		return "unknown-se"
	}

	// Если начинается с цифры — префикс "se-"
	if s[0] >= '0' && s[0] <= '9' {
		s = "se-" + s
	}

	// Обрезка до 63 символов
	if len(s) > 63 {
		s = s[:63]
		// Убираем trailing дефис после обрезки
		s = strings.TrimRight(s, "-")
	}

	return s
}

// parseSEURL разбирает URL Storage Element на host, port и признак TLS.
// Дефолтные порты: HTTPS → "443", HTTP → "80".
func parseSEURL(seURL string) (host, port string, tlsEnabled bool, err error) {
	if seURL == "" {
		return "", "", false, fmt.Errorf("пустой URL")
	}

	parsed, parseErr := url.Parse(seURL)
	if parseErr != nil {
		return "", "", false, fmt.Errorf("невалидный URL: %w", parseErr)
	}

	host = parsed.Hostname()
	if host == "" {
		return "", "", false, fmt.Errorf("не удалось извлечь host из URL: %s", seURL)
	}

	tlsEnabled = parsed.Scheme == "https"

	port = parsed.Port()
	if port == "" {
		if tlsEnabled {
			port = "443"
		} else {
			port = "80"
		}
	}

	return host, port, tlsEnabled, nil
}

// RegisterSEEndpoint регистрирует SE как динамический endpoint в dephealth.
// Создаёт HTTP checker для /health/ready и вызывает AddEndpoint.
// Вызов идемпотентен — повторная регистрация не вызывает ошибку.
func (ds *DephealthService) RegisterSEEndpoint(name, seURL string) error {
	depName := NormalizeSEDepName(name)

	host, port, tlsEnabled, err := parseSEURL(seURL)
	if err != nil {
		return fmt.Errorf("parseSEURL(%s): %w", seURL, err)
	}

	// Создаём HTTP checker для readiness probe SE
	checker := httpcheck.New(
		httpcheck.WithHealthPath(seHealthPath),
		httpcheck.WithTLSEnabled(tlsEnabled),
		httpcheck.WithTLSSkipVerify(ds.tlsSkipVerify),
	)

	ep := dephealth.Endpoint{
		Host: host,
		Port: port,
	}

	if err := ds.dh.AddEndpoint(depName, dephealth.TypeHTTP, false, ep, checker); err != nil {
		return fmt.Errorf("AddEndpoint(%s, %s:%s): %w", depName, host, port, err)
	}

	ds.logger.Info("SE endpoint зарегистрирован в dephealth",
		slog.String("dep_name", depName),
		slog.String("host", host),
		slog.String("port", port),
		slog.Bool("tls", tlsEnabled),
	)

	return nil
}

// UnregisterSEEndpoint удаляет SE endpoint из dephealth.
// Вызов идемпотентен — удаление несуществующего endpoint не вызывает ошибку.
func (ds *DephealthService) UnregisterSEEndpoint(name, seURL string) error {
	depName := NormalizeSEDepName(name)

	host, port, _, err := parseSEURL(seURL)
	if err != nil {
		return fmt.Errorf("parseSEURL(%s): %w", seURL, err)
	}

	if err := ds.dh.RemoveEndpoint(depName, host, port); err != nil {
		return fmt.Errorf("RemoveEndpoint(%s, %s:%s): %w", depName, host, port, err)
	}

	ds.logger.Info("SE endpoint удалён из dephealth",
		slog.String("dep_name", depName),
		slog.String("host", host),
		slog.String("port", port),
	)

	return nil
}

// UpdateSEEndpoint обновляет SE endpoint в dephealth при изменении name или URL.
//
// Если изменилось имя — выполняется Remove(old) + Add(new), т.к. SDK не поддерживает
// переименование depName. Если изменился только URL — используется атомарный UpdateEndpoint.
// Если ничего не изменилось — noop.
func (ds *DephealthService) UpdateSEEndpoint(oldName, oldURL, newName, newURL string) error {
	oldDepName := NormalizeSEDepName(oldName)
	newDepName := NormalizeSEDepName(newName)

	oldHost, oldPort, _, oldErr := parseSEURL(oldURL)
	if oldErr != nil {
		return fmt.Errorf("parseSEURL(old=%s): %w", oldURL, oldErr)
	}

	newHost, newPort, newTLS, newErr := parseSEURL(newURL)
	if newErr != nil {
		return fmt.Errorf("parseSEURL(new=%s): %w", newURL, newErr)
	}

	// Ничего не изменилось
	if oldDepName == newDepName && oldHost == newHost && oldPort == newPort {
		return nil
	}

	// Изменилось имя — Remove + Add (SDK не поддерживает переименование depName)
	if oldDepName != newDepName {
		// Удаляем старый (идемпотентно)
		if err := ds.dh.RemoveEndpoint(oldDepName, oldHost, oldPort); err != nil {
			return fmt.Errorf("RemoveEndpoint(%s): %w", oldDepName, err)
		}

		// Регистрируем новый
		checker := httpcheck.New(
			httpcheck.WithHealthPath(seHealthPath),
			httpcheck.WithTLSEnabled(newTLS),
			httpcheck.WithTLSSkipVerify(ds.tlsSkipVerify),
		)
		ep := dephealth.Endpoint{Host: newHost, Port: newPort}
		if err := ds.dh.AddEndpoint(newDepName, dephealth.TypeHTTP, false, ep, checker); err != nil {
			return fmt.Errorf("AddEndpoint(%s): %w", newDepName, err)
		}

		ds.logger.Info("SE endpoint переименован в dephealth",
			slog.String("old_dep_name", oldDepName),
			slog.String("new_dep_name", newDepName),
			slog.String("host", newHost),
			slog.String("port", newPort),
		)
		return nil
	}

	// Изменился только URL — атомарный UpdateEndpoint
	checker := httpcheck.New(
		httpcheck.WithHealthPath(seHealthPath),
		httpcheck.WithTLSEnabled(newTLS),
		httpcheck.WithTLSSkipVerify(ds.tlsSkipVerify),
	)
	newEp := dephealth.Endpoint{Host: newHost, Port: newPort}

	if err := ds.dh.UpdateEndpoint(newDepName, oldHost, oldPort, newEp, checker); err != nil {
		// Если endpoint не найден (e.g. dephealth не успел зарегистрировать) —
		// пробуем Add как fallback
		if err := ds.dh.AddEndpoint(newDepName, dephealth.TypeHTTP, false, newEp, checker); err != nil {
			return fmt.Errorf("UpdateEndpoint fallback AddEndpoint(%s): %w", newDepName, err)
		}
	}

	ds.logger.Info("SE endpoint обновлён в dephealth",
		slog.String("dep_name", newDepName),
		slog.String("old_host", oldHost),
		slog.String("old_port", oldPort),
		slog.String("new_host", newHost),
		slog.String("new_port", newPort),
	)

	return nil
}
