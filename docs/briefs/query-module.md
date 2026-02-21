# Query Module — Бриф модуля

**Версия**: 1.0.0
**Дата**: 2026-02-21
**Статус**: Draft
**Порты**: 8030-8039

---

## 1. Назначение модуля

Query Module — модуль поиска и скачивания файлов системы Artsore.
Предоставляет полнотекстовый поиск по метаданным файлов и проксирует
скачивание файлов с Storage Elements.

### Ключевые концепции

**Три режима поиска** — поиск по метаданным файлов:

- `exact` — точное совпадение (поле = значение, case-insensitive для строк)
- `partial` — частичное совпадение (ILIKE '%value%')
- `fulltext` — полнотекстовый поиск (PostgreSQL FTS, GIN-индексы)
  по полям `original_filename`, `description`, `tags`.
  Результаты ранжируются по `relevance_score`

**Proxy Download** — клиент скачивает файлы через Query Module, не зная
прямых URL Storage Elements. Query Module определяет SE, на котором
хранится файл, и проксирует запрос. Поддерживаются HTTP Range requests
для возобновляемого скачивания.

**Ленивая очистка реестра** — при проксировании скачивания, если SE
вернул 404 (файл удалён GC или отсутствует), Query Module обновляет
статус файла в реестре (`deleted`) и возвращает 404 клиенту. Это один
из 4 механизмов синхронизации файлового реестра с источником истины
(`attr.json` на SE).

**Shared PostgreSQL** — Query Module использует одну PostgreSQL
с Admin Module:

- Admin Module владеет таблицей файлов (file registry) и пишет в неё
- Query Module добавляет GIN/FTS индексы к этой же таблице и читает из неё
- Синхронизация между модулями не нужна — оба работают с одной БД
- Query Module управляет своими миграциями (только FTS-индексы)

**In-memory LRU cache** — кэш горячих метаданных для снижения нагрузки
на PostgreSQL. TTL 30–60 секунд. Кэширует результаты поиска метаданных
по `file_id` (для download proxy). При промахе — запрос к PostgreSQL.

---

## 2. Топология

Query Module располагается **внутри кластера Kubernetes**. Входящие
запросы от клиентов проходят через Envoy Gateway. Для проксирования
скачивания обращается к Storage Elements через WAN (TLS).

```text
                    Envoy Gateway (TLS, CORS)
                           │
┌──────────────────────────┼───────────────────────────┐
│          Kubernetes кластер                          │
│                          │                           │
│                  ┌───────▼───────┐                   │
│                  │    Query      │                   │
│                  │    Module     │                   │
│                  └───┬───────┬───┘                   │
│                      │       │                       │
│        ┌─────────────┘       └──────────┐            │
│        ▼                                ▼            │
│  ┌──────────────┐              ┌──────────────┐      │
│  │ Admin Module │              │  PostgreSQL  │      │
│  │ (SE URLs,    │              │  (shared,    │      │
│  │  JWT JWKS)   │              │   прямой     │      │
│  └──────────────┘              │   доступ)    │      │
│                                └──────────────┘      │
│                                                      │
└──────────────────────┬───────────────────────────────┘
                       │  WAN / TLS (proxy download)
                       ▼
                 ┌──────────┐    ┌──────────┐
                 │ SE #1    │    │ SE #2    │
                 │ (remote) │    │ (remote) │
                 └──────────┘    └──────────┘
```

**Следствия:**

- Query Module читает файловый реестр из PostgreSQL напрямую (shared DB)
- Для proxy download нужен URL SE — получает из Admin Module API
  (`GET /api/v1/storage-elements/{id}`) или из shared PostgreSQL
- Входящий трафик: plain HTTP (TLS terminates на Envoy Gateway)
- Исходящий к SE: TLS (SE remote, WAN) — для proxy download
- Горизонтальное масштабирование: несколько реплик, read-нагрузка на PostgreSQL

---

## 3. Зависимости

### Инфраструктурные

| Зависимость | Назначение |
|-------------|------------|
| PostgreSQL (shared) | Чтение file registry + FTS-индексы (GIN) |

Query Module не владеет таблицей файлов — только читает и добавляет индексы.
Владелец таблицы — Admin Module.

### Межмодульные

| Модуль | Направление | Назначение |
|--------|-------------|------------|
| Admin Module | Query → Admin | JWKS (`GET /auth/jwks`) для валидации входящих JWT |
| Admin Module | Query → Admin | JWT token (`POST /auth/token`) для обращения к SE |
| Admin Module | Query → Admin | URL SE (`GET /storage-elements/{id}`) для proxy download |
| Storage Element | Query → SE | Скачивание файла (`GET /api/v1/files/{file_id}/download`) |
| PostgreSQL | Query ↔ DB | Чтение file registry (search), запись статуса (lazy cleanup) |

