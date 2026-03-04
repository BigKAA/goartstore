// dephealth.go — интеграция с topologymetrics SDK для мониторинга зависимостей.
//
// Ingester Module мониторит:
//   - Admin Module — HTTP checker к /health/ready (critical)
//   - Keycloak JWKS — HTTP checker к JWKS endpoint (critical)
//   - Storage Elements — HTTP checker к /health/ready (динамические, non-critical)
//
// SE endpoints синхронизируются периодически через опрос Admin Module API
// (SEListFetcher). При старте IM список SE пуст, первые данные появляются
// после первого цикла SE sync.
//
// Метрики доступны на /metrics вместе с остальными Prometheus-метриками:
//   - app_dependency_health — состояние зависимости (1 = ok, 0 = fail)
//   - app_dependency_latency_seconds — задержка проверки
//   - app_dependency_status — категория статуса
//   - app_dependency_status_detail — детальный статус
package service

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/BigKAA/topologymetrics/sdk-go/dephealth"
	"github.com/BigKAA/topologymetrics/sdk-go/dephealth/checks/httpcheck"
	"github.com/prometheus/client_golang/prometheus"
)

// reNonAlphaNum — паттерн для замены спецсимволов при нормализации имени SE.
var reNonAlphaNum = regexp.MustCompile(`[^a-z0-9]+`)

// reMultiDash — коллапс нескольких дефисов подряд.
var reMultiDash = regexp.MustCompile(`-{2,}`)

// seHealthPath — путь readiness probe SE для health check.
const seHealthPath = "/health/ready"

// SEEndpoint — описание SE endpoint для SE sync.
type SEEndpoint struct {
	ID   string
	Name string
	URL  string
}

// SEListFetcher — функция получения списка SE из Admin Module.
// Используется для периодической синхронизации SE endpoints в dephealth.
type SEListFetcher func(ctx context.Context) ([]SEEndpoint, error)

// seEndpointInfo — закэшированная информация о SE endpoint.
type seEndpointInfo struct {
	url     string
	name    string // имя SE (для NormalizeSEDepName)
	depName string
}

// DephealthService — сервис мониторинга зависимостей через topologymetrics.
type DephealthService struct {
	dh            *dephealth.DepHealth
	logger        *slog.Logger
	tlsSkipVerify bool

	// SE sync
	seListFetcher SEListFetcher
	seEndpoints   map[string]seEndpointInfo // key = SE ID
	mu            sync.Mutex
	checkInterval time.Duration
	cancel        context.CancelFunc
}

// NewDephealthService создаёт сервис мониторинга зависимостей.
// Упрощённая версия AM dephealth: без PostgreSQL, но с Keycloak JWKS и SE sync.
//
// Параметры:
//   - serviceID — имя вершины графа текущего приложения (e.g. "ingester-module")
//   - group — имя группы в метриках (IM_DEPHEALTH_GROUP)
//   - adminModuleURL — URL Admin Module health endpoint
//   - jwksURL — URL JWKS endpoint Keycloak
//   - checkInterval — интервал проверки зависимостей (IM_DEPHEALTH_CHECK_INTERVAL)
//   - tlsSkipVerify — пропускать проверку TLS-сертификатов (IM_TLS_SKIP_VERIFY)
//   - isEntry — при true добавляет лейбл isentry=yes ко всем зависимостям (DEPHEALTH_ISENTRY)
//   - seListFetcher — функция получения списка SE (nil = без SE sync)
func NewDephealthService(
	serviceID string,
	group string,
	adminModuleURL string,
	jwksURL string,
	checkInterval time.Duration,
	tlsSkipVerify bool,
	isEntry bool,
	seListFetcher SEListFetcher,
	logger *slog.Logger,
) (*DephealthService, error) {
	return newDephealthService(serviceID, group, adminModuleURL, jwksURL,
		checkInterval, tlsSkipVerify, isEntry, seListFetcher, logger)
}

