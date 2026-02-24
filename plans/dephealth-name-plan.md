# План разработки: DEPHEALTH_NAME — корректная метка `name` в topologymetrics

## Метаданные

- **Версия плана**: 1.0.0
- **Дата создания**: 2026-02-24
- **Последнее обновление**: 2026-02-24
- **Статус**: Pending

---

## История версий

- **v1.0.0** (2026-02-24): Начальная версия плана

---

## Текущий статус

- **Активная фаза**: Phase 2
- **Активный подпункт**: 2.1
- **Последнее обновление**: 2026-02-24
- **Примечание**: Phase 1 завершена — Admin Module Go-код реализован и протестирован

---

## Контекст задачи

Метка `name` в метриках topologymetrics SDK должна содержать имя **владельца** пода
(Deployment / StatefulSet / DaemonSet), а не имя самого пода. SDK предоставляет
env-переменную `DEPHEALTH_NAME` и первый аргумент `dephealth.New(name, ...)`.
Исходные коды SDK не модифицируются — реализация на стороне модулей Artstore.

### Цепочка определения `name`

```text
1. DEPHEALTH_NAME (env) → использовать как есть
2. Парсинг os.Hostname() → извлечь имя владельца → slog.Warn
3. Fallback модуля → текущее значение ("admin-module" / cfg.StorageID)
```

### Паттерны парсинга hostname

| Тип | Паттерн | Regex | Пример |
|-----|---------|-------|--------|
| Deployment | `{name}-{rs-hash 8-10}-{pod-hash 4-5}` | `^(.+)-[a-z0-9]{8,10}-[a-z0-9]{4,5}$` | `admin-module-7d8f9b6c4f-x2k9z` → `admin-module` |
| StatefulSet | `{name}-{ordinal}` | `^(.+)-(\d+)$` | `storage-element-se-01-0` → `storage-element-se-01` |
| Fallback | hostname целиком | — | `my-app` → `my-app` |

---

## Оглавление

