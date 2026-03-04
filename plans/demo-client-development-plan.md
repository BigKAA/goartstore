# План разработки: Demo Client

## Метаданные

- **Версия плана**: 1.1.0
- **Дата создания**: 2026-03-04
- **Последнее обновление**: 2026-03-04
- **Статус**: Pending
- **Требования**: `docs/briefs/demo-client-requirements.md`

---

## История версий

- **v1.0.0** (2026-03-04): Начальная версия плана
- **v1.1.0** (2026-03-04): Улучшения: Makefile → Phase 1, CSP/CSRF middleware, embed.go,
  дополнение Config (CA_CERT, SCOPES, MAX_UPLOAD_SIZE, Version), gateway/errors.go,
  явные зависимости Phase 4.3 → 2.2, подпункты unit-тестов Phase 2

---

## Текущий статус

- **Активная фаза**: Phase 1
- **Активный подпункт**: 1.1
- **Последнее обновление**: 2026-03-04
- **Примечание**: План создан, ожидает утверждения

---

## Архитектура

### Общая схема

```
┌──────────────────────────────────────────────────────┐
│                    Browser (User)                     │
│  Templ HTML + HTMX + Alpine.js + Tailwind CSS        │
└───────────────────────┬──────────────────────────────┘
                        │ HTTP (HTMX partials, forms, downloads)
                        ▼
┌──────────────────────────────────────────────────────┐
│              Demo Client Backend (Go)                 │
│                                                       │
│  ┌─────────┐  ┌──────────┐  ┌───────────────────┐   │
│  │ chi     │  │ UI       │  │ Gateway Client    │   │
│  │ router  │  │ handlers │  │ (HTTP → artstore) │   │
│  └────┬────┘  └────┬─────┘  └────────┬──────────┘   │
│       │            │                  │               │
│  ┌────┴────────────┴──────────────────┴───────────┐  │
│  │              Service Layer                      │  │
│  │  ┌──────────┐ ┌──────────┐ ┌────────────────┐  │  │
│  │  │ Upload   │ │ Search   │ │ Activity Log   │  │  │
│  │  │ Service  │ │ Service  │ │ (in-memory)    │  │  │
│  │  └──────────┘ └──────────┘ └────────────────┘  │  │
│  └────────────────────┬───────────────────────────┘  │
│                       │                               │
│  ┌────────────────────┴───────────────────────────┐  │
│  │         Token Manager (SA auto-refresh)         │  │
│  └────────────────────┬───────────────────────────┘  │
└───────────────────────┼──────────────────────────────┘
                        │ HTTPS + Bearer JWT
                        ▼
              ┌──────────────────┐
              │   API Gateway    │  artstore.kryukov.lan
              │   (Envoy GW)    │
              └───┬──────────┬──┘
                  │          │
          ┌───────┘          └────────┐
          ▼                           ▼
   ┌─────────────┐           ┌──────────────┐
   │  Ingester   │           │    Query     │
   │  /upload/*  │           │  /query/*    │
   └─────────────┘           └──────────────┘
```

### Структура проекта

