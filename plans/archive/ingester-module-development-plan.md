# План разработки: Ingester Module v0.1.0

## Метаданные

- **Версия плана**: 1.0.0
- **Дата создания**: 2026-03-01
- **Последнее обновление**: 2026-03-01
- **Статус**: Done

---

## История версий

- **v1.0.0** (2026-03-01): Начальная версия плана

---

## Текущий статус

- **Активная фаза**: Все завершены
- **Активный подпункт**: —
- **Последнее обновление**: 2026-03-01
- **Примечание**: Все фазы 0-5 Done. Phase 5: 3 drawio-диаграммы, обновление брифа (IG_*→IM_*, Sequential Fill, retry), обновление CLAUDE.md. План перенесён в archive.

---

## Оглавление

- [x] [Phase 0: Подготовка Admin Module (priority в SE)](#phase-0-подготовка-admin-module-priority-в-se)
- [x] [Phase 1: Каркас проекта и кодогенерация](#phase-1-каркас-проекта-и-кодогенерация)
- [x] [Phase 2: Инфраструктурный слой (конфиг, middleware, health)](#phase-2-инфраструктурный-слой-конфиг-middleware-health)
- [x] [Phase 3: Бизнес-логика (upload pipeline, SE selection, file registration)](#phase-3-бизнес-логика-upload-pipeline-se-selection-file-registration)
- [x] [Phase 4: Сборка, деплой и интеграционные тесты](#phase-4-сборка-деплой-и-интеграционные-тесты)
- [x] [Phase 5: Sequence-диаграммы и документация](#phase-5-sequence-диаграммы-и-документация)

---

## Ключевые архитектурные решения

| Решение | Выбор | Обоснование |
|---------|-------|-------------|
| Алгоритм выбора SE | Sequential Fill (by priority) | Предсказуемо, проще мониторить заполнение |
| Upload strategy | Phase 1: sync + temp-file; Phase 2: async (v0.2) | MVP быстрее, async — следующая версия |
| Буферизация файла | Temp-file (emptyDir) | Большие файлы без RAM overhead, retry при 507 |
| 507 Retry | Auto-retry до 3 попыток | Прозрачно для клиента, как в старом проекте |
| Gateway prefix | `/upload` | По функции модуля |
| Replicas | 1 под в тест-среде, 2+ в production | Горизонтальное масштабирование |
| Env prefix | `IM_*` | По аналогии с QM |
| Metrics prefix | `im_*` | По аналогии с QM |
| Keycloak client | `artstore-ingester` | Отдельный SA для модуля |
| Порт | 8020 | Диапазон 8020-8029 |

---

## Phase 0: Подготовка Admin Module (priority в SE)

**Dependencies**: None
**Status**: Done

### Описание

Добавление поля `priority` в таблицу `storage_elements` Admin Module.
Это необходимо для Sequential Fill Algorithm в Ingester Module.
Поле `priority` определяет порядок заполнения SE: lower value = higher priority.
Результат: AM API возвращает `priority` в ответах, UI позволяет задавать приоритет.

### Подпункты

- [x] **0.1 Миграция БД: добавление колонки priority**
  - **Dependencies**: None
  - **Description**: Новая SQL-миграция `004` в Admin Module (последняя существующая — `003`):
    `ALTER TABLE storage_elements ADD COLUMN priority INTEGER NOT NULL DEFAULT 0`.
    Значение 0 = наивысший приоритет. SE с одинаковым priority сортируются по `name ASC`.
    Также добавить индекс `CREATE INDEX idx_se_priority ON storage_elements (priority, mode, status)`.
    Down-миграция: `DROP INDEX idx_se_priority; ALTER TABLE storage_elements DROP COLUMN priority`.
  - **Creates**:
    - `src/admin-module/internal/database/migrations/004_add_se_priority.up.sql`
    - `src/admin-module/internal/database/migrations/004_add_se_priority.down.sql`

- [x] **0.2 Обновление OpenAPI контракта Admin Module**
  - **Dependencies**: 0.1
  - **Description**: Добавить поле `priority` (integer, default 0) в схемы:
    - `StorageElement` (response) — новое поле `priority` (integer)
    - `StorageElementCreate` (request) — опциональное поле `priority` (integer, default 0).
      Текущие поля: `name` (required), `url` (required)
    - `StorageElementUpdate` (request) — опциональное поле `priority` (integer).
      Текущие поля: `name` (optional), `url` (optional)
    Добавить query-параметр `sort_by=priority` (опционально) в `GET /api/v1/storage-elements`.
    Текущие параметры: `mode`, `status`, `limit`, `offset`.
    Обновить кодогенерацию: `make generate`.
  - **Creates**:
    - `docs/api-contracts/admin-module-openapi.yaml` (обновление)
    - `src/admin-module/internal/api/generated/types.gen.go` (перегенерация)
    - `src/admin-module/internal/api/generated/server.gen.go` (перегенерация)

- [x] **0.3 Обновление бэкенда Admin Module (model → repo → service → handler)**
  - **Dependencies**: 0.2
  - **Description**:
    **Domain model** — `storage_element.go`: добавить `Priority int`.
    **Repository** — `storage_element.go`:
    - `Create` INSERT — добавить priority в список колонок и значений
    - `GetByID` SELECT — добавить priority в select-list и Scan
    - `List` SELECT — добавить priority в select-list и Scan,
      изменить `ORDER BY created_at DESC` → `ORDER BY priority ASC, name ASC`
    - `Update` SET — добавить `priority=$N`
    **Service** — `storage_elements.go`:
    - `Create(ctx, name, url)` → `Create(ctx, name, url, priority int)` — передача в repository
    - `Update(ctx, id, name, url)` → `Update(ctx, id, name, url, priority *int)` — опционально
    **Handler** — `storage_elements.go`:
    - `CreateStorageElement` — извлечь `req.Priority`, передать в service
    - `UpdateStorageElement` — извлечь `req.Priority`, передать в service
    - `mapStorageElement` — добавить `Priority: se.Priority` в маппинг
  - **Creates**:
    - `src/admin-module/internal/domain/model/storage_element.go` (обновление)
    - `src/admin-module/internal/repository/storage_element.go` (обновление)
    - `src/admin-module/internal/service/storage_elements.go` (обновление)
    - `src/admin-module/internal/api/handlers/storage_elements.go` (обновление)

- [x] **0.3b Обновление Admin UI (Templ templates + i18n)**
  - **Dependencies**: 0.3
  - **Description**:
    **Templ шаблоны** (5 файлов):
    - `pages/se_list.templ` — добавить `Priority int` в `SEListItem`, колонку в таблицу
    - `pages/se_detail.templ` — добавить `Priority int` в `SEDetailData`, строку в блок «Свойства»
    - `pages/partials/se_edit.templ` — добавить поле ввода priority в форму редактирования,
      обновить сигнатуру `SEEditForm(id, name, url string)` → включить priority
    - `pages/partials/se_discover.templ` — добавить поле priority в форму регистрации SE
    - `pages/partials/se_table.templ` — обновить `seColCount()` / `sePartialColCount()`
      (текущее: 8 admin / 7 readonly)
    **i18n** (2 файла):
    - `i18n/locales/ru.json` — добавить ключи: `se.table.priority`, `se_detail.priority`,
      `se_edit.priority_label`, `se_edit.priority_placeholder`
    - `i18n/locales/en.json` — аналогичные ключи на английском
    Запустить `make templ-generate` для перегенерации `_templ.go` файлов.
    Запустить `make i18n-check` для проверки паритета ключей.
  - **Creates**:
    - `src/admin-module/internal/ui/pages/se_list.templ` (обновление)
    - `src/admin-module/internal/ui/pages/se_detail.templ` (обновление)
    - `src/admin-module/internal/ui/pages/partials/se_edit.templ` (обновление)
    - `src/admin-module/internal/ui/pages/partials/se_discover.templ` (обновление)
    - `src/admin-module/internal/ui/pages/partials/se_table.templ` (обновление)
    - `src/admin-module/internal/ui/i18n/locales/ru.json` (обновление)
    - `src/admin-module/internal/ui/i18n/locales/en.json` (обновление)

- [x] **0.4 Сборка и деплой обновлённого Admin Module**
  - **Dependencies**: 0.3b
  - **Description**: Сборка нового Docker-образа AM.
    Текущий тег: `v0.3.0-1` → новый: `v0.3.0-2`.
    1. `make docker-build-am AM_TAG=v0.3.0-2` (из `tests/`)
    2. `make docker-push-am AM_TAG=v0.3.0-2`
    3. Обновить `tests/Makefile`: `AM_TAG ?= v0.3.0-2`
    4. `make apps-up AM_TAG=v0.3.0-2` — деплой в тестовый кластер
    Проверка: GET /api/v1/storage-elements возвращает поле `priority` для каждого SE.
    Проверка: миграция 004 применена (`priority = 0` для существующих SE).
    Проверка: Admin UI отображает priority в таблице SE и позволяет редактировать.
  - **Creates**:
    - Docker image `harbor.kryukov.lan/library/admin-module:v0.3.0-2`
    - `tests/Makefile` (обновление `AM_TAG`)

### Критерии завершения Phase 0

- [x] Все подпункты завершены (0.1–0.4, включая 0.3b)
- [x] Миграция `004` применяется без ошибок
- [x] `GET /api/v1/storage-elements` возвращает поле `priority` для каждого SE
- [x] `GET /api/v1/storage-elements` сортирует результаты по `priority ASC, name ASC`
- [x] `POST /api/v1/storage-elements` принимает и сохраняет `priority`
- [x] `PUT /api/v1/storage-elements/{id}` принимает и сохраняет `priority`
- [x] Admin UI отображает priority в таблице SE, деталях и формах
- [x] `make i18n-check` проходит (паритет ключей ru.json / en.json)
- [x] `make ui-build` (templ-generate + css-build) проходит без ошибок
- [x] Существующие интеграционные тесты AM проходят (`make test-am`)
- [x] `go vet ./...` и `make lint` проходят для AM

---

## Phase 1: Каркас проекта и кодогенерация

**Dependencies**: None (может выполняться параллельно с Phase 0)
**Status**: Done

### Описание

Создание скелета Go-модуля Ingester Module, обновление OpenAPI контракта
(приведение к решениям: `IM_*` env, `im_*` метрики, Sequential Fill),
кодогенерация, базовая структура директорий со stub-обработчиками и health endpoints.
Результат: компилируемый проект с `go build`, `curl /health/live` → 200.

### Подпункты

- [x] **1.1 Обновление OpenAPI контракта Ingester Module**
  - **Dependencies**: None
  - **Description**: Ревизия `docs/api-contracts/ingester-module-openapi.yaml`:
    - Проверить соответствие принятым решениям (sync upload, temp-file, Sequential Fill)
    - Привести примеры env-переменных к префиксу `IM_*` (если в spec упоминаются `IG_*`)
    - Привести названия метрик к префиксу `im_*` (вместо `ingester_*`)
    - Убедиться, что endpoint `POST /api/v1/files/upload` корректно описан:
      multipart/form-data с полями file, description, tags, retention_policy, ttl_days
    - **Добавить** поле `storage_element_id` (UUID) в схему `UploadResponse` —
      идентификатор SE, в который загружен файл
    - Response 201: UploadResponse (file_id, original_filename, content_type, size,
      checksum, uploaded_by, uploaded_at, status, retention_policy, ttl_days, expires_at,
      description, tags, storage_element_id)
    - Error responses: 400, 401, 403, 413, 502, 507, 500
    - Health endpoints: /health/live, /health/ready, /metrics
    - Readiness response: 3 check-а (admin_module, edit_storage, rw_storage) —
      сверить со структурой `HealthReadyResponse` в текущей spec
  - **Creates**:
    - `docs/api-contracts/ingester-module-openapi.yaml` (обновление)
  - **Примечание**: Бриф `docs/briefs/ingester-module.md` содержит устаревшие решения
    (префикс `IG_*`, метрики `ingester_*`, выбор SE по наибольшему месту).
    Бриф будет обновлён в Phase 5.4.

- [x] **1.2 Инициализация Go-модуля и структуры директорий**
  - **Dependencies**: None
  - **Description**: `go mod init github.com/bigkaa/goartstore/ingester-module`.
    `go.mod`: версия `go 1.25.6` (по аналогии с QM).
    Создание дерева директорий по паттерну QM.
    `.gitignore` — исключить `bin/`, временные файлы.
    Makefile с targets: build, test, generate, lint, lint-docker, lint-helm,
    security-scan, secrets-scan, check-all, docker-build, clean.
    Makefile ссылается на корневые lint-конфиги (`.hadolint.yaml`, `.golangci.yml`).
    Конфиги oapi-codegen:
    - `oapi-codegen-types.yaml`: `generate.models: true`
    - `oapi-codegen-server.yaml`: `generate.chi-server: true`, `generate.embedded-spec: true`
  - **Creates**:
    - `src/ingester-module/go.mod`
    - `src/ingester-module/.gitignore`
    - `src/ingester-module/Makefile`
    - `src/ingester-module/oapi-codegen-types.yaml`
    - `src/ingester-module/oapi-codegen-server.yaml`
    - Дерево директорий `internal/`
  - **Links**:
    - Паттерн: `src/query-module/Makefile`
    - Паттерн: `src/query-module/.gitignore`
    - Паттерн: `src/query-module/oapi-codegen-types.yaml`
    - Паттерн: `src/query-module/oapi-codegen-server.yaml`

- [x] **1.3 Кодогенерация из OpenAPI**
  - **Dependencies**: 1.1, 1.2
  - **Description**: Запуск `oapi-codegen` для генерации `types.gen.go` и `server.gen.go`
    из `docs/api-contracts/ingester-module-openapi.yaml`. Проверка компиляции.
  - **Creates**:
    - `src/ingester-module/internal/api/generated/types.gen.go`
    - `src/ingester-module/internal/api/generated/server.gen.go`

- [x] **1.4 Stub-обработчики и health endpoints**
  - **Dependencies**: 1.3
  - **Description**: Реализация `APIHandler` (implements `generated.ServerInterface`)
    со stub-методом `UploadFile`, возвращающим 501 Not Implemented.
    `APIHandler` делегирует health-методы в `HealthHandler` (паттерн QM):
    - `HealthLive` — 200 + JSON `{status, timestamp, version, service: "ingester-module"}`
    - `GetMetrics` — `promhttp.Handler()`
    - `HealthReady` — stub (всегда ok), полная реализация в Phase 3.7
    `errors.go` — типизированные ошибки по OpenAPI контракту IM:
    `VALIDATION_ERROR` (400), `UNAUTHORIZED` (401), `FORBIDDEN` (403),
    `FILE_TOO_LARGE` (413), `NO_STORAGE_AVAILABLE` (502),
    `SE_UPLOAD_FAILED` (502), `ADMIN_UNAVAILABLE` (502),
    `STORAGE_FULL` (507), `INTERNAL_ERROR` (500).
    Базовый конструктор `WriteError(w, statusCode, code, message)` + хелперы.
    Вспомогательные функции в `handler.go`: `writeJSON`, `checkAuth` (stub).
  - **Creates**:
    - `src/ingester-module/internal/api/handlers/handler.go`
    - `src/ingester-module/internal/api/handlers/health.go`
    - `src/ingester-module/internal/api/errors/errors.go`
  - **Links**:
    - Паттерн: `src/query-module/internal/api/handlers/handler.go`
    - Паттерн: `src/query-module/internal/api/handlers/health.go`
    - Паттерн: `src/query-module/internal/api/errors/errors.go`

- [x] **1.5 Минимальный main.go и server.go**
  - **Dependencies**: 1.4
  - **Description**: `config.go` с минимальным набором env-vars:
    - `IM_PORT` (8020), `IM_LOG_LEVEL` (info), `IM_LOG_FORMAT` (json)
    - `IM_HTTP_READ_TIMEOUT` (30s), `IM_HTTP_WRITE_TIMEOUT` (600s),
      `IM_HTTP_IDLE_TIMEOUT` (120s), `IM_SHUTDOWN_TIMEOUT` (10s)
    Хелперы: `getEnvRequired`, `getEnvDefault`, `getEnvInt`, `getEnvDuration`, `getEnvBool`.
    `var Version = "dev"` — для инъекции через `-ldflags`.
    `server.go` — chi router, `http.Server` с таймаутами из config,
    graceful shutdown (SIGINT/SIGTERM) с `ShutdownTimeout`,
    `JWTAuthWithExclusions` хелпер (подготовка для Phase 2).
    `main.go` — запуск HTTP-сервера без auth и зависимостей.
    Проверка: `go build` и `curl /health/live` возвращает 200.
  - **Creates**:
    - `src/ingester-module/cmd/ingester-module/main.go`
    - `src/ingester-module/internal/config/config.go`
    - `src/ingester-module/internal/server/server.go`
  - **Links**:
    - Паттерн: `src/query-module/cmd/query-module/main.go`
    - Паттерн: `src/query-module/internal/config/config.go`
    - Паттерн: `src/query-module/internal/server/server.go`

### Критерии завершения Phase 1

- [x] Все подпункты завершены (1.1–1.5)
- [x] `make generate` генерирует `types.gen.go` и `server.gen.go` без ошибок
- [x] `go build ./...` компилируется без ошибок
- [x] `go test ./...` проходит
- [x] `curl /health/live` возвращает `{"status":"ok","service":"ingester-module"}`
- [x] `curl /health/ready` возвращает `{"status":"ok"}` (stub)
- [x] Stub upload endpoint возвращает 501
- [x] `go vet ./...` без ошибок
- [x] `make lint` проходит без ошибок
- [x] `.gitignore` покрывает `bin/`

---

## Phase 2: Инфраструктурный слой (конфиг, middleware, health)

**Dependencies**: Phase 1
**Status**: Done

### Описание

Полная конфигурация из env-переменных, JWT auth middleware, logging/metrics middleware,
readiness check (Admin Module). Ingester Module **не использует собственную БД** в v0.1 —
вся регистрация файлов через Admin Module API.
Результат: полностью настроенный инфраструктурный слой с auth, логированием, метриками.

### Подпункты

- [x] **2.1 Полная конфигурация (config.go)**
  - **Dependencies**: None
  - **Description**: Расширение config.go из Phase 1.5. Добавить переменные
    (все с префиксом `IM_`, кроме `DEPHEALTH_NAME` и `DEPHEALTH_ISENTRY` — без префикса,
    по аналогии с QM):
    - Сервер: `IM_PORT` (8020), `IM_LOG_LEVEL` (info), `IM_LOG_FORMAT` (json)
      — уже из Phase 1.5
    - JWT/JWKS: `IM_JWKS_URL` (**required**), `IM_JWT_ISSUER` (""),
      `IM_JWKS_REFRESH_INTERVAL` (15s), `IM_JWT_LEEWAY` (5s),
      `IM_JWKS_CLIENT_TIMEOUT` (fallback → HTTPClientTimeout)
    - RBAC: `IM_ROLE_ADMIN_GROUPS` ("artstore-admins"),
      `IM_ROLE_READONLY_GROUPS` ("artstore-viewers")
    - Admin Module: `IM_ADMIN_URL` (**required**), `IM_ADMIN_TIMEOUT` (10s)
    - Keycloak OAuth2: `IM_CLIENT_ID` (**required**), `IM_CLIENT_SECRET` (**required**)
    - SE Upload: `IM_SE_UPLOAD_TIMEOUT` (10m), `IM_SE_CA_CERT_PATH` ("")
    - Upload: `IM_MAX_FILE_SIZE` (1073741824 = 1GB), `IM_DEFAULT_TTL_DAYS` (30),
      `IM_MAX_RETRIES` (3)
    - TLS: `IM_CA_CERT_PATH` ("")
    - HTTP client: `IM_HTTP_CLIENT_TIMEOUT` (30s) — общий таймаут для HTTP-клиентов
    - HTTP server: `IM_HTTP_READ_TIMEOUT` (30s), `IM_HTTP_WRITE_TIMEOUT` (600s),
      `IM_HTTP_IDLE_TIMEOUT` (120s), `IM_SHUTDOWN_TIMEOUT` (10s)
      — уже из Phase 1.5
    - Topologymetrics: `IM_DEPHEALTH_CHECK_INTERVAL` (15s), `IM_DEPHEALTH_GROUP` (""),
      `DEPHEALTH_NAME` ("ingester-module"), `DEPHEALTH_ISENTRY` (false)
    Обязательные переменные (без default): `IM_JWKS_URL`, `IM_ADMIN_URL`,
    `IM_CLIENT_ID`, `IM_CLIENT_SECRET`.
    Добавить `getEnvDurationFallback` (для JWKS_CLIENT_TIMEOUT → HTTPClientTimeout).
    `SetupLogger(cfg)` — уже из Phase 1.5.
    IM НЕ имеет DB-переменных (stateless, нет PostgreSQL).
  - **Creates**:
    - `src/ingester-module/internal/config/config.go` (обновление)
  - **Links**:
    - Паттерн: `src/query-module/internal/config/config.go`

- [x] **2.2 JWT auth middleware**
  - **Dependencies**: 2.1
  - **Description**: Копирование и адаптация `auth.go` из QM (~530 строк).
    QM middleware уже является упрощённой версией (без RoleOverrideProvider из AM).
    Ключевые компоненты:
    - `AuthClaims` struct (Subject, SubjectType, Roles, Scopes, ClientID) +
      методы `HasRole`, `HasAnyRole`, `HasScope`, `HasAnyScope`
    - `keycloakClaims` — внутренний тип для парсинга JWT (RealmAccess, Groups, Scope)
    - `JWTAuth` struct + `NewJWTAuth(jwksURL, caCertPath, issuer, adminGroups,
      readonlyGroups, jwksClientTimeout, jwksRefreshInterval, jwtLeeway, logger)`
    - `httpClientWithCA(caCertPath, timeout)` — HTTP-клиент с опциональным CA
    - `Middleware()` — извлечение Bearer token, парсинг JWT (RS256, Expiration, Leeway)
    - `buildAuthClaims` → `buildUserClaims` (Groups → Role) / `buildSAClaims` (Scope → Scopes)
    - `ClaimsFromContext`, `SubjectFromContext` — хелперы контекста
    - `JWTAuth.Close()` — no-op, вызывается через defer в main.go
    Отличия от QM:
    - Upload авторизация: role `admin` ИЛИ scope `files:write`
      (QM: `admin`/`readonly` ИЛИ `files:read`)
    - `KeycloakReadinessChecker` — оставить (проверка JWKS endpoint),
      пригодится для readiness check в Phase 3.7
  - **Creates**:
    - `src/ingester-module/internal/api/middleware/auth.go`
  - **Links**:
    - Паттерн: `src/query-module/internal/api/middleware/auth.go`

- [x] **2.3 Logging и Metrics middleware**
  - **Dependencies**: None
  - **Description**:
    **logging.go** (~70 строк, копия из QM):
    - `responseWriter` обёртка — перехват statusCode и bytes, метод `Unwrap()`
      для совместимости с `http.ResponseController`
    - `RequestLogger(logger)` middleware — уровни: INFO (1xx-3xx), WARN (4xx), ERROR (5xx).
      Поля: method, path, status, duration, bytes, remote_addr
    **metrics.go** (~100 строк, адаптация из QM):
    - `im_http_requests_total` (Counter, labels: method, path, status)
    - `im_http_request_duration_seconds` (Histogram, labels: method, path)
    - `metricsResponseWriter` обёртка — перехват statusCode + `Unwrap()`
    - `normalizePath` — для IM проще чем в QM: все пути статические
      (`/health/live`, `/health/ready`, `/metrics`, `/api/v1/files/upload`),
      UUID-нормализация не нужна (IM не имеет path-параметров)
    Бизнес-метрики (im_uploads_total, im_upload_duration_seconds и др.)
    будут добавлены в Phase 3.
  - **Creates**:
    - `src/ingester-module/internal/api/middleware/logging.go`
    - `src/ingester-module/internal/api/middleware/metrics.go`
  - **Links**:
    - Паттерн: `src/query-module/internal/api/middleware/logging.go`
    - Паттерн: `src/query-module/internal/api/middleware/metrics.go`

- [x] **2.4 Обновление main.go и server.go**
  - **Dependencies**: 2.1, 2.2, 2.3
  - **Description**: Интеграция Phase 2 в main.go. Порядок инициализации:
    1. `config.Load()` — полная конфигурация
    2. `config.SetupLogger(cfg)` — slog JSON/text
    3. `middleware.NewJWTAuth(cfg.JWKSURL, cfg.CACertPath, cfg.JWTIssuer,
       cfg.RoleAdminGroups, cfg.RoleReadonlyGroups, cfg.JWKSClientTimeout,
       cfg.JWKSRefreshInterval, cfg.JWTLeeway, logger)`, defer `jwtAuth.Close()`
    4. `handlers.NewHealthHandler()` — readiness пока stub (Admin Module checker
       и JWKS checker будут в Phase 3.7)
    5. `handlers.NewAPIHandler(healthHandler, nil, logger)` — uploadService = nil (Phase 3)
    6. `server.New(cfg, logger, apiHandler,
       middleware.MetricsMiddleware(),
       middleware.RequestLogger(logger),
       server.JWTAuthWithExclusions(jwtAuth.Middleware(), "/health/", "/metrics"))`
    Порядок middleware: metrics → logging → JWT (с exclusions).
    Порядок важен: metrics первым = считает все запросы включая 401.
    server.go: `HTTPWriteTimeout` = 600s уже задан в Phase 1.5.
    IM НЕ имеет шагов database/repository/cache (stateless).
  - **Creates**:
    - `src/ingester-module/cmd/ingester-module/main.go` (обновление)

### Критерии завершения Phase 2

- [x] Все подпункты завершены (2.1–2.4)
- [x] Запрос без JWT → 401 (`UNAUTHORIZED`)
- [x] Запрос с невалидным/просроченным JWT → 401
- [x] Запрос с JWT без role admin и без scope files:write → 403 (`FORBIDDEN`)
- [x] Запрос с JWT admin → проходит (stub 501 от upload)
- [x] `/health/live`, `/health/ready`, `/metrics` доступны без JWT
- [x] Логи в JSON формате (`IM_LOG_FORMAT=json`), уровни корректны (INFO/WARN/ERROR)
- [x] `/metrics` содержит `im_http_requests_total` и `im_http_request_duration_seconds`
- [x] Обязательные переменные: приложение не стартует без `IM_JWKS_URL`, `IM_ADMIN_URL`,
  `IM_CLIENT_ID`, `IM_CLIENT_SECRET`
- [x] `go test ./...` проходит
- [x] `go vet ./...` без ошибок
- [x] `make lint` проходит без ошибок

---

## Phase 3: Бизнес-логика (upload pipeline, SE selection, file registration)

**Dependencies**: Phase 0, Phase 2
**Status**: Done

### Описание

Ядро Ingester Module: Admin Module HTTP-клиент, SE HTTP-клиент,
алгоритм Sequential Fill для выбора SE, upload pipeline (temp-file → SE → register),
retry при 507, topologymetrics.
Результат: полностью работающий endpoint `POST /api/v1/files/upload`.

### Подпункты

- [x] **3.1 Admin Module HTTP-клиент**
  - **Dependencies**: None
  - **Description**: `adminclient/client.go` — HTTP-клиент к Admin Module.
    Адаптация QM adminclient (~200 строк) с расширением для IM.
    **Методы:**
    - `GetToken(ctx) (string, error)` — client_credentials grant к Keycloak
      через AM proxy `{adminURL}/auth/token` (по паттерну QM, endpoint
      не документирован в OpenAPI, но работает как proxy к Keycloak token endpoint).
      Кэширование: double-check locking, `expiresAt = now + expires_in - 30s`.
    - `GetStorageElements(ctx, mode, status string) ([]SEInfo, error)` —
      `GET /api/v1/storage-elements?mode={mode}&status={status}`.
      Парсинг: `StorageElementListResponse.items` → `[]SEInfo`.
      **Внимание**: `available_bytes` nullable в AM API — если nil, считать 0
      (SE без информации о месте не подходит для upload).
    - `RegisterFile(ctx, req FileRegisterRequest) (*FileRecord, error)` —
      `POST /api/v1/files`. Добавляет `Authorization: Bearer <SA token>`.
    - `CheckHealth(ctx) error` — `GET /health/ready`, для readiness check IM.
    **Структуры:**
    ```
    SEInfo {
      ID, Name, URL, Mode, Status string
      AvailableBytes *int64   // nullable в AM API
      Priority       int      // после Phase 0
    }
    FileRegisterRequest {
      FileID           string  // UUID, required — из ответа SE
      OriginalFilename string  // required
      ContentType      string  // required
      Size             int64   // required
      Checksum         string  // SHA-256, required
      StorageElementID string  // UUID, required — ID записи SE в AM
      RetentionPolicy  string  // "temporary"|"permanent", required
      UploadedBy       string  // sub из JWT конечного пользователя
      Description      *string // опционально
      Tags             []string// опционально
      TTLDays          *int    // required для temporary (1-365)
    }
    ```
    **Важно: `uploaded_by` gap** — AM извлекает `uploaded_by` из JWT `sub` вызывающего.
    Если IM вызывает с SA-токеном, `uploaded_by` = SA client ID, а не конечный пользователь.
    Решение: AM API `POST /api/v1/files` уже принимает все поля включая контекст.
    IM передаёт `uploaded_by` (из пользовательского JWT) как поле `FileRegisterRequest`.
    Если AM API не поддерживает `uploaded_by` в запросе — потребуется доработка AM
    (добавить опциональное поле, приоритет над JWT sub при наличии scope `files:write`).
    - TLS: `buildTLSConfig(caCertPath)` — опциональный CA (по паттерну QM)
    - Таймаут: `IM_ADMIN_TIMEOUT`
  - **Creates**:
    - `src/ingester-module/internal/adminclient/client.go`
  - **Links**:
    - Паттерн: `src/query-module/internal/adminclient/client.go`
    - AM API: `docs/api-contracts/admin-module-openapi.yaml`

- [x] **3.2 SE HTTP-клиент (upload)**
  - **Dependencies**: None
  - **Description**: `seclient/client.go` — HTTP-клиент для загрузки файлов в SE.
    Адаптация QM seclient (Download → Upload, ~150 строк).
    **Метод:**
    - `Upload(ctx, seURL string, file io.ReadSeeker, filename, contentType string,
      description string, tags []string) (*UploadResult, error)` —
      `POST {seURL}/api/v1/files/upload` (multipart/form-data).
      Реализация: `io.Pipe` + `multipart.Writer` в горутине для streaming:
      горутина пишет multipart fields (file из io.ReadSeeker, description, tags как JSON-строка)
      в `PipeWriter`, HTTP-клиент читает из `PipeReader`.
      Добавляет `Authorization: Bearer <SA token>` через TokenProvider.
    **Структура `UploadResult`** (поля из ответа SE, без IM-level полей):
    ```
    UploadResult {
      FileID           string  // UUID
      OriginalFilename string
      ContentType      string
      Size             int64
      Checksum         string  // SHA-256
      UploadedBy       string  // sub из SA JWT (не конечный пользователь)
      UploadedAt       time.Time
      Description      *string
      Tags             []string
      Status           string  // "active"
    }
    ```
    **Внимание**: SE НЕ возвращает `retention_policy`, `ttl_days`, `expires_at` —
    это IM-level поля, устанавливаемые при регистрации в AM.
    `io.ReadSeeker` вместо `io.Reader` — для поддержки retry (Seek(0,0) при 507).
    **Обработка ошибок SE:**
    - 201 → успех, парсинг UploadResult
    - 507 → `ErrStorageFull` (sentinel error для retry в upload pipeline)
    - 413 → `ErrFileTooLarge` (SE имеет своё ограничение размера)
    - остальные → generic error с HTTP status
    - TLS: `buildTLSConfig(caCertPath)` — опциональный CA
    - Таймаут: `IM_SE_UPLOAD_TIMEOUT` (10m)
    - `TokenProvider func(ctx) (string, error)` — обычно adminclient.GetToken
    - `MaxIdleConnsPerHost: 10` (по паттерну QM)
  - **Creates**:
    - `src/ingester-module/internal/seclient/client.go`
  - **Links**:
    - Паттерн: `src/query-module/internal/seclient/client.go`
    - SE API: `docs/api-contracts/storage-element-openapi.yaml`

- [x] **3.3 Storage Element Selector (Sequential Fill)**
  - **Dependencies**: 3.1
  - **Description**: `service/selector.go` — алгоритм выбора SE:
    - `SelectSE(ctx, fileSize int64, retentionPolicy string,
      excludedSEIDs []string) (*adminclient.SEInfo, error)`
    - Логика:
      1. Определить mode: `temporary` → `"edit"`, `permanent` → `"rw"`
      2. Запросить список SE из Admin Module: `GetStorageElements(ctx, mode, "online")`
      3. Отсортировать по `Priority ASC`, при равном priority — по `Name ASC`
         (`sort.Slice` с двухуровневым компаратором)
      4. Для каждого SE по порядку:
         - Пропустить если ID в excludedSEIDs (map lookup для O(1))
         - Пропустить если `AvailableBytes == nil` (SE без данных о месте)
         - Проверить `*AvailableBytes >= fileSize`
         - Вернуть первый подходящий
      5. Если нет подходящего → `ErrNoStorageAvailable` (sentinel error)
    - Sentinel errors: `ErrNoStorageAvailable`
    - Prometheus: `im_se_selection_total{result=success|no_storage}`
  - **Creates**:
    - `src/ingester-module/internal/service/selector.go`

- [x] **3.4 Upload Service (pipeline + retry)**
  - **Dependencies**: 3.2, 3.3
  - **Description**: `service/upload.go` — координатор upload pipeline (~300 строк).
    ```
    Upload(ctx, file multipart.File, header *multipart.FileHeader,
      params UploadParams) (*UploadResponse, error)
    ```
    **Pipeline (9 шагов):**
      1. Валидация: retention_policy (temporary|permanent),
         ttl_days (1-365 для temporary, запрещён для permanent)
      2. Сохранить файл во временный файл (`os.CreateTemp` в emptyDir),
         `defer os.Remove(tempPath)` — гарантия очистки
      3. `stat(tempFile)` → fileSize. Проверка: fileSize ≤ MaxFileSize
         (после записи, не из header — header.Size может быть неточным)
      4. Вызвать Selector: `SelectSE(ctx, fileSize, retentionPolicy, excludedSEIDs)`
      5. `tempFile.Seek(0, 0)` — перемотать на начало (после записи или после retry!)
      6. SE client `Upload(ctx, seURL, tempFile, filename, contentType, description, tags)`
      7. При ошибке от SE:
         - `ErrStorageFull` (507): добавить SE в excludedSEIDs,
           **Seek(0, 0)** temp-file, повторить с п.4 (до `IM_MAX_RETRIES`)
         - `ErrFileTooLarge` (413): пробросить как 413 (SE limit < IM limit)
         - Другие ошибки: пробросить как SE_UPLOAD_FAILED
      8. При успехе: зарегистрировать файл в AM — `adminclient.RegisterFile(ctx, req)`.
         `req.UploadedBy = params.UploadedBy` (sub из JWT конечного пользователя,
         извлекается в handler, передаётся через UploadParams).
         `req.StorageElementID = selectedSE.ID` (ID записи SE в реестре AM).
         Если RegisterFile вернул ошибку → 502 ADMIN_UNAVAILABLE
         (файл остаётся на SE как сирота, будет удалён GC, лог Warning).
      9. Собрать `UploadResponse` из данных SE (UploadResult) + AM (FileRecord)
         + IM params (retention_policy, ttl_days, expires_at, uploaded_by)
    **Sentinel errors:**
    ```
    ErrFileTooLarge       // → 413
    ErrNoStorageAvailable // → 502 (из selector)
    ErrStorageFull        // → 507 (все retry исчерпаны)
    ErrSEUploadFailed     // → 502
    ErrAMUnavailable      // → 502
    ```
    **Prometheus метрики** (promauto, по паттерну QM download.go):
      - `im_uploads_total{retention_policy, status=success|error}` (Counter)
      - `im_upload_duration_seconds{retention_policy}` (Histogram, buckets для upload:
        0.5, 1, 5, 10, 30, 60, 120, 300, 600)
      - `im_upload_size_bytes{retention_policy}` (Histogram)
      - `im_se_upload_duration_seconds` (Histogram) — время передачи в SE
      - `im_active_uploads` (Gauge) — текущие активные загрузки
      - `im_retry_total{reason=507|se_error}` (Counter) — количество retry
    **Структуры:**
    ```
    UploadParams {
      UploadedBy      string   // sub из JWT конечного пользователя
      Description     *string
      Tags            []string
      RetentionPolicy string   // "temporary"|"permanent"
      TTLDays         *int     // 1-365 для temporary
    }
    UploadResponse {
      FileID, OriginalFilename, ContentType string
      Size int64; Checksum, UploadedBy string
      UploadedAt time.Time
      Description *string; Tags []string
      Status, RetentionPolicy string
      TTLDays *int; ExpiresAt *time.Time
      StorageElementID string
    }
    ```
  - **Creates**:
    - `src/ingester-module/internal/service/upload.go`

- [x] **3.5 Upload HTTP-обработчик**
  - **Dependencies**: 3.4
  - **Description**: `handlers/upload.go` — реализация `UploadFile`:
    1. Авторизация: `checkAuth(w, r)` — role `admin` ИЛИ scope `files:write`
       (обновить stub из Phase 1.4 на реальную проверку через `ClaimsFromContext`)
    2. Извлечь `uploaded_by` = `claims.Subject` из контекста JWT
    3. Парсинг multipart: `r.ParseMultipartForm(32 << 20)` (32 MB буфер на заголовки)
    4. `r.FormFile("file")` — извлечение файла (multipart.File + FileHeader)
    5. Извлечение form полей: description, retention_policy, ttl_days
    6. Валидация tags: `r.FormValue("tags")` → `json.Unmarshal` → `[]string`
    7. Сборка `UploadParams{UploadedBy, Description, Tags, RetentionPolicy, TTLDays}`
    8. `uploadService.Upload(ctx, file, header, params)` — вызов pipeline
    9. Ответ: `201 Created` + UploadResponse (JSON) через `writeJSON`
    **Error mapping** (по паттерну QM files.go — switch по sentinel errors):
    ```
    ErrFileTooLarge       → apierrors.FileTooLarge(w, msg)       // 413
    ErrNoStorageAvailable → apierrors.NoStorageAvailable(w, msg) // 502
    ErrStorageFull        → apierrors.StorageFull(w, msg)        // 507
    ErrSEUploadFailed     → apierrors.SEUploadFailed(w, msg)     // 502
    ErrAMUnavailable      → apierrors.AMUnavailable(w, msg)      // 502
    default               → apierrors.InternalError(w, msg)      // 500
    ```
    Обновление `handler.go`:
    - `NewAPIHandler(health, uploadService, logger)` — uploadService вместо nil
    - `checkAuth` — реальная реализация (HasAnyRole("admin") || HasAnyScope("files:write"))
  - **Creates**:
    - `src/ingester-module/internal/api/handlers/upload.go`
    - `src/ingester-module/internal/api/handlers/handler.go` (обновление)

- [x] **3.6 Topologymetrics**
  - **Dependencies**: None
  - **Description**: `service/dephealth.go` — настройка topologymetrics.
    Упрощённая версия QM dephealth: только 1 зависимость (без PostgreSQL).
    **Зависимости:**
    - Admin Module (critical) — HTTP checker к `/health/ready`
      (не `/health/live` — ready проверяет downstream зависимости AM)
    - Без PostgreSQL (IM stateless, нет собственной БД)
    - Без динамических SE endpoints (SE определяются на лету при каждом upload)
    **Упрощённая сигнатура** (без `*sql.DB` и `pgConnURL`):
    ```
    NewDephealthService(
      serviceID, group, adminModuleURL string,
      checkInterval time.Duration,
      isEntry bool,
      logger *slog.Logger,
    ) (*DephealthService, error)
    ```
    Использует только `dephealth.HTTP("admin-module", amDepOpts...)`.
    Import: `_ "github.com/BigKAA/topologymetrics/sdk-go/dephealth/checks/httpcheck"`.
    Без import `pgcheck` (не нужен).
    `Start(ctx)`, `Stop()`, `Health()` — по паттерну QM.
  - **Creates**:
    - `src/ingester-module/internal/service/dephealth.go`
  - **Links**:
    - Паттерн: `src/query-module/internal/service/dephealth.go`

- [x] **3.7 Интеграция в main.go и обновление readiness check**
  - **Dependencies**: 3.1, 3.4, 3.5, 3.6
  - **Description**: Финальная версия main.go. Порядок инициализации
    (шаги 1-6 уже существуют из Phase 2.4, добавляются 7-13):
    1. `config.Load()` — уже из Phase 2.4
    2. `config.SetupLogger(cfg)` — уже из Phase 2.4
    3. `middleware.NewJWTAuth(...)`, defer `jwtAuth.Close()` — уже из Phase 2.4
    **Новые шаги:**
    4. `adminclient.New(cfg.AdminURL, cfg.CACertPath, cfg.AdminTimeout,
       cfg.ClientID, cfg.ClientSecret, logger)` — AM HTTP-клиент
    5. `seclient.New(cfg.SECACertPath, cfg.SEUploadTimeout,
       adminClient.GetToken, logger)` — SE HTTP-клиент (TokenProvider = adminclient.GetToken)
    6. `service.NewSelectorService(adminClient)` — Sequential Fill selector
    7. `service.NewUploadService(selector, adminClient, seClient,
       cfg.MaxFileSize, cfg.DefaultTTLDays, cfg.MaxRetries, logger)` — upload pipeline
    8. `handlers.NewHealthHandler(adminClient, &jwtAuth.KeycloakReadinessChecker)` —
       readiness: AM + JWKS endpoint
    9. `handlers.NewAPIHandler(healthHandler, uploadService, logger)` — полный handler
    10. `server.New(cfg, logger, apiHandler, MetricsMiddleware(), RequestLogger(logger),
        JWTAuthWithExclusions(...))` — middleware уже из Phase 2.4
    11. `service.NewDephealthService(cfg.DephealthName, cfg.DephealthGroup,
        cfg.AdminURL, cfg.DephealthCheckInterval, cfg.DephealthIsEntry, logger)` —
        graceful start (warn при ошибке, не exit), defer `dephealthSvc.Stop()`
    12. `srv.Run()` — блокирующий запуск с graceful shutdown
    **Обновление health.go — readiness:**
    ```
    ReadinessChecker interface { CheckReady() (status, message string) }
    ```
    HealthHandler принимает два checker-а:
    - `adminChecker` — HTTP к AM `/health/ready` через adminclient.CheckHealth
    - `jwksChecker` — JWKS endpoint через KeycloakReadinessChecker (из Phase 2.2)
    Readiness response (по OpenAPI IM spec):
    ```json
    {
      "status": "ok|degraded|fail",
      "checks": {
        "admin_module": {"status":"ok","message":"..."},
        "jwks": {"status":"ok","message":"..."}
      }
    }
    ```
    `overallStatus(statuses...)` — агрегация: любой fail → fail, degraded → degraded
  - **Creates**:
    - `src/ingester-module/cmd/ingester-module/main.go` (финальная версия)
    - `src/ingester-module/internal/api/handlers/health.go` (обновление readiness)

### Критерии завершения Phase 3

- [x] Все подпункты завершены (3.1–3.7)
- [x] `POST /api/v1/files/upload` загружает файл в SE и возвращает 201
- [x] `uploaded_by` в ответе = `sub` из JWT конечного пользователя
- [x] `storage_element_id` в ответе = ID записи SE в реестре AM
- [x] Sequential Fill выбирает SE по priority (lowest value first)
- [x] Retry при 507 работает (исключает SE, Seek(0,0) temp-file, пробует следующий)
- [x] 413 от SE корректно пробрасывается клиенту
- [x] Файл регистрируется в Admin Module file registry (`POST /api/v1/files`)
- [x] Temp-файл удаляется после upload (или при ошибке) — defer cleanup
- [x] `/health/ready` проверяет Admin Module + JWKS endpoint
- [x] Topologymetrics: `app_dependency_health{dependency="admin-module"}` отображается
- [x] `/metrics` содержит `im_uploads_total`, `im_active_uploads`,
  `im_upload_duration_seconds`, `im_retry_total`
- [x] Error mapping: 413 FILE_TOO_LARGE, 502 NO_STORAGE_AVAILABLE/SE_UPLOAD_FAILED/
  ADMIN_UNAVAILABLE, 507 STORAGE_FULL
- [x] `go test ./...` проходит
- [x] `go vet ./...` без ошибок
- [x] `make lint` проходит без ошибок

---

## Phase 4: Сборка, деплой и интеграционные тесты

**Dependencies**: Phase 3
**Status**: Done

### Описание

Docker-образ, Helm chart, деплой в тестовый кластер,
Keycloak client, Gateway API routing, интеграционные тесты.
Результат: IM работает в Kubernetes, доступен через
`artstore.kryukov.lan/upload/*`, все 16 тестов проходят.
Docker-образ: `harbor.kryukov.lan/library/ingester-module:v0.1.0-2`.

### Подпункты

- [x] **4.1 Dockerfile**
  - **Dependencies**: None
  - **Description**: Multi-stage Dockerfile (по паттерну QM):
    Stage 1 (builder): `golang:1.25-alpine` — `ARG VERSION=dev`,
    `go mod download`, `CGO_ENABLED=0 go build` с `-ldflags "-X .../config.Version=$VERSION"`.
    Stage 2 (runtime): `alpine:3.19` — `ca-certificates`, non-root user `appuser`,
    `EXPOSE 8020`, `HEALTHCHECK CMD wget -qO- http://localhost:8020/health/live || exit 1`.
    **Без docker-compose.yaml** — по конвенции проекта разработка и тесты
    ведутся только через Kubernetes (CLAUDE.md). QM также не имеет docker-compose.
    Проверка: `make docker-build` собирает образ.
  - **Creates**:
    - `src/ingester-module/Dockerfile`
  - **Links**:
    - Паттерн: `src/query-module/Dockerfile`

- [x] **4.2 Helm chart (charts/ingester-module/)**
  - **Dependencies**: None
  - **Description**: Helm chart для production-деплоя:
    `Chart.yaml` (v0.1.0), `values.yaml`, `templates/` (deployment, service, httproute).
    Values: replicas (2), port (8020), все IM_* env vars, resources, probes, tls, dephealth.
    HTTPRoute: path prefix `/upload` + strip prefix через URLRewrite.
    Маршруты: /upload/api/v1/* → /api/v1/*, /upload/health/* → /health/*,
    /upload/metrics → /metrics.
    Probes: liveness (GET /health/live, period 10s), readiness (GET /health/ready, period 15s).
  - **Creates**:
    - `src/ingester-module/charts/ingester-module/Chart.yaml`
    - `src/ingester-module/charts/ingester-module/values.yaml`
    - `src/ingester-module/charts/ingester-module/templates/_helpers.tpl`
    - `src/ingester-module/charts/ingester-module/templates/deployment.yaml`
    - `src/ingester-module/charts/ingester-module/templates/service.yaml`
    - `src/ingester-module/charts/ingester-module/templates/httproute.yaml`
    - `src/ingester-module/charts/ingester-module/templates/configmap.yaml`
    - `src/ingester-module/charts/ingester-module/templates/secret.yaml`
  - **Links**:
    - Паттерн: `src/query-module/charts/query-module/`

- [x] **4.3 Keycloak client (artstore-ingester) — УЖЕ СУЩЕСТВУЕТ**
  - **Dependencies**: None
  - **Description**: Клиент `artstore-ingester` **уже полностью настроен** в
    `tests/helm/artstore-infra/files/artstore-realm.json`:
    - clientId: `artstore-ingester`
    - secret: `ingester-test-secret`
    - serviceAccountsEnabled: true
    - directAccessGrantsEnabled: false (только client_credentials)
    - fullScopeAllowed: false
    - Default scopes: `files:read`, `files:write`, `storage:read`
    **Действие**: Только проверить, что клиент корректно работает после деплоя IM
    (получить SA токен через `POST /auth/realms/artstore/protocol/openid-connect/token`).
  - **Creates**: Нет (уже существует)

- [x] **4.4 Тестовая инфраструктура (artstore-apps)**
  - **Dependencies**: 4.1
  - **Description**: Добавление IM в `tests/helm/artstore-apps/`:
    **templates/ingester-module.yaml** — Deployment (1 replica, как QM в тесте) + Service:
      - `strategy: Recreate` (как QM в тестовом chart, не RollingUpdate)
      - `initContainers`:
        - `wait-for-am` — `alpine:3.19`, curl к AM `/health/ready`, timeout 300s
        (Без wait-for-pg — IM stateless, не использует PostgreSQL)
      - Env (inline, как в QM тестовом chart, не через ConfigMap/Secret):
        `IM_PORT`, `IM_LOG_LEVEL`, `IM_LOG_FORMAT`,
        `IM_JWKS_URL` → `{{ keycloakHttpUrl }}/realms/artstore/protocol/openid-connect/certs`,
        `IM_JWT_ISSUER` → `https://artstore.kryukov.lan/realms/artstore`,
        `IM_ADMIN_URL` → `http://admin-module.<ns>.svc.cluster.local:<amPort>`
        (**HTTP**, не HTTPS — AM внутри кластера без TLS, по паттерну QM),
        `IM_CLIENT_ID` → `artstore-ingester`,
        `IM_CLIENT_SECRET` → `ingester-test-secret`,
        `IM_SE_UPLOAD_TIMEOUT=10m`, `IM_MAX_FILE_SIZE=1073741824`,
        `IM_HTTP_WRITE_TIMEOUT=600s`, `IM_MAX_RETRIES=3`,
        `IM_CA_CERT_PATH=/certs/ca.crt`, `IM_SE_CA_CERT_PATH=/certs/ca.crt`,
        `IM_DEFAULT_TTL_DAYS=30`,
        `DEPHEALTH_NAME=ingester-module`, `IM_DEPHEALTH_CHECK_INTERVAL=15s`,
        `IM_DEPHEALTH_GROUP=core-modules`, `DEPHEALTH_ISENTRY=false`,
        `IM_SHUTDOWN_TIMEOUT=5s`
      - Volumes:
        - `tls-certs` — secret `{{ .Values.tls.secretName }}` (как QM)
        - **`temp-upload`** — `emptyDir: { sizeLimit: "2Gi" }`,
          mountPath `/tmp/uploads` — для temp-файлов upload pipeline.
          Размер 2Gi = 2x IM_MAX_FILE_SIZE (1GB) для параллельных uploads
      - Resources: requests 100m/128Mi, limits 500m/256Mi
    **templates/ingester-module-httproute.yaml** — HTTPRoute
      (conditional: `if .Values.ingesterModule.httproute.enabled`):
      - `/upload/api/v1` → strip prefix → `/api/v1`
      - `/upload/health` → strip prefix → `/health`
      - `/upload/metrics` → strip prefix → `/metrics`
    **Обновление `values.yaml`** — секция `ingesterModule`:
      port, logLevel, logFormat, keycloakRealm, clientId, clientSecret,
      roleAdminGroups, roleReadonlyGroups, dephealthCheckInterval,
      dephealthGroup, shutdownTimeout, resources, seUploadTimeout,
      maxFileSize, maxRetries, defaultTtlDays
    **Обновление `_helpers.tpl`** — helper функции для IM:
      `artstore-apps.imImage`, `artstore-apps.im.selectorLabels`
    **Обновление `tests/Makefile`**:
    Переменные:
    ```
    IM_TAG   ?= v0.1.0-1
    IM_IMAGE := $(DOCKER_REGISTRY)/ingester-module
    IM_ROOT  := $(PROJECT_ROOT)/src/ingester-module
    IM_PORT  ?= 8020
    PF_IM    := 18020:$(IM_PORT)
    ```
    Экспорт для тестов:
    ```
    export IM_URL                  := http://localhost:18020
    export KC_IM_SA_CLIENT_ID      := artstore-ingester
    export KC_IM_SA_CLIENT_SECRET  := ingester-test-secret
    ```
    Targets:
    - `docker-build-im` / `docker-push-im` — сборка и push
    - `apps-up` — добавить `--set imTag=$(IM_TAG)` в helm upgrade
    - `kubectl wait` для `app.kubernetes.io/name=ingester-module`
    - port-forward: `svc/ingester-module $(PF_IM)`, PID в `.port-forward/ingester-module.pid`
    - `test-im` → `$(SCRIPTS_DIR)/test-im-all.sh`
    - `test-all: test-am test-qm test-im`
  - **Creates**:
    - `tests/helm/artstore-apps/templates/ingester-module.yaml`
    - `tests/helm/artstore-apps/templates/ingester-module-httproute.yaml`
    - `tests/helm/artstore-apps/values.yaml` (обновление)
    - `tests/helm/artstore-apps/templates/_helpers.tpl` (обновление)
    - `tests/Makefile` (обновление)

- [x] **4.5 Сборка Docker-образа и деплой**
  - **Dependencies**: 4.4
  - **Description**: Сборка Docker-образа `ingester-module:v0.1.0-1`,
    push в Harbor (`harbor.kryukov.lan/library/ingester-module:v0.1.0-1`).
    ```
    make docker-build-im IM_TAG=v0.1.0-1
    make docker-push-im IM_TAG=v0.1.0-1
    ```
    Деплой тестового окружения:
    ```
    make apps-up IM_TAG=v0.1.0-1
    ```
    Проверки:
    - 1 IM pod running, health checks passing, logs OK
    - `artstore.kryukov.lan/upload/health/live` через Gateway → 200
    - port-forward `localhost:18020/health/live` → 200
    - Keycloak SA token: `artstore-ingester` получает токен через client_credentials
    - `/metrics` содержит `im_` метрики
  - **Creates**:
    - Docker image `harbor.kryukov.lan/library/ingester-module:v0.1.0-1`

- [x] **4.6 Интеграционные тесты**
  - **Dependencies**: 4.5
  - **Description**: Bash + curl тесты в `tests/scripts/` (по паттерну QM, используя `lib.sh`):
    - `test-im-health.sh` (3 теста):
      1. GET /health/live → 200, status=ok, service=ingester-module
      2. GET /health/ready → 200, checks.admin_module.status=ok, checks.jwks.status=ok
      3. GET /metrics → 200, содержит `go_goroutines` + `im_http_requests_total`
    - `test-im-auth.sh` (3 теста):
      4. POST /api/v1/files/upload без JWT → 401 UNAUTHORIZED
      5. POST /api/v1/files/upload с viewer JWT (readonly, без scope files:write) → 403 FORBIDDEN
      6. POST /api/v1/files/upload с admin JWT + минимальный файл → не 401 и не 403
         (подтверждение что авторизация проходит; полный upload тестируется в upload-группе)
    - `test-im-upload.sh` (~10 тестов):
      7. Upload файла с retention_policy=temporary, ttl_days=7 → 201,
         проверка полей: file_id, checksum, storage_element_id, uploaded_by, status
      8. Upload файла с retention_policy=permanent → 201,
         проверка: ttl_days=null, expires_at=null
      9. Upload с description и tags → 201, проверка полей в ответе
      10. Upload без файла → 400 VALIDATION_ERROR
      11. Upload с невалидными tags (не JSON array) → 400 VALIDATION_ERROR
      12. Upload с ttl_days=0 → 400 VALIDATION_ERROR
      13. Upload с retention_policy=temporary без ttl_days → 400 VALIDATION_ERROR
      14. Проверка expires_at: upload temporary с ttl_days=1 → 201,
          проверка что expires_at ≈ uploaded_at + 1 day (±1 минута)
      15. Проверка регистрации в AM: GET `${AM_URL}/api/v1/files/{file_id}`
          → 200, file_id совпадает, storage_element_id совпадает
      16. **Cross-module**: Проверка доступности через QM:
          GET `${QM_URL}/api/v1/files/{file_id}/download` → 200
          (Зависимость: QM должен быть задеплоен. Тест мягкий — pass с warning
          если QM недоступен, по аналогии с мягкими проверками в QM download tests)
    - `test-im-all.sh` — оркестратор (запуск всех групп, итоговый отчёт,
      поддержка `--skip-cross-module` для пропуска теста 16)
    Все скрипты используют общую библиотеку `lib.sh` (http_get/post, get_user_token,
    assert_status, test_pass/test_fail, print_summary).
    Обновление Makefile: `make test-im`, обновление `make test-all`.
  - **Creates**:
    - `tests/scripts/test-im-health.sh`
    - `tests/scripts/test-im-auth.sh`
    - `tests/scripts/test-im-upload.sh`
    - `tests/scripts/test-im-all.sh`
    - `tests/Makefile` (обновление test targets)

### Критерии завершения Phase 4

- [x] Все подпункты завершены (4.1–4.6, 4.3 уже выполнен)
- [x] Docker-образ собран и загружен в Harbor (`ingester-module:v0.1.0-2`)
- [x] 1 под IM работает в тестовом кластере Kubernetes (health checks passing)
- [x] Gateway API маршрутизирует `artstore.kryukov.lan/upload/*` → IM
- [x] SA токен `artstore-ingester` успешно получается через client_credentials
- [x] Все интеграционные тесты проходят: `make test-im` — 16 PASS / 0 FAIL
- [x] Upload через Gateway работает (end-to-end: клиент → Gateway → IM → SE → AM register)
- [x] Temp-файлы записываются в emptyDir volume `/tmp/uploads`
- [x] `go vet ./...` и `make lint` проходят
- [x] `helm lint` проходит для production chart (`charts/ingester-module/`)
- [x] `make lint-helm` проходит для тестового chart (`tests/helm/artstore-apps/`)

### Замечания Phase 4

- **IM_TOKEN_URL**: Добавлена env-переменная для прямого обращения к Keycloak token endpoint (вместо proxy через AM). Изменения: config.go, adminclient/client.go, main.go, Helm templates.
- **Keycloak protocolMapper**: Добавлен `client_id` mapper (oidc-usersessionmodel-note-mapper) для клиентов `artstore-ingester` и `artstore-query` — необходим для распознавания SA в AM.
- **SE replicas=1**: Edit SE (se-edit-1, se-edit-2) работают с replicas=1 как workaround проблемы leader hostname resolution в Deployment. Задача на рефакторинг: `plans/se-stateless-refactoring-plan.md`.

---

## Phase 5: Sequence-диаграммы и документация

**Dependencies**: Phase 4
**Status**: Done

### Описание

Создание sequence-диаграмм в формате drawio (по аналогии с QM),
обновление документации проекта.
Результат: полный набор диаграмм и актуальная документация.

### Подпункты

- [x] **5.1 Sequence-диаграмма: Upload файла**
  - **Dependencies**: None
  - **Description**: Создание drawio-диаграммы `im-file-upload-sequence.drawio`:
    Клиент → Gateway → Ingester → Admin Module (GET SE list) →
    Ingester → SE (POST upload) → Ingester → Admin Module (POST register) →
    Ingester → Gateway → Клиент.
    Включить: happy path, 507 retry path, error paths.
    Стиль и формат: по аналогии с `docs/design/qm-get-file-by-id-sequence.drawio`.
  - **Creates**:
    - `docs/design/im-file-upload-sequence.drawio`
  - **Links**:
    - Паттерн: `docs/design/qm-get-file-by-id-sequence.drawio`

- [x] **5.2 Sequence-диаграмма: SE Selection (Sequential Fill)**
  - **Dependencies**: None
  - **Description**: Создание drawio-диаграммы `im-se-selection-sequence.drawio`:
    Подробная последовательность выбора SE по Sequential Fill Algorithm:
    Ingester → AM (GET SE list) → Сортировка по priority →
    Проверка available_bytes → Выбор / Retry / Error.
    Включить: scenario с 507 retry и исключением SE.
  - **Creates**:
    - `docs/design/im-se-selection-sequence.drawio`

- [x] **5.3 Module-level диаграмма**
  - **Dependencies**: None
  - **Description**: Создание drawio-диаграммы `im-upload-modules.drawio`:
    Обзор модулей участвующих в upload: Client, Gateway, Ingester,
    Admin Module, Storage Element. Показать направления вызовов и протоколы.
    По аналогии с `docs/design/qm-get-file-by-id-modules.drawio`.
  - **Creates**:
    - `docs/design/im-upload-modules.drawio`
  - **Links**:
    - Паттерн: `docs/design/qm-get-file-by-id-modules.drawio`

- [x] **5.4 Обновление документации**
  - **Dependencies**: 5.1, 5.2, 5.3
  - **Description**: Обновить:
    - `docs/briefs/ingester-module.md` — добавить: Sequential Fill Algorithm,
      sync upload с temp-file, retry при 507, ссылки на диаграммы
    - `CLAUDE.md` — обновить таблицу модулей (IM статус, Keycloak client)
    - План: отметить все пункты как завершённые, перенести в `plans/archive/`
  - **Creates**:
    - `docs/briefs/ingester-module.md` (обновление)
    - `CLAUDE.md` (обновление)

### Критерии завершения Phase 5

- [x] Все подпункты завершены (5.1–5.4)
- [x] 3 drawio-диаграммы созданы и корректны
- [x] Документация обновлена и актуальна
- [x] План перенесён в `plans/archive/`

---

## Примечания

### Зависимости Go-модуля

| Пакет | Назначение |
|-------|------------|
| `github.com/go-chi/chi/v5` | HTTP router |
| `github.com/golang-jwt/jwt/v5` | JWT parsing |
| `github.com/MicahParks/jwkset` | JWKS storage |
| `github.com/MicahParks/keyfunc/v3` | JWT key function |
| `github.com/oapi-codegen/runtime` | oapi-codegen runtime |
| `github.com/prometheus/client_golang` | Prometheus метрики |
| `github.com/google/uuid` | UUID |
| `github.com/BigKAA/topologymetrics/sdk-go` | topologymetrics |

### Отличия от Query Module

| Аспект | Query Module | Ingester Module |
|--------|-------------|----------------|
| Собственная БД | Да (shared PG, свои индексы) | Нет (stateless) |
| Таблица миграций | `schema_migrations_qm` | Нет |
| LRU Cache | Да (метаданные файлов) | Нет |
| Repository | FileRepository (SELECT) | Нет |
| Admin Client | GetToken, GetStorageElement | GetToken, GetStorageElements, RegisterFile |
| SE Client | Download (streaming GET) | Upload (multipart POST) |
| HTTPWriteTimeout | 60s | 600s (upload) |
| Replicas | 1 | 2 (горизонтальное масштабирование) |
| Gateway prefix | /query | /upload |
| Keycloak scope | files:read | files:write |

### Оценка трудоёмкости

| Фаза | Контекстов AI | Описание |
|------|---------------|----------|
| Phase 0 | 1 | AM migration + priority field |
| Phase 1 | 1 | Каркас, OpenAPI, кодогенерация, stubs |
| Phase 2 | 1 | Config, JWT, logging, metrics |
| Phase 3 | 2 | Admin client, SE client, selector, upload pipeline |
| Phase 4 | 2 | Docker, Helm, K8s, интеграционные тесты (KC client уже есть) |
| Phase 5 | 1 | Drawio диаграммы, документация |

### Roadmap v0.2 (будущее)

- Async upload: `202 Accepted` + `GET /api/v1/uploads/{id}/status`
- PVC для persistent буфера файлов
- Background worker для отправки в удалённые SE
- Таблица `uploads` в PostgreSQL для tracking
- WebSocket/SSE для real-time status updates

---
