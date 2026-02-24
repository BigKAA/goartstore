# План разработки: SE Capacity Management

## Метаданные

- **Версия плана**: 1.0.0
- **Дата создания**: 2026-02-24
- **Последнее обновление**: 2026-02-24
- **Статус**: In Progress

---

## История версий

- **v1.0.0** (2026-02-24): Начальная версия плана

---

## Текущий статус

- **Активная фаза**: Phase 2
- **Активный подпункт**: 2.1
- **Последнее обновление**: 2026-02-24
- **Примечание**: Phase 1 завершена — Config.MaxCapacity, Index.totalActiveSize, unit-тесты

---

## Контекст проблемы

Storage Element (SE) сообщает информацию о дисковом пространстве через `syscall.Statfs()`,
что возвращает размер **всей файловой системы**, а не сконфигурированный лимит SE.
На скриншоте Admin UI показывает 5.4 TB (весь NFS-диск) вместо реального лимита.

### Ключевые проблемы

1. `CapacityInfo` в `/api/v1/info` показывает размер всего диска, а не лимит SE
2. Нет конфигурационного параметра `SE_MAX_CAPACITY`
3. Нет проверки свободного места перед загрузкой файла в пределах лимита

### Принятые решения (brainstorm)

| Решение | Выбор |
|---------|-------|
| Конфигурация лимита | `SE_MAX_CAPACITY` — обязательная env-переменная |
| AM override | Через конфиг при деплое (Helm values), не runtime |
| Поведение при переполнении | HTTP 507 `STORAGE_FULL` до начала записи |
| Подсчёт used_bytes | Из индекса (`TotalActiveSize`), O(1) кумулятивный счётчик |
| Учёт overhead | Только файл данных (без attr.json, без WAL) |
| Уведомление AM | AM получает через polling `/api/v1/info` |
| API ответ | Только лимит (`total=MAX_CAPACITY`), без физического диска |
| Обязательность | SE не запускается без `SE_MAX_CAPACITY` |

---

## Оглавление