```
src/demo-client/
├── cmd/demo-client/main.go           — Точка входа
├── internal/
│   ├── config/config.go              — Конфигурация (DC_ env vars)
│   ├── server/server.go              — HTTP-сервер, chi, graceful shutdown
│   ├── gateway/                      — HTTP-клиент к API Gateway
│   │   ├── client.go                 — Базовый HTTP-клиент + auth header
│   │   ├── errors.go                — Типизированные ошибки (ErrFileArchived, ErrNotFound, ...)
│   │   ├── upload.go                 — POST /upload/api/v1/files/upload
│   │   ├── search.go                 — POST /query/api/v1/search
│   │   ├── download.go              — GET /query/api/v1/files/{id}/download
│   │   ├── metadata.go              — GET /query/api/v1/files/{id}
│   │   ├── health.go                — Health checks через Gateway
│   │   └── models.go                — Типы запросов/ответов Gateway API
│   ├── token/                        — SA Token Manager
│   │   └── manager.go               — Client Credentials flow, auto-refresh
│   ├── activity/                     — Activity Log (in-memory ring buffer)
│   │   └── log.go                   — Запись, чтение, SSE stream
│   ├── service/                      — Бизнес-логика
│   │   ├── upload.go                — Загрузка (single + batch)
│   │   ├── search.go                — Поиск + фильтрация
│   │   ├── download.go             — Скачивание + обработка ошибок
│   │   └── dashboard.go            — Статистика, health-checks
│   └── ui/                           — Веб-интерфейс
│       ├── handlers/                 — HTTP-обработчики страниц и partials
│       │   ├── dashboard.go         — GET / (dashboard)
│       │   ├── upload.go            — GET /upload, POST /upload
│       │   ├── search.go            — GET /search, GET /search/results (partial)
│       │   ├── download.go          — GET /download/{id}
│       │   ├── settings.go          — GET /settings
│       │   ├── language.go          — POST /set-language
│       │   └── activity.go          — GET /activity/stream (SSE)
│       ├── layouts/
│       │   ├── base.templ           — HTML5 shell (head, scripts, fonts)
│       │   ├── page.templ           — Sidebar + main content wrapper
│       │   └── sidebar.templ        — Навигация
│       ├── pages/
│       │   ├── dashboard.templ      — Dashboard: health, stats, activity log
│       │   ├── upload.templ         — Форма загрузки (single + batch)
│       │   ├── search.templ         — Поиск: строка + фильтры + результаты
│       │   ├── file_detail.templ    — Карточка метаданных файла
│       │   ├── settings.templ       — Параметры подключения, health
│       │   └── partials/
│       │       ├── search_results.templ  — Таблица результатов (HTMX partial)
│       │       ├── upload_result.templ   — Результат загрузки
│       │       ├── batch_progress.templ  — Прогресс batch upload
│       │       ├── activity_row.templ    — Строка лога операций
│       │       ├── health_status.templ   — Статус health-check
│       │       ├── token_status.templ    — Статус SA токена
│       │       └── archived_modal.templ  — Модалка FILE_ARCHIVED
│       ├── components/
│       │   ├── button.templ         — Кнопки (primary, secondary, danger)
│       │   ├── badge.templ          — Бейджи (status, mode, retention)
│       │   ├── toast.templ          — Toast-уведомления
│       │   ├── modal.templ          — Модальное окно
│       │   ├── data_table.templ     — Таблица с сортировкой
│       │   ├── pagination.templ     — Пагинация
│       │   ├── stat_card.templ      — Карточка статистики
│       │   ├── filter_panel.templ   — Панель фильтров
│       │   ├── file_icon.templ      — Иконка по типу файла
│       │   ├── copy_button.templ    — Кнопка копирования в буфер
│       │   └── tag_input.templ      — Ввод тегов (чипы)
│       ├── i18n/
│       │   ├── i18n.go             — Bundle, T(), Tf(), middleware
│       │   ├── middleware.go        — Language detection (cookie → header → default)
│       │   └── locales/
│       │       ├── ru.json          — Русская локализация
│       │       └── en.json          — Английская локализация
│       └── static/
│           ├── embed.go             — //go:embed для встраивания static в бинарник
│           ├── css/
│           │   ├── input.css        — Tailwind directives + custom layers
│           │   └── output.css       — Сгенерированный CSS (в .gitignore)
│           ├── js/
│           │   ├── htmx.min.js      — HTMX 2.x
│           │   ├── htmx-sse.js      — HTMX SSE extension
│           │   └── alpine.min.js    — Alpine.js 3.x
│           └── favicon.ico
├── charts/demo-client/               — Helm chart
│   ├── Chart.yaml
│   ├── values.yaml
│   └── templates/
│       ├── deployment.yaml
│       ├── service.yaml
│       ├── httproute.yaml
│       ├── configmap.yaml
│       ├── secret.yaml
│       └── _helpers.tpl
├── Dockerfile                        — Multi-stage (templ → tailwind → go → alpine)
├── docker-compose.yaml               — Локальная разработка
├── Makefile                          — Build targets
├── tailwind.config.js                — Tailwind тема (как AM, но с акцентом)
├── go.mod
└── go.sum
```

### Ключевые компоненты

#### Token Manager

