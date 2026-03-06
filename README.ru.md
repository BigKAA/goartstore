# Artstore

Распределённое файловое хранилище с микросервисной архитектурой, написанное на Go.

![C4 Context Diagram](docs/guides/images/c4-context.png)

## Обзор

Artstore -- распределённая система хранения файлов, спроектированная для надёжного
хранения, поиска и скачивания файлов через множество узлов хранения.
Система использует микросервисную архитектуру: каждый модуль -- независимый Go-проект
со своим API-контрактом, миграциями БД, Helm chart-ом и CI-пайплайном.

**Основные возможности:**

- Распределённое хранение файлов на нескольких Storage Elements
- Поиск файлов по атрибутам через PostgreSQL Full-Text Search
- Write-Ahead Log (WAL) для атомарности файловых операций
- Leader/Follower репликация с NFS-based leader election
- Жизненный цикл Storage Element (edit -> rw -> ro -> ar)
- Ролевой доступ (RBAC) через Keycloak (OAuth 2.0 / OIDC)
- Встроенный Admin UI для управления системой
- Метрики Prometheus и дашборды Grafana
- Kubernetes-native деплой с Helm charts

## Архитектура

![C4 Container Diagram](docs/guides/images/c4-container.png)

### Модули

| Модуль | Порт | Назначение | Версия |
|--------|------|------------|--------|
| [Admin Module](src/admin-module/) | 8000 | Keycloak IdP, RBAC, реестр SE и файлов, Service Accounts, Admin UI | 0.1.0 |
| [Storage Element](src/storage-element/) | 8010 | Физическое хранение файлов, WAL, attr.json, репликация | 0.1.0 |
| [Ingester Module](src/ingester-module/) | 8020 | Sync upload, Sequential Fill выбор SE, регистрация файлов | 0.1.0 |
| [Query Module](src/query-module/) | 8030 | Поиск (PostgreSQL FTS), LRU cache, proxy download | 0.1.0 |
| [Demo Client](src/demo-client/) | 8080 | Web UI демо-клиент для загрузки/поиска/скачивания | 0.1.0 |

### Технический стек

- **Язык**: Go 1.25+
- **HTTP**: net/http + chi router
- **API**: OpenAPI 3.0.3, contract-first (oapi-codegen)
- **БД**: PostgreSQL 17, pgx/v5 (без ORM), golang-migrate
- **Аутентификация**: JWT RS256 через Keycloak (OAuth 2.0 / OIDC)
- **Метрики**: Prometheus (client_golang)
- **Логирование**: slog (stdlib) + JSON
- **Сборка**: Docker multi-stage (golang:1.25-alpine -> alpine:3.19)
- **Деплой**: Helm 3, Kubernetes (Gateway API)
- **Admin UI**: Templ + HTMX + Alpine.js + Tailwind CSS + ApexCharts

## Потоки данных

### Загрузка файлов

![Data Flow: Upload](docs/guides/images/dataflow-upload.png)

1. Клиент отправляет файл в **Ingester Module** через API Gateway
2. Ingester выбирает оптимальный Storage Element (алгоритм Sequential Fill)
3. Файл загружается напрямую в выбранный SE
4. Ingester регистрирует метаданные файла в **Admin Module**

### Поиск и скачивание

![Data Flow: Search](docs/guides/images/dataflow-search.png)

1. Клиент выполняет поиск через **Query Module** (PostgreSQL FTS)
2. QM возвращает метаданные из локального реестра с LRU-кэшированием
3. Клиент запрашивает скачивание -- QM проксирует поток из SE

![Data Flow: Download](docs/guides/images/dataflow-download.png)

## Развёртывание

![Kubernetes Deployment](docs/guides/images/deploy-k8s.png)

### Предварительные требования

- Kubernetes кластер с Gateway API (Envoy Gateway)
- cert-manager с ClusterIssuer
- PostgreSQL 17
- Keycloak 26+

### Быстрый старт с Helm

