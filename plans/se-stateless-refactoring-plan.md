# План рефакторинга: SE Stateless Architecture

## Метаданные

- **Версия плана**: 1.0.0
- **Дата создания**: 2026-03-01
- **Последнее обновление**: 2026-03-01
- **Статус**: Discussion

> **Внимание**: Это дискуссионный документ, а не финальный план.
> Все фазы помечены как "Not Started" и требуют обсуждения перед началом работы.

---

## История версий

- **v1.0.0** (2026-03-01): Начальная версия — дискуссионный документ

---

## Текущий статус

- **Активная фаза**: Нет (документ на стадии обсуждения)
- **Активный подпункт**: N/A
- **Последнее обновление**: 2026-03-01
- **Примечание**: Требуется обсуждение и утверждение перед началом работы

---

## Контекст и мотивация

### Текущая архитектура SE

Storage Element (SE) использует архитектуру **Leader/Follower** с NFS flock-based leader
election:

- **Leader** захватывает эксклюзивную блокировку (`flock()`) на файле `.leader.lock`
  в shared NFS-директории
- Leader обрабатывает **все** операции записи (upload, delete, mode transition)
- Leader запускает фоновые процессы: **GC** (сборка мусора) и **Reconcile** (сверка)
- **Follower** обслуживает **только чтение** (download, list, metadata)
- Follower **проксирует** write-запросы к leader через `httputil.ReverseProxy`
- Follower периодически обновляет in-memory индекс (`FollowerRefreshService`)
  и синхронизирует режим из `mode.json`
- Адрес leader записывается в `.leader.info` на shared FS
- Follower пытается захватить lock каждые 5 секунд (failover при смерти leader)

### Текущая структура кода

```
internal/
├── replica/
│   ├── election.go      — Leader election через flock() на NFS
│   ├── proxy.go         — Chi middleware: follower→leader proxy для write-запросов
│   ├── role.go          — RoleProvider интерфейс, Role enum, StandaloneProvider
│   ├── refresh.go       — FollowerRefreshService: обновление индекса/режима на follower
│   └── mode_file.go     — Чтение/запись mode.json на shared FS
├── storage/
│   ├── wal/             — WAL (Write-Ahead Log) с глобальным sync.Mutex
│   ├── filestore/       — Физические операции с файлами (SaveFile, ReadFile, Delete)
│   ├── attr/            — Чтение/запись attr.json метаданных
│   └── index/           — In-memory индекс метаданных
├── service/
│   ├── upload.go        — Upload pipeline (WAL → SaveFile → attr.json → index)
│   ├── download.go      — Download сервис (index → ReadFile → ServeContent)
│   ├── gc.go            — GC: mark expired, delete deleted files
│   └── reconcile.go     — Reconciliation: проверка целостности данных
├── server/
│   └── server.go        — HTTP сервер с TLS, proxy middleware integration
└── config/
    └── config.go        — Конфигурация (SE_REPLICA_MODE, election params)
```

### Развёртывание (K8s Helm chart)

- **Standalone mode**: `Deployment` с 1 replica, отдельные PVC для data и WAL
- **Replicated mode**: `StatefulSet` с headless Service, shared data PVC (RWX),
  WAL per-pod через `volumeClaimTemplates` (RWO)

### Проблемы текущей архитектуры

1. **Hostname resolution в K8s Deployment**: Leader записывает свой hostname
   в `.leader.info`. В K8s `Deployment` hostname pod-а (например,
   `se-edit-1-7f9b4c6d8-x2k9p`) **не резолвится** из других pod-ов.
   Для корректной работы требуется `StatefulSet` + headless Service,
   что усложняет конфигурацию.

2. **Single point of write**: Все write-операции проходят через одну ноду (leader).
   При высокой нагрузке на запись leader становится узким местом.

3. **Proxy overhead**: Follower-ы проксируют write-запросы к leader — дополнительная
   задержка и сетевой трафик.

4. **Failover latency**: При смерти leader, follower обнаруживает это только через
   retry interval (5s) + NFS v4 lease timeout (~90s). Итого до ~95s downtime
   для операций записи.

5. **Сложность кода**: Пакет `replica/` (~400 строк) реализует election, proxy,
   refresh, mode sync — всё это не нужно при stateless архитектуре.

6. **SE может работать вне K8s**: На bare metal серверах с NFS-монтированием.
   Leader election через DNS reverse lookup привязан к K8s DNS.

