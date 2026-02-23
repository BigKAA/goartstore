# План разработки: Admin UI для Admin Module

## Метаданные

- **Версия плана**: 1.0.0
- **Дата создания**: 2026-02-23
- **Последнее обновление**: 2026-02-23
- **Статус**: In Progress
- **Спецификация требований**: `docs/briefs/admin-ui-requirements.md`

---

## История версий

- **v1.0.0** (2026-02-23): Начальная версия плана

---

## Текущий статус

- **Активная фаза**: Phase 4 ✅ (завершена)
- **Активный подпункт**: —
- **Последнее обновление**: 2026-02-23
- **Примечание**: Phase 4 завершена, готово к Phase 5-7 (могут выполняться параллельно)

---

## Оглавление

- [x] [Phase 1: Фундамент и инструментарий](#phase-1-фундамент-и-инструментарий)
- [x] [Phase 2: Аутентификация и сессии](#phase-2-аутентификация-и-сессии)
- [x] [Phase 3: Базовый layout и UI-компоненты](#phase-3-базовый-layout-и-ui-компоненты)
- [x] [Phase 4: Dashboard](#phase-4-dashboard)
- [ ] [Phase 5: Storage Elements](#phase-5-storage-elements)
- [ ] [Phase 6: Файловый менеджер](#phase-6-файловый-менеджер)
- [ ] [Phase 7: Управление доступом](#phase-7-управление-доступом)
- [ ] [Phase 8: Мониторинг, SSE и настройки](#phase-8-мониторинг-sse-и-настройки)
- [ ] [Phase 9: Сборка, деплой и тестирование](#phase-9-сборка-деплой-и-тестирование)

---

## Phase 1: Фундамент и инструментарий

**Dependencies**: None
**Status**: ✅ Done

### Описание

Подготовка инфраструктуры для разработки UI: установка и настройка Templ, Tailwind
Standalone CLI, подключение статических JS-библиотек (HTMX, Alpine.js, ApexCharts),
настройка `embed.FS` для статики, обновление Makefile и go.mod. Создание миграции
БД для таблицы `ui_settings`. Базовая конфигурация UI (новые env-переменные).

### Подпункты

- [x] **1.1 Зависимости и Templ**
  - **Dependencies**: None
  - **Description**: Добавить зависимость `github.com/a-h/templ` в go.mod.
    Установить `templ` CLI (`go install github.com/a-h/templ/cmd/templ@latest`).
    Создать структуру директорий UI:
    `internal/ui/{layouts,pages,components,handlers,static/{css,js}}`.
    Создать минимальный `.templ`-файл для проверки кодогенерации.
  - **Creates**:
    - `internal/ui/layouts/` (директория)
    - `internal/ui/pages/` (директория)
    - `internal/ui/components/` (директория)
    - `internal/ui/handlers/` (директория)
    - `internal/ui/static/css/` (директория)
    - `internal/ui/static/js/` (директория)
    - `internal/ui/hello.templ` (проверочный файл, удалить после проверки)
  - **Links**:
    - [Templ — Getting Started](https://templ.guide/quick-start/installation)

- [x] **1.2 Tailwind CSS Standalone CLI**
  - **Dependencies**: 1.1
  - **Description**: Скачать Tailwind Standalone CLI (бинарник, без Node.js).
    Создать `tailwind.config.js` с dark green дизайн-токенами из спецификации.
    Создать `internal/ui/static/css/input.css` с директивами `@tailwind` и
    кастомными CSS-переменными. Добавить `tailwindcss` бинарник в `.gitignore`.
    Настроить content paths для `.templ` файлов.
  - **Creates**:
    - `tailwind.config.js`
    - `internal/ui/static/css/input.css`
    - `internal/ui/static/css/output.css` (скомпилированный, embed)
  - **Links**:
    - [Tailwind Standalone CLI](https://tailwindcss.com/blog/standalone-cli)

- [x] **1.3 Статические JS-ассеты и embed.FS**
  - **Dependencies**: 1.1
  - **Description**: Скачать минифицированные JS-файлы: `htmx.min.js` (v2.x),
    `htmx-sse.js` (SSE extension), `alpine.min.js` (v3.x), `apexcharts.min.js`.
    Создать `internal/ui/static/embed.go` с `//go:embed` директивой для всей
    директории `static/`. Добавить обработчик `/static/*` в роутер.
  - **Creates**:
    - `internal/ui/static/js/htmx.min.js`
    - `internal/ui/static/js/htmx-sse.js`
    - `internal/ui/static/js/alpine.min.js`
    - `internal/ui/static/js/apexcharts.min.js`
    - `internal/ui/static/embed.go`
  - **Links**:
    - [HTMX — Installation](https://htmx.org/docs/#installing)
    - [HTMX SSE Extension](https://htmx.org/extensions/sse/)
    - [Alpine.js — Installation](https://alpinejs.dev/essentials/installation)
    - [ApexCharts CDN](https://cdn.jsdelivr.net/npm/apexcharts)

- [x] **1.4 Миграция БД: таблица ui_settings**
  - **Dependencies**: None
  - **Description**: Создать миграцию `003_ui_settings.up.sql` / `.down.sql`.
    Таблица `ui_settings` (key TEXT PK, value TEXT, updated_at TIMESTAMPTZ,
    updated_by TEXT). Создать `UISettingsRepository` с методами Get, Set, List,
    Delete. Создать `UISettingsService` с бизнес-логикой (валидация ключей,
    типизированные геттеры для Prometheus-конфигурации).
  - **Creates**:
    - `internal/database/migrations/003_ui_settings.up.sql`
    - `internal/database/migrations/003_ui_settings.down.sql`
    - `internal/repository/ui_settings.go`
    - `internal/service/ui_settings.go`
  - **Links**: N/A

- [x] **1.5 Конфигурация UI и Makefile**
  - **Dependencies**: 1.1, 1.2, 1.3, 1.4
  - **Description**: Добавить новые env-переменные в `config.go`:
    `AM_UI_ENABLED` (bool, default true), `AM_UI_SESSION_SECRET` (string, optional,
    автогенерация если пусто), `AM_UI_OIDC_CLIENT_ID` (string, default
    `artstore-admin-ui`). Обновить Makefile: добавить цели `templ-generate`,
    `css-build`, `ui-build` (объединённая), обновить `build` цель. Обновить
    `.gitignore` для tailwindcss бинарника и _templ.go файлов (опционально).
  - **Creates**:
    - Изменения в `internal/config/config.go`
    - Изменения в `Makefile`
    - Изменения в `.gitignore`
  - **Links**: N/A

### Критерии завершения Phase 1

- [x] Все подпункты завершены (1.1–1.5)
- [x] `templ generate` успешно генерирует Go-код из `.templ` файлов
- [x] `tailwindcss` компилирует CSS с кастомными дизайн-токенами
- [x] Статические JS-файлы доступны через `embed.FS`
- [x] `go build` проходит без ошибок с новыми зависимостями
- [ ] Миграция `003_ui_settings` применяется без ошибок (проверить при деплое)
- [x] `make build` (или `make ui-build && make build`) собирает бинарник

---

## Phase 2: Аутентификация и сессии

**Dependencies**: Phase 1
**Status**: ✅ Done

### Описание

Реализация OAuth 2.0 Authorization Code + PKCE flow для аутентификации пользователей
UI через Keycloak. Зашифрованные HTTP-only cookies для хранения сессий (access token +
refresh token). UI auth middleware для проверки и обновления токенов. Настройка
Keycloak-клиента `artstore-admin-ui`.

### Подпункты

- [x] **2.1 OIDC-клиент и session crypto**
  - **Dependencies**: None
  - **Description**: Создать пакет `internal/ui/auth/` с компонентами:
    (a) `session.go` — шифрование/дешифрование cookie (AES-256-GCM), структура
    `SessionData` (access_token, refresh_token, expires_at, username, role);
    (b) `oidc.go` — OIDC-клиент: генерация PKCE (code_verifier + code_challenge),
    формирование authorization URL, обмен code → tokens, refresh tokens;
    (c) Использовать стандартный `crypto/aes`, `crypto/cipher`, `crypto/rand`.
    Ключ шифрования из `AM_UI_SESSION_SECRET` или автогенерация (32 bytes).
  - **Creates**:
    - `internal/ui/auth/session.go`
    - `internal/ui/auth/oidc.go`
  - **Links**:
    - [RFC 7636 — PKCE](https://tools.ietf.org/html/rfc7636)
    - [Keycloak OIDC Endpoints](https://www.keycloak.org/docs/latest/securing_apps/)

- [x] **2.2 Auth handlers: login, callback, logout**
  - **Dependencies**: 2.1
  - **Description**: Создать `internal/ui/handlers/auth.go` с обработчиками:
    (a) `GET /admin/login` — redirect на Keycloak authorize endpoint с PKCE;
    (b) `GET /admin/callback` — обмен authorization code на tokens, создание
    зашифрованного cookie, redirect на `/admin`;
    (c) `POST /admin/logout` — очистка cookie, redirect на Keycloak logout;
    (d) Сохранять PKCE code_verifier в short-lived cookie (state cookie).
  - **Creates**:
    - `internal/ui/handlers/auth.go`
  - **Links**: N/A

- [x] **2.3 UI auth middleware**
  - **Dependencies**: 2.1, 2.2
  - **Description**: Создать `internal/ui/middleware/auth.go`:
    (a) Извлекать сессию из зашифрованного cookie;
    (b) Проверять срок действия access token;
    (c) При истечении — автоматический refresh через Keycloak;
    (d) При невозможности refresh — redirect на `/admin/login`;
    (e) Помещать `SessionData` (username, role, groups) в context запроса;
    (f) Middleware применяется ко всем `/admin/*` маршрутам, кроме
    `/admin/login`, `/admin/callback`, `/admin/logout`.
  - **Creates**:
    - `internal/ui/middleware/auth.go`
  - **Links**: N/A

- [x] **2.4 Интеграция в роутер и main.go**
  - **Dependencies**: 2.2, 2.3
  - **Description**: Обновить `server.go`: добавить UI-роуты `/admin/*` с
    ui auth middleware. Обновить `main.go`: инициализировать OIDC-клиент,
    session manager, UI auth middleware, UI auth handlers. Зарегистрировать
    маршруты: `/admin/login`, `/admin/callback`, `/admin/logout`.
    Добавить обработчик `/static/*` для embed.FS. Условная регистрация
    UI маршрутов при `AM_UI_ENABLED=true`.
  - **Creates**:
    - Изменения в `internal/server/server.go`
    - Изменения в `cmd/admin-module/main.go`
  - **Links**: N/A

- [x] **2.5 Настройка Keycloak-клиента для тестов**
  - **Dependencies**: 2.4
  - **Description**: Обновить тестовый Helm chart `tests/helm/artstore-infra/`
    (или init-job): добавить Keycloak-клиент `artstore-admin-ui` (public client,
    Authorization Code + PKCE, redirect URI: `http://localhost:8000/admin/callback`
    и `https://artstore.kryukov.lan/admin/callback`). Документировать ручную
    настройку клиента в Keycloak для production.
  - **Creates**:
    - Изменения в тестовом Keycloak realm export / init-job
  - **Links**: N/A

### Критерии завершения Phase 2

- [x] Все подпункты завершены (2.1–2.5)
- [x] Переход на `/admin` без cookie → redirect на Keycloak login
- [x] После аутентификации в Keycloak → redirect на `/admin` с cookie
- [x] Повторный запрос `/admin` с cookie → доступ без redirect
- [x] Logout → cookie удалён, redirect на Keycloak logout
- [x] Expired access token → автоматический refresh
- [x] Unit-тесты для session crypto (encrypt/decrypt round-trip)
- [ ] Контейнер собирается и запускается (проверить при деплое)

---

## Phase 3: Базовый layout и UI-компоненты

**Dependencies**: Phase 2
**Status**: ✅ Done

### Описание

Создание базового HTML-layout (темная зелёная тема), sidebar навигации, header с
информацией о пользователе. Библиотека переиспользуемых Templ-компонентов: карточки
метрик, таблицы, модальные окна, пагинация, фильтры, бейджи, кнопки, индикаторы
статуса, progress bars.

### Подпункты

- [x] **3.1 Base layout**
  - **Dependencies**: None
  - **Description**: Создать `internal/ui/layouts/base.templ` — HTML5 shell:
    `<head>` (meta, tailwind CSS, favicon), `<body>` (dark bg-base),
    подключение JS (htmx, alpine, apexcharts в конце body).
    Определить layout structure: sidebar (fixed left) + main content area.
    Создать `internal/ui/layouts/page.templ` — обёртка страницы с заголовком,
    breadcrumbs.
  - **Creates**:
    - `internal/ui/layouts/base.templ`
    - `internal/ui/layouts/page.templ`
  - **Links**: N/A

- [x] **3.2 Sidebar и header**
  - **Dependencies**: 3.1
  - **Description**: Создать `internal/ui/layouts/sidebar.templ` — боковая
    навигация с пунктами: Dashboard, Мониторинг, Storage Elements, Файлы,
    Управление доступом, Настройки (admin only). Активный пункт с
    `accent-primary` подсветкой. Логотип/название Artstore сверху.
    Collapse на мобильных (Alpine.js `x-data`).
    Создать `internal/ui/layouts/header.templ` — верхняя панель: заголовок
    страницы, имя пользователя, роль (badge), кнопка logout.
  - **Creates**:
    - `internal/ui/layouts/sidebar.templ`
    - `internal/ui/layouts/header.templ`
  - **Links**: N/A

- [x] **3.3 Базовые UI-компоненты (часть 1)**
  - **Dependencies**: 3.1
  - **Description**: Создать переиспользуемые Templ-компоненты:
    (a) `stat_card.templ` — карточка метрики (значение, подпись, иконка,
    цвет левой границы);
    (b) `badge.templ` — бейдж статуса (online/offline/degraded, mode edit/rw/ro/ar,
    role admin/readonly) с цветовой индикацией;
    (c) `button.templ` — primary, secondary, danger, disabled варианты;
    (d) `capacity_bar.templ` — progress bar (used/total) с процентом;
    (e) `alert.templ` — success, warning, error, info уведомления.
  - **Creates**:
    - `internal/ui/components/stat_card.templ`
    - `internal/ui/components/badge.templ`
    - `internal/ui/components/button.templ`
    - `internal/ui/components/capacity_bar.templ`
    - `internal/ui/components/alert.templ`
  - **Links**: N/A

- [x] **3.4 Базовые UI-компоненты (часть 2)**
  - **Dependencies**: 3.1
  - **Description**: Создать компоненты для работы с данными:
    (a) `data_table.templ` — базовая таблица с заголовками, строками,
    чередующимися цветами (bg-surface/bg-elevated), hover-эффектом;
    (b) `pagination.templ` — пагинация (prev/next, номера страниц,
    total items), HTMX-навигация;
    (c) `modal.templ` — модальное окно (Alpine.js `x-show`, backdrop,
    закрытие по Escape/клику вне), слоты: header, body, footer;
    (d) `filter_bar.templ` — панель фильтров (select, search input), HTMX
    `hx-get` для фильтрации;
    (e) `confirm_dialog.templ` — диалог подтверждения (Alpine.js) для
    опасных операций (delete).
  - **Creates**:
    - `internal/ui/components/data_table.templ`
    - `internal/ui/components/pagination.templ`
    - `internal/ui/components/modal.templ`
    - `internal/ui/components/filter_bar.templ`
    - `internal/ui/components/confirm_dialog.templ`
  - **Links**: N/A

- [x] **3.5 Страница-заглушка и проверка layout**
  - **Dependencies**: 3.1, 3.2, 3.3, 3.4
  - **Description**: Создать временную страницу Dashboard (`internal/ui/pages/
    dashboard.templ`) с использованием base layout, sidebar, header и нескольких
    компонентов (stat_card, badge, button) для визуальной проверки темы и layout.
    Создать `internal/ui/handlers/dashboard.go` с handler для `GET /admin/`.
    Зарегистрировать маршрут в server.go. Проверить визуально в браузере.
  - **Creates**:
    - `internal/ui/pages/dashboard.templ` (временная версия)
    - `internal/ui/handlers/dashboard.go`
    - Изменения в `internal/server/server.go`
  - **Links**: N/A

### Критерии завершения Phase 3

- [x] Все подпункты завершены (3.1–3.5)
- [x] Страница `/admin/` отображается с sidebar, header и компонентами
- [x] Тёмная зелёная тема визуально корректна (цвета из спецификации)
- [x] Sidebar подсвечивает активный пункт
- [x] Sidebar сворачивается на мобильных (responsive)
- [x] Модальное окно открывается/закрывается корректно (Alpine.js)
- [ ] Контейнер собирается и запускается (проверить при деплое)

---

## Phase 4: Dashboard

**Dependencies**: Phase 3
**Status**: ✅ Done

### Описание

Полноценная страница Dashboard: карточки метрик (SE по статусам, файлы по режимам,
объём хранилища), блок статуса зависимостей (PostgreSQL, Keycloak), список SE с
цветовой индикацией, графики (использование хранилища, распределение файлов).

### Подпункты

- [x] **4.1 Dashboard handler и сбор данных**
  - **Dependencies**: None
  - **Description**: Обновить `internal/ui/handlers/dashboard.go`: собрать данные
    из существующих сервисов — `StorageElementService.ListSE()`,
    `FileRegistryService.ListFiles()`, `DepHealthService` (статусы зависимостей).
    Создать структуру `DashboardData` с агрегированными метриками (counts by
    status, mode; total/used storage; dependency statuses).
  - **Creates**:
    - Изменения в `internal/ui/handlers/dashboard.go`
  - **Links**: N/A

- [x] **4.2 Карточки метрик и статус зависимостей**
  - **Dependencies**: 4.1
  - **Description**: Обновить `internal/ui/pages/dashboard.templ`:
    (a) Верхний ряд — 4 stat_card: SE (total, online/offline/degraded),
    Файлы (total, по retention), Хранилище (used/total GB, % bar),
    Service Accounts (total, active/suspended);
    (b) Блок зависимостей — PostgreSQL, Keycloak: статус (badge online/offline),
    latency (мс). Использовать данные из topologymetrics/DepHealthService.
  - **Creates**:
    - Изменения в `internal/ui/pages/dashboard.templ`
  - **Links**: N/A

- [x] **4.3 Список SE и графики**
  - **Dependencies**: 4.2
  - **Description**: Дополнить dashboard:
    (a) Компактная таблица SE: имя, mode (badge), status (badge),
    capacity bar, файлов. Ссылка на `/admin/storage-elements/{id}`;
    (b) График: использование хранилища по SE (horizontal bar chart,
    ApexCharts). Alpine.js `x-init` для инициализации графика;
    (c) График: распределение файлов по SE (donut chart, ApexCharts).
    Данные передаются из handler через JSON в data-атрибуте элемента
    или inline `<script>`.
  - **Creates**:
    - Изменения в `internal/ui/pages/dashboard.templ`
    - `internal/ui/static/js/charts.js` (опционально, хелперы для ApexCharts)
  - **Links**:
    - [ApexCharts — Bar Chart](https://apexcharts.com/javascript-chart-demos/bar-charts/)
    - [ApexCharts — Donut Chart](https://apexcharts.com/javascript-chart-demos/pie-charts/)

### Критерии завершения Phase 4

- [x] Все подпункты завершены (4.1–4.3)
- [x] Dashboard показывает актуальные метрики из БД
- [x] Карточки отображают корректные числа (SE, файлы, хранилище, SA)
- [x] Статусы зависимостей (PG, KC) отображаются корректно
- [x] Список SE с цветовыми бейджами mode/status
- [x] Графики рендерятся (bar chart, donut chart)
- [ ] Контейнер собирается и запускается (проверить при деплое)

---

## Phase 5: Storage Elements

**Dependencies**: Phase 3
**Status**: Pending

### Описание

Полноценная страница управления Storage Elements: таблица с фильтрами и поиском,
discover (предпросмотр по URL), регистрация, редактирование, удаление, синхронизация
(одиночная и массовая), детальная страница SE.

### Подпункты

- [ ] **5.1 SE list page и таблица**
  - **Dependencies**: None
  - **Description**: Создать `internal/ui/pages/se_list.templ` — страница
    списка SE с data_table: имя, URL, mode (badge), status (badge),
    capacity_bar, файлов (число), latency (мс), last_sync_at.
    Создать `internal/ui/handlers/storage_elements.go` — handler для
    `GET /admin/storage-elements`, получает данные из `StorageElementService`.
    Кнопки действий в каждой строке: Sync, Edit, Delete.
    Кнопка «Добавить SE» (открывает discover modal).
  - **Creates**:
    - `internal/ui/pages/se_list.templ`
    - `internal/ui/handlers/storage_elements.go`
  - **Links**: N/A

- [ ] **5.2 Фильтры и поиск**
  - **Dependencies**: 5.1
  - **Description**: Добавить filter_bar на страницу SE:
    (a) Фильтр по mode: select (all, edit, rw, ro, ar);
    (b) Фильтр по status: select (all, online, offline, degraded, maintenance);
    (c) Поиск по имени/URL: text input с debounce.
    Фильтрация через HTMX `hx-get` — partial endpoint
    `GET /admin/partials/se-table` возвращает только tbody.
    Создать partial handler.
  - **Creates**:
    - Изменения в `internal/ui/pages/se_list.templ`
    - `internal/ui/pages/partials/se_table.templ`
    - Изменения в `internal/ui/handlers/storage_elements.go` (partial handler)
  - **Links**: N/A

- [ ] **5.3 Discover и регистрация SE**
  - **Dependencies**: 5.1
  - **Description**: Реализовать flow добавления SE:
    (a) Кнопка «Добавить SE» → modal с полем URL;
    (b) HTMX POST `/admin/partials/se-discover` → вызов
    `StorageElementService.DiscoverSE(url)` → отображение preview
    (имя, mode, storage_type, capacity, used, файлов);
    (c) Кнопка «Зарегистрировать» в preview → HTMX POST
    `/admin/partials/se-register` → вызов `StorageElementService.RegisterSE()`;
    (d) После регистрации — обновить таблицу (HTMX swap).
    Создать templ-компоненты для discover modal и preview.
  - **Creates**:
    - `internal/ui/pages/partials/se_discover.templ`
    - Изменения в `internal/ui/handlers/storage_elements.go`
  - **Links**: N/A

- [ ] **5.4 Редактирование, удаление, синхронизация**
  - **Dependencies**: 5.1
  - **Description**: Реализовать действия над SE:
    (a) Edit: modal с формой (name, description, URL).
    HTMX PUT `/admin/partials/se-edit/{id}` → `StorageElementService.UpdateSE()`.
    После сохранения — обновить строку таблицы;
    (b) Delete: confirm_dialog → HTMX DELETE `/admin/partials/se-delete/{id}`.
    Проверка наличия файлов (если есть — показать ошибку);
    (c) Sync: HTMX POST `/admin/partials/se-sync/{id}` →
    `StorageElementService.SyncSE()`. Показать результат (изменения);
    (d) Sync All: кнопка «Синхронизировать всё» →
    POST `/admin/partials/se-sync-all`.
  - **Creates**:
    - `internal/ui/pages/partials/se_edit.templ`
    - Изменения в `internal/ui/handlers/storage_elements.go`
  - **Links**: N/A

- [ ] **5.5 Детальная страница SE**
  - **Dependencies**: 5.1
  - **Description**: Создать `internal/ui/pages/se_detail.templ` —
    страница `/admin/storage-elements/{id}`:
    (a) Полная информация о SE (все поля из модели);
    (b) Capacity bar (большой);
    (c) Сводная статистика (файлов, занято, свободно);
    (d) Список файлов на этом SE (компактная таблица с пагинацией,
    HTMX partial `/admin/partials/se-files/{id}`);
    (e) Кнопки: Sync, Edit, Back to list.
    Создать handler `GET /admin/storage-elements/{id}`.
  - **Creates**:
    - `internal/ui/pages/se_detail.templ`
    - `internal/ui/pages/partials/se_files.templ`
    - Изменения в `internal/ui/handlers/storage_elements.go`
  - **Links**: N/A

### Критерии завершения Phase 5

- [ ] Все подпункты завершены (5.1–5.5)
- [ ] Таблица SE отображает все зарегистрированные SE
- [ ] Фильтрация по mode/status и поиск работают (HTMX partial swap)
- [ ] Discover: ввод URL → предпросмотр данных SE
- [ ] Регистрация SE: предпросмотр → кнопка → SE в таблице
- [ ] Редактирование: modal → сохранение → обновление строки
- [ ] Удаление: confirm → удаление (или ошибка если есть файлы)
- [ ] Sync: кнопка → синхронизация → обновление данных
- [ ] Детальная страница: полная информация + список файлов
- [ ] RBAC: readonly видит всё, но кнопки действий скрыты
- [ ] Контейнер собирается и запускается

---

## Phase 6: Файловый менеджер

**Dependencies**: Phase 3
**Status**: Pending

### Описание

Страница файлового реестра: таблица с пагинацией, фильтрами, поиском, сортировкой.
Modal с метаданными файла. Редактирование (description, tags) и soft delete для
админов.

### Подпункты

- [ ] **6.1 File list page с пагинацией**
  - **Dependencies**: None
  - **Description**: Создать `internal/ui/pages/file_list.templ` — страница
    списка файлов с data_table: original_filename, size (human-readable),
    content_type, SE name, uploaded_by, uploaded_at, status (badge).
    Пагинация (pagination component) через HTMX partial
    `GET /admin/partials/file-table?page=N`.
    Создать `internal/ui/handlers/files.go` — handler для
    `GET /admin/files`, получает данные из `FileRegistryService`.
  - **Creates**:
    - `internal/ui/pages/file_list.templ`
    - `internal/ui/pages/partials/file_table.templ`
    - `internal/ui/handlers/files.go`
  - **Links**: N/A

- [ ] **6.2 Фильтры, поиск, сортировка**
  - **Dependencies**: 6.1
  - **Description**: Добавить filter_bar:
    (a) Фильтр по status: select (active/deleted/expired);
    (b) Фильтр по retention: select (permanent/temporary);
    (c) Фильтр по SE: select (список зарегистрированных SE);
    (d) Фильтр по content_type: select (основные MIME-типы);
    (e) Поиск по имени: text input с debounce;
    (f) Сортировка: клик по заголовку колонки (name, size, date)
    через HTMX `hx-get` с параметрами sort/order.
    «Показать удалённые» toggle — видим только admin (Alpine.js `x-show`
    проверка роли из data-атрибута).
  - **Creates**:
    - Изменения в `internal/ui/pages/file_list.templ`
    - Изменения в `internal/ui/pages/partials/file_table.templ`
    - Изменения в `internal/ui/handlers/files.go`
  - **Links**: N/A

- [ ] **6.3 File detail modal и действия**
  - **Dependencies**: 6.1
  - **Description**: Реализовать:
    (a) Клик по строке файла → HTMX GET `/admin/partials/file-detail/{id}`
    → modal с полными метаданными (все поля: file_id, original_filename,
    content_type, size, checksum, SE, uploaded_by, uploaded_at, description,
    tags, status, retention_policy, ttl_days, expires_at);
    (b) Кнопка «Редактировать» (admin only) → inline-формы для description
    (textarea) и tags (input, comma-separated) → HTMX PUT
    `/admin/partials/file-edit/{id}`;
    (c) Кнопка «Удалить» (admin only) → confirm_dialog → HTMX DELETE
    `/admin/partials/file-delete/{id}`.
  - **Creates**:
    - `internal/ui/pages/partials/file_detail.templ`
    - Изменения в `internal/ui/handlers/files.go`
  - **Links**: N/A

### Критерии завершения Phase 6

- [ ] Все подпункты завершены (6.1–6.3)
- [ ] Таблица файлов с пагинацией работает
- [ ] Фильтрация по status/retention/SE/content_type работает
- [ ] Поиск по имени файла работает
- [ ] Сортировка по клику на заголовок работает
- [ ] Modal с метаданными отображает все поля
- [ ] Редактирование description/tags сохраняет изменения (admin only)
- [ ] Soft delete работает с подтверждением (admin only)
- [ ] «Показать удалённые» toggle работает (admin only)
- [ ] RBAC: readonly — только просмотр, кнопки действий скрыты
- [ ] Контейнер собирается и запускается

---

## Phase 7: Управление доступом

**Dependencies**: Phase 3
**Status**: Pending

### Описание

Страница «Управление доступом» с двумя табами: Пользователи (из Keycloak + role
overrides) и Service Accounts (CRUD + ротация secret + синхронизация с Keycloak).

### Подпункты

- [ ] **7.1 Страница с табами и Users list**
  - **Dependencies**: None
  - **Description**: Создать `internal/ui/pages/access.templ` — страница
    с двумя табами (Alpine.js `x-data` для переключения):
    Tab 1 «Пользователи», Tab 2 «Service Accounts».
    Реализовать tab «Пользователи»: таблица (username, email, groups,
    effective_role badge, enabled badge). Фильтры: по роли (admin/readonly),
    по статусу (enabled/disabled). Поиск по username/email.
    HTMX partial для фильтрации.
    Создать `internal/ui/handlers/access.go`.
  - **Creates**:
    - `internal/ui/pages/access.templ`
    - `internal/ui/pages/partials/users_table.templ`
    - `internal/ui/handlers/access.go`
  - **Links**: N/A

- [ ] **7.2 User detail и role overrides**
  - **Dependencies**: 7.1
  - **Description**: Реализовать:
    (a) Клик по строке пользователя → modal с деталями: username, email,
    first_name, last_name, groups, idp_role, role_override, effective_role,
    enabled, created_at;
    (b) Кнопка «Повысить роль» (admin only, если effective_role != admin) →
    HTMX POST `/admin/partials/user-role-override/{id}` →
    `AdminUserService.AddRoleOverride()`;
    (c) Кнопка «Убрать дополнение» (admin only, если есть override) →
    confirm → HTMX DELETE `/admin/partials/user-role-override/{id}` →
    `AdminUserService.RemoveRoleOverride()`.
  - **Creates**:
    - `internal/ui/pages/partials/user_detail.templ`
    - Изменения в `internal/ui/handlers/access.go`
  - **Links**: N/A

- [ ] **7.3 Service Accounts tab — список и создание**
  - **Dependencies**: 7.1
  - **Description**: Реализовать tab «Service Accounts»:
    (a) Таблица SA: client_id, name, scopes (badges), status (badge),
    last_synced_at. Фильтр по status, поиск по name/client_id;
    (b) Блок статуса IdP: connected/disconnected, last SA sync time;
    (c) Кнопка «Создать SA» (admin only) → modal с формой: name,
    description, scopes (multi-select checkboxes). HTMX POST
    `/admin/partials/sa-create` → `ServiceAccountService.CreateSA()`.
    **Важно**: после создания показать client_id и secret один раз
    в modal с кнопкой copy-to-clipboard (Alpine.js
    `navigator.clipboard.writeText()`).
  - **Creates**:
    - `internal/ui/pages/partials/sa_table.templ`
    - `internal/ui/pages/partials/sa_create.templ`
    - Изменения в `internal/ui/handlers/access.go`
  - **Links**: N/A

- [ ] **7.4 SA: редактирование, удаление, ротация, sync**
  - **Dependencies**: 7.3
  - **Description**: Реализовать действия над SA:
    (a) Edit: modal с формой (name, description, scopes, status).
    HTMX PUT `/admin/partials/sa-edit/{id}`;
    (b) Delete: confirm_dialog → HTMX DELETE `/admin/partials/sa-delete/{id}`;
    (c) Rotate secret: confirm_dialog (предупреждение: старый secret
    перестанет работать) → HTMX POST `/admin/partials/sa-rotate/{id}` →
    показать новый secret один раз (copy-to-clipboard);
    (d) Sync SA кнопка (admin only) → HTMX POST `/admin/partials/sa-sync`
    → `IDPService.SyncServiceAccounts()` → показать результат.
  - **Creates**:
    - `internal/ui/pages/partials/sa_edit.templ`
    - `internal/ui/pages/partials/sa_secret.templ`
    - Изменения в `internal/ui/handlers/access.go`
  - **Links**: N/A

### Критерии завершения Phase 7

- [ ] Все подпункты завершены (7.1–7.4)
- [ ] Табы переключаются без перезагрузки страницы (Alpine.js)
- [ ] Таблица пользователей отображает данные из Keycloak
- [ ] Фильтрация и поиск пользователей работают
- [ ] Role override: повышение и удаление работает (admin only)
- [ ] Таблица SA отображает все Service Accounts
- [ ] Создание SA: форма → secret показан один раз → copy works
- [ ] Редактирование SA сохраняет изменения
- [ ] Удаление SA с подтверждением
- [ ] Ротация secret: предупреждение → новый secret → copy
- [ ] Sync SA: кнопка → результат синхронизации
- [ ] RBAC: readonly — только просмотр; admin — все действия
- [ ] Контейнер собирается и запускается

---

## Phase 8: Мониторинг, SSE и настройки

**Dependencies**: Phase 4, Phase 5
**Status**: Pending

### Описание

Страница мониторинга: здоровье зависимостей (real-time), состояние фоновых задач,
алерты. SSE endpoints для live-обновлений на Dashboard и SE pages. Prometheus-клиент
(опциональный) для исторических графиков latency. Страница настроек (Prometheus
конфигурация, сохранение в БД).

### Подпункты

- [ ] **8.1 SSE endpoints**
  - **Dependencies**: None
  - **Description**: Создать `internal/ui/handlers/events.go` — SSE handlers:
    (a) `GET /admin/events/system-status` — периодическая отправка (каждые 15с):
    статусы PostgreSQL, Keycloak (из DepHealthService), статусы всех SE
    (из topologymetrics), агрегированные метрики (count by status);
    (b) Формат SSE events: `event: dep-status\ndata: {json}\n\n`,
    `event: se-status\ndata: {json}\n\n`;
    (c) Graceful disconnect при закрытии клиента (context cancel).
    Обработка множественных клиентов (каждый SSE — отдельная goroutine).
  - **Creates**:
    - `internal/ui/handlers/events.go`
    - Изменения в `internal/server/server.go` (регистрация SSE роутов)
  - **Links**:
    - [HTMX SSE Extension](https://htmx.org/extensions/sse/)

- [ ] **8.2 Monitoring page — здоровье и задачи**
  - **Dependencies**: 8.1
  - **Description**: Создать `internal/ui/pages/monitoring.templ`:
    (a) Карточки зависимостей: PostgreSQL, Keycloak — статус (badge),
    latency (мс). Live-обновление через HTMX SSE `sse-swap`;
    (b) Карточки SE: по каждому SE — имя, status, mode, latency.
    Live-обновление через SSE;
    (c) Фоновые задачи: File Sync (last run, result, interval),
    SA Sync (last run, result, interval), Dep Health Check (interval).
    Данные из сервисов (StorageSyncService, SASyncService);
    (d) Алерты: список проблем (SE offline/degraded, зависимость
    недоступна, SE заполнен >80%). Данные агрегируются в handler.
    Создать `internal/ui/handlers/monitoring.go`.
  - **Creates**:
    - `internal/ui/pages/monitoring.templ`
    - `internal/ui/handlers/monitoring.go`
  - **Links**: N/A

- [ ] **8.3 Prometheus-клиент (опциональный)**
  - **Dependencies**: None
  - **Description**: Создать `internal/ui/prometheus/client.go`:
    (a) HTTP-клиент для Prometheus Query API (`/api/v1/query`,
    `/api/v1/query_range`);
    (b) Методы: `QueryLatency(target string, period time.Duration)` —
    запрос `app_dependency_latency_seconds{target=...}[period]`;
    (c) Методы: `QueryStorageUsage(period)` — запрос storage metrics;
    (d) `IsAvailable()` — проверка доступности Prometheus;
    (e) Конфигурация загружается из `UISettingsService` (Prometheus URL,
    enabled, timeout). Если не настроен — все методы возвращают empty result.
  - **Creates**:
    - `internal/ui/prometheus/client.go`
  - **Links**:
    - [Prometheus HTTP API](https://prometheus.io/docs/prometheus/latest/querying/api/)

- [ ] **8.4 Графики latency (с Prometheus)**
  - **Dependencies**: 8.2, 8.3
  - **Description**: Дополнить monitoring page:
    (a) Если Prometheus настроен: графики latency к каждому SE (line chart,
    ApexCharts), latency PostgreSQL и Keycloak (line chart),
    использование хранилища по SE (stacked bar chart);
    (b) Если Prometheus не настроен: блок «Prometheus не настроен,
    перейдите в Настройки» (ссылка на /admin/settings);
    (c) Selector периода: 1h, 6h, 24h, 7d. HTMX partial для обновления
    графиков при смене периода;
    (d) Данные для графиков загружаются через HTMX partial
    `GET /admin/partials/monitoring-charts?period=24h` → JSON в
    data-атрибуте → ApexCharts init через Alpine.js.
  - **Creates**:
    - `internal/ui/pages/partials/monitoring_charts.templ`
    - Изменения в `internal/ui/pages/monitoring.templ`
    - Изменения в `internal/ui/handlers/monitoring.go`
  - **Links**: N/A

- [ ] **8.5 Страница настроек**
  - **Dependencies**: 8.3
  - **Description**: Создать `internal/ui/pages/settings.templ` —
    страница `/admin/settings` (admin only):
    (a) Секция «Prometheus»: форма (URL, enabled toggle, timeout,
    retention period). HTMX PUT `/admin/partials/settings-prometheus`;
    (b) Кнопка «Проверить подключение» → HTMX POST
    `/admin/partials/settings-prometheus-test` →
    `PrometheusClient.IsAvailable()` → показать результат (success/error);
    (c) Данные загружаются из `UISettingsService`, сохраняются
    обратно в `ui_settings`.
    Создать `internal/ui/handlers/settings.go`.
  - **Creates**:
    - `internal/ui/pages/settings.templ`
    - `internal/ui/pages/partials/settings_prometheus.templ`
    - `internal/ui/handlers/settings.go`
  - **Links**: N/A

- [ ] **8.6 SSE-интеграция на Dashboard и SE pages**
  - **Dependencies**: 8.1
  - **Description**: Дополнить существующие страницы SSE:
    (a) Dashboard: блок статуса зависимостей обновляется через SSE
    (`hx-ext="sse"`, `sse-connect`, `sse-swap`);
    (b) SE list: статусы SE обновляются через SSE (blink/update
    badge при изменении status). Добавить `hx-ext="sse"` на
    контейнер таблицы или отдельные ячейки status/latency.
  - **Creates**:
    - Изменения в `internal/ui/pages/dashboard.templ`
    - Изменения в `internal/ui/pages/se_list.templ`
  - **Links**: N/A

### Критерии завершения Phase 8

- [ ] Все подпункты завершены (8.1–8.6)
- [ ] SSE endpoint `/admin/events/system-status` отправляет статусы каждые 15с
- [ ] Monitoring page показывает здоровье всех зависимостей (real-time)
- [ ] Фоновые задачи: последний запуск, результат, интервал
- [ ] Алерты отображаются при проблемах (SE offline, dependency down)
- [ ] Prometheus: если настроен — графики latency отображаются
- [ ] Prometheus: если не настроен — placeholder с ссылкой на настройки
- [ ] Settings: конфигурация Prometheus сохраняется в БД
- [ ] Settings: «Проверить подключение» работает
- [ ] Dashboard: статусы зависимостей обновляются через SSE
- [ ] SE list: статусы SE обновляются через SSE
- [ ] Контейнер собирается и запускается

---

## Phase 9: Сборка, деплой и тестирование

**Dependencies**: Phase 4, Phase 5, Phase 6, Phase 7, Phase 8
**Status**: Pending

### Описание

Обновление Dockerfile (templ generate + tailwind compile в build stage), Helm chart
(новые env-переменные, HTTPRoute для `/admin`), docker-compose.yaml. Обновление
тестового окружения. Интеграционное тестирование UI. Обновление документации.

### Подпункты

- [ ] **9.1 Обновление Dockerfile**
  - **Dependencies**: None
  - **Description**: Обновить Dockerfile для Admin Module:
    (a) Добавить stage для templ generate: установить templ CLI,
    запустить `templ generate`;
    (b) Добавить stage для Tailwind: скачать Tailwind Standalone CLI,
    запустить `tailwindcss -i input.css -o output.css --minify`;
    (c) Убедиться что `_templ.go` файлы и `output.css` попадают
    в build context для `go build`;
    (d) Финальный образ не меняется (alpine + binary).
  - **Creates**:
    - Изменения в `Dockerfile`
  - **Links**: N/A

- [ ] **9.2 Обновление Helm chart**
  - **Dependencies**: None
  - **Description**: Обновить `charts/admin-module/`:
    (a) Добавить env-переменные в ConfigMap/Secret:
    `AM_UI_ENABLED`, `AM_UI_SESSION_SECRET`, `AM_UI_OIDC_CLIENT_ID`;
    (b) Обновить values.yaml: секция `ui` (enabled, sessionSecret,
    oidcClientId);
    (c) Обновить HTTPRoute: добавить pathPrefix `/admin` и `/static`
    (помимо существующих `/api/v1`, `/health`);
    (d) Обновить NOTES.txt (информация о доступе к UI).
  - **Creates**:
    - Изменения в `charts/admin-module/values.yaml`
    - Изменения в `charts/admin-module/templates/configmap.yaml`
    - Изменения в `charts/admin-module/templates/secret.yaml`
    - Изменения в `charts/admin-module/templates/httproute.yaml`
  - **Links**: N/A

- [ ] **9.3 Обновление docker-compose.yaml и тестового окружения**
  - **Dependencies**: 9.1
  - **Description**: Обновить docker-compose.yaml:
    (a) Добавить env-переменные UI в admin-module service;
    (b) Добавить port mapping для UI (если отличается от 8000).
    Обновить тестовое окружение (tests/helm/):
    (c) Добавить `AM_UI_ENABLED=true` в artstore-apps values;
    (d) Обновить Keycloak realm: клиент `artstore-admin-ui`;
    (e) Обновить HTTPRoute в тестовом окружении.
  - **Creates**:
    - Изменения в `docker-compose.yaml`
    - Изменения в `tests/helm/artstore-apps/`
  - **Links**: N/A

- [ ] **9.4 Интеграционное тестирование**
  - **Dependencies**: 9.1, 9.2, 9.3
  - **Description**: Собрать Docker-образ с UI. Развернуть в тестовом
    кластере Kubernetes. Проверить:
    (a) Доступ к `/admin` → redirect на Keycloak login;
    (b) Аутентификация → redirect обратно → Dashboard отображается;
    (c) Навигация по всем страницам (sidebar);
    (d) SE: discover, register, edit, sync, delete;
    (e) Files: список, фильтры, detail modal;
    (f) Access: users list, role override, SA CRUD, rotate secret;
    (g) Monitoring: статусы, SSE обновления;
    (h) Settings: Prometheus конфигурация;
    (i) RBAC: readonly user видит данные, но не может изменять.
    Создать интеграционный тест-скрипт `tests/scripts/test-am-ui.sh`.
  - **Creates**:
    - `tests/scripts/test-am-ui.sh`
    - Docker image (сборка)
  - **Links**: N/A

- [ ] **9.5 Документация**
  - **Dependencies**: 9.4
  - **Description**: Обновить документацию:
    (a) Обновить `src/admin-module/README.md` — секция Admin UI
    (стек, сборка, конфигурация, структура);
    (b) Обновить `docs/briefs/admin-ui-requirements.md` — статус → Released v1.0;
    (c) Обновить `CLAUDE.md` — отметить Admin UI как ✅ готов;
    (d) Обновить `tests/Makefile` — добавить цель `test-am-ui`;
    (e) Обновить `Makefile` в admin-module — документировать новые цели.
  - **Creates**:
    - Изменения в `src/admin-module/README.md`
    - Изменения в `docs/briefs/admin-ui-requirements.md`
    - Изменения в `CLAUDE.md`
    - Изменения в `tests/Makefile`
  - **Links**: N/A

### Критерии завершения Phase 9

- [ ] Все подпункты завершены (9.1–9.5)
- [ ] **Docker-образ успешно собран** с UI (templ + tailwind + embed)
- [ ] **Все страницы UI работают** в тестовом кластере Kubernetes
- [ ] **Аутентификация через Keycloak** работает end-to-end
- [ ] **RBAC** работает: admin vs readonly
- [ ] **SSE** обновления работают в real-time
- [ ] **Интеграционные тесты пройдены**
- [ ] Документация обновлена
- [ ] Образ опубликован в Harbor registry

---

## Примечания

- **Phases 4, 5, 6, 7 могут выполняться параллельно** — все зависят только от
  Phase 3 (базовый layout и компоненты). Каждая фаза создаёт отдельную страницу.
- **Phase 8** зависит от Phase 4 и 5 (SSE интеграция на Dashboard и SE pages).
- Каждая фаза — один контекст AI. Подпункты внутри фазы выполняются последовательно.
- При разработке использовать `docker-compose up` для локального тестирования.
- Тег Docker-образа при разработке: `v0.2.0-N` (новый minor для UI feature).
- Keycloak-клиент `artstore-admin-ui` нужно создать вручную в production Keycloak
  (в тестовом — через init-job).

---

**План готов к использованию.**