- [x] [Phase 1: Admin Module — Go-код](#phase-1-admin-module--go-код)
- [ ] [Phase 2: Storage Element — Go-код](#phase-2-storage-element--go-код)
- [ ] [Phase 3: Helm charts и сборка](#phase-3-helm-charts-и-сборка)
- [ ] [Phase 4: Деплой и верификация](#phase-4-деплой-и-верификация)

---

## Phase 1: Admin Module — Go-код

**Dependencies**: None
**Status**: ✅ Done

### Описание

Добавить конфигурационный параметр `DEPHEALTH_NAME` и логику auto-resolve
из hostname в Admin Module. Убрать хардкод `"admin-module"` в `main.go`.

### Подпункты

- [x] **1.1 Добавить поле `DephealthName` в конфигурацию**
  - **Dependencies**: None
  - **Description**: В `internal/config/config.go`:
    - Добавить поле `DephealthName string` в struct `Config` (секция "Синхронизация",
      после `DephealthGroup`)
    - В `Load()` добавить загрузку: `cfg.DephealthName = getEnvDefault("DEPHEALTH_NAME", "")`
      (после строки загрузки `AM_DEPHEALTH_GROUP`)
  - **Modifies**:
    - `src/admin-module/internal/config/config.go`

- [x] **1.2 Добавить функции резолва и обновить main.go**
  - **Dependencies**: 1.1
  - **Description**: В `cmd/admin-module/main.go`:
    - Добавить `"regexp"` в imports
    - Добавить функцию `parseOwnerName(hostname string) string` — парсинг hostname
      по regex-паттернам Deployment и StatefulSet
    - Добавить функцию `resolveOwnerFromHostname(logger, fallback) string` —
      обёртка с `os.Hostname()` + `slog.Warn`
    - Перед вызовом `service.NewDephealthService()` добавить блок резолва:
      `DEPHEALTH_NAME → auto-resolve → fallback "admin-module"`
    - Заменить хардкод `"admin-module"` на `dephealthName` в вызове
      `service.NewDephealthService()`
    - Добавить `slog.String("name", dephealthName)` в info-лог
      `"topologymetrics запущен"`
  - **Modifies**:
    - `src/admin-module/cmd/admin-module/main.go`

- [x] **1.3 Unit-тест `parseOwnerName`**
  - **Dependencies**: 1.2
  - **Description**: Создать `cmd/admin-module/main_test.go` (или
    `cmd/admin-module/dephealth_name_test.go`) с table-driven тестами:
    - `admin-module-7d8f9b6c4f-x2k9z` → `admin-module` (Deployment)
    - `storage-element-se-01-5fbcd8d7b9-k4m2j` → `storage-element-se-01` (Deployment)
    - `my-sts-0` → `my-sts` (StatefulSet)
    - `my-sts-42` → `my-sts` (StatefulSet)
    - `my-app` → `my-app` (Fallback)
    - `localhost` → `localhost` (Fallback)
  - **Creates**:
    - `src/admin-module/cmd/admin-module/dephealth_name_test.go`

### Критерии завершения Phase 1

- [x] Все подпункты завершены (1.1, 1.2, 1.3)
- [x] `go build ./...` проходит без ошибок в `src/admin-module/`
- [x] `go test ./...` проходит без ошибок в `src/admin-module/`

---

## Phase 2: Storage Element — Go-код

**Dependencies**: None (параллельно с Phase 1)
**Status**: Pending

### Описание

Аналогичные изменения для Storage Element. Функции `parseOwnerName` и
`resolveOwnerFromHostname` дублируются (модули — независимые Go-проекты).

### Подпункты

- [ ] **2.1 Добавить поле `DephealthName` в конфигурацию**
  - **Dependencies**: None
  - **Description**: В `internal/config/config.go`:
    - Добавить поле `DephealthName string` в struct `Config`
      (после `DephealthDepName`)
    - В `Load()` добавить загрузку: `cfg.DephealthName = getEnvDefault("DEPHEALTH_NAME", "")`
      (после строки загрузки `SE_DEPHEALTH_DEP_NAME`)
  - **Modifies**:
    - `src/storage-element/internal/config/config.go`

- [ ] **2.2 Добавить функции резолва и обновить main.go**
  - **Dependencies**: 2.1
  - **Description**: В `cmd/storage-element/main.go`:
    - Добавить `"regexp"` в imports
    - Добавить функции `parseOwnerName` и `resolveOwnerFromHostname`
      (дубль из Admin Module)
    - Перед вызовом `service.NewDephealthService()` добавить блок резолва:
      `DEPHEALTH_NAME → auto-resolve → fallback cfg.StorageID`
    - Заменить `cfg.StorageID` на `dephealthName` в вызове
      `service.NewDephealthService()`
    - Добавить `slog.String("name", dephealthName)` в info-лог
      `"topologymetrics запущен"`
  - **Modifies**:
    - `src/storage-element/cmd/storage-element/main.go`

- [ ] **2.3 Unit-тест `parseOwnerName`**
  - **Dependencies**: 2.2
  - **Description**: Создать `cmd/storage-element/dephealth_name_test.go`
    с аналогичными table-driven тестами (дубль из Phase 1.3)
  - **Creates**:
    - `src/storage-element/cmd/storage-element/dephealth_name_test.go`

### Критерии завершения Phase 2

- [ ] Все подпункты завершены (2.1, 2.2, 2.3)
- [ ] `go build ./...` проходит без ошибок в `src/storage-element/`
- [ ] `go test ./...` проходит без ошибок в `src/storage-element/`

---

## Phase 3: Helm charts и сборка

**Dependencies**: Phase 1, Phase 2
**Status**: Pending

### Описание

Обновить Helm charts обоих модулей — добавить `DEPHEALTH_NAME` со значением
из Helm helper `fullname`. Собрать Docker-образы и запушить в Harbor.

### Подпункты

- [ ] **3.1 Helm chart Admin Module**
  - **Dependencies**: None
  - **Description**: В `charts/admin-module/templates/configmap.yaml`:
    - Добавить строку `DEPHEALTH_NAME: {{ include "am.fullname" . | quote }}`
      в секцию `# --- Dependency health (topologymetrics) ---`
      (перед `AM_DEPHEALTH_CHECK_INTERVAL`)
  - **Modifies**:
    - `src/admin-module/charts/admin-module/templates/configmap.yaml`

- [ ] **3.2 Helm chart Storage Element**
  - **Dependencies**: None
  - **Description**: В `charts/storage-element/templates/_helpers.tpl`:
    - Добавить переменную в define `se.envVars`:
      ```yaml
      - name: DEPHEALTH_NAME
        value: {{ include "se.fullname" . | quote }}
      ```
      (после `SE_STORAGE_ID`, перед `SE_DATA_DIR`)
  - **Modifies**:
    - `src/storage-element/charts/storage-element/templates/_helpers.tpl`

- [ ] **3.3 Сборка Docker-образов**
  - **Dependencies**: 3.1, 3.2
  - **Description**: Собрать и запушить образы обоих модулей в Harbor:
    - `harbor.kryukov.lan/library/admin-module:v<next-tag>`
    - `harbor.kryukov.lan/library/storage-element:v<next-tag>`
    - Спросить пользователя о номере тега перед сборкой
  - **Creates**:
    - Docker image admin-module
    - Docker image storage-element

### Критерии завершения Phase 3

- [ ] Все подпункты завершены (3.1, 3.2, 3.3)
- [ ] `helm template` рендерит `DEPHEALTH_NAME` корректно для обоих модулей
- [ ] Docker-образы собраны и запушены в Harbor

---

## Phase 4: Деплой и верификация

**Dependencies**: Phase 3
**Status**: Pending

### Описание

Задеплоить обновлённые модули в тестовый K8s-кластер и проверить, что метрики
содержат корректную метку `name` (имя Deployment/StatefulSet).

### Подпункты

- [ ] **4.1 Деплой в тестовый кластер**
  - **Dependencies**: None
  - **Description**: Обновить тестовое окружение:
    - `make apps-up` (Admin Module)
    - `make se-up` (Storage Elements)
    - Дождаться ready-статуса всех подов
  - **Links**:
    - Тестовые Makefile targets в `tests/`

- [ ] **4.2 Верификация метрик**
  - **Dependencies**: 4.1
  - **Description**: Для каждого модуля проверить endpoint `/metrics`:
    - Метрика `app_dependency_health` содержит `name="admin-module"`
      (а не имя пода)
    - Метрика `app_dependency_health` для SE содержит
      `name="storage-element-<elementId>"` (а не `<elementId>`)
    - В логах подов **нет** warning `DEPHEALTH_NAME не задана` (Helm задаёт)
  - **Links**:
    - `make port-forward-start` для доступа к метрикам

- [ ] **4.3 Проверка auto-resolve (без Helm)**
  - **Dependencies**: 4.1
  - **Description**: Опциональная проверка fallback-логики:
    - Через `docker-compose` или `go run` без `DEPHEALTH_NAME`
    - Убедиться, что в логах появляется warning с resolved_name
    - Метрика содержит имя из hostname (или fallback)

### Критерии завершения Phase 4

- [ ] Все подпункты завершены (4.1, 4.2, 4.3)
- [ ] Метрики в K8s содержат корректную метку `name`
- [ ] Логи не содержат unexpected warnings при деплое через Helm
- [ ] Auto-resolve работает корректно без `DEPHEALTH_NAME`

---

## Примечания

- Исходные коды SDK (`topologymetrics/sdk-go`) **не модифицируются**
- Функции `parseOwnerName`/`resolveOwnerFromHostname` дублируются в обоих модулях
  (модули — независимые Go-проекты без shared dependencies)
- Env-переменная `DEPHEALTH_NAME` — стандартная из SDK (без префикса модуля)
- Phase 1 и Phase 2 могут выполняться параллельно
