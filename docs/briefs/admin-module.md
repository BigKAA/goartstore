# Admin Module — Бриф модуля

**Версия**: 1.0.0
**Дата**: 2026-02-21
**Статус**: Draft
**Порты**: 8000-8009

---

## 1. Назначение модуля

Admin Module — центральный модуль системы Artsore, отвечающий за аутентификацию,
авторизацию, управление пользователями и сервисными аккаунтами, ведение реестра
Storage Elements и файлового реестра.

### Ключевые концепции

**OAuth 2.0 Client Credentials** — межсервисное взаимодействие (M2M).
Ingester, Query Module и другие сервисы получают JWT access token по
`client_id` + `client_secret`. Token содержит scopes для авторизации.
Refresh token не выдаётся — SA повторно запрашивает token по credentials.

**Admin Auth (login/password)** — аутентификация администраторов (H2M)
через login/password. Выдаёт пару access + refresh token. Access token
содержит role для авторизации.

**JWT RS256** — все токены подписываются RSA-256. Публичный ключ доступен
через JWKS endpoint (`GET /api/v1/auth/jwks`) для валидации токенов
другими модулями (Storage Element, Ingester, Query Module).

**RBAC** — двухуровневая авторизация:

- **Scopes** (M2M) — для Service Accounts: `files:read`, `files:write`,
  `storage:read`, `storage:write`, `admin:read`, `admin:write`
- **Roles** (H2M) — для Admin Users: `admin` (полный доступ),
  `readonly` (только чтение)

**File Registry** — реестр файлов в PostgreSQL. Вторичный индекс метаданных,
восстанавливаемый из `attr.json` файлов на Storage Elements. Ingester
регистрирует файлы после загрузки, Admin Module синхронизирует реестр
с SE периодически и по запросу.

**Shared PostgreSQL** — Admin Module и Query Module используют один PostgreSQL
instance. Admin Module владеет таблицами и пишет. Query Module добавляет
FTS-индексы и читает.

---

## 2. Топология

Admin Module располагается **внутри кластера Kubernetes** (надёжная внутренняя
сеть). Взаимодействие с Admin UI, Ingester и Query Module — низкая latency,
без TLS (TLS terminates на Envoy Gateway). Взаимодействие с Storage Elements —
через WAN, TLS.

```text
┌──────────────────────────────────────────────────────┐
│                  Kubernetes кластер                   │
│                                                      │
│  ┌──────────────┐   ┌───────────┐   ┌─────────────┐ │
│  │  Admin UI    │──▶│  Admin    │◀──│  Ingester   │ │
│  │  (Templ/     │   │  Module   │   │  Module     │ │
│  │   HTMX)      │   │          │   │             │ │
│  └──────────────┘   └─────┬─────┘   └─────────────┘ │
│                           │    ▲                     │
│                           │    │                     │
│  ┌──────────────┐         │    │   ┌──────────────┐  │
│  │  Query       │─────────┘    │   │  PostgreSQL  │  │
│  │  Module      │──────────────┘   │  (shared)    │  │
│  └──────────────┘                  └──────────────┘  │
│                                                      │
│             Envoy Gateway (TLS termination, CORS)     │
└──────────────────────┬───────────────────────────────┘
                       │  WAN / TLS
                       ▼
                 ┌──────────┐    ┌──────────┐
                 │ SE #1    │    │ SE #2    │
                 │ (remote) │    │ (remote) │
                 └──────────┘    └──────────┘
```

**Следствия:**

- Входящие запросы от клиентов (Admin UI, SA) проходят через Envoy Gateway —
  TLS termination и CORS на уровне gateway, Admin Module принимает plain HTTP
- Исходящие запросы к Storage Elements — через TLS (SE remote, WAN)
- Admin Module должен доверять TLS-сертификатам SE (CA bundle)
- JWKS endpoint должен быть доступен из-за пределов кластера (SE валидирует JWT)

---

## 3. Зависимости

### Инфраструктурные

