# Ревизия API: Storage Element

**Дата**: 2026-02-21
**Статус**: Draft
**Модуль**: Storage Element (old_artstore/storage-element/)

---

## 1. Сводка текущего API

Storage Element содержит **7 групп endpoints** + **1 неподключённую группу**:

| # | Группа | Prefix | Endpoints | Назначение |
|---|--------|--------|-----------|------------|
| 1 | File Operations | `/api/v1/files/*` | 6 | Upload, download, metadata, delete, list |
| 2 | System Info | `/api/v1/info` | 1 | Auto-discovery для Admin Module |
| 3 | Capacity | `/api/v1/capacity` | 1 | Информация о ёмкости для Ingester |
| 4 | GC Operations | `/api/v1/gc/*` | 2 | Физическое удаление файлов |
| 5 | Cache Management | `/api/v1/cache/*` | 4 | Управление PostgreSQL кешем метаданных |
| 6 | Health | `/health/*` | 2 | Kubernetes probes |
| 7 | Metrics | `/metrics` | 1 | Prometheus metrics |
| 8* | Mode Management | `/api/v1/mode/*` | 5 | Переключение режимов (НЕ ПОДКЛЮЧЁН в router) |

**Итого: ~17 активных + 5 неподключённых endpoints**

---

## 2. Выявленные проблемы и несоответствия

### 2.1. Mode endpoints определены, но не подключены

В `mode.py` реализованы 5 endpoints (`/info`, `/transition`, `/history`, `/matrix`, `/validate`), но `router.py` их не импортирует и не монтирует. При этом:

- README утверждает: *«Mode определяется ТОЛЬКО конфигурацией при запуске»*
- `mode.py` содержит `POST /mode/transition` для runtime-переключения rw→ro и ro→ar

**Противоречие**: Документация говорит одно, код делает другое. В новой версии нужно определиться.

### 2.2. Пагинация — минимальная

`GET /api/v1/files/` использует `skip`/`limit`, но ответ содержит только `{total, files}` — без `skip`, `limit` и `total_pages`. Клиент не знает текущую позицию в пагинации.

### 2.3. Schemas определены в endpoint-файлах

Все Pydantic-модели (`FileMetadataResponse`, `FileUploadResponse`, etc.) определены прямо в `endpoints/files.py`, `endpoints/info.py` и т.д. Директория `schemas/` пуста. Это нарушает разделение слоёв.

### 2.4. Info и Capacity частично дублируются

| Поле | `/api/v1/info` | `/api/v1/capacity` |
|------|:--------------:|:------------------:|
| capacity_bytes / total | ✅ | ✅ |
| used_bytes / used | ✅ | ✅ |
| free / available | ❌ | ✅ |
| percent_used | ❌ | ✅ |
| mode | ✅ | ✅ |
| status | ✅ (operational) | ❌ (health) |
| element_id / storage_id | ✅ | ✅ |

Два endpoint с пересекающимися данными и разными именами полей для одного и того же.

### 2.5. Two-Phase Commit в upload

`POST /api/v1/files/upload` принимает параметры `file_id` и `finalize_transaction_id` для Two-Phase Commit. Решено **убрать** Two-Phase Commit — эти параметры нужно удалить.

### 2.6. Cache TTL — внутренние поля в публичном API

`FileMetadataResponse` содержит `cache_updated_at`, `cache_ttl_hours`, `cache_expired` — это детали реализации кеша, которые не несут ценности для клиентов API.

### 2.7. Несовпадение RBAC-ролей с Admin Module

| Роль | Admin Module (SA) | Storage Element |
|------|:-----------------:|:---------------:|
| ADMIN | ✅ | ✅ |
| USER | ✅ | ✅ |
| AUDITOR | ✅ | ❌ |
| READONLY | ✅ | ✅ |
| OPERATOR | ❌ | ✅ |

Роль `OPERATOR` существует только в SE, роль `AUDITOR` — только в Admin Module.

