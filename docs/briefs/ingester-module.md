# Ingester Module — Бриф модуля

**Версия**: 0.1.0
**Дата**: 2026-03-01
**Статус**: Ready
**Порты**: 8020-8029

---

## 1. Назначение модуля

Ingester Module — точка входа для загрузки и удаления файлов в системе Artstore.
Принимает файлы от клиентов, валидирует, определяет целевой Storage Element,
выполняет upload и регистрирует файл в реестре Admin Module.
Также обеспечивает удаление файлов из SE в режиме `edit` с обновлением реестра.

Ingester — stateless-модуль. Не хранит данные, не имеет собственной БД.
Вся информация о SE и файлах — в Admin Module. Горизонтально масштабируется
через Kubernetes replicas.

### Ключевые концепции

**Retention Policy** — клиент при загрузке указывает политику хранения:

- `temporary` — файл загружается в Edit SE, имеет TTL (1–365 дней,
  default 30). По истечении TTL файл автоматически удаляется GC на SE
- `permanent` — файл загружается в RW SE, хранится бессрочно

Ingester автоматически выбирает подходящий SE на основе retention_policy.

**Sync Upload с Temp-file** — файл сначала сохраняется во временный файл
на emptyDir volume (2Gi), затем передаётся в SE через `io.Pipe` (streaming).
Буферизация в temp-file (а не в RAM) позволяет обрабатывать файлы размером
до 1 GB при ограниченном объёме RAM. Temp-file удаляется после upload
(или при ошибке) через `defer os.Remove(tempPath)`. Seek(0,0) используется
при retry после 507.

**Sequential Fill Algorithm** — Ingester запрашивает у Admin Module список
доступных SE с подходящим режимом, затем выбирает SE по алгоритму
Sequential Fill:

1. Определить mode: `temporary` → `"edit"`, `permanent` → `"rw"`
2. Запросить список SE: `GET /api/v1/storage-elements?mode={mode}&status=online`
3. Отсортировать по `priority ASC`, при равном priority — по `name ASC`
4. Для каждого SE по порядку:
   - Пропустить если ID в excludedSEIDs (после retry)
   - Пропустить если `available_bytes == nil`
   - Проверить `*available_bytes >= fileSize`
   - Вернуть первый подходящий
5. Если нет подходящего → ошибка `NO_STORAGE_AVAILABLE`

Lower priority value = higher priority. Это обеспечивает предсказуемое
последовательное заполнение SE, удобное для мониторинга.

**Retry при 507** — если SE возвращает `507 Insufficient Storage`,
Ingester автоматически:

1. Добавляет SE в excludedSEIDs
2. Перематывает temp-file: `Seek(0, 0)`
3. Повторяет выбор SE и upload (до `IM_MAX_RETRIES=3` попыток)

Если все retry исчерпаны → ошибка `STORAGE_FULL` (507).

**Двухэтапная регистрация** — после успешной загрузки файла в SE,
Ingester регистрирует файл в реестре Admin Module (`POST /api/v1/files`).
Если регистрация не удалась (Admin Module недоступен), файл на SE останется
сиротой и будет обнаружен при следующей синхронизации.

### Диаграммы

- [Upload файла — sequence](../design/im-file-upload-sequence.drawio) —
  полный поток загрузки файла (happy path + 507 retry + error paths)
- [SE Selection — sequence](../design/im-se-selection-sequence.drawio) —
  подробная последовательность алгоритма Sequential Fill
- [Upload — взаимодействие модулей](../design/im-upload-modules.drawio) —
  высокоуровневая диаграмма модулей и направления вызовов

---

## 2. Топология

Ingester Module располагается **внутри кластера Kubernetes**. Входящие
запросы от клиентов проходят через Envoy Gateway (TLS termination, CORS).
Исходящие запросы к Storage Elements — через WAN, TLS.