| Зависимость | Назначение |
|-------------|------------|
| PostgreSQL | Хранение admin users, SA, SE registry, file registry, JWT keys |
| RSA key pair | Подпись JWT токенов (генерируется при первом запуске или загружается из файла) |

Admin Module не использует Redis или другие внешние хранилища.

### Межмодульные

| Модуль | Направление | Назначение |
|--------|-------------|------------|
| Storage Element | Admin → SE | Discovery (`GET /api/v1/info`), sync (`GET /api/v1/files`), mode transition |
| Ingester Module | Ingester → Admin | Получение JWT (`POST /auth/token`), регистрация файлов (`POST /files`), список SE (`GET /storage-elements`) |
| Query Module | Query → Admin | Получение JWT (`POST /auth/token`), чтение file registry (через shared PostgreSQL), список SE (`GET /storage-elements`) |
| Admin UI | UI → Admin | Все административные операции через REST API |
| Все модули | Модуль → Admin | JWKS endpoint (`GET /auth/jwks`) для валидации JWT |

---

## 4. Аутентификация и авторизация

### Service Accounts (M2M)

| Аспект | Описание |
|--------|----------|
| Flow | OAuth 2.0 Client Credentials |
| Токен | JWT RS256, содержит `sub` (client_id), `scopes` (массив) |
| Время жизни | Access token — 1 час (конфигурируемо) |
| Refresh token | Не выдаётся — SA повторно запрашивает по credentials |
| client_id формат | `sa_<name>_<random>` (читаемый) |
| Secret expiration | Конфигурируемый (default 90 дней, 0 = без истечения) |

**Scopes:**

| Scope | Назначение |
|-------|------------|
| `files:read` | Чтение метаданных и скачивание файлов |
| `files:write` | Загрузка, обновление и удаление файлов |
| `storage:read` | Чтение информации о Storage Elements |
| `storage:write` | Управление SE (sync, mode transition) |
| `admin:read` | Чтение административных данных |
| `admin:write` | Управление пользователями и SA |

### Admin Users (H2M)

| Аспект | Описание |
|--------|----------|
| Flow | Login/password → JWT RS256 |
| Токен | Access token + refresh token |
| Access token TTL | 30 минут (конфигурируемо) |
| Refresh token TTL | 24 часа (конфигурируемо) |
| Роли | `admin` — полный доступ, `readonly` — только чтение |

### Блокировка аккаунтов

- После 5 неудачных попыток входа (конфигурируемо) — блокировка на 15 минут
  (конфигурируемо)
- Счётчик сбрасывается при успешном входе
- Ручная разблокировка через `POST /admin-users/{id}/unlock`

### JWT Key Rotation

- Один активный RSA key pair
- При ротации (`POST /jwt-keys/rotate`) генерируется новая пара
- Старый ключ остаётся в JWKS в течение grace period (default 60 минут)
- Во время grace period оба ключа валидны для проверки подписи

---

## 5. API endpoints

36 endpoints, сгруппированных по назначению. Полная спецификация —
[admin-module-openapi.yaml](../api-contracts/admin-module-openapi.yaml).

### Auth — OAuth 2.0 (2 endpoints)

| Метод | Endpoint | Назначение | Аутентификация |
|-------|----------|------------|----------------|
| `POST` | `/api/v1/auth/token` | JWT token для SA (Client Credentials) | без (credentials в body) |
| `GET` | `/api/v1/auth/jwks` | Публичные ключи (JWKS) | без аутентификации |

### Admin Auth (4 endpoints)

| Метод | Endpoint | Назначение | Аутентификация |
|-------|----------|------------|----------------|
| `POST` | `/api/v1/admin-auth/login` | Вход администратора | без (credentials в body) |
| `POST` | `/api/v1/admin-auth/refresh` | Обновление access token | без (refresh token в body) |
| `GET` | `/api/v1/admin-auth/me` | Текущий администратор | JWT (любая роль) |
| `POST` | `/api/v1/admin-auth/change-password` | Смена пароля | JWT (любая роль) |

