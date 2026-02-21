# Требования к переработке ArtStore

**Дата**: 2026-02-21
**Обновлено**: 2026-02-21
**Статус**: Draft

---

## 1. Цель проекта

Переработка существующего проекта ArtStore (Python/FastAPI) с нуля на языке Go. Старый проект в `old_artstore/` избыточно сложен (overengineered): слишком много архитектурных паттернов (Saga, Circuit Breaker, Two-Phase Commit и др.), сложные взаимосвязи между модулями. Новый проект должен быть прагматичным и простым.

## 2. Принятые решения

### 2.1. Организация проекта

- **Монорепо**: все модули в одном репозитории, каждый в своей директории
- **Язык**: Go
- **Целевая структура**:
  ```
  artsore/
  ├── admin-module/       # Аутентификация и управление
  ├── storage-element/    # Физическое хранение файлов
  ├── ingester-module/    # Загрузка файлов
  ├── query-module/       # Поиск и скачивание
  ├── admin-ui/           # Веб-интерфейс (Angular, позднее)
  ├── pkg/                # Общие Go-пакеты (если понадобятся)
  ├── docs/               # Документация и API-контракты
  └── old_artstore/       # Старый проект (справочный материал)
  ```

### 2.2. API

- **Формат**: REST с OpenAPI 3.x спецификацией
- **Межсервисное взаимодействие**: REST (HTTP)
- **Документация**: OpenAPI spec (yaml) + человекочитаемый README по каждому модулю

### 2.3. Технологический стек Go

| Компонент | Выбор |
|-----------|-------|
| HTTP фреймворк | `net/http` + `chi` router |
| PostgreSQL драйвер | `pgx` |
| Генерация кода из SQL | `sqlc` |
| Миграции БД | `golang-migrate` |
| OpenAPI | OpenAPI 3.x (yaml) |
| Admin UI шаблоны | `templ` (type-safe HTML) |
| Admin UI интерактивность | HTMX |
| Admin UI стили | Tailwind CSS (standalone) или Bootstrap |

### 2.4. Архитектурные паттерны

**Сохраняем** из старого проекта:
- WAL protocol (Write-Ahead Log) для атомарности файловых операций
- Attribute-first model (`*.attr.json` как источник истины для метаданных)

**Убираем** (избыточные):
- Saga Pattern
- Circuit Breaker
- Two-Phase Commit Finalization
- Raft consensus
- Redis Pub/Sub Service Discovery
- Adaptive Capacity Thresholds
- Multi-level caching
- Application-level compression (HTTP-level достаточно)

### 2.5. Топология развёртывания

**В Kubernetes кластере** (надёжная внутренняя сеть):
- Admin Module
- Ingester Module
- Query Module
- Admin UI
- PostgreSQL (shared между Admin Module и Query Module)

**Вне кластера** (remote, потенциально WAN, ненадёжная сеть):
- Storage Element — может располагаться где угодно (другие кластеры, bare metal,
  edge-локации)

**Следствия:**
- Межсервисное взаимодействие Admin/Ingester/Query — надёжное, низкая latency
- Взаимодействие со Storage Element — через WAN, требует TLS, устойчивость к сбоям
- CORS обрабатывается на уровне Envoy Gateway (не в Go-модулях)
- OpenTelemetry tracing включён для всех модулей

### 2.6. Shared PostgreSQL (Admin Module + Query Module)

Admin Module и Query Module используют **один PostgreSQL instance** в кластере:
- Admin Module владеет таблицей файлов (file registry) и пишет в неё
- Query Module добавляет GIN/FTS индексы к этой же таблице и читает из неё
- **Синхронизация не нужна** — оба модуля работают с одной БД
- Таблица `file_metadata_cache` из старого проекта **не нужна**
- Каждый модуль управляет своими миграциями (Query Module добавляет индексы)
- In-memory LRU cache (30-60s TTL) в Query Module для горячих метаданных

### 2.7. Аутентификация и авторизация (RBAC)

**Service Accounts (M2M)** — для межсервисного взаимодействия:
- OAuth 2.0 Client Credentials flow → JWT RS256
- Авторизация через **scopes** в JWT: `files:read`, `files:write`,
  `storage:read`, `storage:write`, `admin:read`, `admin:write`
- Формат client_id: читаемый `sa_<name>_<random>` (не UUID)
- Secret expiration: конфигурируемый (default 90 дней, 0 = без истечения)