7. **Будущая поддержка S3**: Leader/Follower не имеет смысла при S3 backend,
   где все операции атомарны на уровне object storage.

---

## Цель рефакторинга

Перевести SE на **stateless архитектуру**, где:

- Все экземпляры SE **равноправны** (нет leader/follower)
- Одна PVC/NFS-шара на группу SE, множество pod-ов/процессов
- Горизонтальное масштабирование через добавление реплик
- Координация через **файловые блокировки на уровне отдельных файлов** (flock per-file)
  вместо глобального leader lock
- **StorageBackend** абстракция для будущей поддержки S3

### Целевая архитектура

```
SE Instance (stateless HTTP server)
    |
    +-- StorageBackend interface
    |   +-- LocalFSBackend (NFS mount, local disk)
    |   |   +-- WAL с file-level flock (блокировка на уровне WAL-транзакции)
    |   |   +-- attr.json per-file
    |   |   +-- GC через .gc.lock flock (singleton в кластере)
    |   |   +-- Reconcile через .reconcile.lock flock
    |   |
    |   +-- S3Backend (будущее)
    |       +-- Atomic PutObject (WAL не нужен)
    |       +-- Metadata в object tags или sidecar-объектах
    |
    +-- HTTP API (без изменений)
```

### Сценарии развёртывания

| Сценарий | Backend | Реплики | Координация |
|----------|---------|---------|-------------|
| K8s + NFS PVC | LocalFS | N pod-ов, 1 PVC (RWX) | flock per-file |
| Bare metal + NFS | LocalFS | N процессов, 1 NFS share | flock per-file |
| Bare metal + local disk | LocalFS | 1 процесс | flock (консистентность) |
| K8s / bare metal + S3 | S3 | N (stateless) | S3 conditional writes |

---

## Оглавление