### 2.8. Redis-зависимые компоненты

Следующие компоненты зависят от Redis (который убираем):

- Health Reporting Service (публикация статуса в Redis)
- Priority-Based Locking для cache operations (Redis distributed locks)
- Master Election для edit/rw кластеров
- Service Discovery publication

### 2.9. Adaptive Capacity Thresholds

Сложная система динамических порогов ёмкости (`ok` → `warning` → `critical` → `full`) с разными формулами для разных режимов и размеров SE. Решено **убрать**.

### 2.10. Непоследовательные имена статусов

| Контекст | Статусы |
|----------|---------|
| `/api/v1/info` → status | `operational`, `degraded`, `maintenance` |
| Admin Module → StorageStatus | `online`, `offline`, `degraded`, `maintenance` |
| `/api/v1/capacity` → health | `healthy`, `degraded`, `unhealthy` |

Три разных набора статусов для одного и того же понятия.

---

## 3. Анализ по группам: сохранить / упростить / убрать

### 3.1. File Operations — СОХРАНИТЬ (ядро модуля)

| Endpoint | Решение | Комментарий |
|----------|---------|-------------|
| `POST /files/upload` | Сохранить, упростить | Убрать file_id, finalize_transaction_id |
| `GET /files/{file_id}` | Сохранить | Основная операция |
| `GET /files/{file_id}/download` | Сохранить | Resumable download (Range requests) |
| `PATCH /files/{file_id}` | Сохранить | Обновление описания и метаданных |
| `DELETE /files/{file_id}` | Сохранить | Только в edit mode |
| `GET /files/` | Сохранить | Поиск и листинг файлов |

**Рекомендации:**

- Upload: убрать `file_id`, `finalize_transaction_id`, `retention_policy` из параметров (это дело Ingester→Admin Module)
- Metadata response: убрать `cache_updated_at`, `cache_ttl_hours`, `cache_expired`
- Добавить `tags` в FileMetadataResponse (есть в attr.json, но не в API)
- List: унифицировать пагинацию (page-based, см. рекомендации Admin Module)
- Добавить поисковые фильтры: `search`, `content_type`, `uploaded_by`, `uploaded_after`, `uploaded_before` (определены в API.md, но не все реализованы)

### 3.2. System Info — СОХРАНИТЬ (упростить)

| Endpoint | Решение | Комментарий |
|----------|---------|-------------|
| `GET /info` | Сохранить | Нужен для Admin Module discovery |

**Рекомендации:**

- Убрать `priority` (Sequential Fill убран)
- Убрать `element_id` (дублирует конфигурацию; Admin Module сам назначает ID при регистрации)
- Унифицировать `status` с Admin Module (использовать `online`, `offline`, `degraded`, `maintenance`)
- Объединить с Capacity — вся информация в одном endpoint

### 3.3. Capacity — ОБЪЕДИНИТЬ с Info

| Endpoint | Решение | Комментарий |
|----------|---------|-------------|
| `GET /capacity` | Убрать | Объединить данные с `/info` |

**Рекомендации:**

- Все поля capacity уже есть в `/info` (capacity_bytes, used_bytes)
- `available`, `percent_used` — клиент рассчитает
- `health`, `location` — перенести в `/info` (если нужны)

### 3.4. Mode Management — ОПРЕДЕЛИТЬСЯ

| Endpoint | Решение | Комментарий |
|----------|---------|-------------|
| `GET /mode/info` | Возможно сохранить | Полезно для introspection |
| `POST /mode/transition` | Убрать | Mode меняется через конфиг + restart |
| `GET /mode/history` | Убрать | Не нужно без runtime transitions |
| `GET /mode/matrix` | Убрать | Статическая информация, документируется |
| `POST /mode/validate` | Убрать | Клиент проверит через mode info |

**Рекомендации:**

- Информация о текущем mode уже есть в `/info`
- Если нужна детальная информация о разрешённых операциях — добавить поле `allowed_operations` в `/info`
- Runtime transition через API — убрать. Mode меняется только через конфиг + restart (как в документации)

