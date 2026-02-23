# План разработки: Разделение тестовой инфраструктуры на три Helm chart

## Метаданные

- **Версия плана**: 2.0.0
- **Дата создания**: 2026-02-23
- **Последнее обновление**: 2026-02-23
- **Статус**: Pending
- **Предшественник**: `plans/test-infrastructure-v1.0.0.md` (v1.0.0, Completed)

---

## История версий

- **v2.0.0** (2026-02-23): Начальная версия плана — разделение monolithic chart на три

---

## Текущий статус

- **Активная фаза**: Phase A
- **Активный подпункт**: A.1
- **Последнее обновление**: 2026-02-23
- **Примечание**: Миграция из monolithic artsore-test в три chart

---

## Оглавление

- [ ] [Phase A: artsore-infra chart (PG + KC)](#phase-a-artsore-infra-chart-pg--kc)
- [ ] [Phase B: artsore-se chart (6 SE)](#phase-b-artsore-se-chart-6-se)
- [ ] [Phase C: artsore-apps chart (AM)](#phase-c-artsore-apps-chart-am)
- [ ] [Phase D: Makefile и Init Job](#phase-d-makefile-и-init-job)
- [ ] [Phase E: Валидация](#phase-e-валидация)
- [ ] [Phase F: Очистка](#phase-f-очистка)

---

## Контекст

### Проблема

Текущий Helm chart `artsore-test` — монолитный. Все компоненты (PG, KC, AM, 6 SE, Init Job) деплоятся и удаляются вместе. Это не позволяет:

- Держать инфраструктуру (PG + KC) постоянно работающей
- Держать набор SE постоянно работающим для тестирования Ingester/Query
- Обновлять AM независимо от инфраструктуры
- Тестировать будущие модули (Ingester, Query) с общей инфрой

### Решение

Разделить на три независимых Helm chart с отдельным lifecycle:

```
tests/helm/
├── artsore-infra/     — PG + KC (постоянный слой 1)
├── artsore-se/        — 6 SE всех типов (постоянный слой 2)
├── artsore-apps/      — AM (+ будущие модули)
└── artsore-test/      — УДАЛИТЬ после миграции
```

### Архитектурные решения

**Namespace**: Все три chart деплоятся в один namespace `artsore-test`.

**Cross-namespace TLS**: Certificate включает FQDN `*.artsore-test.svc.cluster.local` для всех сервисов, чтобы модули из других namespace (ingester-test, query-test) могли обращаться к инфре по FQDN.

**AM = приложение**: AM деплоится отдельно от PG+KC. При разработке AM можно обновить его не трогая инфру. Будущие Ingester/Query используют общий AM из artsore-test.

**Init Job = отдельная команда**: `make init-data` вместо Helm post-install hook. Может быть запущен повторно.

**Helm release имена**: `artsore-infra`, `artsore-se`, `artsore-apps` — три отдельных release в одном namespace.

**Certificate ownership**: Certificate создаётся в artsore-infra (первый chart). artsore-se и artsore-apps используют TLS secret по имени (`artsore-test-tls`).

**Realm ConfigMap ownership**: Создаётся в artsore-infra. Keycloak subchart маунтит его для import.

**Будущие модули**: Ingester/Query — chart-ы в `src/<module>/tests/helm/`, деплоятся в свои namespace (ingester-test, query-test), используют сервисы из artsore-test через FQDN.

### Зависимости между chart-ами

```
artsore-infra (PG + KC)        — Helm release #1
    ↓ (PG, KC, TLS secret)
artsore-se (6 SE)              — Helm release #2, нужен KC (JWKS), cert (TLS)
    ↓
artsore-apps (AM)              — Helm release #3, нужен PG, KC, cert
    ↓
make init-data (Job)           — отдельный kubectl apply, нужен KC, AM, все SE
```

### Структура файлов (целевая)

```
tests/
├── Makefile
├── scripts/
│   └── lib.sh
└── helm/
    ├── artsore-infra/
    │   ├── Chart.yaml            — Bitnami Keycloak dependency
    │   ├── values.yaml           — PG + KC config
    │   ├── files/
    │   │   └── artsore-realm.json
    │   └── templates/
    │       ├── _helpers.tpl
    │       ├── namespace.yaml
    │       ├── certificate.yaml
    │       ├── postgresql.yaml
    │       └── realm-configmap.yaml
    ├── artsore-se/
    │   ├── Chart.yaml            — no dependencies
    │   ├── values.yaml           — SE instances config
    │   └── templates/
    │       ├── _helpers.tpl
    │       ├── se-replicated.yaml
    │       └── se-standalone.yaml
    ├── artsore-apps/
    │   ├── Chart.yaml            — no dependencies
    │   ├── values.yaml           — AM config
    │   └── templates/
    │       ├── _helpers.tpl
    │       └── admin-module.yaml
    └── init-job/
        ├── job.yaml              — Job manifest (kubectl apply)
        ├── lib.sh                — → symlink на tests/scripts/lib.sh
        └── init-data.sh          — скрипт инициализации
```

---

## Phase A: artsore-infra chart (PG + KC)

**Dependencies**: None
**Status**: Pending

### Описание

Создание Helm chart `artsore-infra` с PostgreSQL и Keycloak (Bitnami subchart). Это базовый слой инфраструктуры — деплоится первым, живёт постоянно.

Chart создаёт: Namespace, Certificate, PostgreSQL (ConfigMap + PVC + Deployment + Service), Realm ConfigMap. Keycloak деплоится через Bitnami subchart.

### Подпункты

- [ ] **A.1 Создать `Chart.yaml` и `values.yaml`**
  - **Dependencies**: None
  - **Description**: Chart.yaml с Bitnami Keycloak dependency. values.yaml с конфигурацией:
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
    - `tests/helm/artsore-test/values.yaml` — исходные values (секции namespace, tls, postgresql, keycloak, storageClass)

- [ ] **A.2 Создать templates**
  - **Dependencies**: A.1
  - **Description**: Перенести и адаптировать templates из artsore-test:
    - `_helpers.tpl` — только helpers для infra: labels, keycloakHttpUrl, keycloakHttpsUrl, pg.selectorLabels. Убрать AM/SE helpers
    - `namespace.yaml` — без изменений
    - `certificate.yaml` — все dnsNames (PG, KC, AM, SE) + FQDN-формат `*.artsore-test.svc.cluster.local`. Certificate создаёт один общий TLS secret для всех chart-ов
    - `postgresql.yaml` — без изменений (ConfigMap initdb + PVC + Deployment + Service)
    - `realm-configmap.yaml` — без изменений
  - **Creates**:
    - `tests/helm/artsore-infra/templates/_helpers.tpl`
    - `tests/helm/artsore-infra/templates/namespace.yaml`
    - `tests/helm/artsore-infra/templates/certificate.yaml`
    - `tests/helm/artsore-infra/templates/postgresql.yaml`
    - `tests/helm/artsore-infra/templates/realm-configmap.yaml`
  - **Links**:
    - `tests/helm/artsore-test/templates/` — исходные templates

- [ ] **A.3 Lint и template**
  - **Dependencies**: A.2
  - **Description**: `helm lint tests/helm/artsore-infra/` + `helm template artsore-infra tests/helm/artsore-infra/` — проверить что все ресурсы рендерятся без ошибок. Для этого необходимо скопировать realm.json в `files/` и выполнить `helm dependency update`
  - **Creates**: N/A
  - **Links**: N/A

### Критерии завершения Phase A

- [ ] Все подпункты завершены (A.1, A.2, A.3)
- [ ] `helm lint` проходит без ошибок
- [ ] `helm template` рендерит: Namespace, Certificate, PG ConfigMap, PG PVC, PG Deployment, PG Service, Realm ConfigMap + Keycloak resources
- [ ] Certificate включает dnsNames для ВСЕХ сервисов (PG, KC, AM, SE) + FQDN формат

---

## Phase B: artsore-se chart (6 SE)

**Dependencies**: Phase A
**Status**: Pending

### Описание

Создание Helm chart `artsore-se` с набором Storage Elements всех типов. Деплоится вторым, после infra. Зависит от TLS secret и Keycloak (JWKS URL).

Chart создаёт: 2 StatefulSet (replicated SE) + 4 Deployment (standalone SE) с PVC и Service.

### Подпункты

- [ ] **B.1 Создать `Chart.yaml` и `values.yaml`**
  - **Dependencies**: None
  - **Description**: Chart.yaml без dependencies (не subchart). values.yaml с конфигурацией:
    - `namespace: artsore-test` — тот же namespace что и infra
    - `registry`, `seImage`, `seTag`, `imagePullPolicy`
    - `tls:` secretName (ссылка на secret из artsore-infra)
    - `keycloak:` realm, serviceName (для построения JWKS URL)
    - `infraReleaseName: artsore-infra` — имя Helm release инфры (для формирования имени KC service: `artsore-infra-keycloak`)
    - `seCommon:` port, logLevel, gcInterval, reconcileInterval, maxFileSize, dephealth, resources
    - `storageClass: nfs-client`
    - `replicatedInstances:` se-edit-1, se-edit-2
    - `standaloneInstances:` se-rw-1, se-rw-2, se-ro, se-ar
  - **Creates**:
    - `tests/helm/artsore-se/Chart.yaml`
    - `tests/helm/artsore-se/values.yaml`
  - **Links**:
    - `tests/helm/artsore-test/values.yaml` — исходные values (секции seCommon, replicatedInstances, standaloneInstances)

- [ ] **B.2 Создать templates**
  - **Dependencies**: B.1
  - **Description**: Перенести и адаптировать templates из artsore-test:
    - `_helpers.tpl` — helpers для SE: labels, seImage, keycloakHttpsUrl (через infraReleaseName), jwksUrl, se.selectorLabels
    - `se-replicated.yaml` — адаптировать include paths (`artsore-se.*` вместо `artsore-test.*`)
    - `se-standalone.yaml` — аналогично
  - **Creates**:
    - `tests/helm/artsore-se/templates/_helpers.tpl`
    - `tests/helm/artsore-se/templates/se-replicated.yaml`
    - `tests/helm/artsore-se/templates/se-standalone.yaml`
  - **Links**:
    - `tests/helm/artsore-test/templates/se-replicated.yaml` — исходный template
    - `tests/helm/artsore-test/templates/se-standalone.yaml` — исходный template

- [ ] **B.3 Lint и template**
  - **Dependencies**: B.2
  - **Description**: `helm lint tests/helm/artsore-se/` + `helm template artsore-se tests/helm/artsore-se/` — проверить что все SE ресурсы рендерятся корректно
  - **Creates**: N/A
  - **Links**: N/A

### Критерии завершения Phase B

- [ ] Все подпункты завершены (B.1, B.2, B.3)
- [ ] `helm lint` проходит без ошибок
- [ ] `helm template` рендерит: 2 StatefulSet (se-edit-1/2) + 4 Deployment (se-rw-1/2, se-ro, se-ar) + PVC + Services
- [ ] SE используют TLS secret из artsore-infra (`artsore-test-tls`)
- [ ] JWKS URL указывает на Keycloak из artsore-infra (через `infraReleaseName`)

---

## Phase C: artsore-apps chart (AM)

**Dependencies**: Phase A
**Status**: Pending

### Описание

Создание Helm chart `artsore-apps` с Admin Module. Деплоится третьим, после infra. Зависит от PG, KC и TLS secret из artsore-infra.

Chart создаёт: AM Deployment (с init containers wait-for-pg, wait-for-kc) + AM Service.

### Подпункты

- [ ] **C.1 Создать `Chart.yaml` и `values.yaml`**
  - **Dependencies**: None
  - **Description**: Chart.yaml без dependencies. values.yaml с конфигурацией:
    - `namespace: artsore-test`
    - `registry`, `amImage`, `amTag`, `imagePullPolicy`
    - `tls:` secretName (ссылка на secret из artsore-infra)
    - `infraReleaseName: artsore-infra` — для формирования имён PG/KC service
    - `adminModule:` port, logLevel, keycloak settings, JWT claims, RBAC groups, sync intervals, dephealth, resources
  - **Creates**:
    - `tests/helm/artsore-apps/Chart.yaml`
    - `tests/helm/artsore-apps/values.yaml`
  - **Links**:
    - `tests/helm/artsore-test/values.yaml` — исходные values (секция adminModule)

- [ ] **C.2 Создать templates**
  - **Dependencies**: C.1
  - **Description**: Перенести и адаптировать templates из artsore-test:
    - `_helpers.tpl` — helpers для AM: labels, amImage, keycloakHttpUrl (через infraReleaseName), adminModuleUrl, am.selectorLabels
    - `admin-module.yaml` — адаптировать include paths, PG/KC service names (через infraReleaseName). Init containers wait-for-pg и wait-for-kc ссылаются на сервисы из artsore-infra
  - **Creates**:
    - `tests/helm/artsore-apps/templates/_helpers.tpl`
    - `tests/helm/artsore-apps/templates/admin-module.yaml`
  - **Links**:
    - `tests/helm/artsore-test/templates/admin-module.yaml` — исходный template

- [ ] **C.3 Lint и template**
  - **Dependencies**: C.2
  - **Description**: `helm lint tests/helm/artsore-apps/` + `helm template artsore-apps tests/helm/artsore-apps/` — проверить что AM ресурсы рендерятся корректно
  - **Creates**: N/A
  - **Links**: N/A

### Критерии завершения Phase C

- [ ] Все подпункты завершены (C.1, C.2, C.3)
- [ ] `helm lint` проходит без ошибок
- [ ] `helm template` рендерит: AM Deployment (с init containers) + AM Service
- [ ] AM использует PG/KC сервисы из artsore-infra (через infraReleaseName)
- [ ] AM использует TLS secret из artsore-infra (`artsore-test-tls`)

---

## Phase D: Makefile и Init Job

**Dependencies**: Phase A, Phase B, Phase C
**Status**: Pending

### Описание

Обновление Makefile для работы с тремя chart-ами: поэтапный деплой (infra → se → apps), композитный `test-env-up`, и вынос Init Job в отдельный `kubectl apply`.

### Подпункты

- [ ] **D.1 Вынести Init Job в standalone manifest**
  - **Dependencies**: None
  - **Description**: Извлечь Init Job из Helm hooks в standalone YAML manifests для запуска через `kubectl apply`. Создать:
    - `tests/helm/init-job/job.yaml` — Job + ConfigMaps (lib.sh + init-data.sh) в одном файле. Все переменные (URLs, credentials) задаются через `sed` или `envsubst` при запуске из Makefile
    - Убрать Helm hook annotations
    - Сохранить ту же логику: wait KC → get token → wait AM → wait SE → upload files → transition modes
  - **Creates**:
    - `tests/helm/init-job/job.yaml`
  - **Links**:
    - `tests/helm/artsore-test/templates/init-job.yaml` — исходный template

- [ ] **D.2 Обновить Makefile**
  - **Dependencies**: D.1
  - **Description**: Переписать Makefile для работы с тремя chart-ами:
    - **Переменные**: INFRA_CHART, SE_CHART, APPS_CHART, INFRA_RELEASE, SE_RELEASE, APPS_RELEASE
    - **Новые targets**:
      - `infra-up` — copy-realm + helm-dep-update + helm upgrade --install artsore-infra
      - `infra-down` — helm uninstall artsore-infra
      - `se-up` — helm upgrade --install artsore-se
      - `se-down` — helm uninstall artsore-se
      - `apps-up` — helm upgrade --install artsore-apps
      - `apps-down` — helm uninstall artsore-apps
      - `init-data` — kubectl apply -f init-job/ + kubectl wait job
      - `init-data-clean` — kubectl delete job artsore-test-init
      - `test-env-up` — infra-up → se-up → apps-up (последовательно, с ожиданием ready)
      - `test-env-down` — apps-down → se-down → infra-down → delete namespace
    - **Port-forward**: разделить на группы:
      - `port-forward-infra` — PG + KC
      - `port-forward-se` — все SE
      - `port-forward-apps` — AM
      - `port-forward-start` — все (infra + se + apps)
    - **Ожидание ready между chart-ами**:
      - После `infra-up`: wait for PG pod ready + KC pod ready
      - После `se-up`: wait for all SE pods ready
      - После `apps-up`: wait for AM pod ready
    - Сохранить обратную совместимость: `test-env-up` и `test-env-down` работают как раньше
  - **Creates**:
    - `tests/Makefile` (перезаписать)
  - **Links**:
    - `tests/Makefile` — текущий Makefile

### Критерии завершения Phase D

- [ ] Все подпункты завершены (D.1, D.2)
- [ ] `make help` показывает все новые targets
- [ ] `make infra-up`, `make se-up`, `make apps-up` работают по отдельности
- [ ] `make test-env-up` разворачивает всё одной командой
- [ ] `make init-data` запускает Init Job отдельно
- [ ] `make test-env-down` корректно удаляет все три release

---

## Phase E: Валидация

**Dependencies**: Phase D
**Status**: Pending

### Описание

Полная проверка работоспособности новой инфраструктуры в Kubernetes: поэтапный деплой, проверка всех компонентов, проверка независимого lifecycle.

### Подпункты

- [ ] **E.1 Lint и template всех chart-ов**
  - **Dependencies**: None
  - **Description**: `helm lint` + `helm template` для artsore-infra, artsore-se, artsore-apps. Подсчитать количество ресурсов, убедиться что общее число совпадает с исходным artsore-test (49 ресурсов)
  - **Creates**: N/A
  - **Links**: N/A

- [ ] **E.2 Деплой и проверка инфраструктуры**
  - **Dependencies**: E.1
  - **Description**: `make infra-up` → проверки:
    - PG pod Running, имеет 2 БД (artsore + keycloak)
    - KC pod Running, realm imported (curl realm info)
    - Certificate создан, TLS secret exists
  - **Creates**: N/A
  - **Links**: N/A

- [ ] **E.3 Деплой и проверка SE**
  - **Dependencies**: E.2
  - **Description**: `make se-up` → проверки:
    - Все 6 SE pods Running (4 standalone + 2 StatefulSet × 2 replicas = 8 pods)
    - SE отвечают на health probes
    - SE получают JWKS от Keycloak (проверить логи: JWKS fetched)
  - **Creates**: N/A
  - **Links**: N/A

- [ ] **E.4 Деплой и проверка AM**
  - **Dependencies**: E.2
  - **Description**: `make apps-up` → проверки:
    - AM pod Running (init containers wait-for-pg и wait-for-kc завершились)
    - AM health/ready = ok
    - AM подключён к PG (проверить логи: migrations applied)
    - AM получает токены из KC
  - **Creates**: N/A
  - **Links**: N/A

- [ ] **E.5 Init Job и end-to-end**
  - **Dependencies**: E.3, E.4
  - **Description**: `make init-data` → проверки:
    - Job Complete
    - Файлы загружены в se-ro и se-ar
    - se-ro в mode `ro`
    - se-ar в mode `ar`
    - Токен из KC принимается SE (JWKS валидация end-to-end)
  - **Creates**: N/A
  - **Links**: N/A

- [ ] **E.6 Проверка независимого lifecycle**
  - **Dependencies**: E.5
  - **Description**: Проверить что слои можно обновлять независимо:
    - `make apps-down` → infra и SE продолжают работать
    - `make apps-up` → AM поднимается и подключается к PG/KC
    - `make se-down` → infra продолжает работать
    - `make se-up` → SE поднимаются, получают JWKS
  - **Creates**: N/A
  - **Links**: N/A

### Критерии завершения Phase E

- [ ] Все подпункты завершены (E.1 — E.6)
- [ ] Все компоненты Running в Kubernetes
- [ ] Init Job Complete
- [ ] Токены работают end-to-end
- [ ] Независимый lifecycle подтверждён (apps-down/up не затрагивает infra/se)

---

## Phase F: Очистка

**Dependencies**: Phase E
**Status**: Pending

### Описание

Удаление старого монолитного chart `artsore-test` и обновление документации.

### Подпункты

- [ ] **F.1 Удалить старый chart и обновить план v1.0.0**
  - **Dependencies**: None
  - **Description**:
    - Удалить `tests/helm/artsore-test/` (кроме `charts/` и `Chart.lock` — они в .gitignore)
    - Обновить статус в `plans/test-infrastructure-v1.0.0.md` → Completed, добавить ссылку на v2.0.0
    - Перенести `plans/test-infrastructure-v1.0.0.md` в `plans/archive/`
  - **Creates**: N/A
  - **Links**:
    - `plans/test-infrastructure-v1.0.0.md`

- [ ] **F.2 Обновить CLAUDE.md**
  - **Dependencies**: F.1
  - **Description**: Обновить секцию тестирования в CLAUDE.md — описать три chart-а и новые Makefile targets
  - **Creates**: N/A
  - **Links**:
    - `CLAUDE.md`

### Критерии завершения Phase F

- [ ] Все подпункты завершены (F.1, F.2)
- [ ] `tests/helm/artsore-test/` удалён
- [ ] План v1.0.0 в архиве
- [ ] CLAUDE.md актуален

---

## Ключевые файлы-источники

| Файл-источник | Назначение |
|--------------|-----------|
| `tests/helm/artsore-test/Chart.yaml` | Исходный Chart (Bitnami KC dependency) |
| `tests/helm/artsore-test/values.yaml` | Исходные values (все секции) |
| `tests/helm/artsore-test/templates/_helpers.tpl` | Template helpers (split по chart-ам) |
| `tests/helm/artsore-test/templates/namespace.yaml` | → artsore-infra |
| `tests/helm/artsore-test/templates/certificate.yaml` | → artsore-infra (+ FQDN dnsNames) |
| `tests/helm/artsore-test/templates/postgresql.yaml` | → artsore-infra |
| `tests/helm/artsore-test/templates/realm-configmap.yaml` | → artsore-infra |
| `tests/helm/artsore-test/templates/admin-module.yaml` | → artsore-apps |
| `tests/helm/artsore-test/templates/se-replicated.yaml` | → artsore-se |
| `tests/helm/artsore-test/templates/se-standalone.yaml` | → artsore-se |
| `tests/helm/artsore-test/templates/init-job.yaml` | → tests/helm/init-job/ (standalone) |
| `tests/Makefile` | Обновить для трёх chart-ов |

---

## Примечания

- **infraReleaseName**: artsore-se и artsore-apps используют параметр `infraReleaseName` для формирования имён сервисов KC и PG. Bitnami KC создаёт service с именем `<release>-keycloak`, поэтому: `artsore-infra-keycloak.artsore-test.svc.cluster.local`
- **Namespace**: НЕ создавать namespace в artsore-se и artsore-apps (уже создан в artsore-infra). Использовать `--namespace artsore-test` при `helm install`
- **TLS secret**: Один Certificate в artsore-infra создаёт secret `artsore-test-tls`. Все chart-ы используют его по имени. Certificate включает dnsNames для ВСЕХ сервисов из всех chart-ов
- **Init Job**: Вынесен из Helm hooks в standalone manifest. Запускается через `make init-data`. Может быть перезапущен через `make init-data-clean && make init-data`
- **Порядок деплоя**: infra → (se + apps параллельно или последовательно) → init-data. В `make test-env-up` — последовательно для надёжности
- **Phase B и C независимы**: artsore-se и artsore-apps оба зависят только от artsore-infra, но не друг от друга. Их можно деплоить параллельно
