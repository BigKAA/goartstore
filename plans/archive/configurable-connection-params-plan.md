# План разработки: Вынос hardcoded параметров соединений в конфигурацию

## Метаданные

- **Версия плана**: 1.0.0
- **Дата создания**: 2026-02-24
- **Последнее обновление**: 2026-02-24
- **Статус**: Done

---

## История версий

- **v1.0.0** (2026-02-24): Начальная версия плана

---

## Текущий статус

- **Активная фаза**: Завершено
- **Активный подпункт**: —
- **Последнее обновление**: 2026-02-24
- **Примечание**: Все фазы завершены. Docker-образы AM v0.2.0-6 и SE v0.1.0-4 собраны, задеплоены, все 40 интеграционных тестов пройдены

---

## Оглавление

- [x] [Phase 1: Admin Module — конфигурация и config struct](#phase-1-admin-module--конфигурация-и-config-struct)
- [x] [Phase 2: Admin Module — применение конфигурации в коде](#phase-2-admin-module--применение-конфигурации-в-коде)
- [x] [Phase 3: Storage Element — конфигурация и применение](#phase-3-storage-element--конфигурация-и-применение)
- [x] [Phase 4: Helm charts, CLAUDE.md, документация](#phase-4-helm-charts-claudemd-документация)
- [x] [Phase 5: Сборка и тестирование](#phase-5-сборка-и-тестирование)

---

## Контекст и мотивация

В кодовой базе найдено **18+ hardcoded параметров соединений** (TLS, таймауты, интервалы), которые:

- Создают **риск безопасности** (`InsecureSkipVerify: true` в 3 местах без возможности отключить)
- Не позволяют **тюнить production** без пересборки (таймауты HTTP-клиентов и серверов)
- Нарушают **12-factor app** принцип конфигурации через окружение

### Архитектурные решения

1. **Иерархия таймаутов**: глобальный default + per-client override
   - `AM_HTTP_CLIENT_TIMEOUT=30s` (глобальный)
   - `AM_KEYCLOAK_CLIENT_TIMEOUT=10s` (override, если задан)
   - Если per-client не задан → fallback на глобальный → fallback на hardcoded default
2. **Переименование**: `AM_SE_CA_CERT_PATH` → `AM_CA_CERT_PATH` (cert используется для всех TLS-соединений)
3. **TLS Skip Verify**: отдельный флаг `{MODULE}_TLS_SKIP_VERIFY` (default: `false`)
4. **Cookie expiry и monitoring period** — остаются hardcoded (UI-константы, не сетевые параметры)

---

## Phase 1: Admin Module — конфигурация и config struct

**Dependencies**: None
**Status**: Done

### Описание

Расширение `Config` struct Admin Module новыми полями для TLS, таймаутов HTTP-клиентов, таймаутов HTTP-сервера, JWT/JWKS параметров и Keycloak-специфичных настроек. Реализация иерархической логики fallback для таймаутов. Переименование `SECACertPath` → `CACertPath`.

### Полный список новых параметров Admin Module

| Переменная | Тип | Default | Категория |
|------------|-----|---------|-----------|
| `AM_TLS_SKIP_VERIFY` | bool | `false` | TLS |
| `AM_CA_CERT_PATH` | string | `""` | TLS (переименование `AM_SE_CA_CERT_PATH`) |
| `AM_HTTP_CLIENT_TIMEOUT` | duration | `30s` | HTTP Client (глобальный) |
| `AM_KEYCLOAK_CLIENT_TIMEOUT` | duration | *→ global* | HTTP Client (override) |
| `AM_SE_CLIENT_TIMEOUT` | duration | *→ global* | HTTP Client (override) |
| `AM_JWKS_CLIENT_TIMEOUT` | duration | *→ global* | HTTP Client (override) |
| `AM_OIDC_CLIENT_TIMEOUT` | duration | *→ global* | HTTP Client (override) |
| `AM_PROMETHEUS_CLIENT_TIMEOUT` | duration | *→ global* | HTTP Client (override) |
| `AM_KEYCLOAK_READINESS_TIMEOUT` | duration | `5s` | Keycloak |
| `AM_HTTP_READ_TIMEOUT` | duration | `30s` | HTTP Server |
| `AM_HTTP_WRITE_TIMEOUT` | duration | `60s` | HTTP Server |
| `AM_HTTP_IDLE_TIMEOUT` | duration | `120s` | HTTP Server |
| `AM_JWKS_REFRESH_INTERVAL` | duration | `15s` | JWT/JWKS |
| `AM_JWT_LEEWAY` | duration | `5s` | JWT/JWKS |
| `AM_KEYCLOAK_TOKEN_REFRESH_THRESHOLD` | duration | `30s` | Keycloak |
| `AM_SSE_INTERVAL` | duration | `15s` | UI |

### Подпункты

- [x] **1.1 Расширение Config struct и парсинг env-переменных**
  - **Dependencies**: None
  - **Description**: Добавить новые поля в `Config` struct. Переименовать `SECACertPath` → `CACertPath` (и env `AM_SE_CA_CERT_PATH` → `AM_CA_CERT_PATH`). Реализовать парсинг всех новых env-переменных. Реализовать логику fallback для per-client таймаутов: если per-client не задан, используется глобальный `HTTPClientTimeout`. Добавить валидацию (таймауты > 0, интервалы > 0).
  - **Modifies**:
    - `src/admin-module/internal/config/config.go`
  - **Links**: N/A

- [x] **1.2 Unit-тесты для новой конфигурации**
  - **Dependencies**: 1.1
  - **Description**: Тесты парсинга: все новые env-переменные, fallback-логика (per-client → global → default), валидация. Тест переименования: `AM_CA_CERT_PATH` парсится корректно. Тест TLS: `AM_TLS_SKIP_VERIFY` парсится как bool.
  - **Modifies**:
    - `src/admin-module/internal/config/config_test.go`
  - **Links**: N/A

### Критерии завершения Phase 1

- [x] Все подпункты завершены (1.1, 1.2)
- [x] `go test ./internal/config/...` проходит
- [x] Fallback-логика покрыта тестами
- [x] Переименование `SE_CA_CERT_PATH` → `CA_CERT_PATH` завершено

---

## Phase 2: Admin Module — применение конфигурации в коде

**Dependencies**: Phase 1
**Status**: Done

### Описание

Замена всех hardcoded значений в Admin Module на параметры из `Config`. Передача TLS-конфигурации в dephealth. Обновление конструкторов и сигнатур функций.

### Подпункты

- [x] **2.1 TLS: dephealth и HTTP-клиенты**
  - **Dependencies**: None
  - **Description**: В `dephealth.go`: заменить `dephealth.WithHTTPTLSSkipVerify(true)` на значение из конфигурации, передаваемое как параметр `tlsSkipVerify bool`. Обновить сигнатуры `NewDephealthService` и `NewDephealthServiceWithRegisterer`. В `main.go`: обновить вызов `NewDephealthService` с передачей `cfg.TLSSkipVerify`. Обновить ссылку `cfg.SECACertPath` → `cfg.CACertPath` во всех местах main.go.
  - **Modifies**:
    - `src/admin-module/internal/service/dephealth.go`
    - `src/admin-module/cmd/admin-module/main.go`
  - **Links**: N/A

- [x] **2.2 HTTP Client Timeouts**
  - **Dependencies**: None
  - **Description**: Заменить hardcoded таймауты во всех HTTP-клиентах на значения из конфигурации. Файлы и текущие hardcoded значения:
    - `seclient/client.go:77` — `30s` → `cfg.SEClientTimeout`
    - `keycloak/client.go:45` — `30s` → `cfg.KeycloakClientTimeout`
    - `keycloak/client.go:417` — `5s` → `cfg.KeycloakReadinessTimeout`
    - `ui/auth/oidc.go:68` — `30s` → `cfg.OIDCClientTimeout`
    - `ui/prometheus/client.go:33` — `30s` → `cfg.PrometheusClientTimeout`
    - `api/middleware/auth.go:230` — `30s` → `cfg.JWKSClientTimeout`
    - `api/middleware/auth.go:529,536` — `5s` → `cfg.KeycloakReadinessTimeout`
    - `cmd/admin-module/main.go:490-491` — `30s` → `cfg.HTTPClientTimeout`
    Конструкторы каждого клиента расширяются параметром timeout (или принимают `*http.Client`).
  - **Modifies**:
    - `src/admin-module/internal/seclient/client.go`
    - `src/admin-module/internal/keycloak/client.go`
    - `src/admin-module/internal/ui/auth/oidc.go`
    - `src/admin-module/internal/ui/prometheus/client.go`
    - `src/admin-module/internal/api/middleware/auth.go`
    - `src/admin-module/cmd/admin-module/main.go`
  - **Links**: N/A

- [x] **2.3 HTTP Server Timeouts**
  - **Dependencies**: None
  - **Description**: В `server/server.go`: заменить hardcoded `ReadTimeout: 30s, WriteTimeout: 60s, IdleTimeout: 120s` на параметры из конфигурации. Расширить конструктор `New()` или структуру `Config` сервера для приёма таймаутов.
  - **Modifies**:
    - `src/admin-module/internal/server/server.go`
    - `src/admin-module/cmd/admin-module/main.go`
  - **Links**: N/A

- [x] **2.4 JWT/JWKS и Keycloak параметры**
  - **Dependencies**: None
  - **Description**: Замена hardcoded значений:
    - `api/middleware/auth.go:187` — JWKS RefreshInterval `15s` → `cfg.JWKSRefreshInterval`
    - `api/middleware/auth.go:288` — JWT Leeway `5s` → `cfg.JWTLeeway`
    - `keycloak/client.go:77` — token refresh threshold `30s` → `cfg.KeycloakTokenRefreshThreshold`
    Конструкторы расширяются дополнительными параметрами.
  - **Modifies**:
    - `src/admin-module/internal/api/middleware/auth.go`
    - `src/admin-module/internal/keycloak/client.go`
  - **Links**: N/A

- [x] **2.5 SSE Interval**
  - **Dependencies**: None
  - **Description**: В `ui/handlers/events.go:21`: заменить `sseInterval = 15 * time.Second` на значение из конфигурации `cfg.SSEInterval`. Передать через конструктор handlers.
  - **Modifies**:
    - `src/admin-module/internal/ui/handlers/events.go`
    - `src/admin-module/cmd/admin-module/main.go` (или где создаётся UI handler)
  - **Links**: N/A

- [x] **2.6 Обновление тестов dephealth**
  - **Dependencies**: 2.1
  - **Description**: Обновить unit-тесты `dephealth_test.go` для новой сигнатуры с `tlsSkipVerify`. Проверить что `false` корректно передаётся в опции dephealth.
  - **Modifies**:
    - `src/admin-module/internal/service/dephealth_test.go`
  - **Links**: N/A

### Критерии завершения Phase 2

- [x] Все подпункты завершены (2.1 — 2.6)
- [x] `go build ./...` проходит без ошибок
- [x] `go test ./...` проходит
- [x] Grep по `InsecureSkipVerify: true` в admin-module возвращает 0 результатов (кроме комментариев)
- [x] Grep по `30 * time.Second` в HTTP-клиентах admin-module возвращает 0 результатов (осталось 2 fallback default)
- [x] Поведение по умолчанию не изменилось (defaults совпадают со старыми hardcoded значениями)

---

## Phase 3: Storage Element — конфигурация и применение

**Dependencies**: None (параллельно с Phase 1-2)
**Status**: Done

### Описание

Аналогичные изменения для Storage Element: расширение конфигурации, замена hardcoded значений.

### Полный список новых параметров Storage Element

| Переменная | Тип | Default | Категория |
|------------|-----|---------|-----------|
| `SE_TLS_SKIP_VERIFY` | bool | `false` | TLS |
| `SE_CA_CERT_PATH` | string | `""` | TLS (переименование `SE_JWKS_CA_CERT`) |
| `SE_HTTP_CLIENT_TIMEOUT` | duration | `30s` | HTTP Client (глобальный) |
| `SE_JWKS_CLIENT_TIMEOUT` | duration | *→ global* | HTTP Client (override) |
| `SE_HTTP_READ_TIMEOUT` | duration | `30s` | HTTP Server |
| `SE_HTTP_WRITE_TIMEOUT` | duration | `60s` | HTTP Server |
| `SE_HTTP_IDLE_TIMEOUT` | duration | `120s` | HTTP Server |
| `SE_JWKS_REFRESH_INTERVAL` | duration | `15s` | JWT/JWKS |
| `SE_JWT_LEEWAY` | duration | `5s` | JWT/JWKS |

### Подпункты

- [x] **3.1 Расширение Config struct Storage Element**
  - **Dependencies**: None
  - **Description**: Добавить новые поля. Переименовать `JWKSCACert` → `CACertPath` (env: `SE_JWKS_CA_CERT` → `SE_CA_CERT_PATH`). Реализовать fallback-логику для таймаутов. Добавить парсинг и валидацию.
  - **Modifies**:
    - `src/storage-element/internal/config/config.go`
  - **Links**: N/A

- [x] **3.2 TLS: dephealth и replica proxy**
  - **Dependencies**: 3.1
  - **Description**: В `dephealth.go:93`: заменить `WithHTTPTLSSkipVerify(true)` на параметр из конфигурации. Обновить сигнатуры `NewDephealthService`. В `replica/proxy.go:44`: заменить `InsecureSkipVerify: true` на значение из конфигурации. Обновить `NewLeaderProxy` для приёма `tlsSkipVerify bool` и опционально `caCertPath string`.
  - **Modifies**:
    - `src/storage-element/internal/service/dephealth.go`
    - `src/storage-element/internal/replica/proxy.go`
    - `src/storage-element/cmd/storage-element/main.go`
  - **Links**: N/A

- [x] **3.3 HTTP Client/Server Timeouts и JWT/JWKS**
  - **Dependencies**: 3.1
  - **Description**: Заменить hardcoded значения:
    - `api/middleware/auth.go:126` — JWKS client timeout `30s` → `cfg.JWKSClientTimeout`
    - `api/middleware/auth.go:87` — JWKS RefreshInterval `15s` → `cfg.JWKSRefreshInterval`
    - `api/middleware/auth.go:174` — JWT Leeway `5s` → `cfg.JWTLeeway`
    - `server/server.go:122-124` — server timeouts → из конфигурации
    Обновить конструкторы для приёма параметров.
  - **Modifies**:
    - `src/storage-element/internal/api/middleware/auth.go`
    - `src/storage-element/internal/server/server.go`
    - `src/storage-element/cmd/storage-element/main.go`
  - **Links**: N/A

- [x] **3.4 Unit-тесты Storage Element**
  - **Dependencies**: 3.1, 3.2, 3.3
  - **Description**: Тесты парсинга конфигурации (новые поля, fallback, переименование). Тесты dephealth с новой сигнатурой. Проверить компиляцию всех тестов.
  - **Modifies**:
    - `src/storage-element/internal/config/config_test.go` (если есть)
    - `src/storage-element/internal/service/dephealth_test.go` (если есть)
  - **Links**: N/A

### Критерии завершения Phase 3

- [x] Все подпункты завершены (3.1 — 3.4)
- [x] `go build ./...` проходит
- [x] `go test ./...` проходит
- [x] Grep по `InsecureSkipVerify: true` в storage-element возвращает 0 результатов (кроме комментариев)
- [x] Поведение по умолчанию не изменилось

---

## Phase 4: Helm charts, CLAUDE.md, документация

**Dependencies**: Phase 1, Phase 2, Phase 3
**Status**: Done

### Описание

Обновление Helm charts для обоих модулей — добавление новых env-переменных в values.yaml, ConfigMap и templates. Обновление переименованных переменных. Добавление глобального правила в CLAUDE.md.

### Подпункты

- [x] **4.1 Helm chart Admin Module**
  - **Dependencies**: None
  - **Description**: Обновить `values.yaml`: добавить секции `tls.skipVerify`, `timeouts.httpClient`, `timeouts.httpServer`, `jwt`, `keycloak.tokenRefreshThreshold`, `ui.sseInterval`. Переименовать `tls.caSecret` → обновить mapping на `AM_CA_CERT_PATH` (вместо `AM_SE_CA_CERT_PATH`). Обновить ConfigMap template с новыми env-переменными. Убедиться что все новые параметры опциональны (не ломают текущий деплой).
  - **Modifies**:
    - `src/admin-module/charts/admin-module/values.yaml`
    - `src/admin-module/charts/admin-module/templates/configmap.yaml`
  - **Links**: N/A

- [x] **4.2 Helm chart Storage Element**
  - **Dependencies**: None
  - **Description**: Обновить `values.yaml` и `_helpers.tpl`: добавить TLS, таймауты, JWT/JWKS параметры. Переименовать `jwksCaCert` → обновить mapping на `SE_CA_CERT_PATH` (вместо `SE_JWKS_CA_CERT`).
  - **Modifies**:
    - `src/storage-element/charts/storage-element/values.yaml`
    - `src/storage-element/charts/storage-element/templates/_helpers.tpl`
  - **Links**: N/A

- [x] **4.3 Тестовые Helm charts**
  - **Dependencies**: 4.1, 4.2
  - **Description**: Обновить тестовые Helm charts в `tests/helm/` для учёта переименований env-переменных. В dev-окружении: установить `TLS_SKIP_VERIFY=true` для совместимости с self-signed сертификатами.
  - **Modifies**:
    - `tests/helm/artstore-apps/values.yaml` (или аналог)
    - `tests/helm/artstore-se/values.yaml` (или аналог)
  - **Links**: N/A

- [x] **4.4 Правило в CLAUDE.md**
  - **Dependencies**: None
  - **Description**: Добавить в секцию "Общие правила" CLAUDE.md:
    > **Запрет hardcoded параметров соединений**: все параметры сетевых соединений (таймауты, TLS настройки, интервалы проверок, размеры пулов) **обязательно** выносить в конфигурацию через env-переменные с разумными defaults. Запрещено: `InsecureSkipVerify: true`, литеральные таймауты (`30 * time.Second`) в HTTP-клиентах, hardcoded health-check пути. Каждый новый HTTP-клиент или сетевое соединение должно использовать параметры из `Config` struct.
  - **Modifies**:
    - `CLAUDE.md`
  - **Links**: N/A

### Критерии завершения Phase 4

- [x] Все подпункты завершены (4.1 — 4.4)
- [x] `helm template` проходит без ошибок для обоих модулей
- [x] Новые параметры опциональны — отсутствие их в values не ломает рендеринг
- [x] Правило добавлено в CLAUDE.md

---

## Phase 5: Сборка и тестирование

**Dependencies**: Phase 1, Phase 2, Phase 3, Phase 4
**Status**: Done

### Описание

Полная сборка Docker-образов обоих модулей, деплой в тестовый кластер Kubernetes, прогон интеграционных тестов. Верификация что defaults не изменили поведение.

### Подпункты

- [x] **5.1 Сборка Docker-образов**
  - **Dependencies**: None
  - **Description**: Собрать Docker-образы для admin-module и storage-element с новым кодом. Тег: текущая версия с инкрементированным суффиксом.
  - **Creates**:
    - Docker images в Harbor
  - **Links**: N/A
  - **Результат**: AM `v0.2.0-6`, SE `v0.1.0-4` — собраны и запушены в Harbor

- [x] **5.2 Деплой в тестовый кластер**
  - **Dependencies**: 5.1
  - **Description**: Обновить Helm values в тестовом окружении. `make test-env-down && make test-env-up`. Убедиться что все поды запустились.
  - **Links**: N/A
  - **Результат**: 11 подов Running, 0 restarts

- [x] **5.3 Интеграционные тесты**
  - **Dependencies**: 5.2
  - **Description**: `make test-all` — прогон всех интеграционных тестов. Проверить что поведение не изменилось (все тесты проходят). Проверить логи на отсутствие ошибок TLS.
  - **Links**: N/A
  - **Результат**: 40/40 тестов PASS, 9 групп, 0 FAIL

- [x] **5.4 Верификация новых параметров**
  - **Dependencies**: 5.2
  - **Description**: Установить `AM_TLS_SKIP_VERIFY=false` и проверить поведение (должно использовать CA cert). Проверить что метрики dephealth работают. Проверить логи на корректность таймаутов.
  - **Links**: N/A
  - **Результат**: dephealth метрики =1, CA-сертификат загружен, TLS_SKIP_VERIFY работает, переименования env применены

- [x] **5.5 Перенос плана в архив**
  - **Dependencies**: 5.3, 5.4
  - **Description**: Перенести план в `plans/archive/`.
  - **Modifies**:
    - `plans/configurable-connection-params-plan.md` → `plans/archive/`
  - **Links**: N/A

### Критерии завершения Phase 5

- [x] Все подпункты завершены (5.1 — 5.5)
- [x] Docker-образы собраны и загружены в Harbor
- [x] Все тесты пройдены
- [x] Поведение по умолчанию не изменилось
- [x] План перенесён в архив

---

## Примечания

- **Обратная совместимость**: все defaults совпадают с текущими hardcoded значениями — без явной настройки поведение не меняется
- **Переименования env-переменных**: `AM_SE_CA_CERT_PATH` → `AM_CA_CERT_PATH`, `SE_JWKS_CA_CERT` → `SE_CA_CERT_PATH`. Старые имена перестают работать (0.x, допустимо)
- **Phase 1-2 и Phase 3** могут выполняться параллельно (разные модули)
- **Принцип иерархии**: per-client timeout → global timeout → hardcoded default. Это позволяет быстро задать один `AM_HTTP_CLIENT_TIMEOUT=10s` для всего модуля или точечно настроить отдельный клиент

---

**План готов к выполнению.**