**Admin Users (H2M)** — для администраторов через Admin UI:
- Login/password → JWT RS256
- Авторизация через **роли**: `admin` (полный доступ), `readonly` (только чтение)
- Account locking: после 5 неудачных попыток — блокировка на 15 минут
  (конфигурируемо) + ручная разблокировка через API

**Убрано**: роли `SUPER_ADMIN`, `OPERATOR`, `AUDITOR` — избыточны.
Двух ролей (`admin`, `readonly`) достаточно для администраторов.

### 2.8. Retention Policy

Файлы загружаются с одной из двух политик хранения:

| Политика | Storage Element | TTL | Описание |
|----------|-----------------|-----|----------|
| `temporary` | Edit SE | 1-365 дней (default 30) | Черновики, auto-expire по TTL |
| `permanent` | RW SE | Нет | Постоянное хранение |

**Смена retention_policy (temporary → permanent) не поддерживается в v1.**
Если нужен постоянный файл — загрузить с `retention_policy=permanent` сразу.
Temporary файлы — настоящие черновики, которые истекают по TTL.
Promotion можно добавить в будущих версиях при реальной потребности.

### 2.9. Унификация API (результаты ревизии)

**Версионирование URL**: все модули используют `/api/v1/...`

**Пагинация** — единый формат для всей системы:

Запрос: `limit` (int, default 100), `offset` (int, default 0)

Ответ:

```json
{
  "items": [...],
  "total": 150,
  "limit": 50,
  "offset": 0,
  "has_more": true
}
```

**Формат ошибок** — единый для всей системы:

```json
{
  "error": {
    "code": "VALIDATION_ERROR",
    "message": "Human-readable description"
  }
}
```

**Health endpoints** — единый формат:

| Endpoint | Назначение |
|----------|------------|
| `GET /health/live` | Liveness probe |
| `GET /health/ready` | Readiness probe |
| `GET /metrics` | Prometheus metrics |

Формат liveness: `{"status": "ok", "timestamp": "...", "version": "...", "service": "..."}`

Статусы readiness: `ok` (200), `degraded` (200), `fail` (503)

**Скрытие внутренних данных** — не показывать клиенту:
- `storage_filename` (внутреннее имя файла в SE)
- `storage_element_url` (прямой URL SE)
- `storage_element_id` — только в Admin Module API (для администраторов)

### 2.10. Синхронизация файлового реестра

**Ключевой принцип**: `*.attr.json` файлы на Storage Elements — **единственный источник
истины** для метаданных файлов. PostgreSQL файловый реестр (Admin Module) — вторичный
индекс, который всегда может быть восстановлен из attr.json файлов.

#### 4 механизма синхронизации

| # | Механизм | Триггер | Описание |
|---|----------|---------|----------|
| 1 | Периодический sync | Таймер в Admin Module | Фоновая задача, интервал 1 час (конфигурируемый). Все SE опрашиваются **параллельно**. Admin Module постранично вычитывает `GET /api/v1/files` с каждого SE и обновляет файловый реестр в PostgreSQL |
| 2 | Ленивая очистка | 404 от SE при скачивании | Query Module проксирует download на SE. Если SE вернул 404 — файл удалён (GC) или отсутствует. Query Module обновляет статус файла в реестре (`deleted`) |
| 3 | Ручной sync | Администратор из Admin UI | Администратор запускает `POST /api/v1/storage-elements/{id}/sync` для конкретного SE. Синхронизирует mode, status, capacity **и файловые метаданные** |
| 4 | Full sync при подключении | Регистрация SE или восстановление из backup | При `POST /api/v1/storage-elements` (регистрация нового SE) или при первом подключении SE после восстановления БД из backup — автоматический full sync файлов |

#### Логика синхронизации файлов

При sync Admin Module для каждого SE:
1. Постранично запрашивает `GET /api/v1/files` (пагинация: limit/offset)
2. Для каждого файла из ответа SE:
   - Если файл есть в реестре — обновить метаданные (status, tags, description)
   - Если файла нет в реестре — добавить запись
3. Файлы в реестре, привязанные к этому SE, но отсутствующие в ответе SE —
   пометить как `deleted`

#### Сценарий восстановления после сбоя PostgreSQL

1. Восстановить PostgreSQL из backup (или развернуть чистый)
2. Административные данные (users, SA, JWT keys) — из backup
3. Файловый реестр — автоматический full sync при подключении каждого SE
4. Система полностью работоспособна после завершения sync всех SE

### 2.11. Модули — все 5 переносятся