### Admin Users (7 endpoints)

| Метод | Endpoint | Назначение | RBAC |
|-------|----------|------------|------|
| `POST` | `/api/v1/admin-users` | Создать администратора | `admin` |
| `GET` | `/api/v1/admin-users` | Список администраторов | `admin`, `readonly` |
| `GET` | `/api/v1/admin-users/{id}` | Получить администратора | `admin`, `readonly` |
| `PUT` | `/api/v1/admin-users/{id}` | Обновить администратора | `admin` |
| `DELETE` | `/api/v1/admin-users/{id}` | Удалить администратора | `admin` |
| `POST` | `/api/v1/admin-users/{id}/reset-password` | Сброс пароля | `admin` |
| `POST` | `/api/v1/admin-users/{id}/unlock` | Разблокировать аккаунт | `admin` |

### Service Accounts (6 endpoints)

| Метод | Endpoint | Назначение | RBAC |
|-------|----------|------------|------|
| `POST` | `/api/v1/service-accounts` | Создать SA | `admin` |
| `GET` | `/api/v1/service-accounts` | Список SA | `admin`, `readonly` |
| `GET` | `/api/v1/service-accounts/{id}` | Получить SA | `admin`, `readonly` |
| `PUT` | `/api/v1/service-accounts/{id}` | Обновить SA | `admin` |
| `DELETE` | `/api/v1/service-accounts/{id}` | Удалить SA | `admin` |
| `POST` | `/api/v1/service-accounts/{id}/rotate-secret` | Ротация secret | `admin` |

### Storage Elements (7 endpoints)

| Метод | Endpoint | Назначение | RBAC |
|-------|----------|------------|------|
| `POST` | `/api/v1/storage-elements/discover` | Предпросмотр SE | `admin`, `readonly` |
| `POST` | `/api/v1/storage-elements` | Регистрация SE (+ full sync) | `admin` |
| `GET` | `/api/v1/storage-elements` | Список SE (фильтры: mode, status) | `admin`, `readonly`, SA `storage:read` |
| `GET` | `/api/v1/storage-elements/{id}` | Получить SE | `admin`, `readonly`, SA `storage:read` |
| `PUT` | `/api/v1/storage-elements/{id}` | Обновить SE (name, url) | `admin` |
| `DELETE` | `/api/v1/storage-elements/{id}` | Удалить SE из реестра | `admin` |
| `POST` | `/api/v1/storage-elements/{id}/sync` | Синхронизация SE | `admin`, SA `storage:write` |

### JWT Keys (2 endpoints)

| Метод | Endpoint | Назначение | RBAC |
|-------|----------|------------|------|
| `GET` | `/api/v1/jwt-keys/status` | Статус ключей | `admin` |
| `POST` | `/api/v1/jwt-keys/rotate` | Ротация ключей | `admin` |

### Files Registry (5 endpoints)

| Метод | Endpoint | Назначение | Аутентификация |
|-------|----------|------------|----------------|
| `POST` | `/api/v1/files` | Регистрация файла (от Ingester) | SA `files:write` |
| `GET` | `/api/v1/files` | Список файлов (фильтры: status, retention, SE, uploaded_by) | SA `files:read`, `admin`, `readonly` |
| `GET` | `/api/v1/files/{file_id}` | Метаданные файла | SA `files:read`, `admin`, `readonly` |
| `PUT` | `/api/v1/files/{file_id}` | Обновление метаданных | SA `files:write`, `admin` |
| `DELETE` | `/api/v1/files/{file_id}` | Soft delete | SA `files:write`, `admin` |

### Health (3 endpoints)

| Метод | Endpoint | Назначение | Аутентификация |
|-------|----------|------------|----------------|
| `GET` | `/health/live` | Liveness probe (процесс жив) | без аутентификации |
| `GET` | `/health/ready` | Readiness probe (PostgreSQL, JWT keys) | без аутентификации |
| `GET` | `/metrics` | Prometheus metrics | без аутентификации |

---

## 6. Фоновые задачи

