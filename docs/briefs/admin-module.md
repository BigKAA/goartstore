# Admin Module — Бриф модуля

**Версия**: 0.1.0
**Дата**: 2026-02-22
**Статус**: Draft
**Порты**: 8000-8009

---

## 1. Назначение модуля

Admin Module — управляющий модуль системы Artsore, отвечающий за управление
Artsore-специфичными сущностями: реестр Storage Elements, файловый реестр,
локальные дополнения ролей пользователей, синхронизация SA с IdP.

**Admin Module не является auth-сервером.** Аутентификация и выдача JWT
полностью делегированы внешнему Identity Provider (Keycloak). Admin Module
получает JWT от API Gateway, проверяет claims (роли, scopes) и принимает
решения по авторизации для своих endpoints.

### Ключевые концепции

**Внешний Identity Provider (Keycloak)** — единый источник аутентификации
для всей системы Artsore. Keycloak выдаёт JWT для всех субъектов:
Admin Users (H2M) и Service Accounts (M2M). Keycloak управляет RSA-ключами,
JWKS endpoint, token lifecycle, brute force protection. Поддерживает
федерацию с LDAP, Active Directory, внешними OAuth/OIDC провайдерами.

**API Gateway** — единая точка входа для всех клиентских запросов к
control plane сервисам (Admin Module, Ingester, Query Module). Выполняет
TLS termination, CORS, routing и валидацию JWT (подпись, expiration).
Технология зависит от окружения: Envoy Gateway в Kubernetes, Nginx /
OpenResty / другой reverse proxy вне Kubernetes. Сервисы не зависят от
конкретной реализации прокси.

**Claims-based авторизация** — Admin Module извлекает из JWT claims
(`roles`, `scopes`, `sub`, `groups`) и проверяет их для каждого endpoint.
Валидация подписи JWT выполняется на уровне API Gateway; Admin Module
дополнительно может валидировать подпись как fallback.

**RBAC** — двухуровневая авторизация:

- **Scopes** (M2M) — для Service Accounts: `files:read`, `files:write`,
  `storage:read`, `storage:write`, `admin:read`, `admin:write`
- **Roles** (H2M) — для Admin Users: `admin` (полный доступ),
  `readonly` (только чтение)
- В будущем предусмотрено расширение ролевой модели (гранулярные роли:
  `storage-admin`, `file-admin` и т.д.)

**Локальные дополнения ролей** — роли пользователей определяются маппингом
групп из IdP (Keycloak). Admin Module может **дополнить** (но не понизить)
роль конкретного пользователя локально. Пример: пользователь в группе
`artsore-viewers` (→ `readonly`) может получить локальное дополнение до
`admin` в Admin Module. Обратное невозможно — если IdP даёт `admin`,
Admin Module не может понизить до `readonly`.

**Параллельное управление SA** — Service Accounts могут создаваться как
через Keycloak, так и через Admin Module. Периодическая синхронизация
обеспечивает согласованность между Keycloak и локальной БД Admin Module.

**File Registry** — реестр файлов в PostgreSQL. Вторичный индекс метаданных,
восстанавливаемый из `attr.json` файлов на Storage Elements. Ingester
регистрирует файлы после загрузки, Admin Module синхронизирует реестр
с SE периодически и по запросу.

**Shared PostgreSQL** — Admin Module и Query Module используют один PostgreSQL
instance. Admin Module владеет таблицами и пишет. Query Module добавляет
FTS-индексы и читает.

---

## 2. Топология

Control plane сервисы (Admin Module, Ingester, Query Module) располагаются
за API Gateway. Все клиентские запросы проходят через gateway. Storage
Elements доступны напрямую по токену (без gateway), физически могут
находиться где угодно.