```bash
# Развернуть инфраструктуру (PostgreSQL + Keycloak)
cd tests && make infra-up

# Развернуть Storage Elements
make se-up

# Развернуть модули приложения (AM + IM + QM)
make apps-up

# Инициализировать тестовые данные
make init-data
```

### Helm Charts

| Chart | Расположение | Описание |
|-------|-------------|----------|
| `artstore` | `charts/artstore/` | Umbrella chart (все модули) |
| `admin-module` | `src/admin-module/charts/` | Admin Module |
| `storage-element` | `src/storage-element/charts/` | Storage Element |
| `ingester-module` | `src/ingester-module/charts/` | Ingester Module |
| `query-module` | `src/query-module/charts/` | Query Module |
| `demo-client` | `src/demo-client/charts/` | Demo Client |

## Структура проекта

```
src/                     -- Исходные коды Go-модулей
  admin-module/          -- Admin Module
  storage-element/       -- Storage Element
  ingester-module/       -- Ingester Module
  query-module/          -- Query Module
  demo-client/           -- Demo Client
charts/artstore/         -- Umbrella Helm chart
docs/
  api-contracts/         -- OpenAPI 3.0.3 спецификации
  design/                -- Архитектурные диаграммы (drawio)
  guides/                -- Руководства: Admin, Developer, Operations (EN + RU)
tests/                   -- Тестовое окружение (Helm + скрипты)
deploy/                  -- Конфигурация realm Keycloak
```

### Структура модуля (общий паттерн)

```
src/<module>/
+-- cmd/<module>/main.go          -- Точка входа
+-- internal/
|   +-- api/
|   |   +-- generated/            -- oapi-codegen (types.gen.go, server.gen.go)
|   |   +-- handlers/             -- HTTP-обработчики
|   |   +-- middleware/            -- auth, logging, metrics
|   |   +-- errors/               -- Типизированные ошибки API
|   +-- config/                   -- Конфигурация из env-переменных
|   +-- server/                   -- HTTP-сервер, graceful shutdown
|   +-- domain/model/             -- Доменные модели
|   +-- service/                  -- Бизнес-логика
+-- charts/<module>/              -- Helm chart
+-- tests/                        -- Интеграционные тесты
+-- Dockerfile
+-- Makefile
+-- go.mod
```

## Тестирование

```bash
cd tests

# Развернуть полное тестовое окружение
make test-env-up

# Запустить все интеграционные тесты (~60 тестов)
make test-all

# Запустить тесты отдельного модуля
make test-am    # Admin Module (~30 тестов)
make test-im    # Ingester Module (16 тестов)
make test-qm    # Query Module (16 тестов)
```

Unit-тесты каждого модуля:

```bash
cd src/<module>
go test ./...
```

## Документация

| Документ | Язык | Описание |
|----------|------|----------|
| [Admin Guide](docs/guides/admin-guide.md) | EN | Keycloak setup, module configuration |
| [Admin Guide](docs/guides/admin-guide.ru.md) | RU | Настройка Keycloak, конфигурация модулей |
| [Developer Guide](docs/guides/developer-guide.md) | EN | API reference, data flows, architecture |
| [Developer Guide](docs/guides/developer-guide.ru.md) | RU | Справка по API, потоки данных, архитектура |
| [Operations Guide](docs/guides/operations-guide.md) | EN | Deployment, monitoring, Grafana dashboards |
| [Operations Guide](docs/guides/operations-guide.ru.md) | RU | Деплой, мониторинг, дашборды Grafana |
| [API-контракты](docs/api-contracts/) | -- | OpenAPI 3.0.3 спецификации всех модулей |

## Участие в разработке

Проект следует [GitHub Flow](GIT-WORKFLOW.md) с Conventional Commits:

```
<type>(<scope>): <subject>
```

Типы: `feat`, `fix`, `docs`, `style`, `refactor`, `test`, `chore`

Все модули на стадии разработки (`0.x.y`). Версия `1.0.0` -- первый production
release после полного интеграционного тестирования.
