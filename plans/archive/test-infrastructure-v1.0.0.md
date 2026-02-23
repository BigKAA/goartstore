# План разработки: Инфраструктура для тестирования Artsore (подготовка к Phase 6)

## Метаданные

- **Версия плана**: 1.0.0
- **Дата создания**: 2026-02-22
- **Последнее обновление**: 2026-02-22
- **Статус**: In Progress

---

## История версий

- **v1.0.0** (2026-02-22): Начальная версия плана
- **v1.0.0** (2026-02-23): Phase A завершена
- **v1.0.0** (2026-02-23): Phase B + C завершены, helm lint + template OK

---

## Текущий статус

- **Активная фаза**: Phase D
- **Активный подпункт**: D.1
- **Последнее обновление**: 2026-02-23
- **Примечание**: Phase A-C завершены. Ожидает валидацию в K8s (Phase D).

---

## Оглавление

- [x] [Phase A: Keycloak Realm](#phase-a-keycloak-realm)
- [x] [Phase B: Helm Chart](#phase-b-helm-chart)
- [x] [Phase C: Makefile и утилиты](#phase-c-makefile-и-утилиты)
- [ ] [Phase D: Валидация](#phase-d-валидация)

---

## Контекст

Перед Phase 6 (Helm chart + интеграционные тесты Admin Module) необходимо подготовить:

1. Общую инфраструктуру проекта (Keycloak realm для всех модулей)
2. Единую тестовую среду в Kubernetes с PostgreSQL + Keycloak + Admin Module + 6 SE

Сейчас тестовая среда существует только для SE (`src/storage-element/tests/helm/se-test/`) с JWKS Mock. Новая среда `artsore-test` — полная интеграция с реальным Keycloak.

### Структура файлов

```
artsore/
├── deploy/
│   └── keycloak/
│       ├── artsore-realm.json          — Keycloak realm (все модули)
│       └── README.md                   — Инструкция по импорту
└── tests/
    ├── Makefile                        — Оркестрация: build, deploy, test
    ├── scripts/
    │   └── lib.sh                      — Утилиты (Keycloak token, HTTP helpers)
    └── helm/
        └── artsore-test/
            ├── Chart.yaml              — keycloak subchart (Bitnami)
            ├── values.yaml             — Конфигурация всех компонентов
            ├── files/                  — Копия realm.json (Makefile копирует)
            └── templates/
                ├── _helpers.tpl
                ├── namespace.yaml
                ├── certificate.yaml
                ├── postgresql.yaml     — PG Deployment + initdb ConfigMap
                ├── admin-module.yaml   — AM Deployment + init containers
                ├── realm-configmap.yaml
                ├── se-replicated.yaml  — 2 StatefulSet (по паттерну se-test)
                ├── se-standalone.yaml  — 4 Deployment (по паттерну se-test)
                └── init-job.yaml       — Загрузка данных, mode transitions
```

### Архитектурные решения

**Keycloak: AM по HTTP, SE по HTTPS:**

- Bitnami Keycloak в dev mode выставляет HTTP (8080) и HTTPS (8443)
- AM использует `AM_KEYCLOAK_URL=http://keycloak:8080` — не нужен CA cert для KC
- SE используют `SE_JWKS_URL=https://keycloak:8443/realms/artsore/.../certs` — через HTTPS, CA cert из cert-manager
- AM получает `AM_SE_CA_CERT_PATH=/certs/ca.crt` для подключения к SE через HTTPS

**PostgreSQL: две базы:**

- `POSTGRES_DB=artsore` — основная БД для AM (создаётся автоматически)
- initdb-скрипт дополнительно создаёт БД `keycloak` для Bitnami KC

**Порядок старта:**

```
Certificate → PostgreSQL → [init containers wait-for-pg] → Keycloak + AM
                                                           → [AM wait-for-kc]
                                                           → SE (readiness probes)
                                                           → Init Job (post-install hook)
```

**Token для Init Job:**

- Client `artsore-test-init` в realm.json с Client Credentials flow
- Скрипт `get_token_from_keycloak()` вместо `get_token()` из se-test

---

## Phase A: Keycloak Realm

**Dependencies**: None
**Status**: Completed

### Описание

Создание Keycloak realm `artsore` с полной конфигурацией клиентов, scopes, ролей, групп и тестовых пользователей. Realm используется всеми модулями системы.

### Подпункты

- [x] **A.1 Создать `deploy/keycloak/artsore-realm.json`**
  - **Dependencies**: None
  - **Description**: Keycloak realm export с конфигурацией:
    - **Client Scopes**: `files:read`, `files:write`, `storage:read`, `storage:write`, `admin:read`, `admin:write`
    - **Clients**:
      - `artsore-admin-module` — confidential, Client Credentials, все scopes + realm-management roles
      - `artsore-ingester` — confidential, Client Credentials, files:\*, storage:read
      - `artsore-query` — confidential, Client Credentials, files:read, storage:read
      - `artsore-admin-ui` — public, Auth Code + PKCE, openid, profile, email
      - `artsore-test-init` — confidential, Client Credentials, files:\*, storage:\*
    - **Секреты** (тестовые): `admin-module-test-secret`, `test-init-secret` и т.д.
    - **Группы → Роли**: `artsore-admins` → `admin`, `artsore-viewers` → `readonly`
    - **Protocol Mappers**: `groups` → `oidc-group-membership-mapper` (claim `groups` в access token)
    - **Тестовые пользователи**: `admin`/`admin` (artsore-admins), `viewer`/`viewer` (artsore-viewers)
    - **Service Account Roles**: `artsore-admin-module` SA получает realm-management roles: `view-users`, `manage-clients`, `view-clients`, `manage-users`, `view-realm`
  - **Creates**:
    - `deploy/keycloak/artsore-realm.json`
  - **Links**:
    - `src/admin-module/docker-compose.yaml` — паттерн KC + PG конфигурации

- [x] **A.2 Создать `deploy/keycloak/README.md`**
  - **Dependencies**: A.1
  - **Description**: Инструкция по импорту realm: ручной через KC Admin UI и автоматический через `--import-realm`
  - **Creates**:
    - `deploy/keycloak/README.md`
  - **Links**: N/A

### Критерии завершения Phase A

- [x] Все подпункты завершены (A.1, A.2)
- [x] JSON валиден (проверка через `jq`)
- [x] Документация содержит оба способа импорта

---

## Phase B: Helm Chart

**Dependencies**: Phase A
**Status**: Completed

### Описание

Создание Helm chart `artsore-test` с полной тестовой средой: PostgreSQL, Keycloak (Bitnami subchart), Admin Module, 6 Storage Elements.

### Подпункты

- [x] **B.1 Создать `Chart.yaml`**
  - **Dependencies**: None
  - **Description**: Chart с Bitnami Keycloak subchart dependency
    ```yaml
    apiVersion: v2
    name: artsore-test
    description: Тестовая среда Artsore — PG + KC + AM + 6 SE
    version: 0.1.0
    dependencies:
      - name: keycloak
        version: "25.3.2"
        repository: "oci://registry-1.docker.io/bitnamicharts"
    ```
  - **Creates**:
    - `tests/helm/artsore-test/Chart.yaml`
  - **Links**: N/A

- [x] **B.2 Создать `values.yaml`**
  - **Dependencies**: None
  - **Description**: Конфигурация всех компонентов:
    - `namespace: artsore-test`
    - `registry: harbor.kryukov.lan/library`
    - `amImage/amTag`, `seImage/seTag`
    - `tls:` clusterIssuer, secretName
    - `postgresql:` port, user, password, dataSize
    - `keycloak:` Bitnami subchart values (production: false, externalDatabase → postgresql, realm import через extraVolumes)
    - `adminModule:` port 8000, все AM env vars
    - `seCommon:` port 8010, общие настройки SE
    - `replicatedInstances:` se-edit-1, se-edit-2 (по 2 реплики)
    - `standaloneInstances:` se-rw-1, se-rw-2, se-ro, se-ar
    - `initJob:` keycloakClientId, keycloakClientSecret
  - **Creates**:
    - `tests/helm/artsore-test/values.yaml`
  - **Links**:
    - `src/storage-element/tests/helm/se-test/values.yaml` — паттерн values для SE
    - `src/admin-module/internal/config/config.go` — все env vars AM

- [x] **B.3 Создать `_helpers.tpl`**
  - **Dependencies**: B.2
  - **Description**: Template helpers: labels, image URLs, keycloakUrl, jwksUrl, tokenEndpoint, selector labels
  - **Creates**:
    - `tests/helm/artsore-test/templates/_helpers.tpl`
  - **Links**:
    - `src/storage-element/tests/helm/se-test/templates/_helpers.tpl`

- [x] **B.4 Создать `namespace.yaml` и `certificate.yaml`**
  - **Dependencies**: B.3
  - **Description**: Namespace artsore-test и cert-manager Certificate для PG + KC + AM + все SE
  - **Creates**:
    - `tests/helm/artsore-test/templates/namespace.yaml`
    - `tests/helm/artsore-test/templates/certificate.yaml`
  - **Links**:
    - `src/storage-element/tests/helm/se-test/templates/namespace.yaml`
    - `src/storage-element/tests/helm/se-test/templates/certificate.yaml`

- [x] **B.5 Создать `postgresql.yaml`**
  - **Dependencies**: B.3
  - **Description**: ConfigMap (initdb с созданием БД keycloak) + PVC + Deployment + Service для PostgreSQL
  - **Creates**:
    - `tests/helm/artsore-test/templates/postgresql.yaml`
  - **Links**: N/A

- [x] **B.6 Создать `realm-configmap.yaml`**
  - **Dependencies**: B.3
  - **Description**: ConfigMap с artsore-realm.json через `.Files.Get "files/artsore-realm.json"`
  - **Creates**:
    - `tests/helm/artsore-test/templates/realm-configmap.yaml`
  - **Links**: N/A

- [x] **B.7 Создать `admin-module.yaml`**
  - **Dependencies**: B.3, B.5
  - **Description**: Deployment (init containers: wait-for-pg + wait-for-kc) + Service для Admin Module
  - **Creates**:
    - `tests/helm/artsore-test/templates/admin-module.yaml`
  - **Links**:
    - `src/admin-module/internal/config/config.go` — env vars AM
    - `src/admin-module/docker-compose.yaml` — паттерн конфигурации

- [x] **B.8 Создать `se-replicated.yaml`**
  - **Dependencies**: B.3
  - **Description**: Range loop: PVC RWX + headless SVC + StatefulSet для реплицированных SE (se-edit-1, se-edit-2)
  - **Creates**:
    - `tests/helm/artsore-test/templates/se-replicated.yaml`
  - **Links**:
    - `src/storage-element/tests/helm/se-test/templates/se-replicated.yaml`

- [x] **B.9 Создать `se-standalone.yaml`**
  - **Dependencies**: B.3
  - **Description**: Range loop: PVC data + PVC wal + Deployment + SVC для standalone SE (se-rw-1, se-rw-2, se-ro, se-ar)
  - **Creates**:
    - `tests/helm/artsore-test/templates/se-standalone.yaml`
  - **Links**:
    - `src/storage-element/tests/helm/se-test/templates/se-standalone.yaml`

- [x] **B.10 Создать `init-job.yaml`**
  - **Dependencies**: B.3, B.7, B.8, B.9
  - **Description**: ConfigMap(lib.sh) + ConfigMap(init-data.sh) + Job (post-install hook). Получает токен из Keycloak (client_credentials), загружает файлы, переключает modes
  - **Creates**:
    - `tests/helm/artsore-test/templates/init-job.yaml`
  - **Links**:
    - `src/storage-element/tests/helm/se-test/templates/init-job.yaml`

### Критерии завершения Phase B

- [x] Все подпункты завершены (B.1 — B.10)
- [x] `helm lint tests/helm/artsore-test/` проходит без ошибок
- [x] Все шаблоны рендерятся корректно (`helm template` — 49 ресурсов)

---

## Phase C: Makefile и утилиты

**Dependencies**: Phase B
**Status**: Completed

### Описание

Создание Makefile для оркестрации сборки, деплоя и тестирования, а также базовой библиотеки утилит для тестовых скриптов.

### Подпункты

- [x] **C.1 Создать `tests/Makefile`**
  - **Dependencies**: None
  - **Description**: Makefile с переменными (AM_TAG, SE_TAG, NAMESPACE=artsore-test) и targets:
    - `docker-build` / `docker-build-am` / `docker-build-se` — сборка и push образов
    - `copy-realm` — копирует `deploy/keycloak/artsore-realm.json` → `helm/artsore-test/files/`
    - `helm-dep-update` — `helm dependency update`
    - `test-env-up` — copy-realm + helm-dep-update + helm upgrade --install + wait init job
    - `test-env-down` — port-forward-stop + helm uninstall + delete namespace
    - `test-env-status` — kubectl get pods/svc/pvc/jobs
    - `port-forward-start` / `port-forward-stop` / `port-forward-status`
    - `test-all` — запуск интеграционных тестов
    - Port-forward маппинг: PG→15432, KC→18080, AM→18000, SE→18010-18015
  - **Creates**:
    - `tests/Makefile`
  - **Links**:
    - `src/storage-element/tests/Makefile` — паттерн Makefile

- [x] **C.2 Создать `tests/scripts/lib.sh`**
  - **Dependencies**: None
  - **Description**: Базовая библиотека (адаптация из se-test):
    - Логирование, счётчики, assertions
    - `get_token_from_keycloak(endpoint, client_id, client_secret)` — Client Credentials flow
    - HTTP helpers: get, post, patch, delete, download
    - `wait_ready(url, timeout)` — ожидание readiness
  - **Creates**:
    - `tests/scripts/lib.sh`
  - **Links**:
    - `src/storage-element/tests/scripts/lib.sh` — паттерн утилит

### Критерии завершения Phase C

- [x] Все подпункты завершены (C.1, C.2)
- [x] `make -C tests help` работает
- [x] `lib.sh` создан с `get_token_from_keycloak()`

---

## Phase D: Валидация

**Dependencies**: Phase A, Phase B, Phase C
**Status**: Pending

### Описание

Полная проверка работоспособности тестовой среды: сборка образов, деплой в Kubernetes, проверка всех компонентов.

### Подпункты

- [ ] **D.1 Lint и сборка**
  - **Dependencies**: None
  - **Description**: `helm lint tests/helm/artsore-test/` и `make -C tests docker-build`
  - **Creates**: N/A
  - **Links**: N/A

- [ ] **D.2 Деплой и проверка**
  - **Dependencies**: D.1
  - **Description**: `make -C tests test-env-up` — все поды Running, init job Complete. Проверки:
    - PG имеет 2 БД (artsore + keycloak)
    - KC импортировал realm (curl realm info)
    - AM health/ready = ok
    - SE отвечают на health probes
    - Токен из KC принимается SE (JWKS валидация)
    - Init job загрузил файлы, переключил modes
  - **Creates**: N/A
  - **Links**: N/A

### Критерии завершения Phase D

- [ ] Все подпункты завершены (D.1, D.2)
- [ ] Все компоненты Running в Kubernetes
- [ ] Init job Complete
- [ ] Токены работают end-to-end

---

## Ключевые файлы-источники

| Файл-источник | Назначение |
|--------------|-----------|
| `src/storage-element/tests/helm/se-test/values.yaml` | Паттерн values для SE |
| `src/storage-element/tests/helm/se-test/templates/se-replicated.yaml` | Шаблон StatefulSet |
| `src/storage-element/tests/helm/se-test/templates/se-standalone.yaml` | Шаблон Deployment |
| `src/storage-element/tests/helm/se-test/templates/init-job.yaml` | Init job с lib.sh |
| `src/storage-element/tests/helm/se-test/templates/_helpers.tpl` | Template helpers |
| `src/storage-element/tests/Makefile` | Паттерн Makefile |
| `src/storage-element/tests/scripts/lib.sh` | Утилиты тестов |
| `src/admin-module/internal/config/config.go` | Все env vars AM |
| `src/admin-module/Dockerfile` | Docker-образ AM |
| `src/admin-module/docker-compose.yaml` | Паттерн PG + KC + AM |

---

## Примечания

- SE используют JWKS URL от реального Keycloak (не JWKS Mock как в se-test)
- Init Job получает токен через Client Credentials flow из Keycloak
- Bitnami Keycloak subchart требует `helm dependency update` перед установкой
- Realm JSON копируется в `files/` через Makefile target `copy-realm`
- Порядок старта обеспечивается init containers и readiness probes