```text
┌─────────────────────────────────────────────────────────────────┐
│                    Control Plane                                │
│                                                                 │
│  ┌────────────┐                                                 │
│  │  Keycloak  │◀─── OIDC/LDAP federation ──▶ [LDAP / AD /     │
│  │  (IdP)     │                               OAuth providers] │
│  └─────┬──────┘                                                 │
│        │ JWKS                                                   │
│        ▼                                                        │
│  ┌─────────────────────────────────────────────────────────┐    │
│  │         API Gateway (Envoy / Nginx)                     │    │
│  │  TLS termination, CORS, JWT validation, routing         │    │
│  └───┬─────────────┬──────────────┬────────────────────────┘    │
│      │             │              │                              │
│      ▼             ▼              ▼                              │
│  ┌────────┐  ┌──────────┐  ┌───────────┐   ┌──────────────┐    │
│  │ Admin  │  │ Ingester │  │  Query    │   │  PostgreSQL  │    │
│  │ Module │  │ Module   │  │  Module   │   │  (shared)    │    │
│  └───┬────┘  └────┬─────┘  └─────┬─────┘   └──────────────┘    │
│      │            │              │                               │
│      │    ┌───────┘              │                               │
│      │    │    ┌─────────────────┘                               │
│      ▼    ▼    ▼                                                 │
│     [прямой доступ к SE по JWT]                                  │
│                                                                 │
│  /health/*, /metrics — напрямую к pod (без gateway)             │
└─────────────────────────────────────────────────────────────────┘
          │  WAN / TLS (прямой доступ по JWT)
          ▼
    ┌──────────┐    ┌──────────┐    ┌──────────┐
    │ SE #1    │    │ SE #2    │    │ SE #N    │
    │ (remote) │    │ (remote) │    │ (remote) │
    └──────────┘    └──────────┘    └──────────┘
```

**Следствия:**

- Входящие запросы от клиентов (встроенный Admin UI, SA) проходят через API Gateway —
  TLS termination, CORS и JWT validation на уровне gateway
- API Gateway проверяет валидность JWT (подпись через JWKS от Keycloak,
  expiration). Сервисы проверяют claims (роли, scopes) для авторизации
- Исходящие запросы к Storage Elements — через TLS напрямую (без gateway).
  SE валидирует JWT самостоятельно через JWKS Keycloak
- Health endpoints (`/health/*`) и metrics (`/metrics`) доступны напрямую
  к pod, минуя gateway (для Kubernetes probes и Prometheus scrape)
- Keycloak должен быть доступен из всех мест, откуда нужна валидация JWT
  (JWKS endpoint): API Gateway, Storage Elements

### Внешний доступ

| Домен | IP | Назначение |
|-------|-----|-----------|
| `artstore.kryukov.lan` | `192.168.218.180` | Внешний доступ к API Gateway (Admin UI встроен в Admin Module, API) |

API Gateway принимает запросы на `artstore.kryukov.lan` и маршрутизирует
к backend-сервисам. Keycloak доступен через отдельный домен или path
на том же gateway (определяется при развёртывании).

### Требования к API Gateway

API Gateway — абстрактная роль, не конкретная технология:

| Окружение | Реализация | JWT validation |
|-----------|-----------|----------------|
| Kubernetes | Envoy Gateway (`SecurityPolicy`) | Встроенный JWT provider |
| Без Kubernetes | Nginx / OpenResty | `auth_request` + token introspection или `lua-resty-jwt` |
| Без Kubernetes | Traefik / Caddy | Встроенные JWT middleware |

Требования к любой реализации:

- TLS termination
- JWT signature validation (через JWKS от Keycloak)
- Forwarding claims в заголовках к backend-сервисам
- CORS headers
- Routing по path prefix к нужному сервису

---

## 3. Зависимости

### Инфраструктурные

| Зависимость | Назначение |
|-------------|------------|
| PostgreSQL | Хранение SA (локальная копия), SE registry, file registry, role overrides |
| Keycloak | Аутентификация, выдача JWT, управление пользователями и SA, JWKS, LDAP/OAuth federation |
| API Gateway | TLS termination, JWT validation, routing (Envoy / Nginx) |

Admin Module не использует Redis или другие внешние хранилища.
Admin Module не управляет RSA-ключами и не выдаёт JWT.

### Межмодульные

| Модуль | Направление | Назначение |
|--------|-------------|------------|
| Keycloak | Admin ↔ Keycloak | Sync SA (Keycloak Admin API), чтение пользователей, маппинг групп |
| Storage Element | Admin → SE | Discovery (`GET /api/v1/info`), sync (`GET /api/v1/files`), mode transition. Доступ по JWT, напрямую (без gateway) |
| Ingester Module | Ingester → Admin | Регистрация файлов (`POST /files`), список SE (`GET /storage-elements`). Через API Gateway |
| Query Module | Query → Admin | Чтение file registry (через shared PostgreSQL), список SE (`GET /storage-elements`). Через API Gateway |
| Admin UI (встроен) | Часть Admin Module | Веб-интерфейс администратора, встроен непосредственно в Admin Module |
| SE, API Gateway | → Keycloak | JWKS endpoint для валидации JWT |

