# Phase 7: S3 Backend для Storage Element

**Dependencies**: Phase 6 (завершена)
**Status**: Not Started
**Ветка**: `refactor/se-stateless`

## Контекст

Phase 6 ввела абстракцию backend (интерфейсы `FileStore`, `AttrStore`, `LockStore`).
Сейчас существуют только LocalFS реализации. Phase 7 добавляет S3-совместимый backend:
`S3FileStore`, `S3AttrStore`, `NoOpLockStore`, `ModeStore` интерфейс, фабрику backend
и обновление Helm chart.

## Архитектурные решения

### Sidecar-объекты для метаданных (не object tags)

S3 object tags ограничены 10 тегами и 2KB. `FileMetadata` содержит ~15 полей.
Sidecar-объекты `{key}.attr.json` — без ограничений, атомарно перезаписываются через `PutObject`,
переиспользуют формат LocalFS attr.json без изменений.

### NoOpLockStore для S3

S3 `PutObject` атомарен. Нет риска частичной записи.
`NoOpLockStore` возвращает пустые результаты для всех методов.
Следствие: `DELETE /api/v1/files/{fileId}` никогда не вернёт 409 при S3 backend.

### AvailableSpace — конфигурируемый quota

`SE_S3_MAX_CAPACITY` — конфигурируемый лимит.
`AvailableSpace()` = `SE_S3_MAX_CAPACITY` - `cachedTotalSize` (atomic counter, обновляется при upload/delete).

### mode.json при S3

Хранится как S3 объект `{storageID}/mode.json`. Новый интерфейс `ModeStore`:

- `LoadMode(ctx) (StorageMode, error)`
- `SaveMode(ctx, mode, updatedBy) error`

LocalFS: делегирует к `modefile.LoadMode()`/`SaveMode()`.
S3: `GetObject`/`PutObject`.

### Структура S3 bucket

Унифицирована с LocalFS — date-based иерархия `YYYY/MM/DD/` (Phase 5.5).

```
{bucket}/
└── {storageID}/
    ├── mode.json
    └── data/
        └── 2026/03/01/
            ├── photo_admin_20260301150405_abc123.jpg
            └── photo_admin_20260301150405_abc123.jpg.attr.json
```

Ключи: `{storageID}/data/{storagePath}`, `{storageID}/data/{storagePath}.attr.json`,
`{storageID}/mode.json`

### SE_DATA_DIR при S3 backend

Оставляем обязательным. При S3 в Helm chart монтируем `EmptyDir` в `/data`.
Минимальные изменения в валидации config.go. DataDir может использоваться для temp-файлов.

---

## Подготовительный рефакторинг (P0)

### P0.1 Пакет `internal/backend/storagepath/`

**Проблема:** `generateStoragePath()` и `sanitize()` определены в
`internal/storage/filestore/filestore.go` как package-private. S3FileStore нужна та же логика.

**Создать:** `internal/backend/storagepath/storagepath.go`

```go
package storagepath

// Generate генерирует относительный путь файла для хранения.
// Формат: YYYY/MM/DD/{name}_{user}_{timestamp}_{uuid8}.{ext}
func Generate(originalFilename, uploadedBy string) string

// sanitize убирает небезопасные символы из строки.
func sanitize(s string) string
```

**Обновить:** `internal/storage/filestore/filestore.go` — заменить `generateStoragePath()`
на `storagepath.Generate()`, удалить локальные функции.

### P0.2 Добавить `FileModTime` в интерфейс FileStore

**Проблема:** `reconcile.go:268-276` вызывает `os.Stat(rs.files.FullPath(dataFile))`.
Для S3 `FullPath()` возвращает S3 key, `os.Stat` упадёт.

**Изменить:** `internal/backend/interfaces.go` — добавить в `FileStore`:

```go
// FileModTime возвращает время последней модификации файла.
// LocalFS: os.Stat().ModTime(). S3: HeadObject → LastModified.
FileModTime(ctx context.Context, storagePath string) (time.Time, error)
```

**Обновить:**

- `internal/storage/filestore/filestore.go` — добавить `FileModTime()`:
  `os.Stat(fullPath).ModTime()`
- `internal/service/reconcile.go:262-276` — заменить:

