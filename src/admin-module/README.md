# Admin Module

Центральный модуль управления Artsore — реестр Storage Elements, файлов, Service Accounts, RBAC и интеграция с Keycloak IdP.

## Возможности

- **RBAC**: JWT-аутентификация через Keycloak, роли `admin` / `readonly`, role overrides
- **Service Accounts**: CRUD с синхронизацией в Keycloak (client credentials)
- **Storage Elements**: обнаружение, регистрация, синхронизация состояния и файлов
- **Files Registry**: регистрация, метаданные, теги, retention policy, soft delete
- **IdP Integration**: статус подключения, принудительная синхронизация SA
- **Фоновые задачи**: периодическая синхронизация SE и SA

## API

29 endpoints, OpenAPI 3.0.3 спецификация: `docs/api-contracts/admin-module-openapi.yaml`

## Сборка

```bash
# Docker-образ
docker build --build-arg VERSION=v0.1.0 -t admin-module:v0.1.0 .

# Локальная сборка
make build    # бинарник → bin/admin-module
make test     # unit-тесты
```

## Конфигурация

Env-переменные (обязательные):

| Переменная | Описание |
|-----------|----------|
| `AM_DB_HOST` | Хост PostgreSQL |
| `AM_DB_NAME` | Имя БД |
| `AM_DB_USER` | Пользователь PostgreSQL |
| `AM_DB_PASSWORD` | Пароль PostgreSQL |
| `AM_KEYCLOAK_URL` | URL Keycloak (без trailing slash) |
| `AM_KEYCLOAK_CLIENT_ID` | Client ID для Admin API Keycloak |
| `AM_KEYCLOAK_CLIENT_SECRET` | Client secret для Admin API Keycloak |

Опциональные (с дефолтами): `AM_PORT=8000`, `AM_LOG_LEVEL=info`, `AM_KEYCLOAK_REALM=artsore`, `AM_SYNC_INTERVAL=1h`, `AM_SA_SYNC_INTERVAL=15m` и др. Полный список: `internal/config/config.go`.

## Деплой в Kubernetes

### Production Helm chart

```bash
cd charts/admin-module/
helm install admin-module . \
  --namespace artsore \
  --set image.tag=v0.1.0 \
  --set database.host=postgresql.artsore.svc \
  --set keycloak.url=https://keycloak.artsore.svc
```

Chart создаёт: Deployment, Service (ClusterIP:8000), HTTPRoute (Gateway API), ConfigMap, Secret.

### Тестовая среда

AM деплоится как часть тестовой инфраструктуры через `tests/helm/artsore-apps/`. Подробности — `tests/Makefile`.

## Интеграционные тесты

```bash
cd tests/
make port-forward-start   # port-forward к тестовой среде
make test-am              # запуск ~30 тестов
```

Тесты покрывают: smoke (health, metrics), admin-auth, admin-users (role overrides), service accounts (CRUD + rotate), storage elements (discover, register, sync), files (register, update, delete), IdP (status, sync-sa), обработку ошибок (401, 403, 409).