- [x] [Phase 1: Config и Index — базовая инфраструктура](#phase-1-config-и-index--базовая-инфраструктура)
- [ ] [Phase 2: Upload проверка и SystemHandler](#phase-2-upload-проверка-и-systemhandler)
- [ ] [Phase 3: Helm chart и OpenAPI](#phase-3-helm-chart-и-openapi)
- [ ] [Phase 4: Сборка, деплой и тестирование](#phase-4-сборка-деплой-и-тестирование)

---

## Phase 1: Config и Index — базовая инфраструктура

**Dependencies**: None
**Status**: Done ✅

### Описание

Добавление конфигурационного параметра `SE_MAX_CAPACITY` и кумулятивного счётчика
`totalActiveSize` в индексе. Это фундамент для всех остальных изменений.

### Подпункты

- [x] **1.1 Добавить `MaxCapacity` в Config**
  - **Dependencies**: None
  - **Description**: Добавить поле `MaxCapacity int64` в структуру `Config`.
    Добавить парсинг обязательной env-переменной `SE_MAX_CAPACITY` (int64, >0).
    Добавить валидацию: `MaxCapacity >= MaxFileSize`.
    Добавить helper-функцию `getEnvInt64Required()`.
  - **Файлы**:
    - `src/storage-element/internal/config/config.go` — модификация
  - **Детали реализации**:
    ```go
    // Новое поле в Config (после MaxFileSize, строка 30):
    // Максимальный объём хранилища SE в байтах
    MaxCapacity int64

    // Парсинг в Load() (после блока SE_MAX_FILE_SIZE, после строки 115):
    cfg.MaxCapacity, err = getEnvInt64Required("SE_MAX_CAPACITY")
    // Валидация: > 0, >= MaxFileSize

    // Новая helper-функция:
    func getEnvInt64Required(key string) (int64, error)
    ```

- [x] **1.2 Добавить `totalActiveSize` в Index**
  - **Dependencies**: None (параллельно с 1.1)
  - **Description**: Добавить поле `totalActiveSize int64` в структуру `Index`.
    Модифицировать методы `Add()`, `Update()`, `Remove()`, `BuildFromDir()` для
    корректного обновления счётчика. Добавить метод `TotalActiveSize() int64`.
    Счётчик учитывает только файлы со статусом `active`.
  - **Файлы**:
    - `src/storage-element/internal/storage/index/index.go` — модификация
  - **Детали реализации**:
    ```
    Структура Index: + totalActiveSize int64

    Add(): если existing active — вычесть; если новый active — прибавить
    Update(): вычесть старое active, прибавить новое active
    Remove(): если active — вычесть
    BuildFromDir(): пересчитать с нуля после сканирования
    TotalActiveSize(): вернуть под RLock
    ```

- [x] **1.3 Unit-тесты Index.TotalActiveSize**
  - **Dependencies**: 1.2
  - **Description**: Тесты для корректности кумулятивного счётчика:
    - Пустой индекс → 0
    - Add active файла → увеличение
    - Add deleted/expired файла → без изменения
    - Update active→deleted → уменьшение
    - Update deleted→active → увеличение
    - Remove active файла → уменьшение
    - BuildFromDir → корректный пересчёт
    - Add с перезаписью existing файла → корректная корректировка
  - **Файлы**:
    - `src/storage-element/internal/storage/index/index_test.go` — модификация

### Критерии завершения Phase 1

- [x] Все подпункты завершены (1.1, 1.2, 1.3)
- [x] `go build ./...` успешна
- [x] `go test ./internal/config/...` проходит
- [x] `go test ./internal/storage/index/...` проходит
- [x] `TotalActiveSize()` возвращает корректные значения при всех операциях

---

## Phase 2: Upload проверка и SystemHandler

**Dependencies**: Phase 1
**Status**: Pending

### Описание

Интеграция capacity check в поток загрузки файлов и замена источника данных
для CapacityInfo в API `/api/v1/info`.

### Подпункты

- [ ] **2.1 Проверка capacity в Upload**
  - **Dependencies**: None
  - **Description**: Добавить проверку `TotalActiveSize() + params.Size > MaxCapacity`
    в `UploadService.Upload()` после проверки `MaxFileSize` и **перед** WAL/SaveFile.
    При нехватке места — возвращать `UploadError{507, STORAGE_FULL, ...}`.
  - **Файлы**:
    - `src/storage-element/internal/service/upload.go` — модификация
  - **Детали реализации**:
    ```
    Между строками 118 и 120 (после проверки MaxFileSize, перед fileID):
    if params.Size > 0 && s.idx.TotalActiveSize()+params.Size > s.cfg.MaxCapacity {
        return nil, &UploadError{507, CodeStorageFull, "Недостаточно места..."}
    }

    Примечание: проверка racy (между check и Add другой upload может занять место),
    но это приемлемо — pre-check отсекает 99% случаев.
    ```

- [ ] **2.2 Заменить diskFn в SystemHandler**
  - **Dependencies**: None (параллельно с 2.1)
  - **Description**: Убрать поле `diskFn` из `SystemHandler`.
    Обновить конструктор `NewSystemHandler` — убрать параметр `diskUsageFn`.
    В `GetStorageInfo()` вычислять capacity из `cfg.MaxCapacity` и `idx.TotalActiveSize()`.
    `available = max(0, MaxCapacity - TotalActiveSize)`.
  - **Файлы**:
    - `src/storage-element/internal/api/handlers/system.go` — модификация
  - **Детали реализации**:
    ```
    SystemHandler: убрать поле diskFn
    NewSystemHandler: убрать параметр diskUsageFn
    GetStorageInfo() строки 69-80: заменить блок diskFn на:
      usedBytes := h.idx.TotalActiveSize()
      availableBytes := h.cfg.MaxCapacity - usedBytes
      if availableBytes < 0 { availableBytes = 0 }
      capacity := generated.CapacityInfo{
          TotalBytes: h.cfg.MaxCapacity,
          UsedBytes: usedBytes,
          AvailableBytes: availableBytes,
      }
    ```

- [ ] **2.3 Обновить main.go и удалить disk_usage.go**
  - **Dependencies**: 2.2
  - **Description**: Обновить вызов `NewSystemHandler` — убрать `diskUsageFn`.
    Удалить функцию `diskUsageFn()` из main.go.
    Удалить файл `disk_usage.go` (функция `getDiskUsage()` больше не используется).
    Добавить логирование `max_capacity` при старте.
  - **Файлы**:
    - `src/storage-element/cmd/storage-element/main.go` — модификация
    - `src/storage-element/cmd/storage-element/disk_usage.go` — удаление
  - **Детали реализации**:
    ```
    Строка 235: убрать diskUsageFn(cfg.DataDir) из вызова NewSystemHandler
    Строка 37: добавить slog.Int64("max_capacity", cfg.MaxCapacity)
    Удалить строки 307-311 (функция diskUsageFn)
    Удалить файл disk_usage.go целиком
    ```

### Критерии завершения Phase 2

- [ ] Все подпункты завершены (2.1, 2.2, 2.3)
- [ ] `go build ./...` успешна
- [ ] `go test ./...` проходит
- [ ] Upload файла сверх лимита возвращает 507
- [ ] `/api/v1/info` возвращает `total_bytes = MaxCapacity` (не размер диска)

---

## Phase 3: Helm chart и OpenAPI

**Dependencies**: Phase 2
**Status**: Pending

### Описание

Обновление Helm chart для передачи `SE_MAX_CAPACITY` и обновление OpenAPI спецификации
с корректными описаниями полей `CapacityInfo`.

### Подпункты

- [ ] **3.1 Обновить Helm chart**
  - **Dependencies**: None
  - **Description**: Добавить параметр `maxCapacity` в `values.yaml` (по умолчанию 10 GB).
    Добавить `SE_MAX_CAPACITY` в `_helpers.tpl` (шаблон `se.envVars`).
    Добавить валидацию в `_helpers.tpl`: `maxCapacity` не может быть больше `dataSize`.
  - **Файлы**:
    - `src/storage-element/charts/storage-element/values.yaml` — модификация
    - `src/storage-element/charts/storage-element/templates/_helpers.tpl` — модификация
  - **Детали реализации**:
    ```yaml
    # values.yaml (после maxFileSize):
    maxCapacity: "10737418240"  # 10GB

    # _helpers.tpl (в блоке se.envVars, после SE_MAX_FILE_SIZE):
    - name: SE_MAX_CAPACITY
      value: {{ .Values.maxCapacity | quote }}
    ```

- [ ] **3.2 Обновить OpenAPI спецификацию**
  - **Dependencies**: None (параллельно с 3.1)
  - **Description**: Обновить описания полей `CapacityInfo` в OpenAPI:
    - `total_bytes` — сконфигурированный лимит (SE_MAX_CAPACITY)
    - `used_bytes` — суммарный размер active файлов
    - `available_bytes` — оставшееся место в пределах лимита
  - **Файлы**:
    - `docs/api-contracts/storage-element-openapi.yaml` — модификация

- [ ] **3.3 Обновить документацию**
  - **Dependencies**: None (параллельно с 3.1, 3.2)
  - **Description**: Добавить `SE_MAX_CAPACITY` в таблицу конфигурации в briefs.
  - **Файлы**:
    - `docs/briefs/storage-element.md` — модификация

### Критерии завершения Phase 3

- [ ] Все подпункты завершены (3.1, 3.2, 3.3)
- [ ] `helm template` без ошибок
- [ ] OpenAPI спецификация валидна
- [ ] Документация актуальна

---

## Phase 4: Сборка, деплой и тестирование

**Dependencies**: Phase 1, Phase 2, Phase 3
**Status**: Pending

### Описание

Сборка Docker-образа, пересоздание тестовой среды с нуля (с `SE_MAX_CAPACITY`),
интеграционное тестирование.

### Подпункты

- [ ] **4.1 Сборка Docker-образа**
  - **Dependencies**: None
  - **Description**: Собрать новый образ SE с поддержкой `SE_MAX_CAPACITY`.
    Запушить в Harbor (`harbor.kryukov.lan/library/storage-element`).
  - **Файлы**:
    - Docker image

- [ ] **4.2 Обновить тестовые Helm values**
  - **Dependencies**: 4.1
  - **Description**: Обновить values для тестовых SE (6 штук в `tests/helm/artstore-se/`)
    — добавить `maxCapacity` для каждого экземпляра.
    Пересоздать тестовую среду с нуля (`make test-env-down && make test-env-up`).
  - **Файлы**:
    - `tests/helm/artstore-se/` — модификация values

- [ ] **4.3 Верификация API**
  - **Dependencies**: 4.2
  - **Description**: Проверить что `/api/v1/info` возвращает корректные данные:
    - `total_bytes` = `SE_MAX_CAPACITY` (не размер диска)
    - `used_bytes` = суммарный размер active файлов
    - `available_bytes` = total - used
    Проверить upload файла сверх лимита → 507.
  - **Файлы**:
    - Результаты тестирования (curl)

- [ ] **4.4 Проверка Admin UI**
  - **Dependencies**: 4.3
  - **Description**: Открыть Admin UI, убедиться что карточка SE показывает
    корректный сконфигурированный лимит вместо размера всего диска.
  - **Файлы**: N/A

### Критерии завершения Phase 4

- [ ] Все подпункты завершены (4.1, 4.2, 4.3, 4.4)
- [ ] Docker-образ собран и запушен
- [ ] Тестовая среда пересоздана с `SE_MAX_CAPACITY`
- [ ] `/api/v1/info` показывает корректный лимит
- [ ] Upload сверх лимита отклоняется с 507
- [ ] Admin UI показывает правильные данные
- [ ] `go test ./...` проходит

---

## Архитектура изменений

### Диаграмма потока Upload (после)

```
Client → POST /api/v1/files/upload
           │
           ▼
   ┌─ CanPerform(OpUpload)? ──── NO → 409 MODE_NOT_ALLOWED
   │        YES
   ▼
   ┌─ Size > MaxFileSize? ────── YES → 413 FILE_TOO_LARGE
   │        NO
   ▼
   ┌─ TotalActiveSize + Size     YES → 507 STORAGE_FULL  ← НОВОЕ
   │  > MaxCapacity? ──────────
   │        NO
   ▼
   WAL StartTransaction
   │
   ▼
   SaveFile (streaming + SHA-256)
   │
   ▼
   Write attr.json → idx.Add(metadata) → totalActiveSize += size
   │
   ▼
   WAL Commit → 200 OK
```

### Диаграмма CapacityInfo (после)

```
AM polling → GET /api/v1/info
               │
               ▼
  ┌──────────────────────────────────┐
  │  totalUsed = idx.TotalActiveSize()│
  │  available = MaxCapacity - used   │
  │  if available < 0: available = 0  │
  │                                   │
  │  CapacityInfo {                   │
  │    total:     cfg.MaxCapacity     │
  │    used:      totalUsed           │
  │    available: available           │
  │  }                                │
  └──────────────────────────────────┘
               │
               ▼
  200 OK + StorageInfo
```

### Затронутые файлы

| Файл | Действие | Фаза |
|------|----------|------|
| `src/storage-element/internal/config/config.go` | Модификация | 1 |
| `src/storage-element/internal/storage/index/index.go` | Модификация | 1 |
| `src/storage-element/internal/storage/index/index_test.go` | Модификация | 1 |
| `src/storage-element/internal/service/upload.go` | Модификация | 2 |
| `src/storage-element/internal/api/handlers/system.go` | Модификация | 2 |
| `src/storage-element/cmd/storage-element/main.go` | Модификация | 2 |
| `src/storage-element/cmd/storage-element/disk_usage.go` | Удаление | 2 |
| `src/storage-element/charts/storage-element/values.yaml` | Модификация | 3 |
| `src/storage-element/charts/storage-element/templates/_helpers.tpl` | Модификация | 3 |
| `docs/api-contracts/storage-element-openapi.yaml` | Модификация | 3 |
| `docs/briefs/storage-element.md` | Модификация | 3 |

---

## Примечания

- **Race condition при upload**: Проверка capacity перед записью — racy (другой upload может занять место между check и Add). Это приемлемо: pre-check отсекает 99% случаев, реальная защита — на уровне ФС.
- **GC и счётчик**: GC вызывает `idx.Remove()` при физическом удалении → `totalActiveSize` уменьшается автоматически. Soft delete через `idx.Update()` (status active→deleted) тоже корректно уменьшает счётчик.
- **Reconcile**: `RebuildFromDir()` → `BuildFromDir()` → пересчёт `totalActiveSize` с нуля. Корректно.
- **Тестовая среда**: Пересоздаётся с нуля (нет миграции существующих данных).
- **Helm валидация**: `maxCapacity` не может быть больше `dataSize` (PVC) — логическая проверка.