```go
// БЫЛО:
fullPath := rs.files.FullPath(dataFile)
if info, statErr := os.Stat(fullPath); statErr == nil {
    if now.Sub(info.ModTime()) < lockTTL {
        ...continue
    }
}

// СТАЛО:
if lockTTL > 0 {
    if modTime, modErr := rs.files.FileModTime(ctx, dataFile); modErr == nil {
        if now.Sub(modTime) < lockTTL {
            ...continue
        }
    }
}
```

При S3 `NoOpLockStore.TTL()=0` → freshness check пропускается, все orphaned объекты
репортируются — корректно.
Убрать `import "os"` из reconcile.go (если единственное использование).

---

## 7.1 Дизайн-документ S3 Backend

**Creates:** `docs/design/se-s3-backend-design.md`

Разделы: обзор архитектуры, структура bucket, стратегия метаданных, обработка ошибок S3
(retries, idempotency), multipart upload, GC/Reconcile при S3, index rebuild,
производительность и стоимость, конфигурация.

---

## 7.2 Реализация S3FileStore

**Creates:** `internal/backend/s3/filestore.go`, `internal/backend/s3/filestore_test.go`

```go
var _ backend.FileStore = (*FileStore)(nil)

type FileStore struct {
    client             *s3.Client
    uploader           *manager.Uploader
    bucket             string
    storageID          string
    useStorageIDPrefix bool
    maxCapacity        int64
    cachedTotalSize    atomic.Int64
}

func NewFileStore(client *s3.Client, bucket, storageID string, usePrefix bool,
    maxCapacity, multipartThreshold, partSize int64) *FileStore
```

| Метод | Реализация |
|-------|-----------|
| `SaveFile` | `storagepath.Generate()` → streaming PutObject (TeeReader + SHA-256). Для >threshold → `manager.Uploader` |
| `ReadFile` | `GetObject` → `response.Body` (io.ReadCloser) |
| `DeleteFile` | `DeleteObject` (идемпотентно) |
| `FileExists` | `HeadObject` (NoSuchKey → false) |
| `FileSize` | `HeadObject` → `ContentLength` |
| `FileModTime` | `HeadObject` → `LastModified` |
| `ComputeChecksum` | `GetObject` → потоковый SHA-256 |
| `AvailableSpace` | `maxCapacity - cachedTotalSize.Load()` |
| `FullPath` | `"s3://{bucket}/{key}"` (для логирования) |
| `ListDataPaths` | `ListObjectsV2` + пагинация, фильтр `.attr.json`/`mode.json` |
| `UpdateCachedSize` | `ListObjectsV2` → суммируем Size, сохраняем в `cachedTotalSize` |

`dataKey(storagePath)` → `"{storageID}/data/{storagePath}"` при `useStorageIDPrefix=true`.

`SaveFile` при каждом upload: `cachedTotalSize.Add(size)`.
`DeleteFile`: `cachedTotalSize.Add(-size)` (через `HeadObject` перед `DeleteObject`).

Тесты через `testcontainers-go` + MinIO.

---

## 7.3 Реализация S3AttrStore

**Creates:** `internal/backend/s3/attrstore.go`, `internal/backend/s3/attrstore_test.go`

```go
var _ backend.AttrStore = (*AttrStore)(nil)

type AttrStore struct {
    client    *s3.Client
    bucket    string
    storageID string
    usePrefix bool
}

func NewAttrStore(client *s3.Client, bucket, storageID string, usePrefix bool) *AttrStore
```

| Метод | Реализация |
|-------|-----------|
| `Write` | `PutObject` ключ `attrKey(storagePath)`, Content-Type: `application/json` |
| `Read` | `GetObject` → JSON decode |
| `Delete` | `DeleteObject` (идемпотентно) |
| `ScanAll` | `ListObjectsV2` prefix=`{storageID}/data/`, фильтр `*.attr.json`, пагинация через `ContinuationToken`. Параллельный `GetObject` через worker pool (concurrency=10, `errgroup`). |

`attrKey(storagePath)` → `"{storageID}/data/{storagePath}.attr.json"`.

---

## 7.4 Реализация NoOpLockStore

**Creates:** `internal/backend/s3/lockstore.go`, `internal/backend/s3/lockstore_test.go`

~30 строк. Все методы — заглушки:

```go
var _ backend.LockStore = (*NoOpLockStore)(nil)

type NoOpLockStore struct{}

func NewNoOpLockStore() *NoOpLockStore
func (*NoOpLockStore) Acquire(context.Context, string) error                              { return nil }
func (*NoOpLockStore) Release(context.Context, string) error                              { return nil }
func (*NoOpLockStore) IsLocked(context.Context, string) (bool, *backend.LockInfo, error)  { return false, nil, nil }
func (*NoOpLockStore) List(context.Context) ([]backend.LockInfo, error)                   { return nil, nil }
func (*NoOpLockStore) Cleanup(context.Context, bool) (*backend.CleanupResult, error)      { return &backend.CleanupResult{}, nil }
func (*NoOpLockStore) TTL() time.Duration                                                 { return 0 }
func (*NoOpLockStore) EnsureDir() error                                                   { return nil }
```

Тесты — простые unit-тесты без контейнера.

---

## 7.5 Адаптация modefile для S3

### Шаг 1: Интерфейс ModeStore

**Creates:** `internal/backend/modestore.go`

```go
type ModeStore interface {
    LoadMode(ctx context.Context) (mode.StorageMode, error)
    SaveMode(ctx context.Context, m mode.StorageMode, updatedBy string) error
}
```

### Шаг 2: LocalFS реализация

**Creates:** `internal/backend/localfs/modestore.go`

```go
type LocalModeStore struct { modeFilePath string }

func NewLocalModeStore(modeFilePath string) *LocalModeStore
func (ms *LocalModeStore) LoadMode(_ context.Context) (mode.StorageMode, error)
func (ms *LocalModeStore) SaveMode(_ context.Context, m mode.StorageMode, updatedBy string) error
```

### Шаг 3: S3 реализация

**Creates:** `internal/backend/s3/modestore.go`, `internal/backend/s3/modestore_test.go`

```go
type S3ModeStore struct {
    client  *s3.Client
    bucket  string
    modeKey string // "{storageID}/mode.json"
}

func NewS3ModeStore(client *s3.Client, bucket, storageID string) *S3ModeStore
func (ms *S3ModeStore) LoadMode(ctx) → GetObject → JSON decode → ParseMode
func (ms *S3ModeStore) SaveMode(ctx, m, updatedBy) → JSON encode ModeFileData → PutObject
```

### Шаг 4: Обновить ModeSyncService

**Изменить:** `internal/service/modesync.go`

```go
// БЫЛО:
type ModeSyncService struct { modeFilePath string; ... }
func NewModeSyncService(modeFilePath string, sm, interval, logger)

// СТАЛО:
type ModeSyncService struct { modeStore backend.ModeStore; ... }
func NewModeSyncService(modeStore backend.ModeStore, sm, interval, logger)
```

`SyncOnce()`: `modefile.LoadMode(mss.modeFilePath)` → `mss.modeStore.LoadMode(ctx)`.
Убрать `import "github.com/.../modefile"`.

### Шаг 5: Обновить modePersisterAdapter в main.go

```go
// БЫЛО:
type modePersisterAdapter struct { path string; updatedByFn func() string }
func (a *modePersisterAdapter) SaveMode(m mode.StorageMode) error {
    return modefile.SaveMode(a.path, m, a.updatedByFn())
}

// СТАЛО:
type modePersisterAdapter struct { modeStore backend.ModeStore; updatedByFn func() string }
func (a *modePersisterAdapter) SaveMode(m mode.StorageMode) error {
    return a.modeStore.SaveMode(context.Background(), m, a.updatedByFn())
}
```

Интерфейс `handlers.ModePersister` НЕ меняется.

---

## 7.6 Конфигурация S3

**Изменить:** `internal/config/config.go`

Добавить в `Config` struct:

| Env-переменная | Поле | Тип | Default | Обязательность |
|---|---|---|---|---|
| `SE_S3_ENDPOINT` | `S3Endpoint` | string | — | обязательно при s3 |
| `SE_S3_BUCKET` | `S3Bucket` | string | — | обязательно при s3 |
| `SE_S3_REGION` | `S3Region` | string | `us-east-1` | — |
| `SE_S3_ACCESS_KEY` | `S3AccessKey` | string | `""` | опционально (IAM roles) |
| `SE_S3_SECRET_KEY` | `S3SecretKey` | string | `""` | опционально |
| `SE_S3_USE_PATH_STYLE` | `S3UsePathStyle` | bool | `true` | — (true для MinIO) |
| `SE_S3_MAX_CAPACITY` | `S3MaxCapacity` | int64 | — | обязательно при s3 |
| `SE_S3_MULTIPART_THRESHOLD` | `S3MultipartThreshold` | int64 | `104857600` (100MB) | — |
| `SE_S3_MULTIPART_PART_SIZE` | `S3MultipartPartSize` | int64 | `16777216` (16MB) | — |
| `SE_S3_CONNECT_TIMEOUT` | `S3ConnectTimeout` | duration | `30s` | — |