// NewDephealthServiceWithRegisterer создаёт сервис с указанным Prometheus registerer.
// Используется в тестах для изоляции метрик.
func NewDephealthServiceWithRegisterer(
	serviceID string,
	group string,
	adminModuleURL string,
	jwksURL string,
	checkInterval time.Duration,
	tlsSkipVerify bool,
	isEntry bool,
	seListFetcher SEListFetcher,
	logger *slog.Logger,
	registerer prometheus.Registerer,
) (*DephealthService, error) {
	return newDephealthService(serviceID, group, adminModuleURL, jwksURL,
		checkInterval, tlsSkipVerify, isEntry, seListFetcher, logger,
		dephealth.WithRegisterer(registerer))
}

// newDephealthService — внутренний конструктор.
func newDephealthService(
	serviceID string,
	group string,
	adminModuleURL string,
	jwksURL string,
	checkInterval time.Duration,
	tlsSkipVerify bool,
	isEntry bool,
	seListFetcher SEListFetcher,
	logger *slog.Logger,
	extraOpts ...dephealth.Option,
) (*DephealthService, error) {
	// Health path для Admin Module
	amHealthPath := "/health/ready"

	// Извлекаем path из JWKS URL для health check.
	// По умолчанию dephealth проверяет /health, но у Keycloak этот endpoint
	// доступен только на management порту (9000). Используем path самого JWKS URL —
	// это подтверждает доступность realm и OIDC endpoints.
	kcHealthPath := "/health"
	if parsed, parseErr := url.Parse(jwksURL); parseErr == nil && parsed.Path != "" {
		kcHealthPath = parsed.Path
	}

	// Опции зависимости Admin Module
	amDepOpts := []dephealth.DependencyOption{
		dephealth.FromURL(adminModuleURL),
		dephealth.WithHTTPHealthPath(amHealthPath),
		dephealth.CheckInterval(checkInterval),
		dephealth.Critical(true),
		dephealth.WithHTTPTLSSkipVerify(tlsSkipVerify),
	}
	if isEntry {
		amDepOpts = append(amDepOpts, dephealth.WithLabel("isentry", "yes"))
	}

	// Опции зависимости Keycloak JWKS
	kcDepOpts := []dephealth.DependencyOption{
		dephealth.FromURL(jwksURL),
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
		// Admin Module — HTTP checker к /health/ready (critical)
		dephealth.HTTP("admin-module", amDepOpts...),
		// Keycloak — HTTP checker к JWKS endpoint (critical)
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
		seListFetcher: seListFetcher,
		seEndpoints:   make(map[string]seEndpointInfo),
		checkInterval: checkInterval,
	}, nil
}

// Start запускает периодическую проверку зависимостей и SE sync.
func (ds *DephealthService) Start(ctx context.Context) error {
	syncCtx, cancel := context.WithCancel(ctx)
	ds.cancel = cancel

	if err := ds.dh.Start(ctx); err != nil {
		cancel()
		return err
	}

	if ds.seListFetcher != nil {
		ds.startSESync(syncCtx)
	}

	ds.logger.Info("Мониторинг зависимостей запущен (Admin Module + Keycloak JWKS + SE sync)")
	return nil
}