---

## 4. Аутентификация и авторизация

### Общая схема

Аутентификация всех субъектов (Admin Users и Service Accounts) выполняется
**Keycloak**. Admin Module не выдаёт токены, не хранит пароли пользователей,
не управляет JWT-ключами.

```text
┌────────────┐   credentials   ┌──────────┐   JWT    ┌─────────────┐  claims  ┌────────────┐
│   Client   │ ──────────────▶ │ Keycloak │ ──────▶ │ API Gateway │ ──────▶ │   Admin    │
│ (UI / SA)  │                 │  (IdP)   │         │ (JWT valid.)│         │   Module   │
└────────────┘                 └──────────┘         └─────────────┘         │ (authz)    │
                                                                            └────────────┘
```

1. Клиент получает JWT от Keycloak (login или client_credentials)
2. Клиент отправляет запрос с JWT в `Authorization: Bearer <token>`
3. API Gateway проверяет подпись и expiration через JWKS Keycloak
4. Admin Module извлекает claims из JWT и проверяет авторизацию (роли, scopes)

### Admin Users (H2M)

| Аспект | Описание |
|--------|----------|
| Flow | OIDC Authorization Code (через Keycloak login page) или Direct Grant |
| Токен | JWT от Keycloak, содержит `sub`, `realm_roles`, `groups` |
| TTL | Настраивается в Keycloak (рекомендуемо: access 30 мин, refresh 24 ч) |
| Роли | Определяются маппингом Keycloak groups → Artsore roles |
| Блокировка | Brute Force Detection в Keycloak |
| Пароли | Управляются в Keycloak |

### Service Accounts (M2M)

| Аспект | Описание |
|--------|----------|
| Flow | OAuth 2.0 Client Credentials (через Keycloak) |
| Токен | JWT от Keycloak, содержит `client_id`, `scope` |
| TTL | Настраивается в Keycloak (рекомендуемо: access 1 час) |
| Refresh token | Не выдаётся — SA повторно запрашивает по credentials |
| Управление | Параллельно в Keycloak и Admin Module, с периодической синхронизацией |

**Scopes:**

| Scope | Назначение |
|-------|------------|
| `files:read` | Чтение метаданных и скачивание файлов |
| `files:write` | Загрузка, обновление и удаление файлов |
| `storage:read` | Чтение информации о Storage Elements |
| `storage:write` | Управление SE (sync, mode transition) |
| `admin:read` | Чтение административных данных |
| `admin:write` | Управление пользователями и SA |

### Identity Federation (Keycloak)

Keycloak поддерживает подключение внешних источников пользователей:

| Провайдер | Механизм | Описание |
|-----------|----------|----------|
| LDAP / Active Directory | User Federation | Import + Sync пользователей и групп. Write-back в будущем |
| OAuth 2.0 / OIDC провайдер | Identity Brokering | Google, Azure AD, GitHub и др. |
| Локальная база Keycloak | Built-in | Пользователи создаются непосредственно в Keycloak |

**Маппинг групп → роли:**

Группы из внешних провайдеров (LDAP, AD) маппятся на Keycloak roles,
которые передаются в JWT claims. Admin Module интерпретирует эти claims.

| Keycloak группа / роль | Artsore роль | Описание |
|------------------------|-------------|----------|
| `artsore-admins` | `admin` | Полный доступ |
| `artsore-viewers` | `readonly` | Только чтение |

Маппинг настраивается в Keycloak (Group Mapper / Role Mapper).
Конкретные имена групп конфигурируемы.

**LDAP-интеграция — текущий уровень:**

- **Import + Sync** — пользователи и группы импортируются из LDAP
  в Keycloak, периодическая синхронизация
- **Write-back** — планируется в будущем (изменения в Keycloak
  синхронизируются обратно в LDAP)

### Локальные дополнения ролей

Admin Module хранит в своей БД таблицу `role_overrides` — локальные
дополнения ролей для конкретных пользователей.

**Правила:**

- Роли из IdP — baseline (минимум привилегий пользователя)
- Локально можно **только повысить** роль, не понизить
- Итоговая роль = max(роль из IdP, локальное дополнение)
- Если пользователь в нескольких группах IdP — объединение (максимальная
  привилегия)
