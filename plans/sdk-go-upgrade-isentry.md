# План разработки: Обновление sdk-go v0.8.0 и поддержка DEPHEALTH_ISENTRY

## Метаданные

- **Версия плана**: 1.0.0
- **Дата создания**: 2026-02-27
- **Последнее обновление**: 2026-02-27
- **Статус**: Pending

---

## История версий

- **v1.0.0** (2026-02-27): Начальная версия плана

---

## Текущий статус

- **Активная фаза**: Phase 5
- **Активный подпункт**: 5.1
- **Последнее обновление**: 2026-02-27

---

## Оглавление

- [x] [Phase 1: Обновление sdk-go и добавление IsEntry в Storage Element](#phase-1-обновление-sdk-go-и-добавление-isentry-в-storage-element)
- [x] [Phase 2: Обновление sdk-go и добавление IsEntry в Admin Module](#phase-2-обновление-sdk-go-и-добавление-isentry-в-admin-module)
- [x] [Phase 3: Обновление Helm charts модулей](#phase-3-обновление-helm-charts-модулей)
- [x] [Phase 4: Обновление тестовых Helm charts](#phase-4-обновление-тестовых-helm-charts)
- [ ] [Phase 5: Сборка, деплой и верификация](#phase-5-сборка-деплой-и-верификация)

---

## Phase 1: Обновление sdk-go и добавление IsEntry в Storage Element

**Dependencies**: None
**Status**: Done

### Описание

Обновить зависимость `github.com/BigKAA/topologymetrics/sdk-go` с v0.6.0 до v0.8.0 в Storage Element.
Добавить конфигурационный параметр `DephealthIsEntry` (bool, default: false) с загрузкой из `DEPHEALTH_ISENTRY`.
При `IsEntry = true` — добавлять `dephealth.WithLabel("isentry", "yes")` ко всем зависимостям.

### Подпункты

- [x] **1.1 Обновить go.mod / go.sum**
  - **Dependencies**: None
  - **Description**: Изменить версию `github.com/BigKAA/topologymetrics/sdk-go` с v0.6.0 на v0.8.0 в `go.mod`. Запустить `go mod tidy` для обновления `go.sum`. Проверить что новые транзитивные зависимости (go-ldap, ldap) не конфликтуют с существующими.
  - **Modifies**:
    - `src/storage-element/go.mod`
    - `src/storage-element/go.sum`

- [x] **1.2 Добавить DephealthIsEntry в Config**
  - **Dependencies**: None
  - **Description**: Добавить поле `DephealthIsEntry bool` в struct Config (рядом с другими Dephealth-полями, строка ~101). Добавить загрузку из env `DEPHEALTH_ISENTRY` через существующую функцию `getEnvBool("DEPHEALTH_ISENTRY", false)`. Допустимые значения: true/false, 1/0 (стандартный `strconv.ParseBool`).
  - **Modifies**:
    - `src/storage-element/internal/config/config.go` — struct Config + Load()

- [x] **1.3 Добавить isEntry в DephealthService**
  - **Dependencies**: 1.2
  - **Description**: Добавить параметр `isEntry bool` в конструкторы `NewDephealthService`, `NewDephealthServiceWithRegisterer` и `newDephealthService`. Внутри `newDephealthService`: если `isEntry == true`, добавлять `dephealth.WithLabel("isentry", "yes")` как DependencyOption к каждой зависимости (HTTP checker). Паттерн: по аналогии с uniproxy `buildDependencyOption()`.
  - **Modifies**:
    - `src/storage-element/internal/service/dephealth.go`

- [x] **1.4 Передать isEntry из main.go**
  - **Dependencies**: 1.2, 1.3
  - **Description**: Обновить вызов `service.NewDephealthService()` в `main.go` (строка ~204): добавить `cfg.DephealthIsEntry` как параметр. Добавить логирование при `isEntry=true`.
  - **Modifies**:
    - `src/storage-element/cmd/storage-element/main.go`

- [x] **1.5 Проверка компиляции**
  - **Dependencies**: 1.1, 1.4
  - **Description**: Запустить `go build ./...` в `src/storage-element/` для проверки что код компилируется без ошибок. Запустить `go vet ./...` для статического анализа.
  - **Creates**: N/A

### Критерии завершения Phase 1

- [x] Все подпункты завершены (1.1-1.5)
- [x] `go build ./...` — успешно
- [x] `go vet ./...` — без ошибок
- [x] sdk-go v0.8.0 в go.mod

---

## Phase 2: Обновление sdk-go и добавление IsEntry в Admin Module

**Dependencies**: None (может выполняться параллельно с Phase 1)
**Status**: Done

### Описание

Аналогичные изменения для Admin Module. Обновить sdk-go до v0.8.0, добавить `DephealthIsEntry` в Config,
пробросить в DephealthService с добавлением label `isentry=yes` ко всем зависимостям (PostgreSQL и Keycloak).

### Подпункты

- [x] **2.1 Обновить go.mod / go.sum**
  - **Dependencies**: None
  - **Description**: Изменить версию sdk-go с v0.6.0 на v0.8.0 в `go.mod`. Запустить `go mod tidy`.
  - **Modifies**:
    - `src/admin-module/go.mod`
    - `src/admin-module/go.sum`

- [x] **2.2 Добавить DephealthIsEntry в Config**
  - **Dependencies**: None
  - **Description**: Добавить поле `DephealthIsEntry bool` в struct Config (рядом с DephealthGroup, строка ~116). Добавить загрузку из `DEPHEALTH_ISENTRY` через `getEnvBool("DEPHEALTH_ISENTRY", false)` (после строки 392).
  - **Modifies**:
    - `src/admin-module/internal/config/config.go`

- [x] **2.3 Добавить isEntry в DephealthService**
  - **Dependencies**: 2.2
  - **Description**: Добавить параметр `isEntry bool` в конструкторы `NewDephealthService`, `NewDephealthServiceWithRegisterer` и `newDephealthService`. Если `isEntry == true` — добавлять `dephealth.WithLabel("isentry", "yes")` к обеим зависимостям (PostgreSQL AddDependency и HTTP "keycloak-jwks").
  - **Modifies**:
    - `src/admin-module/internal/service/dephealth.go`

- [x] **2.4 Передать isEntry из main.go**
  - **Dependencies**: 2.2, 2.3
  - **Description**: Обновить вызов `service.NewDephealthService()` в `main.go` (строка ~241): добавить `cfg.DephealthIsEntry`. Добавить логирование при `isEntry=true`.
  - **Modifies**:
    - `src/admin-module/cmd/admin-module/main.go`

- [x] **2.5 Проверка компиляции**
  - **Dependencies**: 2.1, 2.4
  - **Description**: Запустить `go build ./...` и `go vet ./...` в `src/admin-module/`.
  - **Creates**: N/A

### Критерии завершения Phase 2

- [x] Все подпункты завершены (2.1-2.5)
- [x] `go build ./...` — успешно
- [x] `go vet ./...` — без ошибок
- [x] sdk-go v0.8.0 в go.mod

---

## Phase 3: Обновление Helm charts модулей

**Dependencies**: Phase 1, Phase 2
**Status**: Done

### Описание

Добавить параметр `isEntry` в Helm charts обоих модулей (values.yaml + templates).
По умолчанию false. При `isEntry: true` — пробрасывать `DEPHEALTH_ISENTRY=true` в pod env.

### Подпункты

- [x] **3.1 Admin Module Helm chart**
  - **Dependencies**: None
  - **Description**:
    1. `values.yaml`: добавить `isEntry: false` в секцию `dephealth` (после `group`).
    2. `templates/configmap.yaml`: добавить условный проброс `DEPHEALTH_ISENTRY` в секции dephealth (после `AM_DEPHEALTH_GROUP`):
       ```yaml
       {{- if .Values.dephealth.isEntry }}
       DEPHEALTH_ISENTRY: "true"
       {{- end }}
       ```
  - **Modifies**:
    - `src/admin-module/charts/admin-module/values.yaml`
    - `src/admin-module/charts/admin-module/templates/configmap.yaml`

- [x] **3.2 Storage Element Helm chart**
  - **Dependencies**: None
  - **Description**:
    1. `values.yaml`: добавить `dephealthIsEntry: false` (после `dephealthDepName`, строка ~65).
    2. `templates/_helpers.tpl`: добавить условный проброс в `se.envVars` (после `SE_DEPHEALTH_DEP_NAME`):
       ```yaml
       {{- if .Values.dephealthIsEntry }}
       - name: DEPHEALTH_ISENTRY
         value: "true"
       {{- end }}
       ```
  - **Modifies**:
    - `src/storage-element/charts/storage-element/values.yaml`
    - `src/storage-element/charts/storage-element/templates/_helpers.tpl`

- [x] **3.3 Helm template validation**
  - **Dependencies**: 3.1, 3.2
  - **Description**: Запустить `helm template` для обоих charts и проверить что templates рендерятся корректно:
    - С `isEntry: false` (default) — `DEPHEALTH_ISENTRY` отсутствует в output
    - С `isEntry: true` — `DEPHEALTH_ISENTRY: "true"` присутствует
  - **Creates**: N/A

### Критерии завершения Phase 3

- [x] Все подпункты завершены (3.1-3.3)
- [x] `helm template` — без ошибок для обоих charts
- [x] При `isEntry: false` переменная не появляется в output
- [x] При `isEntry: true` переменная корректно пробрасывается

---

## Phase 4: Обновление тестовых Helm charts

**Dependencies**: Phase 3
**Status**: Done

### Описание

Добавить параметр `isEntry` в тестовые Helm charts (`tests/helm/artstore-apps`, `tests/helm/artstore-se`).
По умолчанию false — тестовые деплойменты не используют isEntry, но параметр доступен для включения.

### Подпункты

- [x] **4.1 artstore-apps (Admin Module)**
  - **Dependencies**: None
  - **Description**:
    1. `values.yaml`: добавить `dephealthIsEntry: false` в секцию `adminModule` (после `dephealthGroup`).
    2. `templates/admin-module.yaml`: добавить условный env (после `AM_DEPHEALTH_GROUP`):
       ```yaml
       {{- if .Values.adminModule.dephealthIsEntry }}
       - name: DEPHEALTH_ISENTRY
         value: "true"
       {{- end }}
       ```
  - **Modifies**:
    - `tests/helm/artstore-apps/values.yaml`
    - `tests/helm/artstore-apps/templates/admin-module.yaml`

- [x] **4.2 artstore-se (Storage Elements)**
  - **Dependencies**: None
  - **Description**:
    1. `values.yaml`: добавить `dephealthIsEntry: false` в секцию `seCommon` (после `dephealthDepName`).
    2. `templates/se-standalone.yaml`: добавить условный env (после `SE_DEPHEALTH_DEP_NAME`):
       ```yaml
       {{- if $.Values.seCommon.dephealthIsEntry }}
       - name: DEPHEALTH_ISENTRY
         value: "true"
       {{- end }}
       ```
    3. `templates/se-replicated.yaml`: аналогичное добавление (проверить наличие dephealth env-переменных).
  - **Modifies**:
    - `tests/helm/artstore-se/values.yaml`
    - `tests/helm/artstore-se/templates/se-standalone.yaml`
    - `tests/helm/artstore-se/templates/se-replicated.yaml`

- [x] **4.3 Helm template validation**
  - **Dependencies**: 4.1, 4.2
  - **Description**: Запустить `helm template` для тестовых charts и проверить корректность.
  - **Creates**: N/A

### Критерии завершения Phase 4

- [x] Все подпункты завершены (4.1-4.3)
- [x] `helm template` — без ошибок
- [x] Тестовые charts содержат `dephealthIsEntry` с default false

---

## Phase 5: Сборка, деплой и верификация

**Dependencies**: Phase 1, Phase 2, Phase 3, Phase 4
**Status**: Pending

### Описание

Сборка Docker образов обоих модулей с обновлённым sdk-go, деплой в тестовый кластер K8s,
верификация что метрики корректно экспортируются с/без лейбла isentry.

### Подпункты

- [ ] **5.1 Сборка Docker образов**
  - **Dependencies**: None
  - **Description**: Собрать Docker образы для обоих модулей:
    - `docker build -t harbor.kryukov.lan/library/storage-element:<tag> src/storage-element/`
    - `docker build -t harbor.kryukov.lan/library/admin-module:<tag> src/admin-module/`
    - Запушить в Harbor registry.
    - Обновить теги образов в тестовых values.yaml.
  - **Creates**: Docker images в Harbor

- [ ] **5.2 Деплой тестовой среды**
  - **Dependencies**: 5.1
  - **Description**: Деплой в тестовый кластер:
    1. `make test-env-up` (если среда не поднята)
    2. Обновить apps и se charts: `make apps-up se-up`
    3. Убедиться что все поды запустились без ошибок
  - **Creates**: Running pods в artstore-test namespace

- [ ] **5.3 Верификация метрик без isEntry**
  - **Dependencies**: 5.2
  - **Description**: Проверить что при `dephealthIsEntry: false` (default):
    1. Метрики `app_dependency_health` экспортируются на `/metrics`
    2. Лейбл `isentry` **отсутствует** в метриках
    3. Все зависимости healthy (PostgreSQL, Keycloak, Admin JWKS)
  - **Creates**: N/A

- [ ] **5.4 Верификация метрик с isEntry**
  - **Dependencies**: 5.3
  - **Description**: Включить isEntry для одного из модулей (например, AM):
    1. Задеплоить с `DEPHEALTH_ISENTRY=true` (через `helm upgrade --set`)
    2. Проверить что в метриках появился лейбл `isentry="yes"`
    3. Убедиться что все метрики содержат этот лейбл
  - **Creates**: N/A

- [ ] **5.5 Интеграционные тесты**
  - **Dependencies**: 5.2
  - **Description**: Запустить `make test-all` для проверки что обновление sdk-go не сломало существующую функциональность.
  - **Creates**: Test results

### Критерии завершения Phase 5

- [ ] Все подпункты завершены (5.1-5.5)
- [ ] Docker образы собраны и запушены
- [ ] Поды запускаются без ошибок
- [ ] Метрики корректно экспортируются
- [ ] isEntry=true добавляет лейбл `isentry="yes"` ко всем метрикам
- [ ] isEntry=false (default) — лейбл отсутствует
- [ ] Интеграционные тесты проходят

---

## Примечания

- **Обратная совместимость SDK**: API sdk-go v0.8.0 обратно-совместим с v0.6.0. Новые возможности (LDAP checker, dynamic endpoints) не требуют изменений в существующем коде. Транзитивные зависимости (go-ldap) добавляются автоматически через go.sum.
- **DEPHEALTH_ISENTRY — не часть SDK**: Реализуется на уровне приложения через `dephealth.WithLabel("isentry", "yes")`. Паттерн взят из проекта uniproxy.
- **Env-переменная без префикса модуля**: `DEPHEALTH_ISENTRY` (аналогично `DEPHEALTH_NAME`) — это SDK-уровневый параметр, общий для всех модулей.
- **Справочный проект**: `/Users/arturkryukov/Projects/personal/ai/topologymetrics/uniproxy` — пример интеграции isEntry.

---

**План готов к использованию.**