### Периодическая синхронизация файлового реестра

Admin Module запускает фоновую задачу, которая с заданным интервалом (default
1 час) синхронизирует файловый реестр с каждым зарегистрированным SE.

**Алгоритм:**

1. Получить список зарегистрированных SE со статусом `online`
2. Для каждого SE **параллельно**:
   a. Запросить `GET /api/v1/info` — обновить mode, status, capacity
   b. Постранично вычитать `GET /api/v1/files` (пагинация: limit/offset)
   c. Для каждого файла из ответа SE:
      - Если файл есть в реестре — обновить метаданные (status, tags, description)
      - Если файла нет в реестре — добавить запись
   d. Файлы в реестре, привязанные к этому SE, но отсутствующие в ответе —
      пометить как `deleted`
   e. Обновить `last_sync_at` и `last_file_sync_at`

### Очистка истёкших JWT refresh tokens

Периодическая очистка истёкших refresh tokens из базы данных (если хранятся).

### Очистка истёкших блокировок

Периодическая очистка истёкших записей о блокировке аккаунтов (автоматическая
разблокировка по таймеру выполняется при проверке во время login, очистка —
для порядка в БД).

---

## 7. Инициализация (Bootstrap)

При первом запуске Admin Module должен автоматически выполнить начальную
настройку:

1. **Применить миграции БД** — создать таблицы (admin_users, service_accounts,
   storage_elements, file_registry, jwt_keys и др.)
2. **Сгенерировать RSA key pair** — если ключ не предоставлен через env/файл,
   сгенерировать и сохранить в БД
3. **Создать начального администратора** — если таблица admin_users пуста,
   создать администратора из env-переменных `AM_INIT_ADMIN_USERNAME`
   и `AM_INIT_ADMIN_PASSWORD` с ролью `admin`

---

## 8. Конфигурация

Все параметры задаются через переменные окружения.

### Сервер

| Переменная | Обязательная | По умолчанию | Описание |
|------------|:------------:|--------------|----------|
| `AM_PORT` | нет | `8000` | Порт HTTP-сервера (диапазон 8000-8009) |
| `AM_LOG_LEVEL` | нет | `info` | Уровень логирования (`debug`, `info`, `warn`, `error`) |
| `AM_LOG_FORMAT` | нет | `json` | Формат логов (`json` — production, `text` — development) |

### PostgreSQL

| Переменная | Обязательная | По умолчанию | Описание |
|------------|:------------:|--------------|----------|
| `AM_DB_HOST` | да | — | Хост PostgreSQL |
| `AM_DB_PORT` | нет | `5432` | Порт PostgreSQL |
| `AM_DB_NAME` | да | — | Имя базы данных |
| `AM_DB_USER` | да | — | Пользователь БД |
| `AM_DB_PASSWORD` | да | — | Пароль БД |
| `AM_DB_SSL_MODE` | нет | `disable` | SSL режим (`disable`, `require`, `verify-ca`, `verify-full`) |

### JWT

| Переменная | Обязательная | По умолчанию | Описание |
|------------|:------------:|--------------|----------|
| `AM_JWT_PRIVATE_KEY_PATH` | нет | — | Путь к RSA private key (PEM). Если не указан — генерируется и хранится в БД |
| `AM_JWT_ACCESS_TTL` | нет | `30m` | TTL access token для Admin Users (Go duration) |
| `AM_JWT_REFRESH_TTL` | нет | `24h` | TTL refresh token для Admin Users (Go duration) |
| `AM_JWT_SA_ACCESS_TTL` | нет | `1h` | TTL access token для Service Accounts (Go duration) |
| `AM_JWT_KEY_GRACE_PERIOD` | нет | `60m` | Grace period при ротации ключей (Go duration) |

### Безопасность