---

## 4. Workflow поиска

```text
Клиент                    Query Module              PostgreSQL
  │                          │                          │
  │  POST /api/v1/search     │                          │
  │  {query, filters, mode}  │                          │
  │─────────────────────────▶│                          │
  │                          │                          │
  │                          │  SQL query (FTS/ILIKE)   │
  │                          │  с GIN-индексами         │
  │                          │─────────────────────────▶│
  │                          │  rows + total count      │
  │                          │◀─────────────────────────│
  │                          │                          │
  │  200 SearchResponse      │                          │
  │  {items, total, ...}     │                          │
  │◀─────────────────────────│                          │
```

---

## 5. Workflow скачивания (proxy download)

```text
Клиент              Query Module         PostgreSQL    Admin Module    Storage Element
  │                     │                    │              │                │
  │ GET /files/{id}/    │                    │              │                │
  │   download          │                    │              │                │
  │────────────────────▶│                    │              │                │
  │                     │                    │              │                │
  │                     │ SELECT             │              │                │
  │                     │ storage_element_id │              │                │
  │                     │───────────────────▶│              │                │
  │                     │ {se_id, status}    │              │                │
  │                     │◀───────────────────│              │                │
  │                     │                    │              │                │
  │                     │ GET /storage-      │              │                │
  │                     │ elements/{se_id}   │              │                │
  │                     │ (или из cache)     │              │                │
  │                     │───────────────────────────────── ▶│                │
  │                     │ {url: "https://.."}│              │                │
  │                     │◀─────────────────────────────────│                │
  │                     │                    │              │                │
  │                     │            GET /api/v1/files/{id}/download        │
  │                     │            (proxy, streaming, Range headers)      │
  │                     │─────────────────────────────────────────────────▶│
  │                     │            200 (streaming binary)                 │
  │  200 streaming      │◀─────────────────────────────────────────────────│
  │  binary data        │                    │              │                │
  │◀────────────────────│                    │              │                │
```

### Ленивая очистка (lazy cleanup)

```text
Клиент              Query Module         PostgreSQL         Storage Element
  │                     │                    │                     │
  │ GET /files/{id}/    │                    │                     │
  │   download          │                    │                     │
  │────────────────────▶│                    │                     │
  │                     │     (определяет SE, запрашивает файл)    │
  │                     │────────────────────────────────────────▶│
  │                     │              404 Not Found               │
  │                     │◀────────────────────────────────────────│
  │                     │                    │                     │
  │                     │ UPDATE file_registry                    │
  │                     │ SET status='deleted'                    │
  │                     │───────────────────▶│                     │
  │                     │        OK          │                     │
  │                     │◀───────────────────│                     │
  │                     │                    │                     │
  │  404 NOT_FOUND      │                    │                     │
  │◀────────────────────│                    │                     │
```

---

## 6. API endpoints

6 endpoints. Полная спецификация —
[query-module-openapi.yaml](../api-contracts/query-module-openapi.yaml).

### Search (1 endpoint)

| Метод | Endpoint | Назначение | Аутентификация |
|-------|----------|------------|----------------|
| `POST` | `/api/v1/search` | Поиск файлов (FTS, partial, exact) | JWT `files:read` или роль `admin`/`readonly` |

**SearchRequest (JSON body):**

| Параметр | Тип | По умолчанию | Описание |
|----------|-----|--------------|----------|
| `query` | string | null | Поисковый запрос (max 500) |
| `filename` | string | null | Фильтр по имени файла |
| `file_extension` | string | null | Фильтр по расширению (без точки) |
| `tags` | string[] | null | Фильтр по тегам (AND-логика, max 50) |
| `uploaded_by` | string | null | Фильтр по загрузившему |
| `retention_policy` | string | null | `temporary` или `permanent` |
| `status` | string | `active` | `active`, `expired`, `deleted` |
| `min_size` | int64 | null | Минимальный размер (байт) |
| `max_size` | int64 | null | Максимальный размер (байт) |
| `uploaded_after` | datetime | null | Фильтр по дате (после) |
| `uploaded_before` | datetime | null | Фильтр по дате (до) |
| `mode` | string | `partial` | `exact`, `partial`, `fulltext` |
| `limit` | integer | `100` | 1–1000 |
| `offset` | integer | `0` | Смещение |
| `sort_by` | string | `uploaded_at` | `uploaded_at`, `original_filename`, `size`, `relevance` |
| `sort_order` | string | `desc` | `asc`, `desc` |

**SearchResponse:**

```json
{
  "items": [
    {
      "file_id": "...",
      "original_filename": "...",
      "content_type": "...",
      "size": 5242880,
      "checksum": "...",
      "uploaded_by": "...",
      "uploaded_at": "...",
      "description": "...",
      "tags": ["..."],
      "status": "active",
      "retention_policy": "permanent",
      "ttl_days": null,
      "expires_at": null,
      "relevance_score": 0.85
    }
  ],
  "total": 150,
  "limit": 100,
  "offset": 0,
  "has_more": true
}
```

