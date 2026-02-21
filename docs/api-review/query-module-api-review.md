# Ревизия API: Query Module

**Дата**: 2026-02-21
**Статус**: Draft
**Модуль**: Query Module (old_artstore/query-module/)

---

## 1. Сводка текущего API

Query Module содержит **4 группы endpoints** и **1 систему событий**:

| # | Группа | Prefix | Endpoints | Назначение |
|---|--------|--------|-----------|------------|
| 1 | Search | `/api/search` | 2 | Поиск файлов (FTS, partial, exact) |
| 2 | Download | `/api/download` | 2 | Скачивание файлов (streaming, resumable) |
| 3 | Health | `/health/*` | 2 | Kubernetes probes |
| 4 | Metrics | `/metrics` | 1 | Prometheus metrics |
| 5 | Events | (Redis Pub/Sub) | 3 типа | Синхронизация cache из Admin Module |

**Итого: 7 endpoints + 3 типа событий + root endpoint**

### Текущие endpoints

| Метод | Путь | Назначение |
|-------|------|------------|
| POST | `/api/search` | Поиск файлов с фильтрацией и FTS |
| GET | `/api/search/{file_id}` | Метаданные файла по ID |
| GET | `/api/download/{file_id}` | Скачивание файла (streaming) |
| GET | `/api/download/{file_id}/metadata` | Метаданные для скачивания |
| GET | `/health/live` | Liveness probe |
| GET | `/health/ready` | Readiness probe |
| GET | `/metrics` | Prometheus metrics |
| GET | `/` | Root (информация о сервисе) |

### Модели БД

| Таблица | Назначение |
|---------|------------|
| `file_metadata_cache` | Кеш метаданных файлов (FTS с GIN индексами) |
| `search_history` | История поисковых запросов (аналитика) |
| `download_statistics` | Статистика скачиваний (мониторинг) |

### Система событий (Redis Pub/Sub)

| Event | Обработчик | Действие |
|-------|------------|----------|
| `file:created` | `EventSubscriber._handle_file_created` | Добавить в cache (PostgreSQL) |
| `file:updated` | `EventSubscriber._handle_file_updated` | Обновить в cache |
| `file:deleted` | `EventSubscriber._handle_file_deleted` | Удалить из cache |

---

## 2. Выявленные проблемы и несоответствия

### 2.1. Нестандартные URL-пути (отличие от остальных модулей)

Все остальные модули используют prefix `/api/v1/...`. Query Module использует `/api/...` **без версии**:

| Модуль | URL prefix |
|--------|-----------|
| Admin Module | `/api/v1/...` |
| Storage Element | `/api/v1/...` |
| Ingester Module | `/api/v1/...` |
| **Query Module** | **`/api/...`** (без v1) |

**Решение**: Унифицировать — использовать `/api/v1/...` для всех модулей.

### 2.2. Пагинация — offset/limit (третий стиль в системе)

Query Module использует `offset/limit` пагинацию:

```json
{
  "results": [...],
  "total_count": 150,
  "limit": 50,
  "offset": 0,
  "has_more": true
}
```

Это третий стиль пагинации в системе:

| Модуль | Стиль | Параметры | Ответ |
|--------|-------|-----------|-------|
| Admin Users | Page-based | `page`, `page_size` | `items`, `total`, `page`, `page_size`, `total_pages` |
| Service Accounts, SE | Offset-based | `skip`, `limit` | `items`, `total`, `skip`, `limit` |
| **Query Module** | Offset-based | `offset`, `limit` | `results`, `total_count`, `offset`, `limit`, `has_more` |

Отличия от остальных offset-based:
- Параметр `offset` вместо `skip`
- Поле `results` вместо `items`
- Поле `total_count` вместо `total`
- Есть `has_more` (полезно, но нет в других)

**Решение**: Унифицировать пагинацию по всей системе.

### 2.3. Дублирование метаданных GET /search/{file_id} и GET /download/{file_id}/metadata

Два endpoint возвращают метаданные одного и того же файла с небольшими отличиями:

| Поле | GET /search/{file_id} | GET /download/{file_id}/metadata |
|------|----------------------|----------------------------------|
| `id` | Да | Да |
| `filename` | Да | Да |
| `file_size` | Да | Да |
| `mime_type` | Да | Да |
| `sha256_hash` | Да | Да |
| `storage_element_id` | Да | Да |
| `created_at` | Да | Да |
| `storage_filename` | Да | Нет |
| `username` | Да | Нет |
| `tags` | Да | Нет |
| `description` | Да | Нет |
| `updated_at` | Да | Нет |
| `relevance_score` | Да | Нет |
| `storage_element_url` | Нет | Да |
| `download_url` | Нет | Да |
| `supports_range_requests` | Нет | Да |

**Решение**: Объединить в один endpoint `GET /files/{file_id}`, возвращающий полные метаданные. Информация о поддержке Range requests — это HTTP заголовок, а не поле метаданных.

### 2.4. Storage Element URL в метаданных скачивания

`GET /download/{file_id}/metadata` возвращает `storage_element_url` — прямой URL SE:

```json
{
  "storage_element_url": "http://se-01:8010",
  "download_url": null
}
```

Аналогичная проблема как в Ingester Module — клиент не должен знать прямые URL Storage Element.

**Решение**: Убрать `storage_element_url`. Download должен проксироваться через Query Module.

### 2.5. Redis-зависимость для cache sync (Pub/Sub)

Cache синхронизация построена на Redis Pub/Sub:

```
Admin Module → Redis Pub/Sub → EventSubscriber → CacheSyncService → PostgreSQL
```

Согласно `docs/REQUIREMENTS.md`, Redis Pub/Sub Service Discovery убираем. Нужно решить вопрос с механизмом синхронизации cache в Query Module.

**Варианты замены:**
1. **Polling**: Query Module периодически запрашивает обновления у Admin Module
2. **Webhooks**: Admin Module вызывает webhook Query Module при изменениях
3. **Message queue**: Заменить Redis Pub/Sub на другой брокер (NATS, RabbitMQ)
4. **Change Data Capture**: Если Query Module читает напрямую из БД Admin Module

### 2.6. Multi-level caching — удаляется

Согласно `docs/REQUIREMENTS.md`, multi-level caching убираем. Текущая архитектура:

```
Request → Local Cache (LRU, 60s) → Redis Cache (5min) → PostgreSQL
```

**В новой архитектуре**: PostgreSQL — единственный источник данных для поиска. Кэширование на уровне приложения (in-memory LRU) можно оставить для горячих запросов, но Redis-слой убираем.

### 2.7. `storage_element_url` в модели БД

Таблица `file_metadata_cache` содержит колонку `storage_element_url`:

```python
storage_element_url: Mapped[str] = mapped_column(String(512), ...)
```

Это внутренняя деталь инфраструктуры. Query Module должен узнавать URL SE через Admin Module API, а не хранить его в своём cache.

**Решение**: Хранить только `storage_element_id`. URL SE получать динамически из Admin Module.

### 2.8. `storage_filename` в ответах

Как и в Ingester Module, `FileMetadataResponse` содержит `storage_filename` — внутреннее имя файла в SE:

```json
{
  "storage_filename": "contract_2025_550e8400.pdf"
}
```

**Решение**: Не показывать клиенту. Это внутренняя деталь Storage Element.

### 2.9. Формат liveness probe несогласован

| Модуль | Liveness response |
|--------|-------------------|
| Admin Module | `{"status": "ok", "timestamp": "...", "version": "...", "service": "..."}` |
| Ingester Module | `{"status": "ok", "timestamp": "...", "version": "...", "service": "..."}` |
| **Query Module** | **`{"status": "alive"}`** |

**Решение**: Унифицировать формат health responses по всем модулям.

### 2.10. Readiness probe — разные статусы и форматы

| Модуль | OK | Degraded | Fail |
|--------|------|----------|------|
| Ingester | `{"status": "ok"}` (200) | `{"status": "degraded"}` (200) | `{"status": "fail"}` (503) |
| **Query** | **`{"status": "ready"}`** (200) | **`{"status": "ready"}`** (200) | **`{"status": "not_ready"}`** (503) |

