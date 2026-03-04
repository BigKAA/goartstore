# Требования: Документация, развёртывание и мониторинг Artstore

[English version (documentation-monitoring-requirements.en.md)](./documentation-monitoring-requirements.en.md)

> Спецификация требований, собранная через brainstorming-сессию.
> Статус: **DRAFT** — на согласовании.

---

## 1. Обзор

Три направления работы:

1. **Документация проекта** — для трёх аудиторий (администраторы, разработчики-интеграторы, операционная команда)
2. **Развёртывание** — umbrella Helm chart + документация для разных масштабов инсталляций
3. **Мониторинг** — Grafana dashboards, ServiceMonitor/Pod annotations, AlertManager rules, topologymetrics

---

## 2. Документация

### 2.1 Целевые аудитории

| Аудитория | Документ | Описание |
|-----------|----------|----------|
| Администраторы | **Admin Guide** | Установка, настройка, управление Keycloak, Admin UI, жизненный цикл SE |
| Разработчики | **Developer Guide** | API-интеграция: upload, download, search. Примеры curl/SDK. Аутентификация |
| Операционная команда | **Operations Guide** | Мониторинг, дашборды, алерты, troubleshooting, рекомендации по backup |

### 2.2 Язык и формат

- **Двуязычная документация**: EN (основной, без суффикса) + RU (суффикс `.ru.md`)
- **Паттерн**: как в проекте [uniproxy](../topologymetrics/uniproxy/):
  - `admin-guide.md` (EN) + `admin-guide.ru.md` (RU)
  - Перекрёстные ссылки в начале каждого файла
  - Полный перевод, не сокращение
- **Формат**: plain Markdown (решение о генераторе сайта — позже)
- **Размещение**: директория `docs/guides/`
- **Диаграммы**: drawio формат, размещение в `docs/design/`
- **Скриншоты приложения**: создаются автоматически средствами Playwright MCP, размещение в `docs/guides/images/`
- **Встраивание изображений**: в Markdown — ссылки на PNG-экспорт из drawio и скриншоты: `![описание](images/filename.png)`

### 2.3 Admin Guide — структура

```
docs/guides/admin-guide.md / admin-guide.ru.md
```

**Содержание:**

1. **Обзор архитектуры**
   - Компоненты системы (AM, SE, IM, QM)
   - Взаимосвязи модулей — **C4 Context diagram** (drawio) + скриншот
   - Потоки данных — **Data Flow diagrams** (drawio) + скриншоты:
     - Upload flow (Client → IM → SE + AM)
     - Download flow (Client → QM → SE)
     - Search flow (Client → QM → PostgreSQL)
   - **Sequence diagrams** (drawio) для ключевых сценариев:
     - Загрузка файла (с выбором SE, регистрацией в AM)
     - Скачивание файла (с кэшированием в QM)
     - Аутентификация (JWT получение + валидация)
   - Варианты развёртывания SE:
     - В Kubernetes (как pod с PVC)
     - На обычных машинах как Docker-контейнер (включая удалённые ЦОД)

2. **Требования**
   - Kubernetes (минимальная версия, Gateway API / Ingress) — для AM, IM, QM
   - Docker (для standalone SE на обычных машинах)
   - PostgreSQL 17
   - Keycloak (версия, realm-конфигурация — подробности см. раздел 2.7)
   - Сетевые требования (связность между K8s-кластером и удалёнными SE)
   - Аппаратные ресурсы (таблица: dev vs production)

3. **Установка**
   - Quick Start (dev-окружение через docker-compose)
   - Production (Kubernetes через umbrella Helm chart)
   - Установка отдельных модулей (альтернативный путь)
   - Установка SE на удалённых машинах (Docker-контейнер вне K8s)
   - Конфигурация Gateway API (основной вариант)
   - Конфигурация Ingress nginx (альтернатива)

4. **Конфигурация**
   - Таблица env-переменных по каждому модулю (с defaults)
   - Настройка PostgreSQL (shared DB, миграции)
   - TLS-сертификаты (cert-manager интеграция)
   - Сетевая конфигурация для удалённых SE (firewall, TLS, доступ к AM)

5. **Управление Storage Elements**
   - Жизненный цикл SE: edit → rw → ro → ar
   - **C4 Component diagram** (drawio): внутренняя структура SE + скриншот
   - Регистрация SE в Admin Module (K8s pods и удалённые Docker-контейнеры)
   - Управление через Admin UI — **скриншоты** (Playwright MCP):
     - Список SE
     - Детали SE
     - Смена режима SE
   - Репликация (leader/follower)
   - Планирование ёмкости
   - Гео-распределённые SE (разные ЦОД):
     - Требования к сети (latency, bandwidth)
     - Конфигурация SE для работы через WAN
     - topologymetrics мониторинг удалённых SE

