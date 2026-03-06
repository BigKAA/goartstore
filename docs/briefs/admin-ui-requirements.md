# Admin UI — Спецификация требований

> **Статус**: Released v1.0
> **Дата**: 2026-02-23
> **Модуль**: Admin Module (src/admin-module)

## 1. Обзор

Встроенный веб-интерфейс для Admin Module — управление Storage Elements, файловым
реестром, пользователями и мониторинг состояния системы Artstore.

UI встраивается непосредственно в бинарник Admin Module (через `embed.FS`),
отдельный frontend-сервер не требуется.

## 2. Технологический стек

| Компонент | Технология | Размер |
|-----------|-----------|--------|
| Шаблоны | **Templ** (типобезопасные Go-шаблоны) | compile-time |
| Интерактивность (AJAX) | **HTMX 2.x** | ~14 KB |
| Клиентская реактивность | **Alpine.js** | ~17 KB |
| Графики | **ApexCharts** | ~50 KB |
| CSS-фреймворк | **Tailwind CSS v4** | compile-time |
| Real-time обновления | SSE через HTMX extension | — |
| Деплой | `embed.FS` в Go-бинарник | — |

**Сборка**: `templ generate && go build` — без Node.js в runtime.

### 2.1. Обоснование выбора

- **Templ** вместо `html/template`: типобезопасность (ошибки при компиляции),
  автокомплит в IDE, компонентный подход
- **HTMX**: AJAX-взаимодействие через HTML-атрибуты, без написания JavaScript.
  SSE extension для real-time обновлений
- **Alpine.js**: клиентская реактивность (модалки, фильтры, валидация форм)
  без тяжёлого SPA-фреймворка
- **ApexCharts**: интерактивные графики для дашбордов и мониторинга
- **Tailwind CSS v4**: utility-first CSS с поддержкой design tokens

## 3. Цветовая схема: Dark Green

```css
/* Фон (слои) */
--bg-base:      #0a0f0a;    /* почти чёрный с зелёным оттенком */
--bg-surface:   #111c11;    /* основная поверхность, карточки */
--bg-elevated:  #1a2e1a;    /* приподнятые элементы, модалки */
--bg-hover:     #243824;    /* hover-состояние */

/* Текст */
--text-primary:   #e8f5e8;  /* основной текст */
--text-secondary: #9cb89c;  /* вспомогательный текст */
--text-muted:     #5a7a5a;  /* неактивный текст */

/* Акценты (Brand/Action) */
--accent-primary:   #22c55e; /* emerald-500, основной акцент */
--accent-light:     #4ade80; /* green-400, hover */
--accent-bright:    #86efac; /* green-300, выделение */
--accent-lime:      #a3e635; /* lime-400, вторичный акцент */

/* Семантические */
--success:  #22c55e;  /* green-500 */
--warning:  #eab308;  /* yellow-500 */
--error:    #ef4444;  /* red-500 */
--info:     #3b82f6;  /* blue-500 */

/* Borders */
--border-default: #1e3a1e;
--border-accent:  #22c55e33; /* зелёный с opacity */

/* Режимы SE */
--mode-edit: #22c55e; /* зелёный */
--mode-rw:   #3b82f6; /* синий */
--mode-ro:   #eab308; /* жёлтый */
--mode-ar:   #6b7280; /* серый */
```

### 3.1. Рекомендации по визуальному дизайну

- **Sidebar**: `bg-base`, активный пункт с `accent-primary` подсветкой
- **Карточки метрик**: `bg-surface` с `border-accent` левой границей
- **Графики**: линии `accent-primary`, заливка с градиентом к прозрачному
- **Таблицы**: строки чередуются между `bg-surface` и `bg-elevated`
- **Кнопки primary**: `accent-primary` фон; secondary — `bg-elevated` + `accent-primary` border
- **Статусы SE**: online = `accent-bright`, degraded = `warning`, offline = `error`,
  maintenance = `info`

## 4. Структура навигации

```
📊 Dashboard          — Обзор состояния системы
🖥️ Мониторинг         — Детальный мониторинг, графики latency, алерты
💾 Storage Elements   — Управление хранилищами
📁 Файлы              — Файловый реестр
👥 Управление доступом — Пользователи [tab] и Service Accounts [tab]
⚙️ Настройки          — Конфигурация UI (Prometheus и др.) [admin only]
```

## 5. Функциональные требования по страницам

### 5.1. Dashboard (/)