```text
                    Envoy Gateway (TLS, CORS)
                           │
┌──────────────────────────┼───────────────────────────┐
│          Kubernetes кластер                          │
│                          │                           │
│                  ┌───────▼───────┐                   │
│                  │   Ingester    │                   │
│                  │   Module      │                   │
│                  └───┬───────┬───┘                   │
│                      │       │                       │
│        ┌─────────────┘       └──────────┐            │
│        ▼                                ▼            │
│  ┌──────────────┐              ┌──────────────┐      │
│  │ Admin Module │              │  Keycloak    │      │
│  │ (SE list,    │              │  (token      │      │
│  │  file        │              │   endpoint)  │      │
│  │  registry)   │              │              │      │
│  └──────────────┘              └──────────────┘      │
│                                                      │
└──────────────────────┬───────────────────────────────┘
                       │  WAN / TLS
                       ▼
                 ┌──────────┐    ┌──────────┐
                 │ SE #1    │    │ SE #2    │
                 │ (edit)   │    │ (rw)     │
                 └──────────┘    └──────────┘
```

**Следствия:**

- Ingester не обращается к PostgreSQL напрямую — только через Admin Module API
- SA token получается напрямую от Keycloak (client_credentials grant, `IM_TOKEN_URL`)
- Входящий трафик: plain HTTP (TLS terminates на Envoy Gateway)
- Исходящий к SE: TLS (SE remote, потенциально WAN)
- Ingester должен доверять TLS-сертификатам SE (CA bundle)
- Горизонтальное масштабирование: несколько реплик за Kubernetes Service
- Gateway prefix: `/upload` (HTTPRoute strip prefix → `/api/v1/*`)

---

## 3. Зависимости

### Инфраструктурные

| Зависимость | Назначение |
|-------------|------------|
| — | Ingester не имеет собственных инфраструктурных зависимостей (stateless) |

Ingester не использует PostgreSQL, Redis или файловую систему для хранения
данных. Все данные передаются транзитом. Temp-файлы размещаются в emptyDir.

### Межмодульные

| Модуль | Направление | Назначение |
|--------|-------------|------------|
| Keycloak | Ingester → Keycloak | SA token (`POST /realms/artstore/.../token`, client_credentials) |
| Keycloak | Ingester → Keycloak | JWKS keys (`GET /.../certs`, кэшируются ~15 сек) |
| Admin Module | Ingester → Admin | Список SE (`GET /api/v1/storage-elements?mode=...&status=online`) |
| Admin Module | Ingester → Admin | Регистрация файла (`POST /api/v1/files`) |
| Admin Module | Ingester → Admin | Получение SE (`GET /api/v1/storage-elements/{id}`) |
| Admin Module | Ingester → Admin | Удаление файла из реестра (`DELETE /api/v1/files/{id}`) |
| Storage Element | Ingester → SE | Загрузка файла (`POST /api/v1/files/upload`) |
| Storage Element | Ingester → SE | Удаление файла (`DELETE /api/v1/files/{id}`) |

---

## 4. Workflow загрузки

### Успешный сценарий

```text
Клиент                    Ingester              Admin Module           Storage Element
  │                          │                       │                       │
  │  POST /files/upload      │                       │                       │
  │  (file + metadata)       │                       │                       │
  │─────────────────────────▶│                       │                       │
  │                          │                       │                       │
  │                          │  1. Валидация params   │                       │
  │                          │  2. Сохранение в       │                       │
  │                          │     temp-file          │                       │
  │                          │                       │                       │
  │                          │  GET /storage-elements │                       │
  │                          │  ?mode=edit&status=    │                       │
  │                          │   online               │                       │
  │                          │──────────────────────▶│                       │
  │                          │  [{id, url, priority, │                       │
  │                          │    available_bytes}]   │                       │
  │                          │◀──────────────────────│                       │
  │                          │                       │                       │
  │                          │  3. Sequential Fill    │                       │
  │                          │     (sort by priority) │                       │
  │                          │                       │                       │
  │                          │                  POST /api/v1/files/upload     │
  │                          │  (streaming file)     │                       │
  │                          │──────────────────────────────────────────────▶│
  │                          │                  {file_id, checksum, size}     │
  │                          │◀──────────────────────────────────────────────│
  │                          │                       │                       │
  │                          │  POST /files           │                       │
  │                          │  (register file)       │                       │
  │                          │──────────────────────▶│                       │
  │                          │  {file_id, status}     │                       │
  │                          │◀──────────────────────│                       │
  │                          │                       │                       │
  │  201 UploadResponse      │                       │                       │
  │◀─────────────────────────│                       │                       │
```