- При будущем расширении ролевой модели: итоговый набор привилегий =
  объединение привилегий из IdP + локальные дополнения

**Пример:**

| Пользователь | Группа IdP | Роль IdP | Локальное дополнение | Итоговая роль |
|-------------|-----------|---------|---------------------|--------------|
| alice | artsore-admins | admin | — | admin |
| bob | artsore-viewers | readonly | admin | admin |
| carol | artsore-viewers | readonly | — | readonly |
| dave | artsore-admins, artsore-viewers | admin (max) | — | admin |

### Admin UI — аутентификация через Keycloak (OIDC)

Admin UI встроен непосредственно в Admin Module (не отдельный сервис).
Технологический стек UI определится позже.

Аутентификация использует стандартный **Authorization Code + PKCE** flow:

```text
1. Пользователь открывает Admin UI в браузере (обслуживается Admin Module)
2. Admin Module обнаруживает отсутствие сессии
   → redirect на Keycloak login page
3. Пользователь вводит логин/пароль на странице Keycloak
   (форма логина кастомизируется темой Keycloak)
4. Keycloak redirect обратно на Admin Module с authorization code
5. Admin Module обменивает code на JWT через Keycloak token endpoint
6. Пользователь аутентифицирован, JWT используется для запросов к API
```

Пользователь видит форму логина **Keycloak**, а не Admin Module.
Внешний вид формы настраивается через Keycloak Themes.

### Keycloak Realm — конфигурация при первом развёртывании

Конфигурация realm `artsore` выполняется **один раз** при первом
развёртывании Keycloak. Параметры фиксированы и документированы
для воспроизводимости.

| Параметр | Значение | Описание |
|----------|---------|----------|
| Realm name | `artsore` | Изолированный realm для системы Artsore |
| Client для Admin UI (встроен в Admin Module) | `artsore-admin-ui` (public, Authorization Code + PKCE) | Аутентификация администраторов через браузер |
| Client для Ingester | `artsore-ingester` (confidential, Client Credentials) | M2M токены для Ingester Module |
| Client для Query | `artsore-query` (confidential, Client Credentials) | M2M токены для Query Module |
| Client для Admin Module | `artsore-admin-module` (confidential, Client Credentials) | Доступ к Keycloak Admin API для синхронизации SA |
| Groups | `artsore-admins`, `artsore-viewers` | Группы для маппинга на роли |
| Group Mapper | groups → `roles` claim в JWT | Protocol Mapper типа «Group Membership» |
| Brute Force Detection | Enabled (5 попыток, 15 мин блокировка) | Защита от перебора паролей |
| Начальный администратор | Создаётся при развёртывании realm | Пользователь с ролью `admin` в группе `artsore-admins` |

Рекомендуется использовать Keycloak Realm Export/Import для
автоматизации развёртывания (JSON-файл конфигурации realm).

---

## 5. API endpoints

29 endpoints, сгруппированных по назначению. Полная спецификация —
[admin-module-openapi.yaml](../api-contracts/admin-module-openapi.yaml).

Все endpoints (кроме Health) находятся за API Gateway и требуют валидный
JWT от Keycloak. API Gateway проверяет подпись и expiration. Admin Module
проверяет claims (роли, scopes).

### Admin Auth (1 endpoint)

| Метод | Endpoint | Назначение | Аутентификация |
|-------|----------|------------|----------------|
| `GET` | `/api/v1/admin-auth/me` | Текущий пользователь (данные из JWT + локальные дополнения) | JWT (любая роль) |

Аутентификация (login, refresh, change-password) выполняется напрямую
через Keycloak. Admin Module не участвует в этих операциях.

### Admin Users (5 endpoints)

| Метод | Endpoint | Назначение | RBAC |
|-------|----------|------------|------|
| `GET` | `/api/v1/admin-users` | Список пользователей (из Keycloak + локальные дополнения) | `admin`, `readonly` |
| `GET` | `/api/v1/admin-users/{id}` | Получить пользователя | `admin`, `readonly` |
| `PUT` | `/api/v1/admin-users/{id}` | Обновить локальные дополнения (роль) | `admin` |
| `DELETE` | `/api/v1/admin-users/{id}` | Удалить локальные дополнения | `admin` |
| `POST` | `/api/v1/admin-users/{id}/role-override` | Установить/изменить локальное дополнение роли | `admin` |

