# План разработки: Документация, развёртывание и мониторинг Artstore

## Метаданные

- **Версия плана**: 1.0.0
- **Дата создания**: 2026-03-03
- **Последнее обновление**: 2026-03-03
- **Статус**: In Progress
- **Требования**: `docs/requirements/documentation-monitoring-requirements.md`

---

## История версий

- **v1.0.0** (2026-03-03): Начальная версия плана

---

## Текущий статус

- **Активная фаза**: Phase 9
- **Активный подпункт**: 9.2, 9.4, 9.5
- **Последнее обновление**: 2026-03-04
- **Примечание**: Phase 1-8 завершены. Phase 9: docker-compose.yaml создан (9.1), monitoring subchart создан (9.3). Остаётся: тестирование docker-compose (9.2), тестирование monitoring subchart (9.4), финальная проверка документации (9.5).

---

## Оглавление

- [x] [Phase 1: Инфраструктура и Pod annotations](#phase-1-инфраструктура-и-pod-annotations)
- [x] [Phase 2: Архитектурные диаграммы (drawio)](#phase-2-архитектурные-диаграммы-drawio)
- [x] [Phase 3: Скриншоты (Playwright MCP)](#phase-3-скриншоты-playwright-mcp)
- [x] [Phase 4: Admin Guide (EN + RU)](#phase-4-admin-guide-en--ru)
- [x] [Phase 5: Developer Guide (EN + RU)](#phase-5-developer-guide-en--ru)
- [x] [Phase 6: Grafana dashboards + AlertManager rules](#phase-6-grafana-dashboards--alertmanager-rules)
- [x] [Phase 7: Operations Guide (EN + RU)](#phase-7-operations-guide-en--ru)
- [x] [Phase 8: Umbrella Helm chart](#phase-8-umbrella-helm-chart)
- [ ] [Phase 9: docker-compose + Monitoring subchart](#phase-9-docker-compose--monitoring-subchart)

---

## Phase 1: Инфраструктура и Pod annotations

**Dependencies**: None
**Status**: ✅ Done

### Описание

Подготовка инфраструктуры: создание структуры директорий для документации, добавление Prometheus Pod annotations во все Helm charts модулей. Быстрый win — минимальные изменения, максимальная отдача для мониторинга.

### Подпункты

- [x] **1.1 Создание структуры директорий для документации**
  - **Dependencies**: None
  - **Description**: Создать директории `docs/guides/`, `docs/guides/images/`. Добавить `.gitkeep` для пустых директорий. Обновить README если нужно.
  - **Creates**:
    - `docs/guides/.gitkeep`
    - `docs/guides/images/.gitkeep`
  - **Links**: N/A

- [x] **1.2 Pod annotations для Prometheus (Admin Module)**
  - **Dependencies**: None
  - **Description**: Добавить `prometheus.io/scrape`, `prometheus.io/port`, `prometheus.io/path` annotations в `src/admin-module/charts/admin-module/templates/deployment.yaml`. Вынести в `values.yaml` как `podAnnotations` с defaults. Порт: `8000`.
  - **Creates**:
    - Изменения в `src/admin-module/charts/admin-module/values.yaml`
    - Изменения в `src/admin-module/charts/admin-module/templates/deployment.yaml`
  - **Links**:
    - [Существующий chart](src/admin-module/charts/admin-module/)

- [x] **1.3 Pod annotations для Prometheus (Storage Element)**
  - **Dependencies**: None
  - **Description**: Аналогично 1.2 для SE. Порт: `8010`. SE может иметь несколько инстансов разных типов, annotations должны быть едиными.
  - **Creates**:
    - Изменения в `src/storage-element/charts/storage-element/values.yaml`
    - Изменения в `src/storage-element/charts/storage-element/templates/deployment.yaml`
  - **Links**:
    - [Существующий chart](src/storage-element/charts/storage-element/)

- [x] **1.4 Pod annotations для Prometheus (Ingester Module)**
  - **Dependencies**: None
  - **Description**: Аналогично 1.2 для IM. Порт: `8020`.
  - **Creates**:
    - Изменения в `src/ingester-module/charts/ingester-module/values.yaml`
    - Изменения в `src/ingester-module/charts/ingester-module/templates/deployment.yaml`
  - **Links**:
    - [Существующий chart](src/ingester-module/charts/ingester-module/)

- [x] **1.5 Pod annotations для Prometheus (Query Module)**
  - **Dependencies**: None
  - **Description**: Аналогично 1.2 для QM. Порт: `8030`.
  - **Creates**:
    - Изменения в `src/query-module/charts/query-module/values.yaml`
    - Изменения в `src/query-module/charts/query-module/templates/deployment.yaml`
  - **Links**:
    - [Существующий chart](src/query-module/charts/query-module/)

- [x] **1.6 Тестирование annotations в K8s**
  - **Dependencies**: 1.2, 1.3, 1.4, 1.5
  - **Description**: Пересобрать charts, задеплоить в тестовый кластер (`make apps-up se-up`), проверить что annotations присутствуют в pods (`kubectl get pods -o jsonpath`). Проверить что `/metrics` эндпоинты отвечают.
  - **Creates**: N/A
  - **Links**: N/A

### ✅ Критерии завершения Phase 1

- [x] Все подпункты завершены (1.1-1.6)
- [x] Все 4 модуля имеют Prometheus annotations в pods
- [x] `kubectl get pods -n artstore-test -o yaml | grep prometheus.io` показывает annotations для всех pods
- [x] `curl <pod-ip>:<port>/metrics` возвращает Prometheus-метрики для каждого модуля

---

## Phase 2: Архитектурные диаграммы (drawio)

**Dependencies**: None (может выполняться параллельно с Phase 1)
**Status**: ✅ Done

### Описание

Создание 13 диаграмм в формате drawio для встраивания в документацию. Типы: C4 (Context, Container, Component), Data Flow, Sequence, Deployment. drawio файлы — XML-формат, создаются программно. PNG-экспорт — через `drawio CLI` или вручную.

### Подпункты

- [x] **2.1 C4 Context Diagram**
  - **Dependencies**: None
  - **Description**: Диаграмма верхнего уровня: Artstore как система, взаимодействие с внешними актёрами (пользователи, администраторы, разработчики-интеграторы) и внешними системами (Keycloak, PostgreSQL). Стиль: C4 стандарт (синие/серые блоки).
  - **Creates**:
    - `docs/design/c4-context.drawio`
  - **Links**:
    - [C4 Model](https://c4model.com/)

- [x] **2.2 C4 Container Diagram**
  - **Dependencies**: 2.1
  - **Description**: Все модули (AM, SE, IM, QM) как контейнеры, их связи (HTTP REST, JWT), базы данных (PostgreSQL), Keycloak. Показать потоки: upload (Client→IM→SE→AM), download (Client→QM→SE), search (Client→QM→PG). Показать SE в K8s и SE как Docker-контейнер (другой ЦОД).
  - **Creates**:
    - `docs/design/c4-container.drawio`
  - **Links**: N/A

- [x] **2.3 C4 Component Diagram (SE)**
  - **Dependencies**: None
  - **Description**: Внутренняя структура Storage Element: WAL, attr.json handler, replication engine, GC/Reconcile, health check, metrics. Для Admin Guide §5.
  - **Creates**:
    - `docs/design/c4-component-se.drawio`
  - **Links**:
    - [SE brief](docs/briefs/storage-element.md)

- [x] **2.4 C4 Component Diagram (AM)**
  - **Dependencies**: None
  - **Description**: Внутренняя структура Admin Module: API handlers, JWT middleware, file registry, SE registry, SA registry, Admin UI (Templ+HTMX), Prometheus client, SSE events, dephealth.
  - **Creates**:
    - `docs/design/c4-component-am.drawio`
  - **Links**:
    - [AM design](docs/design/admin-module-design.md)

- [x] **2.5 Data Flow Diagrams (Upload, Download, Search)**
  - **Dependencies**: None
  - **Description**: Три диаграммы потоков данных. Upload: Client → Gateway → IM (auth, SE selection via AM) → SE (store + WAL) → AM (register file). Download: Client → Gateway → QM (auth, cache check) → AM (file lookup) → SE (stream). Search: Client → Gateway → QM (auth) → PostgreSQL FTS.
  - **Creates**:
    - `docs/design/dataflow-upload.drawio`
    - `docs/design/dataflow-download.drawio`
    - `docs/design/dataflow-search.drawio`
  - **Links**:
    - [Существующие IM диаграммы](docs/design/im-file-upload-sequence.drawio) — как референс

- [x] **2.6 Sequence Diagrams (Upload, Download, Auth, SE Registration)**
  - **Dependencies**: None
  - **Description**: Четыре sequence-диаграммы. File Upload: Client→Gateway→IM→AM(getSE)→SE(upload)→AM(registerFile). File Download: Client→Gateway→QM→cache?→AM(getFile)→SE(stream). JWT Auth: Client→KC(getToken)→Gateway→Module(validateJWT via JWKS). SE Registration: Admin→AM(registerSE) / SE→AM(healthcheck).
  - **Creates**:
    - `docs/design/seq-file-upload.drawio`
    - `docs/design/seq-file-download.drawio`
    - `docs/design/seq-jwt-auth.drawio`
    - `docs/design/seq-se-registration.drawio`
  - **Links**:
    - [Существующие seq диаграммы](docs/design/) — как референс стиля

- [x] **2.7 Deployment Diagrams (K8s + Hybrid)**
  - **Dependencies**: None
  - **Description**: Deployment K8s: все модули в namespace, PVC для SE, PostgreSQL, Keycloak, Gateway/Ingress, Prometheus. Deployment Hybrid: K8s кластер (AM, IM, QM, PG, KC) + remote Docker hosts (SE instances), WAN connection, firewall, TLS.
  - **Creates**:
    - `docs/design/deploy-k8s.drawio`
    - `docs/design/deploy-hybrid.drawio`
  - **Links**: N/A

- [x] **2.8 PNG-экспорт всех диаграмм**
  - **Dependencies**: 2.1-2.7
  - **Description**: Экспортировать все 13 drawio файлов в PNG для встраивания в Markdown. Использовать `drawio CLI` (`drawio --export --format png --output <file>.png <file>.drawio`) или вручную. Сохранить в `docs/guides/images/`.
  - **Creates**:
    - `docs/guides/images/c4-context.png`
    - `docs/guides/images/c4-container.png`
    - `docs/guides/images/c4-component-se.png`
    - `docs/guides/images/c4-component-am.png`
    - `docs/guides/images/dataflow-upload.png`
    - `docs/guides/images/dataflow-download.png`
    - `docs/guides/images/dataflow-search.png`
    - `docs/guides/images/seq-file-upload.png`
    - `docs/guides/images/seq-file-download.png`
    - `docs/guides/images/seq-jwt-auth.png`
    - `docs/guides/images/seq-se-registration.png`
    - `docs/guides/images/deploy-k8s.png`
    - `docs/guides/images/deploy-hybrid.png`
  - **Links**:
    - [drawio CLI docs](https://www.drawio.com/doc/faq/command-line-export)

### ✅ Критерии завершения Phase 2

- [x] Все подпункты завершены (2.1-2.8)
- [x] 13 drawio файлов созданы в `docs/design/`
- [x] 13 PNG файлов экспортированы в `docs/guides/images/`
- [x] Диаграммы корректно отображаются при открытии в drawio editor
- [x] PNG-файлы имеют читабельное разрешение (1600px ширина)

---

## Phase 3: Скриншоты (Playwright MCP)

**Dependencies**: Phase 1 (тестовое окружение должно быть задеплоено с annotations)
**Status**: ✅ Done

### Описание

Автоматическое создание скриншотов Admin UI (9 шт.) и Keycloak Admin Console (25 шт.) средствами Playwright MCP. Требуется работающее тестовое окружение.

### Подпункты

- [x] **3.1 Подготовка тестового окружения**
  - **Dependencies**: None
  - **Description**: Убедиться что тестовое окружение работает: `make test-env-up && make init-data && make port-forward-start`. Проверить доступность `https://artstore.kryukov.lan/admin/` и `https://artstore.kryukov.lan/admin/master/console/`.
  - **Creates**: N/A
  - **Links**: N/A

- [x] **3.2 Скриншоты Admin UI (9 шт.)**
  - **Dependencies**: 3.1
  - **Description**: Через Playwright MCP: открыть Admin UI → авторизоваться (admin/admin) → последовательно снять скриншоты: Dashboard, Файлы (список), Файл (детали), SE (список), SE (детали), SE (смена режима), Мониторинг, Настройки, Service Accounts. Сохранить с именами `ui-*.png`.
  - **Creates**:
    - `docs/guides/images/ui-dashboard.png`
    - `docs/guides/images/ui-files-list.png`
    - `docs/guides/images/ui-file-details.png`
    - `docs/guides/images/ui-se-list.png`
    - `docs/guides/images/ui-se-details.png`
    - `docs/guides/images/ui-se-mode-change.png`
    - `docs/guides/images/ui-monitoring.png`
    - `docs/guides/images/ui-settings.png`
    - `docs/guides/images/ui-service-accounts.png`
  - **Links**: N/A

- [x] **3.3 Скриншот Keycloak Login (кастомная тема)**
  - **Dependencies**: 3.1
  - **Description**: Через Playwright MCP: открыть `https://artstore.kryukov.lan/admin/` (не авторизованным — покажет KC login). Снять скриншот страницы логина с кастомной темой `artstore`.
  - **Creates**:
    - `docs/guides/images/kc-login.png`
  - **Links**: N/A

- [x] **3.4 Скриншоты Keycloak — Realm Settings (4 шт.)**
  - **Dependencies**: 3.1
  - **Description**: Через Playwright MCP: открыть KC Admin Console → realm `artstore` → Realm Settings. Снять скриншоты вкладок: General, Login, Sessions, Tokens.
  - **Creates**:
    - `docs/guides/images/kc-realm-general.png`
    - `docs/guides/images/kc-realm-login.png`
    - `docs/guides/images/kc-realm-sessions.png`
    - `docs/guides/images/kc-realm-tokens.png`
  - **Links**: N/A

- [x] **3.5 Скриншоты Keycloak — Roles & Groups (3 шт.)**
  - **Dependencies**: 3.1
  - **Description**: Realm Roles (список), Groups → artstore-admins (role mapping), Groups → artstore-viewers (role mapping).
  - **Creates**:
    - `docs/guides/images/kc-realm-roles.png`
    - `docs/guides/images/kc-group-admins.png`
    - `docs/guides/images/kc-group-viewers.png`
  - **Links**: N/A

- [x] **3.6 Скриншоты Keycloak — Client Scopes (4 шт.)**
  - **Dependencies**: 3.1
  - **Description**: Client Scopes (список), scope `files:read` (Settings), scope `files:read` (Mappers → audience mapper), scope `groups` (Mappers → group membership mapper).
  - **Creates**:
    - `docs/guides/images/kc-client-scopes-list.png`
    - `docs/guides/images/kc-scope-files-read.png`
    - `docs/guides/images/kc-scope-files-read-mappers.png`
    - `docs/guides/images/kc-scope-groups-mapper.png`
  - **Links**: N/A

- [x] **3.7 Скриншоты Keycloak — Clients (10 шт.)**
  - **Dependencies**: 3.1
  - **Description**: Для каждого клиента — Settings, Credentials (где применимо), Client Scopes, SA Roles (где применимо). AM: Settings, Credentials, Scopes, SA Roles (4). IM: Settings, Scopes (2). QM: Settings (1). Admin UI: Settings, Scopes (2). Mapper `client_id` (1).
  - **Creates**:
    - `docs/guides/images/kc-client-am-settings.png`
    - `docs/guides/images/kc-client-am-credentials.png`
    - `docs/guides/images/kc-client-am-scopes.png`
    - `docs/guides/images/kc-client-am-sa-roles.png`
    - `docs/guides/images/kc-client-im-settings.png`
    - `docs/guides/images/kc-client-im-scopes.png`
    - `docs/guides/images/kc-client-qm-settings.png`
    - `docs/guides/images/kc-client-ui-settings.png`
    - `docs/guides/images/kc-client-ui-scopes.png`
    - `docs/guides/images/kc-mapper-client-id.png`
  - **Links**: N/A

- [x] **3.8 Скриншоты Keycloak — Create Client Wizard (4 шт.)**
  - **Dependencies**: 3.1
  - **Description**: Пошаговые скриншоты создания нового клиента: Create Client wizard Step 1 (General), Step 2 (Capability), Step 3 (Login settings), Add mapper → By configuration → User Session Note.
  - **Creates**:
    - `docs/guides/images/kc-wizard-step1-general.png`
    - `docs/guides/images/kc-wizard-step2-capability.png`
    - `docs/guides/images/kc-wizard-step3-login.png`
    - `docs/guides/images/kc-add-mapper-step.png`
  - **Links**: N/A

### ✅ Критерии завершения Phase 3

- [x] Все подпункты завершены (3.1-3.8)
- [x] 9 скриншотов Admin UI в `docs/guides/images/ui-*.png`
- [x] 25 скриншотов Keycloak в `docs/guides/images/kc-*.png`
- [x] Все скриншоты читабельны, UI-элементы различимы
- [x] Скриншоты отражают актуальное состояние тестового окружения

---

## Phase 4: Admin Guide (EN + RU)

**Dependencies**: Phase 2 (диаграммы), Phase 3 (скриншоты)
**Status**: ✅ Done

### Описание

Написание Admin Guide — основного документа для администраторов системы. Включает обзор архитектуры, установку, конфигурацию, управление SE, Admin UI, подробную конфигурацию Keycloak, обновление. Создаётся в двух языковых версиях (EN + RU).

### Подпункты

- [x] **4.1 Admin Guide — обзор архитектуры + требования (EN)**
  - **Dependencies**: None
  - **Description**: Написать §1 (обзор архитектуры — с ссылками на C4 и Data Flow диаграммы, описание вариантов развёртывания SE: K8s + Docker remote) и §2 (требования — K8s, Docker, PG, KC, сеть, ресурсы). Использовать данные из `docs/briefs/`, `docs/api-contracts/`, `docs/requirements/`.
  - **Creates**:
    - `docs/guides/admin-guide.md` (§1-2)
  - **Links**:
    - [Требования](docs/requirements/documentation-monitoring-requirements.md) — раздел 2.3
    - [Модульные брифы](docs/briefs/)

- [x] **4.2 Admin Guide — установка + конфигурация (EN)**
  - **Dependencies**: 4.1
  - **Description**: Написать §3 (установка — Quick Start, Production, отдельные модули, SE remote, Gateway API, Ingress) и §4 (конфигурация — env-переменные всех модулей, PG, TLS, сеть для удалённых SE). Извлечь env-переменные из `internal/config/` каждого модуля.
  - **Creates**:
    - Дополнение `docs/guides/admin-guide.md` (§3-4)
  - **Links**:
    - [AM config](src/admin-module/internal/config/)
    - [SE config](src/storage-element/internal/config/)
    - [IM config](src/ingester-module/internal/config/)
    - [QM config](src/query-module/internal/config/)

- [x] **4.3 Admin Guide — управление SE + Admin UI (EN)**
  - **Dependencies**: 4.1
  - **Description**: Написать §5 (управление SE — жизненный цикл, C4 Component diagram, регистрация K8s/Docker, скриншоты Admin UI, репликация, ёмкость, гео-распределённые SE) и §6 (Admin UI — скриншоты всех страниц, описание функций). Вставить ссылки на скриншоты из `images/`.
  - **Creates**:
    - Дополнение `docs/guides/admin-guide.md` (§5-6)
  - **Links**: N/A

- [x] **4.4 Admin Guide — Keycloak конфигурация (EN)**
  - **Dependencies**: 4.1
  - **Description**: Написать §7 (Keycloak) — самый подробный раздел. Realm settings, роли, группы, client scopes, все 4 клиента, mapper `client_id`, URL-схема, кастомная тема, пошаговое создание нового клиента. Каждый подраздел со скриншотами Keycloak Admin Console. Опираться на данные из `tests/helm/artstore-infra/files/artstore-realm.json`.
  - **Creates**:
    - Дополнение `docs/guides/admin-guide.md` (§7)
  - **Links**:
    - [Realm JSON](tests/helm/artstore-infra/files/artstore-realm.json)
    - [KC theme](deploy/keycloak/)
    - [Требования](docs/requirements/documentation-monitoring-requirements.md) — раздел 2.8

- [x] **4.5 Admin Guide — обновление + финализация (EN)**
  - **Dependencies**: 4.2, 4.3, 4.4
  - **Description**: Написать §8 (обновление — порядок, миграции, совместимость). Добавить Table of Contents, перекрёстные ссылки между разделами, ссылку на RU-версию. Финальная вычитка и проверка всех ссылок на изображения.
  - **Creates**:
    - Финализация `docs/guides/admin-guide.md`
  - **Links**: N/A

- [x] **4.6 Admin Guide — RU-версия**
  - **Dependencies**: 4.5
  - **Description**: Полный перевод `admin-guide.md` на русский язык. Сохранить как `admin-guide.ru.md`. Добавить перекрёстные ссылки: EN→RU и RU→EN в начале каждого файла. Все заголовки, таблицы, описания, примеры кода (комментарии) — на русском.
  - **Creates**:
    - `docs/guides/admin-guide.ru.md`
  - **Links**: N/A

### ✅ Критерии завершения Phase 4

- [x] Все подпункты завершены (4.1-4.6)
- [x] `docs/guides/admin-guide.md` — полный EN-документ (8 разделов)
- [x] `docs/guides/admin-guide.ru.md` — полный RU-перевод
- [x] Все ссылки на изображения (`![](images/*.png)`) валидны
- [x] Перекрёстные ссылки EN↔RU работают
- [x] Markdown lint проверка пройдена

---

## Phase 5: Developer Guide (EN + RU)

**Dependencies**: Phase 2 (диаграммы — Data Flow, Sequence)
**Status**: ✅ Done

### Описание

Написание Developer Guide для разработчиков-интеграторов. Документирует API: аутентификация, upload, search, download, file management, error handling. С примерами curl, Python, Go.

### Подпункты

- [x] **5.1 Developer Guide — обзор API + аутентификация (EN)**
  - **Dependencies**: None
  - **Description**: Написать §1 (обзор API — архитектура, базовые URL, ссылки на OpenAPI specs) и §2 (аутентификация — JWT получение через KC password grant и client credentials, scopes, роли, примеры curl). Использовать OpenAPI specs из `docs/api-contracts/`.
  - **Creates**:
    - `docs/guides/developer-guide.md` (§1-2)
  - **Links**:
    - [AM OpenAPI](docs/api-contracts/admin-module-openapi.yaml)
    - [IM OpenAPI](docs/api-contracts/ingester-module-openapi.yaml)
    - [QM OpenAPI](docs/api-contracts/query-module-openapi.yaml)
    - [SE OpenAPI](docs/api-contracts/storage-element-openapi.yaml)

- [x] **5.2 Developer Guide — Upload + Search + Download (EN)**
  - **Dependencies**: 5.1
  - **Description**: Написать §3 (Upload — endpoint, параметры, multipart, примеры curl/Python/Go), §4 (Search — FTS параметры, пагинация, фильтрация, примеры) и §5 (Download — proxy через QM, кэш, streaming, примеры). Вставить ссылки на Data Flow и Sequence диаграммы.
  - **Creates**:
    - Дополнение `docs/guides/developer-guide.md` (§3-5)
  - **Links**:
    - [IM handlers](src/ingester-module/internal/api/handlers/)
    - [QM handlers](src/query-module/internal/api/handlers/)

- [x] **5.3 Developer Guide — File Management + Errors + финализация (EN)**
  - **Dependencies**: 5.2
  - **Description**: Написать §6 (File Management через AM API — CRUD, метаданные) и §7 (Error Handling — формат ошибок, таблица кодов). Добавить ToC, перекрёстные ссылки. Финальная вычитка.
  - **Creates**:
    - Финализация `docs/guides/developer-guide.md`
  - **Links**:
    - [AM handlers](src/admin-module/internal/api/handlers/)

- [x] **5.4 Developer Guide — RU-версия**
  - **Dependencies**: 5.3
  - **Description**: Полный перевод `developer-guide.md` на русский. Примеры кода остаются на английском (curl, Python, Go), комментарии в примерах переводятся. Перекрёстные ссылки EN↔RU.
  - **Creates**:
    - `docs/guides/developer-guide.ru.md`
  - **Links**: N/A

### ✅ Критерии завершения Phase 5

- [x] Все подпункты завершены (5.1-5.4)
- [x] `docs/guides/developer-guide.md` — полный EN-документ (7 разделов)
- [x] `docs/guides/developer-guide.ru.md` — полный RU-перевод
- [x] Все примеры curl проверены на корректность синтаксиса
- [x] Ссылки на OpenAPI specs валидны
- [x] Markdown lint проверка пройдена

---

## Phase 6: Grafana dashboards + AlertManager rules

**Dependencies**: Phase 1 (annotations — для тестирования)
**Status**: ✅ Done

### Описание

Создание 6 Grafana dashboards (JSON) и AlertManager rules (YAML). Dashboards покрывают: Overview, каждый модуль, Dependency Topology (topologymetrics).

### Подпункты

- [x] **6.1 Grafana Dashboard: Artstore Overview**
  - **Dependencies**: None
  - **Description**: Создать JSON dashboard. Rows: System Health (traffic lights на базе `app_dependency_health`), Request Rate (stacked по модулям), Latency (p50/p95/p99 histogram_quantile), Error Rate (% 5xx), Storage (files, bytes, capacity). Variables: `$namespace`, `$interval`. Datasource: Prometheus.
  - **Creates**:
    - `charts/artstore/dashboards/overview.json`
  - **Links**: N/A

- [x] **6.2 Grafana Dashboard: Admin Module**
  - **Dependencies**: None
  - **Description**: AM-специфичный dashboard. Метрики: `am_http_requests_total`, `am_http_request_duration_seconds`. Panels: RPS by path, Latency by path, Error rate, Active connections (если метрика есть).
  - **Creates**:
    - `charts/artstore/dashboards/admin-module.json`
  - **Links**: N/A

- [x] **6.3 Grafana Dashboard: Storage Element**
  - **Dependencies**: None
  - **Description**: SE-специфичный dashboard. Метрики: `se_http_*`, `se_files_total{status}`, `se_storage_bytes`, `se_operations_total{operation,result}`. Panels: Files by status, Storage usage, Operations (upload/delete/replicate), RPS, Latency. Variable: `$instance` (для выбора конкретного SE).
  - **Creates**:
    - `charts/artstore/dashboards/storage-element.json`
  - **Links**: N/A

- [x] **6.4 Grafana Dashboard: Ingester Module**
  - **Dependencies**: None
  - **Description**: IM-специфичный dashboard. Метрики: `im_http_requests_total`, `im_http_request_duration_seconds`. Panels: Upload RPS, Latency (p50/p95/p99), Error rate by status code. (Бизнес-метрики пока отсутствуют — см. открытый вопрос №3).
  - **Creates**:
    - `charts/artstore/dashboards/ingester-module.json`
  - **Links**: N/A

- [x] **6.5 Grafana Dashboard: Query Module**
  - **Dependencies**: None
  - **Description**: QM-специфичный dashboard. Метрики: `qm_http_requests_total`, `qm_http_request_duration_seconds`. Panels: Search RPS, Download RPS, Latency by endpoint, Error rate. (Cache метрики пока отсутствуют — см. открытый вопрос №2).
  - **Creates**:
    - `charts/artstore/dashboards/query-module.json`
  - **Links**: N/A

- [x] **6.6 Grafana Dashboard: Dependency Topology**
  - **Dependencies**: None
  - **Description**: topologymetrics dashboard. Метрики: `app_dependency_health`, `app_dependency_latency_seconds`, `app_dependency_status`. Panels: Health Matrix (status map), Dependency Latency (heatmap), Critical Dependencies (таблица), Topology Graph (Node Graph panel — Grafana 10+). Variables: `$namespace`, `$group`.
  - **Creates**:
    - `charts/artstore/dashboards/dependency-topology.json`
  - **Links**:
    - [topologymetrics SDK](https://github.com/BigKAA/topologymetrics)

- [x] **6.7 AlertManager Rules**
  - **Dependencies**: None
  - **Description**: Создать YAML с PrometheusRule (Kubernetes CRD) или standalone rules. Алерты: ArtstoreServiceDown, ArtstoreHighErrorRate, ArtstoreHighLatency, SEStorageFull, SENoEditAvailable, DependencyLatencyHigh. Каждый алерт с annotations (summary, description, runbook_url).
  - **Creates**:
    - `charts/artstore/alerts/artstore-alerts.yaml`
  - **Links**: N/A

- [x] **6.8 Тестирование dashboards**
  - **Dependencies**: 6.1-6.7
  - **Description**: Импортировать dashboards в Grafana тестового кластера (если установлена). Проверить что все панели рендерятся, queries возвращают данные. Если Grafana не установлена — валидировать JSON через `jq` и проверить PromQL queries через `curl` к Prometheus API.
  - **Creates**: N/A
  - **Links**: N/A

### ✅ Критерии завершения Phase 6

- [x] Все подпункты завершены (6.1-6.8)
- [x] 6 JSON dashboard файлов в `charts/artstore/dashboards/`
- [x] 1 YAML файл алертов в `charts/artstore/alerts/`
- [x] JSON файлы валидны (`jq . < file.json` без ошибок)
- [x] PromQL queries синтаксически корректны
- [x] AlertManager rules YAML валиден

---

## Phase 7: Operations Guide (EN + RU)

**Dependencies**: Phase 6 (dashboards, alerts), Phase 3 (скриншоты мониторинга)
**Status**: ✅ Done

### Описание

Написание Operations Guide для операционной команды. Покрывает мониторинг, dashboards, алертинг, troubleshooting, backup-рекомендации, масштабирование.

### Подпункты

- [x] **7.1 Operations Guide — Мониторинг + Dashboards (EN)**
  - **Dependencies**: None
  - **Description**: Написать §1 (обзор метрик — таблица всех метрик по модулям, настройка ServiceMonitor/Pod annotations, встроенный мониторинг Admin UI со скриншотом, topologymetrics + uniproxy) и §2 (Grafana Dashboards — установка, provisioning через ConfigMap, описание каждого из 6 дашбордов с ключевыми панелями).
  - **Creates**:
    - `docs/guides/operations-guide.md` (§1-2)
  - **Links**:
    - [Требования](docs/requirements/documentation-monitoring-requirements.md) — раздел 2.9, 4

- [x] **7.2 Operations Guide — Алертинг + Troubleshooting (EN)**
  - **Dependencies**: 7.1
  - **Description**: Написать §3 (алертинг — описание каждого алерта, пороги, severity, runbooks: что делать при срабатывании) и §4 (troubleshooting — типичные проблемы, диагностика через topologymetrics, формат логов slog JSON, health check endpoints, примеры `kubectl logs` + `jq`).
  - **Creates**:
    - Дополнение `docs/guides/operations-guide.md` (§3-4)
  - **Links**: N/A

- [x] **7.3 Operations Guide — Backup + Масштабирование + финализация (EN)**
  - **Dependencies**: 7.2
  - **Description**: Написать §5 (backup рекомендации — pg_dump для PG, VolumeSnapshot для PVC SE, KC realm export, пометка что автоматизация = будущая задача) и §6 (масштабирование — horizontal scaling IM/QM, добавление SE, рекомендации по ресурсам). ToC, ссылки, вычитка.
  - **Creates**:
    - Финализация `docs/guides/operations-guide.md`
  - **Links**: N/A

- [x] **7.4 Operations Guide — RU-версия**
  - **Dependencies**: 7.3
  - **Description**: Полный перевод `operations-guide.md` на русский. Перекрёстные ссылки EN↔RU.
  - **Creates**:
    - `docs/guides/operations-guide.ru.md`
  - **Links**: N/A

### ✅ Критерии завершения Phase 7

- [x] Все подпункты завершены (7.1-7.4)
- [x] `docs/guides/operations-guide.md` — полный EN-документ (6 разделов)
- [x] `docs/guides/operations-guide.ru.md` — полный RU-перевод
- [x] Runbooks для каждого алерта описаны
- [x] Ссылки на dashboards JSON корректны
- [x] Markdown lint проверка пройдена

---

## Phase 8: Umbrella Helm chart

**Dependencies**: Phase 1 (annotations), Phase 6 (dashboards, alerts)
**Status**: ✅ Done

### Описание

Создание единого umbrella Helm chart `artstore` с subcharts для всех модулей. Два профиля: dev и production. Включает provisioning Grafana dashboards и AlertManager rules.

### Подпункты

- [x] **8.1 Chart.yaml + зависимости**
  - **Dependencies**: None
  - **Description**: Создать `charts/artstore/Chart.yaml` (type: application). Определить dependencies: admin-module, storage-element, ingester-module, query-module — как file-based subcharts (`file://../../src/<module>/charts/<module>`). Условные зависимости: postgresql (bitnami), keycloak (bitnami/codecentric). `_helpers.tpl` с общими шаблонами.
  - **Creates**:
    - `charts/artstore/Chart.yaml`
    - `charts/artstore/templates/_helpers.tpl`
  - **Links**:
    - [Helm subcharts](https://helm.sh/docs/chart_template_guide/subcharts_and_globals/)

- [x] **8.2 values.yaml (defaults)**
  - **Dependencies**: 8.1
  - **Description**: Создать `values.yaml` с defaults для всех subcharts. Включить секции: global (domain, namespace, registry), per-module configs (env-vars, resources, replicas), monitoring (enabled, annotations), gateway/ingress toggle.
  - **Creates**:
    - `charts/artstore/values.yaml`
  - **Links**: N/A

- [x] **8.3 values-dev.yaml (dev профиль)**
  - **Dependencies**: 8.2
  - **Description**: Dev/test профиль: minimal resources (100m/128Mi), 1 replica each, 1 SE (edit mode), monitoring enabled (встроенный subchart), no TLS, Keycloak start-dev. Должен работать на minikube/kind с 4GB RAM.
  - **Creates**:
    - `charts/artstore/values-dev.yaml`
  - **Links**: N/A

- [x] **8.4 values-production.yaml (production профиль)**
  - **Dependencies**: 8.2
  - **Description**: Production профиль: recommended resources, HA replicas (IM: 2, QM: 2, AM: 1), multiple SE (edit: 1, rw: 2+), TLS via cert-manager, Gateway API HTTPRoute, external Keycloak, ServiceMonitor CRDs, anti-affinity rules.
  - **Creates**:
    - `charts/artstore/values-production.yaml`
  - **Links**: N/A

- [x] **8.5 Templates (namespace, gateway/ingress, configmaps)**
  - **Dependencies**: 8.2
  - **Description**: Templates: опциональный namespace.yaml, HTTPRoute (Gateway API) или Ingress (nginx) через условие, ConfigMaps для Grafana dashboards provisioning, PrometheusRule для alerts. Все управляется через values flags.
  - **Creates**:
    - `charts/artstore/templates/namespace.yaml`
    - `charts/artstore/templates/httproute.yaml`
    - `charts/artstore/templates/ingress.yaml`
    - `charts/artstore/templates/grafana-dashboards-cm.yaml`
    - `charts/artstore/templates/prometheus-rules.yaml`
  - **Links**: N/A

- [x] **8.6 Тестирование umbrella chart (dev профиль)**
  - **Dependencies**: 8.1-8.5
  - **Description**: `helm template artstore charts/artstore/ -f charts/artstore/values-dev.yaml` — проверить рендеринг. Попробовать `helm install` в тестовый кластер (отдельный namespace). Проверить что все pods стартуют.
  - **Creates**: N/A
  - **Links**: N/A

### ✅ Критерии завершения Phase 8

- [x] Все подпункты завершены (8.1-8.6)
- [x] `helm template` рендерит корректные манифесты для обоих профилей
- [x] `helm lint charts/artstore/` без ошибок
- [ ] Dev профиль успешно устанавливается в тестовый кластер
- [x] Grafana dashboards provisioning работает (ConfigMaps созданы)
- [x] PrometheusRule создаётся при `monitoring.alerts.enabled: true`

---

## Phase 9: docker-compose + Monitoring subchart

**Dependencies**: Phase 8 (umbrella chart — для monitoring subchart)
**Status**: Pending

### Описание

Завершающая фаза: docker-compose для Quick Start разработчиков и опциональный monitoring subchart (Prometheus + Grafana) для тестовых/небольших инсталляций.

### Подпункты

- [x] **9.1 docker-compose.yaml (Quick Start)**
  - **Dependencies**: None
  - **Description**: Создать `docker-compose.yaml` в корне проекта. Сервисы: PostgreSQL 17, Keycloak (с realm import из `deploy/keycloak/artstore-realm.json`), Admin Module, Storage Element (1, mode edit), Ingester Module, Query Module. Все env-переменные inline. Порты: стандартные (8000-8039). Volume для PG data и SE storage.
  - **Creates**:
    - `docker-compose.yaml`
  - **Links**:
    - [Keycloak realm](deploy/keycloak/artstore-realm.json)

- [ ] **9.2 docker-compose тестирование**
  - **Dependencies**: 9.1
  - **Description**: `docker-compose up -d` → проверить что все 6 контейнеров стартуют → health checks → basic API call (получить токен, загрузить файл, найти файл, скачать файл). Задокументировать troubleshooting если что-то не стартует.
  - **Creates**: N/A
  - **Links**: N/A

- [x] **9.3 Monitoring subchart (Prometheus + Grafana)**
  - **Dependencies**: None
  - **Description**: Создать subchart `charts/artstore/charts/monitoring/`. Prometheus: minimal config со scrape artstore pods (через kubernetes_sd_configs + relabeling по annotations). Grafana: pre-provisioned dashboards из `dashboards/` directory, anonymous access для dev. Всё управляется через `monitoring.enabled` в values.
  - **Creates**:
    - `charts/artstore/charts/monitoring/Chart.yaml`
    - `charts/artstore/charts/monitoring/values.yaml`
    - `charts/artstore/charts/monitoring/templates/prometheus-deployment.yaml`
    - `charts/artstore/charts/monitoring/templates/prometheus-configmap.yaml`
    - `charts/artstore/charts/monitoring/templates/grafana-deployment.yaml`
    - `charts/artstore/charts/monitoring/templates/grafana-provisioning.yaml`
  - **Links**:
    - [Prometheus K8s SD](https://prometheus.io/docs/prometheus/latest/configuration/configuration/#kubernetes_sd_config)

- [ ] **9.4 Monitoring subchart тестирование**
  - **Dependencies**: 9.3
  - **Description**: Установить umbrella chart с `monitoring.enabled: true` (values-dev.yaml). Проверить: Prometheus scraping метрики всех модулей, Grafana dashboards доступны, данные отображаются на панелях.
  - **Creates**: N/A
  - **Links**: N/A

- [ ] **9.5 Финальная проверка и обновление документации**
  - **Dependencies**: 9.2, 9.4
  - **Description**: Проверить что Admin Guide §3 (Quick Start) соответствует реальному docker-compose. Проверить что Operations Guide §2 (Grafana) описывает provisioning через monitoring subchart. Обновить если есть расхождения. Проверить все ссылки в документации.
  - **Creates**:
    - Возможные обновления `docs/guides/admin-guide.md`, `docs/guides/operations-guide.md`
  - **Links**: N/A

### ✅ Критерии завершения Phase 9

- [ ] Все подпункты завершены (9.1-9.5)
- [ ] `docker-compose up -d` стартует все сервисы
- [ ] Monitoring subchart устанавливается и показывает метрики
- [ ] Вся документация (3 guide-а × 2 языка) проверена на актуальность
- [ ] Все ссылки на изображения и файлы валидны

---

## Примечания

- **drawio формат**: drawio файлы — XML-based. AI может генерировать их программно, но результат может требовать ручной доводки в drawio editor. Рекомендуется review каждой диаграммы перед PNG-экспортом.
- **Playwright MCP скриншоты**: требуют работающего тестового окружения. Если окружение недоступно, Phase 3 блокируется. Обходной путь: создать placeholder-изображения и заменить позже.
- **Открытые вопросы из требований** (раздел 7): вопросы 1-3 (добавление бизнес-метрик в SE/QM/IM) могут потребовать отдельного плана разработки. В текущем плане dashboards создаются на основе существующих метрик.
- **Объём работ**: ~47 изображений (13 диаграмм + 34 скриншота), 6 документов (3 guide × 2 языка), 6 dashboards JSON, 1 alerts YAML, 1 umbrella chart, 1 docker-compose, 1 monitoring subchart.
- **Оценка фаз по размеру**: Phase 4 (Admin Guide) — самая объёмная из-за подробного раздела Keycloak. Рекомендуется выделять на неё отдельную длинную сессию.

---

**План готов к использованию.**