### Retry при 507

```text
  │                          │  POST /api/v1/files/upload                     │
  │                          │──────────────────────────────────────────────▶│
  │                          │  507 Insufficient Storage                     │
  │                          │◀──────────────────────────────────────────────│
  │                          │                       │                       │
  │                          │  Exclude SE, Seek(0,0)│                       │
  │                          │  SelectSE (retry)     │                       │
  │                          │──────────────────────▶│  (новый SE)           │
  │                          │──────────────────────────────────────────────▶│
  │                          │  201 OK               │                       │
  │                          │◀──────────────────────────────────────────────│
```

### Обработка ошибок

| Этап | Ошибка | Реакция Ingester |
|------|--------|------------------|
| Валидация | Файл отсутствует, невалидные параметры | 400 VALIDATION_ERROR |
| Валидация | Размер файла превышает лимит | 413 FILE_TOO_LARGE |
| Выбор SE | Admin Module недоступен | 502 ADMIN_UNAVAILABLE |
| Выбор SE | Нет подходящих SE (нет edit/rw SE online) | 502 NO_STORAGE_AVAILABLE |
| Upload в SE | SE вернул 507, все retry исчерпаны | 507 STORAGE_FULL |
| Upload в SE | SE недоступен или вернул другую ошибку | 502 SE_UPLOAD_FAILED |
| Регистрация | Admin Module вернул ошибку | 502 ADMIN_UNAVAILABLE (файл на SE — сирота) |

При ошибке на этапе upload в SE — клиент может повторить запрос (идемпотентность
обеспечивается тем, что новый upload создаёт новый file_id).

---

## 4.1. Workflow удаления

Удаление файлов доступно только для SE в режиме `edit` (temporary файлы).
Ingester проксирует запрос: получает URL SE из Admin Module, выполняет
soft-delete на SE, затем обновляет реестр Admin Module.

### Успешный сценарий

```text
Клиент                    Ingester              Admin Module           Storage Element
  │                          │                       │                       │
  │  DELETE /files/{id}      │                       │                       │
  │  ?storage_element_id=    │                       │                       │
  │─────────────────────────▶│                       │                       │
  │                          │                       │                       │
  │                          │  GET /storage-elements │                       │
  │                          │  /{se_id}              │                       │
  │                          │──────────────────────▶│                       │
  │                          │  {id, url, mode, ...}  │                       │
  │                          │◀──────────────────────│                       │
  │                          │                       │                       │
  │                          │                  DELETE /api/v1/files/{id}     │
  │                          │──────────────────────────────────────────────▶│
  │                          │                  204 No Content               │
  │                          │◀──────────────────────────────────────────────│
  │                          │                       │                       │
  │                          │  DELETE /files/{id}    │                       │
  │                          │  (обновить реестр)     │                       │
  │                          │──────────────────────▶│                       │
  │                          │  204 / 200             │                       │
  │                          │◀──────────────────────│                       │
  │                          │                       │                       │
  │  204 No Content          │                       │                       │
  │◀─────────────────────────│                       │                       │
```

### Обработка ошибок удаления