Расширить `validBackends`: `{"localfs": true, "s3": true}`.
Добавить `loadS3Config(cfg)` — вызывается при `StorageBackend == "s3"`.

---

## 7.7 Обновление фабрики backend

### Creates: `internal/backend/factory.go`

```go
// Stores — все store-ы для Storage Element.
type Stores struct {
    Files FileStore
    Attrs AttrStore
    Locks LockStore
    Mode  ModeStore
}

// New создаёт Stores на основе конфигурации.
func New(ctx context.Context, cfg *config.Config, hostname string,
    logger *slog.Logger) (*Stores, error)
```

`switch cfg.StorageBackend`:

- `"localfs"` → `newLocalFS()`: filestore.New + attr.NewStore + lockfile.NewLockManager +
  LocalModeStore. Вызывает `EnsureDir()` для locks.
- `"s3"` → `newS3()`:
  1. `aws.Config` через `config.LoadDefaultConfig()` + static credentials (если заданы)
  2. `s3.NewFromConfig()` с endpoint + path-style
  3. `HeadBucket` проверка доступности
  4. Создание `S3FileStore`, `S3AttrStore`, `NoOpLockStore`, `S3ModeStore`
  5. `UpdateCachedSize()` при старте

### Обновить: `cmd/storage-element/main.go`

Заменить прямое создание stores на:

```go
stores, err := backend.New(ctx, cfg, hostname, logger)
```

Передавать `stores.Files`, `stores.Attrs`, `stores.Locks`, `stores.Mode` в сервисы и handlers.

Инициализация initialMode: `stores.Mode.LoadMode(ctx)` вместо `modefile.LoadMode(modeFilePath)`.
ModeSyncService: `NewModeSyncService(stores.Mode, sm, ...)`.
modePersisterAdapter: через `stores.Mode`.

Убрать прямые импорты `filestore`, `attr`, `lockfile`, `modefile` из main.go.

---

## 7.8 Интеграционные тесты с MinIO

### Unit-тесты (testcontainers-go)

**Creates:** `internal/backend/s3/testhelper_test.go` — общий helper `setupMinIO(t)`:

- MinIO container `minio/minio:latest`, порт 9000
- Создание test bucket
- Возврат `*s3.Client`

Тест-файлы (уже перечислены в 7.2-7.5):

- `filestore_test.go` — SaveAndRead, Delete, FileExists, FileModTime, AvailableSpace,
  ListDataPaths, ComputeChecksum
- `attrstore_test.go` — WriteAndRead, Delete, ScanAll (пустой, пагинация, concurrent)
- `lockstore_test.go` — AllMethods, IsLocked_AlwaysFalse, TTL_Zero
- `modestore_test.go` — SaveAndLoad, LoadMode_NotFound

### Интеграционные тесты K8s

**Creates:** `tests/scripts/test-se-s3.sh`

Тест-кейсы: upload/download/delete через S3 SE, GET /locks → пустой,
POST /locks/cleanup → empty, mode transition, multi-replica test.

MinIO deployment в тестовом окружении (helm chart или standalone).

---

## 7.9 Обновление Helm chart

**Изменить:** `charts/storage-element/`

### values.yaml

Добавить:

```yaml
storageBackend: localfs

s3:
  endpoint: ""
  bucket: ""
  region: "us-east-1"
  usePathStyle: true
  maxCapacity: ""
  multipartThreshold: "104857600"
  multipartPartSize: "16777216"
  connectTimeout: "30s"
  credentialsSecretName: ""   # Secret с ключами accessKey, secretKey
```

### _helpers.tpl — `se.envVars`

Добавить условный блок `{{- if eq .Values.storageBackend "s3" }}` с SE_S3_* env-переменными.
Credentials через `secretKeyRef` из `credentialsSecretName`.

### pvc.yaml

Обернуть в `{{- if ne .Values.storageBackend "s3" }}` — PVC только для localfs.

### deployment.yaml

Volumes и volumeMounts для `/data`:

- При `localfs`: PVC (как сейчас)
- При `s3`: EmptyDir (для SE_DATA_DIR=/data, temp-файлы)

---

## Новые Go-зависимости