Создание и удаление пользователей, сброс паролей, разблокировка —
выполняются в Keycloak (через Keycloak Admin Console или API).

### Service Accounts (6 endpoints)

| Метод | Endpoint | Назначение | RBAC |
|-------|----------|------------|------|
| `POST` | `/api/v1/service-accounts` | Создать SA (+ синхронизация в Keycloak) | `admin` |
| `GET` | `/api/v1/service-accounts` | Список SA | `admin`, `readonly` |
| `GET` | `/api/v1/service-accounts/{id}` | Получить SA | `admin`, `readonly` |
| `PUT` | `/api/v1/service-accounts/{id}` | Обновить SA (+ синхронизация в Keycloak) | `admin` |
| `DELETE` | `/api/v1/service-accounts/{id}` | Удалить SA (+ синхронизация в Keycloak) | `admin` |
| `POST` | `/api/v1/service-accounts/{id}/rotate-secret` | Ротация secret (+ синхронизация в Keycloak) | `admin` |

SA управляются параллельно: можно создать в Keycloak или в Admin Module.
Периодическая фоновая синхронизация обеспечивает согласованность.

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

### Files Registry (5 endpoints)

| Метод | Endpoint | Назначение | Аутентификация |
|-------|----------|------------|----------------|
| `POST` | `/api/v1/files` | Регистрация файла (от Ingester) | SA `files:write` |
| `GET` | `/api/v1/files` | Список файлов (фильтры: status, retention, SE, uploaded_by) | SA `files:read`, `admin`, `readonly` |
| `GET` | `/api/v1/files/{file_id}` | Метаданные файла | SA `files:read`, `admin`, `readonly` |
| `PUT` | `/api/v1/files/{file_id}` | Обновление метаданных | SA `files:write`, `admin` |
| `DELETE` | `/api/v1/files/{file_id}` | Soft delete | SA `files:write`, `admin` |

### IdP Status (2 endpoints)

| Метод | Endpoint | Назначение | RBAC |
|-------|----------|------------|------|
| `GET` | `/api/v1/idp/status` | Статус подключения к Keycloak, информация о realm | `admin` |
| `POST` | `/api/v1/idp/sync-sa` | Принудительная синхронизация SA с Keycloak | `admin` |

### Health (3 endpoints)

| Метод | Endpoint | Назначение | Аутентификация |
|-------|----------|------------|----------------|
| `GET` | `/health/live` | Liveness probe (процесс жив) | без аутентификации |
| `GET` | `/health/ready` | Readiness probe (PostgreSQL + Keycloak) | без аутентификации |
| `GET` | `/metrics` | Prometheus metrics | без аутентификации |

Health и metrics endpoints доступны напрямую к pod, **минуя API Gateway**.

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

### Периодическая синхронизация SA с Keycloak

Admin Module запускает фоновую задачу, которая с заданным интервалом (default
15 минут) синхронизирует Service Accounts между локальной БД и Keycloak.

**Алгоритм:**

1. Получить список clients из Keycloak (через Admin API) с фильтром
   по `clientId` prefix `sa_*`
2. Получить список SA из локальной БД
3. Для каждого SA:
   - Есть в Keycloak, нет локально → создать локальную запись
   - Есть локально, нет в Keycloak → создать client в Keycloak
   - Есть в обоих → сравнить scopes, обновить при расхождении
     (стратегия: последнее изменение побеждает, по `updated_at`)
4. Обновить `last_sa_sync_at`

---

## 7. Инициализация (Bootstrap)

При первом запуске Admin Module должен автоматически выполнить начальную
настройку:

1. **Применить миграции БД** — создать таблицы (service_accounts,
   storage_elements, file_registry, role_overrides и др.)
2. **Проверить подключение к Keycloak** — убедиться что realm `artsore`
   существует и Admin Module имеет доступ к Keycloak Admin API
3. **Выполнить начальную синхронизацию SA** — если есть clients в Keycloak
   с prefix `sa_*`, импортировать их в локальную БД

Начальный администратор создаётся **в Keycloak** (при развёртывании Keycloak
realm). Admin Module не создаёт пользователей.

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

### Keycloak