| Этап | Ошибка | Код | Реакция Ingester |
|------|--------|-----|------------------|
| Валидация | Отсутствует storage_element_id | 400 | VALIDATION_ERROR |
| Получение SE | SE не найден в реестре AM | 502 | SE_NOT_FOUND |
| Удаление из SE | Файл не найден на SE | 404 | FILE_NOT_FOUND |
| Удаление из SE | SE не в режиме edit | 409 | MODE_NOT_ALLOWED |
| Удаление из SE | Файл в процессе загрузки | 409 | FILE_UPLOAD_IN_PROGRESS |
| Удаление из SE | SE недоступен / другая ошибка | 502 | SE_DELETE_FAILED |
| Обновление реестра | AM вернул ошибку | — | Warning в логах (файл уже удалён с SE) |

Примечание: если файл удалён с SE, но обновление реестра AM не удалось,
Ingester логирует предупреждение и возвращает 204 (файл физически удалён).

---

## 5. API endpoints

5 endpoints. Полная спецификация —
[ingester-module-openapi.yaml](../api-contracts/ingester-module-openapi.yaml).

### Upload (1 endpoint)

| Метод | Endpoint | Назначение | Аутентификация |
|-------|----------|------------|----------------|
| `POST` | `/api/v1/files/upload` | Загрузка файла (multipart/form-data) | JWT `files:write` или роль `admin` |

**Параметры запроса (multipart/form-data):**

| Параметр | Тип | Обязательный | По умолчанию | Описание |
|----------|-----|:------------:|--------------|----------|
| `file` | binary | да | — | Загружаемый файл |
| `description` | string | нет | null | Описание (max 1000 символов) |
| `tags` | string (JSON) | нет | null | JSON-массив тегов (`'["logo", "draft"]'`) |
| `retention_policy` | string | нет | `temporary` | `temporary` или `permanent` |
| `ttl_days` | integer | нет | `30` | TTL в днях (1–365, только для temporary) |

**Ответ (UploadResponse):**

| Поле | Тип | Описание |
|------|-----|----------|
| `file_id` | UUID | Уникальный идентификатор файла |
| `original_filename` | string | Оригинальное имя файла |
| `content_type` | string | MIME-тип (определяется автоматически) |
| `size` | int64 | Размер в байтах |
| `checksum` | string | SHA-256 хэш |
| `uploaded_by` | string | Идентификатор загрузившего (из JWT `sub`) |
| `uploaded_at` | datetime | Дата загрузки (RFC 3339, UTC) |
| `description` | string/null | Описание файла |
| `tags` | string[] | Теги |
| `status` | string | Всегда `active` при загрузке |
| `retention_policy` | string | `temporary` или `permanent` |
| `ttl_days` | int/null | TTL в днях (только для temporary) |
| `expires_at` | datetime/null | Дата истечения (только для temporary) |
| `storage_element_id` | UUID | ID записи SE в реестре Admin Module |

### Delete (1 endpoint)

| Метод | Endpoint | Назначение | Аутентификация |
|-------|----------|------------|----------------|
| `DELETE` | `/api/v1/files/{file_id}` | Удаление файла из SE и реестра | JWT `files:write` или роль `admin` |

**Query параметры:**

| Параметр | Тип | Обязательный | Описание |
|----------|-----|:------------:|----------|
| `storage_element_id` | UUID | да | ID Storage Element, на котором хранится файл |

**Ответ:** `204 No Content` (без тела).

**Коды ошибок:**

| Код HTTP | Код ошибки | Описание |
|----------|-----------|----------|
| 400 | `VALIDATION_ERROR` | Отсутствует storage_element_id или невалидный UUID |
| 404 | `FILE_NOT_FOUND` | Файл не найден на SE |
| 409 | `MODE_NOT_ALLOWED` | SE не в режиме edit |
| 409 | `FILE_UPLOAD_IN_PROGRESS` | Файл в процессе загрузки |
| 502 | `SE_NOT_FOUND` | Storage Element не найден в реестре AM |
| 502 | `SE_DELETE_FAILED` | Ошибка при удалении файла на SE |

### Health (3 endpoints)

