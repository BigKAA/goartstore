# План рефакторинга: SE Stateless Architecture

## Метаданные

- **Версия плана**: 2.0.0
- **Дата создания**: 2026-03-01
- **Последнее обновление**: 2026-03-01
- **Статус**: In Progress (Phase 2 завершена)

---

## История версий

- **v1.0.0** (2026-03-01): Начальная версия — дискуссионный документ
- **v2.0.0** (2026-03-01): Полная переработка после обсуждения. Ключевые решения:
  - Удаление WAL (заменён на per-file lock с TTL)
  - Удаление flock-based координации (GC/Reconcile идемпотентны)
  - Добавлен Lock API (`GET /api/v1/locks`, `POST /api/v1/locks/cleanup`)
  - DELETE при активном lock → 409 Conflict

---

## Текущий статус

- **Активная фаза**: Phase 2 завершена
- **Активный подпункт**: N/A
- **Последнее обновление**: 2026-03-01

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
- Адрес leader записывается в `.leader.info` на shared FS
- Для записи используется **WAL** (Write-Ahead Log) с глобальным `sync.Mutex`

### Проблемы текущей архитектуры

1. **Hostname resolution в K8s Deployment**: Leader записывает свой hostname
   в `.leader.info`. В K8s `Deployment` hostname pod-а **не резолвится** из других pod-ов.
   Требуется `StatefulSet` + headless Service.

2. **Single point of write**: Все write-операции проходят через leader — узкое место.

3. **Proxy overhead**: Follower проксирует write-запросы к leader — задержка и трафик.

4. **Failover latency**: При смерти leader — до ~95s downtime (5s retry + 90s NFS lease).

5. **Сложность кода**: Пакет `replica/` (~1100 строк) + WAL (~300 строк) — не нужны
   при stateless архитектуре.

6. **Привязка к K8s**: Leader election через DNS reverse lookup привязан к K8s DNS.

7. **Несовместимость с S3**: Leader/Follower и WAL не имеют смысла при S3 backend.

8. **WAL на shared storage**: WAL спроектирован для одного процесса (`sync.Mutex`).
   При нескольких pod-ах на shared NFS WAL не обеспечивает межпроцессную координацию.
   WAL per-pod (emptyDir) теряется при рестарте pod-а, делая recovery невозможной.

---

## Целевая архитектура

### Принципы

- Все экземпляры SE **равноправны** (нет leader/follower)
- Одна PVC/NFS-шара (или S3 bucket) на группу SE, множество pod-ов/процессов
- Горизонтальное масштабирование через добавление реплик
- Координация через **per-file lock-файлы с TTL** вместо flock/WAL
- **WAL удалён** — его функции заменены lock-файлами + atomic writes + GC cleanup
- GC и Reconcile **идемпотентны**, запускаются на каждом pod-е без singleton-координации
- **StorageBackend** абстракция для будущей поддержки S3

### Схема

```
SE Instance (stateless HTTP server)
    |
    +-- StorageBackend interface
    |   +-- LocalFSBackend (NFS mount, local disk)
    |   |   +-- Per-file lock ({dataDir}/.locks/{fileID}.lock) с TTL
    |   |   +-- Atomic writes (temp → fsync → rename)
    |   |   +-- attr.json per-file (source of truth)
    |   |   +-- GC идемпотентный (проверяет .locks/ перед удалением)
    |   |   +-- Reconcile идемпотентный (проверяет .locks/ перед cleanup)
    |   |
    |   +-- S3Backend (будущее)
    |       +-- Atomic PutObject (lock не нужен)
    |       +-- Metadata в object tags или sidecar-объектах
    |
    +-- Lock API (GET /api/v1/locks, POST /api/v1/locks/cleanup)
    +-- HTTP API (без изменений кроме lock API и удалённых полей)
```

### Upload pipeline (новый, без WAL)

```
1. Создать .locks/{fileID}.lock (JSON: holder, acquired_at, ttl_seconds)
2. filestore.SaveFile()             ← atomic: temp → fsync → rename
3. attr.WriteAttr()                 ← atomic: temp → fsync → rename
4. index.Add()                      ← in-memory
5. Удалить .locks/{fileID}.lock
```

**Crash recovery без WAL:**

| Crash после шага | Состояние на диске | Восстановление |
|---|---|---|
| 1 | lock есть, данных нет | TTL истечёт → cleanup удалит lock |
| 2 | lock + data, нет attr.json | TTL истечёт → GC увидит orphaned file → удалит |
| 3 | lock + data + attr.json | TTL истечёт → index rebuild подхватит из attr.json → **файл доступен** |
| 4 | lock + data + attr + index | Lock удалится при cleanup, всё работает |

**Шаг 3 (WriteAttr) — point of no return.** Если attr.json записан — файл считается
закоммиченным. Lock-файл — только защита от преждевременного GC.

### Per-file lock: формат и алгоритм

**Путь**: `{dataDir}/.locks/{fileID}.lock`

**Формат (JSON)**:
```json
{
  "holder": "se-edit-1-7f9b4c6d8-x2k9p",
  "file_id": "abc-123-def-456",
  "acquired_at": "2026-03-01T12:00:00Z",
  "ttl_seconds": 120
}
```