Разные значения статуса (`ok`/`fail` vs `ready`/`not_ready`). Degraded в Query Module имеет статус `ready`, а не выделен как отдельное состояние.

**Решение**: Унифицировать: `ok`, `degraded`, `fail` — по всем модулям.

### 2.11. Search через POST вместо GET

Поиск выполняется через `POST /api/search` с JSON body. Типичная практика для сложных поисковых запросов (много параметров, массивы тегов), но отклоняется от REST-стандарта где `GET` — для чтения.

**Обоснование для POST**: массив `tags[]`, множество фильтров, потенциально большой query string. Это допустимо и даже предпочтительно для сложного поиска. Оставляем POST.

### 2.12. Неиспользуемые схемы

В `schemas/download.py` определены, но не используются в endpoints:
- `DownloadProgress` — не подключена к API
- `DownloadResponse` — не подключена (download возвращает StreamingResponse)
- `DownloadStats` — не подключена (статистика только в БД)

---

## 3. Анализ по группам

### 3.1. Search API

#### POST /api/search — СОХРАНИТЬ, ИСПРАВИТЬ URL

Основной endpoint модуля. Хорошо спроектирован.

**SearchRequest — текущие параметры:**

| Параметр | Тип | Статус |
|----------|-----|--------|
| `query` | string (max 500) | Сохранить |
| `filename` | string (max 255) | Сохранить |
| `file_extension` | string (max 10) | Сохранить |
| `tags` | string[] (max 50) | Сохранить |
| `username` | string | Сохранить |
| `min_size` | integer | Сохранить |
| `max_size` | integer | Сохранить |
| `created_after` | datetime | Сохранить |
| `created_before` | datetime | Сохранить |
| `mode` | enum (exact/partial/fulltext) | Сохранить |
| `limit` | integer (1-1000, def: 100) | Сохранить, унифицировать имя |
| `offset` | integer (def: 0) | Сохранить, унифицировать имя |
| `sort_by` | enum | Сохранить |
| `sort_order` | enum (asc/desc) | Сохранить |

**Добавить**: фильтр по `retention_policy` (temporary/permanent).

**SearchResponse — изменения:**

| Поле | Текущее имя | Новое имя | Статус |
|------|-------------|-----------|--------|
| `results` | `results` | `items` | Унифицировать |
| `total_count` | `total_count` | `total` | Унифицировать |
| `limit` | `limit` | Сохранить |
| `offset` | `offset` | Сохранить |
| `has_more` | `has_more` | Сохранить |

#### GET /api/search/{file_id} — СОХРАНИТЬ как GET /files/{file_id}

Переименовать в более логичный путь. Это получение конкретного файла, а не поиск.

**FileMetadataResponse — изменения:**

| Поле | Статус |
|------|--------|
| `id` | Сохранить |
| `filename` | Сохранить |
| `storage_filename` | **Удалить** (внутренняя деталь) |
| `file_size` | Сохранить |
| `mime_type` | Сохранить |
| `sha256_hash` | Сохранить (переименовать в `checksum`?) |
| `username` | Сохранить |
| `tags` | Сохранить |
| `description` | Сохранить |
| `created_at` | Сохранить |
| `updated_at` | Сохранить |
| `storage_element_id` | Обсудить (внутренняя деталь) |
| `relevance_score` | Сохранить (только в результатах поиска) |
| **Добавить** `retention_policy` | temporary/permanent |
| **Добавить** `ttl_expires_at` | Для temporary файлов |

### 3.2. Download API

#### GET /api/download/{file_id} — СОХРАНИТЬ, ИСПРАВИТЬ URL

Streaming download с поддержкой HTTP Range requests. Хорошо реализован.

**Особенности:**
- `Content-Disposition: attachment; filename="..."` — корректно
- `Accept-Ranges: bytes` — корректно
- HTTP 206 Partial Content для Range requests — корректно
- StreamingResponse для больших файлов — корректно

