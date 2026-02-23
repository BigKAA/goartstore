# Ingester Module — Бриф модуля

**Версия**: 0.1.0
**Дата**: 2026-02-21
**Статус**: Draft
**Порты**: 8020-8029

---

## 1. Назначение модуля

Ingester Module — точка входа для загрузки файлов в систему Artstore.
Принимает файлы от клиентов, валидирует, определяет целевой Storage Element,
выполняет streaming upload и регистрирует файл в реестре Admin Module.

Ingester — stateless-модуль. Не хранит данные, не имеет собственной БД.
Вся информация о SE и файлах — в Admin Module. Горизонтально масштабируется
через Kubernetes replicas.

### Ключевые концепции

**Retention Policy** — клиент при загрузке указывает политику хранения:

- `temporary` — файл загружается в Edit SE, имеет TTL (1–365 дней,
  default 30). По истечении TTL файл автоматически удаляется GC на SE
- `permanent` — файл загружается в RW SE, хранится бессрочно

Ingester автоматически выбирает подходящий SE на основе retention_policy.

**Streaming Upload** — файл передаётся от клиента в SE потоково (streaming),
без буферизации целиком в памяти Ingester. Это позволяет обрабатывать файлы
значительного размера при ограниченном объёме RAM.

**Выбор Storage Element** — Ingester запрашивает у Admin Module список
доступных SE с подходящим режимом и достаточной ёмкостью, затем выбирает
SE для загрузки. Критерии выбора:

- `retention_policy=temporary` → SE в режиме `edit`, статус `online`
- `retention_policy=permanent` → SE в режиме `rw`, статус `online`
- Достаточно свободного места (`available_bytes >= file size`)

При наличии нескольких подходящих SE — выбирается SE с наибольшим
свободным местом (простая стратегия, без Sequential Fill Algorithm).

**Двухэтапная регистрация** — после успешной загрузки файла в SE,
Ingester регистрирует файл в реестре Admin Module (`POST /api/v1/files`).
Если регистрация не удалась (Admin Module недоступен), файл на SE останется
сиротой и будет обнаружен при следующей синхронизации.

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
│  │ Admin Module │              │  PostgreSQL  │      │
│  │ (JWT, SE     │              │  (shared,    │      │
│  │  list, file  │              │   не прямой  │      │
│  │  registry)   │              │   доступ)    │      │
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
- Входящий трафик: plain HTTP (TLS terminates на Envoy Gateway)
- Исходящий к SE: TLS (SE remote, потенциально WAN)
- Ingester должен доверять TLS-сертификатам SE (CA bundle)
- Горизонтальное масштабирование: несколько реплик за Kubernetes Service

---

## 3. Зависимости

### Инфраструктурные

| Зависимость | Назначение |
|-------------|------------|
| — | Ingester не имеет собственных инфраструктурных зависимостей (stateless) |

Ingester не использует PostgreSQL, Redis или файловую систему для хранения
данных. Все данные передаются транзитом.

### Межмодульные

| Модуль | Направление | Назначение |
|--------|-------------|------------|
| Admin Module | Ingester → Admin | JWT token (`POST /auth/token`), JWKS (`GET /auth/jwks`) |
| Admin Module | Ingester → Admin | Список SE (`GET /storage-elements?mode=...&status=online`) |
| Admin Module | Ingester → Admin | Регистрация файла (`POST /files`) |
| Storage Element | Ingester → SE | Загрузка файла (`POST /api/v1/files/upload`) |
| Admin Module | Admin → Ingester | JWKS endpoint для валидации JWT (если клиент — SA) |

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
  │                          │  GET /storage-elements │                       │
  │                          │  ?mode=edit&status=    │                       │
  │                          │   online               │                       │
  │                          │──────────────────────▶│                       │
  │                          │  [{id, url, mode,     │                       │
  │                          │    available_bytes}]   │                       │
  │                          │◀──────────────────────│                       │
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

### Обработка ошибок

