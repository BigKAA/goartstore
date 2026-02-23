# Admin Module

Центральный модуль управления Artstore — реестр Storage Elements, файлов, Service Accounts, RBAC и интеграция с Keycloak IdP. Включает встроенный Admin UI.

## Возможности

- **RBAC**: JWT-аутентификация через Keycloak, роли `admin` / `readonly`, role overrides
- **Service Accounts**: CRUD с синхронизацией в Keycloak (client credentials)
- **Storage Elements**: обнаружение, регистрация, синхронизация состояния и файлов
- **Files Registry**: регистрация, метаданные, теги, retention policy, soft delete
- **IdP Integration**: статус подключения, принудительная синхронизация SA
- **Фоновые задачи**: периодическая синхронизация SE и SA
- **Admin UI**: веб-интерфейс администратора (встроен в бинарник)

## API

29 endpoints, OpenAPI 3.0.3 спецификация: `docs/api-contracts/admin-module-openapi.yaml`

## Admin UI

Встроенный веб-интерфейс для управления Artstore. Доступен по адресу `/admin/`.

### Технический стек UI

- **Шаблоны**: [Templ](https://templ.guide/) — type-safe Go templates, компиляция в Go-код
- **Интерактивность**: [HTMX](https://htmx.org/) 2.x + [Alpine.js](https://alpinejs.dev/) 3.x
- **Графики**: [ApexCharts](https://apexcharts.com/)
- **CSS**: [Tailwind CSS](https://tailwindcss.com/) v3 (Standalone CLI, compile-time)
- **Real-time**: SSE через HTMX SSE extension
- **Деплой**: все ассеты встроены в Go-бинарник через `embed.FS`

### Страницы UI

| Страница | URL | Описание |
|----------|-----|----------|
| Dashboard | `/admin/` | Метрики, графики, статусы зависимостей |
| Storage Elements | `/admin/storage-elements` | Discover, регистрация, sync, CRUD |
| SE Detail | `/admin/storage-elements/{id}` | Полная информация о SE + файлы |
| Файлы | `/admin/files` | Реестр файлов, фильтры, пагинация |
| Управление доступом | `/admin/access` | Пользователи + Service Accounts |
| Мониторинг | `/admin/monitoring` | Здоровье зависимостей, SSE, Prometheus |
| Настройки | `/admin/settings` | Конфигурация Prometheus (admin only) |

### Аутентификация UI

OAuth 2.0 Authorization Code + PKCE через Keycloak. Требуется public client `artstore-admin-ui` в realm.

### Структура UI

```
internal/ui/
├── auth/           — OIDC-клиент, session crypto (AES-256-GCM)
├── components/     — Переиспользуемые Templ-компоненты
├── handlers/       — HTTP-обработчики UI страниц
├── layouts/        — Base layout, sidebar, header
├── middleware/     — UI auth middleware
├── pages/          — Templ-шаблоны страниц
│   └── partials/   — HTMX partial responses
├── prometheus/     — Prometheus Query API клиент
└── static/         — CSS, JS (embed.FS)
```

## Сборка

```bash
# Docker-образ (включает templ generate + tailwind compile)
docker build --build-arg VERSION=v0.2.0 -t admin-module:v0.2.0 .

# Локальная сборка (требует templ CLI + tailwindcss binary)
make build    # ui-build + go build → bin/admin-module
make ui-build # templ-generate + css-build
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

Env-переменные Admin UI (опциональные):

| Переменная | По умолчанию | Описание |
|-----------|-------------|----------|
| `AM_UI_ENABLED` | `true` | Включить Admin UI |
| `AM_UI_SESSION_SECRET` | автогенерация | Ключ шифрования session cookie (32 bytes) |
| `AM_UI_OIDC_CLIENT_ID` | `artstore-admin-ui` | OIDC Client ID (public client, PKCE) |

Прочие опциональные (с дефолтами): `AM_PORT=8000`, `AM_LOG_LEVEL=info`, `AM_KEYCLOAK_REALM=artstore`, `AM_SYNC_INTERVAL=1h`, `AM_SA_SYNC_INTERVAL=15m` и др. Полный список: `internal/config/config.go`.

## Деплой в Kubernetes

### Production Helm chart

```bash
cd charts/admin-module/
helm install admin-module . \
  --namespace artstore \
  --set image.tag=v0.1.0 \
  --set database.host=postgresql.artstore.svc \
  --set keycloak.url=https://keycloak.artstore.svc
```

Chart создаёт: Deployment, Service (ClusterIP:8000), HTTPRoute (Gateway API), ConfigMap, Secret.

### Тестовая среда

AM деплоится как часть тестовой инфраструктуры через `tests/helm/artstore-apps/`. Подробности — `tests/Makefile`.

## Интеграционные тесты

```bash
cd tests/
make port-forward-start   # port-forward к тестовой среде
make test-am              # запуск ~40 тестов (API + UI)
make test-am-ui           # только тесты Admin UI
```

Тесты покрывают: smoke (health, metrics), admin-auth, admin-users (role overrides), service accounts (CRUD + rotate), storage elements (discover, register, sync), files (register, update, delete), IdP (status, sync-sa), обработку ошибок (401, 403, 409), Admin UI (статика, redirect, PKCE, SSE, logout).