| Метод | Endpoint | Назначение | Аутентификация |
|-------|----------|------------|----------------|
| `GET` | `/health/live` | Liveness probe (процесс жив) | без аутентификации |
| `GET` | `/health/ready` | Readiness probe (Admin Module, JWKS) | без аутентификации |
| `GET` | `/metrics` | Prometheus metrics | без аутентификации |

**Readiness checks:**

| Проверка | Описание | Влияние |
|----------|----------|---------|
| `admin_module` | Admin Module доступен (`/health/ready`) | `fail` → весь Ingester fail |
| `jwks` | JWKS endpoint Keycloak доступен | `fail` → весь Ingester fail |

Статусы: `ok` (200), `degraded` (200), `fail` (503).

---

## 6. Аутентификация

JWT RS256 токены, выданные Keycloak.

**Публичные endpoints** (без аутентификации):

- `/health/live`, `/health/ready`, `/metrics` — Kubernetes probes и мониторинг

**Защищённые endpoints** — требуют JWT Bearer token:

| Scope / Role | Операции |
|-------------|----------|
| SA `files:write` | Загрузка и удаление файлов |
| Роль `admin` | Загрузка и удаление файлов |

Валидация JWT:

- Алгоритм: RS256
- Публичный ключ: получается через JWKS endpoint Keycloak
- Claims: `sub` (идентификатор субъекта), `scopes` (массив) или `role` (строка)

### Собственный Service Account

Ingester сам является клиентом Keycloak. Для обращения к API Admin Module
(список SE, регистрация файлов) и Storage Elements (upload) Ingester
использует собственный Service Account с scopes `storage:read` + `files:write` +
`files:read`.

Keycloak client: `artstore-ingester` (client_credentials grant).
Credentials SA (`IM_CLIENT_ID`, `IM_CLIENT_SECRET`) передаются через env-переменные.
SA token получается напрямую от Keycloak (`IM_TOKEN_URL`), кэшируется
с double-check locking, обновляется за 30 секунд до истечения TTL.

Важно: Keycloak client `artstore-ingester` должен иметь `client_id`
protocolMapper (oidc-usersessionmodel-note-mapper) для корректного
распознавания SA в Admin Module.

---

## 7. Конфигурация

Все параметры задаются через переменные окружения с префиксом `IM_`.

### Сервер

| Переменная | Обязательная | По умолчанию | Описание |
|------------|:------------:|--------------|----------|
| `IM_PORT` | нет | `8020` | Порт HTTP-сервера (диапазон 8020-8029) |
| `IM_LOG_LEVEL` | нет | `info` | Уровень логирования (`debug`, `info`, `warn`, `error`) |
| `IM_LOG_FORMAT` | нет | `json` | Формат логов (`json` — production, `text` — development) |

### JWT / JWKS

| Переменная | Обязательная | По умолчанию | Описание |
|------------|:------------:|--------------|----------|
| `IM_JWKS_URL` | да | — | URL JWKS endpoint Keycloak для валидации входящих JWT |
| `IM_JWT_ISSUER` | нет | `""` | Ожидаемый issuer в JWT |
| `IM_JWKS_REFRESH_INTERVAL` | нет | `15s` | Интервал обновления JWKS ключей |
| `IM_JWT_LEEWAY` | нет | `5s` | Допуск при проверке exp/nbf |

### Admin Module

| Переменная | Обязательная | По умолчанию | Описание |
|------------|:------------:|--------------|----------|
| `IM_ADMIN_URL` | да | — | Базовый URL Admin Module (`http://admin-module:8000`) |
| `IM_ADMIN_TIMEOUT` | нет | `10s` | Таймаут запросов к Admin Module |

### Keycloak OAuth2 (SA)

| Переменная | Обязательная | По умолчанию | Описание |
|------------|:------------:|--------------|----------|
| `IM_CLIENT_ID` | да | — | client_id SA для обращения к AM и SE |
| `IM_CLIENT_SECRET` | да | — | client_secret SA |
| `IM_TOKEN_URL` | нет | `""` | URL Keycloak token endpoint (прямой) |