- [ ] [Phase 1: Удаление leader election и proxy](#phase-1-удаление-leader-election-и-proxy)
- [ ] [Phase 2: StorageBackend интерфейс](#phase-2-storagebackend-интерфейс)
- [ ] [Phase 3: Обновление Helm charts и тестовой инфраструктуры](#phase-3-обновление-helm-charts-и-тестовой-инфраструктуры)
- [ ] [Phase 4: Сборка, интеграционные тесты, валидация](#phase-4-сборка-интеграционные-тесты-валидация)
- [ ] [Phase 5: S3 Backend (будущее)](#phase-5-s3-backend-будущее)

---

## Phase 1: Удаление leader election и proxy

**Dependencies**: None
**Status**: Not Started

### Описание

Удаление механизма leader/follower и замена глобальной блокировки на файловый уровень.
Все экземпляры SE становятся равноправными: каждый pod обрабатывает и чтение, и запись.

Координация между pod-ами обеспечивается через:

- **flock per WAL-транзакция**: каждая WAL-транзакция создаёт свой lock-файл
  (`{walDir}/{txID}.lock`), поэтому параллельные upload-ы от разных pod-ов
  не блокируют друг друга
- **flock для GC**: файл `.gc.lock` в `dataDir` — гарантирует, что GC запущен
  только на одном pod-е в каждый момент времени
- **flock для Reconcile**: файл `.reconcile.lock` в `dataDir` — аналогично GC

WAL-движок (`internal/storage/wal/`) уже использует `sync.Mutex` внутри одного
процесса. Для межпроцессной координации (несколько pod-ов на одном NFS)
нужен дополнительный flock на уровне WAL-директории или отдельных операций.

### Подпункты

- [ ] **1.1 Удаление пакета `internal/replica/`**
  - **Dependencies**: None
  - **Description**: Полное удаление пакета `replica`:
    - `election.go` — leader election через flock
    - `election_test.go` — тесты election
    - `proxy.go` — LeaderProxy middleware
    - `proxy_test.go` — тесты proxy
    - `role.go` — RoleProvider, Role enum, StandaloneProvider
    - `refresh.go` — FollowerRefreshService
    - `mode_file.go` — SaveMode/LoadMode (перенести в другой пакет, см. 1.4)
  - **Creates**:
    - Удалённый пакет `internal/replica/`
  - **Links**: N/A

- [ ] **1.2 Рефакторинг `cmd/storage-element/main.go`**
  - **Dependencies**: 1.1
  - **Description**: Удалить всю логику, связанную с leader election:
    - Удалить import `replica`
    - Удалить переменные `election`, `refreshSvc`, `proxyMiddleware`
    - Удалить блок `if cfg.ReplicaMode == "replicated"` — больше нет разделения
      standalone/replicated
    - GC и Reconcile запускаются **безусловно** на каждом pod-е (с flock-координацией)
    - Удалить `roleProviderAdapter`, `standaloneRoleAdapter`
    - Удалить `modePersisterAdapter` — mode.json управляется иначе (см. 1.4)
    - Упростить graceful shutdown: убрать `election.Stop()`, `refreshSvc.Stop()`
    - Удалить конфигурационный параметр `SE_REPLICA_MODE` (больше не нужен)
    - Удалить параметр `SE_ELECTION_RETRY_INTERVAL`
    - Удалить параметр `SE_INDEX_REFRESH_INTERVAL`
  - **Creates**:
    - Обновлённый `cmd/storage-element/main.go`
  - **Links**: N/A

- [ ] **1.3 Рефакторинг `internal/server/server.go`**
  - **Dependencies**: 1.1
  - **Description**: Удалить proxy-related код:
    - Удалить интерфейс `ProxyMiddleware`
    - Удалить поле `proxy` из struct `Server`
    - Удалить `proxy.Middleware` из цепочки middleware
    - Убрать `proxy` из конструктора `New()`
  - **Creates**:
    - Обновлённый `internal/server/server.go`
  - **Links**: N/A

- [ ] **1.4 Рефакторинг mode.json**
  - **Dependencies**: 1.1
  - **Description**: Перенести функции `SaveMode`/`LoadMode`/`ModeFilePath` из
    удалённого пакета `replica` в подходящий пакет (например, `internal/domain/mode/`
    или отдельный `internal/modefile/`). Логика остаётся, но убирается связь
    с leader (поле `updated_by` можно заменить на hostname:port текущего экземпляра).
    Все pod-ы могут менять режим через API — последний записавший mode.json побеждает.
    Остальные pod-ы подхватывают изменение через периодическое чтение mode.json.
  - **Creates**:
    - `internal/domain/mode/mode_file.go` (или `internal/modefile/`)
  - **Links**: N/A

- [ ] **1.5 Рефакторинг handlers: удаление RoleProvider**
  - **Dependencies**: 1.1, 1.2
  - **Description**: Удалить интерфейс `handlers.RoleProvider` и все связанные поля
    из handlers (`SystemHandler`, `HealthHandler`). Обновить API-ответы `/api/v1/info`
    и `/health/ready`:
    - Убрать поля `role`, `leader_addr` из ответа `/api/v1/info` (или всегда возвращать
      `role: "standalone"`)
    - Обновить health check: убрать зависимость от leader connectivity
    - Убрать или адаптировать поле `replica_mode` в ответе info
  - **Creates**:
    - Обновлённые handlers
  - **Links**:
    - `docs/api-contracts/storage-element-api.yaml` (может потребовать обновления)

- [ ] **1.6 File-level flock для GC и Reconcile**
  - **Dependencies**: 1.2
  - **Description**: GC и Reconcile запускаются на **каждом** pod-е, но в каждый момент
    времени должен работать только один экземпляр (singleton через flock):
    - GC: перед запуском `RunOnce()` — попытка неблокирующего flock на
      `{dataDir}/.gc.lock`
    - Если flock не получен — пропустить итерацию (другой pod уже выполняет GC)
    - Reconcile: аналогично через `{dataDir}/.reconcile.lock`
    - Блокировка удерживается на время выполнения и освобождается после завершения
    - При крахе pod-а блокировка освобождается автоматически (NFS v4 lease timeout
      или закрытие fd)
    - Graceful handling: если lock holder упал, другой pod подхватит на следующей
      итерации
  - **Creates**:
    - Обновлённые `internal/service/gc.go` и `internal/service/reconcile.go`
    - Возможно `internal/flock/flock.go` — утилита для flock операций
  - **Links**: N/A

- [ ] **1.7 Периодическая синхронизация mode.json**
  - **Dependencies**: 1.4
  - **Description**: Без leader/follower все pod-ы должны периодически читать mode.json
    для синхронизации режима. Реализовать лёгкий фоновый сервис (аналог
    `FollowerRefreshService`, но для всех pod-ов):
    - Периодическое чтение mode.json (интервал из конфига)
    - Если режим в mode.json отличается от текущего — вызов `sm.ForceMode()`
    - Опционально: пересборка индекса (если файлы могли быть добавлены/удалены
      другим pod-ом)
  - **Creates**:
    - `internal/service/modesync.go` (или аналог)
  - **Links**: N/A

- [ ] **1.8 Периодическая пересборка индекса**
  - **Dependencies**: 1.7
  - **Description**: In-memory индекс (`internal/storage/index/`) строится из attr.json
    на диске при старте. При stateless архитектуре, когда несколько pod-ов записывают
    файлы на общий NFS, индекс каждого pod-а может стать неактуальным.
    Варианты решения (требуют обсуждения):
    - **Вариант A**: Периодическая полная пересборка индекса из attr.json
      (простой, но O(N) по количеству файлов)
    - **Вариант B**: inotify/fsnotify на директорию данных (реактивный, но не все NFS
      поддерживают inotify)
    - **Вариант C**: Принять eventual consistency индекса — каждый pod видит свои
      изменения мгновенно, чужие — после пересборки
    - Рекомендация: **Вариант A + C** — периодическая пересборка с настраиваемым
      интервалом, immediate consistency для собственных операций
  - **Creates**:
    - Обновлённый `internal/service/modesync.go` (добавить index rebuild)
    - Или отдельный `internal/service/indexsync.go`
  - **Links**: N/A

- [ ] **1.9 Обновление конфигурации**
  - **Dependencies**: 1.2, 1.6, 1.7
  - **Description**: Обновить `internal/config/config.go`:
    - **Удалить**: `SE_REPLICA_MODE`, `SE_ELECTION_RETRY_INTERVAL`,
      `SE_INDEX_REFRESH_INTERVAL` (в текущем виде)
    - **Добавить**: `SE_INDEX_SYNC_INTERVAL` — интервал пересборки индекса
      (по умолчанию 30s, аналог старого `SE_INDEX_REFRESH_INTERVAL`)
    - **Добавить**: `SE_MODE_SYNC_INTERVAL` — интервал синхронизации mode.json
      (по умолчанию 10s)
    - Поля `ReplicaMode`, `IndexRefreshInterval`, `ElectionRetryInterval` удаляются
      из struct `Config`
  - **Creates**:
    - Обновлённый `internal/config/config.go`
  - **Links**: N/A

- [ ] **1.10 Обновление unit-тестов**
  - **Dependencies**: 1.1 - 1.9
  - **Description**: Удалить тесты для удалённых компонентов:
    - `internal/replica/election_test.go`
    - `internal/replica/proxy_test.go`
    - Обновить тесты GC и Reconcile (добавить flock-мок или тесты с flock)
    - Добавить тесты для mode sync и index sync сервисов
  - **Creates**:
    - Обновлённые тесты
  - **Links**: N/A

### Критерии завершения Phase 1

- [ ] Все подпункты завершены (1.1 - 1.10)
- [ ] Пакет `internal/replica/` полностью удалён
- [ ] `SE_REPLICA_MODE` больше не используется
- [ ] Нет упоминаний leader/follower в коде (кроме комментариев в истории)
- [ ] GC и Reconcile используют flock для координации между pod-ами
- [ ] Mode.json синхронизируется между pod-ами без leader
- [ ] In-memory индекс периодически обновляется
- [ ] `go test ./...` проходит без ошибок
- [ ] `go vet ./...` без предупреждений

---

## Phase 2: StorageBackend интерфейс

**Dependencies**: Phase 1
**Status**: Not Started

### Описание

Выделение абстракции `StorageBackend` — интерфейса, через который SE работает
с хранилищем. Текущая реализация (`filestore`, `wal`, `attr`, `index`) оборачивается
в `LocalFSBackend`. В будущем добавляется `S3Backend` (Phase 5).

Цель — разделить бизнес-логику SE (upload pipeline, download, GC) от конкретной
реализации хранилища. Это позволит:

- Добавлять новые backend-ы без изменения сервисного слоя
- Тестировать бизнес-логику через mock backend
- Конфигурировать backend через env-переменные (`SE_STORAGE_BACKEND=localfs|s3`)

### Предлагаемый интерфейс

```go
// StorageBackend — абстракция над физическим хранилищем файлов.
type StorageBackend interface {
    // Store сохраняет файл из reader, возвращает результат с путём, размером и checksum.
    // Операция атомарна (WAL для LocalFS, PutObject для S3).
    Store(ctx context.Context, params StoreParams) (*StoreResult, error)

    // Get возвращает reader для чтения файла и его метаданные.
    Get(ctx context.Context, fileID string) (io.ReadCloser, *FileInfo, error)

    // Delete помечает файл как deleted (soft delete через attr/metadata).
    Delete(ctx context.Context, fileID string) error

    // Stat возвращает информацию о файле (существование, размер, checksum).
    Stat(ctx context.Context, fileID string) (*FileInfo, error)

    // List возвращает список файлов с пагинацией и фильтрацией.
    List(ctx context.Context, opts ListOptions) ([]FileInfo, error)

    // Available возвращает доступное место в хранилище (байты).
    Available(ctx context.Context) (int64, error)

    // RunGC выполняет один цикл сборки мусора.
    RunGC(ctx context.Context) (*GCResult, error)

    // RunReconcile выполняет один цикл сверки.
    RunReconcile(ctx context.Context) (*ReconcileResult, error)
}
```

### Подпункты

- [ ] **2.1 Определение интерфейса `StorageBackend`**
  - **Dependencies**: None
  - **Description**: Создать пакет `internal/backend/` с определением интерфейса
    `StorageBackend` и вспомогательных типов (`StoreParams`, `StoreResult`,
    `FileInfo`, `ListOptions`, `GCResult`, `ReconcileResult`).
    Типы должны быть backend-агностичными (без привязки к файловой системе или S3).
  - **Creates**:
    - `internal/backend/backend.go` — интерфейс и типы
  - **Links**: N/A

- [ ] **2.2 Реализация `LocalFSBackend`**
  - **Dependencies**: 2.1
  - **Description**: Обернуть существующие компоненты (`filestore`, `wal`, `attr`,
    `index`) в реализацию `LocalFSBackend`:
    - `Store()` → WAL StartTransaction + SaveFile + WriteAttr + index.Add + WAL Commit
    - `Get()` → index.Get + ReadFile
    - `Delete()` → index.Get + UpdateAttr(deleted) + index.Update
    - `Stat()` → index.Get
    - `List()` → index.List
    - `Available()` → MaxCapacity - index.TotalActiveSize
    - `RunGC()` → GCService.RunOnce с flock
    - `RunReconcile()` → ReconcileService.RunOnce с flock
  - **Creates**:
    - `internal/backend/localfs/localfs.go`
    - `internal/backend/localfs/localfs_test.go`
  - **Links**: N/A

- [ ] **2.3 Рефакторинг сервисного слоя**
  - **Dependencies**: 2.2
  - **Description**: Обновить `UploadService`, `DownloadService` для работы через
    `StorageBackend` интерфейс вместо прямых вызовов `filestore`, `wal`, `attr`, `index`.
    Сервисы больше не знают о деталях хранилища.
  - **Creates**:
    - Обновлённые `internal/service/upload.go`, `internal/service/download.go`
  - **Links**: N/A

- [ ] **2.4 Фабрика backend-ов**
  - **Dependencies**: 2.2
  - **Description**: Создать фабрику, которая создаёт backend на основе конфигурации:
    - `SE_STORAGE_BACKEND=localfs` (по умолчанию) → `LocalFSBackend`
    - `SE_STORAGE_BACKEND=s3` → `S3Backend` (future)
    - Обновить `main.go` для использования фабрики
  - **Creates**:
    - `internal/backend/factory.go`
    - Обновлённый `cmd/storage-element/main.go`
  - **Links**: N/A

- [ ] **2.5 Unit-тесты для backend**
  - **Dependencies**: 2.2, 2.3
  - **Description**: Написать тесты:
    - Тесты для `LocalFSBackend` (используя temp-директории)
    - Mock backend для тестирования сервисного слоя
    - Тесты фабрики backend-ов
  - **Creates**:
    - `internal/backend/localfs/localfs_test.go`
    - `internal/backend/mock_test.go`
  - **Links**: N/A

### Критерии завершения Phase 2

- [ ] Все подпункты завершены (2.1 - 2.5)
- [ ] Интерфейс `StorageBackend` определён и реализован для LocalFS
- [ ] Сервисный слой работает через `StorageBackend`
- [ ] `go test ./...` проходит без ошибок
- [ ] Существующая функциональность не нарушена (API backward compatible)

---

## Phase 3: Обновление Helm charts и тестовой инфраструктуры

**Dependencies**: Phase 1
**Status**: Not Started

### Описание

Обновление Helm charts для нового stateless режима:

- Удаление StatefulSet template (больше не нужен)
- Deployment с replicas > 1 для edit/rw SE
- Shared PVC (RWX) для данных, shared PVC для WAL (или WAL в data dir)
- Удаление headless Service (больше не нужен для leader resolution)
- Обновление тестовой инфраструктуры в `tests/helm/artstore-se/`

### Подпункты

- [ ] **3.1 Обновление production Helm chart**
  - **Dependencies**: None
  - **Description**: Обновить `src/storage-element/charts/storage-element/`:
    - Удалить `templates/statefulset.yaml`
    - Удалить `templates/service-headless.yaml`
    - Обновить `templates/deployment.yaml`: убрать условие `standalone`, поддержать
      `replicas > 1`, shared PVC (RWX)
    - Обновить `values.yaml`: убрать `replicaMode`, обновить defaults
    - Обновить env-переменные: убрать `SE_REPLICA_MODE`, `SE_ELECTION_RETRY_INTERVAL`
    - Добавить `SE_INDEX_SYNC_INTERVAL`, `SE_MODE_SYNC_INTERVAL`
    - WAL-директория: вариант A — отдельный shared PVC (RWX), вариант B — поддиректория
      в data PVC. **Требует обсуждения.**
  - **Creates**:
    - Обновлённый Helm chart
  - **Links**: N/A

- [ ] **3.2 Обновление тестового Helm chart**
  - **Dependencies**: 3.1
  - **Description**: Обновить `tests/helm/artstore-se/`:
    - Обновить templates для stateless SE
    - Edit SE может иметь `replicas: 2` (тестирование параллельной записи)
    - RO/AR SE — `replicas: 1` (не нужно масштабирование для read-only)
  - **Creates**:
    - Обновлённый тестовый chart
  - **Links**: N/A

- [ ] **3.3 WAL-директория: решение по архитектуре**
  - **Dependencies**: None
  - **Description**: Выбрать стратегию для WAL в stateless режиме:
    - **Вариант A**: WAL в поддиректории data PVC (`{dataDir}/.wal/`).
      Pros: один PVC, простота. Cons: WAL-файлы на NFS (медленнее).
    - **Вариант B**: Отдельный shared WAL PVC (RWX).
      Pros: можно использовать быстрый storage. Cons: ещё один PVC.
    - **Вариант C**: WAL per-pod (emptyDir или local PVC).
      Pros: быстрый WAL. Cons: WAL теряется при рестарте pod-а (нужна
      recover-логика, что уже есть).
    - Рекомендация: **Вариант C** — WAL per-pod через emptyDir. WAL recovery
      при старте уже реализована. Незавершённые WAL-транзакции откатываются
      автоматически.
  - **Creates**:
    - Решение, зафиксированное в этом документе
  - **Links**: N/A

### Критерии завершения Phase 3

- [ ] Все подпункты завершены (3.1 - 3.3)
- [ ] Helm chart поддерживает `replicas > 1` для edit/rw SE
- [ ] StatefulSet и headless Service удалены
- [ ] Тестовый chart обновлён

---

## Phase 4: Сборка, интеграционные тесты, валидация

**Dependencies**: Phase 1, Phase 3
**Status**: Not Started

### Описание

Сборка Docker-образа, развёртывание в тестовом K8s-кластере и проверка
корректности работы SE в stateless режиме.

### Подпункты

- [ ] **4.1 Сборка Docker-образа**
  - **Dependencies**: None
  - **Description**: Собрать Docker-образ SE с новой архитектурой.
    Обновить Dockerfile если нужно. Тег: `v0.X.Y-N` (суффикс инкрементируется).
  - **Creates**:
    - Docker-образ в Harbor
  - **Links**: N/A

- [ ] **4.2 Развёртывание в тестовом кластере**
  - **Dependencies**: 4.1, 3.2
  - **Description**: Развернуть SE в namespace `artstore-test`:
    - Edit SE с 2 репликами (проверка параллельной записи)
    - RW SE с 1 репликой
    - RO/AR SE с 1 репликой
    - Проверить запуск pod-ов, readiness/liveness probes
  - **Creates**:
    - Рабочее развёртывание в K8s
  - **Links**: N/A

- [ ] **4.3 Интеграционные тесты: параллельная запись**
  - **Dependencies**: 4.2
  - **Description**: Тесты для проверки корректности при параллельной записи
    с нескольких pod-ов:
    - Upload файла через pod-1, проверка доступности через pod-2
    - Одновременный upload через оба pod-а (race condition тест)
    - Delete через pod-1, проверка отсутствия через pod-2
    - Mode transition через pod-1, проверка синхронизации на pod-2
  - **Creates**:
    - Интеграционные тесты в `tests/scripts/`
  - **Links**: N/A

- [ ] **4.4 Интеграционные тесты: GC и Reconcile**
  - **Dependencies**: 4.2
  - **Description**: Проверка корректности singleton-координации GC/Reconcile:
    - Запуск GC на обоих pod-ах — только один должен выполнять работу
    - Reconcile с двумя pod-ами
    - Проверка через метрики (только один pod увеличивает `se_gc_runs_total`)
  - **Creates**:
    - Интеграционные тесты
  - **Links**: N/A

- [ ] **4.5 Регрессионные тесты**
  - **Dependencies**: 4.2
  - **Description**: Прогон существующих интеграционных тестов SE для проверки
    backward compatibility. Все текущие тесты должны проходить без изменений.
  - **Creates**:
    - Результаты тестирования
  - **Links**: N/A

### Критерии завершения Phase 4

- [ ] Все подпункты завершены (4.1 - 4.5)
- [ ] Docker-образ собран и загружен в Harbor
- [ ] SE работает в K8s с `replicas > 1`
- [ ] Параллельная запись корректна (нет потери данных, нет гонок)
- [ ] GC/Reconcile singleton координация работает
- [ ] Все существующие интеграционные тесты проходят
- [ ] Документация обновлена

---

## Phase 5: S3 Backend (будущее)

**Dependencies**: Phase 2
**Status**: Not Started

### Описание

Реализация S3-совместимого backend-а для SE. S3 обеспечивает атомарность на уровне
`PutObject`, поэтому WAL не нужен. Метаданные хранятся в object tags или
sidecar-объектах (`{fileID}.attr.json`).

Эта фаза является перспективной и будет детализирована при необходимости.

### Подпункты

- [ ] **5.1 Проектирование S3Backend**
  - **Dependencies**: None
  - **Description**: Разработка дизайна S3Backend:
    - Структура bucket-а (key naming convention)
    - Хранение метаданных: object metadata vs sidecar objects
    - Conditional writes (S3 `If-None-Match` для предотвращения перезаписи)
    - Listing и пагинация (S3 ListObjectsV2)
    - GC и Reconcile для S3 (lifecycle rules vs application-level)
    - Eventual consistency для listing операций
  - **Creates**:
    - Дизайн-документ
  - **Links**:
    - [AWS S3 Conditional Writes](https://aws.amazon.com/about-aws/whats-new/2024/08/amazon-s3-conditional-writes/)

- [ ] **5.2 Реализация S3Backend**
  - **Dependencies**: 5.1
  - **Description**: Реализация `StorageBackend` для S3:
    - `Store()` → PutObject с conditional write
    - `Get()` → GetObject
    - `Delete()` → DeleteObject (или soft delete через tag)
    - `Stat()` → HeadObject
    - `List()` → ListObjectsV2
    - `Available()` → бесконечно (или конфигурируемый лимит)
    - `RunGC()` → listing + delete expired
    - `RunReconcile()` → listing + validation
  - **Creates**:
    - `internal/backend/s3/s3.go`
    - `internal/backend/s3/s3_test.go`
  - **Links**: N/A

- [ ] **5.3 Конфигурация S3**
  - **Dependencies**: 5.2
  - **Description**: Добавить конфигурацию для S3:
    - `SE_S3_ENDPOINT` — endpoint S3-совместимого хранилища
    - `SE_S3_BUCKET` — имя bucket-а
    - `SE_S3_REGION` — регион
    - `SE_S3_ACCESS_KEY` / `SE_S3_SECRET_KEY` — credentials
    - `SE_S3_USE_PATH_STYLE` — path-style addressing (для MinIO)
  - **Creates**:
    - Обновлённый `internal/config/config.go`
  - **Links**: N/A

- [ ] **5.4 Тестирование S3Backend**
  - **Dependencies**: 5.2, 5.3
  - **Description**: Тестирование с MinIO:
    - Unit-тесты с testcontainers (MinIO)
    - Интеграционные тесты в K8s с MinIO
  - **Creates**:
    - Тесты
  - **Links**: N/A

### Критерии завершения Phase 5

- [ ] Все подпункты завершены (5.1 - 5.4)
- [ ] S3Backend реализован и протестирован
- [ ] SE работает с S3 backend (MinIO)
- [ ] Документация обновлена
- [ ] API backward compatible

---

## Влияние на другие модули

### Backward Compatibility

| Компонент | Влияние |
|-----------|---------|
| **SE HTTP API** | Без изменений. Все endpoints сохраняются. |
| **Admin Module** | Без изменений. AM регистрирует SE по URL, внутренняя архитектура SE для AM прозрачна. |
| **Ingester Module** | Без изменений. IM обращается к SE через K8s Service (load balancing). |
| **Query Module** | Без изменений. QM скачивает файлы через SE API. |

### Изменения API (минимальные)

- `/api/v1/info`: поле `replica_mode` можно убрать или всегда возвращать `"standalone"`.
  Поля `role` и `leader_addr` убираются.
- `/health/ready`: убирается проверка leader connectivity.

### Производительность

- **Положительное влияние**: Нет proxy overhead при записи. Нативный K8s load balancing
  распределяет нагрузку равномерно.
- **NFS latency**: Файловые блокировки (flock) на NFS добавляют латенсию. Для LocalFS
  backend это приемлемо — flock удерживается кратковременно (на время одной операции).
- **Index eventual consistency**: Индекс одного pod-а может отставать от реального
  состояния NFS. Периодическая пересборка (30s) — компромисс.

---

## Риски и митигации

| Риск | Вероятность | Влияние | Митигация |
|------|-------------|---------|-----------|
| NFS flock ненадёжен на некоторых NFS-реализациях | Средняя | Высокое | Тестирование на целевом NFS; fallback на advisory locking; документирование требований к NFS v4+ |
| Гранулярность файловых блокировок: per-file vs per-directory | Средняя | Среднее | WAL-транзакции уже per-file (UUID); flock на WAL-файл, а не на директорию |
| GC singleton через flock — lock holder может зависнуть | Низкая | Среднее | Таймаут на GC-операцию; NFS lease автоматически освобождает flock при смерти процесса |
| S3 eventual consistency для listing | Средняя (Phase 5) | Среднее | S3 strong consistency (с 2020); для совместимых хранилищ — документировать ограничения |
| Index desync между pod-ами | Средняя | Среднее | Периодическая пересборка (30s default); не критично для upload (каждый pod добавляет в свой индекс сразу) |
| Параллельный upload одинакового файла | Низкая | Низкое | UUID в имени файла гарантирует уникальность; attr.json per-file |
| WAL per-pod (emptyDir) — потеря при рестарте | Средняя | Низкое | WAL recovery уже реализована (rollback pending); pending операции = незавершённые upload-ы, которые откатываются |

---

## Открытые вопросы для обсуждения

1. **WAL-директория**: Shared PVC, отдельный PVC или emptyDir per-pod?
   Рекомендация — emptyDir, но нужно обсудить.

2. **Index sync интервал**: 30s по умолчанию — достаточно? Нужен ли более
   реактивный механизм (fsnotify)?

3. **Mode.json конфликт**: Если два pod-а одновременно меняют режим через API,
   последний записавший побеждает. Это приемлемо? Нужен ли flock на mode.json?

4. **API изменения**: Убираем поля `role`, `leader_addr`, `replica_mode` из ответа
   `/api/v1/info`? Или оставляем для backward compatibility с фиксированными
   значениями?

5. **Версионирование**: Этот рефакторинг — minor bump (`0.X.0`) или patch
   (`0.Y.Z+1`)? Рекомендация — minor bump, т.к. архитектурное изменение.

6. **Phase 2 (StorageBackend)**: Выполнять сразу или отложить до момента,
   когда реально понадобится S3? Можно ограничиться Phase 1 + Phase 3 + Phase 4
   и получить рабочий stateless SE без абстракции backend.

---

## Примечания

- Текущий SE API-контракт (`docs/api-contracts/storage-element-api.yaml`)
  не требует изменений на уровне endpoints. Изменения затрагивают только
  внутреннюю архитектуру и некоторые поля в ответах `/api/v1/info`.
- Перед началом Phase 1 рекомендуется создать отдельную ветку
  `refactor/se-stateless` от `main`.
- Каждая фаза — один контекст AI (одна сессия разработки).
