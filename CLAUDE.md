# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Обзор проекта

Artstore — переработка проекта ArtStore (распределённое файловое хранилище с микросервисной архитектурой). Старый код на Python/FastAPI находится в `old_artstore/` как справочный материал. Новая разработка ведётся на **Go**, каждый модуль — отдельный независимый Go-проект в директории `src/`.

### Цель переработки (из task.md)

- Разделить модули на отдельные независимые проекты
- Провести ревизию API каждого модуля, унифицировать контракты
- Перевести на Go
- Начать разработку каждого модуля с чистого листа, опираясь на API-контракт и документацию

### Модули системы

| Модуль | Порты | Назначение | Статус |
|--------|-------|------------|--------|
| Storage Element | 8010-8019 | Физическое хранение файлов, WAL, attr.json, репликация | ✅ Готов (Phase 6) |
| Admin Module | 8000-8009 | Keycloak IdP, RBAC, реестр SE и файлов, Service Accounts, Admin UI | ✅ API + UI + i18n готовы |
| Ingester Module | 8020-8029 | Streaming upload, валидация, выбор SE, регистрация файлов | ⏳ Не начат |
| Query Module | 8030-8039 | Поиск (PostgreSQL FTS), LRU cache, proxy download | ⏳ Не начат |

Admin UI встроен в Admin Module. Стек: Templ + HTMX + Alpine.js + Tailwind CSS + ApexCharts. Keycloak-клиент `artstore-admin-ui` (Authorization Code + PKCE) используется для аутентификации администраторов через браузер.

## Технический стек

- **Язык**: Go 1.25+
- **HTTP**: `net/http` + `chi` router
- **OpenAPI**: oapi-codegen (v3.0.3), contract-first
- **БД**: PostgreSQL 17, `pgx/v5` (без ORM), `golang-migrate` (embedded migrations)
- **Аутентификация**: JWT RS256 через Keycloak (OAuth 2.0 / OIDC)
- **Метрики**: Prometheus (`prometheus/client_golang`)
- **Логирование**: `slog` (stdlib) + JSON
- **Сборка**: Docker multi-stage (`golang:1.25-alpine` → `alpine:3.19`), Helm 3
- **Тесты**: Unit-тесты (`testcontainers-go`), интеграционные (bash + curl)

## Общие правила

- Общаться на русском языке
- Комментарии и документацию вести на русском языке
- Подробные комментарии в коде
- Если не знаешь ответа — останови выполнение, спроси у пользователя
- Все md файлы проверять linter
- Разработку, отладку и тестирование вести **только через Docker или Kubernetes**
- Доменные имена для тестов/разработки добавлять в файл hosts (просить пользователя о ручном добавлении)
- **Запрет hardcoded параметров соединений**: все параметры сетевых соединений (таймауты, TLS настройки, интервалы проверок, размеры пулов) **обязательно** выносить в конфигурацию через env-переменные с разумными defaults. Запрещено: `InsecureSkipVerify: true`, литеральные таймауты (`30 * time.Second`) в HTTP-клиентах, hardcoded health-check пути. Каждый новый HTTP-клиент или сетевое соединение должно использовать параметры из `Config` struct.

## Инструменты

- `kubectl` — настроен на работу с тестовым кластером Kubernetes
- `helm` — для работы с helm charts
- `docker` — работа с контейнерами

## Хранилище контейнеров

Harbor: https://harbor.kryukov.lan — admin/password. Использовать публичный проект `library`.

## Тестовый кластер Kubernetes

- Gateway API (Envoy Gateway, gatewayClassName: `eg`)
- MetalLB (LoadBalancer, IP pool: 192.168.218.180-190)
- Ingress controller отсутствует
- cert-manager (ClusterIssuer: `dev-ca-issuer`)
- Домены для отладки: `test1.kryukov.lan`, `test2.kryukov.lan`, `test3.kryukov.lan` → 192.168.218.180
- Домен проекта: `artstore.kryukov.lan` → 192.168.218.180 (внешний доступ к API Gateway)
- DNS сервер: 192.168.218.9

## Git Workflow (GitHub Flow + Semver Tags)

**Обязательно** следовать при любых изменениях файлов проекта. Полное описание: `GIT-WORKFLOW.md`.

### Ветки

Основная ветка — `main` (всегда deployable). Feature branches от main:
- `feature/`, `bugfix/`, `docs/`, `refactor/`, `test/`, `hotfix/`

### Commits — Conventional Commits

```
<type>(<scope>): <subject>
```
Типы: `feat`, `fix`, `docs`, `style`, `refactor`, `test`, `chore`

### Workflow

1. `git checkout main && git pull` → `git checkout -b <type>/<description>`
2. Работа в ветке
3. Спросить пользователя: «Создать commit?»
4. После commit — предложить: локальный merge (`--no-ff`) или GitHub PR
5. Удалить ветку после merge

### Версионирование модулей

Все модули на стадии разработки используют версии `0.x.y` (semver):

- `0.Y.Z` — стадия разработки (Y = minor/milestone, Z = patch/bugfix)
- `1.0.0` — первый production release (все модули интегрированы и протестированы)
- Бамп minor-версии (`0.1.0` → `0.2.0`) — **только с разрешения пользователя**
- Бамп patch-версии (`0.1.0` → `0.1.1`) — баг-фиксы, без разрешения

### Теги образов

- При разработке: `v0.Y.Z-N` (суффикс инкрементируется при каждой сборке)
- При релизе: `v0.Y.Z` (суффикс убирается, patch увеличивается)