| Переменная | Обязательная | По умолчанию | Описание |
|------------|:------------:|--------------|----------|
| `AM_LOCK_MAX_ATTEMPTS` | нет | `5` | Максимум неудачных попыток входа до блокировки |
| `AM_LOCK_DURATION` | нет | `15m` | Длительность блокировки аккаунта (Go duration) |
| `AM_SA_SECRET_EXPIRATION_DAYS` | нет | `90` | Срок действия secret SA в днях (0 — без истечения) |
| `AM_PASSWORD_MIN_LENGTH` | нет | `8` | Минимальная длина пароля |

### Синхронизация

| Переменная | Обязательная | По умолчанию | Описание |
|------------|:------------:|--------------|----------|
| `AM_DEPHEALTH_CHECK_INTERVAL` | нет | `15s` | Интервал проверки зависимостей topologymetrics (Go duration) |
| `AM_SYNC_INTERVAL` | нет | `1h` | Интервал периодической синхронизации SE (Go duration) |
| `AM_SYNC_PAGE_SIZE` | нет | `1000` | Размер страницы при sync файлов с SE |
| `AM_SE_CA_CERT_PATH` | нет | — | Путь к CA-сертификату для TLS-соединений с SE |

### Инициализация

| Переменная | Обязательная | По умолчанию | Описание |
|------------|:------------:|--------------|----------|
| `AM_INIT_ADMIN_USERNAME` | нет | `admin` | Username начального администратора |
| `AM_INIT_ADMIN_PASSWORD` | нет | — | Пароль начального администратора (обязателен при первом запуске) |

---

## 9. Синхронизация файлового реестра

`attr.json` файлы на Storage Elements — **единственный источник истины**
для метаданных файлов. PostgreSQL файловый реестр — вторичный индекс.

### 4 механизма синхронизации

| # | Механизм | Триггер | Описание |
|---|----------|---------|----------|
| 1 | Периодический | Таймер (`AM_SYNC_INTERVAL`) | Фоновая задача, все SE параллельно |
| 2 | Ленивая очистка | 404 от SE при скачивании | Query Module помечает файл как `deleted` |
| 3 | Ручной | Администратор из UI | `POST /storage-elements/{id}/sync` |
| 4 | Full sync | Регистрация SE / восстановление из backup | Автоматически при `POST /storage-elements` |

### SyncResponse

Ответ sync содержит две части:

- **storage_element** — обновлённые данные SE (mode, status, capacity)
- **file_sync** — результат синхронизации файлов:
  - `files_on_se` — общее количество файлов на SE
  - `files_added` — новых файлов добавлено в реестр
  - `files_updated` — файлов обновлено
  - `files_marked_deleted` — файлов помечено как deleted
  - `started_at`, `completed_at` — временные метки

### Восстановление после сбоя PostgreSQL

1. Восстановить PostgreSQL из backup (или развернуть чистый)
2. Административные данные (users, SA, JWT keys) — из backup
3. Файловый реестр — автоматический full sync при подключении каждого SE
4. Система полностью работоспособна после завершения sync всех SE

---

## 10. Схема базы данных (концептуальная)

```text
┌──────────────────┐     ┌─────────────────────┐
│   admin_users    │     │  service_accounts   │
│──────────────────│     │─────────────────────│
│ id (UUID, PK)    │     │ id (UUID, PK)       │
│ username         │     │ client_id           │
│ password_hash    │     │ client_secret_hash  │
│ email            │     │ name                │
│ role             │     │ description         │
│ is_locked        │     │ scopes (text[])     │
│ locked_until     │     │ status              │
│ failed_attempts  │     │ secret_expires_at   │
│ last_login_at    │     │ secret_expiration_  │
│ created_at       │     │   days              │
│ updated_at       │     │ created_at          │
└──────────────────┘     │ updated_at          │
                         └─────────────────────┘

┌──────────────────────┐     ┌──────────────────────┐
│  storage_elements    │     │    file_registry     │
│──────────────────────│     │──────────────────────│
│ id (UUID, PK)        │     │ file_id (UUID, PK)   │
│ name                 │     │ original_filename    │
│ url                  │     │ content_type         │
│ storage_id           │     │ size                 │
│ mode                 │     │ checksum             │
│ status               │     │ storage_element_id   │──▶ storage_elements.id
│ capacity_bytes       │     │ uploaded_by          │
│ used_bytes           │     │ uploaded_at          │
│ available_bytes      │     │ description          │
│ last_sync_at         │     │ tags (text[])        │
│ last_file_sync_at    │     │ status               │
│ created_at           │     │ retention_policy     │
│ updated_at           │     │ ttl_days             │
└──────────────────────┘     │ expires_at           │
                             │ created_at           │
┌──────────────────────┐     │ updated_at           │
│     jwt_keys         │     └──────────────────────┘
│──────────────────────│
│ id (PK)              │
│ kid                  │
│ private_key_pem      │
│ public_key_pem       │
│ algorithm            │
│ is_active            │
│ expires_at           │
│ created_at           │
└──────────────────────┘
```

