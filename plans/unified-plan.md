# План разработки: Тестовая инфраструктура + Admin Module Phase 6

## Метаданные

- **Версия плана**: 1.0.0
- **Дата создания**: 2026-02-23
- **Последнее обновление**: 2026-02-23
- **Статус**: In Progress
- **Объединяет**:
  - `plans/test-infrastructure-v2.0.0.md` (Phase A-F) → Phase 1-5, 8
  - `plans/admin-module.md` (Phase 6.2, 6.4, 6.5) → Phase 6-8
- **Архивировано**:
  - `plans/test-infrastructure-v1.0.0.md` → Phase A-C done, Phase D superseded
  - `plans/test-infrastructure-v2.0.0.md` → merged into this plan
  - `plans/admin-module.md` → Phase 1-5 done, Phase 6 merged into this plan

---

## История версий

- **v1.0.0** (2026-02-23): Начальная версия — объединение трёх планов

---

## Текущий статус

- **Активная фаза**: Phase 5
- **Активный подпункт**: 5.1
- **Последнее обновление**: 2026-02-23
- **Примечание**: Phase 1 завершена

---

## Оглавление

**Инфраструктура (приоритет 1):**

- [x] [Phase 1: artsore-infra chart (PG + KC)](#phase-1-artsore-infra-chart-pg--kc)
- [x] [Phase 2: artsore-se chart (6 SE)](#phase-2-artsore-se-chart-6-se)
- [x] [Phase 3: artsore-apps chart (AM)](#phase-3-artsore-apps-chart-am)
- [x] [Phase 4: Makefile и Init Job](#phase-4-makefile-и-init-job)
- [ ] [Phase 5: Валидация инфраструктуры](#phase-5-валидация-инфраструктуры)

**Admin Module (приоритет 2):**

- [ ] [Phase 6: Production Helm chart для AM](#phase-6-production-helm-chart-для-am)
- [ ] [Phase 7: Интеграционные тесты AM](#phase-7-интеграционные-тесты-am)
- [ ] [Phase 8: Финализация и очистка](#phase-8-финализация-и-очистка)

---

## Контекст

### Что уже сделано

**Тестовая инфраструктура (test-infrastructure v1.0.0):**
- Keycloak realm `artsore` (`deploy/keycloak/artsore-realm.json`) — клиенты, scopes, роли, группы, пользователи
- Monolithic Helm chart `artsore-test` (`tests/helm/artsore-test/`) — PG + KC + AM + 6 SE
- Makefile (`tests/Makefile`) и библиотека утилит (`tests/scripts/lib.sh`)

**Admin Module (Phase 1-5):**
- Полностью функциональный код: конфигурация, DB, RBAC, JWT, Keycloak клиент, SE клиент
- 29 API endpoints реализованы
- Фоновые задачи: storage sync, SA sync, topologymetrics
- Docker-образ собирается и работает в docker-compose

### Что осталось

1. **Разделить monolithic chart** `artsore-test` → три chart (infra, se, apps) для независимого lifecycle
2. **Production Helm chart** для AM (Deployment, Service, HTTPRoute для API Gateway)
3. **Интеграционные тесты AM** (bash + curl, ~25-30 тестов)
4. **Деплой в K8s** — верификация полного стека

### Целевая архитектура тестовой среды

```
tests/helm/
├── artsore-infra/     — PG + KC (постоянный слой 1)
├── artsore-se/        — 6 SE всех типов (постоянный слой 2)
├── artsore-apps/      — AM (постоянный слой 3)
└── init-job/          — standalone Job (make init-data)
```

Все три chart деплоятся в один namespace `artsore-test`.

Будущие модули (Ingester, Query) деплоятся в свои namespace и используют сервисы из `artsore-test` через cross-namespace FQDN: `<svc>.artsore-test.svc.cluster.local`.

### Ключевые архитектурные решения

- **Certificate ownership**: artsore-infra создаёт один Certificate с dnsNames для ВСЕХ сервисов (PG, KC, AM, SE) + FQDN формат. Остальные chart-ы используют TLS secret по имени
- **infraReleaseName**: artsore-se и artsore-apps содержат параметр `infraReleaseName: artsore-infra` для формирования имён сервисов KC/PG (Bitnami KC создаёт service `<release>-keycloak`)
- **Init Job = отдельная команда**: `make init-data` вместо Helm hooks. Может быть перезапущен
- **Namespace**: artsore-infra создаёт namespace, остальные chart-ы используют `--namespace artsore-test`

---

## Phase 1: artsore-infra chart (PG + KC)

**Dependencies**: None
**Status**: Done
**Origin**: test-infrastructure-v2.0.0 Phase A

### Описание

Создание Helm chart `artsore-infra` с PostgreSQL и Keycloak (Bitnami subchart). Базовый слой инфраструктуры — деплоится первым, живёт постоянно.

Chart создаёт: Namespace, Certificate, PostgreSQL (ConfigMap + PVC + Deployment + Service), Realm ConfigMap. Keycloak — через Bitnami subchart.

### Подпункты

- [x] **1.1 Создать `Chart.yaml` и `values.yaml`**
  - **Dependencies**: None
  - **Description**: Chart.yaml с Bitnami Keycloak dependency. values.yaml:
    - `namespace: artsore-test`
    - `tls:` clusterIssuer, secretName
    - `postgresql:` image, port, user, password, database, dataSize, resources
    - `keycloak:` Bitnami subchart values (production: false, externalDatabase, tls, realm import)
    - `storageClass: nfs-client`
  - **Creates**:
    - `tests/helm/artsore-infra/Chart.yaml`
    - `tests/helm/artsore-infra/values.yaml`
  - **Links**:
    - `tests/helm/artsore-test/Chart.yaml` — исходный chart
    - `tests/helm/artsore-test/values.yaml` — секции namespace, tls, postgresql, keycloak, storageClass

- [x] **1.2 Создать templates**
  - **Dependencies**: 1.1
  - **Description**: Перенести и адаптировать из artsore-test:
    - `_helpers.tpl` — helpers для infra: labels, keycloakHttpUrl, keycloakHttpsUrl, pg.selectorLabels
    - `namespace.yaml` — без изменений
    - `certificate.yaml` — dnsNames для ВСЕХ сервисов (PG, KC, AM, SE) + FQDN `*.artsore-test.svc.cluster.local`
    - `postgresql.yaml` — ConfigMap initdb + PVC + Deployment + Service
    - `realm-configmap.yaml` — без изменений
  - **Creates**:
    - `tests/helm/artsore-infra/templates/_helpers.tpl`
    - `tests/helm/artsore-infra/templates/namespace.yaml`
    - `tests/helm/artsore-infra/templates/certificate.yaml`
    - `tests/helm/artsore-infra/templates/postgresql.yaml`
    - `tests/helm/artsore-infra/templates/realm-configmap.yaml`
  - **Links**:
    - `tests/helm/artsore-test/templates/` — исходные templates

- [x] **1.3 Lint и template**
  - **Dependencies**: 1.2
  - **Description**: Скопировать realm.json в `files/`, `helm dependency update`, `helm lint` + `helm template`
  - **Creates**: N/A
  - **Links**: N/A

### Критерии завершения Phase 1

- [x] Все подпункты завершены (1.1, 1.2, 1.3)
- [x] `helm lint` без ошибок
- [x] `helm template` рендерит: Namespace, Certificate, PG (ConfigMap+PVC+Deployment+Service), Realm ConfigMap + Keycloak resources
- [x] Certificate включает dnsNames для ВСЕХ сервисов + FQDN формат

---

## Phase 2: artsore-se chart (6 SE)

**Dependencies**: Phase 1
**Status**: Done
**Origin**: test-infrastructure-v2.0.0 Phase B

### Описание

Helm chart `artsore-se` с набором Storage Elements всех типов. Деплоится после infra. Зависит от TLS secret и Keycloak (JWKS URL).

### Подпункты

- [x] **2.1 Создать `Chart.yaml` и `values.yaml`**
  - **Dependencies**: None
  - **Description**: Chart.yaml без dependencies. values.yaml:
    - `namespace: artsore-test`
    - `registry`, `seImage`, `seTag`, `imagePullPolicy`
    - `tls:` secretName (ссылка на secret из artsore-infra)
    - `infraReleaseName: artsore-infra` — для формирования KC service name
    - `keycloak:` realm
    - `seCommon:` port, logLevel, gcInterval, reconcileInterval, maxFileSize, dephealth, resources
    - `storageClass: nfs-client`
    - `replicatedInstances:` se-edit-1, se-edit-2
    - `standaloneInstances:` se-rw-1, se-rw-2, se-ro, se-ar
  - **Creates**:
    - `tests/helm/artsore-se/Chart.yaml`
    - `tests/helm/artsore-se/values.yaml`
  - **Links**:
    - `tests/helm/artsore-test/values.yaml` — секции seCommon, replicatedInstances, standaloneInstances

- [x] **2.2 Создать templates**
  - **Dependencies**: 2.1
  - **Description**: Перенести и адаптировать:
    - `_helpers.tpl` — labels, seImage, keycloakHttpsUrl (через infraReleaseName), jwksUrl, se.selectorLabels
    - `se-replicated.yaml` — include paths `artsore-se.*`
    - `se-standalone.yaml` — аналогично
  - **Creates**:
    - `tests/helm/artsore-se/templates/_helpers.tpl`
    - `tests/helm/artsore-se/templates/se-replicated.yaml`
    - `tests/helm/artsore-se/templates/se-standalone.yaml`
  - **Links**:
    - `tests/helm/artsore-test/templates/se-replicated.yaml`
    - `tests/helm/artsore-test/templates/se-standalone.yaml`

- [x] **2.3 Lint и template**
  - **Dependencies**: 2.2
  - **Description**: `helm lint` + `helm template`
  - **Creates**: N/A
  - **Links**: N/A

### Критерии завершения Phase 2

- [x] Все подпункты завершены (2.1, 2.2, 2.3)
- [x] `helm lint` без ошибок
- [x] `helm template` рендерит: 2 StatefulSet + 4 Deployment + PVC + Services
- [x] TLS secret из artsore-infra, JWKS URL через infraReleaseName

---

## Phase 3: artsore-apps chart (AM)

**Dependencies**: Phase 1
**Status**: Done
**Origin**: test-infrastructure-v2.0.0 Phase C

### Описание

Helm chart `artsore-apps` с Admin Module для тестовой среды. Деплоится после infra. Зависит от PG, KC и TLS secret.

### Подпункты

- [x] **3.1 Создать `Chart.yaml` и `values.yaml`**
  - **Dependencies**: None
  - **Description**: Chart.yaml без dependencies. values.yaml:
    - `namespace: artsore-test`
    - `registry`, `amImage`, `amTag`, `imagePullPolicy`
    - `tls:` secretName
    - `infraReleaseName: artsore-infra`
    - `adminModule:` port, logLevel, keycloak settings, JWT claims, RBAC groups, sync intervals, dephealth, resources
  - **Creates**:
    - `tests/helm/artsore-apps/Chart.yaml`
    - `tests/helm/artsore-apps/values.yaml`
  - **Links**:
    - `tests/helm/artsore-test/values.yaml` — секция adminModule

- [x] **3.2 Создать templates**
  - **Dependencies**: 3.1
  - **Description**: Перенести и адаптировать:
    - `_helpers.tpl` — labels, amImage, keycloakHttpUrl (через infraReleaseName), adminModuleUrl, am.selectorLabels
    - `admin-module.yaml` — PG/KC service names через infraReleaseName, init containers wait-for-pg/kc
  - **Creates**:
    - `tests/helm/artsore-apps/templates/_helpers.tpl`
    - `tests/helm/artsore-apps/templates/admin-module.yaml`
  - **Links**:
    - `tests/helm/artsore-test/templates/admin-module.yaml`

- [x] **3.3 Lint и template**
  - **Dependencies**: 3.2
  - **Description**: `helm lint` + `helm template`
  - **Creates**: N/A
  - **Links**: N/A

### Критерии завершения Phase 3

- [x] Все подпункты завершены (3.1, 3.2, 3.3)
- [x] `helm lint` без ошибок
- [x] `helm template` рендерит: AM Deployment (init containers) + AM Service
- [x] PG/KC через infraReleaseName, TLS secret из artsore-infra

---

## Phase 4: Makefile и Init Job

**Dependencies**: Phase 1, Phase 2, Phase 3
**Status**: Done
**Origin**: test-infrastructure-v2.0.0 Phase D

### Описание

Обновление Makefile для трёх chart-ов и вынос Init Job в standalone manifest.

### Подпункты

- [x] **4.1 Вынести Init Job в standalone manifest**
  - **Dependencies**: None
  - **Description**: Извлечь Init Job из Helm hooks:
    - `tests/helm/init-job/job.yaml` — Job + ConfigMaps (lib.sh + init-data.sh)
    - Переменные через `envsubst` при запуске из Makefile
    - Убрать Helm hook annotations
    - Логика: wait KC → get token → wait AM → wait SE → upload files → transition modes
  - **Creates**:
    - `tests/helm/init-job/job.yaml`
  - **Links**:
    - `tests/helm/artsore-test/templates/init-job.yaml` — исходный template

- [x] **4.2 Обновить Makefile**
  - **Dependencies**: 4.1
  - **Description**: Переписать для трёх chart-ов:
    - **Переменные**: INFRA_CHART, SE_CHART, APPS_CHART + release names
    - **Targets**:
      - `infra-up` / `infra-down` — PG + KC
      - `se-up` / `se-down` — 6 SE
      - `apps-up` / `apps-down` — AM
      - `init-data` / `init-data-clean` — standalone Job
      - `test-env-up` — infra-up → se-up → apps-up (последовательно, с wait ready)
      - `test-env-down` — apps-down → se-down → infra-down → delete namespace
    - **Port-forward**: группы (infra/se/apps) + общий `port-forward-start`
    - **Wait ready**: между chart-ами ожидание pod ready
  - **Creates**:
    - `tests/Makefile` (перезаписать)
  - **Links**:
    - `tests/Makefile` — текущий

### Критерии завершения Phase 4

- [x] Все подпункты завершены (4.1, 4.2)
- [x] `make help` показывает все targets
- [ ] `make infra-up`, `make se-up`, `make apps-up` работают по отдельности
- [ ] `make test-env-up` разворачивает всё одной командой
- [ ] `make init-data` запускает Init Job отдельно

---

## Phase 5: Валидация инфраструктуры

**Dependencies**: Phase 4
**Status**: Pending
**Origin**: test-infrastructure-v2.0.0 Phase E

### Описание

Полная проверка в Kubernetes: поэтапный деплой, проверка всех компонентов, проверка независимого lifecycle.

### Подпункты

- [ ] **5.1 Lint и template всех chart-ов**
  - **Dependencies**: None
  - **Description**: `helm lint` + `helm template` для всех трёх chart-ов. Проверить количество ресурсов
  - **Creates**: N/A
  - **Links**: N/A

- [ ] **5.2 Деплой инфраструктуры**
  - **Dependencies**: 5.1
  - **Description**: `make infra-up` → PG Running (2 БД), KC Running (realm imported), Certificate + TLS secret
  - **Creates**: N/A
  - **Links**: N/A

- [ ] **5.3 Деплой SE**
  - **Dependencies**: 5.2
  - **Description**: `make se-up` → 8 SE pods Running (4 standalone + 2×2 replicated), health probes OK, JWKS fetched
  - **Creates**: N/A
  - **Links**: N/A

- [ ] **5.4 Деплой AM**
  - **Dependencies**: 5.2
  - **Description**: `make apps-up` → AM pod Running (init containers OK), health/ready OK, подключён к PG и KC
  - **Creates**: N/A
  - **Links**: N/A

- [ ] **5.5 Init Job и end-to-end**
  - **Dependencies**: 5.3, 5.4
  - **Description**: `make init-data` → Job Complete, файлы загружены, se-ro в mode `ro`, se-ar в mode `ar`, токены работают
  - **Creates**: N/A
  - **Links**: N/A

- [ ] **5.6 Проверка независимого lifecycle**
  - **Dependencies**: 5.5
  - **Description**: `make apps-down` → infra+SE работают → `make apps-up` → AM подключается. `make se-down` → infra работает → `make se-up` → SE поднимаются
  - **Creates**: N/A
  - **Links**: N/A

### Критерии завершения Phase 5

- [ ] Все подпункты завершены (5.1 — 5.6)
- [ ] Все компоненты Running
- [ ] Init Job Complete, токены end-to-end
- [ ] Независимый lifecycle подтверждён

---

## Phase 6: Production Helm chart для AM

**Dependencies**: Phase 5
**Status**: Pending
**Origin**: admin-module.md Phase 6.2

### Описание

Helm chart для production-деплоя Admin Module: Deployment, Service, HTTPRoute для Gateway API. Отдельный chart в `src/admin-module/charts/admin-module/`.

### Подпункты

- [ ] **6.1 Создать production Helm chart**
  - **Dependencies**: None
  - **Description**: Chart `src/admin-module/charts/admin-module/`:
    - **Chart.yaml**: name admin-module, version 0.1.0
    - **values.yaml**: image, replicas, resources, env vars (DB, KC, TLS, sync intervals)
    - **templates**:
      - `_helpers.tpl` — labels, image, selector labels
      - `deployment.yaml` — stateless Deployment, probes: HTTP GET /health/live, /health/ready
      - `service.yaml` — ClusterIP, port 8000
      - `httproute.yaml` — Gateway API HTTPRoute, маршрутизация /api/v1/* и /health/* с `artsore.kryukov.lan`
      - `configmap.yaml` — не-секретные env vars
      - `secret.yaml` — DB credentials, Keycloak credentials
    - `helm lint` проходит
  - **Creates**:
    - `src/admin-module/charts/admin-module/Chart.yaml`
    - `src/admin-module/charts/admin-module/values.yaml`
    - `src/admin-module/charts/admin-module/templates/_helpers.tpl`
    - `src/admin-module/charts/admin-module/templates/deployment.yaml`
    - `src/admin-module/charts/admin-module/templates/service.yaml`
    - `src/admin-module/charts/admin-module/templates/httproute.yaml`
    - `src/admin-module/charts/admin-module/templates/configmap.yaml`
    - `src/admin-module/charts/admin-module/templates/secret.yaml`
  - **Links**:
    - `src/storage-element/charts/storage-element/` — паттерн Helm chart
    - `docs/design/admin-module-design.md` (раздел 12. Deployment)

### Критерии завершения Phase 6

- [ ] Все подпункты завершены (6.1)
- [ ] `helm lint` проходит
- [ ] `helm template` рендерит все ресурсы
- [ ] HTTPRoute маршрутизирует на artsore.kryukov.lan

---

## Phase 7: Интеграционные тесты AM

**Dependencies**: Phase 5
**Status**: Pending
**Origin**: admin-module.md Phase 6.4

### Описание

Интеграционные тесты Admin Module — bash-скрипты с curl + jq. Тесты используют инфраструктуру из Phase 1-5 (artsore-test namespace).

### Подпункты

- [ ] **7.1 Создать тестовые скрипты**
  - **Dependencies**: None
  - **Description**: Тесты в `tests/scripts/` (используют port-forward к AM из artsore-test). Используют `tests/scripts/lib.sh`. ~25-30 тестовых сценариев:
    - **Smoke** (1-3): health live, health ready, metrics
    - **Admin auth** (4): GET /admin-auth/me → текущий пользователь, effective_role
    - **Admin users** (5-9): list, get, set role override, verify effective role, delete override
    - **Service accounts** (10-15): create SA, list, get, update scopes, rotate secret, delete
    - **Storage elements** (16-20): discover SE, register (+ full sync), list, update, manual sync
    - **Files** (21-24): register, list, update metadata, soft delete
    - **IdP** (25-26): get IdP status, force sync SA
    - **Errors** (27-30): без JWT→401, нет роли→403, SA без scope→403, конфликт→409
  - **Creates**:
    - `tests/scripts/test-am-smoke.sh`
    - `tests/scripts/test-am-admin-auth.sh`
    - `tests/scripts/test-am-admin-users.sh`
    - `tests/scripts/test-am-service-accounts.sh`
    - `tests/scripts/test-am-storage-elements.sh`
    - `tests/scripts/test-am-files.sh`
    - `tests/scripts/test-am-idp.sh`
    - `tests/scripts/test-am-errors.sh`
    - `tests/scripts/test-am-all.sh`
  - **Links**:
    - `src/storage-element/tests/scripts/` — паттерн тестовых скриптов
    - `docs/api-contracts/admin-module-openapi.yaml` — API контракт
    - `tests/scripts/lib.sh` — общая библиотека утилит

- [ ] **7.2 Добавить target test-am в Makefile**
  - **Dependencies**: 7.1
  - **Description**: Добавить `make test-am` в `tests/Makefile`. Требует port-forward-start
  - **Creates**:
    - `tests/Makefile` (дополнить)
  - **Links**: N/A

- [ ] **7.3 Запуск тестов и отладка**
  - **Dependencies**: 7.2
  - **Description**: `make port-forward-start && make test-am` — все тесты PASS. Отладка и исправление найденных проблем в AM или тестах
  - **Creates**: N/A
  - **Links**: N/A

### Критерии завершения Phase 7

- [ ] Все подпункты завершены (7.1, 7.2, 7.3)
- [ ] Все ~25-30 тестов проходят
- [ ] `make test-am` — zero failures
- [ ] JWT авторизация end-to-end (KC → AM)
- [ ] Sync SE → файловый реестр обновляется
- [ ] Sync SA → согласованность с Keycloak

---

## Phase 8: Финализация и очистка

**Dependencies**: Phase 6, Phase 7
**Status**: Pending
**Origin**: admin-module.md Phase 6.5 + test-infrastructure-v2.0.0 Phase F

### Описание

Финальная верификация, деплой через production chart, удаление старого monolithic chart, архивация старых планов, обновление документации.

### Подпункты

- [ ] **8.1 Деплой через production Helm chart**
  - **Dependencies**: None
  - **Description**: Деплой AM через production chart `src/admin-module/charts/admin-module/`:
    - Сборка Docker-образа, push в Harbor
    - `helm install` в namespace с настроенным PG + KC (или использовать artsore-test infra)
    - Проверка: все endpoints через API Gateway (`artsore.kryukov.lan`)
    - HTTPRoute корректно маршрутизирует
  - **Creates**: N/A
  - **Links**:
    - Harbor: `harbor.kryukov.lan/library/admin-module`
    - API Gateway: `artsore.kryukov.lan` → 192.168.218.180

- [ ] **8.2 Удалить старый chart и архивировать планы**
  - **Dependencies**: 8.1
  - **Description**:
    - Удалить `tests/helm/artsore-test/`
    - Перенести в `plans/archive/`:
      - `plans/test-infrastructure-v1.0.0.md`
      - `plans/admin-module.md`
    - Удалить `plans/test-infrastructure-v2.0.0.md` (содержание в unified-plan)
  - **Creates**: N/A
  - **Links**: N/A

- [ ] **8.3 Обновить документацию**
  - **Dependencies**: 8.2
  - **Description**:
    - Обновить `CLAUDE.md` — описать три chart-а, новые Makefile targets
    - Создать `src/admin-module/README.md` — инструкции по деплою AM
    - Перенести этот план в `plans/archive/` со статусом Completed
  - **Creates**:
    - `src/admin-module/README.md`
  - **Links**:
    - `CLAUDE.md`

### Критерии завершения Phase 8

- [ ] Все подпункты завершены (8.1, 8.2, 8.3)
- [ ] AM работает за API Gateway
- [ ] `tests/helm/artsore-test/` удалён
- [ ] Старые планы в архиве
- [ ] CLAUDE.md и README актуальны

---

## Зависимости между фазами

```
Phase 1 (artsore-infra) ─┬─→ Phase 2 (artsore-se) ──┐
                         └─→ Phase 3 (artsore-apps) ─┤
                                                      ↓
                                                 Phase 4 (Makefile + Init Job)
                                                      ↓
                                                 Phase 5 (Валидация инфры)
                                                   ↓     ↓
                                           Phase 6 (Prod chart)  Phase 7 (Тесты AM)
                                                   ↓     ↓
                                                 Phase 8 (Финализация)
```

## Рекомендуемый порядок AI-сессий

| Сессия | Фазы | Ветка | Описание |
|--------|------|-------|----------|
| A | 1.1, 1.2, 1.3 | `feature/test-infrastructure` | artsore-infra chart |
| B | 2.1, 2.2, 2.3 | `feature/test-infrastructure` | artsore-se chart |
| C | 3.1, 3.2, 3.3 | `feature/test-infrastructure` | artsore-apps chart |
| D | 4.1, 4.2 | `feature/test-infrastructure` | Makefile + Init Job |
| E | 5.1 — 5.6 | `feature/test-infrastructure` | Валидация в K8s |
| F | 6.1 | `feature/am-phase-6-helm` | Production Helm chart |
| G | 7.1, 7.2, 7.3 | `feature/am-phase-6-helm` | Интеграционные тесты |
| H | 8.1, 8.2, 8.3 | `feature/am-phase-6-helm` | Деплой + очистка |

## Ключевые файлы-источники

| Файл | Назначение |
|------|-----------|
| `tests/helm/artsore-test/` | Исходный monolithic chart (разделяется на 3) |
| `tests/Makefile` | Текущий Makefile (перезаписывается) |
| `tests/scripts/lib.sh` | Библиотека утилит (переиспользуется) |
| `deploy/keycloak/artsore-realm.json` | Keycloak realm (уже создан) |
| `src/admin-module/internal/config/config.go` | Все env vars AM |
| `src/admin-module/Dockerfile` | Docker-образ AM |
| `src/storage-element/charts/storage-element/` | Паттерн production Helm chart |
| `docs/api-contracts/admin-module-openapi.yaml` | API контракт v0.1.0 (29 endpoints) |
| `docs/design/admin-module-design.md` | Технический дизайн AM |

---

## Примечания

- **Phase 2 и 3 независимы** — оба зависят от Phase 1, но не друг от друга
- **Phase 6 и 7 независимы** — оба зависят от Phase 5, но не друг от друга
- **infraReleaseName**: `artsore-infra-keycloak.artsore-test.svc.cluster.local` — имя KC service
- **Тесты AM в `tests/scripts/`** (не в `src/admin-module/tests/scripts/`) — используют общую инфру
- **AM Phase 6.1 (Keycloak realm)** — уже выполнена в test-infra v1.0.0 Phase A, пропущена
- **AM Phase 6.3 (Тестовая среда)** — заменена Phase 1-5 этого плана (три chart вместо am-test)