`relevance_score` присутствует только при `mode=fulltext`, иначе `null`.

### Files (2 endpoints)

| Метод | Endpoint | Назначение | Аутентификация |
|-------|----------|------------|----------------|
| `GET` | `/api/v1/files/{file_id}` | Метаданные файла | JWT `files:read` или роль `admin`/`readonly` |
| `GET` | `/api/v1/files/{file_id}/download` | Скачивание файла (proxy к SE) | JWT `files:read` или роль `admin`/`readonly` |

**Download — HTTP заголовки ответа:**

| Заголовок | Описание |
|-----------|----------|
| `Content-Type` | MIME-тип файла |
| `Content-Length` | Размер файла (или части) |
| `Content-Disposition` | `attachment; filename="original_name.ext"` |
| `Accept-Ranges` | `bytes` — поддержка Range requests |
| `ETag` | Хэш для кэширования (на основе checksum) |
| `Content-Range` | Только для 206 Partial Content |

### Health (3 endpoints)

| Метод | Endpoint | Назначение | Аутентификация |
|-------|----------|------------|----------------|
| `GET` | `/health/live` | Liveness probe (процесс жив) | без аутентификации |
| `GET` | `/health/ready` | Readiness probe (PostgreSQL) | без аутентификации |
| `GET` | `/metrics` | Prometheus metrics | без аутентификации |

**Readiness checks:**

| Проверка | Описание | Влияние |
|----------|----------|---------|
| `postgresql` | PostgreSQL доступен (shared DB) | `fail` → весь Query Module fail |

Статусы: `ok` (200), `degraded` (200), `fail` (503).

---

## 7. Аутентификация

JWT RS256 токены, выданные Admin Module.

**Публичные endpoints** (без аутентификации):

- `/health/live`, `/health/ready`, `/metrics` — Kubernetes probes и мониторинг

**Защищённые endpoints** — требуют JWT Bearer token:

| Scope / Role | Операции |
|-------------|----------|
| SA `files:read` | Поиск, метаданные, скачивание |
| Роль `admin` | Поиск, метаданные, скачивание |
| Роль `readonly` | Поиск, метаданные, скачивание |

Валидация JWT:

- Алгоритм: RS256
- Публичный ключ: получается через JWKS endpoint Admin Module
- Claims: `sub`, `scopes` (SA) или `role` (Admin User)

### Собственный Service Account

Query Module является клиентом Admin Module и Storage Elements.
Для обращения к их API использует собственный Service Account
с scopes `files:read` + `storage:read`.

Credentials SA передаются через env-переменные. Query Module
получает JWT token при старте и обновляет по истечении TTL.

---

## 8. Конфигурация

Все параметры задаются через переменные окружения.

### Сервер

| Переменная | Обязательная | По умолчанию | Описание |
|------------|:------------:|--------------|----------|
| `QM_PORT` | нет | `8030` | Порт HTTP-сервера (диапазон 8030-8039) |
| `QM_LOG_LEVEL` | нет | `info` | Уровень логирования (`debug`, `info`, `warn`, `error`) |
| `QM_LOG_FORMAT` | нет | `json` | Формат логов (`json` — production, `text` — development) |

### PostgreSQL (shared с Admin Module)

| Переменная | Обязательная | По умолчанию | Описание |
|------------|:------------:|--------------|----------|
| `QM_DB_HOST` | да | — | Хост PostgreSQL |
| `QM_DB_PORT` | нет | `5432` | Порт PostgreSQL |
| `QM_DB_NAME` | да | — | Имя базы данных (та же БД, что у Admin Module) |
| `QM_DB_USER` | да | — | Пользователь БД (может быть отдельный read-user) |
| `QM_DB_PASSWORD` | да | — | Пароль БД |
| `QM_DB_SSL_MODE` | нет | `disable` | SSL режим |

### Admin Module

| Переменная | Обязательная | По умолчанию | Описание |
|------------|:------------:|--------------|----------|
| `QM_ADMIN_URL` | да | — | Базовый URL Admin Module (для SE URLs) |
| `QM_JWKS_URL` | да | — | URL JWKS endpoint для валидации входящих JWT |
| `QM_CLIENT_ID` | да | — | client_id собственного SA |
| `QM_CLIENT_SECRET` | да | — | client_secret собственного SA |

### Кэширование

| Переменная | Обязательная | По умолчанию | Описание |
|------------|:------------:|--------------|----------|
| `QM_CACHE_TTL` | нет | `60s` | TTL записей в LRU cache (Go duration) |
| `QM_CACHE_MAX_SIZE` | нет | `10000` | Максимальное количество записей в LRU cache |