6. **Admin UI**
   - Аутентификация через Keycloak (Authorization Code + PKCE)
   - Обзор функций — **скриншоты каждой страницы** (Playwright MCP):
     - Dashboard (обзор системы)
     - Файлы (список, детали, поиск)
     - Storage Elements (список, управление)
     - Мониторинг (статусы зависимостей, графики)
     - Настройки (Prometheus URL, параметры)
     - Service Accounts (список, создание, scopes)
   - Управление Service Accounts

7. **Конфигурация Keycloak** (подробная)
   - Описание realm `artstore` (настройки безопасности, таймауты сессий)
   - Кастомная тема `artstore` (установка, сборка Docker-образа)
   - Подробности — см. раздел 2.7

8. **Обновление**
   - Порядок обновления модулей
   - Миграции БД (автоматические через golang-migrate)
   - Совместимость версий

### 2.4 Диаграммы — перечень и формат

Все диаграммы создаются в формате **drawio** и размещаются в `docs/design/`.
PNG-экспорт для встраивания в документацию — в `docs/guides/images/`.

#### C4 Diagrams

| Диаграмма | Файл | Используется в |
|-----------|------|----------------|
| **C4 Context** — Artstore и внешние системы (пользователи, Keycloak, PostgreSQL) | `docs/design/c4-context.drawio` | Admin Guide §1 |
| **C4 Container** — все модули, их связи, протоколы, БД | `docs/design/c4-container.drawio` | Admin Guide §1 |
| **C4 Component (SE)** — внутренняя структура Storage Element | `docs/design/c4-component-se.drawio` | Admin Guide §5 |
| **C4 Component (AM)** — внутренняя структура Admin Module | `docs/design/c4-component-am.drawio` | Admin Guide §7 |

#### Data Flow Diagrams

| Диаграмма | Файл | Используется в |
|-----------|------|----------------|
| **Upload Flow** — Client → IM → SE + AM (регистрация файла) | `docs/design/dataflow-upload.drawio` | Admin Guide §1, Developer Guide §3 |
| **Download Flow** — Client → QM → cache/SE (proxy download) | `docs/design/dataflow-download.drawio` | Admin Guide §1, Developer Guide §5 |
| **Search Flow** — Client → QM → PostgreSQL FTS | `docs/design/dataflow-search.drawio` | Admin Guide §1, Developer Guide §4 |

#### Sequence Diagrams

| Диаграмма | Файл | Используется в |
|-----------|------|----------------|
| **File Upload** — полная последовательность (auth → SE selection → upload → register) | `docs/design/seq-file-upload.drawio` | Developer Guide §3 |
| **File Download** — последовательность (auth → cache check → AM lookup → SE stream) | `docs/design/seq-file-download.drawio` | Developer Guide §5 |
| **JWT Authentication** — получение и валидация токена (user + SA) | `docs/design/seq-jwt-auth.drawio` | Developer Guide §2, Admin Guide §7 |
| **SE Registration** — регистрация SE в AM (K8s pod и Docker remote) | `docs/design/seq-se-registration.drawio` | Admin Guide §5 |

#### Deployment Diagrams

| Диаграмма | Файл | Используется в |
|-----------|------|----------------|
| **Deployment (K8s)** — все модули в одном кластере | `docs/design/deploy-k8s.drawio` | Admin Guide §3 |
| **Deployment (Hybrid)** — AM/IM/QM в K8s + SE в удалённых ЦОД (Docker) | `docs/design/deploy-hybrid.drawio` | Admin Guide §3, §5 |

> **Примечание**: существующие диаграммы в `docs/design/` (am-login-modules.drawio, im-*.drawio, qm-*.drawio) сохраняются как есть и могут быть включены в соответствующие разделы документации.

### 2.5 Скриншоты — создание через Playwright MCP

Скриншоты Admin UI создаются автоматически средствами **Playwright MCP** (browser automation).
Это обеспечивает воспроизводимость и актуальность скриншотов при обновлении UI.

**Размещение**: `docs/guides/images/`

**Перечень скриншотов:**

**Admin UI (Artstore):**

| Скриншот | Файл | Используется в |
|----------|------|----------------|
| Dashboard (обзор системы) | `images/ui-dashboard.png` | Admin Guide §6 |
| Список файлов | `images/ui-files-list.png` | Admin Guide §6 |
| Детали файла | `images/ui-file-details.png` | Admin Guide §6 |
| Список Storage Elements | `images/ui-se-list.png` | Admin Guide §5, §6 |
| Детали SE | `images/ui-se-details.png` | Admin Guide §5 |
| Смена режима SE | `images/ui-se-mode-change.png` | Admin Guide §5 |
| Мониторинг (статусы зависимостей) | `images/ui-monitoring.png` | Admin Guide §6, Ops Guide §1 |
| Настройки | `images/ui-settings.png` | Admin Guide §6 |
| Service Accounts | `images/ui-service-accounts.png` | Admin Guide §6 |

