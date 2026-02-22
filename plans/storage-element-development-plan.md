# План разработки: Storage Element (Go)

## Контекст

Storage Element (SE) — первый модуль системы Artsore для реализации на Go. SE отвечает за физическое хранение файлов на локальной файловой системе с гарантией атомарности через WAL. Модуль автономный, может работать вне кластера K8s (bare metal, Docker, edge). Все проектные решения, API-контракт (12 endpoints) и бриф уже зафиксированы. Цель — реализовать полнофункциональный SE, собрать Docker-образ и развернуть в тестовом K8s кластере.

## Метаданные

- **Версия плана**: 1.1.0
- **Дата создания**: 2026-02-21
- **Последнее обновление**: 2026-02-22
- **Статус**: In Progress

---

## История версий

- **v1.1.0** (2026-02-22): Переработка Phase 5.3 (интеграционные тесты в K8s вместо Docker), расширение Phase 6 (7 подпунктов: JWKS Mock, Test Helm chart, Init scripts, 30 тестов, Makefile, Production Helm chart, деплой)
- **v1.0.0** (2026-02-21): Начальная версия плана

---

## Текущий статус

- **Активная фаза**: Phase 6
- **Активный подпункт**: 6.6 (Основной Helm chart)
- **Последнее обновление**: 2026-02-22
- **Примечание**: Phase 5 (5.1-5.2) завершена. Phase 6.1 (JWKS Mock Server) завершена. Phase 6.2 (Helm chart se-test) завершена. Phase 6.3 (Init scripts) завершена — lib.sh + init-data.sh, 8 шагов инициализации, очистка data-файлов se-ar (только attr.json). Phase 6.4 (Тестовые bash-скрипты) завершена — 8 скриптов, 30 тестов (smoke 1-3, files 4-11, modes 12-18, replica 19-22, data 23-24, gc/reconcile 25-26, errors 27-30, test-all runner). Phase 6.5 (Makefile) завершена — 18 targets (docker-build, test-env-up/down/status/logs, port-forward-start/stop/status, 7 test targets, clean). 107+ unit-тестов SE, race detector clean.

---

## Оглавление

