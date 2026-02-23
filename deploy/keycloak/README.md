# Keycloak Realm — Artsore

Конфигурация Keycloak realm `artsore` для всех модулей системы.

## Содержимое realm

### Client Scopes

| Scope | Назначение |
|-------|-----------|
| `files:read` | Чтение метаданных и скачивание файлов |
| `files:write` | Загрузка, обновление и удаление файлов |
| `storage:read` | Чтение информации о Storage Elements |
| `storage:write` | Управление SE (sync, mode transition) |
| `admin:read` | Чтение административных данных |
| `admin:write` | Управление пользователями и SA |
| `groups` | Маппинг групп в JWT claim `groups` |

### Клиенты

| Client ID | Тип | Flow | Scopes |
|-----------|-----|------|--------|
| `artsore-admin-module` | confidential | Client Credentials | все scopes + realm-management roles |
| `artsore-ingester` | confidential | Client Credentials | files:\*, storage:read |
| `artsore-query` | confidential | Client Credentials | files:read, storage:read |
| `artsore-admin-ui` | public | Auth Code + PKCE | openid, profile, email, groups |
| `artsore-test-init` | confidential | Client Credentials | files:\*, storage:\* |

### Тестовые секреты

| Client ID | Secret |
|-----------|--------|
| `artsore-admin-module` | `admin-module-test-secret` |
| `artsore-ingester` | `ingester-test-secret` |
| `artsore-query` | `query-test-secret` |
| `artsore-test-init` | `test-init-secret` |

> **Внимание**: секреты предназначены только для тестовой/dev среды.
> В production используйте секреты, сгенерированные Keycloak.

### Группы и роли

| Группа | Realm Role |
|--------|-----------|
| `artsore-admins` | `admin` |
| `artsore-viewers` | `readonly` |

### Тестовые пользователи

| Username | Password | Группа | Роль |
|----------|----------|--------|------|
| `admin` | `admin` | `artsore-admins` | `admin` |
| `viewer` | `viewer` | `artsore-viewers` | `readonly` |

### Service Account — Admin Module

Service account клиента `artsore-admin-module` имеет роли `realm-management`:

- `view-users` — просмотр пользователей
- `manage-clients` — управление клиентами
- `view-clients` — просмотр клиентов
- `manage-users` — управление пользователями
- `view-realm` — просмотр настроек realm

## Импорт realm

### Автоматический (через `--import-realm`)

При запуске Keycloak в Docker/Kubernetes:

```bash
# Docker (Keycloak 26.x)
docker run -d \
  -e KC_BOOTSTRAP_ADMIN_USERNAME=admin \
  -e KC_BOOTSTRAP_ADMIN_PASSWORD=admin \
  -v $(pwd)/artsore-realm.json:/opt/keycloak/data/import/artsore-realm.json \
  quay.io/keycloak/keycloak:26.1 \
  start-dev --import-realm

# Bitnami Keycloak (Helm)
# Realm импортируется через extraVolumeMounts + extraVolumes в values.yaml:
#   keycloak:
#     extraVolumeMounts:
#       - name: realm-config
#         mountPath: /opt/bitnami/keycloak/data/import
#     extraVolumes:
#       - name: realm-config
#         configMap:
#           name: artsore-realm
```

Keycloak импортирует realm при первом запуске. Если realm уже существует — импорт
пропускается (если не указана стратегия перезаписи).

### Ручной (через Admin UI)

1. Откройте Keycloak Admin Console: `http://localhost:8080/admin/`
2. Войдите под bootstrap admin (admin/admin)
3. В левом верхнем углу нажмите на выпадающий список realm
4. Нажмите **Create realm**
5. Нажмите **Browse...** и выберите файл `artsore-realm.json`
6. Нажмите **Create**

### Через Keycloak Admin CLI

```bash
# Получить токен admin
TOKEN=$(curl -s -X POST \
  "http://localhost:8080/realms/master/protocol/openid-connect/token" \
  -d "grant_type=client_credentials" \
  -d "client_id=admin-cli" \
  -d "username=admin" \
  -d "password=admin" \
  -d "grant_type=password" | jq -r '.access_token')

# Импортировать realm
curl -s -X POST "http://localhost:8080/admin/realms" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d @artsore-realm.json
```

## Проверка импорта

```bash
# Получить информацию о realm (публичный endpoint)
curl -s http://localhost:8080/realms/artsore | jq .

# Ожидаемый ответ содержит:
# "realm": "artsore"
# "public_key": "..."

# Получить JWKS (публичный endpoint)
curl -s http://localhost:8080/realms/artsore/protocol/openid-connect/certs | jq .

# Получить токен через Client Credentials
curl -s -X POST \
  "http://localhost:8080/realms/artsore/protocol/openid-connect/token" \
  -d "grant_type=client_credentials" \
  -d "client_id=artsore-test-init" \
  -d "client_secret=test-init-secret" | jq .
```