1. **Admin Module** (порты 8000-8009) — аутентификация OAuth 2.0 JWT (RS256), управление
2. **Storage Element** (порты 8010-8019) — физическое хранение файлов
3. **Ingester Module** (порты 8020-8029) — загрузка файлов
4. **Query Module** (порты 8030-8039) — поиск и скачивание
5. **Admin UI** (порт 4200) — Go + Templ + HTMX

### 2.12. Порядок реализации

1. ~~Ревизия API всех модулей старого проекта~~ ✅
2. ~~Рекомендации по унификации API~~ ✅
3. **Фиксация контрактов** — OpenAPI spec для каждого модуля
4. **Бриф** по каждому модулю (OpenAPI + README)
5. **Реализация на Go**, начиная со **Storage Element**

## 3. Функциональные требования

### 3.1. Storage Element (первый модуль, remote)

- CRUD операции с файлами
- Четыре режима работы: `edit`, `rw`, `ro`, `ar`
- **Mode transition через API** (runtime, без restart) — Admin Module вызывает
  `POST /api/v1/mode/transition`
- WAL protocol для атомарности
- Файлы атрибутов `*.attr.json` как источник истины
- **Встроенный GC**: SE сам удаляет expired файлы по TTL из attr.json
- **Встроенный reconciliation**: периодическая сверка attr.json ↔ filesystem +
  endpoint `POST /api/v1/maintenance/reconcile` для ручного запуска
- Health endpoints: `/health/live`, `/health/ready`
- Метрики Prometheus на `/metrics`
- **Только local filesystem** в v1 (S3/MinIO — позже при необходимости)
- Минимальные фильтры для `GET /api/v1/files`: `limit`, `offset`,
  опционально `status` (active/expired). Сложный поиск — задача Query Module
- TLS для всех API (SE remote, потенциально WAN)
- JWT валидация (публичный ключ от Admin Module)

### 3.2. Admin Module

- OAuth 2.0 Client Credentials flow для Service Accounts
- Login/password для Admin Users
- JWT RS256 генерация и валидация
- Управление Service Accounts (CRUD, rotate-secret)
- Управление Admin Users (CRUD, reset-password, change-password)
- Управление Storage Elements (CRUD, discover, sync по одному). **sync-all убран**
- File registry (CRUD файловых записей)
- **Синхронизация файлового реестра** (см. раздел 2.10):
  - Периодический фоновый sync всех SE (параллельно, интервал 1 час, конфигурируемый)
  - Ручной sync по одному SE (из Admin UI)
  - Full sync файлов при регистрации SE или восстановлении из backup
- JWT key rotation (status, rotate)
- RBAC: scopes для SA, roles для Admin Users (см. раздел 2.7)
- Account locking (5 попыток → 15 мин блокировка)

### 3.3. Ingester Module

- Streaming upload файлов в Storage Element
- Retention policy: `temporary` (Edit SE) / `permanent` (RW SE)
- TTL для temporary файлов (1-365 дней, default 30)
- Пользовательские метаданные (JSON)
- Регистрация файла в Admin Module file registry
- Выбор целевого SE через Admin Module API
- **Максимальный размер файла**: конфигурируемый лимит (default 1 GB)
- **Без application-level сжатия** — HTTP-level compression достаточно
- **Без Finalize API** — Two-Phase Commit убран
- **Без streaming progress** — не в v1

### 3.4. Query Module

- Поиск файлов по метаданным (POST `/api/v1/search`)
- PostgreSQL Full-Text Search (GIN индексы)
- Три режима поиска: `exact`, `partial`, `fulltext`
- Фильтры: filename, extension, tags, username, size, date, **retention_policy**
- Скачивание файлов: `GET /api/v1/files/{file_id}/download`
  (streaming, HTTP Range requests для resumable downloads)
- **Ленивая очистка реестра**: если SE возвращает 404 при скачивании —
  обновить статус файла в реестре (`deleted`)
- Метаданные файла: `GET /api/v1/files/{file_id}`
- **Shared PostgreSQL** с Admin Module (Query Module читает таблицу файлов
  Admin Module, добавляет FTS индексы)
- **In-memory LRU cache** для горячих метаданных (30-60s TTL)
- **Без Redis** — не нужен
- **Без search_history / download_statistics** — не в MVP

### 3.5. Admin UI (Go + Templ + HTMX)

- **Go-модуль** в монорепо (не отдельная SPA-кодовая база)
- **Templ** — type-safe HTML шаблоны, компилируются в Go-код
- **HTMX** — интерактивность без JavaScript (динамические таблицы, формы,
  фильтры через HTML-атрибуты)
