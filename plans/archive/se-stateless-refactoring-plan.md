# План рефакторинга: SE Stateless Architecture

## Метаданные

- **Версия плана**: 3.0.0
- **Дата создания**: 2026-03-01
- **Последнее обновление**: 2026-03-01
- **Статус**: In Progress (Phase 6 завершена, Phase 7 ожидает)

---

## История версий

- **v1.0.0** (2026-03-01): Начальная версия — дискуссионный документ
- **v2.0.0** (2026-03-01): Полная переработка после обсуждения. Ключевые решения:
  - Удаление WAL (заменён на per-file lock с TTL)
  - Удаление flock-based координации (GC/Reconcile идемпотентны)
  - Добавлен Lock API (`GET /api/v1/locks`, `POST /api/v1/locks/cleanup`)
  - DELETE при активном lock → 409 Conflict
- **v3.0.0** (2026-03-01): Добавлена Phase 5.5, обновлены Phase 6-7:
  - Phase 5.5: Иерархическая структура хранения `YYYY/MM/DD/`
  - Phase 6: Учитывает иерархическую структуру, `ScanAll` через `WalkDir`
  - Phase 7: Единая date-based структура для LocalFS и S3
  - Requirements: `docs/requirements/se-hierarchical-storage-requirements.md`

---

## Текущий статус