| ID | Требование | Приоритет |
|----|-----------|-----------|
| FR-D1 | Карточки метрик: количество SE (по статусам), файлов (по режимам SE), объём хранилища (used/total %) | P0 |
| FR-D2 | Статус зависимостей: PostgreSQL, Keycloak (статус + latency), real-time через SSE | P0 |
| FR-D3 | Краткий список SE с цветовой индикацией mode/status | P0 |
| FR-D4 | График использования хранилища по SE (horizontal bar chart, ApexCharts) | P1 |
| FR-D5 | Распределение файлов по SE (donut chart) | P1 |
| FR-D6 | Доступ: admin и readonly | P0 |

### 5.2. Мониторинг (/monitoring)

| ID | Требование | Приоритет |
|----|-----------|-----------|
| FR-M1 | Здоровье зависимостей: PostgreSQL, Keycloak, каждый SE — статус, latency, real-time SSE | P0 |
| FR-M2 | Графики latency к каждому SE (line chart, за выбранный период) | P1 |
| FR-M3 | Графики latency PostgreSQL и Keycloak (line chart) | P1 |
| FR-M4 | Состояние фоновых задач: file sync, SA sync, dep health check — последний запуск, результат, интервал | P0 |
| FR-M5 | Алерты: SE offline/degraded, зависимость недоступна, SE заполнен >N% | P1 |
| FR-M6 | Использование хранилища по SE (stacked bar chart) | P1 |
| FR-M7 | Доступ: admin и readonly | P0 |

**Примечание**: Исторические данные latency берутся из Prometheus-метрик
(`app_dependency_latency_seconds`). Для графиков нужен SSE endpoint, который
периодически отправляет текущие значения метрик.

### 5.3. Storage Elements (/storage-elements)

| ID | Требование | Приоритет |
|----|-----------|-----------|
| FR-SE1 | Таблица SE: имя, URL, mode (цвет), status (цвет), capacity bar, файлов, latency, last sync | P0 |
| FR-SE2 | Discover: ввод URL, предпросмотр данных SE (modal) | P0 |
| FR-SE3 | Регистрация: после discover — создание записи + full sync файлов | P0 |
| FR-SE4 | Редактирование: имя, описание, URL (mode/status только через sync) | P0 |
| FR-SE5 | Ручная синхронизация одного SE | P0 |
| FR-SE6 | Массовая синхронизация всех SE | P1 |
| FR-SE7 | Удаление SE (с подтверждением, проверка отсутствия файлов) | P0 |
| FR-SE8 | Детальная страница SE: полная информация, список файлов на SE, история sync | P1 |
| FR-SE9 | Сводная статистика (группировка по mode, status) | P1 |
| FR-SE10 | Live-обновление статусов через SSE | P1 |
| FR-SE11 | Фильтры: по mode, status. Поиск по имени/URL | P0 |
| FR-SE12 | Роль: readonly — просмотр; admin — все операции | P0 |

### 5.4. Файлы (/files)

| ID | Требование | Приоритет |
|----|-----------|-----------|
| FR-F1 | Таблица файлов: имя, размер, тип (content_type), SE, uploaded_by, дата, статус | P0 |
| FR-F2 | Пагинация | P0 |
| FR-F3 | Фильтры: retention (permanent/temporary), SE, content_type | P0 |
| FR-F4 | Поиск по имени файла | P0 |
| FR-F5 | Сортировка: имя, размер, дата | P1 |
| FR-F6 | Modal с метаданными файла (все поля: checksum, tags, description и т.д.) | P0 |
| FR-F7 | Редактирование метаданных (description, tags) — только admin | P1 |
| FR-F8 | Hard delete с подтверждением — только admin | P0 |
| FR-F10 | Роль: readonly — просмотр; admin — редактирование, удаление | P0 |

**Примечание**: Скачивание содержимого файлов — задача Query Module. В v1 файловый
менеджер работает только с метаданными из реестра (file_registry в PostgreSQL).

### 5.5. Управление доступом (/access)

#### Tab: Пользователи

| ID | Требование | Приоритет |
|----|-----------|-----------|
| FR-U1 | Таблица пользователей (из Keycloak): username, email, groups, effective role, enabled | P0 |
| FR-U2 | Фильтры: по роли (admin/readonly), по статусу (enabled/disabled). Поиск по username/email | P0 |
| FR-U3 | Просмотр деталей пользователя (modal): группы IdP, IdP role, override, effective role | P0 |
| FR-U4 | Role override — повышение роли (readonly → admin) — только admin | P0 |
| FR-U5 | Удаление role override — только admin | P0 |
| FR-U6 | Роль: readonly — просмотр; admin — управление role overrides | P0 |

**Примечание**: Пользователи создаются и управляются в Keycloak. Admin Module
отображает их данные и может дополнять роль через локальную таблицу role_overrides.
Можно только повысить роль (readonly → admin), не понизить.

#### Tab: Service Accounts