**Изменить URL**: `/api/v1/download/{file_id}` или `/api/v1/files/{file_id}/download`

#### GET /api/download/{file_id}/metadata — УДАЛИТЬ (дубль)

Дублирует `GET /search/{file_id}`. Поля `download_url`, `supports_range_requests`, `storage_element_url` — не нужны клиенту.

`supports_range_requests` — HTTP заголовок `Accept-Ranges`, а не поле метаданных.

### 3.3. Health API — СОХРАНИТЬ, УНИФИЦИРОВАТЬ

#### GET /health/live — унифицировать формат

Текущий: `{"status": "alive"}`
Новый: `{"status": "ok", "timestamp": "...", "version": "...", "service": "artstore-query"}`

#### GET /health/ready — упростить

Текущие проверки:
- PostgreSQL — критичная (503 если down)
- Redis — опциональная (degraded если down)

После удаления Redis останется только PostgreSQL. Формат ответа: `ok`, `degraded`, `fail`.

### 3.4. Metrics — СОХРАНИТЬ

`GET /metrics` — Prometheus ASGI app. Единственный модуль, где метрики уже подключены. Хороший пример для остальных модулей.

### 3.5. Root endpoint — УДАЛИТЬ

`GET /` возвращает JSON с описанием сервиса. Не несёт функциональной нагрузки.

### 3.6. Events (Redis Pub/Sub) — ЗАМЕНИТЬ МЕХАНИЗМ

| Event | Действие | Статус |
|-------|----------|--------|
| `file:created` | INSERT в file_metadata_cache | Сохранить логику, заменить транспорт |
| `file:updated` | UPDATE в file_metadata_cache | Сохранить логику, заменить транспорт |
| `file:deleted` | DELETE из file_metadata_cache | Сохранить логику, заменить транспорт |

**Транспорт**: Redis Pub/Sub → (webhooks / polling / message queue — открытый вопрос).

---

## 4. Удаляемые зависимости и компоненты

### Компоненты для удаления

| Компонент | Причина удаления |
|-----------|-----------------|
| Redis Cache (multi-level) | Удаляем multi-level caching (REQUIREMENTS.md) |
| Redis Pub/Sub (event sync) | Удаляем Redis-зависимости |
| Local LRU Cache | Может остаться для hot data, обсудить |
| `storage_element_url` в БД | Внутренняя деталь, получать из Admin Module |

### Схемы для удаления

| Схема | Причина |
|-------|---------|
| `DownloadMetadata` | Дубль FileMetadataResponse + internal data |
| `DownloadProgress` | Не используется в API |
| `DownloadResponse` | Не используется (StreamingResponse) |
| `DownloadStats` | Не используется в API (только БД) |
| `RangeRequest` | Внутренняя модель, не API schema |

### Таблицы БД — обсудить

| Таблица | Статус | Комментарий |
|---------|--------|-------------|
| `file_metadata_cache` | Сохранить | Основная таблица, но переименовать? Это не cache, а source of truth для поиска |
| `search_history` | Обсудить | Полезно для аналитики, но нужно ли в MVP? |
| `download_statistics` | Обсудить | Полезно для мониторинга, но нужно ли в MVP? |

---

## 5. Рекомендации для нового API

### 5.1. Таблица endpoints нового Query Module

| Метод | Путь | Назначение |
|-------|------|------------|
| POST | `/api/v1/search` | Поиск файлов (FTS, partial, exact) |
| GET | `/api/v1/files/{file_id}` | Метаданные файла по ID |
| GET | `/api/v1/files/{file_id}/download` | Скачивание файла (streaming) |
| GET | `/health/live` | Liveness probe |
| GET | `/health/ready` | Readiness probe |
| GET | `/metrics` | Prometheus metrics |

**Итого: 6 endpoints** (было 7 + root, дубль metadata убран)

### 5.2. Новый SearchRequest