**Keycloak Admin Console:**

| Скриншот | Файл | Используется в |
|----------|------|----------------|
| Login page (кастомная тема) | `images/kc-login.png` | Admin Guide §7 |
| Realm Settings → General | `images/kc-realm-general.png` | Admin Guide §7 (2.8.1) |
| Realm Settings → Login | `images/kc-realm-login.png` | Admin Guide §7 (2.8.1) |
| Realm Settings → Sessions | `images/kc-realm-sessions.png` | Admin Guide §7 (2.8.1) |
| Realm Settings → Tokens | `images/kc-realm-tokens.png` | Admin Guide §7 (2.8.1) |
| Realm Roles (список) | `images/kc-realm-roles.png` | Admin Guide §7 (2.8.2) |
| Groups → artstore-admins (role mapping) | `images/kc-group-admins.png` | Admin Guide §7 (2.8.2) |
| Groups → artstore-viewers (role mapping) | `images/kc-group-viewers.png` | Admin Guide §7 (2.8.2) |
| Client Scopes (список) | `images/kc-client-scopes-list.png` | Admin Guide §7 (2.8.3) |
| Scope `files:read` → Settings | `images/kc-scope-files-read.png` | Admin Guide §7 (2.8.3) |
| Scope `files:read` → Mappers (audience) | `images/kc-scope-files-read-mappers.png` | Admin Guide §7 (2.8.3) |
| Scope `groups` → Mappers (group membership) | `images/kc-scope-groups-mapper.png` | Admin Guide §7 (2.8.3) |
| Client `artstore-admin-module` → Settings | `images/kc-client-am-settings.png` | Admin Guide §7 (2.8.4) |
| Client `artstore-admin-module` → Credentials | `images/kc-client-am-credentials.png` | Admin Guide §7 (2.8.4) |
| Client `artstore-admin-module` → Client Scopes | `images/kc-client-am-scopes.png` | Admin Guide §7 (2.8.4) |
| Client `artstore-admin-module` → SA Roles | `images/kc-client-am-sa-roles.png` | Admin Guide §7 (2.8.4) |
| Client `artstore-ingester` → Settings | `images/kc-client-im-settings.png` | Admin Guide §7 (2.8.4) |
| Client `artstore-ingester` → Client Scopes | `images/kc-client-im-scopes.png` | Admin Guide §7 (2.8.4) |
| Client `artstore-query` → Settings | `images/kc-client-qm-settings.png` | Admin Guide §7 (2.8.4) |
| Client `artstore-admin-ui` → Settings | `images/kc-client-ui-settings.png` | Admin Guide §7 (2.8.4) |
| Client `artstore-admin-ui` → Client Scopes | `images/kc-client-ui-scopes.png` | Admin Guide §7 (2.8.4) |
| Mapper `client_id` → детали (User Session Note) | `images/kc-mapper-client-id.png` | Admin Guide §7 (2.8.5) |
| Create Client wizard → Step 1 (General) | `images/kc-create-client-step1.png` | Admin Guide §7 (2.8.8) |
| Create Client wizard → Step 2 (Capability) | `images/kc-create-client-step2.png` | Admin Guide §7 (2.8.8) |
| Create Client wizard → Step 3 (Login settings) | `images/kc-create-client-step3.png` | Admin Guide §7 (2.8.8) |
| Add mapper → By configuration → User Session Note | `images/kc-add-mapper-step.png` | Admin Guide §7 (2.8.8) |

**Процесс создания скриншотов:**

1. Развернуть тестовое окружение (`make test-env-up && make init-data`)
2. Запустить port-forward (`make port-forward-start`)
3. Средствами Playwright MCP:
   - **Admin UI**: открыть `https://artstore.kryukov.lan/admin/`, авторизоваться (admin/admin), снять скриншоты страниц
   - **Keycloak Admin Console**: открыть `https://artstore.kryukov.lan/admin/master/console/`, авторизоваться (admin KC), перейти в realm `artstore`, снять скриншоты каждой настройки
4. Сохранить в `docs/guides/images/`

### 2.6 Developer Guide — структура

```
docs/guides/developer-guide.md / developer-guide.ru.md
```

**Содержание:**

1. **Обзор API**
   - Архитектура API (REST, JWT RS256)
   - Базовые URL и версионирование
   - Ссылки на OpenAPI-спецификации

2. **Аутентификация**
   - Получение JWT-токена (пользовательский — password grant, service account — client credentials)
   - Настройка Keycloak-клиента для интеграции
   - Scopes и роли (admin, readonly, files:read, files:write, storage:read)
   - Примеры curl

3. **Upload (Ingester Module)**
   - Endpoint: `POST /upload/files`
   - Параметры, форматы, лимиты
   - Примеры: curl, Python (requests), Go
   - Ошибки и коды ответов

