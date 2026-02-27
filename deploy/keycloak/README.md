# Keycloak — Artstore

Кастомный Docker-образ Keycloak с темой Artstore и конфигурация realm `artstore`.

## Кастомный образ

Базовый образ: `quay.io/keycloak/keycloak:26.1`
Registry: `harbor.kryukov.lan/library/keycloak-artstore`

Образ включает кастомную login-тему `artstore` — тёмная зелёная тема,
соответствующая дизайну Admin Module.

### Сборка и push

```bash
# Из директории tests/
make docker-build-kc docker-push-kc

# Или вручную
docker build --platform linux/amd64 \
  -t harbor.kryukov.lan/library/keycloak-artstore:v26.1-1 \
  deploy/keycloak/
docker push harbor.kryukov.lan/library/keycloak-artstore:v26.1-1
```

### Структура темы

```
themes/artstore/login/
├── theme.properties            # parent=keycloak.v2, darkMode=true
├── template.ftl                # Логотип Artstore (единственный .ftl)
├── messages/
│   ├── messages_en.properties  # EN
│   └── messages_ru.properties  # RU
└── resources/
    ├── css/artstore.css        # Переопределение PatternFly 5 → палитра Artstore
    └── img/logo.svg            # SVG логотип
```

Подход: CSS-only кастомизация с наследованием от `keycloak.v2`.
Переопределяется только `template.ftl` (для логотипа). Все страницы
стилизуются единым CSS-файлом.

### Обновление Keycloak

При обновлении версии Keycloak:

1. Обновить `FROM` в `Dockerfile`
2. Проверить совместимость `template.ftl` с новой версией `keycloak.v2`
3. Собрать и протестировать новый образ
4. Обновить тег в `tests/helm/artstore-infra/values.yaml`

## Содержимое realm

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
| `artstore-admin-module` | confidential | Client Credentials | все scopes + realm-management roles |
| `artstore-ingester` | confidential | Client Credentials | files:\*, storage:read |
| `artstore-query` | confidential | Client Credentials | files:read, storage:read |
| `artstore-admin-ui` | public | Auth Code + PKCE | openid, profile, email, groups |
| `artstore-test-user` | confidential | Password (ROPC) | groups (default) + все scopes (optional) |
| `artstore-test-init` | confidential | Client Credentials | files:\*, storage:\* |

### Тестовые секреты

| Client ID | Secret |
|-----------|--------|
| `artstore-admin-module` | `admin-module-test-secret` |
| `artstore-ingester` | `ingester-test-secret` |
| `artstore-query` | `query-test-secret` |
| `artstore-test-user` | `test-user-secret` |
| `artstore-test-init` | `test-init-secret` |

> **Внимание**: секреты предназначены только для тестовой/dev среды.
> В production используйте секреты, сгенерированные Keycloak.

### Группы и роли

| Группа | Realm Role |
|--------|-----------|
| `artstore-admins` | `admin` |
| `artstore-viewers` | `readonly` |

### Тестовые пользователи

| Username | Password | Группа | Роль |
|----------|----------|--------|------|
| `admin` | `admin` | `artstore-admins` | `admin` |
| `viewer` | `viewer` | `artstore-viewers` | `readonly` |

### Service Account — Admin Module

Service account клиента `artstore-admin-module` имеет роли `realm-management`:

- `view-users` — просмотр пользователей
- `manage-clients` — управление клиентами
- `view-clients` — просмотр клиентов
- `manage-users` — управление пользователями
- `view-realm` — просмотр настроек realm
- `query-users` — поиск пользователей
- `query-clients` — поиск клиентов

### Protocol Mappers (клиентские)

| Client | Mapper | Claim | Назначение |
|--------|--------|-------|-----------|
| `artstore-admin-module` | `client_id` | `client_id` | Идентификация SA в JWT |
| `artstore-test-user` | `realm roles` | `realm_access.roles` | Роли пользователя в JWT |
| `artstore-test-user` | `preferred_username` | `preferred_username` | Username в JWT |
| `artstore-test-user` | `sub` | `sub` | Subject в JWT |
| `artstore-test-user` | `email` | `email` | Email в JWT |

## Импорт realm

### Автоматический (через `--import-realm`)

При запуске Keycloak в Docker/Kubernetes:

```bash
# Docker (Keycloak 26.x)
docker run -d \
  -e KC_BOOTSTRAP_ADMIN_USERNAME=admin \
  -e KC_BOOTSTRAP_ADMIN_PASSWORD=admin \
  -v $(pwd)/artstore-realm.json:/opt/keycloak/data/import/artstore-realm.json \
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
#           name: artstore-realm
```

Keycloak импортирует realm при первом запуске. Если realm уже существует — импорт
пропускается (если не указана стратегия перезаписи).

### Ручной (через Admin UI)

1. Откройте Keycloak Admin Console: `http://localhost:8080/admin/`
2. Войдите под bootstrap admin (admin/admin)
3. В левом верхнем углу нажмите на выпадающий список realm
4. Нажмите **Create realm**
5. Нажмите **Browse...** и выберите файл `artstore-realm.json`
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
  -d @artstore-realm.json
```

## Проверка импорта

```bash
# Получить информацию о realm (публичный endpoint)
curl -s http://localhost:8080/realms/artstore | jq .

# Ожидаемый ответ содержит:
# "realm": "artstore"
# "public_key": "..."

# Получить JWKS (публичный endpoint)
curl -s http://localhost:8080/realms/artstore/protocol/openid-connect/certs | jq .

# Получить токен через Client Credentials
curl -s -X POST \
  "http://localhost:8080/realms/artstore/protocol/openid-connect/token" \
  -d "grant_type=client_credentials" \
  -d "client_id=artstore-test-init" \
  -d "client_secret=test-init-secret" | jq .
```