### 3.5. GC Operations — ПЕРЕОСМЫСЛИТЬ

| Endpoint | Решение | Комментарий |
|----------|---------|-------------|
| `DELETE /gc/{file_id}` | Сохранить | Физическое удаление файла GC-ом |
| `GET /gc/{file_id}/exists` | Сохранить | Проверка существования перед удалением |

**Рекомендации:**

- GC endpoints нужны для Admin Module GC Service
- Поскольку убираем GC из Admin Module (он был overengineered), GC может стать проще:
  - Вариант A: Admin Module просто вызывает `DELETE /files/{file_id}` на SE
  - Вариант B: SE сам чистит expired файлы по cron (встроенный GC)
- Если GC остаётся на стороне Admin Module — GC endpoints нужны
- Если GC переносим на SE — отдельные GC endpoints не нужны

### 3.6. Cache Management — УПРОСТИТЬ ЗНАЧИТЕЛЬНО

| Endpoint | Решение | Комментарий |
|----------|---------|-------------|
| `POST /cache/rebuild` | Сохранить | Полная пересборка кеша — полезна при восстановлении |
| `POST /cache/rebuild/incremental` | Убрать | Объединить с full rebuild через query param |
| `GET /cache/consistency` | Сохранить | Полезна для диагностики |
| `POST /cache/cleanup-expired` | Убрать | Автоматизировать внутри SE |

**Рекомендации:**

- Объединить full/incremental: `POST /cache/rebuild?mode=full|incremental`
- Убрать Redis distributed locks — использовать in-process mutex
- Cleanup expired — внутренняя автоматическая операция, не нужен API endpoint
- Cache TTL — внутренняя деталь, не экспортировать в API

### 3.7. Health — СОХРАНИТЬ

| Endpoint | Решение | Комментарий |
|----------|---------|-------------|
| `GET /health/live` | Сохранить | Kubernetes liveness probe |
| `GET /health/ready` | Сохранить | Kubernetes readiness probe |
| `GET /metrics` | Сохранить | Prometheus metrics |

**Рекомендации:**

- Убрать Redis из readiness check (Redis убран)
- Для S3 backend: проверка bucket/folder доступности (как сейчас)
- Для Local backend: проверка доступности директории (как сейчас)
- Добавить `GET /health/startup` (как в Admin Module)

---

## 4. Ключевые архитектурные вопросы

### 4.1. Attr.json — источник истины (СОХРАНЯЕМ)

Это фундаментальная концепция, которую мы сохраняем:

- Каждый файл имеет `*.attr.json` рядом с собой
- PostgreSQL — только кеш для быстрого поиска
- При расхождении — attr.json приоритетнее
- Backup = копирование файлов, кеш пересоздаётся

### 4.2. WAL Protocol (СОХРАНЯЕМ)

Атомарность записи: WAL entry → attr.json → DB cache → WAL commit

### 4.3. Storage Backends: Local + S3

Абстракция over storage backend. В новой версии на Go:

- Interface для storage backend
- Local filesystem — основная реализация
- S3/MinIO — вторичная реализация

### 4.4. Где должен быть GC?

| Вариант | Плюсы | Минусы |
|---------|-------|--------|
| GC в Admin Module | Централизованный контроль, видит все SE | Требует отдельные GC endpoints на SE |
| GC в Storage Element | Проще, SE сам знает свои файлы | Нет координации, каждый SE сам по себе |
| Гибрид | Admin Module решает ЧТО удалять, SE — КАК | Сложнее, но гибче |

**Рекомендация**: GC в Storage Element. SE лучше знает свои файлы (attr.json), и может сам определять expired файлы. Admin Module не нужен как координатор GC.

### 4.5. Naming Convention для файлов

Текущий формат: `{original_name}_{username}_{timestamp}_{uuid}.{ext}`