// Stop останавливает мониторинг зависимостей и SE sync горутину.
func (ds *DephealthService) Stop() {
	if ds.cancel != nil {
		ds.cancel()
	}
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

// registerSEEndpoint регистрирует SE как динамический endpoint в dephealth.
// seName — имя SE (используется для формирования dep_name в метриках).
func (ds *DephealthService) registerSEEndpoint(seName, seURL string) error {
	depName := NormalizeSEDepName(seName)

	host, port, tlsEnabled, err := parseSEURL(seURL)
	if err != nil {
		return fmt.Errorf("parseSEURL(%s): %w", seURL, err)
	}

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

// unregisterSEEndpoint удаляет SE endpoint из dephealth.
func (ds *DephealthService) unregisterSEEndpoint(seName, seURL string) error {
	depName := NormalizeSEDepName(seName)

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

// updateSEEndpoint обновляет SE endpoint в dephealth при изменении URL.
func (ds *DephealthService) updateSEEndpoint(seName, oldURL, newURL string) error {
	depName := NormalizeSEDepName(seName)

	oldHost, oldPort, _, oldErr := parseSEURL(oldURL)
	if oldErr != nil {
		return fmt.Errorf("parseSEURL(old=%s): %w", oldURL, oldErr)
	}

	newHost, newPort, newTLS, newErr := parseSEURL(newURL)
	if newErr != nil {
		return fmt.Errorf("parseSEURL(new=%s): %w", newURL, newErr)
	}

	// Ничего не изменилось
	if oldHost == newHost && oldPort == newPort {
		return nil
	}

	// Изменился URL — атомарный UpdateEndpoint
	checker := httpcheck.New(
		httpcheck.WithHealthPath(seHealthPath),
		httpcheck.WithTLSEnabled(newTLS),
		httpcheck.WithTLSSkipVerify(ds.tlsSkipVerify),
	)
	newEp := dephealth.Endpoint{Host: newHost, Port: newPort}

	if err := ds.dh.UpdateEndpoint(depName, oldHost, oldPort, newEp, checker); err != nil {
		// Если endpoint не найден — пробуем Add как fallback
		if addErr := ds.dh.AddEndpoint(depName, dephealth.TypeHTTP, false, newEp, checker); addErr != nil {
			return fmt.Errorf("UpdateEndpoint fallback AddEndpoint(%s): %w", depName, addErr)
		}
	}

	ds.logger.Info("SE endpoint обновлён в dephealth",
		slog.String("dep_name", depName),
		slog.String("old_host", oldHost),
		slog.String("old_port", oldPort),
		slog.String("new_host", newHost),
		slog.String("new_port", newPort),
	)

	return nil
}

// startSESync запускает фоновую горутину для периодической синхронизации SE endpoints.
func (ds *DephealthService) startSESync(ctx context.Context) {
	go func() {
		// Первая синхронизация сразу при старте
		ds.syncSEEndpoints(ctx)

		ticker := time.NewTicker(ds.checkInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				ds.logger.Debug("SE sync горутина остановлена")
				return
			case <-ticker.C:
				ds.syncSEEndpoints(ctx)
			}
		}
	}()
}

// syncSEEndpoints синхронизирует SE endpoints из Admin Module с dephealth.
// Вычисляет diff (add/remove/update) и применяет изменения.
func (ds *DephealthService) syncSEEndpoints(ctx context.Context) {
	elements, err := ds.seListFetcher(ctx)
	if err != nil {
		ds.logger.Warn("Ошибка получения списка SE для sync",
			slog.String("error", err.Error()),
		)
		return
	}

	ds.mu.Lock()
	defer ds.mu.Unlock()

	// Текущий набор SE из AM
	currentSEs := make(map[string]SEEndpoint, len(elements))
	for _, e := range elements {
		currentSEs[e.ID] = e
	}

	// Удаление SE, которых больше нет в AM
	for id, info := range ds.seEndpoints {
		if _, exists := currentSEs[id]; !exists {
			if err := ds.unregisterSEEndpoint(info.name, info.url); err != nil {
				ds.logger.Warn("Ошибка удаления SE endpoint",
					slog.String("se_id", id),
					slog.String("se_name", info.name),
					slog.String("error", err.Error()),
				)
			}
			delete(ds.seEndpoints, id)
		}
	}

	// Добавление новых и обновление изменённых SE
	for id, se := range currentSEs {
		existing, exists := ds.seEndpoints[id]
		if !exists {
			// Новый SE — регистрируем (имя SE используется для dep_name в метриках)
			if err := ds.registerSEEndpoint(se.Name, se.URL); err != nil {
				ds.logger.Warn("Ошибка регистрации SE endpoint",
					slog.String("se_id", id),
					slog.String("se_name", se.Name),
					slog.String("error", err.Error()),
				)
				continue
			}
			ds.seEndpoints[id] = seEndpointInfo{
				url:     se.URL,
				name:    se.Name,
				depName: NormalizeSEDepName(se.Name),
			}
		} else if existing.url != se.URL {
			// URL изменился — обновляем
			if err := ds.updateSEEndpoint(existing.name, existing.url, se.URL); err != nil {
				ds.logger.Warn("Ошибка обновления SE endpoint",
					slog.String("se_id", id),
					slog.String("se_name", existing.name),
					slog.String("error", err.Error()),
				)
				continue
			}
			ds.seEndpoints[id] = seEndpointInfo{
				url:     se.URL,
				name:    existing.name,
				depName: NormalizeSEDepName(existing.name),
			}
		}
	}
}
