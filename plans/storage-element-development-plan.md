# План разработки: Storage Element (Go)

## Контекст

Storage Element (SE) — первый модуль системы Artsore для реализации на Go. SE отвечает за физическое хранение файлов на локальной файловой системе с гарантией атомарности через WAL. Модуль автономный, может работать вне кластера K8s (bare metal, Docker, edge). Все проектные решения, API-контракт (12 endpoints) и бриф уже зафиксированы. Цель — реализовать полнофункциональный SE, собрать Docker-образ и развернуть в тестовом K8s кластере.

## Метаданные

- **Версия плана**: 1.0.0
- **Дата создания**: 2026-02-21
- **Последнее обновление**: 2026-02-21
- **Статус**: Pending

---

## История версий

- **v1.0.0** (2026-02-21): Начальная версия плана

---

## Текущий статус

- **Активная фаза**: Phase 1
- **Активный подпункт**: 1.1
- **Последнее обновление**: 2026-02-21
- **Примечание**: План создан, ожидает начала реализации

---

## Оглавление

- [ ] [Phase 1: Инфраструктура проекта и скелет сервера](#phase-1-инфраструктура-проекта-и-скелет-сервера)
- [ ] [Phase 2: Ядро хранилища (WAL, attr.json, файловое хранилище, индекс, режимы)](#phase-2-ядро-хранилища)
- [ ] [Phase 3: API handlers, JWT middleware, Prometheus метрики](#phase-3-api-handlers-jwt-middleware-prometheus-метрики)
- [ ] [Phase 4: Фоновые процессы (GC, Reconciliation, topologymetrics)](#phase-4-фоновые-процессы)
- [ ] [Phase 5: Replicated mode (Leader/Follower)](#phase-5-replicated-mode)
- [ ] [Phase 6: Helm chart и деплой в Kubernetes](#phase-6-helm-chart-и-деплой-в-kubernetes)

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
**Status**: Pending

### Описание

Создание фундамента: Go-модуль, структура директорий, конфигурация, кодогенерация из OpenAPI, скелет HTTP-сервера с TLS и health endpoints, Docker-образ. По завершении сервер запускается в Docker и отвечает на health probes.

### Подпункты

- [ ] **1.1 Структура проекта и конфигурация**
  - **Dependencies**: None
  - **Description**: Go-модуль (`src/storage-element/go.mod`), структура `cmd/` + `internal/`. Пакет `internal/config/`: парсинг 17 env-переменных (SE_PORT, SE_STORAGE_ID, SE_DATA_DIR, SE_WAL_DIR, SE_MODE, SE_MAX_FILE_SIZE, SE_GC_INTERVAL, SE_RECONCILE_INTERVAL, SE_JWKS_URL, SE_TLS_CERT, SE_TLS_KEY, SE_LOG_LEVEL, SE_LOG_FORMAT, SE_REPLICA_MODE, SE_INDEX_REFRESH_INTERVAL, SE_DEPHEALTH_CHECK_INTERVAL) через stdlib. Валидация обязательных полей, Go duration парсинг, значения по умолчанию. Настройка slog (JSON/text). Минимальный `main.go`. Unit-тесты config.
  - **Creates**:
    - `src/storage-element/go.mod`
    - `src/storage-element/cmd/storage-element/main.go`
    - `src/storage-element/internal/config/config.go`
    - `src/storage-element/internal/config/config_test.go`
  - **Links**:
    - [Go Project Layout](https://github.com/golang-standards/project-layout)

- [ ] **1.2 Кодогенерация из OpenAPI (oapi-codegen)**
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

- [ ] **1.3 HTTP-сервер, TLS, health endpoints**
  - **Dependencies**: 1.2
  - **Description**: Пакет `internal/server/`: chi-роутер, монтирование ServerInterface через `HandlerFromMux`, TLS, graceful shutdown (SIGINT/SIGTERM, 30s timeout). Health endpoints в `internal/api/handlers/health.go`: /health/live (200), /health/ready (заглушка), /metrics (promhttp.Handler). Middleware логирования запросов через slog. Обновление main.go.
  - **Creates**:
    - `src/storage-element/internal/server/server.go`
    - `src/storage-element/internal/api/handlers/health.go`
    - `src/storage-element/internal/api/middleware/logging.go`
  - **Links**:
    - [chi router](https://github.com/go-chi/chi)
    - [Prometheus Go client](https://github.com/prometheus/client_golang)

- [ ] **1.4 Dockerfile, Makefile, docker-compose**
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

- [ ] `go build ./...` компилируется без ошибок
- [ ] `go test ./...` проходит (unit-тесты config)
- [ ] `make generate` генерирует код из OpenAPI
- [ ] Docker-образ собирается (`make docker-build TAG=v1.0.0-1`)
- [ ] `curl -k https://localhost:8010/health/live` = `{"status":"ok",...}`
- [ ] Все endpoints кроме health возвращают 501 Not Implemented
- [ ] Логирование в JSON/text работает

---

## Phase 2: Ядро хранилища

**Dependencies**: Phase 1
**Status**: Pending

### Описание

Core-компоненты без HTTP-слоя: WAL, attr.json, файловое хранилище, in-memory индекс, конечный автомат режимов. Все покрыты unit-тестами.

### Подпункты

- [ ] **2.1 WAL-движок (файловый)**
  - **Dependencies**: None
  - **Description**: `internal/storage/wal/`: WALEntry (transaction_id, operation_type [file_create/file_update/file_delete], status [pending/committed/rolled_back], file_id, started_at, completed_at). Движок: StartTransaction (создаёт .wal.json с pending), Commit, Rollback, RecoverPending (при старте — сканирование pending-записей). Атомарная запись: temp + fsync + os.Rename. Unit-тесты: commit, rollback, recovery, concurrent access.
  - **Creates**:
    - `src/storage-element/internal/storage/wal/entry.go`
    - `src/storage-element/internal/storage/wal/wal.go`
    - `src/storage-element/internal/storage/wal/wal_test.go`
  - **Links**:
    - `old_artstore/storage-element/app/services/wal_service.py`

- [ ] **2.2 attr.json и файловое хранилище**
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

- [ ] **2.3 Конечный автомат режимов работы**
  - **Dependencies**: None (параллельно с 2.1, 2.2)
  - **Description**: `internal/domain/mode/`: StorageMode enum (edit, rw, ro, ar). StateMachine (sync.RWMutex): CurrentMode, CanTransitionTo, TransitionTo(target, confirm, subject), AllowedOperations, CanPerform. Два цикла: edit (изолированный) и rw->ro->ar (откат ro->rw с confirm). Unit-тесты всех допустимых и запрещённых переходов.
  - **Creates**:
    - `src/storage-element/internal/domain/mode/state_machine.go`
    - `src/storage-element/internal/domain/mode/state_machine_test.go`
  - **Links**:
    - `docs/briefs/storage-element.md` (раздел "Режимы работы")
    - `old_artstore/storage-element/app/core/storage_mode.py`

- [ ] **2.4 In-memory индекс метаданных**
  - **Dependencies**: 2.2
  - **Description**: `internal/storage/index/`: Index (sync.RWMutex + map[string]*FileMetadata). BuildFromDir (сканирование *.attr.json), Add, Update, Remove, Get, List(limit, offset, statusFilter) -> ([]*FileMetadata, total), Count, CountByStatus, RebuildFromDir. Thread-safe. Unit-тесты: построение, CRUD, пагинация, фильтрация, race detector.
  - **Creates**:
    - `src/storage-element/internal/storage/index/index.go`
    - `src/storage-element/internal/storage/index/index_test.go`
  - **Links**: N/A

### Критерии завершения Phase 2

- [ ] `go test ./internal/storage/... ./internal/domain/...` — все тесты проходят
- [ ] `go test -race ./...` — нет data races
- [ ] WAL: create, commit, rollback, recovery работают
- [ ] attr.json: атомарная запись/чтение
- [ ] In-memory индекс: построение, CRUD, пагинация, фильтрация
- [ ] Mode state machine: все переходы корректны

---

## Phase 3: API handlers, JWT middleware, Prometheus метрики

**Dependencies**: Phase 2
**Status**: Pending

### Описание

Реализация всех 12 endpoints, JWT-аутентификации (RS256 + JWKS), единого формата ошибок, Prometheus метрик. По завершении SE полностью функционален.

### Подпункты

- [ ] **3.1 Ошибки, JWT middleware, Prometheus метрики**
  - **Dependencies**: None
  - **Description**: (a) `internal/api/errors/`: конструкторы для всех кодов ошибок, WriteError(w, statusCode, code, message). (b) JWT middleware (`internal/api/middleware/auth.go`): JWKS через keyfunc, Bearer token, RS256, claims (sub, scopes), RequireScope middleware. Публичные endpoints без auth. (c) Prometheus (`internal/api/middleware/metrics.go`): se_http_requests_total, se_http_request_duration_seconds, se_files_total, se_storage_bytes, se_operations_total. Unit-тесты.
  - **Creates**:
    - `src/storage-element/internal/api/errors/errors.go`
    - `src/storage-element/internal/api/middleware/auth.go`
    - `src/storage-element/internal/api/middleware/auth_test.go`
    - `src/storage-element/internal/api/middleware/metrics.go`
  - **Links**:
    - [keyfunc JWKS](https://github.com/MicahParks/keyfunc)

- [ ] **3.2 Upload handler (POST /api/v1/files/upload)**
  - **Dependencies**: 3.1
  - **Description**: Handler + сервисный слой `internal/service/upload.go`. Поток: проверка mode -> scope -> multipart -> размер -> место на диске -> WAL start -> SaveFile (SHA-256) -> WriteAttrFile -> index.Add -> WAL commit. При ошибке: cleanup + rollback. Retention policy из mode (edit->temporary, rw->permanent). Ответ: 201 + FileMetadata. Unit + integration тест.
  - **Creates**:
    - `src/storage-element/internal/service/upload.go`
    - `src/storage-element/internal/service/upload_test.go`
    - `src/storage-element/internal/api/handlers/files.go`
  - **Links**:
    - `docs/api-contracts/storage-element-openapi.yaml` (POST /api/v1/files/upload)

- [ ] **3.3 Download handler (GET /api/v1/files/{file_id}/download)**
  - **Dependencies**: 3.1
  - **Description**: `internal/service/download.go`. Проверка mode (edit/rw/ro) -> scope -> index.Get -> status==active -> открытие файла -> заголовки (Content-Type, Content-Disposition, ETag, Accept-Ranges). http.ServeContent для Range requests (206) и ETag (If-None-Match). Ошибки: 404, 409, 416. Unit-тесты.
  - **Creates**:
    - `src/storage-element/internal/service/download.go`
    - `src/storage-element/internal/service/download_test.go`
  - **Links**:
    - `docs/api-contracts/storage-element-openapi.yaml` (GET /api/v1/files/{file_id}/download)

- [ ] **3.4 List, metadata, update, delete handlers**
  - **Dependencies**: 3.1
  - **Description**: (a) GET /api/v1/files — пагинация из индекса, FileListResponse. (b) GET /api/v1/files/{file_id} — из индекса, 404. (c) PATCH /api/v1/files/{file_id} — scope files:write, mode edit/rw, обновление description/tags в attr + index. (d) DELETE /api/v1/files/{file_id} — scope files:write, mode edit only, soft delete (status=deleted). Unit-тесты.
  - **Creates**:
    - `src/storage-element/internal/api/handlers/files.go` (дополнение)
    - `src/storage-element/internal/api/handlers/files_test.go`
  - **Links**:
    - `docs/api-contracts/storage-element-openapi.yaml`

- [ ] **3.5 System info, mode transition, reconcile + полная сборка**
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

- [ ] Все 12 endpoints реализованы и соответствуют OpenAPI
- [ ] JWT RS256 + JWKS валидация работает
- [ ] Ошибки в формате `{"error": {"code": "...", "message": "..."}}`
- [ ] Upload через WAL, download с Range/ETag, пагинация, фильтрация
- [ ] Mode transition runtime, все переходы корректны
- [ ] Prometheus метрики на /metrics
- [ ] `go test -race ./...` — нет ошибок
- [ ] Ручное тестирование через curl в Docker

---

## Phase 4: Фоновые процессы

**Dependencies**: Phase 3
**Status**: Pending

### Описание

GC (очистка expired/deleted файлов), Reconciliation (сверка attr.json с FS), topologymetrics (мониторинг JWKS). Интеграционное тестирование.

### Подпункты

- [ ] **4.1 GC — фоновая очистка файлов**
  - **Dependencies**: None
  - **Description**: `internal/service/gc.go`. GCService: горутина с ticker (SE_GC_INTERVAL). RunOnce: сканирование индекса -> пометка expired (expires_at < now) -> физическое удаление deleted файлов. Prometheus: se_gc_runs_total, se_gc_files_deleted_total, se_gc_duration_seconds. Unit-тесты.
  - **Creates**:
    - `src/storage-element/internal/service/gc.go`
    - `src/storage-element/internal/service/gc_test.go`
  - **Links**:
    - `docs/briefs/storage-element.md` (GC)

- [ ] **4.2 Reconciliation — фоновая сверка**
  - **Dependencies**: None (параллельно с 4.1)
  - **Description**: `internal/service/reconcile.go`. ReconcileService: горутина с ticker (SE_RECONCILE_INTERVAL), sync.Mutex для защиты от параллельного запуска. RunOnce: сканирование FS vs attr.json, выявление orphaned/missing/mismatch. Пересборка индекса. Обновление maintenance handler. Prometheus: se_reconcile_runs_total, se_reconcile_issues_total{type}. Unit-тесты.
  - **Creates**:
    - `src/storage-element/internal/service/reconcile.go`
    - `src/storage-element/internal/service/reconcile_test.go`
    - Обновление: `maintenance.go`
  - **Links**:
    - `docs/api-contracts/storage-element-openapi.yaml` (ReconcileResponse)

- [ ] **4.3 topologymetrics — мониторинг зависимостей**
  - **Dependencies**: None (параллельно с 4.1, 4.2)
  - **Description**: `internal/service/dephealth.go`. Интеграция с `github.com/BigKAA/topologymetrics/sdk-go/dephealth`. Зависимость: Admin Module JWKS (HTTP GET, critical). Метрики на /metrics: app_dependency_health, app_dependency_latency_seconds. Обновление main.go. Unit-тесты с mock HTTP server.
  - **Creates**:
    - `src/storage-element/internal/service/dephealth.go`
    - `src/storage-element/internal/service/dephealth_test.go`
    - Обновление: `main.go`
  - **Links**:
    - [topologymetrics SDK](https://github.com/BigKAA/topologymetrics)

- [ ] **4.4 Интеграционное тестирование в Docker**
  - **Dependencies**: 4.1, 4.2, 4.3
  - **Description**: Integration тесты в Docker. Сценарии: (1) полный цикл upload->list->download->update->delete, (2) GC удаление expired, (3) reconciliation обнаружение orphaned, (4) mode transitions (rw->ro блокирует upload, ro->rw с confirm), (5) Range download, (6) проверка /metrics. Dev JWT (self-signed RS256). Makefile target: `integration-test`.
  - **Creates**:
    - `src/storage-element/tests/integration_test.go`
    - `src/storage-element/tests/testdata/`
    - Обновление: `docker-compose.yaml`, `Makefile`
  - **Links**: N/A

### Критерии завершения Phase 4

- [ ] GC удаляет expired и deleted файлы
- [ ] Reconciliation обнаруживает orphaned/missing/mismatch
- [ ] topologymetrics метрики на /metrics
- [ ] Интеграционные тесты проходят в Docker
- [ ] `go test -race ./...` — все тесты проходят

---

## Phase 5: Replicated mode

**Dependencies**: Phase 4
**Status**: Pending

### Описание

Leader/Follower для HA. Leader обрабатывает запись, Follower — чтение + proxy записи к Leader. flock() на NFS.

### Подпункты

- [ ] **5.1 Leader election и управление ролями**
  - **Dependencies**: None
  - **Description**: `internal/replica/`. RoleManager (standalone/leader/follower). LeaderElection: flock() на `{SE_DATA_DIR}/.leader.lock`, запись `.leader.info` (host, port). mode.json на NFS для режима. При standalone — leader election не запускается. GC/reconciliation только на leader. Unit-тесты.
  - **Creates**:
    - `src/storage-element/internal/replica/role.go`
    - `src/storage-element/internal/replica/leader.go`
    - `src/storage-element/internal/replica/leader_test.go`
    - Обновление: `main.go`
  - **Links**:
    - `docs/briefs/storage-element.md` (раздел "Масштабирование")

- [ ] **5.2 Follower proxy и index refresh**
  - **Dependencies**: 5.1
  - **Description**: `internal/replica/follower.go`. Proxy write-запросов к leader через httputil.ReverseProxy (адрес из .leader.info). Index refresh по таймеру (SE_INDEX_REFRESH_INTERVAL). Обновление handlers: проверка роли перед write. /api/v1/info возвращает replica_mode и role. Unit-тесты.
  - **Creates**:
    - `src/storage-element/internal/replica/follower.go`
    - `src/storage-element/internal/replica/follower_test.go`
    - Обновление: handlers, docker-compose.yaml
  - **Links**:
    - `docs/briefs/storage-element.md` (Follower proxy)

- [ ] **5.3 Интеграционное тестирование replicated mode**
  - **Dependencies**: 5.2
  - **Description**: docker-compose: 2 SE с общим volume, SE_REPLICA_MODE=replicated. Сценарии: leader/follower election, upload на follower (proxy), failover при остановке leader, download на follower (прямой). Makefile target: `integration-test-replicated`.
  - **Creates**:
    - `src/storage-element/tests/integration_replicated_test.go`
    - Обновление: `docker-compose.yaml`, `Makefile`
  - **Links**: N/A

### Критерии завершения Phase 5

- [ ] Leader election через flock() работает
- [ ] Follower проксирует write-запросы к leader
- [ ] Follower обновляет индекс по таймеру
- [ ] Failover: при падении leader follower становится leader
- [ ] /api/v1/info корректно отображает replica_mode и role
- [ ] GC/reconciliation только на leader
- [ ] Интеграционные тесты replicated mode проходят

---

## Phase 6: Helm chart и деплой в Kubernetes

**Dependencies**: Phase 5
**Status**: Pending

### Описание

Helm chart для standalone и replicated mode. Деплой в тестовый K8s кластер.

### Подпункты

- [ ] **6.1 Helm chart — standalone mode**
  - **Dependencies**: None
  - **Description**: `src/storage-element/charts/storage-element/`: Chart.yaml, values.yaml, templates (deployment, service, configmap, pvc, certificate via cert-manager, httproute via Gateway API). Probes: liveness /health/live, readiness /health/ready. `helm lint` + `helm template`.
  - **Creates**:
    - `src/storage-element/charts/storage-element/` (все файлы)
  - **Links**:
    - cert-manager ClusterIssuer: `dev-ca-issuer`
    - Gateway API: gatewayClassName `eg`

- [ ] **6.2 Helm chart — replicated mode**
  - **Dependencies**: 6.1
  - **Description**: values-replicated.yaml, StatefulSet (вместо Deployment), общий PVC (ReadWriteMany), headless service. Условный рендеринг: standalone=Deployment, replicated=StatefulSet.
  - **Creates**:
    - `src/storage-element/charts/storage-element/values-replicated.yaml`
    - `src/storage-element/charts/storage-element/templates/statefulset.yaml`
    - `src/storage-element/charts/storage-element/templates/service-headless.yaml`
  - **Links**: N/A

- [ ] **6.3 Деплой в тестовый K8s и тестирование**
  - **Dependencies**: 6.2
  - **Description**: Docker build + push в Harbor (`harbor.kryukov.lan/library/storage-element:v1.0.0-1`). helm install standalone. Тестирование через Gateway API. helm install replicated (2 реплики). Failover тест. README.md с инструкциями.
  - **Creates**:
    - Docker image в Harbor
    - K8s resources
    - `src/storage-element/README.md`
  - **Links**:
    - Harbor: `harbor.kryukov.lan/library/storage-element`
    - MetalLB: 192.168.218.180-190

### Критерии завершения Phase 6

- [ ] `helm lint` проходит
- [ ] Standalone: SE в K8s, все endpoints работают
- [ ] Replicated: 2 реплики, leader/follower, failover
- [ ] TLS через cert-manager
- [ ] /metrics доступен для Prometheus
- [ ] README.md с инструкциями по деплою

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