### TLS (исходящие к SE)

| Переменная | Обязательная | По умолчанию | Описание |
|------------|:------------:|--------------|----------|
| `QM_SE_CA_CERT_PATH` | нет | — | Путь к CA-сертификату для TLS-соединений с SE |

### Таймауты

| Переменная | Обязательная | По умолчанию | Описание |
|------------|:------------:|--------------|----------|
| `QM_ADMIN_TIMEOUT` | нет | `10s` | Таймаут запросов к Admin Module (Go duration) |
| `QM_SE_DOWNLOAD_TIMEOUT` | нет | `5m` | Таймаут proxy download от SE (Go duration) |

---

## 9. FTS-индексы (миграции Query Module)

Query Module при старте применяет свои миграции к shared PostgreSQL.
Миграции Query Module **только добавляют индексы** — не создают и не
изменяют таблицы (таблицы принадлежат Admin Module).

### GIN-индексы для полнотекстового поиска

```sql
-- Полнотекстовый поиск по filename, description, tags
CREATE INDEX idx_file_registry_fts
  ON file_registry
  USING GIN (
    to_tsvector('russian',
      coalesce(original_filename, '') || ' ' ||
      coalesce(description, '') || ' ' ||
      array_to_string(tags, ' ')
    )
  );

-- Фильтр по тегам (GIN для массивов)
CREATE INDEX idx_file_registry_tags
  ON file_registry USING GIN (tags);

-- Фильтр по расширению
CREATE INDEX idx_file_registry_extension
  ON file_registry (
    lower(
      substring(original_filename from '\.([^.]+)$')
    )
  );

-- Составной индекс для частых фильтров
CREATE INDEX idx_file_registry_status_uploaded
  ON file_registry (status, uploaded_at DESC);

-- Фильтр по retention_policy
CREATE INDEX idx_file_registry_retention
  ON file_registry (retention_policy, status);
```

Индексы создаются с `IF NOT EXISTS` для идемпотентности.

---

## 10. Метрики Prometheus

| Метрика | Тип | Labels | Описание |
|---------|-----|--------|----------|
| `query_search_total` | counter | `mode`, `status` | Количество поисковых запросов |
| `query_search_duration_seconds` | histogram | `mode` | Латентность поиска |
| `query_downloads_total` | counter | `status` | Количество скачиваний |
| `query_download_duration_seconds` | histogram | — | Латентность proxy download |
| `query_download_bytes_total` | counter | — | Объём скачанных данных (байт) |
| `query_lazy_cleanup_total` | counter | — | Счётчик ленивых очисток (404 от SE) |
| `query_cache_hits_total` | counter | — | Попадания в LRU cache |
| `query_cache_misses_total` | counter | — | Промахи LRU cache |
| `query_active_downloads` | gauge | — | Количество активных proxy downloads |

---

## 11. Сборка и запуск

### Docker

```bash
# Сборка образа
docker build -t harbor.kryukov.lan/library/query-module:v1.0.0 \
  -f query-module/Dockerfile .

# Запуск контейнера
docker run -d \
  --name query-module \
  -p 8030:8030 \
  -e QM_DB_HOST=postgres \
  -e QM_DB_PORT=5432 \
  -e QM_DB_NAME=artsore \
  -e QM_DB_USER=artsore_readonly \
  -e QM_DB_PASSWORD=secret \
  -e QM_ADMIN_URL=http://admin-module:8000 \
  -e QM_JWKS_URL=http://admin-module:8000/api/v1/auth/jwks \
  -e QM_CLIENT_ID=sa_query_xyz789 \
  -e QM_CLIENT_SECRET=cs_secret_value_here \
  -e QM_SE_CA_CERT_PATH=/certs/ca.crt \
  -v /path/to/ca-certs:/certs:ro \
  harbor.kryukov.lan/library/query-module:v1.0.0
```

### Kubernetes (Helm)

```bash
# Установка через Helm chart
helm install query-module ./query-module/chart \
  --set db.host=postgresql.artsore.svc \
  --set db.name=artsore \
  --set db.user=artsore_readonly \
  --set db.password=secret \
  --set adminUrl=http://admin-module.artsore.svc:8000 \
  --set jwksUrl=http://admin-module.artsore.svc:8000/api/v1/auth/jwks \
  --set clientId=sa_query_xyz789 \
  --set clientSecret=cs_secret_value_here \
  --set seCaCert.secretName=se-ca-cert
```

---

## 12. Порты

| Порт | Назначение |
|------|------------|
| 8030-8039 | HTTP API Query Module (по одному порту на экземпляр) |

По умолчанию используется порт `8030`. При горизонтальном масштабировании
все реплики работают на одном порту, балансировка через Kubernetes Service.
