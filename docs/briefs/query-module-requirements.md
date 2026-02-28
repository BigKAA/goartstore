# Query Module — Спецификация требований

## 1. Цели модуля

Query Module (QM) — read-only сервис для поиска и скачивания файлов из распределённого хранилища Artstore.
Предоставляет API для:

- Поиска файлов по атрибутам (exact, partial)
- Получения метаданных файлов
- Proxy download файлов из Storage Elements

## 2. Функциональные требования

### 2.1 Поиск файлов (POST /api/v1/search)

- Два режима поиска:
  - `exact` — точное совпадение (case-insensitive)
  - `partial` — ILIKE '%value%' (режим по умолчанию)
- Фильтры по атрибутам: filename, file_extension, tags[], uploaded_by, retention_policy,
  status, min_size, max_size, uploaded_after, uploaded_before
- Пагинация: limit (default 100), offset
- Сортировка: uploaded_at, original_filename, size

### 2.2 Метаданные файла (GET /api/v1/files/{file_id})

- Возвращает полную информацию о файле из `file_registry`
- Использует LRU cache (см. 2.5)

### 2.3 Proxy Download (GET /api/v1/files/{file_id}/download)

- Streaming proxy: клиент → QM → Storage Element
- Клиент не знает URL Storage Element
- Поддержка HTTP Range requests (возобновляемое скачивание)
- Ответы: 200, 206 (Partial Content), 416 (Invalid Range), 502 (SE_UNAVAILABLE)
- Заголовки: Content-Type, Content-Length, Content-Disposition, Accept-Ranges, ETag, Content-Range
- QM получает URL SE из Admin Module API (`GET /api/v1/storage-elements/{id}`)
- QM обращается к SE с собственным SA токеном (не проксирует JWT пользователя)

### 2.4 Ленивая очистка реестра

- Если SE вернул 404 при download → QM обновляет `status='deleted'` в PostgreSQL
- Возвращает 404 клиенту
- Инвалидирует запись в LRU cache для этого file_id

### 2.5 In-memory LRU Cache

- Кэш метаданных файлов (FileRecord по file_id)
- TTL: 60s (настраивается `QM_CACHE_TTL`)
- Максимальный размер: 10000 записей (`QM_CACHE_MAX_SIZE`)
- Per-instance (каждый pod имеет свой кэш)
- Инвалидация при lazy cleanup

### 2.6 Health Checks

- `GET /health/live` — liveness probe (всегда 200)
- `GET /health/ready` — readiness probe (проверка PostgreSQL)
- Keycloak НЕ проверяется в readiness (в отличие от AM)

### 2.7 Метрики (GET /metrics)

Prometheus-метрики:

| Метрика | Тип | Labels |
|---------|-----|--------|
| `query_search_total` | counter | `status` |
| `query_search_duration_seconds` | histogram | — |
| `query_downloads_total` | counter | `status` |
| `query_download_duration_seconds` | histogram | — |
| `query_download_bytes_total` | counter | — |
| `query_lazy_cleanup_total` | counter | — |
| `query_cache_hits_total` | counter | — |
| `query_cache_misses_total` | counter | — |
| `query_active_downloads` | gauge | — |

Плюс метрики topologymetrics: `app_dependency_health`, `app_dependency_latency_seconds`,
`app_dependency_status`, `app_dependency_status_detail`.

## 3. Нефункциональные требования

### 3.1 Горизонтальное масштабирование

- **Stateless** — нет shared state между инстансами
- LRU cache per-instance, допускается stale data до TTL (60s) после lazy cleanup на другом инстансе
- DB connection pool: настраиваемый `QM_DB_MAX_CONNS` с учётом `replicas × pool_size`
- Helm chart: `replicas` в values (HPA добавляется позже при необходимости)
- Kubernetes readiness probe: каждый pod проверяет свой PostgreSQL коннект

### 3.2 Безопасность / Аутентификация

- JWT RS256 валидация через JWKS (`QM_JWKS_URL`)
- Авторизация: scope `files:read` или роль `admin`/`readonly`
- SA (Service Account): **отдельный Keycloak клиент** `artstore-query-module`
  (client_credentials grant, scopes: `files:read`, `storage-elements:read`)
- SA токен используется для обращения к AM API и SE API
- Health и metrics endpoints — без аутентификации
- Rate limiting — на уровне Envoy Gateway, вне scope QM

### 3.3 Shared PostgreSQL

- QM использует **ту же БД**, что и Admin Module
- QM **только читает** таблицу `file_registry` (и пишет `status='deleted'` при lazy cleanup)
- QM управляет своими миграциями — **только индексы для поиска по атрибутам** (с `IF NOT EXISTS`)
- Миграции AM должны быть выполнены до миграций QM (зависимость при деплое)

### 3.4 Индексы для поиска по атрибутам (миграции QM)

```sql
-- Теги (GIN для массивов)
CREATE INDEX IF NOT EXISTS idx_file_registry_tags ON file_registry USING GIN (tags);

-- Расширение файла
CREATE INDEX IF NOT EXISTS idx_file_registry_extension ON file_registry
  (lower(substring(original_filename from '\.([^.]+)$')));

-- Составной для частых фильтров
CREATE INDEX IF NOT EXISTS idx_file_registry_status_uploaded ON file_registry
  (status, uploaded_at DESC);

-- Retention policy
CREATE INDEX IF NOT EXISTS idx_file_registry_retention ON file_registry
  (retention_policy, status);

-- Имя файла (для ILIKE-поиска)
CREATE INDEX IF NOT EXISTS idx_file_registry_filename_lower ON file_registry
  (lower(original_filename));

-- Загрузивший пользователь
CREATE INDEX IF NOT EXISTS idx_file_registry_uploaded_by ON file_registry
  (uploaded_by);
```

