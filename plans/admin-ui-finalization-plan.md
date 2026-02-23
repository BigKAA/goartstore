# План разработки: Admin UI — финализация, i18n и деплой

## Метаданные

- **Версия плана**: 1.0.0
- **Дата создания**: 2026-02-23
- **Последнее обновление**: 2026-02-23
- **Статус**: In Progress
- **Предыдущий план**: `plans/admin-ui-development-plan.md` (Phase 1–9 ✅)

---

## История версий

- **v1.0.0** (2026-02-23): Начальная версия плана

---

## Текущий статус

- **Активная фаза**: Phase 2
- **Активный подпункт**: 2.1
- **Последнее обновление**: 2026-02-23
- **Примечание**: План создан на основе незакрытых пунктов admin-ui-development-plan.md
  и новых требований (переименование модуля, i18n, HTTPRoute для тестов).

---

## Контекст

Все 9 фаз разработки Admin UI завершены (код написан). Осталось:

1. **16 незакрытых пунктов** — все связаны с верификацией при деплое
2. **Новые требования**:
   - Переименование Go-модулей: `github.com/arturkryukov/artstore` → `github.com/bigkaa/goartstore`
   - HTTPRoute для тестового окружения (`tests/helm/artstore-apps/`)
   - Интернационализация UI: English + Русский

---

## Оглавление

