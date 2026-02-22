# План разработки: Admin Module (Go) v2.0.0

## Контекст

Admin Module — управляющий модуль системы Artsore, отвечающий за реестр Storage Elements, файловый реестр, управление Service Accounts и локальные дополнения ролей пользователей. В v2.0.0 аутентификация полностью делегирована Keycloak (IdP). Admin Module не выдаёт JWT и не управляет ключами — он получает JWT от API Gateway, проверяет claims и принимает решения по авторизации. OpenAPI-спецификация обновлена до v2.0.0 (29 endpoints). Технический дизайн зафиксирован в `docs/design/admin-module-design.md`.

## Метаданные

- **Версия плана**: 1.0.0
- **Дата создания**: 2026-02-22
- **Последнее обновление**: 2026-02-22
- **Статус**: In Progress

---

## История версий

- **v1.0.0** (2026-02-22): Начальная версия плана

---

## Текущий статус

- **Активная фаза**: Phase 2 ✅
- **Активный подпункт**: —
- **Последнее обновление**: 2026-02-22
- **Примечание**: Phase 2 завершена

---

## Оглавление

- [x] [Phase 1: Инфраструктура проекта и скелет сервера](#phase-1-инфраструктура-проекта-и-скелет-сервера)
- [x] [Phase 2: База данных, доменные модели и RBAC](#phase-2-база-данных-доменные-модели-и-rbac)
- [ ] [Phase 3: Внешние клиенты и JWT middleware](#phase-3-внешние-клиенты-и-jwt-middleware)
- [ ] [Phase 4: API handlers (29 endpoints)](#phase-4-api-handlers)
- [ ] [Phase 5: Фоновые задачи (sync SE, sync SA, topologymetrics)](#phase-5-фоновые-задачи)
- [ ] [Phase 6: Helm chart, интеграционное тестирование и деплой](#phase-6-helm-chart-интеграционное-тестирование-и-деплой)

---

## Принятые решения

| Решение | Выбор |
|---------|-------|
| Путь в монорепо | `src/admin-module/` (отдельный go.mod) |
| Go layout | Стандартный: `cmd/`, `internal/` (как SE) |
| Кодогенерация | oapi-codegen (типы + chi-server интерфейс) |
| HTTP | net/http + chi router (как SE) |
| PostgreSQL driver | pgx/v5 (pgxpool) — без ORM |
| Миграции БД | golang-migrate/migrate |
| Логирование | slog (stdlib) |
| Конфигурация | env-переменные через stdlib (без Viper) |
| TLS | Нет (HTTP внутри, TLS на API Gateway). TLS client — для исходящих к SE |
| JWT | Fallback валидация через JWKS Keycloak (основная — на gateway) |
| Keycloak API | HTTP-клиент к Admin REST API (без SDK) |

## Целевая структура директорий

```text
src/admin-module/
├── cmd/admin-module/main.go
├── internal/
│   ├── config/config.go, config_test.go
│   ├── api/
│   │   ├── generated/          # oapi-codegen: types.gen.go, server.gen.go
│   │   ├── handlers/           # admin_auth, admin_users, service_accounts,
│   │   │                       # storage_elements, files, idp, health
│   │   ├── middleware/         # auth.go, logging.go, metrics.go
│   │   └── errors/errors.go
│   ├── domain/
│   │   ├── model/              # user, service_account, storage_element, file, sync
│   │   └── rbac/               # effective role, group→role mapping
│   ├── repository/             # PostgreSQL CRUD (pgx, чистый SQL)
│   ├── service/                # бизнес-логика, sync
│   ├── keycloak/               # HTTP-клиент к Keycloak Admin API
│   ├── seclient/               # HTTP-клиент к Storage Elements
│   ├── database/               # pgxpool, migrations
│   │   └── migrations/
│   └── server/server.go
├── charts/admin-module/        # Helm chart
├── tests/                      # Интеграционные тесты
├── Dockerfile
├── Makefile
├── docker-compose.yaml
├── oapi-codegen-types.yaml
├── oapi-codegen-server.yaml
├── go.mod
└── README.md
```

## Git Workflow

Каждая фаза — отдельная feature branch:

- `feature/am-phase-1-infrastructure`
- `feature/am-phase-2-database`
- `feature/am-phase-3-clients`
- `feature/am-phase-4-api`
- `feature/am-phase-5-background`
- `feature/am-phase-6-helm`

Commits: `feat(admin-module): <subject>`. Теги образов: `v2.0.0-N`.

## Ключевые файлы-источники

| Файл | Назначение |
|------|-----------|
| `docs/api-contracts/admin-module-openapi.yaml` | OpenAPI контракт v2.0.0 (29 endpoints) |
| `docs/briefs/admin-module.md` | Бриф v2.0.0: архитектура, Keycloak, RBAC, sync |
| `docs/design/admin-module-design.md` | Технический дизайн: структура, DB schema, компоненты |
| `src/storage-element/` | Референс Go-паттернов (config, middleware, server, handlers) |
| `src/storage-element/internal/config/config.go` | Паттерн config из env vars |
| `src/storage-element/internal/server/server.go` | Паттерн HTTP server + graceful shutdown |
| `src/storage-element/internal/api/middleware/auth.go` | Паттерн JWT middleware |
| `src/storage-element/internal/service/dephealth.go` | Паттерн topologymetrics |

---

## Phase 1: Инфраструктура проекта и скелет сервера

**Dependencies**: None
**Status**: Done

### Описание

Создание фундамента Go-модуля: структура директорий, конфигурация из env vars, кодогенерация из OpenAPI v2.0.0, скелет HTTP-сервера с health endpoints, Docker-образ. По завершении сервер запускается в Docker и отвечает на health probes.

### Подпункты

- [x] **1.0 Финализация OpenAPI v2.0.0 контракта**
  - **Dependencies**: None
  - **Description**: Ревизия и финализация `docs/api-contracts/admin-module-openapi.yaml` (v2.0.0). Контракт был обновлён с v1.0.0 (36 endpoints) до v2.0.0 (29 endpoints) в ходе проектирования: удалены self-managed auth endpoints, добавлены role-override, IdP status, SA sync. Шаг включает: (1) верификация соответствия спецификации брифу v2.0.0 и техническому дизайну, (2) проверка всех schemas (CurrentUser, AdminUser, ServiceAccount, StorageElement, FileRecord, IdpStatus, SASyncResult, ErrorResponse и др.), (3) проверка security requirements (bearerAuth на всех endpoints кроме health/metrics), (4) валидация спецификации (`npx @redocly/cli lint`), (5) commit финальной версии. Это блокирующий шаг — кодогенерация (1.2) зависит от стабильного контракта.
  - **Creates**:
    - `docs/api-contracts/admin-module-openapi.yaml` (обновление/финализация)
  - **Links**:
    - `docs/briefs/admin-module.md` — бриф v2.0.0 (источник требований)
    - `docs/design/admin-module-design.md` — технический дизайн (детали schemas и endpoints)

- [x] **1.1 Структура проекта и конфигурация**
  - **Dependencies**: None
  - **Description**: Go-модуль (`src/admin-module/go.mod`), структура `cmd/` + `internal/`. Пакет `internal/config/`: парсинг 25 env-переменных (AM_PORT, AM_LOG_LEVEL, AM_LOG_FORMAT, AM_DB_HOST, AM_DB_PORT, AM_DB_NAME, AM_DB_USER, AM_DB_PASSWORD, AM_DB_SSL_MODE, AM_KEYCLOAK_URL, AM_KEYCLOAK_REALM, AM_KEYCLOAK_CLIENT_ID, AM_KEYCLOAK_CLIENT_SECRET, AM_KEYCLOAK_SA_PREFIX, AM_JWT_ISSUER, AM_JWT_JWKS_URL, AM_JWT_ROLES_CLAIM, AM_JWT_GROUPS_CLAIM, AM_DEPHEALTH_CHECK_INTERVAL, AM_SYNC_INTERVAL, AM_SYNC_PAGE_SIZE, AM_SA_SYNC_INTERVAL, AM_SE_CA_CERT_PATH, AM_ROLE_ADMIN_GROUPS, AM_ROLE_READONLY_GROUPS) через stdlib. Валидация обязательных полей, Go duration парсинг, значения по умолчанию. Настройка slog (JSON/text). Минимальный `main.go`. Unit-тесты config. Паттерн: `src/storage-element/internal/config/config.go`.
  - **Creates**:
    - `src/admin-module/go.mod`
    - `src/admin-module/cmd/admin-module/main.go`
    - `src/admin-module/internal/config/config.go`
    - `src/admin-module/internal/config/config_test.go`
  - **Links**:
    - `src/storage-element/internal/config/config.go` — паттерн
    - `docs/briefs/admin-module.md` (раздел 8. Конфигурация)

- [x] **1.2 Кодогенерация из OpenAPI v2.0.0 (oapi-codegen)**
  - **Dependencies**: 1.0, 1.1
  - **Description**: Конфигурация oapi-codegen: генерация Go-типов (`types.gen.go`) и chi-server интерфейса `ServerInterface` (`server.gen.go`) из `docs/api-contracts/admin-module-openapi.yaml` (v2.0.0, 29 endpoints). Makefile target `generate`. Заглушка `stub.go`, реализующая `ServerInterface` с ответами 501. Паттерн: `src/storage-element/oapi-codegen-*.yaml`.
  - **Creates**:
    - `src/admin-module/oapi-codegen-types.yaml`
    - `src/admin-module/oapi-codegen-server.yaml`
    - `src/admin-module/internal/api/generated/types.gen.go`
    - `src/admin-module/internal/api/generated/server.gen.go`
    - `src/admin-module/internal/api/handlers/stub.go`
    - `src/admin-module/Makefile`
  - **Links**:
    - `docs/api-contracts/admin-module-openapi.yaml` — OpenAPI v2.0.0
    - `src/storage-element/oapi-codegen-types.yaml` — паттерн
    - [oapi-codegen](https://github.com/oapi-codegen/oapi-codegen)

- [x] **1.3 HTTP-сервер, health endpoints, middleware**
  - **Dependencies**: 1.2
  - **Description**: Пакет `internal/server/`: chi-роутер, монтирование ServerInterface через `HandlerFromMux`, **без TLS** (HTTP, TLS на API Gateway), graceful shutdown (SIGINT/SIGTERM, configurable timeout). Health endpoints в `internal/api/handlers/health.go`: `/health/live` (200), `/health/ready` (заглушка — проверяет PostgreSQL + Keycloak), `/metrics` (promhttp.Handler). Пакет `internal/api/errors/errors.go`: конструкторы ErrorResponse для всех кодов. Middleware: `logging.go` (slog), `metrics.go` (Prometheus HTTP metrics). Обновление main.go. Паттерн: `src/storage-element/internal/server/server.go`.
  - **Creates**:
    - `src/admin-module/internal/server/server.go`
    - `src/admin-module/internal/api/handlers/health.go`
    - `src/admin-module/internal/api/errors/errors.go`
    - `src/admin-module/internal/api/middleware/logging.go`
    - `src/admin-module/internal/api/middleware/metrics.go`
  - **Links**:
    - `src/storage-element/internal/server/server.go` — паттерн
    - `src/storage-element/internal/api/errors/errors.go` — паттерн

- [x] **1.4 Dockerfile, docker-compose, .dockerignore**
  - **Dependencies**: 1.3
  - **Description**: Multi-stage Dockerfile (golang:1.25-alpine → alpine:3.19). **HTTP** health check (не HTTPS). Порт 8000. docker-compose.yaml с PostgreSQL 17 + Keycloak 26.1 + Admin Module. Маппинг портов, volumes. Проверка: `docker compose up` → `curl http://localhost:8000/health/live` = 200.
  - **Creates**:
    - `src/admin-module/Dockerfile`
    - `src/admin-module/docker-compose.yaml`
    - `src/admin-module/.dockerignore`
    - `src/admin-module/Makefile` (обновление: docker-build, docker-run)
  - **Links**:
    - `src/storage-element/Dockerfile` — паттерн
    - Harbor: `harbor.kryukov.lan/library/admin-module`

### Критерии завершения Phase 1

- [x] `go build ./...` компилируется без ошибок
- [x] `go test ./...` проходит (unit-тесты config)
- [x] `make generate` генерирует код из OpenAPI v2.0.0
- [x] Docker-образ собирается (`make docker-build TAG=v2.0.0-1`)
- [x] `docker compose up` запускает PostgreSQL + Keycloak + Admin Module
- [x] `curl http://localhost:8000/health/live` = `{"status":"ok",...}`
- [x] Все endpoints кроме health возвращают 501 Not Implemented
- [x] Логирование в JSON/text работает

---

## Phase 2: База данных, доменные модели и RBAC

**Dependencies**: Phase 1
**Status**: Done

### Описание

PostgreSQL подключение (pgxpool), миграции (golang-migrate), доменные модели, RBAC-логика (effective role, group→role mapping), слой репозиториев. Все покрыты unit-тестами.

### Подпункты

- [x] **2.1 PostgreSQL подключение и миграции**
  - **Dependencies**: None
  - **Description**: Пакет `internal/database/`: инициализация pgxpool (connection string из config), ping check, Close. Миграция `001_initial_schema.up.sql` и `.down.sql`: 5 таблиц (storage_elements, file_registry, service_accounts, role_overrides, sync_state), индексы, триггеры updated_at, начальная запись sync_state. golang-migrate embedded migrations. Функция `database.Connect(cfg) (*pgxpool.Pool, error)` и `database.Migrate(pool) error`. Unit-тесты с testcontainers-go (PostgreSQL).
  - **Creates**:
    - `src/admin-module/internal/database/database.go`
    - `src/admin-module/internal/database/database_test.go`
    - `src/admin-module/internal/database/migrations/001_initial_schema.up.sql`
    - `src/admin-module/internal/database/migrations/001_initial_schema.down.sql`
  - **Links**:
    - `docs/design/admin-module-design.md` (раздел 4. Схема базы данных)
    - [pgx](https://github.com/jackc/pgx)
    - [golang-migrate](https://github.com/golang-migrate/migrate)

- [x] **2.2 Доменные модели и RBAC**
  - **Dependencies**: None (параллельно с 2.1)
  - **Description**: Пакет `internal/domain/model/`: структуры AdminUser, RoleOverride, ServiceAccount, StorageElement, FileRecord, SyncState, SyncResult. Пакет `internal/domain/rbac/`: EffectiveRole(idpRoles, roleOverride) → string, maxRole(a, b), highestRole(roles), MapGroupsToRole(groups, adminGroups, readonlyGroups) → string. Правила: итоговая роль = max(IdP, local override), только повышение. Unit-тесты RBAC: все комбинации ролей (admin+readonly, override up, override ignored).
  - **Creates**:
    - `src/admin-module/internal/domain/model/user.go`
    - `src/admin-module/internal/domain/model/service_account.go`
    - `src/admin-module/internal/domain/model/storage_element.go`
    - `src/admin-module/internal/domain/model/file.go`
    - `src/admin-module/internal/domain/model/sync.go`
    - `src/admin-module/internal/domain/rbac/rbac.go`
    - `src/admin-module/internal/domain/rbac/rbac_test.go`
  - **Links**:
    - `docs/briefs/admin-module.md` (раздел 4. Аутентификация и авторизация)
    - `docs/design/admin-module-design.md` (раздел 5.3. Определение эффективной роли)

- [x] **2.3 Слой репозиториев (PostgreSQL CRUD)**
  - **Dependencies**: 2.1, 2.2
  - **Description**: Пакет `internal/repository/`: интерфейсы и реализации для каждой таблицы. `repository.go` — общий интерфейс `TxRunner` для транзакций. `storage_element.go` — Create, GetByID, List(mode, status, limit, offset), Update, Delete, Count. `file_registry.go` — Register, GetByID, List(filters, limit, offset), Update, Delete(soft), BatchUpsert(files) для sync, MarkDeletedExcept(seID, existingIDs). `service_account.go` — Create, GetByID, GetByClientID, List(status, limit, offset), Update, Delete. `role_override.go` — Upsert, GetByKeycloakUserID, Delete, List. `sync_state.go` — Get, UpdateSASyncAt, UpdateFileSyncAt. Все запросы — чистый SQL с pgx. Unit-тесты с testcontainers-go.
  - **Creates**:
    - `src/admin-module/internal/repository/repository.go`
    - `src/admin-module/internal/repository/storage_element.go`
    - `src/admin-module/internal/repository/file_registry.go`
    - `src/admin-module/internal/repository/service_account.go`
    - `src/admin-module/internal/repository/role_override.go`
    - `src/admin-module/internal/repository/sync_state.go`
    - `src/admin-module/internal/repository/repository_test.go`
  - **Links**:
    - `docs/design/admin-module-design.md` (раздел 4. Схема базы данных)

### Критерии завершения Phase 2

- [x] PostgreSQL подключение через pgxpool работает
- [x] Миграции применяются и откатываются корректно
- [x] Все 5 таблиц созданы с индексами и триггерами
- [x] RBAC: EffectiveRole корректно вычисляет итоговую роль
- [x] Репозитории: CRUD для всех таблиц, batch upsert для file_registry
- [x] `go test -race ./...` — все тесты проходят
- [x] Health /ready проверяет подключение к PostgreSQL (ping)

---

## Phase 3: Внешние клиенты и JWT middleware

**Dependencies**: Phase 2
**Status**: Pending

### Описание

HTTP-клиенты для Keycloak Admin API и Storage Elements, JWT middleware для извлечения claims и RBAC авторизации. После этой фазы Admin Module может взаимодействовать с Keycloak и SE.

### Подпункты

- [ ] **3.1 JWT middleware (claims extraction + RBAC)**
  - **Dependencies**: None
  - **Description**: `internal/api/middleware/auth.go`. Два слоя: (1) JWTAuth middleware — извлекает `Authorization: Bearer <token>`, валидирует подпись через JWKS (fallback, основная на gateway), извлекает claims (sub, preferred_username, email, realm_access.roles, groups, scope, client_id), помещает в context. Поддержка обоих типов субъектов: Admin User (roles/groups) и Service Account (scope/client_id). (2) RBAC helpers — `RequireRole(roles ...string)` middleware, `RequireScope(scopes ...string)` middleware, `RequireRoleOrScope(roles, scopes)`. Интеграция с role_overrides через repository. Публичные endpoints (/health/*, /metrics) без auth. Паттерн: `src/storage-element/internal/api/middleware/auth.go` (расширенный для claims-based RBAC). Unit-тесты с httptest.
  - **Creates**:
    - `src/admin-module/internal/api/middleware/auth.go`
    - `src/admin-module/internal/api/middleware/auth_test.go`
  - **Links**:
    - `src/storage-element/internal/api/middleware/auth.go` — паттерн JWT
    - `docs/briefs/admin-module.md` (раздел 4. RBAC)
    - [keyfunc JWKS](https://github.com/MicahParks/keyfunc)

- [ ] **3.2 Keycloak Admin API клиент**
  - **Dependencies**: None (параллельно с 3.1)
  - **Description**: Пакет `internal/keycloak/`: HTTP-клиент к Keycloak Admin REST API. `client.go` — New(url, realm, clientID, clientSecret, httpClient), автоматическое получение service account token через Client Credentials flow, кэширование токена (обновление за 30s до expiration). Модели (`models.go`): KeycloakUser, KeycloakClient, KeycloakGroup, TokenResponse. Операции: ListUsers(query), GetUser(id), GetUserGroups(id), ListClients(prefix), CreateClient, UpdateClient, DeleteClient, GetClientSecret, RegenerateClientSecret, RealmInfo. Unit-тесты с httptest mock server.
  - **Creates**:
    - `src/admin-module/internal/keycloak/client.go`
    - `src/admin-module/internal/keycloak/models.go`
    - `src/admin-module/internal/keycloak/client_test.go`
  - **Links**:
    - [Keycloak Admin REST API](https://www.keycloak.org/docs-api/latest/rest-api/index.html)
    - `docs/briefs/admin-module.md` (раздел 4. Keycloak)

- [ ] **3.3 SE HTTP-клиент**
  - **Dependencies**: None (параллельно с 3.1, 3.2)
  - **Description**: Пакет `internal/seclient/`: HTTP-клиент для взаимодействия с Storage Elements. `client.go` — New(caCertPath, tokenProvider), TLS с кастомным CA (AM_SE_CA_CERT_PATH). Info(ctx, seURL) — GET /api/v1/info, возвращает SEInfo (storage_id, mode, status, capacity). ListFiles(ctx, seURL, limit, offset) — GET /api/v1/files с пагинацией, возвращает FileListResponse. tokenProvider — функция, возвращающая JWT для авторизации на SE (от Keycloak). Unit-тесты с httptest.
  - **Creates**:
    - `src/admin-module/internal/seclient/client.go`
    - `src/admin-module/internal/seclient/client_test.go`
  - **Links**:
    - `docs/api-contracts/storage-element-openapi.yaml` — API SE

### Критерии завершения Phase 3

- [ ] JWT middleware: извлекает claims, определяет тип субъекта (User/SA)
- [ ] RBAC: RequireRole, RequireScope, RequireRoleOrScope middleware
- [ ] Role overrides применяются при определении effective role
- [ ] Keycloak клиент: аутентификация Client Credentials, кэширование токена
- [ ] Keycloak клиент: CRUD users (read), CRUD clients (SA)
- [ ] SE клиент: Info + ListFiles с TLS
- [ ] `go test -race ./...` — все тесты проходят

---

## Phase 4: API handlers

**Dependencies**: Phase 3
**Status**: Pending

### Описание

Реализация всех 29 endpoints. Сервисный слой (business logic) + HTTP handlers. По завершении Admin Module полностью функционален.

### Подпункты

- [ ] **4.1 Admin auth + Admin users (6 endpoints)**
  - **Dependencies**: None
  - **Description**: Сервис `internal/service/admin_users.go`: GetCurrentUser(claims) — данные из JWT + role override из БД. ListUsers(limit, offset) — пользователи из Keycloak + role overrides. GetUser(id) — из Keycloak + override. UpdateUser(id, update) — обновить override. DeleteUser(id) — удалить override. SetRoleOverride(id, role, createdBy) — создать/обновить override в БД, проверить существование в Keycloak. Handlers `internal/api/handlers/admin_auth.go` и `admin_users.go`: маппинг HTTP ↔ service, RBAC декораторы. 6 endpoints: GET /admin-auth/me, GET /admin-users, GET /admin-users/{id}, PUT /admin-users/{id}, DELETE /admin-users/{id}, POST /admin-users/{id}/role-override.
  - **Creates**:
    - `src/admin-module/internal/service/admin_users.go`
    - `src/admin-module/internal/api/handlers/admin_auth.go`
    - `src/admin-module/internal/api/handlers/admin_users.go`
  - **Links**:
    - `docs/api-contracts/admin-module-openapi.yaml` (admin-auth, admin-users)

- [ ] **4.2 Service accounts (6 endpoints)**
  - **Dependencies**: None (параллельно с 4.1)
  - **Description**: Сервис `internal/service/service_accounts.go`: Create(name, description, scopes) — создать в Keycloak (Client Credentials grant) + сохранить в БД, вернуть client_secret. List, Get, Update (+ sync в Keycloak), Delete (+ удалить в Keycloak), RotateSecret (regenerate в Keycloak). Handler `internal/api/handlers/service_accounts.go`. 6 endpoints: POST /service-accounts, GET /service-accounts, GET /service-accounts/{id}, PUT /service-accounts/{id}, DELETE /service-accounts/{id}, POST /service-accounts/{id}/rotate-secret.
  - **Creates**:
    - `src/admin-module/internal/service/service_accounts.go`
    - `src/admin-module/internal/api/handlers/service_accounts.go`
  - **Links**:
    - `docs/api-contracts/admin-module-openapi.yaml` (service-accounts)

- [ ] **4.3 Storage elements (7 endpoints)**
  - **Dependencies**: None (параллельно с 4.1, 4.2)
  - **Description**: Сервис `internal/service/storage_elements.go`: Discover(url) — вызов seclient.Info, вернуть DiscoverResponse. Create(name, url) — discover + сохранить в БД + запустить full sync (файловый реестр). List(mode, status, limit, offset). Get, Update, Delete. Sync(id) — делегирует storage_sync.SyncOne (Phase 5, заглушка на этом этапе). Handler `internal/api/handlers/storage_elements.go`. 7 endpoints: POST /storage-elements/discover, POST /storage-elements, GET /storage-elements, GET /storage-elements/{id}, PUT /storage-elements/{id}, DELETE /storage-elements/{id}, POST /storage-elements/{id}/sync.
  - **Creates**:
    - `src/admin-module/internal/service/storage_elements.go`
    - `src/admin-module/internal/api/handlers/storage_elements.go`
  - **Links**:
    - `docs/api-contracts/admin-module-openapi.yaml` (storage-elements)

- [ ] **4.4 Files registry (5 endpoints)**
  - **Dependencies**: None (параллельно с 4.1-4.3)
  - **Description**: Сервис `internal/service/file_registry.go`: Register(req) — валидация, проверка SE exists, INSERT в file_registry. List(filters, limit, offset). Get(fileID). Update(fileID, update). Delete(fileID) — soft delete (status → deleted). Handler `internal/api/handlers/files.go`. 5 endpoints: POST /files, GET /files, GET /files/{file_id}, PUT /files/{file_id}, DELETE /files/{file_id}.
  - **Creates**:
    - `src/admin-module/internal/service/file_registry.go`
    - `src/admin-module/internal/api/handlers/files.go`
  - **Links**:
    - `docs/api-contracts/admin-module-openapi.yaml` (files)

- [ ] **4.5 IdP endpoints + полная сборка**
  - **Dependencies**: 4.1, 4.2, 4.3, 4.4
  - **Description**: (a) Сервис для IdP: GetStatus — проверить доступность Keycloak, вернуть realm info, users_count, clients_count, last_sa_sync_at. SyncSA — делегирует sa_sync.SyncNow (Phase 5, заглушка). (b) Handler `internal/api/handlers/idp.go`. 2 endpoints: GET /idp/status, POST /idp/sync-sa. (c) Обновление health.go: readiness проверяет PostgreSQL (ping) + Keycloak (realm info). (d) Обновление server.go: все handlers вместо stub, middleware chain (logging → metrics → auth → handler). (e) Обновление main.go: инициализация всех компонентов (DB → Keycloak → SE client → repos → services → handlers → server), graceful shutdown. (f) Тестирование в Docker: docker compose up → curl все 29 endpoints.
  - **Creates**:
    - `src/admin-module/internal/api/handlers/idp.go`
    - Обновление: `health.go`, `server.go`, `main.go`
  - **Links**:
    - `docs/api-contracts/admin-module-openapi.yaml` (idp, health)
    - `src/storage-element/cmd/storage-element/main.go` — паттерн main.go

### Критерии завершения Phase 4

- [ ] Все 29 endpoints реализованы и соответствуют OpenAPI v2.0.0
- [ ] JWT claims-based авторизация работает (роли + scopes)
- [ ] Role overrides применяются корректно (повышение, не понижение)
- [ ] SA CRUD синхронизируется с Keycloak
- [ ] SE discover вызывает GET /api/v1/info на SE
- [ ] Files CRUD работает с PostgreSQL
- [ ] Ошибки в формате `{"error": {"code": "...", "message": "..."}}`
- [ ] Health /ready проверяет PostgreSQL + Keycloak
- [ ] `go test -race ./...` — все тесты проходят
- [ ] Ручное тестирование через curl в Docker (docker compose)

---

## Phase 5: Фоновые задачи

**Dependencies**: Phase 4
**Status**: Pending

### Описание

Фоновые процессы: периодическая синхронизация файлового реестра с SE, периодическая синхронизация SA с Keycloak, topologymetrics для мониторинга зависимостей.

### Подпункты

- [ ] **5.1 Синхронизация файлового реестра с SE**
  - **Dependencies**: None
  - **Description**: `internal/service/storage_sync.go`. StorageSyncService: Start(ctx) — горутина с ticker (AM_SYNC_INTERVAL, default 1h), Stop(). SyncAll(ctx) — получить SE со статусом online, для каждого параллельно (errgroup) SyncOne. SyncOne(ctx, seID) — (1) seclient.Info → обновить mode/status/capacity в БД, (2) постраничный seclient.ListFiles (AM_SYNC_PAGE_SIZE, default 1000) → batch upsert в file_registry (INSERT ON CONFLICT UPDATE), (3) пометить отсутствующие как deleted (MarkDeletedExcept), (4) обновить last_sync_at, last_file_sync_at, (5) вернуть SyncResult. Prometheus: admin_module_sync_duration_seconds, admin_module_sync_files_processed. Подключение SyncOne к handler POST /storage-elements/{id}/sync. Подключение SyncOne к handler POST /storage-elements (full sync при создании). Unit-тесты.
  - **Creates**:
    - `src/admin-module/internal/service/storage_sync.go`
    - `src/admin-module/internal/service/storage_sync_test.go`
    - Обновление: `storage_elements.go` (handler + service), `main.go`
  - **Links**:
    - `docs/briefs/admin-module.md` (раздел 6. Фоновые задачи, раздел 9. Синхронизация)

- [ ] **5.2 Синхронизация SA с Keycloak**
  - **Dependencies**: None (параллельно с 5.1)
  - **Description**: `internal/service/sa_sync.go`. SASyncService: Start(ctx) — горутина с ticker (AM_SA_SYNC_INTERVAL, default 15m), Stop(). SyncNow(ctx) — (1) ListClients из Keycloak с prefix sa_*, (2) List SA из БД, (3) reconciliation: в KC не локально → создать (source=keycloak), локально не в KC → создать в KC (source=local), в обоих → сравнить scopes, обновить по updated_at (last write wins), (4) обновить sync_state.last_sa_sync_at, (5) вернуть SASyncResult. Подключение к handler POST /idp/sync-sa. Prometheus: admin_module_sa_sync_duration_seconds. Unit-тесты.
  - **Creates**:
    - `src/admin-module/internal/service/sa_sync.go`
    - `src/admin-module/internal/service/sa_sync_test.go`
    - Обновление: `idp.go` (handler), `main.go`
  - **Links**:
    - `docs/briefs/admin-module.md` (раздел 6. Фоновые задачи — SA sync)

- [ ] **5.3 topologymetrics — мониторинг зависимостей**
  - **Dependencies**: None (параллельно с 5.1, 5.2)
  - **Description**: `internal/service/dephealth.go`. Интеграция с `github.com/BigKAA/topologymetrics/sdk-go/dephealth`. Две зависимости: (1) PostgreSQL — `sqldb.FromDB("postgresql", db, ...)`, critical; (2) Keycloak — `dephealth.NewHTTPCheck("keycloak", jwksURL, ...)`, critical. Метрики на /metrics: app_dependency_health, app_dependency_latency_seconds, app_dependency_status, app_dependency_status_detail. Обновление main.go. Паттерн: `src/storage-element/internal/service/dephealth.go`.
  - **Creates**:
    - `src/admin-module/internal/service/dephealth.go`
    - Обновление: `main.go`
  - **Links**:
    - `src/storage-element/internal/service/dephealth.go` — паттерн
    - [topologymetrics SDK](https://github.com/BigKAA/topologymetrics)

### Критерии завершения Phase 5

- [ ] Storage sync: периодический + ручной (POST /storage-elements/{id}/sync)
- [ ] Storage sync: full sync при регистрации SE (POST /storage-elements)
- [ ] Storage sync: batch upsert файлов, пометка отсутствующих как deleted
- [ ] SA sync: периодический + ручной (POST /idp/sync-sa)
- [ ] SA sync: reconciliation в обе стороны (local ↔ Keycloak)
- [ ] topologymetrics: PostgreSQL + Keycloak на /metrics
- [ ] `go test -race ./...` — все тесты проходят
- [ ] Docker compose: полный цикл (регистрация SE → sync → проверка файлового реестра)

---

## Phase 6: Helm chart, интеграционное тестирование и деплой

**Dependencies**: Phase 5
**Status**: Pending

### Описание

Helm chart для Kubernetes, интеграционные тесты в K8s (PostgreSQL + Keycloak + Admin Module + SE), Keycloak realm конфигурация, деплой в тестовый кластер.

### Подпункты

- [ ] **6.1 Keycloak realm конфигурация (JSON import)**
  - **Dependencies**: None
  - **Description**: JSON-файл конфигурации Keycloak realm `artsore` для автоматизированного развёртывания. Содержит: realm settings, clients (artsore-admin-ui public PKCE, artsore-ingester confidential CC, artsore-query confidential CC, artsore-admin-module confidential CC с admin roles), groups (artsore-admins, artsore-viewers), group mapper (groups → roles claim в JWT), brute force detection (5 attempts, 15 min lockout), начальный пользователь admin. Используется через Keycloak Realm Import (env `KC_IMPORT`). Скрипт валидации realm.
  - **Creates**:
    - `src/admin-module/deploy/keycloak/artsore-realm.json`
    - `src/admin-module/deploy/keycloak/README.md`
  - **Links**:
    - `docs/briefs/admin-module.md` (раздел 4. Keycloak Realm)
    - [Keycloak Realm Import](https://www.keycloak.org/server/importExport)

- [ ] **6.2 Production Helm chart (Deployment, Service, HTTPRoute)**
  - **Dependencies**: None (параллельно с 6.1)
  - **Description**: Helm chart `src/admin-module/charts/admin-module/`. Deployment (stateless, replicas configurable). Service (ClusterIP, port 8000). HTTPRoute (Gateway API) — маршрутизация /api/v1/* и /health/* с artsore.kryukov.lan. Secret (DB credentials, Keycloak credentials). ConfigMap (не-секретные env vars). Опциональный Certificate от cert-manager (если нужен mTLS внутри кластера). values.yaml с defaults. Probes: HTTP GET /health/live, /health/ready. `helm lint` проходит.
  - **Creates**:
    - `src/admin-module/charts/admin-module/Chart.yaml`
    - `src/admin-module/charts/admin-module/values.yaml`
    - `src/admin-module/charts/admin-module/templates/_helpers.tpl`
    - `src/admin-module/charts/admin-module/templates/deployment.yaml`
    - `src/admin-module/charts/admin-module/templates/service.yaml`
    - `src/admin-module/charts/admin-module/templates/httproute.yaml`
    - `src/admin-module/charts/admin-module/templates/configmap.yaml`
    - `src/admin-module/charts/admin-module/templates/secret.yaml`
  - **Links**:
    - `src/storage-element/charts/storage-element/` — паттерн Helm chart
    - `docs/design/admin-module-design.md` (раздел 12. Deployment)

- [ ] **6.3 Тестовая среда в K8s (PostgreSQL + Keycloak + Admin Module + SE)**
  - **Dependencies**: 6.1, 6.2
  - **Description**: Helm chart для тестовой среды в namespace `am-test`. Содержит: PostgreSQL (Deployment + PVC), Keycloak (Deployment + realm import), Admin Module (Deployment), 2 экземпляра SE (из существующего SE Helm chart или standalone Deployment), JWKS Mock (для SE JWT валидации). TLS Certificate через cert-manager. Init Job: создание БД, применение миграций, настройка Keycloak realm. Makefile в `src/admin-module/tests/` для оркестрации: test-env-up, test-env-down, test-env-status.
  - **Creates**:
    - `src/admin-module/tests/helm/am-test/Chart.yaml`
    - `src/admin-module/tests/helm/am-test/values.yaml`
    - `src/admin-module/tests/helm/am-test/templates/` (postgresql, keycloak, admin-module, se instances, init job)
    - `src/admin-module/tests/Makefile`
  - **Links**:
    - `src/storage-element/tests/helm/se-test/` — паттерн тестовой среды

- [ ] **6.4 Интеграционные тесты (bash + curl + jq)**
  - **Dependencies**: 6.3
  - **Description**: Интеграционные тесты — bash-скрипты с curl + jq, аналогично SE.

    **Тестовые сценарии (~25):**

    *Smoke (1-3):* health live, health ready (postgresql+keycloak ok), metrics
    *Admin auth (4):* GET /admin-auth/me → текущий пользователь, effective_role
    *Admin users (5-9):* list users, get user, set role override, verify effective role, delete override
    *Service accounts (10-15):* create SA, list SA, get SA, update SA scopes, rotate secret, delete SA
    *Storage elements (16-20):* discover SE, register SE (+ full sync), list SE, update SE, sync SE (manual)
    *Files (21-24):* register file, list files, update file metadata, soft delete file
    *IdP (25-26):* get IdP status, force sync SA
    *Errors (27-30):* без JWT→401, нет роли→403, SA без scope→403, конфликт→409

    Все тесты используют `lib.sh` (переиспользовать из SE tests или создать свою).
  - **Creates**:
    - `src/admin-module/tests/scripts/lib.sh`
    - `src/admin-module/tests/scripts/test-smoke.sh`
    - `src/admin-module/tests/scripts/test-admin-auth.sh`
    - `src/admin-module/tests/scripts/test-admin-users.sh`
    - `src/admin-module/tests/scripts/test-service-accounts.sh`
    - `src/admin-module/tests/scripts/test-storage-elements.sh`
    - `src/admin-module/tests/scripts/test-files.sh`
    - `src/admin-module/tests/scripts/test-idp.sh`
    - `src/admin-module/tests/scripts/test-errors.sh`
    - `src/admin-module/tests/scripts/test-all.sh`
  - **Links**:
    - `src/storage-element/tests/scripts/` — паттерн тестовых скриптов
    - `docs/api-contracts/admin-module-openapi.yaml` — API контракт

- [ ] **6.5 Деплой в K8s и верификация**
  - **Dependencies**: 6.2, 6.3, 6.4
  - **Description**: Финальная верификация: (1) Сборка Docker-образа Admin Module, push в Harbor. (2) Деплой тестовой среды `make test-env-up`. (3) Запуск интеграционных тестов `make test-all` — все PASS. (4) Деплой через production Helm chart. (5) Верификация: все endpoints через API Gateway (artsore.kryukov.lan). (6) README.md с инструкциями.
  - **Creates**:
    - Docker image в Harbor (admin-module)
    - K8s resources
    - `src/admin-module/README.md`
  - **Links**:
    - Harbor: `harbor.kryukov.lan/library/admin-module`
    - API Gateway: `artsore.kryukov.lan` → 192.168.218.180

### Критерии завершения Phase 6

- [ ] Keycloak realm `artsore` настроен (clients, groups, mappers)
- [ ] Docker-образ собран и pushed в Harbor
- [ ] Production Helm chart: `helm lint` проходит
- [ ] Тестовая среда разворачивается (PostgreSQL + Keycloak + Admin Module + SE)
- [ ] Интеграционные тесты проходят (~25-30 тестов)
- [ ] Admin Module работает за API Gateway (artsore.kryukov.lan)
- [ ] JWT от Keycloak принимается и авторизация работает
- [ ] Sync SE → файловый реестр обновляется
- [ ] Sync SA → согласованность с Keycloak
- [ ] /metrics доступен для Prometheus
- [ ] README.md с инструкциями по деплою

---

## Зависимости между фазами

```text
Phase 1 (Infrastructure) → Phase 2 (Database + Domain) → Phase 3 (Clients + JWT)
                                                              ↓
                                                         Phase 4 (API Handlers)
                                                              ↓
                                                         Phase 5 (Background Tasks)
                                                              ↓
                                                         Phase 6 (Helm + Tests + Deploy)
```

## Рекомендуемый порядок AI-сессий

| Сессия | Подпункты | Ветка | Описание |
|--------|-----------|-------|----------|
| A | 1.0, 1.1, 1.2 | `feature/am-phase-1-infrastructure` | API contract + config + codegen |
| B | 1.3, 1.4 | `feature/am-phase-1-infrastructure` | HTTP server + Docker |
| C | 2.1, 2.2 | `feature/am-phase-2-database` | PostgreSQL + domain + RBAC |
| D | 2.3 | `feature/am-phase-2-database` | Repository layer + tests |
| E | 3.1, 3.2, 3.3 | `feature/am-phase-3-clients` | JWT + Keycloak + SE clients |
| F | 4.1, 4.2 | `feature/am-phase-4-api` | Admin users + SA handlers |
| G | 4.3, 4.4 | `feature/am-phase-4-api` | SE + Files handlers |
| H | 4.5 | `feature/am-phase-4-api` | IdP + full assembly + Docker test |
| I | 5.1, 5.2, 5.3 | `feature/am-phase-5-background` | Sync + topologymetrics |
| J | 6.1, 6.2 | `feature/am-phase-6-helm` | Keycloak realm + Helm chart |
| K | 6.3, 6.4 | `feature/am-phase-6-helm` | Test env + integration tests |
| L | 6.5 | `feature/am-phase-6-helm` | Deploy + README |

## Примечания

- **Порядок инициализации при старте**: config → logger → DB connect → migrate → Keycloak client → health check Keycloak → SE client → repos → services → handlers → JWT middleware → server → background tasks (storage sync, SA sync, topologymetrics) → initial SA sync → server.Run()
- **Graceful shutdown**: остановка HTTP → фоновые задачи (storage sync, SA sync) → topologymetrics → DB close
- **Без TLS для API**: HTTP внутри кластера, TLS termination на API Gateway. TLS client для исходящих к SE (AM_SE_CA_CERT_PATH)
- **Health /ready**: проверяет PostgreSQL (ping) + Keycloak (realm info GET)
- **OpenAPI v2.0.0**: спецификация уже обновлена и зафиксирована