**Алгоритм захвата**:
1. `os.Create()` — fileID уникален (UUID v4), коллизий нет
2. Записать JSON метаданные
3. Выполнить операцию записи
4. `os.Remove()` — удалить lock после завершения

**Проверка TTL** (GC, Reconcile, Delete):
1. Прочитать lock-файл
2. Парсить JSON → проверить `acquired_at + ttl_seconds`
3. Если TTL не истёк → пропустить файл
4. Если TTL истёк → файл можно обрабатывать (pod-владелец мёртв)

### Сценарии развёртывания

| Сценарий | Backend | Реплики | Координация |
|----------|---------|---------|-------------|
| K8s + NFS PVC | LocalFS | N pod-ов, 1 PVC (RWX) | per-file lock |
| Bare metal + NFS | LocalFS | N процессов, 1 NFS share | per-file lock |
| Bare metal + local disk | LocalFS | 1 процесс | per-file lock (консистентность) |
| K8s / bare metal + S3 | S3 | N (stateless) | S3 conditional writes |

---

## Важные архитектурные решения

> **ВАЖНО: DELETE при активном lock → 409 Conflict**
>
> Если клиент отправляет `DELETE /api/v1/files/{fileId}`, а файл в данный момент
> загружается (lock-файл существует и TTL не истёк), SE возвращает **409 Conflict**
> с сообщением "file is currently being uploaded".
>
> Это предотвращает race condition: upload завершается и перезаписывает attr.json
> без флага `deleted`, "воскрешая" файл. Клиент должен повторить DELETE после
> завершения upload-а.
>
> **Это поведение должно быть отражено в OpenAPI-спецификации и документации.**

---

## Оглавление

