# Changelog

Все значимые изменения проекта Artstore документируются в этом файле.

Формат основан на [Keep a Changelog](https://keepachangelog.com/ru/1.1.0/),
проект придерживается [Semantic Versioning](https://semver.org/lang/ru/).

## Storage Element

### [v0.3.0] — 2026-03-06

Stateless-архитектура, per-file lock, Storage Backend абстракция.

#### Добавлено

- Phase 1-2: Stateless архитектура, per-file lock, lock-aware сервисы
- Phase 3: Lock API, обновление OpenAPI, удаление legacy полей
- Phase 4-5: Sync-сервисы, Helm charts, Docker-образ, интеграционные тесты
- Phase 5.5: Иерархическая структура хранения YYYY/MM/DD/
- Phase 6: Storage Backend абстракция (FileStore, AttrStore, LockStore)
- Docker-compose Quick Start и monitoring subchart

#### Изменено

- Удаление file status (hard delete вместо soft delete)
- Обновление sdk-go v0.8.0 и поддержка DEPHEALTH_ISENTRY
- Вынос параметров соединений в конфигурацию

#### Исправлено

- Исправление тестового Helm chart для v0.3.0-2
- Исправление всех golangci-lint issues (401 -> 0)

### [v0.2.0]

Capacity management, dephealth, topologymetrics.

#### Добавлено

- Phase 1: Config.MaxCapacity и Index.totalActiveSize
- Phase 2: Capacity check в Upload и SystemHandler
- Phase 3-4: Helm, OpenAPI, сборка и верификация
- DEPHEALTH_NAME — корректная метка name в topologymetrics
- Prometheus pod-аннотации для auto-discovery метрик

#### Изменено

- Обновление topologymetrics SDK v0.5.0 -> v0.6.0
- Переименование Go-модулей arturkryukov/artstore -> bigkaa/goartstore

### [v0.1.0]

Первая полная версия Storage Element.

#### Добавлено

- Phase 1: Инфраструктура проекта и скелет сервера
- Phase 2: Ядро хранилища (WAL, attr.json, filestore, index, mode)
- Phase 3: API handlers, JWT middleware, Prometheus метрики
- Phase 4: GC, Reconciliation, topologymetrics
- Phase 5: Replicated mode (Leader/Follower)
- Phase 6: Деплой в K8s, 30/30 интеграционных тестов, Helm charts

---

## Admin Module

### [v0.3.0] — 2026-03-06

Admin UI, виртуальный статус "В архиве", priority для SE.

#### Добавлено

- Admin UI Phase 1-9: полноценный веб-интерфейс (Templ + HTMX + Alpine.js + Tailwind CSS)
  - Dashboard, управление SE, файловый реестр, управление доступом
  - Аутентификация через Keycloak (Authorization Code + PKCE)
  - Мониторинг, SSE, настройки, i18n
- Виртуальный статус "В архиве" для файлов в SE с mode=ar
- Скрытие SE priority для режимов ro/ar и валидация обновления
- Поле priority в Storage Elements
- Интеграция dephealth с SE lifecycle — динамический мониторинг SE
- Кастомная тема Artstore для Keycloak
- Интеграционные тесты AM (~30 тестов)
- Production Helm chart и деплой

#### Изменено

- Удаление file status (hard delete вместо soft delete)
- Обновление sdk-go v0.8.0 и поддержка DEPHEALTH_ISENTRY
- Вынос параметров соединений в конфигурацию
- Переименование Go-модулей arturkryukov/artstore -> bigkaa/goartstore

#### Исправлено

- Исправление редактирования SE и стабилизация topology metrics
- Исправлен JSON-тег в FileListResponse seclient
- Обновление otel/sdk v1.39.0 -> v1.40.0 (CVE-2026-24051)
- Исправление всех golangci-lint issues

### [v0.2.0]

API, бизнес-логика, интеграция с Keycloak.

#### Добавлено

- Phase 1: Инфраструктура проекта и скелет сервера
- Phase 2: БД, доменные модели и RBAC
- Phase 3: Внешние клиенты и JWT middleware
- Phase 4: API handlers (29 endpoints) и полная сборка
- Phase 5: Фоновые задачи (sync SE, sync SA, topologymetrics)
- Production Helm chart, интеграционные тесты

---

## Ingester Module

### [v0.1.0] — 2026-03-06

Первый релиз. Sync upload, Sequential Fill SE selection, регистрация файлов.

#### Добавлено

- Phase 1: Каркас проекта, кодогенерация, stub handlers
- Phase 2: Инфраструктурный слой (конфиг, JWT, middleware)
- Phase 3: Бизнес-логика (upload pipeline, SE selection, file registration)
- Phase 4: Dockerfile, Helm chart, интеграционные тесты, деплой в K8s
- Мониторинг Keycloak JWKS и динамических SE в dephealth
- Удаление файлов из SE в режиме edit

#### Изменено

- Удаление file status (hard delete вместо soft delete)

---

## Query Module

### [v0.1.0] — 2026-03-06

Первый релиз. Поиск (PostgreSQL FTS), LRU cache, proxy download.

#### Добавлено

- Phase 1: Каркас проекта, кодогенерация, stubs, health
- Phase 2: Инфраструктурный слой (БД, конфиг, middleware)
- Phase 3: Бизнес-логика (поиск, метаданные, кэш)
- Phase 4: Proxy download, hard delete, topologymetrics
- Phase 5: Сборка, деплой и интеграционные тесты
- Обработка архивных файлов — 410 Gone для download, se_mode в search
- Мониторинг Keycloak JWKS и динамических SE в dephealth

#### Изменено

- Переименование lazy cleanup -> hard delete
- Удаление file status (hard delete вместо soft delete)

#### Исправлено

- topologymetrics — добавлен httpcheck import, унифицирована группа core-modules
- CSRF upload, search hx-include, Unicode NFC

---

## Demo Client

### [v0.1.0] — 2026-03-06

Первый релиз. Демо-приложение для Artstore (Templ + HTMX + Alpine.js + Tailwind CSS).

#### Добавлено

- Phase 1: Каркас, конфиг, Token Manager, HTTP-сервер
- Phase 2: Gateway Client, Activity Log, Service Layer
- Phase 3: UI Framework (layouts, components, i18n)
- Phase 4: Dashboard, Activity Log SSE, Settings
- Phase 5: Upload (single + batch)
- Phase 6: Search, Download, File Detail
- Phase 7: Docker, Helm, Keycloak client

#### Изменено

- Удаление file status (hard delete вместо soft delete)

#### Исправлено

- Закрытие модального окна после успешного удаления файла
- Перемещение кнопки удаления между close и download в модальном окне
- CSRF upload, search hx-include, Unicode NFC