| ID | Требование | Приоритет |
|----|-----------|-----------|
| FR-SA1 | Таблица SA: client_id, name, scopes, status, last_synced_at | P0 |
| FR-SA2 | Создание SA (modal): name, description, scopes. Отображение secret один раз + copy-to-clipboard | P0 |
| FR-SA3 | Редактирование SA: name, description, scopes, status (active/suspended) — только admin | P0 |
| FR-SA4 | Ротация secret: подтверждение, показ один раз + copy-to-clipboard — только admin | P0 |
| FR-SA5 | Удаление SA: с подтверждением — только admin | P0 |
| FR-SA6 | Синхронизация SA с Keycloak (кнопка) — только admin | P1 |
| FR-SA7 | Статус IdP (блок): connected/disconnected, last sync | P0 |
| FR-SA8 | Фильтр по status. Поиск по name/client_id | P0 |

## 6. Нефункциональные требования

| ID | Требование | Приоритет |
|----|-----------|-----------|
| NFR-1 | Аутентификация через Keycloak (Authorization Code + PKCE, клиент `artstore-admin-ui`) | P0 |
| NFR-2 | RBAC: admin (полный доступ), readonly (только просмотр) | P0 |
| NFR-3 | Responsive layout (sidebar collapse на мобильных/планшетах) | P1 |
| NFR-4 | Все статические ассеты через `embed.FS` — один бинарник | P0 |
| NFR-5 | Тёмная зелёная тема по умолчанию (без переключателя light/dark в v1) | P0 |
| NFR-6 | SSE для real-time обновлений (статусы SE, зависимости) | P1 |
| NFR-7 | Сборка: `templ generate && go build` (без Node.js в runtime) | P0 |
| NFR-8 | Поддержка браузеров: Chrome/Edge/Firefox/Safari последние 2 версии | P1 |

## 7. Архитектура (высокоуровневая)

### 7.1. Структура директорий

```
src/admin-module/
├── internal/
│   ├── api/                          # REST API (JSON) — существующий код
│   │   ├── generated/
│   │   ├── handlers/
│   │   └── middleware/
│   ├── ui/                           # Admin UI (HTML) — новый код
│   │   ├── layouts/                  # base.templ, sidebar.templ, header.templ
│   │   ├── pages/                    # dashboard.templ, monitoring.templ, ...
│   │   ├── components/               # stat_card.templ, table.templ, modal.templ, ...
│   │   ├── handlers/                 # HTTP handlers для UI-страниц
│   │   └── static/                   # Статические ассеты (embed.FS)
│   │       ├── css/                  # tailwind output, custom styles
│   │       └── js/                   # htmx.min.js, alpine.min.js, apexcharts.min.js
│   └── ...
```

### 7.2. Маршрутизация

```go
r := chi.NewRouter()

// REST API (JSON) — текущий код, без изменений
r.Route("/api/v1", func(r chi.Router) { ... })

// Admin UI (HTML) — новый код
r.Route("/admin", func(r chi.Router) {
    r.Use(uiAuthMiddleware)       // Keycloak OIDC (Authorization Code + PKCE)
    r.Get("/", uiHandlers.Dashboard)
    r.Get("/monitoring", uiHandlers.Monitoring)
    r.Get("/storage-elements", uiHandlers.SEList)
    r.Get("/storage-elements/{id}", uiHandlers.SEDetail)
    r.Get("/files", uiHandlers.FileList)
    r.Get("/access", uiHandlers.AccessManagement)

    // HTMX partial endpoints (возвращают фрагменты HTML)
    r.Route("/partials", func(r chi.Router) { ... })

    // SSE endpoints для real-time обновлений
    r.Route("/events", func(r chi.Router) { ... })
})

// Статические ассеты
r.Handle("/static/*", http.FileServer(http.FS(staticFS)))
```

### 7.3. Аутентификация UI

Для UI используется **OAuth 2.0 Authorization Code + PKCE** (не Client Credentials):

1. Пользователь открывает `/admin`
2. Middleware проверяет наличие session cookie
3. Если нет — redirect на Keycloak login page
4. После аутентификации — callback `/admin/callback` обменивает code на tokens
5. Access token сохраняется в HTTP-only cookie (или server-side session)
6. Все запросы к UI проходят через middleware, которая проверяет token

Keycloak клиент: `artstore-admin-ui` (public client, Authorization Code + PKCE).

## 8. Зависимости от текущего API

UI v1 использует **существующие** API endpoints Admin Module. Новые API endpoints
не требуются, за исключением:

| Новый endpoint | Назначение |
|---------------|-----------|
| `GET /admin/events/system-status` | SSE: статусы зависимостей и SE (real-time) |
| `GET /admin/events/se-updates` | SSE: обновления статусов SE |
| `GET /admin/partials/*` | HTMX partial responses (фрагменты HTML) |