| Переменная | Обязательная | По умолчанию | Описание |
|------------|:------------:|--------------|----------|
| `AM_KEYCLOAK_URL` | да | — | URL Keycloak (например `https://keycloak.kryukov.lan`) |
| `AM_KEYCLOAK_REALM` | нет | `artsore` | Имя realm в Keycloak |
| `AM_KEYCLOAK_CLIENT_ID` | да | — | Client ID для доступа к Keycloak Admin API |
| `AM_KEYCLOAK_CLIENT_SECRET` | да | — | Client Secret для доступа к Keycloak Admin API |
| `AM_KEYCLOAK_SA_PREFIX` | нет | `sa_` | Prefix для идентификации SA clients в Keycloak |

### JWT (валидация)

| Переменная | Обязательная | По умолчанию | Описание |
|------------|:------------:|--------------|----------|
| `AM_JWT_ISSUER` | нет | (из Keycloak URL) | Expected issuer в JWT (для дополнительной валидации) |
| `AM_JWT_JWKS_URL` | нет | (из Keycloak URL) | JWKS URL для валидации JWT (fallback, основная валидация на gateway) |
| `AM_JWT_ROLES_CLAIM` | нет | `realm_access.roles` | JSON path к ролям в JWT claims |
| `AM_JWT_GROUPS_CLAIM` | нет | `groups` | JSON path к группам в JWT claims |

### Синхронизация

| Переменная | Обязательная | По умолчанию | Описание |
|------------|:------------:|--------------|----------|
| `AM_DEPHEALTH_CHECK_INTERVAL` | нет | `15s` | Интервал проверки зависимостей topologymetrics (Go duration) |
| `AM_SYNC_INTERVAL` | нет | `1h` | Интервал периодической синхронизации SE (Go duration) |
| `AM_SYNC_PAGE_SIZE` | нет | `1000` | Размер страницы при sync файлов с SE |
| `AM_SA_SYNC_INTERVAL` | нет | `15m` | Интервал синхронизации SA с Keycloak (Go duration) |
| `AM_SE_CA_CERT_PATH` | нет | — | Путь к CA-сертификату для TLS-соединений с SE |

### Роли — маппинг

| Переменная | Обязательная | По умолчанию | Описание |
|------------|:------------:|--------------|----------|
| `AM_ROLE_ADMIN_GROUPS` | нет | `artsore-admins` | Группы IdP, маппящиеся на роль `admin` (через запятую) |
| `AM_ROLE_READONLY_GROUPS` | нет | `artsore-viewers` | Группы IdP, маппящиеся на роль `readonly` (через запятую) |

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
2. SA — восстановятся из Keycloak при следующей синхронизации
3. Файловый реестр — автоматический full sync при подключении каждого SE
4. Локальные дополнения ролей — из backup (единственные данные,
   не восстанавливаемые автоматически)
5. Система полностью работоспособна после завершения sync всех SE

---

## 10. Схема базы данных (концептуальная)

```text
┌───────────────────────┐     ┌─────────────────────┐
│   role_overrides      │     │  service_accounts   │
│───────────────────────│     │─────────────────────│
│ id (UUID, PK)         │     │ id (UUID, PK)       │
│ keycloak_user_id      │     │ keycloak_client_id  │
│ username (cached)     │     │ client_id           │
│ additional_role       │     │ name                │
│ created_by            │     │ description         │
│ created_at            │     │ scopes (text[])     │
│ updated_at            │     │ status              │
└───────────────────────┘     │ source (keycloak/   │
                              │   local)            │
                              │ last_synced_at      │
                              │ created_at          │
                              │ updated_at          │
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
│  sync_state          │     └──────────────────────┘
│──────────────────────│
│ id (PK)              │
│ last_sa_sync_at      │
│ last_file_sync_at    │
│ created_at           │
│ updated_at           │
└──────────────────────┘
```

**Убраны по сравнению с v1:**

- `admin_users` — пользователи живут в Keycloak
- `jwt_keys` — ключи управляются Keycloak

**Добавлены:**

- `role_overrides` — локальные дополнения ролей пользователей
- `sync_state` — состояние синхронизации с Keycloak

---

## 11. Сборка и запуск

### Docker

```bash
# Сборка образа
docker build -t harbor.kryukov.lan/library/admin-module:v0.1.0 \
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
  -e AM_KEYCLOAK_URL=https://keycloak.kryukov.lan \
  -e AM_KEYCLOAK_REALM=artsore \
  -e AM_KEYCLOAK_CLIENT_ID=artsore-admin-module \
  -e AM_KEYCLOAK_CLIENT_SECRET=secret \
  -e AM_SE_CA_CERT_PATH=/certs/ca.crt \
  -v /path/to/ca-certs:/certs:ro \
  harbor.kryukov.lan/library/admin-module:v0.1.0
```