| Этап | Ошибка | Реакция Ingester |
|------|--------|------------------|
| Валидация | Файл отсутствует, невалидные параметры | 400 VALIDATION_ERROR |
| Валидация | Размер файла превышает лимит | 413 FILE_TOO_LARGE |
| Выбор SE | Admin Module недоступен | 502 ADMIN_UNAVAILABLE |
| Выбор SE | Нет подходящих SE (нет edit/rw SE online) | 502 NO_STORAGE_AVAILABLE |
| Выбор SE | На всех SE недостаточно места | 507 STORAGE_FULL |
| Upload в SE | SE недоступен или вернул ошибку | 502 SE_UPLOAD_FAILED |
| Регистрация | Admin Module вернул ошибку | 502 ADMIN_UNAVAILABLE (файл остаётся на SE как сирота) |

При ошибке на этапе upload в SE — клиент может повторить запрос (идемпотентность
обеспечивается тем, что новый upload создаёт новый file_id).

---

## 5. API endpoints

4 endpoints. Полная спецификация —
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

### Health (3 endpoints)

| Метод | Endpoint | Назначение | Аутентификация |
|-------|----------|------------|----------------|
| `GET` | `/health/live` | Liveness probe (процесс жив) | без аутентификации |
| `GET` | `/health/ready` | Readiness probe (Admin Module, SE) | без аутентификации |
| `GET` | `/metrics` | Prometheus metrics | без аутентификации |

**Readiness checks:**

| Проверка | Описание | Влияние |
|----------|----------|---------|
| `admin_module` | Admin Module доступен | `fail` → весь Ingester fail |
| `edit_storage` | Есть хотя бы 1 online edit SE | `fail` → degraded (нельзя temporary) |
| `rw_storage` | Есть хотя бы 1 online rw SE | `fail` → degraded (нельзя permanent) |

Статусы: `ok` (200), `degraded` (200), `fail` (503).

---

## 6. Аутентификация

JWT RS256 токены, выданные Admin Module.

**Публичные endpoints** (без аутентификации):

- `/health/live`, `/health/ready`, `/metrics` — Kubernetes probes и мониторинг

**Защищённые endpoints** — требуют JWT Bearer token:

| Scope / Role | Операции |
|-------------|----------|
| SA `files:write` | Загрузка файлов |
| Роль `admin` | Загрузка файлов |

Валидация JWT:

- Алгоритм: RS256
- Публичный ключ: получается через JWKS endpoint Admin Module
- Claims: `sub` (идентификатор субъекта), `scopes` (массив) или `role` (строка)

### Собственный Service Account

Ingester сам является клиентом Admin Module. Для обращения к API Admin Module
(список SE, регистрация файлов) Ingester использует собственный Service Account
с scopes `storage:read` + `files:write`.

Credentials SA (`client_id`, `client_secret`) передаются через env-переменные.
Ingester получает JWT token при старте и обновляет его по истечении TTL.

---

## 7. Конфигурация

Все параметры задаются через переменные окружения.

### Сервер

| Переменная | Обязательная | По умолчанию | Описание |
|------------|:------------:|--------------|----------|
| `IG_PORT` | нет | `8020` | Порт HTTP-сервера (диапазон 8020-8029) |
| `IG_LOG_LEVEL` | нет | `info` | Уровень логирования (`debug`, `info`, `warn`, `error`) |
| `IG_LOG_FORMAT` | нет | `json` | Формат логов (`json` — production, `text` — development) |

### Admin Module

| Переменная | Обязательная | По умолчанию | Описание |
|------------|:------------:|--------------|----------|
| `IG_ADMIN_URL` | да | — | Базовый URL Admin Module (например, `http://admin-module:8000`) |
| `IG_JWKS_URL` | да | — | URL JWKS endpoint Admin Module для валидации входящих JWT |
| `IG_CLIENT_ID` | да | — | client_id собственного SA для обращения к Admin Module |
| `IG_CLIENT_SECRET` | да | — | client_secret собственного SA |

### Загрузка файлов

| Переменная | Обязательная | По умолчанию | Описание |
|------------|:------------:|--------------|----------|
| `IG_MAX_FILE_SIZE` | нет | `1073741824` | Максимальный размер файла в байтах (default 1 GB) |
| `IG_DEFAULT_TTL_DAYS` | нет | `30` | TTL по умолчанию для temporary файлов (дни) |

### TLS (исходящие к SE)