Для Go-реализации — оставить как есть. Формат проверен в production и удобен для human-readable просмотра хранилища.

---

## 5. Итоговая таблица endpoints (новый API)

### File Operations (6 endpoints)

| Метод | Endpoint | Назначение | Аутентификация | Mode |
|-------|----------|------------|----------------|------|
| POST | `/api/v1/files/upload` | Загрузка файла (multipart) | JWT Bearer | edit, rw |
| GET | `/api/v1/files/{id}` | Метаданные файла | JWT Bearer | все |
| GET | `/api/v1/files/{id}/download` | Скачивание файла (streaming) | JWT Bearer | edit, rw, ro |
| PATCH | `/api/v1/files/{id}` | Обновление метаданных | JWT Bearer | edit, rw |
| DELETE | `/api/v1/files/{id}` | Удаление файла | JWT Bearer | edit |
| GET | `/api/v1/files` | Поиск и листинг файлов | JWT Bearer | все |

### System (2 endpoints)

| Метод | Endpoint | Назначение | Аутентификация |
|-------|----------|------------|----------------|
| GET | `/api/v1/info` | Информация о SE (discovery + capacity) | Без аутентификации |
| POST | `/api/v1/cache/rebuild` | Пересборка кеша (full/incremental) | Service Account (ADMIN) |
| GET | `/api/v1/cache/consistency` | Проверка консистентности кеша | Service Account (ADMIN) |

### Health (3 endpoints + metrics)

| Метод | Endpoint | Назначение |
|-------|----------|------------|
| GET | `/health/live` | Liveness probe |
| GET | `/health/ready` | Readiness probe |
| GET | `/health/startup` | Startup probe |
| GET | `/metrics` | Prometheus metrics |

### Итого: ~12 endpoints (было ~22 включая неподключённые)

---

## 6. Что убрано (сводка)

| Что убрано | Причина |
|------------|---------|
| `GET /capacity` | Объединён с `/info` |
| `GET /mode/info` | Информация о mode есть в `/info` |
| `POST /mode/transition` | Mode через конфиг + restart |
| `GET /mode/history` | Не нужно без runtime transitions |
| `GET /mode/matrix` | Статическая документация |
| `POST /mode/validate` | Клиент проверит через info |
| `DELETE /gc/{file_id}` | GC переносится внутрь SE |
| `GET /gc/{file_id}/exists` | GC переносится внутрь SE |
| `POST /cache/rebuild/incremental` | Объединён с full rebuild через query param |
| `POST /cache/cleanup-expired` | Автоматическая внутренняя операция |
| Cache TTL поля из metadata | Детали реализации |
| `file_id` / `finalize_transaction_id` в upload | Two-Phase Commit убран |
| `retention_policy` в upload | Не дело SE, это Ingester→Admin |
| `priority`, `element_id` из info | Sequential Fill и Redis убраны |
| Health Reporting to Redis | Redis убран |
| Priority-Based Locking (Redis) | Redis убран |
| Adaptive Capacity Thresholds | Убрано (over-engineering) |

---

## 7. Открытые вопросы

1. **GC**: Встроенный в SE (SE сам чистит expired файлы) или управляемый Admin Module (через отдельные endpoints)? Рекомендую встроенный.
2. **Mode transition через API**: Нужно ли оставить runtime transition (rw→ro, ro→ar) или строго через конфиг + restart?
3. **Search в files**: Какие фильтры нужны? В API.md описано много (`search_query`, `uploaded_by`, `min_size`, `max_size`, `tags` и т.д.), но в коде реализован только базовый skip/limit.
4. **S3 backend**: Сохраняем ли поддержку S3/MinIO в первой версии Go, или только local filesystem?
5. **Reconciliation**: Автоматический scheduled process или только manual через cache/rebuild?
6. **RBAC**: Какие роли оставить? В SE есть OPERATOR (нет в Admin Module), в Admin Module есть AUDITOR (нет в SE). Нужна унификация.