- [x] [Phase 1: Переименование Go-модулей](#phase-1-переименование-go-модулей)
- [ ] [Phase 2: Интернационализация (i18n)](#phase-2-интернационализация-i18n)
- [ ] [Phase 3: Тестовое окружение и HTTPRoute](#phase-3-тестовое-окружение-и-httproute)
- [ ] [Phase 4: Сборка, деплой и верификация](#phase-4-сборка-деплой-и-верификация)

---

## Phase 1: Переименование Go-модулей

**Dependencies**: None
**Status**: ✅ Done

### Описание

Переименование Go-модулей с `github.com/arturkryukov/artstore` на
`github.com/bigkaa/goartstore` во всех файлах проекта: go.mod, Go-импорты,
OpenAPI-контракты, документация. Затрагивает оба модуля: admin-module и
storage-element.

### Подпункты

- [x] **1.1 Переименование admin-module**
  - **Dependencies**: None
  - **Description**: Обновить `go.mod` в `src/admin-module/`:
    `github.com/arturkryukov/artstore/admin-module` →
    `github.com/bigkaa/goartstore/admin-module`.
    Обновить все Go-импорты во всех `.go` файлах модуля (включая `_templ.go`
    нельзя — они перегенерируются из `.templ`; но `.templ` файлы тоже могут
    содержать импорты). Использовать `sed` или `go mod edit` + массовую замену.
    После замены выполнить `go mod tidy` и убедиться что `go build` проходит.
    Количество файлов с импортами: ~60+ `.go` файлов.
  - **Creates**:
    - Изменения в `src/admin-module/go.mod`
    - Изменения во всех `.go` файлах с импортами
    - Изменения в `.templ` файлах с Go-импортами (если есть)
  - **Links**: N/A

- [x] **1.2 Переименование storage-element**
  - **Dependencies**: None
  - **Description**: Обновить `go.mod` в `src/storage-element/`:
    `github.com/arturkryukov/artstore/storage-element` →
    `github.com/bigkaa/goartstore/storage-element`.
    Обновить все Go-импорты во всех `.go` файлах модуля.
    После замены выполнить `go mod tidy` и убедиться что `go build` проходит.
  - **Creates**:
    - Изменения в `src/storage-element/go.mod`
    - Изменения во всех `.go` файлах с импортами
  - **Links**: N/A

- [x] **1.3 Обновление OpenAPI-контрактов и документации**
  - **Dependencies**: 1.1, 1.2
  - **Description**: Обновить ссылки на GitHub-репозиторий:
    (a) `docs/api-contracts/*.yaml` — поле `contact.url`:
    `https://github.com/arturkryukov/artstore` →
    `https://github.com/bigkaa/goartstore`;
    (b) `CLAUDE.md` — ссылки на go.mod модулей;
    (c) `src/admin-module/README.md` — ссылки на репозиторий;
    (d) `src/storage-element/README.md` — ссылки на репозиторий;
    (e) Любые другие файлы с упоминанием старого пути.
    Использовать `grep -r "arturkryukov/artstore"` для поиска оставшихся
    ссылок.
  - **Creates**:
    - Изменения в `docs/api-contracts/*.yaml`
    - Изменения в `CLAUDE.md`
    - Изменения в README-файлах модулей
  - **Links**: N/A

- [x] **1.4 Перегенерация и проверка сборки**
  - **Dependencies**: 1.1, 1.2, 1.3
  - **Description**: Перегенерировать все автогенерированные файлы:
    (a) `templ generate` в `src/admin-module/` — обновит `_templ.go` файлы
    с новыми импортами;
    (b) `oapi-codegen` — перегенерировать `types.gen.go` и `server.gen.go`
    (если они содержат module path);
    (c) `go build ./...` в обоих модулях — проверка компиляции;
    (d) `go test ./...` в обоих модулях — проверка тестов.
  - **Creates**:
    - Обновлённые `_templ.go` файлы
    - Обновлённые `*.gen.go` файлы (если нужно)
  - **Links**: N/A

### Критерии завершения Phase 1

- [x] Все подпункты завершены (1.1–1.4)
- [x] `go build ./...` проходит в обоих модулях без ошибок
- [x] `go test ./...` проходит в обоих модулях
- [x] `grep -r "arturkryukov/artstore"` не находит ни одного вхождения
- [x] `templ generate` проходит без ошибок

---

## Phase 2: Интернационализация (i18n)

**Dependencies**: Phase 1
**Status**: Pending

### Описание

Добавление поддержки двух языков (English, Русский) в Admin UI. Использование
библиотеки `nicksnyder/go-i18n` для управления переводами. Определение языка:
cookie `lang` (установленный переключателем) → заголовок `Accept-Language` →
default `en`. Переключатель языка в header UI.

Затронутые файлы: 33 `.templ` файла (4 layouts, 10 components, 7 pages,
12 partials), handlers, middleware.

### Подпункты

- [ ] **2.1 Пакет i18n и каталоги переводов**
  - **Dependencies**: None
  - **Description**: Добавить зависимость `github.com/nicksnyder/go-i18n/v2`
    в `go.mod`. Создать пакет `internal/ui/i18n/`:
    (a) `i18n.go` — инициализация `i18n.Bundle`, загрузка JSON-каталогов
    из `embed.FS`, функции `T(ctx, msgID)` и `Tp(ctx, msgID, count, data)`
    (с плюрализацией);
    (b) `middleware.go` — middleware для определения языка из cookie `lang`,
    fallback на `Accept-Language`, default `en`. Помещает `*i18n.Localizer`
    в `context.Context`;
    (c) `locales/en.json` — английский каталог переводов;
    (d) `locales/ru.json` — русский каталог переводов;
    (e) `embed.go` — `//go:embed locales/*.json` для встраивания каталогов.
    Начать с базовых ключей (навигация, общие элементы).
  - **Creates**:
    - `internal/ui/i18n/i18n.go`
    - `internal/ui/i18n/middleware.go`
    - `internal/ui/i18n/embed.go`
    - `internal/ui/i18n/locales/en.json`
    - `internal/ui/i18n/locales/ru.json`
  - **Links**:
    - [go-i18n](https://github.com/nicksnyder/go-i18n)
    - [go-i18n Templ integration](https://github.com/nicksnyder/go-i18n#template-functions)

- [ ] **2.2 Интеграция i18n middleware в роутер**
  - **Dependencies**: 2.1
  - **Description**: Обновить `internal/server/server.go`:
    (a) Добавить i18n middleware в цепочку middleware для `/admin/*` маршрутов
    (после auth middleware, перед handlers);
    (b) Обновить `cmd/admin-module/main.go` — инициализация i18n Bundle
    при старте приложения;
    (c) Добавить endpoint `POST /admin/set-language` — handler для
    переключения языка (устанавливает cookie `lang`, redirect back).
  - **Creates**:
    - Изменения в `internal/server/server.go`
    - Изменения в `cmd/admin-module/main.go`
    - `internal/ui/handlers/language.go`
  - **Links**: N/A

- [ ] **2.3 Переключатель языка в UI**
  - **Dependencies**: 2.2
  - **Description**: Обновить `internal/ui/layouts/header.templ`:
    (a) Добавить переключатель языка (dropdown или toggle EN/RU) рядом
    с кнопкой logout;
    (b) При клике — POST `/admin/set-language?lang=en|ru` через HTMX
    или обычную форму;
    (c) Текущий язык подсвечивается;
    (d) Иконка глобуса или флага.
  - **Creates**:
    - Изменения в `internal/ui/layouts/header.templ`
  - **Links**: N/A

- [ ] **2.4 i18n: Layouts и компоненты**
  - **Dependencies**: 2.1
  - **Description**: Заменить все захардкоженные строки на вызовы `T(ctx, key)`
    в layouts и компонентах:
    (a) **Layouts** (4 файла): `base.templ` (title, meta), `sidebar.templ`
    (пункты навигации: «Панель управления», «Мониторинг», «Storage Elements»,
    «Файлы», «Управление доступом», «Настройки»), `header.templ` (роль, logout),
    `page.templ` (breadcrumbs);
    (b) **Компоненты** (10 файлов): `alert.templ` (типы алертов),
    `badge.templ` (статусы), `button.templ` (тексты кнопок),
    `capacity_bar.templ` (метки), `confirm_dialog.templ` (подтверждение,
    отмена), `data_table.templ` (пустое состояние), `filter_bar.templ`
    (метки фильтров), `modal.templ` (закрыть), `pagination.templ`
    (навигация страниц), `stat_card.templ` (метки).
    Добавить все ключи в `en.json` и `ru.json`.
  - **Creates**:
    - Изменения в 14 `.templ` файлах (layouts + components)
    - Обновлённые `en.json` и `ru.json`
  - **Links**: N/A

- [ ] **2.5 i18n: Страницы (pages)**
  - **Dependencies**: 2.4
  - **Description**: Заменить все захардкоженные строки на вызовы `T(ctx, key)`
    в страницах:
    (a) `dashboard.templ` — заголовки карточек, метки графиков, секция
    зависимостей, таблица SE;
    (b) `monitoring.templ` — статусы, фоновые задачи, алерты, период;
    (c) `se_list.templ` — заголовки колонок, кнопки, фильтры;
    (d) `se_detail.templ` — все метки полей, кнопки действий;
    (e) `file_list.templ` — заголовки, фильтры, сортировка;
    (f) `access.templ` — табы, таблицы пользователей и SA;
    (g) `settings.templ` — форма Prometheus, кнопки.
    Добавить все ключи в `en.json` и `ru.json`.
  - **Creates**:
    - Изменения в 7 `.templ` файлах (pages)
    - Обновлённые `en.json` и `ru.json`
  - **Links**: N/A

- [ ] **2.6 i18n: Partials**
  - **Dependencies**: 2.5
  - **Description**: Заменить захардкоженные строки в partials:
    (a) `file_detail.templ`, `file_table.templ` — метки полей файла;
    (b) `monitoring_charts.templ` — метки графиков;
    (c) `sa_edit.templ`, `sa_secret.templ`, `sa_table.templ` — SA формы;
    (d) `se_discover.templ`, `se_edit.templ`, `se_files.templ`,
    `se_table.templ` — SE формы и таблицы;
    (e) `user_detail.templ`, `users_table.templ` — пользователи.
    Добавить все ключи в `en.json` и `ru.json`.
  - **Creates**:
    - Изменения в 12 `.templ` файлах (partials)
    - Обновлённые `en.json` и `ru.json`
  - **Links**: N/A

- [ ] **2.7 i18n: Handlers (сообщения об ошибках и успехе)**
  - **Dependencies**: 2.1
  - **Description**: Обновить UI handlers для использования i18n в сообщениях:
    (a) Все flash/toast сообщения (success, error) должны использовать
    переводы;
    (b) Сообщения валидации форм;
    (c) Обновить handlers: `storage_elements.go`, `files.go`, `access.go`,
    `settings.go`, `monitoring.go`, `dashboard.go`.
    Сообщения передаются в templ-контекст и отображаются через компонент
    `alert.templ`.
  - **Creates**:
    - Изменения в UI handler файлах
    - Обновлённые `en.json` и `ru.json`
  - **Links**: N/A

- [ ] **2.8 Проверка i18n и перегенерация**
  - **Dependencies**: 2.3, 2.4, 2.5, 2.6, 2.7
  - **Description**: Финальная проверка i18n системы:
    (a) `templ generate` — перегенерация всех `_templ.go` файлов;
    (b) `go build ./...` — проверка компиляции;
    (c) `go test ./...` — проверка тестов;
    (d) Проверка полноты каталогов: все ключи из `.templ` файлов
    присутствуют в обоих JSON-каталогах;
    (e) Проверка: нет необёрнутых русских строк в `.templ` файлах
    (поиск кириллицы вне комментариев).
  - **Creates**:
    - Обновлённые `_templ.go` файлы
  - **Links**: N/A

### Критерии завершения Phase 2

- [ ] Все подпункты завершены (2.1–2.8)
- [ ] Библиотека `go-i18n` интегрирована и работает
- [ ] Каталоги `en.json` и `ru.json` содержат все ключи
- [ ] Переключатель языка в header работает
- [ ] Cookie `lang` сохраняется и используется
- [ ] Fallback на `Accept-Language` работает
- [ ] Все 33 `.templ` файла используют `T(ctx, key)` вместо хардкода
- [ ] `templ generate` и `go build` проходят без ошибок
- [ ] Нет кириллицы вне каталогов переводов (кроме комментариев)

---

## Phase 3: Тестовое окружение и HTTPRoute

**Dependencies**: Phase 2
**Status**: Pending

### Описание

Добавление HTTPRoute для Admin Module в тестовый Helm chart
(`tests/helm/artstore-apps/`). Обновление тестового окружения для поддержки
Admin UI с i18n. Проверка готовности всех компонентов перед деплоем.

### Подпункты

- [ ] **3.1 HTTPRoute в тестовом Helm chart**
  - **Dependencies**: None
  - **Description**: Добавить HTTPRoute ресурс в
    `tests/helm/artstore-apps/templates/admin-module.yaml` (или отдельный файл
    `admin-module-httproute.yaml`):
    (a) HTTPRoute для Admin Module с path prefixes:
    `/api/v1`, `/health`, `/admin`, `/static`;
    (b) Gateway: `eg` (Envoy Gateway), namespace: `envoy-gateway-system`;
    (c) Hostname: `artstore.kryukov.lan`;
    (d) Backend: сервис `admin-module` порт 8000;
    (e) Условное включение через values (`httproute.enabled`).
    Обновить `tests/helm/artstore-apps/values.yaml` — добавить секцию
    `httproute` для admin-module.
  - **Creates**:
    - `tests/helm/artstore-apps/templates/admin-module-httproute.yaml`
      (или изменения в `admin-module.yaml`)
    - Изменения в `tests/helm/artstore-apps/values.yaml`
  - **Links**: N/A

- [ ] **3.2 Обновление тестовых values для i18n**
  - **Dependencies**: None
  - **Description**: Проверить и обновить тестовое окружение:
    (a) Убедиться что `AM_UI_ENABLED=true` в values;
    (b) Убедиться что Keycloak-клиент `artstore-admin-ui` создаётся
    в init-job (redirect URIs включают `https://artstore.kryukov.lan/admin/callback`);
    (c) Проверить что тестовые пользователи имеют разные роли
    (admin и readonly) для проверки RBAC.
  - **Creates**:
    - Изменения в `tests/helm/artstore-apps/values.yaml` (при необходимости)
  - **Links**: N/A

- [ ] **3.3 Обновление Dockerfile и Makefile**
  - **Dependencies**: None
  - **Description**: Проверить и обновить Dockerfile:
    (a) Убедиться что JSON-каталоги i18n (`locales/*.json`) включены
    в `embed.FS` и попадают в финальный образ;
    (b) Обновить Makefile: добавить target `i18n-check` для проверки
    полноты каталогов переводов;
    (c) Обновить `make ui-build` если необходимо.
  - **Creates**:
    - Изменения в `src/admin-module/Dockerfile` (при необходимости)
    - Изменения в `src/admin-module/Makefile`
  - **Links**: N/A

### Критерии завершения Phase 3

- [ ] Все подпункты завершены (3.1–3.3)
- [ ] HTTPRoute для Admin Module добавлен в тестовый chart
- [ ] `helm template` для artstore-apps генерирует корректный HTTPRoute
- [ ] Тестовые values содержат все необходимые UI-конфигурации
- [ ] Dockerfile корректно включает i18n каталоги

---

## Phase 4: Сборка, деплой и верификация

**Dependencies**: Phase 3
**Status**: Pending

### Описание

Финальная сборка Docker-образа Admin Module с UI и i18n. Деплой в тестовый
кластер Kubernetes. Верификация всех функций: аутентификация, RBAC, все
страницы UI, SSE, i18n переключение. Запуск интеграционных тестов.
Публикация образа в Harbor.

Эта фаза закрывает все 16 незавершённых пунктов из предыдущего плана.

### Подпункты

- [ ] **4.1 Сборка Docker-образа**
  - **Dependencies**: None
  - **Description**: Собрать Docker-образ Admin Module:
    (a) `docker build -t harbor.kryukov.lan/library/admin-module:v0.2.0-1 .`
    в директории `src/admin-module/`;
    (b) Убедиться что все стадии сборки проходят: templ generate,
    tailwind compile, go build;
    (c) Проверить размер образа;
    (d) Запустить `docker run` и проверить `/health/live`.
    **Закрывает**: «Docker-образ успешно собран с UI» (Phase 9).
  - **Creates**:
    - Docker image `harbor.kryukov.lan/library/admin-module:v0.2.0-1`
  - **Links**: N/A

- [ ] **4.2 Деплой тестового окружения**
  - **Dependencies**: 4.1
  - **Description**: Развернуть полное тестовое окружение:
    (a) `make test-env-up` (infra + SE + apps) в namespace `artstore-test`;
    (b) Дождаться готовности всех подов;
    (c) Проверить что Admin Module под запущен и healthy;
    (d) Проверить что HTTPRoute создан и принимает трафик;
    (e) `make init-data` — загрузка тестовых данных.
    **Закрывает**: «Контейнер собирается и запускается» (Phases 1–8).
  - **Creates**:
    - Тестовое окружение в K8s
  - **Links**: N/A

- [ ] **4.3 Верификация миграции БД**
  - **Dependencies**: 4.2
  - **Description**: Проверить применение миграции `003_ui_settings`:
    (a) Подключиться к PostgreSQL в тестовом кластере;
    (b) Проверить наличие таблицы `ui_settings`;
    (c) Проверить структуру: `key TEXT PK, value TEXT, updated_at, updated_by`;
    (d) Проверить что `schema_migrations` содержит version 3.
    **Закрывает**: «Миграция 003_ui_settings применяется без ошибок» (Phase 1).
  - **Creates**: N/A
  - **Links**: N/A

- [ ] **4.4 Верификация аутентификации**
  - **Dependencies**: 4.2
  - **Description**: Проверить OAuth 2.0 + PKCE flow:
    (a) Переход на `https://artstore.kryukov.lan/admin` → redirect на
    Keycloak login;
    (b) Аутентификация пользователем admin → redirect обратно на `/admin`
    с cookie;
    (c) Повторный запрос `/admin` → доступ без redirect;
    (d) Logout → cookie удалён, redirect на Keycloak logout;
    (e) Expired token → автоматический refresh.
    **Закрывает**: «Аутентификация через Keycloak работает e2e» (Phase 9).
  - **Creates**: N/A
  - **Links**: N/A

- [ ] **4.5 Верификация RBAC**
  - **Dependencies**: 4.4
  - **Description**: Проверить разграничение доступа:
    (a) Login как admin → все кнопки действий видны (create, edit, delete,
    sync, role override, create SA, settings);
    (b) Login как readonly (viewer) → данные видны, кнопки действий скрыты;
    (c) Readonly не может выполнить действия через прямые HTTP-запросы
    (POST/PUT/DELETE);
    (d) Страница «Настройки» доступна только admin.
    **Закрывает**: «RBAC работает: admin vs readonly» (Phase 9).
  - **Creates**: N/A
  - **Links**: N/A

- [ ] **4.6 Верификация всех страниц UI**
  - **Dependencies**: 4.4
  - **Description**: Проверить работу каждой страницы:
    (a) Dashboard: карточки метрик, статусы зависимостей, список SE, графики;
    (b) Storage Elements: таблица, фильтры, discover, register, edit, sync,
    delete, детальная страница;
    (c) Files: таблица, пагинация, фильтры, поиск, сортировка, detail modal,
    edit, delete;
    (d) Access Management: табы Users/SA, фильтры, role override, SA CRUD,
    rotate secret;
    (e) Monitoring: статусы зависимостей, фоновые задачи, алерты;
    (f) Settings: Prometheus конфигурация, test connection.
    **Закрывает**: «Все страницы UI работают в K8s» (Phase 9).
  - **Creates**: N/A
  - **Links**: N/A

- [ ] **4.7 Верификация SSE**
  - **Dependencies**: 4.4
  - **Description**: Проверить SSE real-time обновления:
    (a) Открыть Dashboard → статусы зависимостей обновляются автоматически;
    (b) Открыть Monitoring → статусы обновляются каждые 15 секунд;
    (c) Проверить DevTools → Network → EventSource подключён;
    (d) Остановить/запустить SE → статус обновляется на Dashboard/Monitoring.
    **Закрывает**: «SSE обновления работают в real-time» (Phase 9).
  - **Creates**: N/A
  - **Links**: N/A

- [ ] **4.8 Верификация i18n**
  - **Dependencies**: 4.4
  - **Description**: Проверить интернационализацию:
    (a) По умолчанию UI на английском (Accept-Language: en);
    (b) Переключить на русский через UI → все тексты на русском;
    (c) Обновить страницу → язык сохранён (cookie);
    (d) Проверить все страницы на обоих языках — нет untranslated строк;
    (e) Проверить плюрализацию (если используется).
  - **Creates**: N/A
  - **Links**: N/A

- [ ] **4.9 Интеграционные тесты**
  - **Dependencies**: 4.3, 4.4, 4.5, 4.6, 4.7, 4.8
  - **Description**: Запустить интеграционные тесты:
    (a) `make test-am` — существующие API-тесты;
    (b) `make test-am-ui` — UI-тесты (если скрипт создан в Phase 9);
    (c) Все тесты должны пройти.
    **Закрывает**: «Интеграционные тесты пройдены» (Phase 9).
  - **Creates**:
    - Обновления в `tests/scripts/test-am-ui.sh` (при необходимости)
  - **Links**: N/A

- [ ] **4.10 Публикация образа в Harbor**
  - **Dependencies**: 4.9
  - **Description**: Опубликовать проверенный образ:
    (a) `docker push harbor.kryukov.lan/library/admin-module:v0.2.0-1`;
    (b) Проверить доступность образа в Harbor UI;
    (c) Обновить тег образа в production values (если применимо).
    **Закрывает**: «Образ опубликован в Harbor registry» (Phase 9).
  - **Creates**:
    - Образ в Harbor registry
  - **Links**: N/A

- [ ] **4.11 Обновление документации и архивация плана**
  - **Dependencies**: 4.10
  - **Description**: Финальные обновления:
    (a) Обновить `CLAUDE.md` — отметить новый module path;
    (b) Обновить `docs/briefs/admin-ui-requirements.md` — статус i18n;
    (c) Отметить все пункты в `plans/admin-ui-development-plan.md` как
    выполненные;
    (d) Перенести `plans/admin-ui-development-plan.md` в `plans/archive/`;
    (e) Перенести данный план в `plans/archive/` после завершения.
  - **Creates**:
    - Изменения в `CLAUDE.md`
    - Изменения в `docs/briefs/admin-ui-requirements.md`
    - Перемещение планов в `plans/archive/`
  - **Links**: N/A

### Критерии завершения Phase 4

- [ ] Все подпункты завершены (4.1–4.11)
- [ ] Docker-образ успешно собран с UI, i18n (templ + tailwind + embed)
- [ ] Все страницы UI работают в тестовом кластере Kubernetes
- [ ] Аутентификация через Keycloak работает end-to-end
- [ ] RBAC работает: admin vs readonly
- [ ] SSE обновления работают в real-time
- [ ] i18n: переключение en/ru работает на всех страницах
- [ ] Интеграционные тесты пройдены
- [ ] Образ опубликован в Harbor registry
- [ ] Документация обновлена
- [ ] Оба плана перенесены в archive

---

## Примечания

- **Phase 1** можно выполнить за одну сессию — это массовая замена строк.
- **Phase 2** — самая объёмная (33 templ файла + handlers). Рекомендуется
  разбить на 3–4 сессии AI (layouts+components, pages, partials, handlers).
- **Phase 3** — небольшая, можно объединить с концом Phase 2.
- **Phase 4** — интерактивная (деплой + ручная проверка в браузере).
- Версия Docker-образа: `v0.2.0-N` (minor bump для i18n feature).
- Библиотека `go-i18n`: использовать v2 (`github.com/nicksnyder/go-i18n/v2`).
- Определение языка: cookie `lang` → `Accept-Language` → default `en`.
- Новый module path: `github.com/bigkaa/goartstore/{admin-module,storage-element}`.

---

**План готов к использованию.**