4. **Search (Query Module)**
   - Endpoint: `GET /query/search`
   - Параметры поиска (PostgreSQL FTS)
   - Пагинация, фильтрация
   - Примеры

5. **Download (Query Module)**
   - Endpoint: `GET /query/files/{id}/download`
   - Прокси-скачивание через QM
   - Кэширование (LRU)
   - Примеры

6. **File Management (Admin Module)**
   - CRUD файлов через AM API
   - Получение метаданных

7. **Error Handling**
   - Унифицированный формат ошибок
   - Таблица кодов и сообщений

### 2.8 Keycloak — подробная конфигурация (Admin Guide §7)

В Admin Guide раздел «Конфигурация Keycloak» должен содержать **исчерпывающее** описание настройки Keycloak для Artstore. Это ключевой раздел, т.к. неправильная конфигурация KC — самая частая причина проблем при развёртывании.

Каждый подраздел сопровождается **скриншотами Keycloak Admin Console** (Playwright MCP), чтобы администратор мог визуально сверить свою конфигурацию с эталонной.

#### 2.8.1 Realm `artstore`

**Скриншоты:** Realm Settings → General, Login, Sessions, Tokens
**Основные настройки:**

| Параметр | Значение | Описание |
|----------|----------|----------|
| Realm name | `artstore` | |
| Login theme | `artstore` | Кастомная тема (тёмная, зелёный акцент) |
| SSL Required | `none` (dev) / `external` (prod) | Для production — обязательно `external` |
| Default Signature Algorithm | `RS256` | Все модули валидируют JWT RS256 |
| Registration | отключена | Пользователи создаются администратором |
| Brute Force Protection | включена | 5 попыток, блокировка до 900 сек |
| Access Token Lifespan | 300 сек (5 мин) | |
| SSO Session Idle | 1800 сек (30 мин) | |
| SSO Session Max | 36000 сек (10 ч) | |

#### 2.8.2 Роли и группы

**Скриншоты:** Realm Roles (список), Groups (список + role mappings для каждой группы)
**Realm-роли:**

| Роль | Назначение |
|------|------------|
| `admin` | Полный доступ (CRUD файлов, управление SE, SA, настройки) |
| `readonly` | Только чтение (просмотр файлов, SE, скачивание) |

**Группы:**

| Группа | Роль | Назначение |
|--------|------|------------|
| `artstore-admins` | `admin` | Администраторы системы |
| `artstore-viewers` | `readonly` | Пользователи с доступом на чтение |

> Роли назначаются через группы — это основной механизм. Модули конфигурируют маппинг через env: `*_ROLE_ADMIN_GROUPS`, `*_ROLE_READONLY_GROUPS`.

#### 2.8.3 Client Scopes (бизнес-scopes)

**Скриншоты:** Client Scopes (список), детали scope `files:read` (Settings + Mappers), scope `groups` (Mappers → group membership mapper)

Каждый scope добавляет `oidc-audience-mapper` в access token:

| Scope | Описание | Используется в |
|-------|----------|----------------|
| `files:read` | Чтение метаданных и скачивание файлов | IM, QM, AM |
| `files:write` | Загрузка, обновление и удаление файлов | IM, AM |
| `storage:read` | Чтение информации о SE | IM, QM, AM |
| `storage:write` | Управление SE (sync, mode transition) | AM |
| `admin:read` | Чтение административных данных | AM |
| `admin:write` | Управление пользователями и SA | AM |

**Специальный scope `groups`:**
- Mapper: `oidc-group-membership-mapper`
- Claim: `groups` (короткие имена, без `/` prefix)
- Обязателен для корректной авторизации через группы

#### 2.8.4 Clients — полное описание

**Скриншоты для каждого клиента:** Settings, Credentials, Client Scopes (Assigned), Service Account Roles (для SA-клиентов)

##### `artstore-admin-module` — Service Account Admin Module

| Параметр | Значение |
|----------|----------|
| Тип | Confidential (client-secret) |
| Grant type | Client Credentials |
| Scopes | `files:read`, `files:write`, `storage:read`, `storage:write`, `admin:read`, `admin:write`, `groups` |
| Realm-management roles | `view-users`, `manage-clients`, `view-clients`, `manage-users`, `view-realm`, `query-users`, `query-clients` |

> **Критически важный mapper `client_id`**: тип `oidc-usersessionmodel-note-mapper`, claim `client_id` в access token. AM использует этот claim для распознавания SA-токенов и маппинга на service account записи в БД.

##### `artstore-ingester` — Service Account Ingester Module

| Параметр | Значение |
|----------|----------|
| Тип | Confidential (client-secret) |
| Grant type | Client Credentials |
| Scopes | `files:read`, `files:write`, `storage:read` |