```bash
cd src/storage-element
go get github.com/aws/aws-sdk-go-v2@latest \
       github.com/aws/aws-sdk-go-v2/config@latest \
       github.com/aws/aws-sdk-go-v2/credentials@latest \
       github.com/aws/aws-sdk-go-v2/service/s3@latest \
       github.com/aws/aws-sdk-go-v2/feature/s3/manager@latest
```

testcontainers-go уже используется в проекте (проверить go.mod).

---

## Сводка файлов

### Новые файлы (14)

| Файл | Описание |
|------|----------|
| `docs/design/se-s3-backend-design.md` | Дизайн-документ |
| `internal/backend/storagepath/storagepath.go` | Общая генерация путей |
| `internal/backend/modestore.go` | Интерфейс ModeStore |
| `internal/backend/factory.go` | Фабрика backend |
| `internal/backend/localfs/modestore.go` | LocalFS ModeStore |
| `internal/backend/s3/filestore.go` | S3 FileStore |
| `internal/backend/s3/attrstore.go` | S3 AttrStore |
| `internal/backend/s3/lockstore.go` | NoOpLockStore |
| `internal/backend/s3/modestore.go` | S3 ModeStore |
| `internal/backend/s3/testhelper_test.go` | MinIO test helper |
| `internal/backend/s3/filestore_test.go` | Тесты FileStore |
| `internal/backend/s3/attrstore_test.go` | Тесты AttrStore |
| `internal/backend/s3/lockstore_test.go` | Тесты NoOpLockStore |
| `internal/backend/s3/modestore_test.go` | Тесты ModeStore |

### Изменяемые файлы (9)

| Файл | Изменения |
|------|-----------|
| `internal/backend/interfaces.go` | +`FileModTime()` в FileStore |
| `internal/config/config.go` | +S3 поля, `loadS3Config()`, `"s3"` в validBackends |
| `internal/storage/filestore/filestore.go` | +`FileModTime()`, `generateStoragePath` → `storagepath.Generate()` |
| `internal/service/reconcile.go` | `os.Stat` → `FileModTime()`, +`lockTTL > 0` guard |
| `internal/service/modesync.go` | `modeFilePath` → `modeStore backend.ModeStore` |
| `cmd/storage-element/main.go` | Фабрика `backend.New()`, ModeStore wiring |
| `charts/storage-element/values.yaml` | +`storageBackend`, +`s3.*` секция |
| `charts/storage-element/templates/_helpers.tpl` | +S3 env-переменные |
| `charts/storage-element/templates/pvc.yaml` | Условное создание PVC |

---

## Порядок реализации

```
 1. [P0.1] storagepath package
 2. [P0.2] FileModTime: interfaces.go + filestore.go + reconcile.go
 3. [7.6]  config.go — S3 поля
 4. [7.4]  NoOpLockStore (минимальный, без контейнера)
 5. [7.2]  S3FileStore
 6. [7.3]  S3AttrStore
 7. [7.5]  ModeStore: интерфейс + localfs + S3 + modesync.go
 8. [7.7]  factory.go + main.go
 9.        go build ./... — проверка компиляции
10. [7.8]  Тесты (testcontainers-go + MinIO)
11. [7.9]  Helm chart
12. [7.1]  Дизайн-документ (пишется по результатам реализации)
```

---

## Верификация

1. `cd src/storage-element && go build ./...` — компиляция без ошибок
2. `go test ./internal/backend/s3/...` — unit-тесты S3 stores с MinIO
3. `go test ./...` — все тесты проходят (включая существующие LocalFS)
4. `SE_STORAGE_BACKEND=localfs go test ./...` — регрессия LocalFS
5. Helm: `helm template` с `storageBackend: localfs` — без изменений
6. Helm: `helm template` с `storageBackend: s3` — PVC не создаётся, S3 env есть

---

## Критерии завершения Phase 7

- [ ] `S3FileStore`, `S3AttrStore`, `NoOpLockStore` реализованы
- [ ] `ModeStore` интерфейс + LocalFS и S3 реализации
- [ ] Backend фабрика `New()` выбирает backend по конфигурации
- [ ] `main.go` использует фабрику, не создаёт stores напрямую
- [ ] `reconcile.go` не использует `os.Stat` напрямую
- [ ] `go build ./...` проходит без ошибок
- [ ] Unit-тесты S3 stores с MinIO (testcontainers-go)
- [ ] Helm chart поддерживает `storageBackend: s3`
- [ ] API полностью backward compatible
- [ ] Дизайн-документ `docs/design/se-s3-backend-design.md`
