# План разработки: Query Module

## Метаданные

- **Версия плана**: 1.0.0
- **Дата создания**: 2026-02-28
- **Последнее обновление**: 2026-02-28
- **Статус**: In Progress

---

## История версий

- **v1.0.0** (2026-02-28): Начальная версия плана

---

## Текущий статус

- **Активная фаза**: Phase 5
- **Активный подпункт**: 5.1
- **Последнее обновление**: 2026-02-28
- **Примечание**: Phase 4 завершена — seclient, download service, download handler, dephealth, main.go интеграция

---

## Оглавление

- [x] [Phase 1: Каркас проекта и кодогенерация](#phase-1-каркас-проекта-и-кодогенерация)
- [x] [Phase 2: Инфраструктурный слой (БД, конфиг, middleware)](#phase-2-инфраструктурный-слой-бд-конфиг-middleware)
- [x] [Phase 3: Бизнес-логика (поиск, метаданные, кэш)](#phase-3-бизнес-логика-поиск-метаданные-кэш)
- [x] [Phase 4: Proxy Download и ленивая очистка](#phase-4-proxy-download-и-ленивая-очистка)
- [ ] [Phase 5: Сборка, деплой и интеграционные тесты](#phase-5-сборка-деплой-и-интеграционные-тесты)

---

## Phase 1: Каркас проекта и кодогенерация

**Dependencies**: None
**Status**: Done

### Описание

Создание скелета Go-модуля, кодогенерация из OpenAPI контракта, базовая структура директорий.
Результат: компилируемый проект со stub-обработчиками и health endpoints.

### Подпункты

- [x] **1.1 Инициализация Go-модуля и структуры директорий**
  - **Dependencies**: None
  - **Description**: `go mod init`, создание дерева директорий по паттерну AM.
    Создание `Makefile` с targets: `build`, `test`, `generate`, `lint`, `docker-build`.
    Создание `oapi-codegen-types.yaml` и `oapi-codegen-server.yaml`.
  - **Creates**:
    - `src/query-module/go.mod`
    - `src/query-module/Makefile`
    - `src/query-module/oapi-codegen-types.yaml`
    - `src/query-module/oapi-codegen-server.yaml`
    - Дерево директорий `internal/`

- [x] **1.2 Кодогенерация из OpenAPI**
  - **Dependencies**: 1.1
  - **Description**: Запуск `oapi-codegen` для генерации `types.gen.go` и `server.gen.go`
    из `docs/api-contracts/query-module-openapi.yaml`. Проверка компиляции.
  - **Creates**:
    - `src/query-module/internal/api/generated/types.gen.go`
    - `src/query-module/internal/api/generated/server.gen.go`

- [x] **1.3 Stub-обработчики и health endpoints**
  - **Dependencies**: 1.2
  - **Description**: Реализация `APIHandler` (implements `generated.ServerInterface`)
    со stub-методами, возвращающими 501 Not Implemented.
    Полная реализация `HealthHandler` (live, ready, metrics) по паттерну AM.
    `ReadinessChecker` interface — пока stub (PostgreSQL checker будет в Phase 2).
  - **Creates**:
    - `src/query-module/internal/api/handlers/handler.go`
    - `src/query-module/internal/api/handlers/health.go`
    - `src/query-module/internal/api/errors/errors.go`

- [x] **1.4 Минимальный main.go и server.go**
  - **Dependencies**: 1.3
  - **Description**: `config.go` с минимальным набором env-vars (только `QM_PORT`,
    `QM_LOG_LEVEL`, `QM_LOG_FORMAT`). `server.go` с chi router, graceful shutdown.
    `main.go` — запуск HTTP-сервера без БД и auth.
    Проверка: `go build` и `curl /health/live` возвращает 200.
  - **Creates**:
    - `src/query-module/cmd/query-module/main.go`
    - `src/query-module/internal/config/config.go`
    - `src/query-module/internal/server/server.go`

### Критерии завершения Phase 1

- [x] Все подпункты завершены (1.1–1.4)
- [x] `go build ./...` компилируется без ошибок
- [x] `go test ./...` проходит (даже если тестов ещё нет)
- [x] `curl /health/live` возвращает `{"status":"ok"}`
- [x] Все stub-endpoints возвращают 501
- [x] `go vet ./...` без ошибок
- [x] `make lint` проходит без ошибок

---

## Phase 2: Инфраструктурный слой (БД, конфиг, middleware)

**Dependencies**: Phase 1
**Status**: Done

### Описание

Подключение к PostgreSQL, миграции (индексы), JWT auth middleware,
logging/metrics middleware, полная конфигурация из env-vars.
Результат: полностью настроенный инфраструктурный слой, готовый к бизнес-логике.

### Подпункты

- [x] **2.1 Полная конфигурация (config.go)**
  - **Dependencies**: None
  - **Description**: Все env-переменные из спецификации (`QM_DB_*`, `QM_JWKS_URL`,
    `QM_CLIENT_ID`, `QM_CLIENT_SECRET`, `QM_ADMIN_URL`, `QM_ADMIN_TIMEOUT`,
    `QM_SE_DOWNLOAD_TIMEOUT`, `QM_SE_CA_CERT_PATH`, `QM_CACHE_TTL`,
    `QM_CACHE_MAX_SIZE`, `QM_DEPHEALTH_CHECK_INTERVAL`, `QM_DB_MAX_CONNS`,
    `QM_HTTP_READ_TIMEOUT`, `QM_HTTP_WRITE_TIMEOUT`, `QM_HTTP_IDLE_TIMEOUT`,
    `QM_SHUTDOWN_TIMEOUT`).
    Хелперы `getEnvRequired`, `getEnvDefault`, `getEnvInt`, `getEnvDuration`,
    `getEnvDurationFallback`, `getEnvBool` — по паттерну AM.
    Методы `DatabaseDSN()`, `DatabaseURL()`.
    `var Version = "dev"` для `-ldflags`.
  - **Creates**:
    - `src/query-module/internal/config/config.go` (обновление)
  - **Links**:
    - Паттерн: `src/admin-module/internal/config/config.go`

- [x] **2.2 Подключение к PostgreSQL и миграции**
  - **Dependencies**: 2.1
  - **Description**: `database.go` — `Connect()` (pgxpool с `QM_DB_MAX_CONNS`),
    `Migrate()` (golang-migrate + embed), `ReadinessChecker`.
    Миграция `001_search_indexes.up.sql` — создание индексов для поиска
    по атрибутам (GIN для tags, B-tree для filename, status, uploaded_by и т.д.).
    Миграция `001_search_indexes.down.sql` — DROP INDEX IF EXISTS.
  - **Creates**:
    - `src/query-module/internal/database/database.go`
    - `src/query-module/internal/database/migrations/001_search_indexes.up.sql`
    - `src/query-module/internal/database/migrations/001_search_indexes.down.sql`
  - **Links**:
    - Паттерн: `src/admin-module/internal/database/database.go`

- [x] **2.3 JWT auth middleware**
  - **Dependencies**: 2.1
  - **Description**: Адаптация auth middleware из AM. JWKS storage (`jwkset`),
    keyfunc, `AuthClaims`, `RequireRoleOrScope`. QM не использует role overrides —
    упрощённая версия без `RoleOverrideProvider`.
    Без группо-ролевого маппинга (QM проверяет роли напрямую из JWT claims).
  - **Creates**:
    - `src/query-module/internal/api/middleware/auth.go`
  - **Links**:
    - Паттерн: `src/admin-module/internal/api/middleware/auth.go`

- [x] **2.4 Logging и Metrics middleware**
  - **Dependencies**: None
  - **Description**: `RequestLogger` middleware (по паттерну AM, динамический log level).
    `MetricsMiddleware` — `qm_http_requests_total`, `qm_http_request_duration_seconds`
    с `normalizePath` для предотвращения cardinality explosion.
    Бизнес-метрики (query_search_total и т.д.) будут добавлены в Phase 3-4.
  - **Creates**:
    - `src/query-module/internal/api/middleware/logging.go`
    - `src/query-module/internal/api/middleware/metrics.go`
  - **Links**:
    - Паттерн: `src/admin-module/internal/api/middleware/logging.go`
    - Паттерн: `src/admin-module/internal/api/middleware/metrics.go`

- [x] **2.5 Обновление main.go и server.go**
  - **Dependencies**: 2.1, 2.2, 2.3, 2.4
  - **Description**: Интеграция всех компонентов Phase 2 в main.go:
    config → logger → migrate → connect → JWT middleware.
    Обновление server.go: подключение middleware (metrics, logging, JWT с exclusions
    для `/health/`, `/metrics`). Интеграция `ReadinessChecker` в health handler.
  - **Creates**:
    - `src/query-module/cmd/query-module/main.go` (обновление)
    - `src/query-module/internal/server/server.go` (обновление)
    - `src/query-module/internal/api/handlers/health.go` (обновление)

### Критерии завершения Phase 2

- [x] Все подпункты завершены (2.1–2.5)
- [x] QM подключается к PostgreSQL, миграции (индексы) применяются
- [x] `/health/ready` проверяет PostgreSQL
- [x] JWT-защищённые endpoints отклоняют запросы без токена (401)
- [x] `/health/live`, `/health/ready`, `/metrics` доступны без JWT
- [x] Логи в JSON формате с корректными уровнями
- [x] `go test ./...` проходит
- [x] `go vet ./...` без ошибок
- [x] `make lint` проходит без ошибок
- [ ] Unit-тесты для config.Load(), middleware (минимальный набор) — перенесены в Phase 3

---

## Phase 3: Бизнес-логика (поиск, метаданные, кэш)

**Dependencies**: Phase 2
**Status**: Done

### Описание

Repository-слой для поиска и чтения файлов, LRU cache, service-слой,
HTTP-обработчики для search и file metadata. Admin Module HTTP-клиент.
Результат: работающие endpoints `/api/v1/search` и `/api/v1/files/{file_id}`.

### Подпункты

- [x] **3.1 Доменная модель и repository**
  - **Dependencies**: None
  - **Description**: `domain/model/file.go` — `FileRecord` struct (маппинг таблицы
    `file_registry`). `repository/repository.go` — `DBTX` interface, `ErrNotFound`.
    `repository/file.go` — `FileRepository` interface + pgx реализация:
    - `GetByID(ctx, fileID) (*FileRecord, error)`
    - `Search(ctx, params SearchParams) ([]FileRecord, int, error)` — динамический
      WHERE + ORDER BY + LIMIT/OFFSET + COUNT
    - `MarkDeleted(ctx, fileID) error` — `UPDATE SET status='deleted'`
    Построение WHERE: `buildSearchWhere` — динамические фильтры
    с поддержкой exact/partial mode для строковых полей.
  - **Creates**:
    - `src/query-module/internal/domain/model/file.go`
    - `src/query-module/internal/repository/repository.go`
    - `src/query-module/internal/repository/file.go`
  - **Links**:
    - Паттерн: `src/admin-module/internal/repository/repository.go`

- [x] **3.2 LRU Cache**
  - **Dependencies**: None
  - **Description**: `service/cache.go` — обёртка над `hashicorp/golang-lru/v2`
    с поддержкой TTL. Интерфейс: `Get(fileID) (*FileRecord, bool)`,
    `Set(fileID, *FileRecord)`, `Delete(fileID)`.
    TTL через `expirable.NewLRU[string, *model.FileRecord](maxSize, nil, ttl)`.
    Prometheus-метрики: `query_cache_hits_total`, `query_cache_misses_total`.
    Зависимость: `github.com/hashicorp/golang-lru/v2`.
  - **Creates**:
    - `src/query-module/internal/service/cache.go`

- [x] **3.3 Admin Module HTTP-клиент**
  - **Dependencies**: None
  - **Description**: `adminclient/client.go` — HTTP-клиент к Admin Module:
    - `GetToken(ctx) (string, error)` — client_credentials grant к Keycloak
      token endpoint (через AM proxy `/auth/token`)
    - `GetStorageElement(ctx, seID) (*SEInfo, error)` — `GET /api/v1/storage-elements/{id}`
    - Автоматическое кэширование SA токена (до истечения `exp - 30s`)
    - TLS с опциональным CA-сертификатом (`QM_SE_CA_CERT_PATH`)
    - Таймаут: `QM_ADMIN_TIMEOUT`
  - **Creates**:
    - `src/query-module/internal/adminclient/client.go`
  - **Links**:
    - Паттерн: `src/admin-module/internal/seclient/client.go`

- [x] **3.4 Service-слой (search, file metadata)**
  - **Dependencies**: 3.1, 3.2
  - **Description**: `service/search.go` — `SearchService`:
    - `Search(ctx, params) (*SearchResponse, error)` — вызывает repository,
      добавляет Prometheus-метрики `query_search_total`, `query_search_duration_seconds`
    - `GetFileMetadata(ctx, fileID) (*FileRecord, error)` — сначала LRU cache,
      при miss — repository → cache set
  - **Creates**:
    - `src/query-module/internal/service/search.go`

- [x] **3.5 HTTP-обработчики (search, file metadata)**
  - **Dependencies**: 3.4
  - **Description**: `handlers/search.go` — `SearchFiles(w, r)`:
    десериализация `SearchRequest`, валидация, вызов service, сериализация `SearchResponse`.
    `handlers/files.go` — `GetFileMetadata(w, r)`:
    извлечение `file_id` из path, вызов service, сериализация `FileMetadata`.
    Авторизация: `RequireRoleOrScope([]string{"admin","readonly"}, []string{"files:read"})`.
    Обновление `handler.go` — wiring новых обработчиков.
  - **Creates**:
    - `src/query-module/internal/api/handlers/search.go`
    - `src/query-module/internal/api/handlers/files.go`
    - `src/query-module/internal/api/handlers/handler.go` (обновление)

- [x] **3.6 Интеграция в main.go и unit-тесты**
  - **Dependencies**: 3.3, 3.5
  - **Description**: Обновление main.go — создание repository, cache, adminclient,
    search service, wiring в APIHandler.
    Unit-тесты: repository (с mock DBTX), cache (TTL, eviction, invalidation),
    search service (с mock repo + cache), handlers (с httptest).
  - **Creates**:
    - `src/query-module/cmd/query-module/main.go` (обновление)
    - `src/query-module/internal/repository/file_test.go`
    - `src/query-module/internal/service/cache_test.go`
    - `src/query-module/internal/service/search_test.go`

### Критерии завершения Phase 3

- [x] Все подпункты завершены (3.1–3.6)
- [x] `POST /api/v1/search` возвращает результаты с пагинацией
- [x] `GET /api/v1/files/{file_id}` возвращает метаданные файла
- [x] LRU cache работает (hit/miss метрики)
- [x] Admin Module client получает SA токен и информацию о SE
- [x] `go test ./...` проходит, unit-тесты покрывают ключевые сценарии
- [x] `go vet ./...` без ошибок
- [x] `make lint` проходит без ошибок

---

## Phase 4: Proxy Download и ленивая очистка

**Dependencies**: Phase 3
**Status**: Done

### Описание

SE HTTP-клиент для proxy download, поддержка HTTP Range requests,
ленивая очистка при 404 от SE, topologymetrics, бизнес-метрики download.
Результат: полностью работающий endpoint `/api/v1/files/{file_id}/download`.

### Подпункты

- [x] **4.1 SE HTTP-клиент (proxy download)**
  - **Dependencies**: None
  - **Description**: `seclient/client.go` — HTTP-клиент к Storage Element:
    - `Download(ctx, seURL, fileID, rangeHeader) (*http.Response, error)` — streaming.
      Не закрывает `resp.Body` — вызывающий код отвечает за это.
      Пробрасывает `Range` header от клиента.
      Добавляет `Authorization: Bearer <SA token>`.
    - TLS с опциональным CA-сертификатом
    - Таймаут: `QM_SE_DOWNLOAD_TIMEOUT` (5m)
    - HTTP Transport: настраиваемый `MaxIdleConnsPerHost`
  - **Creates**:
    - `src/query-module/internal/seclient/client.go`
  - **Links**:
    - Паттерн: `src/admin-module/internal/seclient/client.go`

- [x] **4.2 Download service**
  - **Dependencies**: 4.1
  - **Description**: `service/download.go` — `DownloadService`:
    - `Download(ctx, w, r, fileID)` — полный pipeline:
      1. Получить FileRecord (из cache или DB)
      2. Получить SE URL из Admin Module client (по `storage_element_id` из FileRecord)
      3. Вызвать SE client Download (пробросить Range header)
      4. Если SE вернул 404 → lazy cleanup (mark deleted + invalidate cache) → 404 клиенту
      5. Если SE вернул 200/206 → streaming copy в ResponseWriter
         с пробросом заголовков (Content-Type, Content-Length, Content-Disposition,
         Accept-Ranges, ETag, Content-Range)
    - Prometheus-метрики: `query_downloads_total`, `query_download_duration_seconds`,
      `query_download_bytes_total`, `query_active_downloads` (gauge),
      `query_lazy_cleanup_total`
  - **Creates**:
    - `src/query-module/internal/service/download.go`

- [x] **4.3 Download HTTP-обработчик**
  - **Dependencies**: 4.2
  - **Description**: `handlers/files.go` — добавление `DownloadFile(w, r)`:
    извлечение `file_id` из path, `Range` header из запроса.
    Авторизация: `RequireRoleOrScope`.
    Вызов `DownloadService.Download()`.
    Обновление `handler.go` — wiring.
  - **Creates**:
    - `src/query-module/internal/api/handlers/files.go` (обновление)
    - `src/query-module/internal/api/handlers/handler.go` (обновление)

- [x] **4.4 Topologymetrics**
  - **Dependencies**: None
  - **Description**: `service/dephealth.go` — настройка topologymetrics:
    зависимости PostgreSQL (critical) + Admin Module (critical).
    Без динамических SE endpoints (в отличие от AM).
    Интеграция в main.go — graceful start (warn on failure, не exit).
  - **Creates**:
    - `src/query-module/internal/service/dephealth.go`
  - **Links**:
    - Паттерн: `src/admin-module/internal/service/dephealth.go`

- [x] **4.5 Интеграция в main.go и unit-тесты**
  - **Dependencies**: 4.1, 4.2, 4.3, 4.4
  - **Description**: Полная интеграция в main.go: SE client, download service,
    dephealth. Финальная последовательность инициализации:
    1. config.Load()
    2. SetupLogger()
    3. database.Migrate()
    4. database.Connect()
    5. stdlib.OpenDBFromPool() (для dephealth)
    6. buildHTTPClientWithCA()
    7. adminclient.New()
    8. seclient.New()
    9. FileRepository
    10. CacheService
    11. SearchService
    12. DownloadService
    13. ReadinessChecker
    14. HealthHandler
    15. APIHandler
    16. JWTAuth middleware
    17. DephealthService (graceful)
    18. server.New() + Run()
    Unit-тесты: download service (mock SE client, mock admin client),
    lazy cleanup logic.
  - **Creates**:
    - `src/query-module/cmd/query-module/main.go` (финальная версия)
    - `src/query-module/internal/service/download_test.go`

### Критерии завершения Phase 4

- [x] Все подпункты завершены (4.1–4.5)
- [x] Proxy download работает (200, streaming)
- [x] HTTP Range requests работают (206 Partial Content)
- [x] Lazy cleanup при 404 от SE — обновляет статус в БД, инвалидирует кэш
- [x] Topologymetrics отображает состояние PostgreSQL и Admin Module
- [x] Все Prometheus-метрики регистрируются и обновляются
- [x] `go test ./...` проходит
- [x] `go vet ./...` без ошибок
- [x] `make lint` проходит без ошибок

---

## Phase 5: Сборка, деплой и интеграционные тесты

**Dependencies**: Phase 4
**Status**: Pending

### Описание

Docker-образ, Helm chart, деплой в тестовый кластер, интеграционные тесты,
Gateway API routing. Результат: QM работает в Kubernetes, доступен через
`artstore.kryukov.lan/query/*`, все тесты проходят.

### Подпункты

- [ ] **5.1 Dockerfile и docker-compose.yaml**
  - **Dependencies**: None
  - **Description**: Dockerfile — multi-stage build (без templ/tailwind, проще AM):
    Stage 1: `golang:1.25-alpine` — `go mod download`, `go build` с `-ldflags`.
    Stage 2: `alpine:3.19` — `ca-certificates`, non-root user, healthcheck.
    `docker-compose.yaml` — для локальной разработки (QM + PostgreSQL + Keycloak).
    Проверка: `make docker-build` собирает образ.
  - **Creates**:
    - `src/query-module/Dockerfile`
    - `src/query-module/docker-compose.yaml`
  - **Links**:
    - Паттерн: `src/admin-module/Dockerfile`

- [ ] **5.2 Helm chart (charts/query-module/)**
  - **Dependencies**: None
  - **Description**: Helm chart для production-деплоя:
    `Chart.yaml`, `values.yaml`, `templates/` (deployment, service, httproute).
    HTTPRoute: path prefix `/query` + `/health` + `/metrics`, strip prefix `/query`
    через Envoy Gateway `URLRewrite`.
    Values: replicas, port, all env vars, resources, probes, tls, dephealth.
  - **Creates**:
    - `src/query-module/charts/query-module/Chart.yaml`
    - `src/query-module/charts/query-module/values.yaml`
    - `src/query-module/charts/query-module/templates/_helpers.tpl`
    - `src/query-module/charts/query-module/templates/deployment.yaml`
    - `src/query-module/charts/query-module/templates/service.yaml`
    - `src/query-module/charts/query-module/templates/httproute.yaml`
  - **Links**:
    - Паттерн: `src/admin-module/charts/admin-module/`

- [ ] **5.3 Тестовая инфраструктура (artstore-apps)**
  - **Dependencies**: 5.1
  - **Description**: Добавление QM в `tests/helm/artstore-apps/`:
    - `templates/query-module.yaml` — Deployment + Service
    - `templates/query-module-httproute.yaml` — HTTPRoute (conditional)
    - Обновление `values.yaml` — секция `queryModule`
    - Init container: `wait-for-pg` (pg_isready)
    - Init container: `wait-for-am` (curl к AM /health/ready)
    - Env vars: shared DB credentials, QM_JWKS_URL, QM_ADMIN_URL,
      QM_CLIENT_ID, QM_CLIENT_SECRET
    Добавление Keycloak клиента `artstore-query-module` в тестовый realm
    (обновление init-job или artstore-infra values).
    Обновление `tests/Makefile` — targets для QM.
  - **Creates**:
    - `tests/helm/artstore-apps/templates/query-module.yaml`
    - `tests/helm/artstore-apps/templates/query-module-httproute.yaml`
    - `tests/helm/artstore-apps/values.yaml` (обновление)
    - `tests/Makefile` (обновление)

- [ ] **5.4 Сборка Docker-образа и деплой в тестовый кластер**
  - **Dependencies**: 5.1, 5.2, 5.3
  - **Description**: Сборка Docker-образа с тегом `v0.1.0-1`,
    push в Harbor (`harbor.kryukov.lan/library/query-module:v0.1.0-1`).
    Деплой тестового окружения (`make test-env-up` или ручной `helm upgrade`).
    Проверка: QM pod running, health checks passing, logs OK.
    Проверка: `artstore.kryukov.lan/query/health/live` через Gateway отвечает 200.
  - **Creates**:
    - Docker image `harbor.kryukov.lan/library/query-module:v0.1.0-1`

- [ ] **5.5 Интеграционные тесты**
  - **Dependencies**: 5.4
  - **Description**: Bash + curl тесты в `tests/scripts/`:
    - `test-qm-health.sh` — health/live, health/ready, metrics
    - `test-qm-search.sh` — поиск по атрибутам (exact/partial, фильтры, пагинация)
    - `test-qm-download.sh` — proxy download, Range requests, lazy cleanup
    - `test-qm-auth.sh` — 401 без токена, 403 без scope, 200 с валидным JWT
    Обновление `tests/Makefile`: `make test-qm`, обновление `make test-all`.
    Init data: убедиться, что тестовые файлы загружены через AM/SE
    (либо расширить `init-job`, либо добавить тестовые данные).
  - **Creates**:
    - `tests/scripts/test-qm-health.sh`
    - `tests/scripts/test-qm-search.sh`
    - `tests/scripts/test-qm-download.sh`
    - `tests/scripts/test-qm-auth.sh`
    - `tests/Makefile` (обновление)

### Критерии завершения Phase 5

- [ ] Все подпункты завершены (5.1–5.5)
- [ ] Docker-образ собран и загружен в Harbor
- [ ] QM работает в тестовом кластере Kubernetes
- [ ] Gateway API маршрутизирует `artstore.kryukov.lan/query/*` → QM
- [ ] Все интеграционные тесты проходят
- [ ] `make test-qm` — все тесты green
- [ ] `go vet ./...` без ошибок
- [ ] `make lint` проходит без ошибок
- [ ] Helm chart lint проходит (`helm lint`)
- [ ] Документация обновлена

---

## Примечания

### Ключевые архитектурные решения

1. **Stateless** — no shared state, per-instance LRU cache
2. **Shared PostgreSQL** — QM не владеет таблицами, только читает + добавляет индексы
3. **SA isoliation** — отдельный Keycloak клиент `artstore-query-module`
4. **Streaming download** — `io.Copy` из SE response → client response (не буферизация)
5. **Lazy cleanup** — обновление статуса в БД при 404 от SE + инвалидация cache

### Зависимости Go-модуля

| Пакет | Назначение |
|-------|------------|
| `github.com/go-chi/chi/v5` | HTTP router |
| `github.com/jackc/pgx/v5` | PostgreSQL driver |
| `github.com/golang-migrate/migrate/v4` | Миграции |
| `github.com/golang-jwt/jwt/v5` | JWT parsing |
| `github.com/MicahParks/jwkset` | JWKS storage |
| `github.com/MicahParks/keyfunc/v3` | JWT key function |
| `github.com/oapi-codegen/runtime` | oapi-codegen runtime |
| `github.com/prometheus/client_golang` | Prometheus метрики |
| `github.com/google/uuid` | UUID |
| `github.com/hashicorp/golang-lru/v2` | LRU cache с TTL |
| `github.com/BigKAA/topologymetrics/sdk-go` | topologymetrics |

### Оценка трудоёмкости по фазам

| Фаза | Контекстов AI | Описание |
|------|---------------|----------|
| Phase 1 | 1 | Каркас, кодогенерация, stubs |
| Phase 2 | 1–2 | БД, config, middleware, auth |
| Phase 3 | 2 | Repository, cache, search, admin client |
| Phase 4 | 1–2 | Proxy download, lazy cleanup, dephealth |
| Phase 5 | 2 | Docker, Helm, деплой, интеграционные тесты |

---