> Также требует mapper `client_id` (идентичный AM).

##### `artstore-query` — Service Account Query Module

| Параметр | Значение |
|----------|----------|
| Тип | Confidential (client-secret) |
| Grant type | Client Credentials |
| Scopes | `files:read`, `storage:read` |

> Также требует mapper `client_id` (идентичный AM).

##### `artstore-admin-ui` — Admin UI (браузерный клиент)

| Параметр | Значение |
|----------|----------|
| Тип | Public (PKCE) |
| Grant type | Authorization Code + PKCE (S256) |
| Redirect URIs | `https://<domain>/*` |
| Default scopes | `openid`, `profile`, `email`, `groups` |
| Optional scopes | Все бизнес-scopes |

> Описать настройку Redirect URIs и Web Origins для production-домена.

#### 2.8.5 Protocol Mapper `client_id` — критический элемент

**Скриншоты:** Client → Mappers → создание mapper-а (пошагово: Add mapper → By configuration → User Session Note → заполнение полей)

Подробно описать создание mapper-а `client_id` (тип `oidc-usersessionmodel-note-mapper`):

- Зачем нужен: AM распознаёт SA-токены по claim `client_id` и маппит на записи в таблице `service_accounts`
- Где создавать: в каждом SA-клиенте (`artstore-admin-module`, `artstore-ingester`, `artstore-query`)
- Настройки: `user.session.note: client_id`, `claim.name: client_id`, Token Claim Name: `client_id`
- Включить в: Access Token, ID Token, Introspection

> **Без этого mapper-а межсервисная авторизация не работает.** Это самая частая ошибка при настройке.

#### 2.8.6 URL-схема в Kubernetes

Описать паттерн двух URL для Keycloak:

| URL | Назначение | Пример |
|-----|------------|--------|
| Внутренний (HTTP) | Обращения модулей к KC (JWKS, token endpoint) | `http://<release>-keycloak.<namespace>.svc.cluster.local:8080` |
| Внешний (HTTPS) | JWT Issuer, браузерная авторизация Admin UI | `https://artstore.kryukov.lan` |

> Объяснить почему Issuer = внешний URL (JWT проверяется по iss claim, который должен совпадать с URL, видимым клиенту), а JWKS/Token URLs = внутренние (для производительности и отсутствия TLS overhead).

#### 2.8.7 Кастомная тема `artstore`