### Kubernetes (Helm)

```bash
# Установка через Helm chart
helm install admin-module ./admin-module/chart \
  --set db.host=postgresql.artsore.svc \
  --set db.name=artsore \
  --set db.user=artsore \
  --set db.password=secret \
  --set keycloak.url=https://keycloak.artsore.svc \
  --set keycloak.realm=artsore \
  --set keycloak.clientId=artsore-admin-module \
  --set keycloak.clientSecret=secret \
  --set seCaCert.secretName=se-ca-cert
```

### Порядок развёртывания

1. **PostgreSQL** — БД для Admin Module и Keycloak (или отдельные инстансы)
2. **Keycloak** — настроить realm `artsore`, создать clients, группы,
   начального администратора, настроить LDAP federation (если нужно)
3. **API Gateway** (Envoy / Nginx) — настроить TLS, JWT validation
   через JWKS Keycloak, routing к backend-сервисам
4. **Admin Module** — подключается к PostgreSQL и Keycloak
5. **Ingester, Query Module** — подключаются через API Gateway

---

## 12. Мониторинг зависимостей (topologymetrics)

Admin Module интегрируется с SDK
[topologymetrics](https://github.com/BigKAA/topologymetrics)
для мониторинга здоровья внешних зависимостей через Prometheus-метрики.

### 12.1. Отслеживаемые зависимости

| Зависимость | Тип проверки | Критичность |
|-------------|-------------|:-----------:|
| PostgreSQL | SQL (`SELECT 1` через пул) | да |
| Keycloak | HTTP GET к JWKS endpoint или realm info | да |

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
    dephealth.NewHTTPCheck("keycloak", cfg.KeycloakJWKSURL,
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

---

## Приложение A. Отличия от старого проекта (Python/FastAPI)

| Аспект | Старый проект | Текущая версия |
|--------|--------|--------|
| Аутентификация | Admin Module выдаёт JWT | Keycloak выдаёт JWT |
| JWT ключи | Управляются Admin Module | Управляются Keycloak |
| Пользователи | Хранятся в PostgreSQL (admin_users) | Хранятся в Keycloak, локально — role overrides |
| LDAP/OAuth | Не поддерживается | Через Keycloak Federation |
| SA токены | Admin Module выдаёт | Keycloak выдаёт (Client Credentials) |
| SA управление | Только Admin Module | Параллельно (Admin Module + Keycloak, sync) |
| API Gateway | TLS termination только | TLS + JWT validation + routing |
| Блокировка аккаунтов | Admin Module | Keycloak Brute Force Detection |
| Количество endpoints | 36 | 29 |
| Убранные endpoint-группы | — | Auth OAuth (2), Admin Auth login/refresh/change-password (3), JWT Keys (2) |
| Добавленные endpoint-группы | — | IdP Status (2), role-override (1) |
| Admin UI аутентификация | POST login/password на Admin Module | OIDC Authorization Code + PKCE через Keycloak (UI встроен в Admin Module) |

---

## Приложение B. Закрытые вопросы (решения)

| # | Вопрос | Решение |
|---|--------|---------|
| 1 | Формат JWT claims от Keycloak | Фиксируется при первом развёртывании realm. Описан в разделе «Keycloak Realm — конфигурация при первом развёртывании». Конфигурируется через `AM_JWT_ROLES_CLAIM` и `AM_JWT_GROUPS_CLAIM` |
| 2 | SA sync: конфликт при одновременном изменении | Стратегия `last write wins` по полю `updated_at`. Простая, без сложного merge |
| 3 | Write-back в LDAP | Управление LDAP полностью через Keycloak. Admin Module не взаимодействует с LDAP напрямую |
| 4 | Admin UI — OIDC flow | Authorization Code + PKCE. UI встроен в Admin Module. Пользователь видит форму логина Keycloak (кастомизируется темой) |

## Приложение C. Открытые вопросы

| # | Вопрос | Контекст |
|---|--------|----------|
| 1 | Гранулярные роли — когда и какие? | Текущая модель (admin/readonly) готова к расширению, но конкретные роли не определены |