## Планы разработки

- Если задача требует создания плана — создавать его по шаблону `.plantemplates/DEVELOPMENT_PLAN_TEMPLATE.md`
- Сохранять план в директории `plans/`
- План должен быть подробным, разбит на фазы
- Одна фаза — один контекст AI
- После выполнения каждого пункта плана — отмечать его как завершённый в файле плана
- После выполнения **всех** пунктов плана — переносить план в `plans/archive/`

## Структура директорий

```
.plantemplates/    — Шаблоны для планов разработки
old_artstore/      — Старый проект (Python/FastAPI) как справочный материал
src/               — Исходные коды Go-модулей
  admin-module/    — Admin Module (go.mod: github.com/bigkaa/goartstore/admin-module)
  storage-element/ — Storage Element (go.mod: github.com/bigkaa/goartstore/storage-element)
plans/             — Планы разработки (активные)
  archive/         — Завершённые планы
docs/              — Документация
  api-contracts/   — OpenAPI 3.0.3 спецификации всех модулей
  api-review/      — Анализ API старого проекта
  briefs/          — Краткие описания модулей
  design/          — Технические дизайн-документы
```

### Структура Go-модуля (общий паттерн)

```
src/<module>/
├── cmd/<module>/main.go          — Точка входа
├── internal/
│   ├── api/
│   │   ├── generated/            — oapi-codegen (types.gen.go, server.gen.go)
│   │   ├── handlers/             — HTTP-обработчики
│   │   ├── middleware/           — auth, logging, metrics
│   │   └── errors/               — Типизированные ошибки API
│   ├── config/                   — Конфигурация из env-переменных
│   ├── server/                   — HTTP-сервер, graceful shutdown
│   ├── domain/model/             — Доменные модели
│   ├── service/                  — Бизнес-логика
│   └── ...                       — Модуль-специфичные пакеты
├── charts/<module>/              — Helm chart
├── tests/                        — Интеграционные тесты
├── Dockerfile
├── docker-compose.yaml
├── Makefile
├── oapi-codegen-*.yaml           — Конфиги кодогенерации
├── go.mod
└── go.sum
```

## Тестирование

- Тестировать в кластере Kubernetes
- Unit-тесты: `go test ./...` в директории модуля
- Интеграционные тесты: bash-скрипты в `tests/scripts/` (curl + assertions)
- Тестовое окружение разворачивается через три Helm chart-а в `tests/helm/`

### Тестовая инфраструктура (три Helm chart-а)

```
tests/helm/
├── artstore-infra/     — PG + KC (базовый слой)
├── artstore-se/        — 6 Storage Elements всех типов
├── artstore-apps/      — Admin Module
└── init-job/          — standalone Job (загрузка данных)
```

Все три chart-а деплоятся в один namespace `artstore-test`.

### Makefile targets (из `tests/`)

```
make infra-up / infra-down     — PG + KC
make se-up / se-down           — 6 SE
make apps-up / apps-down       — AM
make test-env-up / test-env-down — всё сразу (последовательно)
make init-data                 — Init Job (загрузка тестовых данных)
make port-forward-start / stop — port-forward ко всем сервисам
make test-am                   — интеграционные тесты AM (~30 тестов)
make test-all                  — все интеграционные тесты
```

### Keycloak тестовые клиенты

| Клиент | Тип | Назначение |
|--------|-----|------------|
| `artstore-test-user` (secret: `test-user-secret`) | Password grant | JWT пользователей (admin/viewer) |
| `artstore-admin-module` (secret: `admin-module-test-secret`) | Client credentials | JWT service account AM |
| `artstore-test-init` (secret: `test-init-secret`) | Client credentials | Инициализация тестовых данных |

## Ключевые документы

| Документ | Путь | Описание |
|----------|------|----------|
| Требования | `docs/REQUIREMENTS.md` | Полные системные требования |
| API-контракты | `docs/api-contracts/*.yaml` | OpenAPI спецификации всех модулей |
| Брифы модулей | `docs/briefs/*.md` | Краткие описания архитектуры модулей |
| Дизайн Admin Module | `docs/design/admin-module-design.md` | Техдизайн: схема БД, компоненты, потоки |
| План разработки | `plans/unified-plan.md` | Объединённый план (Phase 7-8) |
| План SE (архив) | `plans/archive/storage-element-development-plan.md` | Завершённый план SE |

## Справка по старому проекту (old_artstore/)

Ключевые архитектурные концепции из старого проекта, перенесённые или адаптированные:

- **Attribute-First Storage Model**: `*.attr.json` — единственный источник истины для метаданных ✅ (реализовано в SE)
- **WAL Protocol**: Write-Ahead Log для атомарности операций с файлами ✅ (реализовано в SE)
- **Storage Element Modes**: edit → rw → ro → ar (жизненный цикл хранилища) ✅ (реализовано в SE)
- **Leader/Follower репликация**: NFS flock-based leader election ✅ (реализовано в SE, вместо Redis)
- **GC и Reconcile**: Сборка мусора и синхронизация attr.json ↔ файловая система ✅ (реализовано в SE)
- **Service Discovery**: Упрощено — Admin Module как реестр (вместо Redis Pub/Sub)
- **Sequential Fill Algorithm**: Выбор Storage Element по priority и capacity (будет в Ingester)
- **Circuit Breaker**: Для inter-service communications (будет при интеграции модулей)

Подробная документация: `old_artstore/README.md`, `old_artstore/CLAUDE.md`, модульные README.
