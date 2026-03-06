# План разработки: Удаление статусов файлов (Hard Delete)

## Метаданные

- **Версия плана**: 1.0.0
- **Дата создания**: 2026-03-04
- **Последнее обновление**: 2026-03-04
- **Статус**: Pending

---

## История версий

- **v1.0.0** (2026-03-04): Начальная версия плана

---

## Текущий статус

- **Активная фаза**: Phase 7
- **Активный подпункт**: 7.1
- **Последнее обновление**: 2026-03-06
- **Примечание**: Phases 1-6 завершены

---

## Философия изменений

**Файл либо есть, либо нет.** Никаких промежуточных состояний (`active`, `deleted`, `expired`).

- Удаление файла — hard delete (физически с SE + запись из AM)
- GC при истечении TTL — сразу физическое удаление (без промежуточного `expired`)
- Архивность — свойство SE mode (`ar`), не статус файла
- TTL метаданные (`retention_policy`, `ttl_days`, `expires_at`) сохраняются для информации

---

## Оглавление

- [x] [Phase 1: OpenAPI контракты](#phase-1-openapi-контракты)
- [x] [Phase 2: Storage Element](#phase-2-storage-element)
- [x] [Phase 3: Admin Module](#phase-3-admin-module)
- [x] [Phase 4: Query Module](#phase-4-query-module)
- [x] [Phase 5: Ingester Module](#phase-5-ingester-module)
- [x] [Phase 6: Demo Client](#phase-6-demo-client)
- [ ] [Phase 7: Интеграционные тесты и сборка](#phase-7-интеграционные-тесты-и-сборка)

---

## Phase 1: OpenAPI контракты

**Dependencies**: None
**Status**: Pending

### Описание

Обновить OpenAPI спецификации всех модулей: убрать `FileStatus` enum, параметры фильтрации по статусу, обновить описания операций DELETE (hard delete вместо soft delete). Это фундамент — кодогенерация во всех модулях зависит от контрактов.

### Подпункты

- [x] **1.1 Storage Element OpenAPI**
  - **Dependencies**: None
  - **Description**:
    - Убрать `FileMetadata.status` поле (enum `active/deleted/expired`)
    - Убрать query-параметр `?status=` из `GET /api/v1/files`
    - Обновить описание `DELETE /api/v1/files/{file_id}` — физическое удаление (не soft delete)
    - Обновить описание GC: однофазный (без `expired`)
    - Убрать `status` из примеров метрик (`se_files_total{status=...}` → `se_files_total`)
    - Сохранить `retention_policy`, `ttl_days`, `expires_at` в `FileMetadata`
  - **Modifies**:
    - `docs/api-contracts/storage-element-openapi.yaml`

- [x] **1.2 Admin Module OpenAPI**
  - **Dependencies**: None
  - **Description**:
    - Убрать `FileRecord.status` поле (enum `active/deleted/expired`)
    - Убрать `FileRecordUpdate.status`
    - Убрать query-параметр `?status=` из `GET /api/v1/files`
    - Обновить описание `DELETE /api/v1/files/{file_id}` — физическое удаление записи из БД
    - Убрать метрику `files_marked_deleted`
    - Сохранить `retention_policy`, `ttl_days`, `expires_at`
  - **Modifies**:
    - `docs/api-contracts/admin-module-openapi.yaml`

- [x] **1.3 Query Module OpenAPI**
  - **Dependencies**: None
  - **Description**:
    - Убрать `FileMetadata.status`, `SearchResultItem.status`
    - Убрать `SearchRequest.status` (фильтр по статусу)
    - Убрать `status` из query-параметров поиска
    - Обновить описание lazy cleanup: вызов AM DELETE вместо soft delete
    - Сохранить `retention_policy`, `ttl_days`, `expires_at` в результатах поиска
  - **Modifies**:
    - `docs/api-contracts/query-module-openapi.yaml`

- [x] **1.4 Ingester Module OpenAPI**
  - **Dependencies**: None
  - **Description**:
    - Убрать `UploadResponse.status` (всегда был `active`, теперь не нужен)
    - Обновить описание delete endpoint (hard delete через SE + AM)
  - **Modifies**:
    - `docs/api-contracts/ingester-module-openapi.yaml`

### Критерии завершения Phase 1

- [x] Все подпункты завершены (1.1–1.4)
- [x] Нет ссылок на `active`, `deleted`, `expired` как статусы файлов
- [x] `retention_policy`, `ttl_days`, `expires_at` сохранены во всех контрактах
- [x] Контракты валидны (lint пройден)

---

## Phase 2: Storage Element

**Dependencies**: Phase 1
**Status**: Pending

### Описание

Рефакторинг SE: убрать `FileStatus`, переделать GC на однофазный, `DELETE` — физическое удаление. Это самый большой объём изменений, т.к. SE — основа хранения.

### Подпункты

- [x] **2.1 Кодогенерация и доменная модель**
  - **Dependencies**: None
  - **Description**:
    - Перегенерировать `types.gen.go`, `server.gen.go` из обновлённого OpenAPI
    - Убрать `FileStatus` type и константы (`StatusActive`, `StatusDeleted`, `StatusExpired`) из `internal/domain/model/metadata.go`
    - Убрать поле `Status` из `FileMetadata` struct
    - Убрать методы `IsExpired()`, `IsActive()` — заменить на проверку наличия файла
    - Адаптировать сериализацию/десериализацию attr.json (обратная совместимость: игнорировать `status` при чтении старых attr.json)
  - **Modifies**:
    - `src/storage-element/internal/api/generated/types.gen.go`
    - `src/storage-element/internal/api/generated/server.gen.go`
    - `src/storage-element/internal/domain/model/metadata.go`

- [x] **2.2 In-memory индекс**
  - **Dependencies**: 2.1
  - **Description**:
    - Убрать `statusFilter` из `List()` метода
    - Убрать `CountByStatus()` — заменить на `Count()` (общее количество)
    - Упростить `Add()`, `Update()`, `Remove()` — убрать логику подсчёта по статусу
    - `totalActiveSize` → `totalSize` (все файлы = "активные")
  - **Modifies**:
    - `src/storage-element/internal/storage/index/index.go`

- [x] **2.3 GC-сервис (однофазный)**
  - **Dependencies**: 2.1, 2.2
  - **Description**:
    - Убрать Phase 1 (маркировка `active → expired`)
    - Убрать Phase 2 (удаление `deleted`/`expired`)
    - Новая логика: сканировать файлы с `retention_policy=temporary` и `expires_at < now` → физическое удаление (файл + attr.json + индекс)
    - Обновить метрики: убрать `se_gc_files_expired_total`, оставить `se_gc_files_deleted_total` (переименовать в `se_gc_files_removed_total`?)
    - WAL: операция удаления через WAL для атомарности
  - **Modifies**:
    - `src/storage-element/internal/service/gc.go`

- [x] **2.4 HTTP-обработчик файлов**
  - **Dependencies**: 2.1, 2.2
  - **Description**:
    - `ListFiles`: убрать фильтр по статусу из query-параметров
    - `GetFile` / `DownloadFile`: убрать проверку `status != active` — файл есть = доступен
    - `DeleteFile`: физическое удаление файла с диска + attr.json + WAL + убрать из индекса (вместо `status = deleted`)
    - `UpdateMetadata`: убрать проверку `status != active`
  - **Modifies**:
    - `src/storage-element/internal/api/handlers/files.go`

- [x] **2.5 Метрики и инициализация**
  - **Dependencies**: 2.1, 2.2
  - **Description**:
    - `main.go`: убрать инициализацию `FilesTotal.WithLabelValues("active/deleted/expired")` → одна метрика `FilesTotal` без label status
    - Обновить middleware метрик
    - Reconcile: адаптировать к отсутствию статуса
  - **Modifies**:
    - `src/storage-element/cmd/storage-element/main.go`
    - `src/storage-element/internal/api/middleware/` (метрики)
    - `src/storage-element/internal/service/reconcile.go` (если есть)

- [x] **2.6 Unit-тесты SE**
  - **Dependencies**: 2.1–2.5
  - **Description**:
    - Обновить все тесты: убрать assertions на `status`
    - Тестировать hard delete (файл и attr.json удалены с диска)
    - Тестировать однофазный GC (TTL истёк → файл удалён)
    - Тестировать обратную совместимость чтения старых attr.json со `status`
  - **Modifies**:
    - `src/storage-element/internal/` (все `*_test.go`)

### Критерии завершения Phase 2

- [x] Все подпункты завершены (2.1–2.6)
- [x] `go build ./...` и `go test ./...` проходят без ошибок
- [x] Нет ссылок на `StatusActive`, `StatusDeleted`, `StatusExpired`
- [x] GC однофазный: TTL → удаление
- [x] DELETE endpoint физически удаляет файлы

---

## Phase 3: Admin Module

**Dependencies**: Phase 1, Phase 2
**Status**: Done

### Описание

Рефакторинг AM: миграция БД (убрать `status`), hard delete из PostgreSQL, обновление sync-логики, обновление UI.

### Подпункты

- [x] **3.1 Кодогенерация**
  - **Dependencies**: None
  - **Description**:
    - Перегенерировать `types.gen.go`, `server.gen.go` из обновлённого OpenAPI
  - **Modifies**:
    - `src/admin-module/internal/api/generated/types.gen.go`
    - `src/admin-module/internal/api/generated/server.gen.go`

- [x] **3.2 Миграция БД**
  - **Dependencies**: None
  - **Description**:
    - Новая миграция: `DROP COLUMN status` из таблицы `files`
    - Убрать CHECK constraint `status IN ('active', 'deleted', 'expired')`
    - Убрать индекс по `status`
    - Сохранить `retention_policy`, `ttl_days`, `expires_at` колонки
  - **Creates**:
    - `src/admin-module/internal/database/migrations/NNN_remove_file_status.up.sql`
    - `src/admin-module/internal/database/migrations/NNN_remove_file_status.down.sql`

- [x] **3.3 Доменная модель и репозиторий**
  - **Dependencies**: 3.1, 3.2
  - **Description**:
    - Убрать `Status` поле из `model.File`
    - Убрать `Status` из `FileListFilters`
    - `Delete()` → `DELETE FROM files WHERE file_id = $1` (hard delete, не UPDATE status)
    - `MarkDeletedExcept()` → `DeleteExcept()` — `DELETE FROM files WHERE se_id = $1 AND file_id != ALL($2)`
    - Убрать `statusActive` константу из service
    - Убрать `status` из SQL INSERT и SELECT запросов
  - **Modifies**:
    - `src/admin-module/internal/domain/model/file.go`
    - `src/admin-module/internal/repository/file_registry.go`
    - `src/admin-module/internal/service/file_registry.go`

- [x] **3.4 Sync-сервис**
  - **Dependencies**: 3.3
  - **Description**:
    - `storage_sync.go`: `MarkDeletedExcept` → `DeleteExcept` (удалять записи, а не помечать)
    - Обновить метрики sync: убрать `"deleted"` label
  - **Modifies**:
    - `src/admin-module/internal/service/storage_sync.go`

- [x] **3.5 HTTP-обработчики API**
  - **Dependencies**: 3.1, 3.3
  - **Description**:
    - `RegisterFile`: убрать установку `Status: "active"`
    - `ListFiles`: убрать фильтр по статусу
    - `UpdateFile`: убрать `Status` из обновления
    - `DeleteFile`: вызывать hard delete в репозитории
    - Маппинг в API-типы: убрать `Status`
  - **Modifies**:
    - `src/admin-module/internal/api/handlers/files.go`

- [x] **3.6 SE-клиент**
  - **Dependencies**: 3.1
  - **Description**:
    - Убрать `Status` из `SEFileMetadata` struct
  - **Modifies**:
    - `src/admin-module/internal/seclient/client.go`

- [x] **3.7 Admin UI**
  - **Dependencies**: 3.3, 3.5
  - **Description**:
    - Убрать фильтр по статусу из списка файлов (select dropdown)
    - Убрать badge статуса файла (`fileStatusVariant`, `fileEffectiveStatusLabel` и т.д.)
    - Убрать виртуальный `archived` статус — вместо этого показывать информацию о SE mode `ar` как предупреждение
    - Убрать `showDeleted` логику из `buildFilters()`
    - Обновить i18n ключи (убрать `filter.active`, `filter.archived` и т.д.)
  - **Modifies**:
    - `src/admin-module/internal/ui/handlers/files.go`
    - `src/admin-module/internal/ui/pages/file_list.templ`
    - `src/admin-module/internal/ui/pages/partials/file_detail.templ`
    - `src/admin-module/internal/ui/pages/partials/file_table.templ`
    - `src/admin-module/internal/ui/components/badge.templ`
    - `src/admin-module/internal/ui/i18n/locales/en.json`
    - `src/admin-module/internal/ui/i18n/locales/ru.json`

- [x] **3.8 Unit-тесты AM**
  - **Dependencies**: 3.1–3.7
  - **Description**:
    - Обновить тесты репозитория: hard delete
    - Обновить тесты сервисного слоя
    - Обновить тесты sync
  - **Modifies**:
    - `src/admin-module/internal/` (все `*_test.go`)

### Критерии завершения Phase 3

- [x] Все подпункты завершены (3.1–3.8)
- [x] `go build ./...` и `go test ./...` проходят без ошибок
- [x] Миграция БД работает (up и down)
- [x] Admin UI не показывает статусы файлов
- [x] DELETE endpoint физически удаляет записи из PostgreSQL

---

## Phase 4: Query Module

**Dependencies**: Phase 1, Phase 3
**Status**: Done

### Описание

Рефакторинг QM: убрать статус из поиска, lazy cleanup через AM DELETE, обновить миграцию.

### Подпункты

- [x] **4.1 Кодогенерация и миграция**
  - **Dependencies**: None
  - **Description**:
    - Перегенерировать типы из обновлённого OpenAPI
    - Новая миграция: `DROP COLUMN status` из таблицы QM `files` (таблица `schema_migrations_qm`)
  - **Modifies**:
    - `src/query-module/internal/api/generated/types.gen.go`
    - `src/query-module/internal/api/generated/server.gen.go`
  - **Creates**:
    - `src/query-module/internal/database/migrations/NNN_remove_file_status.up.sql`
    - `src/query-module/internal/database/migrations/NNN_remove_file_status.down.sql`

- [x] **4.2 Доменная модель и репозиторий**
  - **Dependencies**: 4.1
  - **Description**:
    - Убрать `Status` из `model.File`
    - Убрать `Status` из `SearchParams`
    - `MarkDeleted()` → `Delete()` — `DELETE FROM files WHERE file_id = $1`
    - Убрать `status` из SQL WHERE в `buildSearchWhere()`
    - Убрать `status` из SQL SELECT
  - **Modifies**:
    - `src/query-module/internal/domain/model/file.go`
    - `src/query-module/internal/repository/file.go`

- [x] **4.3 Сервис download и lazy cleanup**
  - **Dependencies**: 4.2
  - **Description**:
    - Убрать `ErrFileDeleted` — файл либо есть в БД, либо нет
    - Убрать проверку `record.Status == "deleted"` перед скачиванием
    - `lazyCleanup()`: вызвать AM `DELETE /api/v1/files/{id}` + удалить запись из локальной БД + инвалидация кэша
    - Обновить метрику `qm_lazy_cleanup_total`
  - **Modifies**:
    - `src/query-module/internal/service/download.go`

- [x] **4.4 HTTP-обработчики поиска**
  - **Dependencies**: 4.1, 4.2
  - **Description**:
    - Убрать default `status = "active"` из search handler
    - Убрать передачу `status` в `SearchParams`
    - Убрать `Status` из маппинга результатов
  - **Modifies**:
    - `src/query-module/internal/api/handlers/search.go`
    - `src/query-module/internal/api/handlers/files.go`

- [x] **4.5 Admin Client**
  - **Dependencies**: 4.3
  - **Description**:
    - Добавить метод `DeleteFile(ctx, fileID)` в adminclient для вызова AM DELETE API
    - Использовать в lazy cleanup
  - **Modifies**:
    - `src/query-module/internal/adminclient/client.go` (или аналогичный)

- [x] **4.6 Unit-тесты QM**
  - **Dependencies**: 4.1–4.5
  - **Description**:
    - Обновить тесты поиска: без фильтра по статусу
    - Обновить тесты download: без проверки deleted
    - Обновить тесты lazy cleanup: вызов AM DELETE
  - **Modifies**:
    - `src/query-module/internal/` (все `*_test.go`)

### Критерии завершения Phase 4

- [x] Все подпункты завершены (4.1–4.6)
- [x] `go build ./...` и `go test ./...` проходят без ошибок
- [x] Поиск не фильтрует по статусу
- [x] Lazy cleanup вызывает AM DELETE + hard delete из БД

---

## Phase 5: Ingester Module

**Dependencies**: Phase 1, Phase 3
**Status**: Done

### Описание

Рефакторинг IM: убрать статус из ответа upload, обновить delete flow (hard delete через SE + AM).

### Подпункты

- [x] **5.1 Кодогенерация**
  - **Dependencies**: None
  - **Description**:
    - Перегенерировать типы из обновлённого OpenAPI
  - **Modifies**:
    - `src/ingester-module/internal/api/generated/types.gen.go`
    - `src/ingester-module/internal/api/generated/server.gen.go`

- [x] **5.2 Сервис upload**
  - **Dependencies**: 5.1
  - **Description**:
    - Убрать `Status` из ответа upload (поле больше не существует)
    - Сохранить `retention_policy`, `ttl_days` в запросе (они остаются)
  - **Modifies**:
    - `src/ingester-module/internal/service/upload.go`
    - `src/ingester-module/internal/api/handlers/upload.go`

- [x] **5.3 Delete flow**
  - **Dependencies**: 5.1
  - **Description**:
    - При удалении файла: вызвать DELETE на SE (физическое удаление) + DELETE на AM (удаление записи)
    - Оба вызова должны быть выполнены
    - Обработка ошибок: если SE DELETE успешен, а AM DELETE нет — AM sync подхватит при следующей синхронизации
  - **Modifies**:
    - `src/ingester-module/internal/service/` (delete-логика)
    - `src/ingester-module/internal/api/handlers/` (delete handler)

- [x] **5.4 Admin Client**
  - **Dependencies**: 5.1
  - **Description**:
    - Убрать `Status` из `FileRecord` struct в adminclient
  - **Modifies**:
    - `src/ingester-module/internal/adminclient/client.go`

- [x] **5.5 Unit-тесты IM**
  - **Dependencies**: 5.1–5.4
  - **Description**:
    - Обновить тесты upload: без status в ответе
    - Обновить тесты delete: hard delete
  - **Modifies**:
    - `src/ingester-module/internal/` (все `*_test.go`)

### Критерии завершения Phase 5

- [x] Все подпункты завершены (5.1–5.5)
- [x] `go build ./...` и `go test ./...` проходят без ошибок
- [x] Upload ответ без `status`
- [x] Delete — hard delete (SE + AM)

---

## Phase 6: Demo Client

**Dependencies**: Phase 4, Phase 5
**Status**: Done

### Описание

Обновление Demo Client: убрать статусы из UI, упростить dashboard, обновить поиск.

### Подпункты

- [x] **6.1 Gateway-модели**
  - **Dependencies**: None
  - **Description**:
    - Убрать `Status` из `UploadResult`, `SearchRequest`, `FileInfo`
    - Оставить `SEMode` в `FileInfo` для отображения архивности
  - **Modifies**:
    - `src/demo-client/internal/gateway/models.go`

- [x] **6.2 Поиск**
  - **Dependencies**: 6.1
  - **Description**:
    - Убрать dropdown "Активные/Архивные" из фильтров поиска
    - Убрать `Status` из параметров поиска
    - Показывать предупреждение о SE mode `ar` (архивный SE) на карточке файла
  - **Modifies**:
    - `src/demo-client/internal/ui/pages/search.templ`
    - `src/demo-client/internal/ui/handlers/search.go`

- [x] **6.3 Dashboard**
  - **Dependencies**: 6.1
  - **Description**:
    - Убрать 3 отдельных запроса (active/expired/deleted) → 1 запрос (все файлы)
    - Упростить `FileStats`: убрать `Active`, `Expired`, `Deleted` → оставить `Total`
    - Обновить виджеты dashboard
  - **Modifies**:
    - `src/demo-client/internal/service/dashboard.go`
    - `src/demo-client/internal/ui/pages/dashboard.templ`

- [x] **6.4 Компоненты и badge**
  - **Dependencies**: 6.1
  - **Description**:
    - Убрать `FileStatusBadge` (или переделать на badge SE mode)
    - Убрать `BadgeFileActive`, `BadgeFileDeleted`, `BadgeFileExpired`
    - Добавить badge/предупреждение для SE mode `ar` (архивный)
  - **Modifies**:
    - `src/demo-client/internal/ui/components/badge.templ` (или `badge_templ.go`)

- [x] **6.5 i18n**
  - **Dependencies**: 6.2, 6.3, 6.4
  - **Description**:
    - Убрать ключи `filter.active`, `filter.archived`, `filter.all_statuses`
    - Убрать ключи `status.active`, `status.deleted`, `status.expired`
    - Добавить/обновить ключи для SE mode предупреждения
  - **Modifies**:
    - `src/demo-client/internal/ui/i18n/locales/en.json`
    - `src/demo-client/internal/ui/i18n/locales/ru.json`

### Критерии завершения Phase 6

- [x] Все подпункты завершены (6.1–6.5)
- [x] `go build ./...` проходит без ошибок
- [x] UI не показывает статусы файлов
- [x] Dashboard с одним запросом
- [x] Архивность отображается через SE mode

---

## Phase 7: Интеграционные тесты и сборка

**Dependencies**: Phase 2, Phase 3, Phase 4, Phase 5, Phase 6
**Status**: Pending

### Описание

Сборка всех Docker-образов, деплой в K8s, прогон интеграционных тестов. Обновление тестовых скриптов.

### Подпункты

- [ ] **7.1 Обновление интеграционных тестов**
  - **Dependencies**: None
  - **Description**:
    - Обновить bash-скрипты тестов AM: убрать тесты на `status` фильтрацию, soft delete
    - Обновить тесты IM: убрать assertions на `status` в ответах
    - Обновить тесты QM: убрать `status` из параметров поиска
    - Добавить тесты hard delete: файл удалён → GET возвращает 404
    - Добавить тесты: удаление через IM → проверить что файл удалён и на SE, и в AM
  - **Modifies**:
    - `tests/scripts/` (тестовые скрипты)

- [ ] **7.2 Сборка Docker-образов**
  - **Dependencies**: 7.1
  - **Description**:
    - Собрать и опубликовать SE, AM, QM, IM, Demo Client
    - Инкрементировать суффикс тегов
  - **Creates**:
    - Docker images в Harbor

- [ ] **7.3 Деплой и тестирование в K8s**
  - **Dependencies**: 7.2
  - **Description**:
    - `make test-env-up` → `make init-data` → `make test-all`
    - Верифицировать что все интеграционные тесты проходят
    - Проверить миграции БД (AM и QM)
    - Ручная проверка Demo Client UI
  - **Creates**:
    - Результаты тестов

### Критерии завершения Phase 7

- [ ] Все подпункты завершены (7.1–7.3)
- [ ] Все Docker-образы собраны и опубликованы
- [ ] Все интеграционные тесты пройдены
- [ ] Миграции БД применены успешно
- [ ] Demo Client работает корректно

---

## Примечания

- **Обратная совместимость attr.json**: при чтении старых attr.json со `status` полем — игнорировать это поле (не падать)
- **Порядок деплоя**: сначала SE, потом AM (миграция БД), потом QM и IM, потом Demo Client
- **Rollback**: миграции должны иметь down-файлы для отката
- **AM sync safety net**: если SE физически удалил файл, а AM ещё не знает — следующий sync удалит запись из БД. Это страховка на случай сбоя при одновременном удалении SE + AM