- [x] [Phase 1: Удаление legacy-компонентов и пакет lockfile](#phase-1-удаление-legacy-компонентов-и-пакет-lockfile)
- [x] [Phase 2: Рефакторинг upload pipeline и сервисов](#phase-2-рефакторинг-upload-pipeline-и-сервисов)
- [ ] [Phase 3: Lock API, handlers, конфигурация](#phase-3-lock-api-handlers-конфигурация)
- [ ] [Phase 4: Периодическая синхронизация и обновление Helm charts](#phase-4-периодическая-синхронизация-и-обновление-helm-charts)
- [ ] [Phase 5: Сборка, интеграционные тесты, валидация](#phase-5-сборка-интеграционные-тесты-валидация)
- [ ] [Phase 6: StorageBackend интерфейс (будущее)](#phase-6-storagebackend-интерфейс-будущее)
- [ ] [Phase 7: S3 Backend (будущее)](#phase-7-s3-backend-будущее)

---

## Phase 1: Удаление legacy-компонентов и пакет lockfile

**Dependencies**: None
**Status**: Done

### Описание

Удаление устаревших компонентов (leader election, proxy, WAL) и создание нового
пакета `internal/lockfile/` — основы координации в stateless архитектуре.

### Подпункты

- [x] **1.1 Создание пакета `internal/lockfile/`**
  - **Dependencies**: None
  - **Description**: Реализовать пакет для работы с per-file lock-файлами:
    - Структура `LockManager` — управление lock-файлами в директории
      `{dataDir}/.locks/`
    - `Acquire(fileID string) error` — создать lock-файл с JSON метаданными
      (holder, file_id, acquired_at, ttl_seconds). Holder = hostname или pod name.
    - `Release(fileID string) error` — удалить lock-файл
    - `IsLocked(fileID string) (bool, *LockInfo, error)` — проверить наличие lock-а
      и его TTL. Возвращает false если lock не существует или TTL истёк.
    - `List() ([]LockInfo, error)` — список всех активных lock-ов
    - `Cleanup(force bool) (*CleanupResult, error)` — удалить expired lock-и.
      При `force=true` — удалить все lock-и (включая активные).
    - `EnsureDir() error` — создать директорию `.locks/` при старте
    - Конфигурация: `dataDir` (базовая директория), `ttl` (из `SE_UPLOAD_LOCK_TTL`)
    - Holder определяется через `os.Hostname()` (работает и в K8s, и на bare metal)
  - **Creates**:
    - `internal/lockfile/lockfile.go` — LockManager, LockInfo, CleanupResult
    - `internal/lockfile/lockfile_test.go` — unit-тесты
  - **Links**: N/A

- [x] **1.2 Удаление пакета `internal/replica/`**
  - **Dependencies**: None
  - **Description**: Полное удаление пакета `replica` (~1100 строк):
    - `election.go` — leader election через flock
    - `election_test.go` — тесты election
    - `proxy.go` — LeaderProxy middleware
    - `proxy_test.go` — тесты proxy
    - `role.go` — RoleProvider, Role enum, StandaloneProvider
    - `refresh.go` — FollowerRefreshService
    - `mode_file.go` — SaveMode/LoadMode (перенести в 1.4)
  - **Creates**:
    - Удалённый пакет `internal/replica/`
  - **Links**: N/A

- [x] **1.3 Удаление пакета `internal/storage/wal/`**
  - **Dependencies**: None
  - **Description**: Полное удаление WAL (~300 строк). WAL заменён на per-file lock
    с TTL + atomic writes + GC cleanup. Файлы для удаления:
    - `wal.go` — WAL engine (StartTransaction, Commit, Rollback, RecoverPending,
      CleanCommitted)
    - `entry.go` — WALEntry, TransactionState
    - `wal_test.go` — тесты
  - **Creates**:
    - Удалённый пакет `internal/storage/wal/`
  - **Links**: N/A

- [x] **1.4 Перенос mode.json в отдельный пакет**
  - **Dependencies**: 1.2
  - **Description**: Перенести функции `SaveMode`/`LoadMode`/`ModeFilePath` из
    удалённого пакета `replica` в `internal/modefile/`. Логика остаётся, но:
    - Убрать связь с leader (поле `updated_by` → hostname текущего экземпляра)
    - Все pod-ы могут менять режим через API — последний записавший побеждает
    - Остальные pod-ы подхватывают изменение через периодическое чтение mode.json
  - **Creates**:
    - `internal/modefile/modefile.go`
    - `internal/modefile/modefile_test.go`
  - **Links**: N/A

### Критерии завершения Phase 1

- [x] Все подпункты завершены (1.1 - 1.4)
- [x] Пакет `internal/lockfile/` создан и покрыт тестами
- [x] Пакет `internal/replica/` полностью удалён
- [x] Пакет `internal/storage/wal/` полностью удалён
- [x] Функции mode.json перенесены в `internal/modefile/`
- [x] Код компилируется (допускаются неиспользуемые импорты в зависимых пакетах)

---

## Phase 2: Рефакторинг upload pipeline и сервисов

**Dependencies**: Phase 1
**Status**: Done

### Описание

Переработка upload pipeline (замена WAL на lock-файлы), рефакторинг GC и Reconcile
(идемпотентные, без singleton), рефакторинг delete (проверка lock → 409 Conflict).

### Подпункты

- [x] **2.1 Рефакторинг `internal/service/upload.go`**
  - **Dependencies**: None
  - **Description**: Переработать upload pipeline:
    - **Убрать**: все вызовы WAL (StartTransaction, Commit, Rollback)
    - **Добавить**: `lockManager.Acquire(fileID)` перед SaveFile
    - **Добавить**: `lockManager.Release(fileID)` после завершения (в defer)
    - Новый pipeline:
      1. `lockManager.Acquire(fileID)`
      2. `filestore.SaveFile()` (atomic: temp → fsync → rename)
      3. `attr.WriteAttr()` (atomic: temp → fsync → rename)
      4. `index.Add()`
      5. `defer lockManager.Release(fileID)`
    - Обработка ошибок: если SaveFile или WriteAttr failed — Release lock,
      попытаться удалить частично записанные файлы (best effort cleanup)
  - **Creates**:
    - Обновлённый `internal/service/upload.go`
  - **Links**: N/A

- [x] **2.2 Рефакторинг `internal/service/gc.go`**
  - **Dependencies**: None
  - **Description**: Сделать GC идемпотентным и lock-aware:
    - **Убрать**: `sync.Mutex` singleton-координацию (каждый pod запускает свой GC)
    - **Добавить**: проверку lock-файла перед удалением:
      - `lockManager.IsLocked(fileID)` → если locked и TTL не истёк → пропустить
    - Phase 1 (mark expired): без изменений — проверка TTL в attr.json
    - Phase 2 (delete): перед физическим удалением проверить lock
    - Удаление файлов идемпотентно: `os.Remove()` на уже удалённый файл → игнорировать
    - Orphaned files (data без attr.json): проверить lock перед удалением
  - **Creates**:
    - Обновлённый `internal/service/gc.go`
  - **Links**: N/A

- [x] **2.3 Рефакторинг `internal/service/reconcile.go`**
  - **Dependencies**: None
  - **Description**: Сделать Reconcile идемпотентным и lock-aware:
    - **Убрать**: `sync.Mutex` singleton-координацию и флаг `inProcess`
    - **Добавить**: при обнаружении `orphaned_file` или `orphaned_attr` —
      проверить lock перед принятием решения. Если lock активен → пропустить
      (файл ещё загружается)
    - Index rebuild после reconcile — обновляет in-memory индекс текущего pod-а
  - **Creates**:
    - Обновлённый `internal/service/reconcile.go`
  - **Links**: N/A

- [x] **2.4 Рефакторинг delete в handlers**
  - **Dependencies**: None
  - **Description**: Обновить handler `DELETE /api/v1/files/{fileId}`:
    - Перед выполнением soft delete проверить `lockManager.IsLocked(fileID)`
    - Если lock активен (TTL не истёк) → вернуть **409 Conflict** с телом:
      ```json
      {
        "code": "FILE_UPLOAD_IN_PROGRESS",
        "message": "Cannot delete file while upload is in progress",
        "details": {
          "file_id": "...",
          "holder": "...",
          "expires_at": "..."
        }
      }
      ```
    - Если lock отсутствует или TTL истёк → выполнить soft delete как обычно
  - **Creates**:
    - Обновлённый `internal/api/handlers/files.go`
  - **Links**: N/A

- [x] **2.5 Unit-тесты для обновлённых сервисов**
  - **Dependencies**: 2.1, 2.2, 2.3, 2.4
  - **Description**: Обновить и добавить unit-тесты:
    - Тесты upload pipeline с lock (acquire → save → attr → release)
    - Тесты upload при ошибке (lock release + cleanup)
    - Тесты GC: пропуск locked файлов, удаление unlocked/expired
    - Тесты Reconcile: пропуск locked файлов
    - Тесты delete: 409 при активном lock, success при отсутствии lock
    - Удалить тесты WAL (wal_test.go уже удалён в Phase 1)
  - **Creates**:
    - Обновлённые тесты в `internal/service/` и `internal/api/handlers/`
  - **Links**: N/A

### Критерии завершения Phase 2

- [x] Все подпункты завершены (2.1 - 2.5)
- [x] Upload pipeline работает без WAL (lock → save → attr → index → unlock)
- [x] GC и Reconcile идемпотентны, проверяют lock перед удалением
- [x] DELETE возвращает 409 при активном lock
- [x] Нет упоминаний WAL в сервисном слое
- [x] `go test ./...` проходит без ошибок

---

## Phase 3: Lock API, handlers, конфигурация

**Dependencies**: Phase 2
**Status**: Pending

### Описание

Добавление Lock API (диагностика и очистка), удаление legacy handlers
(RoleProvider, ProxyMiddleware), обновление конфигурации.

### Подпункты

- [ ] **3.1 Lock API handlers**
  - **Dependencies**: None
  - **Description**: Добавить два новых endpoint-а:
    - `GET /api/v1/locks` — просмотр текущих lock-ов (диагностика):
      ```json
      {
        "locks": [
          {
            "file_id": "abc-123",
            "holder": "se-edit-1-7f9b4c6d8-x2k9p",
            "acquired_at": "2026-03-01T12:00:00Z",
            "ttl_seconds": 120,
            "expires_at": "2026-03-01T12:02:00Z",
            "expired": false
          }
        ],
        "total": 1,
        "active": 1,
        "expired": 0
      }
      ```
    - `POST /api/v1/locks/cleanup` — удаление expired lock-ов:
      - Без параметров: удалить только expired lock-и
      - С параметром `?force=true`: удалить все lock-и (аварийная очистка)
      - Ответ:
        ```json
        {
          "cleaned": 3,
          "remaining": 1,
          "removed": [
            {"file_id": "abc-123", "holder": "...", "expired_at": "..."}
          ],
          "active": [
            {"file_id": "def-456", "holder": "...", "expires_at": "..."}
          ]
        }
        ```
    - Авторизация: оба endpoint-а доступны **только пользователям с правами RW на SE**
      (scope `storage:write` или role `admin`)
  - **Creates**:
    - `internal/api/handlers/locks.go`
    - `internal/api/handlers/locks_test.go`
  - **Links**: N/A

- [x] **3.2 Удаление RoleProvider и ProxyMiddleware из handlers** *(выполнено в Phase 1)*
  - **Dependencies**: None
  - **Description**: Удалить legacy-интерфейсы и код:
    - Удалить интерфейс `handlers.RoleProvider` и все связанные поля
      из `SystemHandler`, `HealthHandler`
    - Обновить `GET /api/v1/info`:
      - Убрать поля `role`, `leader_addr`
      - Убрать поле `replica_mode`
      - Оставить: `storage_id`, `mode`, `capacity`, `used`, `available`,
        `file_count`, `version`
    - Обновить `GET /health/ready`: убрать проверку leader connectivity
  - **Creates**:
    - Обновлённые `internal/api/handlers/system.go`, `health.go`
  - **Links**:
    - `docs/api-contracts/storage-element-openapi.yaml` (обновить)

- [x] **3.3 Удаление proxy из `internal/server/server.go`** *(выполнено в Phase 1)*
  - **Dependencies**: None
  - **Description**: Удалить proxy-related код:
    - Удалить интерфейс `ProxyMiddleware`
    - Удалить поле `proxy` из struct `Server`
    - Удалить `proxy.Middleware` из цепочки middleware
    - Убрать `proxy` из конструктора `New()`
  - **Creates**:
    - Обновлённый `internal/server/server.go`
  - **Links**: N/A

- [x] **3.4 Рефакторинг `cmd/storage-element/main.go`** *(выполнено в Phase 1)*
  - **Dependencies**: 3.1, 3.2, 3.3
  - **Description**: Переработать точку входа:
    - **Удалить**: import `replica`, `wal`
    - **Удалить**: переменные `election`, `refreshSvc`, `proxyMiddleware`
    - **Удалить**: блок `if cfg.ReplicaMode == "replicated"` — больше нет
      разделения standalone/replicated
    - **Удалить**: `roleProviderAdapter`, `standaloneRoleAdapter`
    - **Удалить**: `modePersisterAdapter` (заменить на `modefile` пакет)
    - **Удалить**: инициализацию WAL engine и recovery pending
    - **Добавить**: инициализацию `LockManager` (из пакета `lockfile`)
    - **Добавить**: `lockManager.EnsureDir()` при старте
    - GC и Reconcile запускаются **безусловно** на каждом pod-е
    - Упростить graceful shutdown: убрать `election.Stop()`, `refreshSvc.Stop()`
    - Передать `lockManager` в UploadService, GCService, ReconcileService, handlers
  - **Creates**:
    - Обновлённый `cmd/storage-element/main.go`
  - **Links**: N/A

- [x] **3.5 Обновление конфигурации** *(выполнено в Phase 1)*
  - **Dependencies**: None
  - **Description**: Обновить `internal/config/config.go`:
    - **Удалить**:
      - `SE_REPLICA_MODE` (ReplicaMode)
      - `SE_ELECTION_RETRY_INTERVAL` (ElectionRetryInterval)
      - `SE_WAL_DIR` (WalDir) — WAL удалён
    - **Переименовать**:
      - `SE_INDEX_REFRESH_INTERVAL` → `SE_INDEX_SYNC_INTERVAL` (IndexSyncInterval,
        default 30s)
    - **Добавить**:
      - `SE_UPLOAD_LOCK_TTL` (UploadLockTTL, default 120s) — TTL lock-файла
        при upload
      - `SE_MODE_SYNC_INTERVAL` (ModeSyncInterval, default 10s) — интервал
        синхронизации mode.json
    - Обновить валидацию: убрать проверки для удалённых параметров,
      добавить для новых
  - **Creates**:
    - Обновлённый `internal/config/config.go`
  - **Links**: N/A

- [ ] **3.6 Обновление OpenAPI-спецификации**
  - **Dependencies**: 3.1, 3.2
  - **Description**: Обновить `docs/api-contracts/storage-element-openapi.yaml`:
    - **Добавить** endpoint `GET /api/v1/locks`
    - **Добавить** endpoint `POST /api/v1/locks/cleanup` с query param `force`
    - **Обновить** `DELETE /api/v1/files/{fileId}` — добавить response 409 Conflict
      с описанием `FILE_UPLOAD_IN_PROGRESS`
    - **Обновить** `GET /api/v1/info` — убрать поля `role`, `leader_addr`,
      `replica_mode`
    - **Добавить** схемы: `LockInfo`, `LockListResponse`, `LockCleanupResponse`
  - **Creates**:
    - Обновлённый `docs/api-contracts/storage-element-openapi.yaml`
  - **Links**: N/A

- [ ] **3.7 Unit-тесты**
  - **Dependencies**: 3.1 - 3.6
  - **Description**: Обновить и добавить тесты:
    - Тесты Lock API handlers (GET /locks, POST /locks/cleanup, force=true)
    - Обновить тесты system handler (убрать role/leader assertions)
    - Обновить тесты health handler (убрать leader connectivity)
    - Обновить тесты config (новые/удалённые параметры)
    - Убрать все тесты, зависящие от WAL и replica
  - **Creates**:
    - Обновлённые тесты
  - **Links**: N/A

### Критерии завершения Phase 3

- [ ] Все подпункты завершены (3.1 - 3.7)
- [ ] Lock API работает (GET /locks, POST /locks/cleanup)
- [ ] Нет упоминаний leader/follower, proxy, WAL в рабочем коде
- [ ] `SE_REPLICA_MODE`, `SE_WAL_DIR` удалены из конфигурации
- [ ] OpenAPI-спецификация обновлена
- [ ] `go test ./...` проходит без ошибок
- [ ] `go vet ./...` без предупреждений

---

## Phase 4: Периодическая синхронизация и обновление Helm charts

**Dependencies**: Phase 3
**Status**: Pending

### Описание

Реализация фоновых сервисов для синхронизации mode.json и index между pod-ами.
Обновление Helm charts для stateless режима.

### Подпункты

- [ ] **4.1 Периодическая синхронизация mode.json**
  - **Dependencies**: None
  - **Description**: Фоновый сервис для синхронизации режима между pod-ами.
    Все pod-ы периодически читают mode.json:
    - Создать `internal/service/modesync.go`:
      - `ModeSyncService` с интервалом из `SE_MODE_SYNC_INTERVAL`
      - Периодическое чтение mode.json через `modefile.LoadMode()`
      - Если режим в mode.json отличается от текущего → `sm.ForceMode()`
      - Логирование смены режима
      - Graceful stop через context cancellation
    - При смене режима через API → `modefile.SaveMode()` записывает mode.json →
      другие pod-ы подхватят на следующей итерации
  - **Creates**:
    - `internal/service/modesync.go`
    - `internal/service/modesync_test.go`
  - **Links**: N/A

- [ ] **4.2 Периодическая пересборка индекса**
  - **Dependencies**: None
  - **Description**: Периодическая полная пересборка in-memory индекса из attr.json
    для обнаружения файлов, записанных другими pod-ами:
    - Создать `internal/service/indexsync.go`:
      - `IndexSyncService` с интервалом из `SE_INDEX_SYNC_INTERVAL` (default 30s)
      - Вызывает `index.RebuildFromDir()` — полный scan attr.json файлов
      - Eventual consistency: собственные операции видны сразу,
        чужие — после rebuild (до 30s)
      - Логирование количества обнаруженных новых/удалённых файлов
      - Graceful stop через context cancellation
    - При rebuild проверять `.locks/` — файлы с активным lock пропускать
      (могут не иметь attr.json)
  - **Creates**:
    - `internal/service/indexsync.go`
    - `internal/service/indexsync_test.go`
  - **Links**: N/A

- [ ] **4.3 Интеграция sync-сервисов в main.go**
  - **Dependencies**: 4.1, 4.2
  - **Description**: Запуск ModeSyncService и IndexSyncService в `main.go`:
    - Оба сервиса запускаются **безусловно** на каждом pod-е
    - Добавить в graceful shutdown
    - Передать зависимости (modefile, index, lockManager)
  - **Creates**:
    - Обновлённый `cmd/storage-element/main.go`
  - **Links**: N/A

- [ ] **4.4 Обновление production Helm chart**
  - **Dependencies**: None
  - **Description**: Обновить `src/storage-element/charts/storage-element/`:
    - **Удалить**: `templates/statefulset.yaml`
    - **Удалить**: `templates/service-headless.yaml`
    - **Удалить**: `templates/pvc-shared.yaml` (shared PVC теперь единственный)
    - **Обновить** `templates/deployment.yaml`:
      - Убрать условие `standalone`/`replicated`
      - Поддержать `replicas > 1`
      - PVC с accessMode: ReadWriteMany (RWX) для shared storage
    - **Обновить** `templates/pvc.yaml`:
      - Единый data PVC (RWX)
      - Убрать отдельный WAL PVC (WAL удалён)
    - **Обновить** `values.yaml`:
      - Убрать `replicaMode`
      - Убрать `walSize`, `walStorageClass`
      - Добавить `uploadLockTTL`, `modeSyncInterval`, `indexSyncInterval`
    - **Обновить** env-переменные в deployment:
      - Убрать `SE_REPLICA_MODE`, `SE_ELECTION_RETRY_INTERVAL`, `SE_WAL_DIR`
      - Добавить `SE_UPLOAD_LOCK_TTL`, `SE_MODE_SYNC_INTERVAL`,
        `SE_INDEX_SYNC_INTERVAL`
  - **Creates**:
    - Обновлённый Helm chart
  - **Links**: N/A

- [ ] **4.5 Обновление тестового Helm chart**
  - **Dependencies**: 4.4
  - **Description**: Обновить `tests/helm/artstore-se/`:
    - Edit SE: `replicas: 2` (тестирование параллельной записи)
    - RW SE: `replicas: 1`
    - RO/AR SE: `replicas: 1`
    - Убрать StatefulSet-specific настройки
    - Обновить env-переменные
  - **Creates**:
    - Обновлённый тестовый chart
  - **Links**: N/A

- [ ] **4.6 Unit-тесты для sync-сервисов**
  - **Dependencies**: 4.1, 4.2
  - **Description**: Тесты для ModeSyncService и IndexSyncService:
    - ModeSyncService: обнаружение изменения mode.json, вызов ForceMode
    - IndexSyncService: обнаружение новых файлов, пропуск locked файлов
  - **Creates**:
    - Тесты в `internal/service/`
  - **Links**: N/A

### Критерии завершения Phase 4

- [ ] Все подпункты завершены (4.1 - 4.6)
- [ ] Mode.json синхронизируется между pod-ами без leader
- [ ] In-memory индекс периодически обновляется
- [ ] Helm chart поддерживает `replicas > 1` для edit/rw SE
- [ ] StatefulSet и headless Service удалены
- [ ] WAL PVC удалён из Helm chart
- [ ] `go test ./...` проходит без ошибок

---

## Phase 5: Сборка, интеграционные тесты, валидация

**Dependencies**: Phase 4
**Status**: Pending

### Описание

Сборка Docker-образа, развёртывание в тестовом K8s-кластере и проверка
корректности работы SE в stateless режиме с несколькими репликами.

### Подпункты

- [ ] **5.1 Обновление Dockerfile**
  - **Dependencies**: None
  - **Description**: Обновить Dockerfile если нужно:
    - Убрать WAL-related директории из VOLUME/mkdir
    - Убрать env defaults для удалённых параметров
    - Проверить multi-stage build
  - **Creates**:
    - Обновлённый `Dockerfile`
  - **Links**: N/A

- [ ] **5.2 Сборка Docker-образа**
  - **Dependencies**: 5.1
  - **Description**: Собрать Docker-образ SE с новой архитектурой.
    Тег: `v0.X.Y-N` (суффикс инкрементируется).
  - **Creates**:
    - Docker-образ в Harbor
  - **Links**: N/A

- [ ] **5.3 Развёртывание в тестовом кластере**
  - **Dependencies**: 5.2
  - **Description**: Развернуть SE в namespace `artstore-test`:
    - Edit SE с 2 репликами (проверка параллельной записи)
    - RW SE с 1 репликой
    - RO/AR SE с 1 репликой
    - Проверить запуск pod-ов, readiness/liveness probes
  - **Creates**:
    - Рабочее развёртывание в K8s
  - **Links**: N/A

- [ ] **5.4 Интеграционные тесты: параллельная запись**
  - **Dependencies**: 5.3
  - **Description**: Тесты для проверки корректности при параллельной записи
    с нескольких pod-ов:
    - Upload файла через pod-1, проверка доступности через pod-2
      (после index sync interval)
    - Одновременный upload через оба pod-а (разные файлы → разные UUID)
    - Delete через pod-1, проверка отсутствия через pod-2
    - Delete файла, который загружается → 409 Conflict
    - Mode transition через pod-1, проверка синхронизации на pod-2
  - **Creates**:
    - Интеграционные тесты в `tests/scripts/`
  - **Links**: N/A

- [ ] **5.5 Интеграционные тесты: Lock API**
  - **Dependencies**: 5.3
  - **Description**: Тесты Lock API:
    - `GET /api/v1/locks` — пустой список, список с lock-ами
    - `POST /api/v1/locks/cleanup` — очистка expired
    - `POST /api/v1/locks/cleanup?force=true` — принудительная очистка
    - Авторизация: проверить что endpoint-ы недоступны без RW прав
  - **Creates**:
    - Интеграционные тесты в `tests/scripts/`
  - **Links**: N/A

- [ ] **5.6 Интеграционные тесты: crash recovery**
  - **Dependencies**: 5.3
  - **Description**: Тесты crash recovery:
    - Имитация crash pod-а во время upload (kill pod-а)
    - Проверка что lock-файл остаётся
    - Ожидание TTL → проверка что GC подхватывает orphaned файл
    - Проверка `POST /api/v1/locks/cleanup` для ручной очистки
  - **Creates**:
    - Интеграционные тесты
  - **Links**: N/A

- [ ] **5.7 Регрессионные тесты**
  - **Dependencies**: 5.3
  - **Description**: Прогон существующих интеграционных тестов SE для проверки
    backward compatibility. Все текущие тесты upload/download/delete/mode/reconcile
    должны проходить.
  - **Creates**:
    - Результаты тестирования
  - **Links**: N/A

### Критерии завершения Phase 5

- [ ] Все подпункты завершены (5.1 - 5.7)
- [ ] Docker-образ собран и загружен в Harbor
- [ ] SE работает в K8s с `replicas > 1` (Edit SE)
- [ ] Параллельная запись корректна (нет потери данных)
- [ ] Lock API работает (авторизация, cleanup, force)
- [ ] Crash recovery через lock TTL + GC работает
- [ ] Все существующие интеграционные тесты проходят
- [ ] DELETE возвращает 409 при активном lock

---

## Phase 6: StorageBackend интерфейс (будущее)

**Dependencies**: Phase 5
**Status**: Not Started

### Описание

Выделение абстракции `StorageBackend` — интерфейса, через который SE работает
с хранилищем. Текущая реализация (`filestore`, `attr`, `index`, `lockfile`)
оборачивается в `LocalFSBackend`. В будущем добавляется `S3Backend` (Phase 7).

Цель — разделить бизнес-логику SE от конкретной реализации хранилища.

> **Примечание**: Эта фаза может быть отложена до момента, когда реально
> понадобится S3. Phases 1-5 дают рабочий stateless SE без абстракции backend.

### Предлагаемый интерфейс

```go
type StorageBackend interface {
    Store(ctx context.Context, params StoreParams) (*StoreResult, error)
    Get(ctx context.Context, fileID string) (io.ReadCloser, *FileInfo, error)
    Delete(ctx context.Context, fileID string) error
    Stat(ctx context.Context, fileID string) (*FileInfo, error)
    List(ctx context.Context, opts ListOptions) ([]FileInfo, error)
    Available(ctx context.Context) (int64, error)
    RunGC(ctx context.Context) (*GCResult, error)
    RunReconcile(ctx context.Context) (*ReconcileResult, error)
    AcquireLock(ctx context.Context, fileID string) error
    ReleaseLock(ctx context.Context, fileID string) error
    IsLocked(ctx context.Context, fileID string) (bool, error)
    ListLocks(ctx context.Context) ([]LockInfo, error)
    CleanupLocks(ctx context.Context, force bool) (*CleanupResult, error)
}
```

### Подпункты

- [ ] **6.1 Определение интерфейса `StorageBackend`**
  - **Dependencies**: None
  - **Description**: Создать пакет `internal/backend/` с интерфейсом и типами.
  - **Creates**:
    - `internal/backend/backend.go`
  - **Links**: N/A

- [ ] **6.2 Реализация `LocalFSBackend`**
  - **Dependencies**: 6.1
  - **Description**: Обернуть `filestore`, `attr`, `index`, `lockfile`
    в реализацию `LocalFSBackend`.
  - **Creates**:
    - `internal/backend/localfs/localfs.go`
    - `internal/backend/localfs/localfs_test.go`
  - **Links**: N/A

- [ ] **6.3 Рефакторинг сервисного слоя**
  - **Dependencies**: 6.2
  - **Description**: Обновить сервисы для работы через `StorageBackend`.
  - **Creates**:
    - Обновлённые сервисы
  - **Links**: N/A

- [ ] **6.4 Фабрика backend-ов и тесты**
  - **Dependencies**: 6.2, 6.3
  - **Description**: Фабрика по `SE_STORAGE_BACKEND` + unit-тесты.
  - **Creates**:
    - `internal/backend/factory.go`
    - Тесты
  - **Links**: N/A

### Критерии завершения Phase 6

- [ ] Все подпункты завершены (6.1 - 6.4)
- [ ] `StorageBackend` интерфейс определён и реализован для LocalFS
- [ ] Сервисный слой работает через `StorageBackend`
- [ ] `go test ./...` проходит без ошибок

---

## Phase 7: S3 Backend (будущее)

**Dependencies**: Phase 6
**Status**: Not Started

### Описание

Реализация S3-совместимого backend-а. S3 обеспечивает атомарность на уровне
`PutObject`, поэтому lock-файлы и WAL не нужны. Метаданные хранятся в object
tags или sidecar-объектах.

Эта фаза является перспективной и будет детализирована при необходимости.

### Подпункты

- [ ] **7.1 Проектирование S3Backend**
  - **Dependencies**: None
  - **Description**: Дизайн: структура bucket-а, хранение метаданных,
    conditional writes, GC через lifecycle rules.
  - **Creates**:
    - Дизайн-документ
  - **Links**: N/A

- [ ] **7.2 Реализация и тестирование S3Backend**
  - **Dependencies**: 7.1
  - **Description**: Реализация `StorageBackend` для S3 + тесты с MinIO.
  - **Creates**:
    - `internal/backend/s3/s3.go`
    - `internal/backend/s3/s3_test.go`
  - **Links**: N/A

- [ ] **7.3 Конфигурация S3**
  - **Dependencies**: 7.2
  - **Description**: Env-переменные: `SE_S3_ENDPOINT`, `SE_S3_BUCKET`,
    `SE_S3_REGION`, `SE_S3_ACCESS_KEY`, `SE_S3_SECRET_KEY`, `SE_S3_USE_PATH_STYLE`.
  - **Creates**:
    - Обновлённый `internal/config/config.go`
  - **Links**: N/A

### Критерии завершения Phase 7

- [ ] S3Backend реализован и протестирован (MinIO)
- [ ] SE работает с S3 backend
- [ ] API backward compatible

---

## Влияние на другие модули

### Backward Compatibility

| Компонент | Влияние |
|-----------|---------|
| **SE HTTP API** | Минимальные изменения: +2 endpoint-а (locks), +409 на DELETE, убраны поля из /info |
| **Admin Module** | Без изменений. AM регистрирует SE по URL, внутренняя архитектура SE для AM прозрачна. |
| **Ingester Module** | Без изменений. IM обращается к SE через K8s Service (load balancing). |
| **Query Module** | Без изменений. QM скачивает файлы через SE API. |

### Удаляемые компоненты

| Компонент | Строк кода | Причина удаления |
|-----------|-----------|-----------------|
| `internal/replica/` | ~1100 | Leader/follower заменён на stateless |
| `internal/storage/wal/` | ~300 | WAL заменён на per-file lock + atomic writes |
| StatefulSet (Helm) | ~100 | Заменён на Deployment с replicas |
| Headless Service (Helm) | ~30 | Не нужен без StatefulSet |
| WAL PVC (Helm) | ~20 | WAL удалён |

### Новые компоненты

| Компонент | Назначение |
|-----------|-----------|
| `internal/lockfile/` | Per-file lock с TTL |
| `internal/modefile/` | Чтение/запись mode.json |
| `internal/service/modesync.go` | Периодическая синхронизация mode.json |
| `internal/service/indexsync.go` | Периодическая пересборка индекса |
| Lock API handlers | GET /locks, POST /locks/cleanup |

### Производительность

- **Положительное**: Нет proxy overhead при записи. Нативный K8s load balancing.
- **NFS latency**: Lock-файлы на NFS — минимальная латенсия (создание/удаление
  маленького JSON).
- **Index eventual consistency**: Индекс одного pod-а может отставать от реального
  состояния NFS. Периодическая пересборка (30s) — компромисс.

---

## Риски и митигации

| Риск | Вероятность | Влияние | Митигация |
|------|-------------|---------|-----------|
| Lock-файл не удалён при crash | Средняя | Низкое | TTL auto-expire + GC cleanup + API cleanup |
| GC удаляет файл во время upload | Низкая | Высокое | Per-file lock: GC проверяет .locks/ перед удалением |
| Index desync между pod-ами | Средняя | Среднее | Периодическая пересборка (30s); собственные операции видны сразу |
| Параллельный delete + upload одного файла | Низкая | Среднее | DELETE → 409 Conflict при активном lock |
| NFS недоступен — lock-файлы зависнут | Низкая | Среднее | TTL-based expiry, ручная очистка через API |
| Orphaned файлы после crash | Средняя | Низкое | GC + Reconcile обнаружат и удалят после TTL |
| Два GC одновременно удаляют один файл | Средняя | Нет | os.Remove() идемпотентен |

---

## Примечания

- Перед началом Phase 1 создать ветку `refactor/se-stateless` от `main`.
- Каждая фаза — один контекст AI (одна сессия разработки).
- Phases 6-7 (StorageBackend, S3) могут быть отложены — Phases 1-5 дают
  полностью рабочий stateless SE.
- Версионирование: minor bump (`0.Y.0`), т.к. архитектурное изменение.
- Текущий API-контракт (`docs/api-contracts/storage-element-openapi.yaml`)
  обновляется в Phase 3.6.