---

## 11. Сборка и запуск

### Docker

```bash
# Сборка образа
docker build -t harbor.kryukov.lan/library/admin-module:v1.0.0 \
  -f admin-module/Dockerfile .

# Запуск контейнера
docker run -d \
  --name admin-module \
  -p 8000:8000 \
  -e AM_DB_HOST=postgres \
  -e AM_DB_PORT=5432 \
  -e AM_DB_NAME=artsore \
  -e AM_DB_USER=artsore \
  -e AM_DB_PASSWORD=secret \
  -e AM_INIT_ADMIN_PASSWORD=changeme123 \
  -e AM_SE_CA_CERT_PATH=/certs/ca.crt \
  -v /path/to/ca-certs:/certs:ro \
  harbor.kryukov.lan/library/admin-module:v1.0.0
```

### Kubernetes (Helm)

```bash
# Установка через Helm chart
helm install admin-module ./admin-module/chart \
  --set db.host=postgresql.artsore.svc \
  --set db.name=artsore \
  --set db.user=artsore \
  --set db.password=secret \
  --set initAdmin.password=changeme123 \
  --set seCaCert.secretName=se-ca-cert
```

---

## 12. Мониторинг зависимостей (topologymetrics)

Admin Module интегрируется с SDK
[topologymetrics](https://github.com/BigKAA/topologymetrics)
для мониторинга здоровья внешних зависимостей через Prometheus-метрики.

### 12.1. Отслеживаемые зависимости

| Зависимость | Тип проверки | Критичность |
|-------------|-------------|:-----------:|
| PostgreSQL | SQL (`SELECT 1` через пул) | да |

### 12.2. Экспортируемые метрики

| Метрика | Тип | Описание |
|---------|-----|----------|
| `app_dependency_health` | Gauge | 1 = доступен, 0 = недоступен |
| `app_dependency_latency_seconds` | Histogram | Время проверки |
| `app_dependency_status` | Gauge | Категория результата (ok, timeout, error...) |
| `app_dependency_status_detail` | Gauge | Детальная причина |

Метрики доступны на endpoint `/metrics` вместе с остальными
Prometheus-метриками Admin Module.

### 12.3. Интеграция в коде

```go
import (
    "github.com/BigKAA/topologymetrics/sdk-go/dephealth"
    "github.com/BigKAA/topologymetrics/sdk-go/dephealth/contrib/sqldb"
    _ "github.com/BigKAA/topologymetrics/sdk-go/dephealth/checks"
)

dh, err := dephealth.New("admin-module", "artsore",
    dephealth.WithCheckInterval(cfg.DephealthCheckInterval),
    sqldb.FromDB("postgresql", db,
        dephealth.FromURL(cfg.DatabaseURL),
        dephealth.Critical(true),
    ),
)
dh.Start(ctx)
defer dh.Stop()
```

---

## 13. Порты

| Порт | Назначение |
|------|------------|
| 8000-8009 | HTTP API Admin Module (по одному порту на экземпляр) |

По умолчанию используется порт `8000`. При запуске нескольких экземпляров
(horizontal scaling) — каждый экземпляр работает с одной БД,
балансировка через Kubernetes Service.