- [x] [Phase 1: Инфраструктура проекта и скелет сервера](#phase-1-инфраструктура-проекта-и-скелет-сервера)
- [x] [Phase 2: Ядро хранилища (WAL, attr.json, файловое хранилище, индекс, режимы)](#phase-2-ядро-хранилища)
- [x] [Phase 3: API handlers, JWT middleware, Prometheus метрики](#phase-3-api-handlers-jwt-middleware-prometheus-метрики)
- [x] [Phase 4: Фоновые процессы (GC, Reconciliation, topologymetrics)](#phase-4-фоновые-процессы)
- [ ] [Phase 5: Replicated mode (Leader/Follower)](#phase-5-replicated-mode) *(5.1-5.2 завершены)*
- [ ] [Phase 6: Helm charts, интеграционное тестирование и деплой в Kubernetes](#phase-6-helm-charts-интеграционное-тестирование-и-деплой-в-kubernetes)

---

## Принятые решения

| Решение | Выбор |
|---------|-------|
| Путь в монорепо | `src/storage-element/` (отдельный go.mod) |
| Go layout | Стандартный: `cmd/`, `internal/`, `pkg/` |
| Кодогенерация | oapi-codegen (типы + chi-server интерфейс) |
| Логирование | slog (stdlib) |
| HTTP | net/http + chi router |
| WAL | Файловый (без БД) |
| Конфигурация | env-переменные через stdlib (без Viper) |

## Целевая структура директорий

```
src/storage-element/
├── cmd/storage-element/main.go
├── internal/
│   ├── config/config.go
│   ├── api/
│   │   ├── generated/          # oapi-codegen: types.gen.go, server.gen.go
│   │   ├── handlers/           # files.go, system.go, mode.go, maintenance.go, health.go
│   │   ├── middleware/         # auth.go, logging.go, metrics.go
│   │   └── errors/errors.go
│   ├── domain/
│   │   ├── mode/state_machine.go
│   │   └── model/metadata.go
│   ├── storage/
│   │   ├── wal/                # WAL-движок (файловый)
│   │   ├── attr/               # Чтение/запись attr.json
│   │   ├── filestore/          # Файловые операции
│   │   └── index/              # In-memory индекс (map + RWMutex)
│   ├── service/
│   │   ├── upload.go, download.go
│   │   ├── gc.go, reconcile.go
│   │   └── dephealth.go
│   ├── replica/                # Leader/Follower (Phase 5)
│   └── server/server.go
├── charts/storage-element/     # Helm chart (Phase 6)
├── tests/                      # Integration tests
├── oapi-codegen-types.yaml
├── oapi-codegen-server.yaml
├── Dockerfile
├── Makefile
├── docker-compose.yaml
├── go.mod
└── go.sum
```

## Git Workflow

Каждая фаза — отдельная feature branch:
- `feature/se-phase-1-infrastructure`
- `feature/se-phase-2-core`
- `feature/se-phase-3-api`
- `feature/se-phase-4-background`
- `feature/se-phase-5-replicated`
- `feature/se-phase-6-helm`

Commits: `feat(storage-element): <subject>`. Теги образов: `v1.0.0-N`.

---

## Phase 1: Инфраструктура проекта и скелет сервера

**Dependencies**: None
**Status**: Done

### Описание

Создание фундамента: Go-модуль, структура директорий, конфигурация, кодогенерация из OpenAPI, скелет HTTP-сервера с TLS и health endpoints, Docker-образ. По завершении сервер запускается в Docker и отвечает на health probes.

### Подпункты

- [x] **1.1 Структура проекта и конфигурация**
  - **Dependencies**: None
  - **Description**: Go-модуль (`src/storage-element/go.mod`), структура `cmd/` + `internal/`. Пакет `internal/config/`: парсинг 17 env-переменных (SE_PORT, SE_STORAGE_ID, SE_DATA_DIR, SE_WAL_DIR, SE_MODE, SE_MAX_FILE_SIZE, SE_GC_INTERVAL, SE_RECONCILE_INTERVAL, SE_JWKS_URL, SE_TLS_CERT, SE_TLS_KEY, SE_LOG_LEVEL, SE_LOG_FORMAT, SE_REPLICA_MODE, SE_INDEX_REFRESH_INTERVAL, SE_DEPHEALTH_CHECK_INTERVAL) через stdlib. Валидация обязательных полей, Go duration парсинг, значения по умолчанию. Настройка slog (JSON/text). Минимальный `main.go`. Unit-тесты config.
  - **Creates**:
    - `src/storage-element/go.mod`
    - `src/storage-element/cmd/storage-element/main.go`
    - `src/storage-element/internal/config/config.go`
    - `src/storage-element/internal/config/config_test.go`
  - **Links**:
    - [Go Project Layout](https://github.com/golang-standards/project-layout)

- [x] **1.2 Кодогенерация из OpenAPI (oapi-codegen)**
  - **Dependencies**: 1.1
  - **Description**: Конфигурация oapi-codegen: генерация Go-типов (`types.gen.go`) и chi-server интерфейса `ServerInterface` (`server.gen.go`) из `docs/api-contracts/storage-element-openapi.yaml`. Makefile target `generate`. Заглушка `stub.go`, реализующая `ServerInterface` с ответами 501.
  - **Creates**:
    - `src/storage-element/oapi-codegen-types.yaml`
    - `src/storage-element/oapi-codegen-server.yaml`
    - `src/storage-element/internal/api/generated/types.gen.go`
    - `src/storage-element/internal/api/generated/server.gen.go`
    - `src/storage-element/internal/api/handlers/stub.go`
    - `src/storage-element/Makefile`
  - **Links**:
    - [oapi-codegen](https://github.com/oapi-codegen/oapi-codegen)
    - `docs/api-contracts/storage-element-openapi.yaml`

- [x] **1.3 HTTP-сервер, TLS, health endpoints**
  - **Dependencies**: 1.2
  - **Description**: Пакет `internal/server/`: chi-роутер, монтирование ServerInterface через `HandlerFromMux`, TLS, graceful shutdown (SIGINT/SIGTERM, 30s timeout). Health endpoints в `internal/api/handlers/health.go`: /health/live (200), /health/ready (заглушка), /metrics (promhttp.Handler). Middleware логирования запросов через slog. Обновление main.go.
  - **Creates**:
    - `src/storage-element/internal/server/server.go`
    - `src/storage-element/internal/api/handlers/health.go`
    - `src/storage-element/internal/api/middleware/logging.go`
  - **Links**:
    - [chi router](https://github.com/go-chi/chi)
    - [Prometheus Go client](https://github.com/prometheus/client_golang)

- [x] **1.4 Dockerfile, Makefile, docker-compose**
  - **Dependencies**: 1.3
  - **Description**: Multi-stage Dockerfile (golang:1.23-alpine -> alpine:3.19). Makefile targets: build, test, generate, lint, docker-build, docker-run, dev-certs (self-signed TLS). docker-compose.yaml с маппингом портов, volumes для data/wal/certs, env. Проверка: `docker-compose up` + `curl -k https://localhost:8010/health/live` = 200.
  - **Creates**:
    - `src/storage-element/Dockerfile`
    - `src/storage-element/Makefile` (обновление)
    - `src/storage-element/docker-compose.yaml`
    - `src/storage-element/.dockerignore`
  - **Links**:
    - Harbor: `harbor.kryukov.lan/library/storage-element`

### Критерии завершения Phase 1

- [x] `go build ./...` компилируется без ошибок
- [x] `go test ./...` проходит (unit-тесты config)
- [x] `make generate` генерирует код из OpenAPI
- [x] Docker-образ собирается (`make docker-build TAG=v1.0.0-1`)
- [x] `curl -k https://localhost:8010/health/live` = `{"status":"ok",...}`
- [x] Все endpoints кроме health возвращают 501 Not Implemented
- [x] Логирование в JSON/text работает

---

## Phase 2: Ядро хранилища

**Dependencies**: Phase 1
**Status**: Done

### Описание

Core-компоненты без HTTP-слоя: WAL, attr.json, файловое хранилище, in-memory индекс, конечный автомат режимов. Все покрыты unit-тестами.

### Подпункты

- [x] **2.1 WAL-движок (файловый)**
  - **Dependencies**: None
  - **Description**: `internal/storage/wal/`: WALEntry (transaction_id, operation_type [file_create/file_update/file_delete], status [pending/committed/rolled_back], file_id, started_at, completed_at). Движок: StartTransaction (создаёт .wal.json с pending), Commit, Rollback, RecoverPending (при старте — сканирование pending-записей). Атомарная запись: temp + fsync + os.Rename. Unit-тесты: commit, rollback, recovery, concurrent access.
  - **Creates**:
    - `src/storage-element/internal/storage/wal/entry.go`
    - `src/storage-element/internal/storage/wal/wal.go`
    - `src/storage-element/internal/storage/wal/wal_test.go`
  - **Links**:
    - `old_artstore/storage-element/app/services/wal_service.py`

- [x] **2.2 attr.json и файловое хранилище**
  - **Dependencies**: None (параллельно с 2.1)
  - **Description**: Доменная модель FileMetadata (`internal/domain/model/metadata.go`). Пакет `internal/storage/attr/`: WriteAttrFile (JSON -> temp -> fsync -> rename), ReadAttrFile, DeleteAttrFile. Пакет `internal/storage/filestore/`: SaveFile (streaming + SHA-256 на лету), ReadFile, DeleteFile, DiskUsage (syscall.Statfs). Имя файла: `{name}_{user}_{ts}_{uuid}.{ext}`. Unit-тесты.
  - **Creates**:
    - `src/storage-element/internal/domain/model/metadata.go`
    - `src/storage-element/internal/storage/attr/attr.go`
    - `src/storage-element/internal/storage/attr/attr_test.go`
    - `src/storage-element/internal/storage/filestore/filestore.go`
    - `src/storage-element/internal/storage/filestore/filestore_test.go`
  - **Links**:
    - `old_artstore/storage-element/app/utils/attr_utils.py`

- [x] **2.3 Конечный автомат режимов работы**
  - **Dependencies**: None (параллельно с 2.1, 2.2)
  - **Description**: `internal/domain/mode/`: StorageMode enum (edit, rw, ro, ar). StateMachine (sync.RWMutex): CurrentMode, CanTransitionTo, TransitionTo(target, confirm, subject), AllowedOperations, CanPerform. Два цикла: edit (изолированный) и rw->ro->ar (откат ro->rw с confirm). Unit-тесты всех допустимых и запрещённых переходов.
  - **Creates**:
    - `src/storage-element/internal/domain/mode/state_machine.go`
    - `src/storage-element/internal/domain/mode/state_machine_test.go`
  - **Links**:
    - `docs/briefs/storage-element.md` (раздел "Режимы работы")
    - `old_artstore/storage-element/app/core/storage_mode.py`

- [x] **2.4 In-memory индекс метаданных**
  - **Dependencies**: 2.2
  - **Description**: `internal/storage/index/`: Index (sync.RWMutex + map[string]*FileMetadata). BuildFromDir (сканирование *.attr.json), Add, Update, Remove, Get, List(limit, offset, statusFilter) -> ([]*FileMetadata, total), Count, CountByStatus, RebuildFromDir. Thread-safe. Unit-тесты: построение, CRUD, пагинация, фильтрация, race detector.
  - **Creates**:
    - `src/storage-element/internal/storage/index/index.go`
    - `src/storage-element/internal/storage/index/index_test.go`
  - **Links**: N/A

### Критерии завершения Phase 2

- [x] `go test ./internal/storage/... ./internal/domain/...` — все тесты проходят (68 тестов)
- [x] `go test -race ./...` — нет data races
- [x] WAL: create, commit, rollback, recovery работают (14 тестов)
- [x] attr.json: атомарная запись/чтение (15 тестов)
- [x] In-memory индекс: построение, CRUD, пагинация, фильтрация (21 тест)
- [x] Mode state machine: все переходы корректны (17 тестов)

---

## Phase 3: API handlers, JWT middleware, Prometheus метрики

**Dependencies**: Phase 2
**Status**: Done

### Описание

Реализация всех 12 endpoints, JWT-аутентификации (RS256 + JWKS), единого формата ошибок, Prometheus метрик. По завершении SE полностью функционален.

### Подпункты

- [x] **3.1 Ошибки, JWT middleware, Prometheus метрики**
  - **Dependencies**: None
  - **Description**: (a) `internal/api/errors/`: конструкторы для всех кодов ошибок, WriteError(w, statusCode, code, message). (b) JWT middleware (`internal/api/middleware/auth.go`): JWKS через keyfunc, Bearer token, RS256, claims (sub, scopes), RequireScope middleware. Публичные endpoints без auth. (c) Prometheus (`internal/api/middleware/metrics.go`): se_http_requests_total, se_http_request_duration_seconds, se_files_total, se_storage_bytes, se_operations_total. Unit-тесты.
  - **Creates**:
    - `src/storage-element/internal/api/errors/errors.go`
    - `src/storage-element/internal/api/middleware/auth.go`
    - `src/storage-element/internal/api/middleware/auth_test.go`
    - `src/storage-element/internal/api/middleware/metrics.go`
  - **Links**:
    - [keyfunc JWKS](https://github.com/MicahParks/keyfunc)

- [x] **3.2 Upload handler (POST /api/v1/files/upload)**
  - **Dependencies**: 3.1
  - **Description**: Handler + сервисный слой `internal/service/upload.go`. Поток: проверка mode -> scope -> multipart -> размер -> место на диске -> WAL start -> SaveFile (SHA-256) -> WriteAttrFile -> index.Add -> WAL commit. При ошибке: cleanup + rollback. Retention policy из mode (edit->temporary, rw->permanent). Ответ: 201 + FileMetadata. Unit + integration тест.
  - **Creates**:
    - `src/storage-element/internal/service/upload.go`
    - `src/storage-element/internal/service/upload_test.go`
    - `src/storage-element/internal/api/handlers/files.go`
  - **Links**:
    - `docs/api-contracts/storage-element-openapi.yaml` (POST /api/v1/files/upload)

- [x] **3.3 Download handler (GET /api/v1/files/{file_id}/download)**
  - **Dependencies**: 3.1
  - **Description**: `internal/service/download.go`. Проверка mode (edit/rw/ro) -> scope -> index.Get -> status==active -> открытие файла -> заголовки (Content-Type, Content-Disposition, ETag, Accept-Ranges). http.ServeContent для Range requests (206) и ETag (If-None-Match). Ошибки: 404, 409, 416. Unit-тесты.
  - **Creates**:
    - `src/storage-element/internal/service/download.go`
    - `src/storage-element/internal/service/download_test.go`
  - **Links**:
    - `docs/api-contracts/storage-element-openapi.yaml` (GET /api/v1/files/{file_id}/download)

- [x] **3.4 List, metadata, update, delete handlers**
  - **Dependencies**: 3.1
  - **Description**: (a) GET /api/v1/files — пагинация из индекса, FileListResponse. (b) GET /api/v1/files/{file_id} — из индекса, 404. (c) PATCH /api/v1/files/{file_id} — scope files:write, mode edit/rw, обновление description/tags в attr + index. (d) DELETE /api/v1/files/{file_id} — scope files:write, mode edit only, soft delete (status=deleted). Unit-тесты.
  - **Creates**:
    - `src/storage-element/internal/api/handlers/files.go` (дополнение)
    - `src/storage-element/internal/api/handlers/files_test.go`
  - **Links**:
    - `docs/api-contracts/storage-element-openapi.yaml`

- [x] **3.5 System info, mode transition, reconcile + полная сборка**
  - **Dependencies**: 3.2, 3.3, 3.4
  - **Description**: (a) GET /api/v1/info — без auth, StorageInfo (storage_id, mode, capacity, version, replica_mode, role). (b) POST /api/v1/mode/transition — scope storage:write, state machine, логирование. (c) POST /api/v1/maintenance/reconcile — заглушка (полная реализация в Phase 4). (d) Обновление health.go: readiness проверяет FS, WAL, индекс. (e) Обновление server.go: все handlers вместо stub, middleware chain. (f) Обновление main.go: инициализация всех компонентов, WAL recovery при старте, index build. Тестирование в Docker: upload -> list -> download -> update -> delete.
  - **Creates**:
    - `src/storage-element/internal/api/handlers/system.go`
    - `src/storage-element/internal/api/handlers/mode.go`
    - `src/storage-element/internal/api/handlers/maintenance.go`
    - Обновление: `health.go`, `server.go`, `main.go`
  - **Links**:
    - `docs/api-contracts/storage-element-openapi.yaml`

### Критерии завершения Phase 3

- [x] Все 12 endpoints реализованы и соответствуют OpenAPI
- [x] JWT RS256 + JWKS валидация работает
- [x] Ошибки в формате `{"error": {"code": "...", "message": "..."}}`
- [x] Upload через WAL, download с Range/ETag, пагинация, фильтрация
- [x] Mode transition runtime, все переходы корректны
- [x] Prometheus метрики на /metrics
- [x] `go test -race ./...` — 77 тестов, нет ошибок
- [ ] Ручное тестирование через curl в Docker

---

## Phase 4: Фоновые процессы

**Dependencies**: Phase 3
**Status**: In Progress

### Описание

GC (очистка expired/deleted файлов), Reconciliation (сверка attr.json с FS), topologymetrics (мониторинг JWKS). Интеграционное тестирование.

### Подпункты

- [x] **4.1 GC — фоновая очистка файлов**
  - **Dependencies**: None
  - **Description**: `internal/service/gc.go`. GCService: горутина с ticker (SE_GC_INTERVAL). RunOnce: сканирование индекса -> пометка expired (expires_at < now) -> физическое удаление deleted файлов. Prometheus: se_gc_runs_total, se_gc_files_deleted_total, se_gc_duration_seconds. Unit-тесты.
  - **Creates**:
    - `src/storage-element/internal/service/gc.go`
    - `src/storage-element/internal/service/gc_test.go`
  - **Links**:
    - `docs/briefs/storage-element.md` (GC)

- [x] **4.2 Reconciliation — фоновая сверка**
  - **Dependencies**: None (параллельно с 4.1)
  - **Description**: `internal/service/reconcile.go`. ReconcileService: горутина с ticker (SE_RECONCILE_INTERVAL), sync.Mutex для защиты от параллельного запуска. RunOnce: сканирование FS vs attr.json, выявление orphaned/missing/mismatch. Пересборка индекса. Обновление maintenance handler. Prometheus: se_reconcile_runs_total, se_reconcile_issues_total{type}. Unit-тесты.
  - **Creates**:
    - `src/storage-element/internal/service/reconcile.go`
    - `src/storage-element/internal/service/reconcile_test.go`
    - Обновление: `maintenance.go`
  - **Links**:
    - `docs/api-contracts/storage-element-openapi.yaml` (ReconcileResponse)

- [x] **4.3 topologymetrics — мониторинг зависимостей**
  - **Dependencies**: None (параллельно с 4.1, 4.2)
  - **Description**: `internal/service/dephealth.go`. Интеграция с `github.com/BigKAA/topologymetrics/sdk-go/dephealth`. Зависимость: Admin Module JWKS (HTTP GET, critical). Метрики на /metrics: app_dependency_health, app_dependency_latency_seconds. Обновление main.go. Unit-тесты с mock HTTP server.
  - **Creates**:
    - `src/storage-element/internal/service/dephealth.go`
    - `src/storage-element/internal/service/dephealth_test.go`
    - Обновление: `main.go`
  - **Links**:
    - [topologymetrics SDK](https://github.com/BigKAA/topologymetrics)

- [x] **4.4 Интеграционное тестирование** *(Объединено с Phase 5.3 → Phase 6.1-6.5)*
  - **Dependencies**: 4.1, 4.2, 4.3
  - **Description**: Интеграционное тестирование standalone и replicated mode реализовано в Kubernetes (Phase 6.1-6.5) вместо Docker-compose. Покрывает все сценарии: полный цикл файловых операций, GC, reconciliation, mode transitions, replicated mode, JWT auth. Отдельный docker-compose integration test не требуется.
  - **Creates**: Нет (реализовано в Phase 6)
  - **Links**: Phase 6.1-6.5

### Критерии завершения Phase 4

- [x] GC удаляет expired и deleted файлы
- [x] Reconciliation обнаруживает orphaned/missing/mismatch
- [x] topologymetrics метрики на /metrics
- [x] Интеграционные тесты реализованы в K8s (Phase 6.1-6.5)
- [x] `go test -race ./...` — 96 тестов, нет ошибок

---

## Phase 5: Replicated mode

**Dependencies**: Phase 4
**Status**: In Progress (5.1-5.2 завершены)

### Описание

Leader/Follower для HA. Leader обрабатывает запись, Follower — чтение + proxy записи к Leader. flock() на NFS.

### Подпункты

- [x] **5.1 Leader election и управление ролями** *(commit 42944c2)*
  - **Dependencies**: None
  - **Description**: `internal/replica/`. RoleProvider (standalone/leader/follower). LeaderElection: flock() на `{SE_DATA_DIR}/.leader.lock`, запись `.leader.info` (host, port). mode.json на NFS для режима. При standalone — leader election не запускается. GC/reconciliation только на leader. ForceMode в StateMachine. Unit-тесты (6 тестов).
  - **Creates**:
    - `src/storage-element/internal/replica/role.go`
    - `src/storage-element/internal/replica/election.go`
    - `src/storage-element/internal/replica/mode_file.go`
    - `src/storage-element/internal/replica/election_test.go`
    - Обновление: `main.go`, `state_machine.go`

- [x] **5.2 Follower proxy и index refresh** *(commit 42944c2)*
  - **Dependencies**: 5.1
  - **Description**: `internal/replica/proxy.go`. Chi middleware проксирования write-запросов к leader через httputil.ReverseProxy. FollowerRefreshService: пересборка индекса + sync mode.json. Обновление handlers: dynamic role в /api/v1/info, leader_connection в health, mode.json persist. Unit-тесты (5 тестов).
  - **Creates**:
    - `src/storage-element/internal/replica/proxy.go`
    - `src/storage-element/internal/replica/refresh.go`
    - `src/storage-element/internal/replica/proxy_test.go`
    - Обновление: handlers (system, mode, health), server.go, docker-compose.yaml

- [ ] **5.3 Интеграционное тестирование (standalone + replicated) в Kubernetes**
  - **Dependencies**: 5.2, Phase 6.1-6.5
  - **Description**: Полноценное интеграционное тестирование SE в Kubernetes вместо Docker-compose. Тестовая среда разворачивается в namespace `se-test` через Helm chart (Phase 6.2), включает 6 экземпляров SE в различных конфигурациях и JWKS Mock Server для аутентификации (Phase 6.1). Данные инициализируются через Helm post-install Job (Phase 6.3). Тестирование выполняется 30 bash-скриптами (Phase 6.4), оркестрируемыми через Makefile (Phase 6.5).

    **Тестовая среда (namespace `se-test`):**

    | Имя | Режим | Реплик | Replica mode | PVC data | PVC WAL |
    |-----|-------|--------|-------------|----------|---------|
    | se-edit-1 | edit | 2 | replicated | 200Mi RWX (shared) | 200Mi RWO x2 (per pod) |
    | se-edit-2 | edit | 2 | replicated | 200Mi RWX (shared) | 200Mi RWO x2 (per pod) |
    | se-rw-1 | rw | 1 | standalone | 200Mi RWO | 200Mi RWO |
    | se-rw-2 | rw | 1 | standalone | 200Mi RWO | 200Mi RWO |
    | se-ro | rw→ro | 1 | standalone | 200Mi RWO | 200Mi RWO |
    | se-ar | rw→ro→ar | 1 | standalone | 200Mi RWO | 200Mi RWO |

    **30 тестовых сценариев:**

    *Smoke (1-3):* health live/ready, /api/v1/info, /metrics
    *Files (4-11):* upload, download, range request, etag/if-none-match, list с пагинацией, metadata, update, delete
    *Modes (12-18):* upload в ro→409, download в ar→409, delete в rw→409, transition rw→ro, ro→rw без confirm→409, ro→rw с confirm→200, edit transition→409
    *Replica (19-22):* leader/follower election, proxy write через follower, kill leader→failover, новый pod→follower
    *Data (23-24):* se-ro содержит 3-4 файла (из init), se-ar содержит 200 файлов (из init)
    *GC/Reconcile (25-26):* GC удаляет expired файл, manual reconcile через API
    *Errors (27-30):* без JWT→401, невалидный JWT→401, нет scope→403, файл>limit→413

  - **Creates**: Нет собственных артефактов — спецификация для Phase 6.1-6.5
  - **Links**:
    - Phase 6.1: JWKS Mock Server
    - Phase 6.2: Helm chart для тестовой среды
    - Phase 6.3: Скрипты инициализации данных
    - Phase 6.4: Тестовые bash-скрипты (30 сценариев)
    - Phase 6.5: Makefile для оркестрации

### Критерии завершения Phase 5

- [x] Leader election через flock() работает
- [x] Follower проксирует write-запросы к leader
- [x] Follower обновляет индекс по таймеру
- [x] Failover: при падении leader follower становится leader
- [x] /api/v1/info корректно отображает replica_mode и role
- [x] GC/reconciliation только на leader
- [ ] 30 интеграционных тестов в K8s проходят (Phase 6.1-6.5)

---

## Phase 6: Helm charts, интеграционное тестирование и деплой в Kubernetes

**Dependencies**: Phase 5
**Status**: In Progress

### Описание

Полный цикл подготовки SE для Kubernetes: JWKS Mock Server для тестовой аутентификации, Helm chart тестовой среды (6 экземпляров SE в namespace `se-test`), инициализация данных, 30 интеграционных тестов (bash + curl + jq), Makefile оркестрация, production-like Helm chart для standalone и replicated mode, деплой в тестовый K8s кластер.

### Подпункты

- [x] **6.1 JWKS Mock Server (Go-сервис + Dockerfile)**
  - **Dependencies**: None
  - **Description**: Минималистичный Go-сервис (~200 строк), имитирующий JWKS endpoint Admin Module для тестовой среды. При старте генерирует RSA-2048 ключевую пару. Два endpoint-а: `GET /jwks` — возвращает JWKS (RFC 7517) с единственным ключом (kty=RSA, use=sig, alg=RS256, kid=test-key-1); `POST /token` — принимает JSON `{sub, scopes, ttl_seconds}`, генерирует подписанный JWT (RS256) с claims `{sub, scopes, exp, iss="jwks-mock"}`, возвращает `{token: "..."}`. TLS: сервис принимает MOCK_TLS_CERT и MOCK_TLS_KEY через env-переменные (сертификат от cert-manager). Порт: 8080. Зависимости: только `golang-jwt/jwt/v5`. Docker-образ: `harbor.kryukov.lan/library/jwks-mock:v1.0.0-1`. Go module отдельный: `github.com/arturkryukov/artsore/jwks-mock`.

    **Формат JWKS ответа:**

    ```json
    {
      "keys": [{
        "kty": "RSA",
        "kid": "test-key-1",
        "use": "sig",
        "alg": "RS256",
        "n": "<base64url-encoded modulus>",
        "e": "<base64url-encoded exponent>"
      }]
    }
    ```

    **Формат запроса POST /token:**

    ```json
    {
      "sub": "test-user",
      "scopes": ["files:read", "files:write", "storage:write"],
      "ttl_seconds": 3600
    }
    ```

    **Env-переменные JWKS Mock:**

    | Переменная | Описание | Default |
    |-----------|----------|---------|
    | MOCK_PORT | Порт HTTP-сервера | 8080 |
    | MOCK_TLS_CERT | Путь к TLS сертификату | (пусто — HTTP) |
    | MOCK_TLS_KEY | Путь к TLS приватному ключу | (пусто — HTTP) |
    | MOCK_KEY_SIZE | Размер RSA ключа | 2048 |

  - **Creates**:
    - `src/storage-element/tests/jwks-mock/main.go`
    - `src/storage-element/tests/jwks-mock/go.mod`
    - `src/storage-element/tests/jwks-mock/go.sum`
    - `src/storage-element/tests/jwks-mock/Dockerfile`
  - **Links**:
    - [RFC 7517 — JSON Web Key](https://www.rfc-editor.org/rfc/rfc7517)
    - `src/storage-element/internal/api/middleware/auth.go` — JWT Claims формат
    - [golang-jwt/jwt](https://github.com/golang-jwt/jwt)

- [x] **6.2 Helm chart для тестовой среды (se-test)**
  - **Dependencies**: 6.1
  - **Description**: Helm chart, разворачивающий полную тестовую среду в namespace `se-test`. Содержит: (1) JWKS Mock Server — Deployment + ClusterIP Service; (2) 6 экземпляров SE — 2 replicated (StatefulSet + headless Service + shared PVC RWX + WAL volumeClaimTemplates RWO) и 4 standalone (Deployment + ClusterIP Service + PVC RWO для data и WAL); (3) TLS Certificate через cert-manager — один Certificate с dnsNames для всех сервисов; (4) Init Job — Helm post-install hook (реализация в 6.3).

    **Replicated instances (se-edit-1, se-edit-2):**
    - StatefulSet с `replicas: 2`, headless Service для DNS
    - Shared PVC (200Mi, RWX, nfs-client) — /data
    - WAL через `volumeClaimTemplates` (200Mi, RWO per pod) — /wal
    - Env: SE_REPLICA_MODE=replicated, SE_INDEX_REFRESH_INTERVAL=10s

    **Standalone instances (se-rw-1, se-rw-2, se-ro, se-ar):**
    - Deployment с `replicas: 1`, strategy: Recreate
    - PVC data (200Mi, RWO) + PVC WAL (200Mi, RWO)
    - Env: SE_REPLICA_MODE=standalone

    **Общие настройки SE:**
    - SE_JWKS_URL: `https://jwks-mock.se-test.svc.cluster.local:8080/jwks`
    - SE_LOG_LEVEL: debug, SE_GC_INTERVAL: 30s, SE_MAX_FILE_SIZE: 10485760 (10MB)

    **cert-manager Certificate:**
    - ClusterIssuer: `dev-ca-issuer`, Secret: `se-test-tls`
    - dnsNames: jwks-mock, se-edit-1, *.se-edit-1, se-edit-2, *.se-edit-2, se-rw-1, se-rw-2, se-ro, se-ar (.se-test.svc.cluster.local)

    **values.yaml — управление через списки:**

    ```yaml
    replicatedInstances:
      - name: se-edit-1
        storageId: se-edit-1
        mode: edit
        replicas: 2
        dataSize: 200Mi
        walSize: 200Mi
      - name: se-edit-2
        ...
    standaloneInstances:
      - name: se-rw-1
        storageId: se-rw-1
        mode: rw
        ...
      - name: se-ro
        mode: rw         # Стартует как rw, init job переведёт в ro
        ...
      - name: se-ar
        mode: rw         # Стартует как rw, init job переведёт через ro в ar
        ...
    ```

    **Probes (HTTPS):**
    - livenessProbe: GET /health/live, port 8010, initialDelay 15s, period 30s
    - readinessProbe: GET /health/ready, port 8010, initialDelay 10s, period 15s

  - **Creates**:
    - `src/storage-element/tests/helm/se-test/Chart.yaml`
    - `src/storage-element/tests/helm/se-test/values.yaml`
    - `src/storage-element/tests/helm/se-test/templates/_helpers.tpl`
    - `src/storage-element/tests/helm/se-test/templates/namespace.yaml`
    - `src/storage-element/tests/helm/se-test/templates/certificate.yaml`
    - `src/storage-element/tests/helm/se-test/templates/jwks-mock.yaml`
    - `src/storage-element/tests/helm/se-test/templates/se-standalone.yaml`
    - `src/storage-element/tests/helm/se-test/templates/se-replicated.yaml`
    - `src/storage-element/tests/helm/se-test/templates/init-job.yaml`
  - **Links**:
    - [cert-manager Certificate](https://cert-manager.io/docs/usage/certificate/)
    - [Kubernetes StatefulSet](https://kubernetes.io/docs/concepts/workloads/controllers/statefulset/)
    - `old_artstore/k8s/charts/storage-element/` — паттерн Helm chart

- [x] **6.3 Скрипты инициализации данных (init job)**
  - **Dependencies**: 6.2
  - **Description**: Bash-скрипт для Helm post-install Job, подготавливающий тестовую среду. Контейнер: `alpine:3.19` + curl + jq. Скрипт и общая библиотека монтируются через ConfigMap.

    **Порядок действий init job:**
    1. Дождаться готовности JWKS Mock (GET /jwks, retry до 60 секунд)
    2. Получить JWT с scopes [files:read, files:write, storage:write]
    3. Дождаться готовности всех 6 SE (GET /health/ready, retry до 120 секунд)
    4. Загрузить 3-4 тестовых файла в se-ro (multipart/form-data)
    5. Загрузить 200 тестовых файлов в se-ar (цикл, ~1KB каждый)
    6. Перевести se-ro: rw → ro
    7. Перевести se-ar: rw → ro → ar
    8. Вывести summary

    **Общая библиотека функций lib.sh:**
    - `get_token(jwks_url, sub, scopes, ttl)` — получить JWT
    - `wait_ready(se_url, timeout)` — дождаться готовности SE
    - `upload_file(se_url, token, filename, content_type)` — загрузить файл
    - `transition_mode(se_url, token, target_mode, confirm)` — сменить режим
    - `assert_status(response, expected_code)` — проверить HTTP статус
    - `log_info/log_ok/log_fail(message)` — цветной вывод

    **Генерация тестовых файлов:** на лету через `dd if=/dev/urandom`.
    **Timeout:** `activeDeadlineSeconds: 300` (5 минут).

  - **Creates**:
    - `src/storage-element/tests/scripts/lib.sh`
    - `src/storage-element/tests/scripts/init-data.sh`
  - **Links**:
    - `docs/api-contracts/storage-element-openapi.yaml` — API контракт (upload, mode transition)

- [x] **6.4 Тестовые bash-скрипты (30 сценариев)**
  - **Dependencies**: 6.1, 6.3 (для lib.sh)
  - **Description**: 30 тестовых сценариев, реализованных как bash-скрипты с curl + jq. Каждый тест: формирует запрос → выполняет curl → проверяет HTTP статус и тело через jq → выводит PASS/FAIL. Тесты сгруппированы по категориям, каждый скрипт запускается независимо. Все используют lib.sh.

    **Переменные окружения для тестов:**
    - `JWKS_MOCK_URL`, `SE_EDIT_1_URL`, `SE_EDIT_2_URL`, `SE_RW_1_URL`, `SE_RW_2_URL`, `SE_RO_URL`, `SE_AR_URL`
    - `CURL_OPTS` — опции curl (default: `-k -s --connect-timeout 5`)

    **test-smoke.sh (тесты 1-3):**
    1. GET /health/live → 200, status == "ok"
    2. GET /health/ready → 200, checks.filesystem.status == "ok"
    3. GET /api/v1/info → 200, storage_id, mode, GET /metrics → contains "se_http_requests_total"

    **test-files.sh (тесты 4-11):**
    4. POST upload (1KB) → 201, file_id != ""
    5. GET download → 200, Content-Type == "application/octet-stream"
    6. GET download Range: bytes=0-99 → 206, Content-Range header
    7. GET download If-None-Match: {etag} → 304
    8. GET /files?limit=2&offset=0 → 200, items length <= 2
    9. GET /files/{id} → 200, metadata
    10. PATCH /files/{id} → 200, description обновлён
    11. DELETE /files/{id} → 204, повторный GET → deleted

    **test-modes.sh (тесты 12-18):**
    12. POST upload на se-ro → 409 MODE_NOT_ALLOWED
    13. GET download на se-ar → 409 MODE_NOT_ALLOWED
    14. DELETE на se-rw-1 → 409 MODE_NOT_ALLOWED
    15. POST transition se-rw-2 {target_mode: "ro"} → 200
    16. POST transition se-rw-2 (теперь ro) {target_mode: "rw"} без confirm → 409 CONFIRMATION_REQUIRED
    17. POST transition se-rw-2 {target_mode: "rw", confirm: true} → 200
    18. POST transition se-edit-1 {target_mode: "rw"} → 409 INVALID_TRANSITION

    **test-replica.sh (тесты 19-22):**
    19. GET /info на оба pod-а se-edit-1 → один leader, один follower
    20. POST upload на follower → 201 (proxy к leader)
    21. kubectl delete pod \<leader\> → follower становится leader (retry 30s)
    22. kubectl scale → новый pod → follower

    **test-data.sh (тесты 23-24):**
    23. GET /files на se-ro → total >= 3
    24. GET /files на se-ar → total == 200

    **test-gc-reconcile.sh (тесты 25-26):**
    25. Upload файл с коротким TTL в se-edit-2, подождать GC (30s), GET → expired/deleted
    26. POST /maintenance/reconcile на se-rw-1 → 200, reconcile stats

    **test-errors.sh (тесты 27-30):**
    27. GET /files без Authorization → 401
    28. GET /files с невалидным JWT → 401
    29. POST upload с JWT без scope files:write → 403
    30. POST upload файл > 10MB → 413

    **test-all.sh:** запускает все группы, подсчитывает PASS/FAIL, exit code != 0 при FAIL.

  - **Creates**:
    - `src/storage-element/tests/scripts/test-smoke.sh`
    - `src/storage-element/tests/scripts/test-files.sh`
    - `src/storage-element/tests/scripts/test-modes.sh`
    - `src/storage-element/tests/scripts/test-replica.sh`
    - `src/storage-element/tests/scripts/test-data.sh`
    - `src/storage-element/tests/scripts/test-gc-reconcile.sh`
    - `src/storage-element/tests/scripts/test-errors.sh`
    - `src/storage-element/tests/scripts/test-all.sh`
  - **Links**:
    - `docs/api-contracts/storage-element-openapi.yaml` — полный API контракт
    - `src/storage-element/internal/api/errors/errors.go` — коды ошибок

- [x] **6.5 Makefile для оркестрации**
  - **Dependencies**: 6.1, 6.2, 6.3, 6.4
  - **Description**: Makefile в `src/storage-element/tests/`, оркестрирующий полный цикл интеграционного тестирования. Переменные: DOCKER_REGISTRY (harbor.kryukov.lan/library), SE_TAG (v1.0.0-1), MOCK_TAG (v1.0.0-1), NAMESPACE (se-test), HELM_CHART (helm/se-test). Все targets идемпотентны.

    **Targets:**

    | Target | Описание |
    |--------|----------|
    | `help` | Показать справку |
    | `docker-build` | Собрать SE + JWKS Mock образы, push в Harbor |
    | `test-env-up` | helm upgrade --install se-test, --wait --timeout 5m |
    | `test-env-status` | kubectl get pods,svc,pvc -n se-test |
    | `test-env-logs` | kubectl logs -n se-test --all-containers |
    | `test-smoke` | scripts/test-smoke.sh |
    | `test-files` | scripts/test-files.sh |
    | `test-modes` | scripts/test-modes.sh |
    | `test-replica` | scripts/test-replica.sh |
    | `test-data` | scripts/test-data.sh |
    | `test-gc` | scripts/test-gc-reconcile.sh |
    | `test-errors` | scripts/test-errors.sh |
    | `test-all` | scripts/test-all.sh |
    | `test-env-down` | helm uninstall + kubectl delete ns |
    | `clean` | test-env-down + docker rmi |

    **Доступ к сервисам:** через `kubectl port-forward` (target `port-forward-start/stop`).

    **Port-forward mapping:**
    - JWKS Mock: localhost:18080 → jwks-mock:8080
    - se-edit-1: localhost:18010 → se-edit-1:8010
    - se-edit-2: localhost:18011 → se-edit-2:8010
    - se-rw-1: localhost:18012 → se-rw-1:8010
    - se-rw-2: localhost:18013 → se-rw-2:8010
    - se-ro: localhost:18014 → se-ro:8010
    - se-ar: localhost:18015 → se-ar:8010

  - **Creates**:
    - `src/storage-element/tests/Makefile`
  - **Links**:
    - `src/storage-element/Makefile` — паттерн Makefile

- [ ] **6.6 Основной Helm chart для SE (standalone + replicated)**
  - **Dependencies**: None (параллельно с 6.1-6.5)
  - **Description**: Production-like Helm chart для деплоя одного экземпляра SE в Kubernetes. Поддерживает два режима: standalone (Deployment) и replicated (StatefulSet). Расположение: `src/storage-element/charts/storage-element/`. Кондициональный рендеринг: `replicaMode: standalone` → Deployment, `replicaMode: replicated` → StatefulSet.

    **Шаблоны:**

    *_helpers.tpl:* `se.fullname` = `storage-element-{{ .Values.elementId }}`, `se.labels`, `se.selectorLabels`, `se.image`

    *deployment.yaml (standalone):*
    - Условие: `{{- if eq .Values.replicaMode "standalone" }}`
    - replicas: 1, strategy: Recreate
    - Env из values (все 19 SE env-переменных)
    - volumeMounts: /data (PVC), /wal (PVC), /certs (TLS Secret)
    - Probes: HTTPS GET /health/live, /health/ready

    *statefulset.yaml (replicated):*
    - Условие: `{{- if eq .Values.replicaMode "replicated" }}`
    - replicas: {{ .Values.replicas }}, serviceName: headless
    - volumeClaimTemplates: WAL (RWO per pod)
    - /data: shared PVC (RWX)

    *service.yaml:* ClusterIP, port {{ .Values.port }}
    *service-headless.yaml:* clusterIP: None (replicated)
    *pvc.yaml:* data + WAL (standalone, RWO)
    *pvc-shared.yaml:* data (replicated, RWX)
    *certificate.yaml:* cert-manager Certificate (dev-ca-issuer)
    *httproute.yaml:* Gateway API HTTPRoute (опционально)

    **values.yaml (standalone defaults):**

    ```yaml
    elementId: se-01
    port: 8010
    mode: rw
    replicaMode: standalone
    replicas: 1
    registry: harbor.kryukov.lan
    image: library/storage-element
    tag: latest
    jwksUrl: "https://admin-module.artsore.svc.cluster.local:8000/api/v1/auth/jwks"
    storageClass: nfs-client
    dataSize: 2Gi
    walSize: 1Gi
    tls:
      clusterIssuer: dev-ca-issuer
    httproute:
      enabled: false
      gatewayName: eg
      gatewayNamespace: envoy-gateway-system
      hostname: "se-01.kryukov.lan"
    ```

  - **Creates**:
    - `src/storage-element/charts/storage-element/Chart.yaml`
    - `src/storage-element/charts/storage-element/values.yaml`
    - `src/storage-element/charts/storage-element/values-replicated.yaml`
    - `src/storage-element/charts/storage-element/templates/_helpers.tpl`
    - `src/storage-element/charts/storage-element/templates/deployment.yaml`
    - `src/storage-element/charts/storage-element/templates/statefulset.yaml`
    - `src/storage-element/charts/storage-element/templates/service.yaml`
    - `src/storage-element/charts/storage-element/templates/service-headless.yaml`
    - `src/storage-element/charts/storage-element/templates/pvc.yaml`
    - `src/storage-element/charts/storage-element/templates/pvc-shared.yaml`
    - `src/storage-element/charts/storage-element/templates/certificate.yaml`
    - `src/storage-element/charts/storage-element/templates/httproute.yaml`
  - **Links**:
    - `old_artstore/k8s/charts/storage-element/` — паттерн Helm chart
    - `old_artstore/k8s/charts/admin-module/templates/httproute.yaml` — паттерн HTTPRoute
    - [Helm Chart Best Practices](https://helm.sh/docs/chart_best_practices/)

- [ ] **6.7 Деплой в K8s и верификация**
  - **Dependencies**: 6.5, 6.6
  - **Description**: Финальная верификация: (1) Сборка Docker-образов SE и JWKS Mock, push в Harbor. (2) Деплой тестовой среды `make test-env-up`. (3) Запуск полного набора интеграционных тестов `make test-all` — 30 тестов PASS. (4) Деплой production-like SE через основной Helm chart (standalone + replicated). (5) Верификация через curl. (6) README.md с инструкциями по деплою.

    **Шаги:**

    1. Сборка: `make docker-build` (SE + JWKS Mock → Harbor)
    2. Тестовая среда: `make test-env-up` → `make test-all` → 30/30 PASS
    3. Production standalone: `helm install se-rw-01 charts/storage-element --set elementId=se-rw-01 --set mode=rw`
    4. Production replicated: `helm install se-edit-01 charts/storage-element -f values-replicated.yaml --set elementId=se-edit-01`
    5. Верификация: health, info, upload, download, metrics
    6. README.md: инструкции по деплою, конфигурация, примеры

  - **Creates**:
    - Docker images в Harbor (storage-element, jwks-mock)
    - K8s resources
    - `src/storage-element/README.md`
  - **Links**:
    - Harbor: `harbor.kryukov.lan/library`
    - MetalLB: 192.168.218.180-190

### Критерии завершения Phase 6

- [x] JWKS Mock Server: /jwks возвращает валидный JWKS, /token выдаёт подписанный JWT
- [x] Docker-образы собраны и pushed в Harbor (storage-element, jwks-mock)
- [x] Тестовая среда (6 SE + JWKS Mock) разворачивается через `make test-env-up`
- [x] Init job: se-ro в режиме ro с 4 файлами, se-ar в режиме ar с 200 attr.json (только метаданные)
- [ ] 30 интеграционных тестов проходят (`make test-all`)
- [ ] Production Helm chart: `helm lint` проходит
- [ ] Standalone SE: в K8s через основной Helm chart, все endpoints работают
- [ ] Replicated SE: 2 реплики, leader/follower, failover через flock()
- [ ] TLS через cert-manager (dev-ca-issuer)
- [ ] /metrics доступен для Prometheus
- [ ] README.md с инструкциями по деплою

### Зависимости между подпунктами Phase 6

```text
6.1 (JWKS Mock) ──┬──> 6.2 (Test Helm) ──> 6.3 (Init Scripts) ──> 6.5 (Makefile) ──> 6.7 (Deploy)
                   │                                                     ^                    ^
                   └──> 6.4 (Test Scripts) ──────────────────────────────┘                    │
                                                                                               │
6.6 (Production Helm) ────────────────────────────────────────────────────────────────────────┘
     (параллельно с 6.1-6.5)
```

### Файловая структура Phase 6

```text
src/storage-element/
├── tests/
│   ├── Makefile                          # (6.5) Оркестрация
│   ├── jwks-mock/                        # (6.1) JWKS Mock Server
│   │   ├── main.go
│   │   ├── Dockerfile
│   │   ├── go.mod
│   │   └── go.sum
│   ├── helm/se-test/                     # (6.2) Тестовый Helm chart
│   │   ├── Chart.yaml
│   │   ├── values.yaml
│   │   └── templates/
│   │       ├── _helpers.tpl
│   │       ├── namespace.yaml
│   │       ├── certificate.yaml
│   │       ├── jwks-mock.yaml
│   │       ├── se-standalone.yaml
│   │       ├── se-replicated.yaml
│   │       └── init-job.yaml
│   └── scripts/                          # (6.3, 6.4) Скрипты
│       ├── lib.sh
│       ├── init-data.sh
│       ├── test-smoke.sh
│       ├── test-files.sh
│       ├── test-modes.sh
│       ├── test-replica.sh
│       ├── test-data.sh
│       ├── test-gc-reconcile.sh
│       ├── test-errors.sh
│       └── test-all.sh
├── charts/storage-element/               # (6.6) Production Helm chart
│   ├── Chart.yaml
│   ├── values.yaml
│   ├── values-replicated.yaml
│   └── templates/
│       ├── _helpers.tpl
│       ├── deployment.yaml
│       ├── statefulset.yaml
│       ├── service.yaml
│       ├── service-headless.yaml
│       ├── pvc.yaml
│       ├── pvc-shared.yaml
│       ├── certificate.yaml
│       └── httproute.yaml
└── README.md                             # (6.7) Инструкции по деплою
```

### Рекомендуемый порядок AI-сессий

| Сессия | Подпункты | Ветка | Описание |
|--------|-----------|-------|----------|
| A | 6.1 | `feature/se-jwks-mock` | JWKS Mock: main.go + Dockerfile + docker build |
| B | 6.6 | `feature/se-helm-chart` | Production Helm chart (standalone + replicated) |
| C | 6.2 | `feature/se-test-environment` | Тестовый Helm chart (se-test) |
| D | 6.3 + 6.4 | `feature/se-test-environment` | Скрипты init + 30 тестов |
| E | 6.5 + 6.7 | `feature/se-phase-6-deploy` | Makefile + деплой + верификация + README |

### Git workflow Phase 6

Commits: `feat(storage-element): <subject>`. Теги образов: `v1.0.0-N`.

---

## Ключевые файлы-источники

| Файл | Назначение |
|------|-----------|
| `docs/api-contracts/storage-element-openapi.yaml` | OpenAPI контракт (12 endpoints), источник для oapi-codegen |
| `docs/briefs/storage-element.md` | Бриф: режимы, replicated mode, topologymetrics, конфигурация |
| `old_artstore/storage-element/app/services/wal_service.py` | Референс WAL (новая реализация — файловая, без БД) |
| `old_artstore/storage-element/app/utils/attr_utils.py` | Референс attr.json (temp + fsync + rename) |
| `old_artstore/storage-element/app/core/storage_mode.py` | Референс state machine режимов |

## Примечания

- **Порядок инициализации при старте**: конфиг -> WAL recovery -> index build -> leader election (replicated) -> фоновые процессы (leader only) -> HTTP-сервер
- **Graceful shutdown**: остановка HTTP (30s) -> фоновые процессы -> leader lock -> выход
- **Retention policy**: edit mode -> temporary (TTL default 30 дней), rw mode -> permanent
- **WAL файловый**: {tx_id}.wal.json в SE_WAL_DIR, не в БД (отличие от старого проекта)