```
Goroutine (background):
  loop:
    if token expires in < DC_TOKEN_REFRESH_BEFORE:
      POST token_url (client_credentials) → new JWT
      store in-memory (atomic)
    sleep(check_interval)

GetToken() string — атомарное чтение текущего токена
TokenInfo() — expiry, valid, client_id (для UI)
```

#### Gateway Client

```
Обёртка над net/http.Client:
  - Автоматическое добавление Authorization: Bearer <token>
  - Logging каждого вызова в Activity Log
  - Метрики (duration, status code)
  - Configurable timeouts (request vs upload)
```

#### Activity Log

```
Ring buffer (фиксированный размер DC_ACTIVITY_LOG_SIZE):
  - Append(entry) — добавление записи
  - List() — все записи (newest first)
  - Subscribe() — SSE channel для HTMX live-updates
```

#### Обработка FILE_ARCHIVED (410)

```
Download flow:
  1. GET /query/api/v1/files/{id}/download через Gateway
  2. Если 200 → stream файл клиенту
  3. Если 410 → вернуть JSON с file_id + filename + tags
  4. UI handler: при 410 → рендерить archived_modal.templ
  5. Modal содержит:
     - Сообщение (i18n)
     - file_id с кнопкой copy (Alpine.js + Clipboard API)
     - Метаданные файла
     - Инструкция "обратитесь к администратору"
```

---

## Оглавление