### 3.5 Производительность

- Таймауты (все настраиваемые через env):
  - `QM_ADMIN_TIMEOUT=10s` — запросы к Admin Module
  - `QM_SE_DOWNLOAD_TIMEOUT=5m` — proxy download к SE
- Graceful shutdown: стандартный 30s, активные downloads прерываются
  (клиент может повторить с HTTP Range)
- HTTP-клиент к SE: настраиваемый `MaxIdleConnsPerHost`

### 3.6 Observability

- topologymetrics: PostgreSQL (critical) + Admin Module (critical)
- Логирование: `slog` + JSON
- Уровень логирования: `QM_LOG_LEVEL` (default: info)

## 4. Маршрутизация (Gateway API)

- Внешний доступ: `artstore.kryukov.lan/query/*` → `query-module:8030/*`
- Envoy Gateway HTTPRoute с path prefix `/query` и strip prefix
- Rate limiting на уровне Gateway (вне scope QM)

## 5. Зависимости между модулями

### QM → Admin Module

| Endpoint | Назначение |
|----------|------------|
| `GET /auth/jwks` | JWKS для валидации входящих JWT |
| `POST /auth/token` | Получение SA токена (client_credentials) |
| `GET /api/v1/storage-elements/{id}` | URL Storage Element для proxy download |

### QM → Storage Element

| Endpoint | Назначение |
|----------|------------|
| `GET /api/v1/files/{file_id}/download` | Proxy download (streaming, Range) |

## 6. Конфигурация (env-переменные)

```
# Сервер
QM_PORT=8030
QM_LOG_LEVEL=info
QM_LOG_FORMAT=json

# База данных (shared с AM)
QM_DB_HOST (required)
QM_DB_PORT=5432
QM_DB_NAME (required)
QM_DB_USER (required)
QM_DB_PASSWORD (required)
QM_DB_SSL_MODE=disable
QM_DB_MAX_CONNS=10

# Аутентификация
QM_JWKS_URL (required)         — JWKS endpoint для валидации JWT
QM_CLIENT_ID (required)        — client_id SA (artstore-query-module)
QM_CLIENT_SECRET (required)    — client_secret SA

# Admin Module
QM_ADMIN_URL (required)        — базовый URL Admin Module
QM_ADMIN_TIMEOUT=10s

# Storage Element
QM_SE_DOWNLOAD_TIMEOUT=5m
QM_SE_CA_CERT_PATH             — CA-сертификат для TLS к SE (optional)

# Cache
QM_CACHE_TTL=60s
QM_CACHE_MAX_SIZE=10000

# topologymetrics
QM_DEPHEALTH_CHECK_INTERVAL=15s
```

## 7. Тестовая инфраструктура

- QM добавляется в существующий Helm chart `tests/helm/artstore-apps/`
- Отдельный Keycloak клиент `artstore-query-module` (client_credentials)
  в тестовой конфигурации Keycloak
- Тестовый клиент `artstore-test-user` переиспользуется для JWT пользователей
- Интеграционные тесты: bash + curl в `tests/scripts/`
- Init data: тестовые файлы через AM/SE для проверки поиска и download

## 8. Структура модуля

```
src/query-module/
├── cmd/query-module/main.go          — Точка входа
├── internal/
│   ├── api/
│   │   ├── generated/                — oapi-codegen (types.gen.go, server.gen.go)
│   │   ├── handlers/                 — search.go, files.go, health.go, handler.go
│   │   ├── middleware/               — auth.go, logging.go, metrics.go
│   │   └── errors/                   — errors.go (+ INVALID_RANGE)
│   ├── config/config.go              — QM_* env vars
│   ├── database/database.go          — pgxpool Connect + Migrate (только индексы)
│   ├── domain/model/file.go          — доменная модель FileRecord
│   ├── repository/                   — search.go, file.go (ReadFileByID, Search, MarkDeleted)
│   ├── service/                      — search.go, download.go, dephealth.go, cache.go
│   ├── adminclient/client.go         — HTTP-клиент к AM (SE URL, token)
│   ├── seclient/client.go            — HTTP-клиент к SE (proxy download)
│   └── server/server.go              — chi router, graceful shutdown
├── migrations/                       — SQL миграции (только индексы)
├── charts/query-module/              — Helm chart
├── tests/                            — Интеграционные тесты
├── Dockerfile
├── docker-compose.yaml
├── Makefile
├── oapi-codegen-server.yaml
├── oapi-codegen-types.yaml
├── go.mod                            — module github.com/bigkaa/goartstore/query-module
└── go.sum
```

## 9. Версионирование

- Начальная версия: `v0.1.0-1`
- Стадия разработки: `0.x.y`

## 10. Что вне scope

- Полнотекстовый поиск (FTS) — только поиск по атрибутам
- Redis / внешний кэш
- search_history, download_statistics
- mTLS (только TLS с CA-сертификатом)
- Rate limiting (на уровне Gateway)
- UI
- HPA (добавляется позже)

## 11. Открытые вопросы

1. Нужен ли endpoint для инвалидации кэша (admin-only)?
   (Рекомендация: не нужен, TTL 60s достаточно)