### Загрузка файлов

| Переменная | Обязательная | По умолчанию | Описание |
|------------|:------------:|--------------|----------|
| `IM_MAX_FILE_SIZE` | нет | `1073741824` | Максимальный размер файла в байтах (1 GB) |
| `IM_DEFAULT_TTL_DAYS` | нет | `30` | TTL по умолчанию для temporary файлов (дни) |
| `IM_MAX_RETRIES` | нет | `3` | Максимальное количество retry при 507 |

### SE Upload

| Переменная | Обязательная | По умолчанию | Описание |
|------------|:------------:|--------------|----------|
| `IM_SE_UPLOAD_TIMEOUT` | нет | `10m` | Таймаут загрузки файла в SE |
| `IM_SE_CA_CERT_PATH` | нет | `""` | Путь к CA-сертификату для TLS-соединений с SE |

### TLS

| Переменная | Обязательная | По умолчанию | Описание |
|------------|:------------:|--------------|----------|
| `IM_CA_CERT_PATH` | нет | `""` | Путь к CA-сертификату для общих TLS-соединений |

### HTTP сервер

| Переменная | Обязательная | По умолчанию | Описание |
|------------|:------------:|--------------|----------|
| `IM_HTTP_READ_TIMEOUT` | нет | `30s` | Таймаут чтения запроса |
| `IM_HTTP_WRITE_TIMEOUT` | нет | `600s` | Таймаут записи ответа (увеличен для upload) |
| `IM_HTTP_IDLE_TIMEOUT` | нет | `120s` | Таймаут idle соединения |
| `IM_SHUTDOWN_TIMEOUT` | нет | `10s` | Таймаут graceful shutdown |

### HTTP клиент

| Переменная | Обязательная | По умолчанию | Описание |
|------------|:------------:|--------------|----------|
| `IM_HTTP_CLIENT_TIMEOUT` | нет | `30s` | Общий таймаут для HTTP-клиентов |

### Topologymetrics

| Переменная | Обязательная | По умолчанию | Описание |
|------------|:------------:|--------------|----------|
| `IM_DEPHEALTH_CHECK_INTERVAL` | нет | `15s` | Интервал проверки зависимостей |
| `IM_DEPHEALTH_GROUP` | нет | `""` | Группа для topologymetrics |
| `DEPHEALTH_NAME` | нет | `ingester-module` | Имя сервиса (без IM_ префикса) |
| `DEPHEALTH_ISENTRY` | нет | `false` | Является ли точкой входа (без IM_ префикса) |

### RBAC

| Переменная | Обязательная | По умолчанию | Описание |
|------------|:------------:|--------------|----------|
| `IM_ROLE_ADMIN_GROUPS` | нет | `artstore-admins` | Keycloak группы для роли admin |
| `IM_ROLE_READONLY_GROUPS` | нет | `artstore-viewers` | Keycloak группы для роли readonly |

---

## 8. Метрики Prometheus

### HTTP метрики (middleware)

| Метрика | Тип | Labels | Описание |
|---------|-----|--------|----------|
| `im_http_requests_total` | counter | `method`, `path`, `status` | Общее количество HTTP-запросов |
| `im_http_request_duration_seconds` | histogram | `method`, `path` | Время обработки HTTP-запросов |

### Бизнес-метрики (upload)

| Метрика | Тип | Labels | Описание |
|---------|-----|--------|----------|
| `im_uploads_total` | counter | `retention_policy`, `status` | Общее количество загрузок |
| `im_upload_duration_seconds` | histogram | `retention_policy` | Общее время загрузки (клиент → ответ) |
| `im_se_upload_duration_seconds` | histogram | — | Время загрузки в SE |
| `im_upload_size_bytes` | histogram | `retention_policy` | Размер загруженных файлов |
| `im_active_uploads` | gauge | — | Количество активных загрузок |
| `im_retry_total` | counter | `reason` | Количество retry (reason=507\|se_error) |
| `im_se_selection_total` | counter | `result` | Результат выбора SE (success\|no_storage) |