- **CSS** — Tailwind CSS (standalone binary) или Bootstrap
- Один бинарник, один Docker image, без Node.js toolchain
- Dashboard, управление аккаунтами, Storage Elements, файлами
- Реализуется последним

## 4. Нефункциональные требования

- Разработка и тестирование только через Docker/Kubernetes
- Образы хранятся в Harbor (harbor.kryukov.lan)
- Деплой в тестовый Kubernetes кластер (Gateway API, MetalLB, cert-manager)
- Git Workflow: GitHub Flow + Conventional Commits + Semver Tags
- Комментарии и документация на русском языке
- Логирование: JSON формат для production, text для development

## 5. Бриф модуля — формат

Каждый модуль должен иметь:

1. **OpenAPI 3.x спецификация** (`api/openapi.yaml`) — машиночитаемый контракт
2. **README.md** — человекочитаемый обзор:
   - Назначение модуля
   - Зависимости (инфраструктурные и межмодульные)
   - Параметры конфигурации (env variables)
   - Endpoints (краткая таблица)
   - Команды сборки и запуска

## 6. Закрытые вопросы (из ревизии API)

Следующие вопросы были открыты по результатам ревизии API 4 модулей
и закрыты по итогам обсуждения:

- [x] **Redis в архитектуре** — не нужен. Shared PostgreSQL для Admin/Query,
  статическая конфигурация SE через Admin Module API
- [x] **Service Discovery** — Ingester/Query узнают о SE через API Admin Module
  (`GET /api/v1/storage-elements` с фильтрами)
- [x] **Синхронизация Query Module** — Shared PostgreSQL, синхронизация не нужна
- [x] **CORS** — обрабатывается на уровне Envoy Gateway
- [x] **Смена retention_policy** — не поддерживается в v1
- [x] **Compression** — убрана из API, HTTP-level достаточно
- [x] **S3 backend** — только local filesystem в v1
- [x] **GC** — встроенный в SE (auto-expire по TTL)
- [x] **Mode transition** — через API (runtime, без restart)
- [x] **Reconciliation** — автоматический в SE + manual endpoint
- [x] **sync-all** — убран (синхронизация SE по одному)
- [x] **client_id формат** — читаемый `sa_<name>_<random>`
- [x] **Secret expiration** — конфигурируемый (default 90 дней)
- [x] **Account locking** — таймер 15 мин + ручная разблокировка
- [x] **RBAC** — scopes для SA (M2M), 2 роли для Admin Users (H2M)
- [x] **Metrics path** — `/metrics` (стандарт Prometheus)
- [x] **storage_filename в ответах** — не показывать клиенту
- [x] **storage_element_id в ответах** — только в Admin Module API
- [x] **Download URL** — `/api/v1/files/{file_id}/download`
- [x] **Max file size** — конфигурируемый (default 1 GB)
- [x] **search/download history** — не в MVP
- [x] **LRU cache** — in-memory, 30-60s TTL в Query Module
- [x] **OpenTelemetry** — включить
- [x] **Upload progress** — не в v1
- [x] **Фильтры SE** — минимальные (limit/offset + status)
- [x] **retention_policy filter** — добавить в поиск
- [x] **file_metadata_cache** — не нужна (shared DB)
- [x] **Endpoints из старого проекта** — определены в ревизиях
  (`docs/api-review/*.md`)
- [x] **Совместимость API** — новый API полностью другой
- [x] **Синхронизация файлового реестра** — 4 механизма: периодический sync
  (1 час, параллельно), ленивая очистка при 404, ручной из UI, full sync
  при подключении SE. attr.json — единственный источник истины, PostgreSQL —
  вторичный индекс, восстанавливаемый из attr.json

## 7. Оставшиеся открытые вопросы

- [ ] Структура Go-проекта внутри модуля: стандартный layout
  (`cmd/`, `internal/`, `pkg/`) или плоская?
- [ ] Использовать ли `oapi-codegen` для генерации Go-кода из OpenAPI спецификации?
- [ ] Как организовать общий код (shared Go packages) между модулями?

## 8. Следующие шаги

1. ~~Провести ревизию API каждого модуля в `old_artstore/`~~ ✅
2. ~~Подготовить рекомендации по унификации API~~ ✅
3. **Зафиксировать API-контракты** (OpenAPI spec для каждого модуля)
4. **Создать бриф** по каждому модулю (OpenAPI + README)
5. **Начать реализацию** Storage Element на Go