| Переменная | Обязательная | По умолчанию | Описание |
|------------|:------------:|--------------|----------|
| `IG_SE_CA_CERT_PATH` | нет | — | Путь к CA-сертификату для TLS-соединений с SE |

### Таймауты

| Переменная | Обязательная | По умолчанию | Описание |
|------------|:------------:|--------------|----------|
| `IG_DEPHEALTH_CHECK_INTERVAL` | нет | `15s` | Интервал проверки зависимостей topologymetrics (Go duration) |
| `IG_ADMIN_TIMEOUT` | нет | `10s` | Таймаут запросов к Admin Module (Go duration) |
| `IG_SE_UPLOAD_TIMEOUT` | нет | `5m` | Таймаут загрузки файла в SE (Go duration) |

---

## 8. Метрики Prometheus

| Метрика | Тип | Labels | Описание |
|---------|-----|--------|----------|
| `ingester_uploads_total` | counter | `retention_policy`, `status` | Общее количество загрузок |
| `ingester_upload_duration_seconds` | histogram | `retention_policy` | Общее время загрузки (клиент → ответ) |
| `ingester_se_upload_duration_seconds` | histogram | `storage_element_id` | Время загрузки в SE |
| `ingester_upload_size_bytes` | histogram | `retention_policy` | Размер загруженных файлов |
| `ingester_active_uploads` | gauge | — | Количество активных загрузок |
| `ingester_errors_total` | counter | `error_code` | Счётчик ошибок по типу |

---

## 9. Сборка и запуск

### Docker

```bash
# Сборка образа
docker build -t harbor.kryukov.lan/library/ingester-module:v0.1.0 \
  -f ingester-module/Dockerfile .

# Запуск контейнера
docker run -d \
  --name ingester-module \
  -p 8020:8020 \
  -e IG_ADMIN_URL=http://admin-module:8000 \
  -e IG_JWKS_URL=http://admin-module:8000/api/v1/auth/jwks \
  -e IG_CLIENT_ID=sa_ingester_abc123 \
  -e IG_CLIENT_SECRET=cs_secret_value_here \
  -e IG_SE_CA_CERT_PATH=/certs/ca.crt \
  -v /path/to/ca-certs:/certs:ro \
  harbor.kryukov.lan/library/ingester-module:v0.1.0
```

### Kubernetes (Helm)

```bash
# Установка через Helm chart
helm install ingester ./ingester-module/chart \
  --set adminUrl=http://admin-module.artstore.svc:8000 \
  --set jwksUrl=http://admin-module.artstore.svc:8000/api/v1/auth/jwks \
  --set clientId=sa_ingester_abc123 \
  --set clientSecret=cs_secret_value_here \
  --set seCaCert.secretName=se-ca-cert
```

---

## 10. Мониторинг зависимостей (topologymetrics)

Ingester Module интегрируется с SDK
[topologymetrics](https://github.com/BigKAA/topologymetrics)
для мониторинга здоровья внешних зависимостей через Prometheus-метрики.

### 10.1. Отслеживаемые зависимости

| Зависимость | Тип проверки | Критичность |
|-------------|-------------|:-----------:|
| Admin Module | HTTP (GET) | да |

### 10.2. Экспортируемые метрики

| Метрика | Тип | Описание |
|---------|-----|----------|
| `app_dependency_health` | Gauge | 1 = доступен, 0 = недоступен |
| `app_dependency_latency_seconds` | Histogram | Время проверки |
| `app_dependency_status` | Gauge | Категория результата (ok, timeout, error...) |
| `app_dependency_status_detail` | Gauge | Детальная причина |

Метрики доступны на endpoint `/metrics` вместе с остальными
Prometheus-метриками Ingester Module.

### 10.3. Интеграция в коде

```go
import (
    "github.com/BigKAA/topologymetrics/sdk-go/dephealth"
    _ "github.com/BigKAA/topologymetrics/sdk-go/dephealth/checks"
)

dh, err := dephealth.New("ingester-module", "artstore",
    dephealth.WithCheckInterval(cfg.DephealthCheckInterval),
    dephealth.HTTP("admin-module",
        dephealth.FromURL(cfg.AdminURL + "/health/live"),
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