- [ ] [Phase 1: Каркас проекта + конфигурация + Token Manager](#phase-1-каркас-проекта--конфигурация--token-manager)
- [ ] [Phase 2: Gateway Client + Service Layer](#phase-2-gateway-client--service-layer)
- [ ] [Phase 3: UI Framework (layouts, components, i18n)](#phase-3-ui-framework-layouts-components-i18n)
- [ ] [Phase 4: Dashboard + Activity Log + Settings](#phase-4-dashboard--activity-log--settings)
- [ ] [Phase 5: Upload (single + batch)](#phase-5-upload-single--batch)
- [ ] [Phase 6: Search + Download + AR handling](#phase-6-search--download--ar-handling)
- [ ] [Phase 7: Docker + Helm + Keycloak client + Integration](#phase-7-docker--helm--keycloak-client--integration)

---

## Phase 1: Каркас проекта + конфигурация + Token Manager

**Dependencies**: None
**Status**: Pending

### Описание

Создание базовой структуры Go-проекта, конфигурации из env-переменных,
Token Manager для автоматического получения и обновления SA JWT токена
через Client Credentials flow. HTTP-сервер с chi router, health endpoints,
Prometheus метрики. Паттерны полностью повторяют Admin Module.

### Подпункты

- [ ] **1.1 Инициализация Go-проекта + Makefile**
  - **Dependencies**: None
  - **Description**: `go mod init`, структура директорий, cmd/demo-client/main.go (заглушка).
    Module path: `github.com/bigkaa/goartstore/demo-client`.
    Makefile с targets: build, test, templ-generate, css-build, css-watch,
    ui-build, docker-build, docker-run, docker-stop, lint, clean, i18n-check.
    Паттерн как в AM. Makefile нужен с самого начала для сборки templ/tailwind/go.
  - **Creates**:
    - `src/demo-client/go.mod`
    - `src/demo-client/cmd/demo-client/main.go`
    - `src/demo-client/Makefile`
    - Директории: `internal/{config,server,gateway,token,activity,service,ui}`
  - **Links**:
    - `src/admin-module/Makefile` — референс

- [ ] **1.2 Конфигурация (config.go)**
  - **Dependencies**: 1.1
  - **Description**: Загрузка DC_ env-переменных. Паттерн как в AM:
    `getEnvRequired`, `getEnvDefault`, `getEnvDuration`, `getEnvBool`, `getEnvInt`.
    Валидация обязательных параметров. Config struct со всеми полями из NFR-01.
    Дополнительные поля не из NFR-01, но необходимые:
    - `DC_CA_CERT_PATH` — путь к CA-сертификату для TLS к Gateway/Keycloak в K8s
    - `DC_SCOPES` — OAuth scopes (default: `files:read files:write`)
    - `DC_MAX_UPLOAD_SIZE` — макс. размер файла (default: `1073741824`, 1GB)
    Переменная `Version` (string) для инъекции через ldflags при сборке.
  - **Creates**:
    - `internal/config/config.go`
  - **Links**:
    - `src/admin-module/internal/config/config.go` — референс

- [ ] **1.3 Token Manager**
  - **Dependencies**: 1.2
  - **Description**: Client Credentials flow → Keycloak token endpoint.
    Background goroutine для auto-refresh. Atomic read/write токена.
    `GetToken() string`, `TokenInfo() TokenStatus`.
    Exponential backoff при ошибках (max 5 min).
    Метрики: `dc_token_refreshes_total{status}`.
  - **Creates**:
    - `internal/token/manager.go`

- [ ] **1.4 HTTP-сервер + health + metrics + security middleware**
  - **Dependencies**: 1.2, 1.3
  - **Description**: Chi router, graceful shutdown (SIGINT/SIGTERM).
    Health endpoints: `/health/live` (always OK), `/health/ready` (token valid + Gateway reachable).
    Prometheus metrics endpoint `/metrics`.
    Request logging middleware (slog).
    Security middleware (NFR-02):
    - CSP заголовки (Content-Security-Policy) для защиты от XSS
    - CSRF-защита для POST-форм (gorilla/csrf или custom token)
    Middleware stack: security headers → metrics → request logging.
  - **Creates**:
    - `internal/server/server.go`
  - **Links**:
    - `src/admin-module/internal/server/server.go` — референс

- [ ] **1.5 main.go — сборка компонентов**
  - **Dependencies**: 1.2, 1.3, 1.4
  - **Description**: Инициализация: config.Load() → Token Manager start →
    HTTP server start. Graceful shutdown в обратном порядке.
    slog setup (JSON/text по конфигурации). Версия через ldflags.
  - **Creates**:
    - `cmd/demo-client/main.go` (финальная версия)

### Критерии завершения Phase 1

- [ ] Все подпункты завершены (1.1–1.5)
- [ ] `go build ./cmd/demo-client/` компилируется без ошибок
- [ ] `go test ./...` проходит
- [ ] Token Manager получает JWT от Keycloak (ручной тест)
- [ ] `/health/live` возвращает 200
- [ ] `/metrics` возвращает Prometheus метрики

---

## Phase 2: Gateway Client + Service Layer

**Dependencies**: Phase 1
**Status**: Pending

### Описание

HTTP-клиент для взаимодействия с artstore API через Gateway.
Сервисный слой для бизнес-логики (upload, search, download, dashboard).
Activity Log (in-memory ring buffer) для записи всех API-вызовов.

### Подпункты

- [ ] **2.1 Gateway базовый клиент + типы ошибок**
  - **Dependencies**: None
  - **Description**: HTTP-клиент с авто-инъекцией `Authorization: Bearer <token>`.
    Configurable timeouts (request vs upload). TLS настройки из конфига
    (CA cert loading из `DC_CA_CERT_PATH`).
    Middleware для логирования запросов в Activity Log.
    Метрики: `dc_gateway_requests_total`, `dc_gateway_request_duration_seconds`.
    Типизированные ошибки: `ErrFileArchived`, `ErrNotFound`, `ErrStorageFull`,
    `ErrFileTooLarge`, `ErrServiceUnavailable` — для маппинга HTTP-кодов
    в user-friendly сообщения на уровне UI.
  - **Creates**:
    - `internal/gateway/client.go`
    - `internal/gateway/models.go`
    - `internal/gateway/errors.go`

- [ ] **2.2 Activity Log**
  - **Dependencies**: None
  - **Description**: Thread-safe ring buffer фиксированного размера.
    Структура записи: timestamp, method, path, status, duration_ms, description.
    `Append()`, `List()`, `Subscribe() <-chan Entry` (для SSE).
    Без БД — чистый in-memory.
  - **Creates**:
    - `internal/activity/log.go`

- [ ] **2.3 Gateway: Upload + Search + Download + Metadata + Health**
  - **Dependencies**: 2.1
  - **Description**: Реализация всех методов Gateway Client:
    - `Upload(file, params) → UploadResult` (multipart/form-data)
    - `Search(params) → SearchResult` (POST JSON)
    - `Download(fileID) → io.ReadCloser, headers, error` (streaming)
    - `GetMetadata(fileID) → FileMetadata`
    - `HealthCheck() → map[string]bool` (IM ready, QM ready)
    Обработка специфичных ошибок: 410 → ErrFileArchived, 404 → ErrNotFound.
  - **Creates**:
    - `internal/gateway/upload.go`
    - `internal/gateway/search.go`
    - `internal/gateway/download.go`
    - `internal/gateway/metadata.go`
    - `internal/gateway/health.go`

- [ ] **2.4 Service Layer**
  - **Dependencies**: 2.2, 2.3
  - **Description**: Сервисы — тонкая прослойка над Gateway Client.
    - `UploadService`: single upload + batch upload (sequential, с отчётом по каждому файлу)
    - `SearchService`: формирование search request, парсинг фильтров
    - `DownloadService`: download + детекция 410 → возврат ArchivedFileInfo
    - `DashboardService`: health-checks + статистика (search count по retention/status)
    Все сервисы пишут в Activity Log.
  - **Creates**:
    - `internal/service/upload.go`
    - `internal/service/search.go`
    - `internal/service/download.go`
    - `internal/service/dashboard.go`

- [ ] **2.5 Unit-тесты Phase 2**
  - **Dependencies**: 2.2, 2.4
  - **Description**: Unit-тесты для компонентов Phase 2:
    - Activity Log: ring buffer overflow, concurrency (goroutines), Subscribe/Unsubscribe
    - Gateway Client: mock HTTP server (`httptest.NewServer`), проверка auth header injection,
      маппинг HTTP-кодов → типизированные ошибки (410→ErrFileArchived, 404→ErrNotFound)
    - Gateway методы: upload multipart, search JSON, download streaming
  - **Creates**:
    - `internal/activity/log_test.go`
    - `internal/gateway/client_test.go`

### Критерии завершения Phase 2

- [ ] Все подпункты завершены (2.1–2.5)
- [ ] Unit-тесты для Activity Log (ring buffer, concurrency)
- [ ] Unit-тесты для Gateway Client (mock HTTP server)
- [ ] `go test ./...` проходит

---

## Phase 3: UI Framework (layouts, components, i18n)

**Dependencies**: Phase 1
**Status**: Pending

### Описание

Базовый UI framework: Templ layouts, переиспользуемые компоненты,
i18n (RU + EN), Tailwind CSS, статические ресурсы (HTMX, Alpine.js).
Полностью аналогично Admin Module, но с собственной темой.

### Подпункты

- [ ] **3.1 Статические ресурсы + Tailwind + embed**
  - **Dependencies**: None
  - **Description**: Скачать и разместить JS-библиотеки:
    - `htmx.min.js` (HTMX 2.x) — динамические обновления UI
    - `htmx-sse.js` (HTMX SSE extension) — для Activity Log live-updates
    - `alpine.min.js` (Alpine.js 3.x) — клиентская интерактивность
    Tailwind config (тема как AM, но с другим акцентным цветом —
    синий для отличия от зелёного AM). input.css с Tailwind directives.
    `embed.go` с `//go:embed` директивами для встраивания static files
    в бинарник (паттерн как `src/admin-module/internal/ui/static/embed.go`).
  - **Creates**:
    - `internal/ui/static/js/htmx.min.js`
    - `internal/ui/static/js/htmx-sse.js`
    - `internal/ui/static/js/alpine.min.js`
    - `internal/ui/static/css/input.css`
    - `internal/ui/static/embed.go`
    - `tailwind.config.js`
  - **Links**:
    - `src/admin-module/internal/ui/static/` — референс embed.go

- [ ] **3.2 i18n (RU + EN)**
  - **Dependencies**: None
  - **Description**: Копировать паттерн из AM: Bundle, T(), Tf(), LangFromContext().
    Middleware для определения языка (cookie → Accept-Language → default "ru").
    Начальный набор ключей для навигации, общих элементов, ошибок.
    POST /set-language handler.
  - **Creates**:
    - `internal/ui/i18n/i18n.go`
    - `internal/ui/i18n/middleware.go`
    - `internal/ui/i18n/locales/ru.json`
    - `internal/ui/i18n/locales/en.json`

- [ ] **3.3 Layouts (base, page, sidebar)**
  - **Dependencies**: 3.1, 3.2
  - **Description**: Base layout (HTML5 shell, head, JS-подключения).
    Page layout (sidebar + main content area).
    Sidebar (навигация: Dashboard, Upload, Search, Settings + language switcher).
    Иконки — SVG inline (Heroicons или Lucide).
  - **Creates**:
    - `internal/ui/layouts/base.templ`
    - `internal/ui/layouts/page.templ`
    - `internal/ui/layouts/sidebar.templ`

- [ ] **3.4 Reusable Components**
  - **Dependencies**: 3.1
  - **Description**: Компоненты из Admin Module адаптированные для Demo Client:
    button, badge, toast, modal, data_table, pagination, stat_card,
    filter_panel, copy_button, tag_input, file_icon.
    Все с i18n поддержкой.
  - **Creates**:
    - `internal/ui/components/*.templ` (11 файлов)

### Критерии завершения Phase 3

- [ ] Все подпункты завершены (3.1–3.4)
- [ ] `templ generate` выполняется без ошибок
- [ ] Tailwind CSS компилируется
- [ ] i18n: все ключи есть в обоих локалях (ru.json, en.json)

---

## Phase 4: Dashboard + Activity Log + Settings

**Dependencies**: Phase 2, Phase 3
**Status**: Pending

### Описание

Реализация Dashboard (главная страница), Activity Log с SSE live-updates,
страница Settings. Регистрация UI-маршрутов, подключение handlers к сервисам.

### Подпункты

- [ ] **4.1 Регистрация UI-маршрутов**
  - **Dependencies**: None
  - **Description**: Chi router: UI routes (GET /, /upload, /search, /settings),
    partials (GET /partials/*), actions (POST /upload, /set-language),
    SSE (GET /activity/stream), download proxy (GET /download/{id}),
    static files (/static/*).
    Middleware stack: i18n, request logging.
  - **Creates**:
    - Обновление `internal/server/server.go` — регистрация UI routes

- [ ] **4.2 Dashboard page**
  - **Dependencies**: 4.1
  - **Description**: Dashboard handler + templ page.
    Карточки: Health (IM/QM статус), Token (валидность, TTL),
    Статистика (total, temporary, permanent, archived — через search API count).
    HTMX polling для auto-refresh статусов (каждые 30с).
    Activity Log panel внизу.
  - **Creates**:
    - `internal/ui/handlers/dashboard.go`
    - `internal/ui/pages/dashboard.templ`
    - `internal/ui/pages/partials/health_status.templ`
    - `internal/ui/pages/partials/token_status.templ`

- [ ] **4.3 Activity Log SSE**
  - **Dependencies**: 4.2, 2.2 (Activity Log backend)
  - **Description**: SSE handler для live-обновления лога операций.
    HTMX SSE extension подключает поток.
    При новой записи в Activity Log → push partial HTML через SSE.
    Начальная загрузка — все текущие записи.
  - **Creates**:
    - `internal/ui/handlers/activity.go`
    - `internal/ui/pages/partials/activity_row.templ`

- [ ] **4.4 Settings page**
  - **Dependencies**: 4.1
  - **Description**: Страница настроек (read-only):
    API Gateway URL, Token URL, Client ID, Scopes.
    Health-check зависимостей с HTMX refresh.
  - **Creates**:
    - `internal/ui/handlers/settings.go`
    - `internal/ui/pages/settings.templ`

### Критерии завершения Phase 4

- [ ] Все подпункты завершены (4.1–4.4)
- [ ] Dashboard отображает health, token status, статистику
- [ ] Activity Log обновляется в реальном времени (SSE)
- [ ] Settings показывает конфигурацию и health-checks
- [ ] Навигация по sidebar работает

---

## Phase 5: Upload (single + batch)

**Dependencies**: Phase 4
**Status**: Pending

### Описание

Форма загрузки файлов: одиночная загрузка с выбором retention policy и TTL,
batch upload нескольких файлов с общими параметрами и прогрессом.

### Подпункты

- [ ] **5.1 Single Upload**
  - **Dependencies**: None
  - **Description**: Форма: file input, description (textarea), tags (tag_input component),
    retention_policy (radio: temporary/permanent),
    ttl_days (number, видим только при temporary, Alpine.js toggle).
    POST multipart → backend → Gateway IM.
    Результат: upload_result partial с file_id, checksum, storage info.
    Обработка ошибок: 413, 502, 507 → toast с i18n сообщением.
    Метрики: `dc_uploads_total{status,retention}`.
  - **Creates**:
    - `internal/ui/handlers/upload.go`
    - `internal/ui/pages/upload.templ`
    - `internal/ui/pages/partials/upload_result.templ`

- [ ] **5.2 Batch Upload**
  - **Dependencies**: 5.1
  - **Description**: Расширение формы: file input с multiple.
    При выборе нескольких файлов — отображение списка файлов.
    Общие параметры (description, tags, retention, ttl).
    Последовательная загрузка через HTMX (один за другим).
    Прогресс: partial обновляется после каждого файла (N/M, ошибки).
    Итоговый отчёт: успешно/неудачно/всего.
  - **Creates**:
    - `internal/ui/pages/partials/batch_progress.templ`

### Критерии завершения Phase 5

- [ ] Все подпункты завершены (5.1–5.2)
- [ ] Одиночная загрузка temporary файла работает
- [ ] Одиночная загрузка permanent файла работает
- [ ] Batch upload 3+ файлов с прогрессом работает
- [ ] Ошибки отображаются через toast
- [ ] Activity Log записывает все upload операции

---

## Phase 6: Search + Download + AR handling

**Dependencies**: Phase 4
**Status**: Pending

### Описание

Поиск файлов (fulltext/partial/exact), результаты с пагинацией,
скачивание файлов, обработка FILE_ARCHIVED (410) с модальным окном.
Просмотр метаданных файла.

### Подпункты

- [ ] **6.1 Search page + results**
  - **Dependencies**: None
  - **Description**: Строка поиска (fulltext по умолчанию).
    Расширенные фильтры (раскрывающаяся панель, Alpine.js toggle):
    extension, tags, retention, status, date range, size range.
    Режим поиска: fulltext/partial/exact (select).
    Результаты: data_table с пагинацией (HTMX partial).
    Колонки: имя, размер, дата, теги, retention, se_mode (badge с цветом).
    Действия: Скачать, Метаданные.
    Для ar файлов: badge "Архивный", кнопка скачивания disabled.
    Сортировка через HTMX (клик по заголовку).
    Метрики: `dc_downloads_total{status}`.
  - **Creates**:
    - `internal/ui/handlers/search.go`
    - `internal/ui/pages/search.templ`
    - `internal/ui/pages/partials/search_results.templ`

- [ ] **6.2 Download + FILE_ARCHIVED modal**
  - **Dependencies**: 6.1
  - **Description**: GET /download/{id} → backend → Gateway QM → stream файл.
    Если 410 → рендерить archived_modal partial:
    - Сообщение (i18n): "Файл в архивном хранилище..."
    - file_id с copy_button (Alpine.js + navigator.clipboard)
    - Метаданные: имя, дата, теги
    - Инструкция: "Обратитесь к администратору..."
    Если 404 → toast "Файл не найден"
    Если 5xx → toast "Сервис недоступен"
  - **Creates**:
    - `internal/ui/handlers/download.go`
    - `internal/ui/pages/partials/archived_modal.templ`

- [ ] **6.3 File detail (metadata)**
  - **Dependencies**: 6.1
  - **Description**: Карточка/модалка с полной информацией о файле.
    HTMX: клик на файл → GET /partials/file/{id} → modal с метаданными.
    Поля: file_id, filename, content_type, size, checksum, uploaded_by,
    uploaded_at, description, tags, retention_policy, ttl_days, expires_at,
    status, storage_element_id, se_mode.
    Для temporary: визуальный TTL countdown.
    Для ar: предупреждение о недоступности.
  - **Creates**:
    - `internal/ui/pages/file_detail.templ`

### Критерии завершения Phase 6

- [ ] Все подпункты завершены (6.1–6.3)
- [ ] Fulltext поиск работает
- [ ] Фильтры (retention, tags, status) работают
- [ ] Пагинация и сортировка работают
- [ ] Скачивание файлов из rw/ro SE работает
- [ ] При попытке скачать ar файл — модалка с file_id + copy
- [ ] Карточка метаданных отображает полную информацию
- [ ] Activity Log записывает search/download операции

---

## Phase 7: Docker + Helm + Keycloak client + Integration

**Dependencies**: Phase 5, Phase 6
**Status**: Pending

### Описание

Сборка Docker-образа, Makefile, Helm chart, добавление Keycloak клиента
в тестовый Helm chart, HTTPRoute для Gateway API.
Интеграционное тестирование в K8s кластере.

### Подпункты

- [ ] **7.1 Dockerfile**
  - **Dependencies**: None
  - **Description**: Multi-stage: templ generate → Tailwind CSS → Go build → Alpine runtime.
    Паттерн как в AM. Non-root user, healthcheck, EXPOSE 8080.
    Makefile уже создан в Phase 1.1 — добавить docker-специфичные targets при необходимости.
  - **Creates**:
    - `Dockerfile`

- [ ] **7.2 docker-compose.yaml**
  - **Dependencies**: 7.1
  - **Description**: Для локальной разработки. Demo Client + env vars.
    Подключение к внешнему Gateway (artstore.kryukov.lan).
  - **Creates**:
    - `docker-compose.yaml`

- [ ] **7.3 Keycloak клиент `artstore-demo-client`**
  - **Dependencies**: None
  - **Description**: Добавить SA клиент в Keycloak realm config
    тестового Helm chart (`tests/helm/artstore-infra/`).
    Scopes: `files:read`, `files:write`.
    Secret: `demo-client-test-secret`.
    protocolMapper для `client_id` (как у ingester/query).
  - **Creates**:
    - Обновление `tests/helm/artstore-infra/` — Keycloak realm config

- [ ] **7.4 Helm chart**
  - **Dependencies**: 7.1, 7.3
  - **Description**: Helm chart для K8s деплоя.
    Deployment (1 replica, stateless), Service (ClusterIP, port 8080),
    HTTPRoute (Gateway API, host: `demoartstore.kryukov.lan`),
    ConfigMap (non-secret env), Secret (client_secret).
    values.yaml с dev defaults.
  - **Creates**:
    - `charts/demo-client/Chart.yaml`
    - `charts/demo-client/values.yaml`
    - `charts/demo-client/templates/*.yaml`

- [ ] **7.5 Docker build + push + K8s deploy + тестирование**
  - **Dependencies**: 7.2, 7.3, 7.4
  - **Description**: Сборка Docker образа (`v0.1.0-1`), push в Harbor.
    Deploy в K8s (namespace artstore-test).
    Ручное тестирование всех сценариев через браузер:
    dashboard, upload (temp + perm), search, download, AR modal.
    Добавить DNS запись `demoartstore.kryukov.lan → 192.168.218.180`.
  - **Creates**:
    - Docker image `harbor.kryukov.lan/library/demo-client:v0.1.0-1`

### Критерии завершения Phase 7

- [ ] Все подпункты завершены (7.1–7.5)
- [ ] Docker образ собран и запушен в Harbor
- [ ] Helm chart деплоится в K8s без ошибок
- [ ] Demo Client доступен по `https://demoartstore.kryukov.lan`
- [ ] Все user stories (US-01 — US-08) проверены
- [ ] Activity Log отображает все API-вызовы
- [ ] i18n переключение RU ↔ EN работает
- [ ] Модалка FILE_ARCHIVED с copy file_id работает
- [ ] CSP заголовки присутствуют в HTTP-ответах
- [ ] CSRF-токен включён в POST-формы

---

## Примечания

- **Phase 3 и Phase 2 могут выполняться параллельно** — UI framework не зависит от Gateway Client
- **Phase 5 и Phase 6 могут выполняться параллельно** — Upload и Search независимы, оба зависят от Phase 4
- **Акцентный цвет UI**: синий (blue-500/600) для визуального отличия от AM (зелёный)
- **Без БД**: приложение полностью stateless, Activity Log теряется при перезапуске — это ОК для демо
- **SA токен scope**: `files:read` + `files:write` достаточно для upload, search, download
- **Превью изображений**: отложено на v0.2.0, требует proxy через backend

---

**План готов к использованию.**