### Бизнес-метрики (delete)

| Метрика | Тип | Labels | Описание |
|---------|-----|--------|----------|
| `im_deletes_total` | counter | `status` | Общее количество удалений (`success`, `error`) |
| `im_delete_duration_seconds` | histogram | — | Время выполнения удаления (SE + AM) |

### Topologymetrics

| Метрика | Тип | Описание |
|---------|-----|----------|
| `app_dependency_health` | Gauge | 1 = доступен, 0 = недоступен |
| `app_dependency_latency_seconds` | Histogram | Время проверки |
| `app_dependency_status` | Gauge | Категория результата (ok, timeout, error...) |
| `app_dependency_status_detail` | Gauge | Детальная причина |

---

## 9. Сборка и запуск

### Docker

```bash
# Сборка образа
cd src/ingester-module
make docker-build VERSION=v0.1.0

# Или напрямую
docker build -t harbor.kryukov.lan/library/ingester-module:v0.1.0 \
  --build-arg VERSION=v0.1.0 \
  -f Dockerfile .
```

### Kubernetes (тестовое окружение)

```bash
# Из директории tests/
make docker-build-im IM_TAG=v0.1.0-2
make docker-push-im IM_TAG=v0.1.0-2
make apps-up IM_TAG=v0.1.0-2
```

### Kubernetes (production Helm chart)

```bash
helm install ingester ./charts/ingester-module \
  --set image.tag=v0.1.0 \
  --set config.adminUrl=http://admin-module:8000 \
  --set config.jwksUrl=https://keycloak.example.com/realms/artstore/.../certs \
  --set config.tokenUrl=https://keycloak.example.com/realms/artstore/.../token \
  --set secrets.clientId=artstore-ingester \
  --set secrets.clientSecret=<secret>
```

---

## 10. Мониторинг зависимостей (topologymetrics)

Ingester Module интегрируется с SDK
[topologymetrics](https://github.com/BigKAA/topologymetrics)
для мониторинга здоровья внешних зависимостей через Prometheus-метрики.

### 10.1. Отслеживаемые зависимости

| Зависимость | Тип проверки | Критичность |
|-------------|-------------|:-----------:|
| Admin Module | HTTP (GET `/health/ready`) | да |

Примечание: SE не отслеживаются через topologymetrics — они определяются
на лету при каждом upload. PostgreSQL не используется (stateless).

### 10.2. Интеграция в коде

```go
import (
    "github.com/BigKAA/topologymetrics/sdk-go/dephealth"
    _ "github.com/BigKAA/topologymetrics/sdk-go/dephealth/checks/httpcheck"
)

dh, err := dephealth.New(cfg.DephealthName, cfg.DephealthGroup,
    dephealth.WithCheckInterval(cfg.DephealthCheckInterval),
    dephealth.HTTP("admin-module",
        dephealth.FromURL(cfg.AdminURL + "/health/ready"),
        dephealth.Critical(true),
    ),
)
dh.Start(ctx)
defer dh.Stop()
```

---

## 11. Порты

| Порт | Назначение |
|------|------------|
| 8020-8029 | HTTP API Ingester Module (по одному порту на экземпляр) |

По умолчанию используется порт `8020`. При горизонтальном масштабировании
все реплики работают на одном порту, балансировка через Kubernetes Service.

---

## 12. Keycloak

| Параметр | Значение |
|----------|----------|
| Client ID | `artstore-ingester` |
| Grant type | `client_credentials` |
| Default scopes | `files:read`, `files:write`, `storage:read` |
| Service Account | да |
| Protocol Mapper | `client_id` (oidc-usersessionmodel-note-mapper) |