Все данные берутся через вызовы к существующему service layer (не через HTTP API,
а напрямую через Go-интерфейсы сервисов).

### 8.2. Новые компоненты для UI

| Компонент | Назначение |
|-----------|-----------|
| Таблица `ui_settings` + миграция | Хранение конфигурации UI (Prometheus и др.) |
| `UISettingsRepository` | CRUD для `ui_settings` |
| `UISettingsService` | Бизнес-логика настроек |
| `PrometheusClient` | Опциональный клиент для PromQL API |
| OIDC callback handler | OAuth 2.0 Authorization Code + PKCE flow |
| Session middleware | Проверка/обновление зашифрованных cookies |

## 9. Отложено на будущие версии

| Функция | Причина | Ожидаемая версия |
|---------|---------|-----------------|
| Audit Log (таблица + API + UI) | Требует новой таблицы, миграции, API — большой scope | v2 |
| Batch-операции (bulk delete, export CSV/JSON) | Не критично для v1 | v2 |
| Скачивание файлов | Ждёт Query Module | v2 |
| Переключатель light/dark тема | Не критично | v2 |
| Drag-and-drop приоритеты SE | Сложно реализовать в HTMX | v2 |
| Уведомления (push, email, telegram) | Требует отдельного сервиса | v3 |

## 10. Решения по архитектурным вопросам

### 10.1. Tailwind CSS сборка

**Решение**: **Tailwind Standalone CLI** — бинарный файл без зависимости от Node.js.

- Скачивается один раз (~30 MB), добавляется в `.gitignore`
- Команда: `tailwindcss -i input.css -o output.css --minify`
- Интегрируется в Makefile: `make css` перед `templ generate && go build`
- В Docker multi-stage: скачивается на этапе сборки

### 10.2. Хранение сессий UI

**Решение**: **Зашифрованные HTTP-only cookies** (stateless).

- Access token + refresh token от Keycloak хранятся в AES-GCM зашифрованном cookie
- Ключ шифрования — env-переменная `AM_UI_SESSION_SECRET` (32 байта)
- Если `AM_UI_SESSION_SECRET` не задан — автогенерация при старте (random 32 bytes).
  Сессии сбросятся при перезапуске пода, но для разработки/малых инсталляций допустимо
- Не требуется таблица в БД, не нужен cleanup expired sessions
- Работает после перезапуска пода без потери сессий (при заданном ключе)
- Cookie атрибуты: `HttpOnly`, `Secure`, `SameSite=Lax`, `Path=/admin`
- Refresh: при истечении access token — автоматический refresh через Keycloak

### 10.3. Prometheus как опциональный источник данных

**Решение**: Prometheus — **опциональная** интеграция для исторических данных.

- Если Prometheus настроен — доступны графики latency, исторические метрики
- Если не настроен — графики не отображаются, только текущие значения (из
  topologymetrics в реальном времени)
- Конфигурация доступа к Prometheus хранится в **PostgreSQL** (таблица `ui_settings`)
- Управление через раздел **«Настройки»** в UI (доступен только admin)

**Параметры конфигурации Prometheus**:

| Параметр | Описание | По умолчанию |
|----------|---------|-------------|
| `prometheus_url` | URL Prometheus API (например `http://prometheus:9090`) | — (не настроен) |
| `prometheus_enabled` | Включена ли интеграция | `false` |
| `prometheus_query_timeout` | Таймаут запросов к Prometheus | `10s` |
| `metrics_retention_period` | Период запрашиваемых данных (для графиков) | `24h` |

**Дополнение к навигации**: добавляется пункт «Настройки» (только для admin):

```
📊 Dashboard
🖥️ Мониторинг
💾 Storage Elements
📁 Файлы
👥 Управление доступом
⚙️ Настройки              ← новый пункт (admin only)
```

### 10.4. Страница «Настройки» (/settings)

| ID | Требование | Приоритет |
|----|-----------|-----------|
| FR-S1 | Раздел «Prometheus»: URL, enabled, timeout, retention period | P1 |
| FR-S2 | Проверка подключения к Prometheus (кнопка «Проверить») | P1 |
| FR-S3 | Сохранение настроек в PostgreSQL (таблица `ui_settings`) | P1 |
| FR-S4 | Доступ: только admin | P0 |

**Таблица `ui_settings`** (key-value с типизацией):

```sql
CREATE TABLE ui_settings (
    key        TEXT PRIMARY KEY,
    value      TEXT NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_by TEXT NOT NULL
);
```

## 11. Открытые вопросы

1. **Количество SE в системе**: десятки или сотни? Влияет на подход к SSE и рендерингу
   таблиц. (Предполагаем десятки — до 50 SE.)