| Параметр | Тип | Обязательный | Default | Описание |
|----------|-----|--------------|---------|----------|
| `query` | string | Нет | null | Поисковый запрос (max 500) |
| `filename` | string | Нет | null | Имя файла (max 255) |
| `file_extension` | string | Нет | null | Расширение (.pdf, .jpg) |
| `tags` | string[] | Нет | null | Теги (max 50) |
| `username` | string | Нет | null | Фильтр по пользователю |
| `retention_policy` | string | Нет | null | temporary/permanent |
| `min_size` | integer | Нет | null | Мин. размер (bytes) |
| `max_size` | integer | Нет | null | Макс. размер (bytes) |
| `created_after` | datetime | Нет | null | Фильтр по дате |
| `created_before` | datetime | Нет | null | Фильтр по дате |
| `mode` | string | Нет | `partial` | exact/partial/fulltext |
| `limit` | integer | Нет | 100 | 1-1000 |
| `offset` | integer | Нет | 0 | Смещение |
| `sort_by` | string | Нет | `created_at` | Поле сортировки |
| `sort_order` | string | Нет | `desc` | asc/desc |

### 5.3. Новый SearchResponse (унифицированная пагинация)

```json
{
  "items": [...],
  "total": 150,
  "limit": 50,
  "offset": 0,
  "has_more": true
}
```

### 5.4. Новый FileMetadataResponse

| Поле | Тип | Описание |
|------|-----|----------|
| `id` | UUID | ID файла |
| `filename` | string | Оригинальное имя |
| `file_size` | integer | Размер (bytes) |
| `mime_type` | string/null | MIME тип |
| `checksum` | string | SHA256 |
| `username` | string | Владелец |
| `tags` | string[] | Теги |
| `description` | string/null | Описание |
| `retention_policy` | string | temporary/permanent |
| `ttl_expires_at` | datetime/null | Дата истечения TTL |
| `created_at` | datetime | Дата создания |
| `updated_at` | datetime | Дата обновления |
| `relevance_score` | float/null | Только в результатах FTS |

**Удалено:**
- `storage_filename` — внутренняя деталь SE
- `storage_element_id` — внутренняя деталь инфраструктуры
- `storage_element_url` — внутренняя деталь

### 5.5. Механизм синхронизации данных

Вместо Redis Pub/Sub нужен механизм доставки изменений из Admin Module в Query Module. Основные варианты:

**Вариант A: Admin Module → Webhook → Query Module**
```
Admin Module → POST /api/v1/internal/sync → Query Module
```
- Плюсы: простота, низкая latency
- Минусы: потеря событий при недоступности Query Module

**Вариант B: Query Module polling Admin Module**
```
Query Module → GET /api/v1/files?updated_after=... → Admin Module (каждые N секунд)
```
- Плюсы: надёжность, Query Module контролирует нагрузку
- Минусы: задержка до N секунд

**Вариант C: Shared PostgreSQL**
```
Query Module читает напрямую из БД Admin Module (read replica)
```
- Плюсы: минимальная задержка, нет API overhead
- Минусы: tight coupling между модулями

---

## 6. Открытые вопросы

1. **Синхронизация cache**: Какой механизм вместо Redis Pub/Sub? Webhooks, polling, shared DB?

2. **Таблица file_metadata_cache**: Переименовать? Это не cache в новой архитектуре, а основная таблица данных для поиска.

3. **search_history и download_statistics**: Нужны ли эти таблицы в MVP? Полезны для аналитики, но добавляют сложность.

4. **Local LRU cache**: Оставляем in-memory кэш для горячих запросов к метаданным файлов?

5. **OpenTelemetry**: В main.py настроен tracing. Включаем в новую архитектуру?

6. **Retention policy фильтр**: В поиске отсутствует фильтрация по retention_policy (temporary/permanent). Нужно добавить.

7. **URL endpoint download**: `/api/v1/download/{file_id}` или `/api/v1/files/{file_id}/download`? Второй вариант более REST-like.

8. **CORS**: Query Module единственный с CORS настройками (для Angular Admin UI на :4200). Нужна ли CORS обработка в Go, или Gateway API (Envoy) берёт это на себя?