- Сборка Docker-образа: `deploy/keycloak/Dockerfile`
- Тема: CSS-only (parent: `keycloak.v2`), тёмная, зелёный акцент (#22c55e)
- Локализация: EN + RU
- Скриншот страницы логина — `images/kc-login.png` (Playwright MCP)

#### 2.8.8 Создание дополнительных клиентов (для интеграции)

**Скриншоты:** пошаговая серия скриншотов — от создания клиента до проверки токена. Каждый шаг сопровождается скриншотом соответствующей страницы KC.

Пошаговая инструкция для администратора:

1. Создание нового Keycloak-клиента (confidential, client credentials) — **скриншот**: Clients → Create Client (wizard steps)
2. Назначение нужных scopes — **скриншот**: Client → Client Scopes → Add client scope
3. Добавление mapper `client_id` — **скриншот**: Client → Client Scopes → Dedicated scope → Mappers → Add mapper
4. Регистрация Service Account в Admin Module (через API или UI) — **скриншот**: Admin UI → Service Accounts → Create
5. Проверка — получение токена и вызов API (пример curl + разбор JWT на jwt.io)

### 2.9 Operations Guide — структура

```
docs/guides/operations-guide.md / operations-guide.ru.md
```

**Содержание:**

1. **Мониторинг**
   - Обзор метрик (какие модули, какие метрики экспортируют)
   - Настройка сбора метрик (ServiceMonitor CRDs / Pod annotations)
   - Встроенный мониторинг в Admin UI
   - topologymetrics (граф зависимостей, uniproxy)

2. **Grafana Dashboards**
   - Установка и provisioning дашбордов
   - Описание каждого дашборда (см. раздел 4)

3. **Алертинг**
   - AlertManager rules
   - Описание каждого алерта (что, почему, как реагировать)
   - Runbooks для критических алертов

4. **Troubleshooting**
   - Типичные проблемы и решения
   - Диагностика зависимостей (через topologymetrics)
   - Логи (slog JSON формат, как искать)
   - Health check endpoints (`/health`, `/ready`)

5. **Backup и восстановление** (рекомендации)
   - PostgreSQL: pg_dump / pg_restore
   - PVC: VolumeSnapshot (CSI driver)
   - Keycloak realm export/import
   - *Автоматизация — будущая задача*

6. **Масштабирование**
   - Горизонтальное масштабирование модулей (IM, QM — stateless)
   - Добавление новых Storage Elements
   - Рекомендации по ресурсам

---

## 3. Развёртывание

### 3.1 Umbrella Helm Chart

Создать единый chart `artstore` со следующей структурой:

```
charts/artstore/
├── Chart.yaml           # type: application, dependencies: AM, SE, IM, QM
├── values.yaml          # Defaults
├── values-dev.yaml      # Dev/test профиль (минимум ресурсов)
├── values-production.yaml  # Production профиль
├── templates/
│   ├── _helpers.tpl
│   ├── namespace.yaml   # Опциональное создание namespace
│   └── tests/           # Helm test hooks
└── charts/              # Subcharts (зависимости)
    ├── admin-module/
    ├── storage-element/
    ├── ingester-module/
    ├── query-module/
    └── monitoring/      # Опциональный subchart (см. 3.3)
```

### 3.2 Профили развёртывания

#### Dev/Test (docker-compose + minimal K8s)

- **docker-compose**: AM + IM + QM + SE(1) + PostgreSQL + Keycloak
  - Один файл `docker-compose.yaml` в корне проекта
  - Minimal ресурсы, без TLS, без мониторинга
  - Для быстрого старта и локальной разработки

- **Minimal K8s**: umbrella chart с `values-dev.yaml`
  - 1 SE (mode: edit)
  - Minimal resource requests/limits
  - Без репликации, без HA
  - Опциональный встроенный мониторинг

#### Production (Kubernetes + удалённые SE)

- Umbrella chart с `values-production.yaml`
- Множество SE (edit, rw, ro — по потребности)
- Рекомендуемые resource requests/limits
- TLS через cert-manager
- Gateway API (основной) / Ingress (альтернатива)
- Интеграция с существующим мониторинг-стеком
- Рекомендации по HA (PG replicas, multiple IM/QM pods)

#### Гибридное развёртывание (SE вне Kubernetes)

Storage Elements могут работать **как Docker-контейнеры на обычных машинах**, в том числе в других ЦОД:

- **Сценарий**: AM, IM, QM в Kubernetes-кластере; SE — на bare-metal или VM с Docker
- **Требования к сети**: SE должны быть доступны по HTTP/HTTPS из K8s-кластера (для IM upload, QM download, AM health check)
- **Регистрация**: SE регистрируются в AM через API с внешним URL (не K8s DNS)
- **Мониторинг**: topologymetrics SDK в SE отправляет метрики → нужен Prometheus с доступом к удалённым SE (`/metrics` endpoint)
- **TLS**: рекомендуется для WAN-соединений (SE поддерживает TLS через env-переменные)
- **Документировать**:
  - `docker run` команда для запуска SE с полным набором env-переменных
  - docker-compose пример для SE на удалённой машине
  - Сетевая схема (firewall rules, необходимые порты)
  - Конфигурация dephealth для мониторинга удалённых SE
  - **Deployment diagram (Hybrid)** — `docs/design/deploy-hybrid.drawio`

### 3.3 Мониторинг как опциональный subchart

Для тестовых и небольших инсталляций — опциональный subchart `monitoring`:

```yaml
# values.yaml
monitoring:
  enabled: false  # По умолчанию отключен

  prometheus:
    enabled: true
    # Минимальный Prometheus для scraping artstore-метрик

  grafana:
    enabled: true
    # Grafana с pre-provisioned dashboards
```

Для production — интеграция с существующим стеком через ServiceMonitor CRDs.

### 3.4 Pod Annotations для Prometheus scraping

Все pods модулей должны иметь аннотации для автоматического обнаружения Prometheus:

```yaml
annotations:
  prometheus.io/scrape: "true"
  prometheus.io/port: "<port>"
  prometheus.io/path: "/metrics"
```

Это позволяет Prometheus с конфигурацией `kubernetes_sd_configs` автоматически находить и scrape-ить модули без ServiceMonitor CRDs.

**Helm values для управления:**

```yaml
podAnnotations:
  prometheus.io/scrape: "true"
  prometheus.io/port: "8000"
  prometheus.io/path: "/metrics"
```

---

## 4. Мониторинг — Grafana Dashboards

### 4.1 Перечень дашбордов

| # | Dashboard | Описание |
|---|-----------|----------|
| 1 | **Artstore Overview** | Общий обзор системы: RPS, latency p50/p95/p99, error rate, статусы SE, зависимости |
| 2 | **Admin Module** | AM-специфичные метрики: HTTP requests, зарегистрированные файлы/SE, SSE connections |
| 3 | **Storage Element** | SE-специфичные: files_total, storage_bytes, operations, статус по mode (edit/rw/ro/ar) |
| 4 | **Ingester Module** | IM-специфичные: upload RPS, размеры файлов, SE selection, ошибки |
| 5 | **Query Module** | QM-специфичные: search RPS, cache hit ratio, download throughput, latency |
| 6 | **Dependency Topology** | topologymetrics: граф зависимостей, health matrix, inter-service latency |

### 4.2 Метрики по модулям (для дашбордов)

**Общие HTTP-метрики** (все модули):

- `<prefix>_http_requests_total{method, path, status}` — counter
- `<prefix>_http_request_duration_seconds{method, path}` — histogram

Префиксы: `am_`, `se_`, `im_`, `qm_`

**Storage Element — бизнес-метрики:**

- `se_files_total{status}` — количество файлов (active/deleted)
- `se_storage_bytes` — использованное место
- `se_operations_total{operation, result}` — операции (upload/delete/replicate)

**topologymetrics — метрики зависимостей:**

- `app_dependency_health{service, target, group}` — 1=ok, 0=fail
- `app_dependency_latency_seconds{service, target, group}` — histogram
- `app_dependency_status{service, target, group}` — категория
- `app_dependency_status_detail{service, target, group}` — детали

### 4.3 Dashboard: Artstore Overview

**Rows:**

1. **System Health** — traffic lights: AM, IM, QM, SE-ы (на базе `app_dependency_health`)
2. **Request Rate** — панели RPS по модулям (stacked graph)
3. **Latency** — p50/p95/p99 по модулям (histogram_quantile)
4. **Error Rate** — % 4xx/5xx по модулям
5. **Storage** — total files, total storage used, SE capacity (gauge/bar)

**Variables:**

- `$namespace` — Kubernetes namespace
- `$interval` — scrape interval

### 4.4 Dashboard: Dependency Topology

**Rows:**

1. **Health Matrix** — status map: service → dependency (красный/зелёный)
2. **Dependency Latency** — heatmap: latency между сервисами
3. **Critical Dependencies** — таблица critical зависимостей с текущим статусом
4. **Topology Graph** — node graph panel (Grafana 10+): визуализация графа зависимостей

**Источник**: метрики `app_dependency_*` из topologymetrics SDK

### 4.5 Формат и provisioning

- Дашборды в JSON формате: `charts/artstore/dashboards/*.json`
- Provisioning через ConfigMap (Grafana sidecar / provisioning API)
- Каждый dashboard — отдельный JSON файл

### 4.6 AlertManager Rules

Примеры критических правил:

| Alert | Условие | Severity |
|-------|---------|----------|
| `ArtstoreServiceDown` | `app_dependency_health == 0` > 1m | critical |
| `ArtstoreHighErrorRate` | `rate(http_requests_total{status=~"5.."}[5m]) / rate(http_requests_total[5m]) > 0.05` | warning |
| `ArtstoreHighLatency` | `histogram_quantile(0.95, rate(http_request_duration_seconds_bucket[5m])) > 2` | warning |
| `SEStorageFull` | `se_storage_bytes / se_storage_capacity_bytes > 0.9` | critical |
| `SENoEditAvailable` | Нет SE в mode=edit | critical |
| `DependencyLatencyHigh` | `app_dependency_latency_seconds p95 > 5s` | warning |

---

## 5. Структура файлов (итоговая)

```
docs/
├── guides/
│   ├── admin-guide.md              # EN
│   ├── admin-guide.ru.md           # RU
│   ├── developer-guide.md          # EN
│   ├── developer-guide.ru.md       # RU
│   ├── operations-guide.md         # EN
│   ├── operations-guide.ru.md      # RU
│   └── images/                     # Скриншоты и PNG-экспорт диаграмм
│       ├── c4-context.png
│       ├── c4-container.png
│       ├── c4-component-se.png
│       ├── c4-component-am.png
│       ├── dataflow-upload.png
│       ├── dataflow-download.png
│       ├── dataflow-search.png
│       ├── seq-file-upload.png
│       ├── seq-file-download.png
│       ├── seq-jwt-auth.png
│       ├── seq-se-registration.png
│       ├── deploy-k8s.png
│       ├── deploy-hybrid.png
│       ├── ui-dashboard.png        # Скриншоты Admin UI (Playwright MCP)
│       ├── ui-files-list.png
│       ├── ui-file-details.png
│       ├── ui-se-list.png
│       ├── ui-se-details.png
│       ├── ui-se-mode-change.png
│       ├── ui-monitoring.png
│       ├── ui-settings.png
│       ├── ui-service-accounts.png
│       ├── kc-login.png            # Keycloak (Playwright MCP)
│       ├── kc-realm-general.png
│       ├── kc-realm-login.png
│       ├── kc-realm-sessions.png
│       ├── kc-realm-tokens.png
│       ├── kc-realm-roles.png
│       ├── kc-group-admins.png
│       ├── kc-group-viewers.png
│       ├── kc-client-scopes-list.png
│       ├── kc-scope-files-read.png
│       ├── kc-scope-groups-mapper.png
│       ├── kc-client-am-settings.png
│       ├── kc-client-am-scopes.png
│       ├── kc-client-am-sa-roles.png
│       ├── kc-client-im-settings.png
│       ├── kc-client-qm-settings.png
│       ├── kc-client-ui-settings.png
│       ├── kc-mapper-client-id.png
│       └── kc-create-client-*.png  # Серия wizard-скриншотов
├── design/                         # Диаграммы drawio
│   ├── c4-context.drawio           # НОВЫЕ — C4
│   ├── c4-container.drawio
│   ├── c4-component-se.drawio
│   ├── c4-component-am.drawio
│   ├── dataflow-upload.drawio      # НОВЫЕ — Data Flow
│   ├── dataflow-download.drawio
│   ├── dataflow-search.drawio
│   ├── seq-file-upload.drawio      # НОВЫЕ — Sequence
│   ├── seq-file-download.drawio
│   ├── seq-jwt-auth.drawio
│   ├── seq-se-registration.drawio
│   ├── deploy-k8s.drawio           # НОВЫЕ — Deployment
│   ├── deploy-hybrid.drawio
│   ├── am-login-modules.drawio     # Существующие
│   ├── im-file-upload-sequence.drawio
│   ├── im-se-selection-sequence.drawio
│   ├── im-upload-modules.drawio
│   ├── qm-get-file-by-id-modules.drawio
│   └── qm-get-file-by-id-sequence.drawio
├── api-contracts/                  # Существующие OpenAPI specs
├── briefs/                         # Существующие брифы
└── requirements/                   # Существующие требования

charts/
└── artstore/                       # Umbrella Helm chart (новый)
    ├── Chart.yaml
    ├── values.yaml
    ├── values-dev.yaml
    ├── values-production.yaml
    ├── dashboards/                 # Grafana dashboard JSONs
    │   ├── overview.json
    │   ├── admin-module.json
    │   ├── storage-element.json
    │   ├── ingester-module.json
    │   ├── query-module.json
    │   └── dependency-topology.json
    ├── alerts/                     # AlertManager rules
    │   └── artstore-alerts.yaml
    └── templates/

docker-compose.yaml                 # Dev/test Quick Start (новый, корень проекта)
```

---

## 6. Приоритеты реализации

| Приоритет | Задача | Обоснование |
|-----------|--------|-------------|
| P0 | Pod annotations для Prometheus | Быстрый win — 1 строка в каждом chart |
| P0 | C4 + Data Flow + Sequence диаграммы (drawio) | Основа для всей документации, визуальная архитектура |
| P0 | Admin Guide (установка + настройка + Keycloak) | Основа для всей остальной документации |
| P1 | Скриншоты Admin UI (Playwright MCP) | Необходимы для Admin Guide |
| P1 | Umbrella Helm chart (базовый) | Упрощает установку для пользователей |
| P1 | Developer Guide | Необходим для интеграторов |
| P1 | Grafana dashboards (Overview + per-module) | Базовый мониторинг |
| P2 | Operations Guide | Мониторинг + troubleshooting |
| P2 | Dependency Topology dashboard | Продвинутый мониторинг |
| P2 | AlertManager rules | Алертинг |
| P2 | Deployment diagrams (K8s + Hybrid) | Документация гибридного развёртывания SE |
| P3 | docker-compose для dev | Quick Start |
| P3 | Monitoring subchart | Автономный мониторинг для тестовых сред |

---

## 7. Открытые вопросы

1. **SE capacity metric**: в SE есть `se_storage_bytes`, но нет `se_storage_capacity_bytes`. Нужно ли добавить метрику максимальной ёмкости SE?
2. **QM cache metrics**: в QM используется LRU cache, но метрики cache hit/miss не экспортируются. Нужно ли добавить?
3. **IM upload metrics**: в IM нет бизнес-метрик (размер загруженных файлов, количество). Нужно ли добавить?
4. **Grafana version**: для Node Graph panel (topology) требуется Grafana 10+. Это допустимо?
5. **Docker-compose**: нужен ли он именно в корне проекта, или лучше в `deploy/docker-compose/`?
6. **Keycloak в docker-compose**: включать готовый realm export или требовать ручную настройку?
7. **Удалённые SE — service discovery**: как Prometheus обнаруживает SE вне K8s? Варианты: static targets в Prometheus config, federation, или Prometheus remote write из SE.
8. **Удалённые SE — Keycloak доступ**: SE валидирует JWT через JWKS endpoint. Нужен ли прямой доступ SE к Keycloak, или достаточно JWKS через AM?
9. **drawio export**: автоматизировать PNG-экспорт из drawio (CLI: `drawio --export`) или делать вручную?

---

*Документ создан: 2026-03-03*
*Следующий шаг: согласование требований → `/sc:design` для архитектуры → `/sc:workflow` для плана реализации*