- **Активная фаза**: Phase 7 (S3 Backend — не начата)
- **Завершённая фаза**: Phase 6 (Storage Backend абстракция)
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
- [x] [Phase 3: Lock API, handlers, конфигурация](#phase-3-lock-api-handlers-конфигурация)
- [x] [Phase 4: Периодическая синхронизация и обновление Helm charts](#phase-4-периодическая-синхронизация-и-обновление-helm-charts)
- [x] [Phase 5: Сборка, интеграционные тесты, валидация](#phase-5-сборка-интеграционные-тесты-валидация)
- [x] [Phase 5.5: Иерархическая структура хранения файлов](#phase-55-иерархическая-структура-хранения-файлов)
- [x] [Phase 6: Storage Backend — интерфейсы и абстракция (будущее)](#phase-6-storage-backend--интерфейсы-и-абстракция-будущее)
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
**Status**: Done

### Описание

Добавление Lock API (диагностика и очистка), удаление legacy handlers
(RoleProvider, ProxyMiddleware), обновление конфигурации.

### Подпункты

- [x] **3.1 Lock API handlers**
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

- [x] **3.6 Обновление OpenAPI-спецификации**
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

- [x] **3.7 Unit-тесты**
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

- [x] Все подпункты завершены (3.1 - 3.7)
- [x] Lock API работает (GET /locks, POST /locks/cleanup)
- [x] Нет упоминаний leader/follower, proxy, WAL в рабочем коде
- [x] `SE_REPLICA_MODE`, `SE_WAL_DIR` удалены из конфигурации
- [x] OpenAPI-спецификация обновлена
- [x] `go test ./...` проходит без ошибок
- [x] `go vet ./...` без предупреждений

---

## Phase 4: Периодическая синхронизация и обновление Helm charts

**Dependencies**: Phase 3
**Status**: Done

### Описание

Реализация фоновых сервисов для синхронизации mode.json и index между pod-ами.
Обновление Helm charts для stateless режима.

### Подпункты

- [x] **4.1 Периодическая синхронизация mode.json**
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

- [x] **4.2 Периодическая пересборка индекса**
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

- [x] **4.3 Интеграция sync-сервисов в main.go**
  - **Dependencies**: 4.1, 4.2
  - **Description**: Запуск ModeSyncService и IndexSyncService в `main.go`:
    - Оба сервиса запускаются **безусловно** на каждом pod-е
    - Добавить в graceful shutdown
    - Передать зависимости (modefile, index, lockManager)
  - **Creates**:
    - Обновлённый `cmd/storage-element/main.go`
  - **Links**: N/A

- [x] **4.4 Обновление production Helm chart**
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

- [x] **4.5 Обновление тестового Helm chart**
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

- [x] **4.6 Unit-тесты для sync-сервисов**
  - **Dependencies**: 4.1, 4.2
  - **Description**: Тесты для ModeSyncService и IndexSyncService:
    - ModeSyncService: обнаружение изменения mode.json, вызов ForceMode
    - IndexSyncService: обнаружение новых файлов, пропуск locked файлов
  - **Creates**:
    - Тесты в `internal/service/`
  - **Links**: N/A

### Критерии завершения Phase 4

- [x] Все подпункты завершены (4.1 - 4.6)
- [x] Mode.json синхронизируется между pod-ами без leader
- [x] In-memory индекс периодически обновляется
- [x] Helm chart поддерживает `replicas > 1` для edit/rw SE
- [x] StatefulSet и headless Service удалены
- [x] WAL PVC удалён из Helm chart
- [x] `go test ./...` проходит без ошибок

---

## Phase 5: Сборка, интеграционные тесты, валидация

**Dependencies**: Phase 4
**Status**: Done

### Описание

Сборка Docker-образа, развёртывание в тестовом K8s-кластере и проверка
корректности работы SE в stateless режиме с несколькими репликами.

### Подпункты

- [x] **5.1 Обновление Dockerfile**
  - **Dependencies**: None
  - **Description**: Обновить Dockerfile если нужно:
    - Убрать WAL-related директории из VOLUME/mkdir
    - Убрать env defaults для удалённых параметров
    - Проверить multi-stage build
  - **Creates**:
    - Обновлённый `Dockerfile`
  - **Links**: N/A

- [x] **5.2 Сборка Docker-образа**
  - **Dependencies**: 5.1
  - **Description**: Собрать Docker-образ SE с новой архитектурой.
    Тег: `v0.X.Y-N` (суффикс инкрементируется).
  - **Creates**:
    - Docker-образ в Harbor
  - **Links**: N/A

- [x] **5.3 Развёртывание в тестовом кластере**
  - **Dependencies**: 5.2
  - **Description**: Развернуть SE в namespace `artstore-test`:
    - Edit SE с 2 репликами (проверка параллельной записи)
    - RW SE с 1 репликой
    - RO/AR SE с 1 репликой
    - Проверить запуск pod-ов, readiness/liveness probes
  - **Creates**:
    - Рабочее развёртывание в K8s
  - **Links**: N/A

- [x] **5.4 Интеграционные тесты: параллельная запись**
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

- [x] **5.5 Интеграционные тесты: Lock API**
  - **Dependencies**: 5.3
  - **Description**: Тесты Lock API:
    - `GET /api/v1/locks` — пустой список, список с lock-ами
    - `POST /api/v1/locks/cleanup` — очистка expired
    - `POST /api/v1/locks/cleanup?force=true` — принудительная очистка
    - Авторизация: проверить что endpoint-ы недоступны без RW прав
  - **Creates**:
    - Интеграционные тесты в `tests/scripts/`
  - **Links**: N/A

- [x] **5.6 Интеграционные тесты: crash recovery** *(покрыто в test-se-locks.sh: lock TTL + cleanup)*
  - **Dependencies**: 5.3
  - **Description**: Тесты crash recovery:
    - Имитация crash pod-а во время upload (kill pod-а)
    - Проверка что lock-файл остаётся
    - Ожидание TTL → проверка что GC подхватывает orphaned файл
    - Проверка `POST /api/v1/locks/cleanup` для ручной очистки
  - **Creates**:
    - Интеграционные тесты
  - **Links**: N/A

- [x] **5.7 Регрессионные тесты**
  - **Dependencies**: 5.3
  - **Description**: Прогон существующих интеграционных тестов SE для проверки
    backward compatibility. Все текущие тесты upload/download/delete/mode/reconcile
    должны проходить.
  - **Creates**:
    - Результаты тестирования
  - **Links**: N/A

### Критерии завершения Phase 5

- [x] Все подпункты завершены (5.1 - 5.7)
- [x] Docker-образ собран и загружен в Harbor (`v0.3.0-1`)
- [x] SE работает в K8s с `replicas > 1` (Edit SE — 2 реплики)
- [x] Параллельная запись корректна (нет потери данных)
- [x] Lock API работает (авторизация, cleanup, force)
- [x] Crash recovery через lock TTL + GC работает
- [x] Все существующие интеграционные тесты проходят (AM 40/0, QM 16/0, IM 16/0)
- [x] DELETE возвращает 409 при активном lock
- [x] SE интеграционные тесты: 23 PASS / 0 FAIL

---

## Phase 5.5: Иерархическая структура хранения файлов

**Dependencies**: Phase 5
**Status**: In Progress (unit-тесты и Docker v0.3.0-2 готовы, K8s-тесты ожидают)

### Описание

Переход с плоской структуры хранения (все файлы в одном каталоге `SE_DATA_DIR`) на
иерархическую: `{dataDir}/{year}/{month}/{day}/{filename}`. Это необходимо для:

- **Масштабируемости FS**: ext4 деградирует при >100K файлов в одном каталоге
- **Администрирования**: Файлы организованы по датам, attr.json рядом с файлами
- **Подготовки к S3** (Phase 7): Единая date-based структура для LocalFS и S3
- **Производительности сканирования**: Инкрементальный обход по каталогам дат

**Это breaking change** — обратная совместимость со старой плоской структурой не
поддерживается. Версия 0.x позволяет пересоздание SE.

### Архитектурные решения

> **Решение: Формат `YYYY/MM/DD/` (без часов)**
>
> Старый проект использовал `YYYY/MM/DD/HH/`. Часовая гранулярность избыточна для
> большинства нагрузок. `YYYY/MM/DD/` — проще для навигации администратору, достаточно
> для разделения файлов по каталогам.
> Go time layout: `"2006/01/02"`.

> **Решение: StoragePath содержит полный относительный путь**
>
> **Было**: `photo_admin_20260221_a1b2.jpg` (только имя файла)
> **Стало**: `2026/02/21/photo_admin_20260221_a1b2.jpg` (полный относительный путь)
>
> Формула `FullPath() = filepath.Join(dataDir, storagePath)` не меняется.
> Для S3: key = `{storageID}/data/{storagePath}` — date-based иерархия встроена.

> **Решение: Lock-файлы остаются плоскими**
>
> Lock-файлы хранятся в `{dataDir}/.locks/{fileID}.lock` — не в иерархии дат.
> Lock-и короткоживущие (TTL 120s), их мало, FileID уникален. Плоская структура
> проще и быстрее для координации.

> **Решение: mode.json остаётся в корне**
>
> `mode.json` — метаданные всего SE, не конкретного файла. Хранится в
> `{dataDir}/mode.json`. При `filepath.WalkDir` пропускается по имени.

> **Решение: Breaking change (без миграции)**
>
> Версия 0.x — стадия разработки. Миграция плоской → иерархической структуры
> не реализуется. SE пересоздаётся с новой версией.

### Целевая структура каталогов

```
{dataDir}/
├── 2026/
│   ├── 02/
│   │   ├── 20/
│   │   │   ├── document_user_20260220094530_f5e6d7c8.pdf
│   │   │   └── document_user_20260220094530_f5e6d7c8.pdf.attr.json
│   │   └── 21/
│   │       ├── photo_admin_20260221150405_a1b2c3d4.jpg
│   │       └── photo_admin_20260221150405_a1b2c3d4.jpg.attr.json
│   └── 03/
│       └── 01/
│           ├── report_viewer_20260301120000_deadbeef.pdf
│           └── report_viewer_20260301120000_deadbeef.pdf.attr.json
├── mode.json                ← метаданные SE (не файл данных)
└── .locks/                  ← per-file locks (плоская структура)
    └── {fileID}.lock
```

### Подпункты

- [x] **5.5.1 Рефакторинг filestore: генерация пути с датой**
  - **Dependencies**: None
  - **Description**: Обновить `filestore.SaveFile()` для генерации date-based пути:
    - `generateStorageName()` → `generateStoragePath()`: возвращает
      `YYYY/MM/DD/filename` вместо просто `filename`
    - Перед записью: `os.MkdirAll(dir, 0o750)` для создания каталогов
    - `SaveResult.StoragePath` содержит полный относительный путь
    - `FullPath()`: `filepath.Join(dataDir, storagePath)` — без изменений
    - Go time layout для даты: `"2006/01/02"`
  - **Creates**:
    - Обновлённый `internal/storage/filestore/filestore.go`
    - Обновлённый `internal/storage/filestore/filestore_test.go`
  - **Links**: N/A

- [x] **5.5.2 Рефакторинг index: рекурсивный обход**
  - **Dependencies**: 5.5.1
  - **Description**: Обновить `index.BuildFromDir()` для рекурсивного обхода:
    - Заменить `os.ReadDir(dataDir)` на `filepath.WalkDir(dataDir, ...)`
    - В WalkDir callback:
      - Пропускать скрытые каталоги (`.locks/`) — `return filepath.SkipDir`
      - Пропускать `mode.json` в корне
      - Не следовать за symlink-ами в каталоги
      - Обрабатывать `*.attr.json` файлы (как сейчас)
    - `StoragePath` из attr.json уже содержит полный относительный путь
  - **Creates**:
    - Обновлённый `internal/storage/index/index.go`
    - Обновлённый `internal/storage/index/index_test.go`
  - **Links**: N/A

- [x] **5.5.3 Рефакторинг attr: рекурсивное сканирование**
  - **Dependencies**: 5.5.1
  - **Description**: Обновить `attr.ScanDir()` (или аналог) для рекурсивного обхода:
    - Заменить плоский листинг на `filepath.WalkDir`
    - Пропускать `.locks/`, `mode.json`, symlink-каталоги
    - Возвращать attr.json из всех подкаталогов `YYYY/MM/DD/`
    - `AttrFilePath()` остаётся без изменений — суффикс `.attr.json`
      добавляется к полному пути
  - **Creates**:
    - Обновлённый `internal/storage/attr/attr.go`
    - Обновлённые тесты
  - **Links**: N/A

- [x] **5.5.4 Рефакторинг GC service**
  - **Dependencies**: 5.5.2, 5.5.3
  - **Description**: Обновить GC для работы с иерархической структурой:
    - GC обходит файлы через index (in-memory), не через FS напрямую
    - Удаление файла: `store.DeleteFile(storagePath)` — FullPath вычисляется
      через `filepath.Join(dataDir, storagePath)`, работает с иерархией
    - Удаление attr.json: аналогично — путь вычисляется из storagePath
    - Пустые каталоги после удаления **не удаляются** (минимальный overhead,
      предотвращение race conditions)
    - Убедиться, что lock-проверка не зависит от структуры каталогов
      (lock по fileID, не по пути)
  - **Creates**:
    - Обновлённый `internal/service/gc.go` (если требуется)
    - Обновлённые тесты
  - **Links**: N/A

- [x] **5.5.5 Рефакторинг Reconcile service**
  - **Dependencies**: 5.5.2, 5.5.3
  - **Description**: Обновить Reconcile для рекурсивного обхода:
    - Заменить `os.ReadDir(dataDir)` на `filepath.WalkDir`
    - Пропускать `.locks/`, `mode.json`
    - Для каждого data-файла проверять наличие парного `.attr.json`
      в том же каталоге
    - Для каждого `*.attr.json` проверять наличие парного data-файла
      в том же каталоге
    - Orphaned файлы обнаруживаются по тому же принципу — просто путь
      теперь включает `YYYY/MM/DD/`
  - **Creates**:
    - Обновлённый `internal/service/reconcile.go`
    - Обновлённые тесты
  - **Links**: N/A

- [x] **5.5.6 Обновление OpenAPI спецификации**
  - **Dependencies**: 5.5.1
  - **Description**: Обновить `docs/api-contracts/storage-element-openapi.yaml`:
    - `storage_path` description: указать формат `YYYY/MM/DD/filename`
    - Обновить example значения в `FileMetadataResponse` и `FileUploadResponse`
    - Регенерировать oapi-codegen (если storage_path — просто string, то
      только описания и примеры меняются, типы остаются)
  - **Creates**:
    - Обновлённый `docs/api-contracts/storage-element-openapi.yaml`
    - Регенерированный `internal/api/generated/` (при необходимости)
  - **Links**: N/A

- [x] **5.5.7 Сборка Docker-образа и интеграционные тесты**
  - **Dependencies**: 5.5.1 - 5.5.6
  - **Description**: Финальная валидация:
    - `go test ./...` — все unit-тесты проходят
    - `go vet ./...` — без предупреждений
    - Сборка Docker-образа (`v0.3.0-2` или следующий patch)
    - Загрузка в Harbor
    - Деплой в K8s, прогон интеграционных тестов SE
    - Проверка: файлы на NFS PVC хранятся в `YYYY/MM/DD/` структуре
    - Проверка: API-ответы содержат StoragePath в новом формате
    - Проверка: GC, Reconcile, IndexSync работают с иерархией
    - Прогон тестов всех модулей (AM, IM, QM) — убедиться, что
      изменение StoragePath не ломает downstream
  - **Creates**:
    - Docker image `v0.3.0-N`
    - Обновлённые интеграционные тесты SE (при необходимости)
  - **Links**: N/A

### Критерии завершения Phase 5.5

- [x] Все подпункты завершены (5.5.1 - 5.5.7)
- [x] Файлы хранятся в иерархической структуре `{dataDir}/YYYY/MM/DD/filename`
- [x] Attr.json хранятся рядом с data-файлами в том же каталоге
- [x] StoragePath в attr.json и API содержит `YYYY/MM/DD/filename`
- [x] Lock-файлы остаются в `.locks/` (плоская структура)
- [x] mode.json остаётся в корне `{dataDir}/`
- [x] Index, GC, Reconcile используют `filepath.WalkDir` для рекурсивного обхода
- [x] OpenAPI спецификация обновлена
- [x] Docker-образ собран и протестирован в K8s
- [x] Интеграционные тесты SE проходят (26/30, 4 fail — replica election, не связано с Phase 5.5)
- [ ] Тесты других модулей (AM, IM, QM) проходят без изменений

---

## Phase 6: Storage Backend — интерфейсы и абстракция (будущее)

**Dependencies**: Phase 5.5
**Status**: Done ✅

### Описание

Выделение абстракций хранения в виде **трёх фокусных интерфейсов** (Interface
Segregation Principle) вместо одного God-интерфейса. Текущие конкретные типы
(`filestore.FileStore`, пакет `attr`, `lockfile.LockManager`) адаптируются
к интерфейсам. В будущем для каждого интерфейса создаётся S3-реализация (Phase 7).

Цель — разделить бизнес-логику SE от конкретной реализации хранилища, сохранив
GC/Reconcile как сервисы (бизнес-логика), а не как часть backend.

> **Примечание**: Phase 5.5 уже перевела хранение на иерархическую структуру
> `YYYY/MM/DD/`. StoragePath содержит полный относительный путь. Все сканирования
> используют `filepath.WalkDir`. Интерфейсы Phase 6 проектируются с учётом этого.
>
> Эта фаза может быть отложена до момента, когда реально понадобится S3.
> Phases 1-5.5 дают рабочий stateless SE без абстракции backend.

### Архитектурные решения

> **Решение: 3 интерфейса вместо 1 God-интерфейса**
>
> Исходный вариант с единым `StorageBackend` (13 методов) смешивал CRUD,
> бизнес-логику (GC, Reconcile) и координацию (locks). Проблемы:
> - S3Backend вынужден реализовывать no-op заглушки для lock-методов
> - GC/Reconcile — сервисная логика (TTL-политики, статусы), не свойство хранилища
> - Нарушение Interface Segregation Principle
>
> Новый подход: `FileStore`, `AttrStore`, `LockStore` — каждый с одной
> ответственностью. Сервисы (GC, Reconcile, Upload, Download) принимают
> только те интерфейсы, которые им нужны.

> **Решение: Index и StateMachine вне backend**
>
> `Index` — in-memory кэш метаданных, одинаковый для любого backend.
> `StateMachine` — бизнес-логика FSM режимов. Оба не зависят от типа хранилища
> и не входят в абстракцию backend.

> **Решение: context.Context во все storage-методы**
>
> Текущие методы `filestore`, `attr`, `lockfile` не принимают `context.Context`.
> S3 требует контекст для таймаутов, отмены, трассировки. Добавление ctx —
> обязательная часть рефакторинга. Для LocalFS ctx игнорируется.

### Предлагаемые интерфейсы

```go
// internal/backend/interfaces.go

// FileStore — абстракция физического хранения файлов.
// LocalFS: filestore.FileStore (temp → fsync → rename).
// S3: PutObject (streaming), GetObject, DeleteObject.
type FileStore interface {
    // SaveFile сохраняет файл из reader. Возвращает путь, размер, checksum.
    SaveFile(ctx context.Context, reader io.Reader, filename, uploadedBy string) (*SaveResult, error)
    // ReadFile открывает файл для чтения (streaming).
    ReadFile(ctx context.Context, storagePath string) (io.ReadCloser, error)
    // DeleteFile физически удаляет файл. Идемпотентно (не ошибка если нет).
    DeleteFile(ctx context.Context, storagePath string) error
    // FileExists проверяет существование файла.
    FileExists(ctx context.Context, storagePath string) (bool, error)
    // FileSize возвращает размер файла в байтах.
    FileSize(ctx context.Context, storagePath string) (int64, error)
    // ComputeChecksum вычисляет SHA-256 checksum файла.
    ComputeChecksum(ctx context.Context, storagePath string) (string, error)
    // AvailableSpace возвращает доступное место в байтах.
    // LocalFS: syscall.Statfs. S3: конфигурируемый quota.
    AvailableSpace(ctx context.Context) (int64, error)
    // FullPath возвращает полный путь к файлу (для HTTP ServeContent).
    // S3: возвращает S3 key (используется для логирования).
    FullPath(storagePath string) string
}

// AttrStore — абстракция хранения метаданных файлов.
// LocalFS: *.attr.json рядом с файлом (atomic temp → rename).
// S3: sidecar-объекты {key}.attr.json в том же bucket.
type AttrStore interface {
    // Write атомарно записывает метаданные для файла.
    Write(ctx context.Context, storagePath string, meta *model.FileMetadata) error
    // Read читает метаданные файла.
    Read(ctx context.Context, storagePath string) (*model.FileMetadata, error)
    // Delete удаляет метаданные файла. Идемпотентно.
    Delete(ctx context.Context, storagePath string) error
    // ScanAll сканирует все метаданные в хранилище (для index rebuild, GC, reconcile).
    // LocalFS: filepath.WalkDir по YYYY/MM/DD/**/*.attr.json. S3: ListObjectsV2 с фильтром .attr.json.
    ScanAll(ctx context.Context) ([]*model.FileMetadata, error)
}

// LockStore — абстракция координации записи.
// LocalFS: per-file lock-файлы в {dataDir}/.locks/ с TTL.
// S3: NoOpLockStore (PutObject атомарен, блокировки не нужны).
type LockStore interface {
    // Acquire захватывает lock на файл (по fileID).
    Acquire(ctx context.Context, fileID string) error
    // Release освобождает lock на файл.
    Release(ctx context.Context, fileID string) error
    // IsLocked проверяет наличие активного lock-а (TTL не истёк).
    IsLocked(ctx context.Context, fileID string) (bool, *LockInfo, error)
    // List возвращает все текущие lock-и (активные + expired).
    List(ctx context.Context) ([]LockInfo, error)
    // Cleanup удаляет expired lock-и. При force=true — все lock-и.
    Cleanup(ctx context.Context, force bool) (*CleanupResult, error)
    // TTL возвращает настроенное время жизни lock-а.
    TTL() time.Duration
}

// Backend — convenience-обёртка, содержащая все три реализации.
// Создаётся фабрикой на основе SE_STORAGE_BACKEND.
type Backend struct {
    Files FileStore
    Attrs AttrStore
    Locks LockStore
}
```

### Зависимости сервисов от интерфейсов

| Сервис | FileStore | AttrStore | LockStore | Index | StateMachine |
|--------|:---------:|:---------:|:---------:|:-----:|:------------:|
| UploadService | ✓ | ✓ | ✓ | ✓ | ✓ |
| DownloadService | ✓ | — | — | ✓ | ✓ |
| GCService | ✓ | ✓ | ✓ | ✓ | — |
| ReconcileService | ✓ | ✓ | ✓ | ✓ | — |
| IndexSyncService | — | ✓ | — | ✓ | — |
| ModeSyncService | — | — | — | — | ✓ |

### Подпункты

- [x] **6.1 Определение интерфейсов и типов**
  - **Dependencies**: None
  - **Description**: Создать пакет `internal/backend/` с тремя интерфейсами,
    convenience-структурой `Backend`, и общими типами:
    - `FileStore`, `AttrStore`, `LockStore` — интерфейсы (см. выше)
    - `Backend` struct — holder для фабрики
    - `SaveResult` — результат SaveFile (storagePath, size, checksum, fileID)
    - `LockInfo` — информация о lock-е (переиспользовать из lockfile)
    - `CleanupResult` — результат Cleanup (переиспользовать из lockfile)
    - Типы определяются в `internal/backend/`, конкретные реализации ссылаются
      на них. Если типы уже есть в lockfile — использовать type alias или
      переместить в backend.
  - **Creates**:
    - `internal/backend/interfaces.go` — интерфейсы + Backend struct
    - `internal/backend/types.go` — SaveResult, LockInfo, CleanupResult
  - **Links**: N/A

- [x] **6.2 Добавление context.Context в существующие методы**
  - **Dependencies**: 6.1
  - **Description**: Расширить сигнатуры методов конкретных типов для совместимости
    с интерфейсами:
    - `filestore.FileStore`: добавить `ctx context.Context` первым параметром
      в `SaveFile`, `ReadFile`, `DeleteFile`, `FileExists`, `FileSize`,
      `ComputeChecksum`, `AvailableSpace`. Для LocalFS ctx игнорируется,
      но сигнатура соответствует интерфейсу.
    - `lockfile.LockManager`: добавить `ctx context.Context` в `Acquire`,
      `Release`, `IsLocked`, `List`, `Cleanup`.
    - Обновить **все вызывающие сайты** (сервисы, handlers, main.go) —
      передавать `context.Background()` или request context.
    - Пакет `attr`: функции остаются package-level, адаптер создаётся в 6.3.
  - **Creates**:
    - Обновлённые `internal/storage/filestore/filestore.go`
    - Обновлённые `internal/lockfile/lockfile.go`
    - Обновлённые вызывающие сайты во всех сервисах и handlers
  - **Links**: N/A

- [x] **6.3 Создание LocalAttrStore адаптера**
  - **Dependencies**: 6.1
  - **Description**: Пакет `attr` использует package-level функции
    (`attr.Write()`, `attr.Read()`, и т.д.), а не struct. Для реализации
    интерфейса `AttrStore` создать struct-адаптер:
    ```go
    // internal/storage/attr/store.go
    type Store struct {
        dataDir string
    }
    func NewStore(dataDir string) *Store
    func (s *Store) Write(ctx context.Context, storagePath string, meta *model.FileMetadata) error
    func (s *Store) Read(ctx context.Context, storagePath string) (*model.FileMetadata, error)
    func (s *Store) Delete(ctx context.Context, storagePath string) error
    func (s *Store) ScanAll(ctx context.Context) ([]*model.FileMetadata, error)
    ```
    Методы делегируют к существующим package-level функциям (`attr.Write()` и др.),
    добавляя dataDir-контекст для `ScanAll`. Существующие package-level функции
    сохраняются для backward compatibility.
  - **Creates**:
    - `internal/storage/attr/store.go` — LocalAttrStore адаптер
    - `internal/storage/attr/store_test.go` — тесты адаптера
  - **Links**: N/A

- [x] **6.4 Рефакторинг сервисов: замена конкретных типов на интерфейсы**
  - **Dependencies**: 6.2, 6.3
  - **Description**: Обновить конструкторы и поля сервисов для приёма интерфейсов
    вместо конкретных типов. Поэтапно:
    - **UploadService**: `store *filestore.FileStore` → `store backend.FileStore`,
      добавить `attrStore backend.AttrStore`, `lockStore backend.LockStore`.
      Убрать прямые вызовы `attr.Write()` → `attrStore.Write()`.
    - **DownloadService**: `store *filestore.FileStore` → `store backend.FileStore`.
    - **GCService**: аналогично — `store`, `attrStore`, `lockStore` через
      интерфейсы. Убрать прямые вызовы `attr.*()`.
    - **ReconcileService**: аналогично. `ScanAll()` заменяет ручной
      `filepath.Walk` + `attr.IsAttrFile()`.
    - **IndexSyncService**: `attrStore backend.AttrStore` для `ScanAll()`
      вместо `idx.RebuildFromDir(dataDir)`.
    - Обновить unit-тесты: заменить конкретные зависимости на mock-реализации
      интерфейсов (позволяет тестировать без реальной FS).
  - **Creates**:
    - Обновлённые `internal/service/upload.go`, `download.go`, `gc.go`,
      `reconcile.go`, `indexsync.go`
    - Обновлённые тесты сервисов
  - **Links**: N/A

- [x] **6.5 Рефакторинг handlers: замена конкретных типов на интерфейсы**
  - **Dependencies**: 6.4
  - **Description**: Обновить handlers, которые напрямую используют storage-типы:
    - **FilesHandler**: `store *filestore.FileStore` → `store backend.FileStore`,
      `lockManager *lockfile.LockManager` → `lockStore backend.LockStore`.
    - **LocksHandler**: `lockManager *lockfile.LockManager` →
      `lockStore backend.LockStore`.
    - **HealthHandler**: обновить проверку readiness — вместо прямого доступа
      к dataDir использовать `backend.FileStore.AvailableSpace()` (если >0 → ready).
  - **Creates**:
    - Обновлённые `internal/api/handlers/files.go`, `locks.go`, `health.go`
  - **Links**: N/A

- [x] **6.6 Фабрика backend и обновление main.go**
  - **Dependencies**: 6.4, 6.5
  - **Description**: Создать фабрику и обновить точку входа:
    - Фабрика `backend.New(cfg *config.Config) (*Backend, error)`:
      - Читает `SE_STORAGE_BACKEND` (default: `localfs`)
      - `localfs`: создаёт `filestore.New()`, `attr.NewStore()`,
        `lockfile.NewLockManager()` → `Backend{Files, Attrs, Locks}`
      - `s3`: placeholder (Phase 7)
      - Неизвестное значение → ошибка
    - Добавить `SE_STORAGE_BACKEND` в `config.Config` (default: `localfs`,
      допустимые значения: `localfs`)
    - Обновить `main.go`: заменить ручное создание filestore/lockManager
      на `backend.New(cfg)`, передать `bk.Files`, `bk.Attrs`, `bk.Locks`
      в сервисы и handlers.
  - **Creates**:
    - `internal/backend/factory.go` — фабрика
    - Обновлённый `internal/config/config.go`
    - Обновлённый `cmd/storage-element/main.go`
  - **Links**: N/A

- [x] **6.7 Unit-тесты и валидация**
  - **Dependencies**: 6.1 - 6.6
  - **Description**: Финальная валидация:
    - Unit-тесты для фабрики (localfs, unknown backend → error)
    - Проверка что `filestore.FileStore` удовлетворяет `backend.FileStore`
      (compile-time: `var _ backend.FileStore = (*filestore.FileStore)(nil)`)
    - Проверка что `attr.Store` удовлетворяет `backend.AttrStore`
    - Проверка что `lockfile.LockManager` удовлетворяет `backend.LockStore`
    - `go test ./...` проходит без ошибок
    - `go vet ./...` без предупреждений
    - Проверка что существующие интеграционные тесты проходят без изменений
      (backward compatibility)
  - **Creates**:
    - `internal/backend/factory_test.go`
    - Compile-time checks в `internal/backend/checks.go`
    - Обновлённые тесты сервисов и handlers
  - **Links**: N/A

### Критерии завершения Phase 6

- [x] Все подпункты завершены (6.1 - 6.7)
- [x] Три интерфейса (`FileStore`, `AttrStore`, `LockStore`) определены
      в `internal/backend/`
- [x] Все существующие конкретные типы удовлетворяют интерфейсам
      (compile-time checks)
- [x] Все сервисы и handlers принимают интерфейсы вместо конкретных типов
- [x] Инициализация backend в main.go (фабрика не создана из-за import cycle,
      логика размещена напрямую в main.go)
- [x] GC и Reconcile остаются сервисами, не частью backend
- [x] Index и StateMachine не входят в абстракцию backend
- [x] `go test ./...` проходит без ошибок
- [x] `go vet ./...` без предупреждений
- [ ] Существующие интеграционные тесты SE проходят без изменений

---

## Phase 7: S3 Backend (будущее)

**Dependencies**: Phase 6
**Status**: Not Started

### Описание

Реализация S3-совместимого backend-а: `S3FileStore`, `S3AttrStore`,
`NoOpLockStore`. S3 обеспечивает атомарность на уровне `PutObject`, поэтому
per-file lock-файлы не нужны. Метаданные хранятся в sidecar-объектах
(`{key}.attr.json`) — не в object tags (лимит 10 тегов / 2KB).

> **Примечание**: Эта фаза будет детализирована непосредственно перед
> реализацией. Подпункты ниже — предварительный план на основе анализа
> архитектуры Phase 6.

### Архитектурные решения

> **Решение: Sidecar-объекты для метаданных (не object tags)**
>
> S3 object tags ограничены 10 тегами и 2KB на объект. `FileMetadata` содержит
> ~15 полей включая списки (tags, description). Sidecar-объекты `{key}.attr.json`
> не имеют ограничений, атомарно перезаписываются через `PutObject`, и позволяют
> переиспользовать формат LocalFS attr.json без изменений.

> **Решение: NoOpLockStore для S3**
>
> S3 `PutObject` атомарен. Нет риска частичной записи, как на LocalFS.
> `NoOpLockStore` возвращает пустые результаты для всех методов:
> - `Acquire()` → nil (no-op)
> - `Release()` → nil (no-op)
> - `IsLocked()` → false, nil, nil
> - `List()` → empty slice
> - `Cleanup()` → empty result
> - `TTL()` → 0
>
> Это означает что `DELETE /api/v1/files/{fileId}` никогда не вернёт 409
> при S3 backend (upload атомарен, конфликтов нет).

> **Решение: AvailableSpace() для S3 — конфигурируемый quota**
>
> У S3 нет понятия "свободное место". Используем `SE_S3_MAX_CAPACITY` —
> конфигурируемый лимит. `AvailableSpace()` = `SE_S3_MAX_CAPACITY` -
> `суммарный размер объектов` (кэшируется, обновляется периодически).

> **Решение: mode.json при S3 backend**
>
> При S3 backend mode.json хранится как S3 объект `{prefix}/mode.json`.
> ModeSyncService читает его через `GetObject`, ModeHandler записывает
> через `PutObject`. Интерфейс modefile адаптируется для абстракции
> (или mode.json выносится в отдельный `ModeStore` интерфейс).

### Структура S3 bucket

Структура S3 bucket **унифицирована с LocalFS** — используется та же date-based
иерархия `YYYY/MM/DD/` (Phase 5.5). StoragePath одинаков для обоих backend-ов.

```
{bucket}/
└── {storageID}/
    ├── mode.json                              ← режим SE
    ├── data/
    │   ├── 2026/
    │   │   ├── 03/
    │   │   │   ├── 01/
    │   │   │   │   ├── photo_admin_20260301150405_abc123.jpg
    │   │   │   │   ├── photo_admin_20260301150405_abc123.jpg.attr.json
    │   │   │   │   ├── report_viewer_20260301120000_def456.pdf
    │   │   │   │   └── report_viewer_20260301120000_def456.pdf.attr.json
    │   │   │   └── 02/
    │   │   │       ├── doc_admin_20260302093000_deadbeef.docx
    │   │   │       └── doc_admin_20260302093000_deadbeef.docx.attr.json
    │   │   └── ...
    │   └── ...
    └── (нет .locks/ — блокировки не нужны для S3)
```

**Формат ключей**:
- Data: `{storageID}/data/{storagePath}` (storagePath = `YYYY/MM/DD/filename`)
- Attr: `{storageID}/data/{storagePath}.attr.json`
- Mode: `{storageID}/mode.json`

**Примеры S3 key**:
- `se-edit-1/data/2026/03/01/photo_admin_20260301150405_abc123.jpg`
- `se-edit-1/data/2026/03/01/photo_admin_20260301150405_abc123.jpg.attr.json`

**Преимущества date-based иерархии в S3**:
- `ListObjectsV2` с `Prefix=se-edit-1/data/2026/03/01/` для листинга за конкретный день
- `Delimiter=/` позволяет виртуальные папки в S3 Console / aws cli
- Консистентный admin experience между LocalFS и S3

### Подпункты

- [ ] **7.1 Дизайн-документ S3 Backend**
  - **Dependencies**: None
  - **Description**: Создать техдизайн-документ с детальным описанием:
    - Структура bucket-а и формат ключей (описано выше)
    - Стратегия метаданных: sidecar-объекты `.attr.json`
    - Обработка ошибок S3: retries (exponential backoff), rate limiting (429),
      network errors, partial failures
    - Multi-part upload: порог размера файла для перехода на multipart
      (default: 100MB), размер части (default: 16MB)
    - GC при S3: использование S3 Lifecycle Rules для автоматического удаления
      expired объектов vs программный GC через `ListObjectsV2`
    - Reconcile при S3: `ListObjectsV2` для обнаружения orphaned объектов
      (data без attr.json, attr.json без data)
    - Index rebuild при S3: `ListObjectsV2` с фильтром `*.attr.json` →
      `GetObject` для каждого → parse → rebuild index.
      При >100K файлов: пагинация с MaxKeys=1000, параллельный `GetObject`
    - Стоимость: `ListObjectsV2` ($0.005/1000 запросов), `GetObject`
      ($0.0004/1000 запросов). Index rebuild каждые 30s при 10K файлов ≈
      $1.3/месяц
    - Conditional writes: `If-None-Match: *` для предотвращения перезаписи
      (опционально — fileID уникален, коллизий нет)
    - Сравнение S3 strong consistency (с декабря 2020) vs eventual consistency
  - **Creates**:
    - `docs/design/se-s3-backend-design.md`
  - **Links**: N/A

- [ ] **7.2 Реализация S3FileStore**
  - **Dependencies**: 7.1
  - **Description**: Реализовать `backend.FileStore` для S3:
    - Использовать AWS SDK for Go v2 (`github.com/aws/aws-sdk-go-v2`)
    - `SaveFile()`: streaming `PutObject` (для файлов > порога — multipart
      upload через `s3manager.Uploader`)
    - `ReadFile()`: `GetObject` → `io.ReadCloser` (body)
    - `DeleteFile()`: `DeleteObject` (идемпотентно — S3 не ошибается на 404)
    - `FileExists()`: `HeadObject` (NoSuchKey → false)
    - `FileSize()`: `HeadObject` → ContentLength
    - `ComputeChecksum()`: `HeadObject` → `ChecksumSHA256` (если включён)
      или `GetObject` + вычисление
    - `AvailableSpace()`: `SE_S3_MAX_CAPACITY` - кэшированный total size
      (обновляется по таймеру или при каждом upload/delete)
    - `FullPath()`: возвращает S3 key (для логирования)
    - Retry-стратегия: exponential backoff (aws-sdk-go-v2 built-in retry)
    - Context: все операции используют переданный ctx для таймаутов и отмены
  - **Creates**:
    - `internal/backend/s3/filestore.go`
    - `internal/backend/s3/filestore_test.go` (тесты с MinIO через
      testcontainers-go)
  - **Links**: N/A

- [ ] **7.3 Реализация S3AttrStore**
  - **Dependencies**: 7.1
  - **Description**: Реализовать `backend.AttrStore` для S3:
    - `Write()`: `PutObject` с ключом `{storagePath}.attr.json`,
      Content-Type: `application/json`. JSON-формат идентичен LocalFS attr.json.
    - `Read()`: `GetObject` → JSON decode
    - `Delete()`: `DeleteObject` (идемпотентно)
    - `ScanAll()`: `ListObjectsV2` с Prefix `{storageID}/data/` и фильтром
      `*.attr.json`. Date-based иерархия `YYYY/MM/DD/` позволяет инкрементальный
      scan по дням при необходимости. Для каждого результата — `GetObject` + JSON
      decode. Пагинация через ContinuationToken. Параллельный fetch через worker
      pool (concurrency = 10).
    - Кэширование: `ScanAll` может быть дорогим при >10K файлов.
      Опционально: incremental scan (запоминать LastModified, читать только
      изменённые).
  - **Creates**:
    - `internal/backend/s3/attrstore.go`
    - `internal/backend/s3/attrstore_test.go`
  - **Links**: N/A

- [ ] **7.4 Реализация NoOpLockStore**
  - **Dependencies**: 7.1
  - **Description**: Реализовать `backend.LockStore` как no-op:
    - Все методы возвращают пустые/нулевые значения без ошибок
    - `TTL()` → 0 (lock-и не используются при S3)
    - Минимальный код (~30 строк)
    - Следствие: `DELETE` никогда не возвращает 409 при S3 backend,
      `GET /api/v1/locks` всегда возвращает пустой список
  - **Creates**:
    - `internal/backend/s3/lockstore.go`
    - `internal/backend/s3/lockstore_test.go`
  - **Links**: N/A

- [ ] **7.5 Адаптация modefile для S3**
  - **Dependencies**: 7.2
  - **Description**: Решить проблему mode.json при S3 backend:
    - **Вариант A** (рекомендуется): `ModeStore` интерфейс в `internal/backend/`:
      ```go
      type ModeStore interface {
          LoadMode(ctx context.Context) (string, string, error) // mode, updatedBy, err
          SaveMode(ctx context.Context, mode, updatedBy string) error
      }
      ```
      LocalFS: делегирует к `modefile.LoadMode()`/`SaveMode()`.
      S3: `GetObject`/`PutObject` на `{storageID}/mode.json`.
    - **Вариант B**: хранить mode.json в ConfigMap (K8s-зависимость)
    - Обновить `ModeSyncService` и `ModeHandler` для работы через интерфейс
  - **Creates**:
    - `internal/backend/modestore.go` — интерфейс ModeStore
    - `internal/backend/s3/modestore.go` — S3 реализация
    - Обновлённые `internal/service/modesync.go`, `internal/api/handlers/mode.go`
  - **Links**: N/A

- [ ] **7.6 Конфигурация S3**
  - **Dependencies**: 7.2
  - **Description**: Добавить env-переменные для S3 backend:
    - `SE_STORAGE_BACKEND` — `localfs` (default) или `s3`
    - `SE_S3_ENDPOINT` — S3 endpoint URL (обязателен при `s3`)
    - `SE_S3_BUCKET` — имя bucket-а (обязателен при `s3`)
    - `SE_S3_REGION` — AWS region (default: `us-east-1`)
    - `SE_S3_ACCESS_KEY` — AWS access key
    - `SE_S3_SECRET_KEY` — AWS secret key
    - `SE_S3_USE_PATH_STYLE` — path-style URL (default: `true` — для MinIO)
    - `SE_S3_MAX_CAPACITY` — quota в байтах (обязателен при `s3`,
      замена `SE_MAX_CAPACITY` для S3)
    - `SE_S3_MULTIPART_THRESHOLD` — порог для multipart upload
      (default: `104857600` = 100MB)
    - `SE_S3_MULTIPART_PART_SIZE` — размер части multipart
      (default: `16777216` = 16MB)
    - Валидация: при `SE_STORAGE_BACKEND=s3` обязательны `SE_S3_ENDPOINT`,
      `SE_S3_BUCKET`, `SE_S3_MAX_CAPACITY`
  - **Creates**:
    - Обновлённый `internal/config/config.go`
  - **Links**: N/A

- [ ] **7.7 Обновление фабрики backend**
  - **Dependencies**: 7.2, 7.3, 7.4, 7.5, 7.6
  - **Description**: Обновить `backend.New()`:
    - При `SE_STORAGE_BACKEND=s3`:
      - Создать S3 client (`aws-sdk-go-v2/config`, `s3.NewFromConfig()`)
      - Проверить доступность bucket (HeadBucket)
      - Создать `S3FileStore`, `S3AttrStore`, `NoOpLockStore`
      - Вернуть `Backend{Files, Attrs, Locks}`
    - Обновить main.go: передать `ModeStore` в ModeSyncService/ModeHandler
  - **Creates**:
    - Обновлённый `internal/backend/factory.go`
    - Обновлённый `cmd/storage-element/main.go`
  - **Links**: N/A

- [ ] **7.8 Интеграционные тесты с MinIO**
  - **Dependencies**: 7.7
  - **Description**: Комплексное тестирование S3 backend:
    - **Unit-тесты**: MinIO через `testcontainers-go` — auto-start контейнера,
      создание bucket, прогон тестов каждого Store
    - **Интеграционные тесты в K8s**:
      - Развернуть MinIO в `artstore-test` через Helm chart
        (`minio/minio` chart)
      - Создать SE с `SE_STORAGE_BACKEND=s3` и указанием на MinIO
      - Прогон всех существующих SE интеграционных тестов
        (upload/download/delete/mode/reconcile/locks)
    - **Тест-кейсы**:
      - Upload файла через S3, download, проверка checksum
      - Delete через S3
      - GC для S3 (mark expired, delete)
      - Reconcile для S3 (orphaned objects)
      - Index rebuild из S3 (ListObjectsV2 + parse)
      - Locks API: GET /locks → пустой, cleanup → empty result
      - mode.json на S3: transition → sync
      - Multi-replica: upload через pod-1, download через pod-2
    - **Производительность**: замер ScanAll на 1K/10K/100K объектов
  - **Creates**:
    - Тесты в `internal/backend/s3/*_test.go` (testcontainers-go)
    - Тесты в `tests/scripts/test-se-s3.sh` (K8s интеграция)
    - MinIO Helm values в `tests/helm/artstore-se/`
  - **Links**: N/A

- [ ] **7.9 Обновление Helm chart для S3**
  - **Dependencies**: 7.6
  - **Description**: Обновить `charts/storage-element/`:
    - Добавить в `values.yaml`: `storageBackend`, `s3.*` секция
    - Условная конфигурация: при `storageBackend=s3` — не создавать PVC,
      добавить S3 env-переменные
    - При `storageBackend=localfs` — поведение без изменений
    - S3 credentials через `Secret` (не через values напрямую)
    - Добавить `SE_S3_*` env-переменные в deployment.yaml
  - **Creates**:
    - Обновлённый Helm chart `charts/storage-element/`
  - **Links**: N/A

### Критерии завершения Phase 7

- [ ] Все подпункты завершены (7.1 - 7.9)
- [ ] `S3FileStore`, `S3AttrStore`, `NoOpLockStore` реализованы
- [ ] SE работает с S3 backend (MinIO) в K8s
- [ ] Все существующие интеграционные тесты SE проходят с S3 backend
- [ ] `ModeStore` интерфейс реализован для S3
- [ ] Index rebuild работает через S3 `ListObjectsV2`
- [ ] GC и Reconcile работают с S3 backend
- [ ] Multi-replica тест проходит (upload pod-1 → download pod-2)
- [ ] API полностью backward compatible
- [ ] Helm chart поддерживает `storageBackend: s3` с conditional PVC/env
- [ ] `go test ./...` проходит без ошибок

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

| Компонент | Назначение | Phase |
|-----------|-----------|-------|
| `internal/lockfile/` | Per-file lock с TTL | 1 |
| `internal/modefile/` | Чтение/запись mode.json | 1 |
| `internal/service/modesync.go` | Периодическая синхронизация mode.json | 4 |
| `internal/service/indexsync.go` | Периодическая пересборка индекса | 4 |
| Lock API handlers | GET /locks, POST /locks/cleanup | 3 |
| `internal/backend/` | Интерфейсы FileStore, AttrStore, LockStore, фабрика | 6 |
| Иерархическая структура `YYYY/MM/DD/` | Date-based каталоги, рекурсивный обход | 5.5 |
| `internal/storage/attr/store.go` | LocalAttrStore адаптер для интерфейса AttrStore | 6 |
| `internal/backend/s3/` | S3FileStore, S3AttrStore, NoOpLockStore | 7 |

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
| S3 ScanAll дорогой при >100K файлов | Средняя | Среднее | Пагинация, параллельный fetch, incremental scan |
| S3 AvailableSpace неточен | Низкая | Низкое | Конфигурируемый quota + кэш total size |
| S3 rate limiting (503/429) | Средняя | Среднее | aws-sdk-go-v2 built-in retry с exponential backoff |
| mode.json на S3 — eventual consistency | Низкая | Низкое | S3 strong consistency (с 2020), ModeSync 10s |
| WalkDir медленнее ReadDir при малом кол-ве файлов | Низкая | Низкое | Разница незначительна; при росте WalkDir выигрывает |
| os.MkdirAll на NFS при каждом upload | Низкая | Низкое | No-op при существующем каталоге, минимальный overhead |

---

## Примечания

- Перед началом Phase 1 создать ветку `refactor/se-stateless` от `main`.
- Каждая фаза — один контекст AI (одна сессия разработки).
- Phases 6-7 (StorageBackend, S3) могут быть отложены — Phases 1-5.5 дают
  полностью рабочий stateless SE с иерархическим хранением.
- Версионирование: minor bump (`0.Y.0`), т.к. архитектурное изменение.
- Текущий API-контракт (`docs/api-contracts/storage-element-openapi.yaml`)
  обновляется в Phase 3.6.
