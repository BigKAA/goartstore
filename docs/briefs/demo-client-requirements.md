# Demo Client — Требования к тестовому клиентскому приложению

## Обзор

**Demo Client** — тестовое WEB-приложение, демонстрирующее работу с goartstore
как внешний клиент. Приложение использует Service Account (M2M) для аутентификации
и работает через API Gateway, не имея прямого доступа к внутренним сервисам.

**Цели:**

- Демонстрация клиентского взаимодействия с artstore API
- Проверка всех пользовательских сценариев (upload, search, download)
- Демонстрация обработки ошибок (archived files, storage full, etc.)
- Визуальная отладка API-вызовов (лог операций)

## Ключевые решения

| Параметр | Решение |
|----------|---------|
| Аутентификация | Service Account (Client Credentials → Keycloak) |
| Стек | Go + Templ + HTMX + Alpine.js + Tailwind CSS |
| API доступ | Через API Gateway (`artstore.kryukov.lan`) |
| Деплой | Docker-compose (разработка) + Helm chart (K8s) |
| Размещение кода | `src/demo-client/` |
| Scope | Расширенный MVP |

## Функциональные требования

### FR-01: Аутентификация (Service Account)

- Приложение получает JWT токен у Keycloak по Client Credentials flow
- Параметры SA (client_id, client_secret, token_url) — из конфигурации (env)
- Автоматическое обновление токена до истечения срока
- На dashboard отображается статус токена (валиден, время до истечения)
- При ошибке аутентификации — retry с exponential backoff

**Keycloak клиент:** Новый клиент `artstore-demo-client` с scopes:
`files:read`, `files:write`

### FR-02: Dashboard (Главная страница)

- **Статус подключения**: health check artstore API через Gateway
- **Статус токена**: валидность, время до истечения, auto-refresh индикатор
- **Статистика**: количество загруженных файлов (через search API)
  - Всего файлов (active)
  - Временных (temporary)
  - Постоянных (permanent)
  - Архивных (в AR, недоступных для скачивания)
- **Лог операций**: последние N API-вызовов с результатами (FR-07)

### FR-03: Загрузка файлов (Upload)

#### FR-03.1: Одиночная загрузка

- Форма загрузки:
  - Выбор файла (input type=file)
  - Описание (textarea, опционально)
  - Теги (input, через запятую или чипы)
  - Retention policy: `temporary` или `permanent` (radio/select)
  - TTL (дни): отображается только при `temporary` (1-365, default 30)
- После загрузки — отображение результата: file_id, checksum, storage info
- Обработка ошибок: 413 (файл слишком большой), 502 (нет доступных SE), 507 (хранилище полно)

#### FR-03.2: Batch Upload

- Выбор нескольких файлов (input multiple)
- Общие параметры: description, tags, retention_policy, ttl_days
- Последовательная загрузка файлов с отчётом по каждому
- Прогресс: N из M загружено, ошибки
- Итоговый отчёт: успешно / неудачно / всего

### FR-04: Поиск файлов (Search)

- **Строка поиска** (fulltext mode по умолчанию)
- **Расширенные фильтры** (раскрывающаяся панель):
  - Расширение файла
  - Теги
  - Retention policy (temporary/permanent/all)
  - Статус (active/expired/deleted)
  - Диапазон дат загрузки
  - Диапазон размеров
- **Режим поиска**: fulltext / partial / exact (переключатель)
- **Результаты**:
  - Таблица/список с пагинацией
  - Для каждого файла: имя, размер, дата, теги, retention, статус, se_mode
  - Действия: Скачать, Метаданные
  - Визуальная индикация se_mode (rw/ro/ar — цветовая маркировка)
  - Для `ar` файлов: иконка/бейдж "Архивный", кнопка скачивания неактивна
- **Сортировка**: по дате, имени, размеру, релевантности (fulltext)

### FR-05: Скачивание файлов (Download)

- Скачивание по клику из результатов поиска
- Proxy-download через QM → API Gateway
- Обработка ошибок:
  - **410 Gone (FILE_ARCHIVED)**: модальное окно с сообщением:
    - "Файл находится в архивном хранилище и недоступен для скачивания"
    - file_id (с кнопкой копирования)
    - Имя файла, дата загрузки, теги
    - Инструкция: "Обратитесь к администратору artstore для восстановления
      доступа к файлу. Передайте file_id."
  - **404 Not Found**: "Файл не найден или удалён"
  - **500/502/503**: "Сервис временно недоступен, попробуйте позже"

### FR-06: Просмотр метаданных файла

- Отдельная карточка/модалка с полной информацией:
  - file_id, original_filename, content_type, size
  - checksum (SHA-256)
  - uploaded_by, uploaded_at
  - description, tags
  - retention_policy, ttl_days, expires_at
  - status, storage_element_id, se_mode
- Для temporary файлов: оставшееся время до истечения (визуально)
- Для ar файлов: предупреждение о недоступности скачивания

### FR-07: Лог операций (Activity Log)

- Панель на dashboard с последними API-вызовами
- Для каждой операции:
  - Время
  - Метод + URL (относительный)
  - HTTP статус (цветовая маркировка: 2xx зелёный, 4xx жёлтый, 5xx красный)
  - Длительность (мс)
  - Краткое описание (upload file X, search "query", download file Y)
- Обновление в реальном времени (HTMX polling или SSE)
- Максимум 100 последних записей (in-memory, без БД)

### FR-08: Конфигурация (Settings)

- Страница с текущими параметрами подключения (только чтение):
  - API Gateway URL
  - Keycloak Token URL
  - Client ID (без секрета)
  - Scopes
- Health-check зависимостей:
  - API Gateway (ping)
  - Upload endpoint (IM health через Gateway)
  - Search endpoint (QM health через Gateway)

## Нефункциональные требования

### NFR-01: Конфигурация

Все параметры через env-переменные:

| Переменная | Описание | Default |
|-----------|----------|---------|
| `DC_PORT` | HTTP порт приложения | `8080` |
| `DC_GATEWAY_URL` | URL API Gateway | `https://artstore.kryukov.lan` |
| `DC_TOKEN_URL` | Keycloak token endpoint | — (обязательный) |
| `DC_CLIENT_ID` | SA client_id | — (обязательный) |
| `DC_CLIENT_SECRET` | SA client_secret | — (обязательный) |
| `DC_LOG_LEVEL` | Уровень логирования | `info` |
| `DC_LOG_FORMAT` | Формат логов | `json` |
| `DC_TLS_SKIP_VERIFY` | Пропуск TLS проверки (dev) | `false` |
| `DC_TOKEN_REFRESH_BEFORE` | За сколько секунд до истечения обновлять токен | `30` |
| `DC_REQUEST_TIMEOUT` | Таймаут HTTP-запросов к Gateway | `30s` |
| `DC_UPLOAD_TIMEOUT` | Таймаут загрузки файлов | `300s` |
| `DC_MAX_UPLOAD_SIZE` | Макс. размер файла для загрузки | `1073741824` (1GB) |
| `DC_ACTIVITY_LOG_SIZE` | Размер лога операций | `100` |

### NFR-02: Безопасность

- Client secret не отображается в UI
- JWT токен хранится in-memory (не в cookies/localStorage)
- Все запросы к Gateway через HTTPS (кроме dev-режима)
- CSP заголовки для защиты от XSS
- CSRF-защита для форм

### NFR-03: UI/UX

- Responsive дизайн (Tailwind CSS)
- Тёмная тема поддержка (опционально для MVP)
- Уведомления (toast) для всех операций
- Загрузка индикаторы (HTMX loading states)
- i18n: русский (по умолчанию) + английский (как в Admin Module)

### NFR-04: Observability

- Структурированные логи (slog + JSON)
- Prometheus метрики (`/metrics`):
  - `dc_requests_total{method,path,status}` — запросы к приложению
  - `dc_gateway_requests_total{method,path,status}` — запросы к Gateway
  - `dc_gateway_request_duration_seconds` — латентность Gateway
  - `dc_token_refreshes_total{status}` — обновления токена
  - `dc_uploads_total{status,retention}` — загрузки
  - `dc_downloads_total{status}` — скачивания
- Health endpoints: `/health/live`, `/health/ready`

### NFR-05: Деплой

- **Dockerfile**: multi-stage (golang:1.25-alpine → alpine:3.19)
- **docker-compose.yaml**: для локальной разработки (обращение к внешнему Gateway)
- **Helm chart** (`charts/demo-client/`): для K8s деплоя
  - HTTPRoute через Gateway API
  - Домен: `demo.artstore.kryukov.lan` или path-based route
- Образ: `harbor.kryukov.lan/library/demo-client:v0.1.0-N`

## User Stories

### US-01: Первый запуск

> Как пользователь, я открываю demo-client в браузере и вижу dashboard
> со статусом подключения к artstore. Токен получен автоматически,
> все сервисы "зелёные".

### US-02: Загрузка временного файла

> Как пользователь, я загружаю файл с retention_policy=temporary, TTL=7 дней.
> Вижу подтверждение с file_id и датой истечения. В логе операций
> отображается POST /upload → 201.

### US-03: Загрузка постоянного файла

> Как пользователь, я загружаю файл с retention_policy=permanent, указываю теги.
> Файл попадает в rw хранилище. Вижу подтверждение.

### US-04: Batch Upload

> Как пользователь, я выбираю 5 файлов, задаю общие теги и retention.
> Вижу прогресс загрузки: 1/5, 2/5... В конце — итоговый отчёт.

### US-05: Поиск и скачивание

> Как пользователь, я ищу файлы по имени, фильтрую по тегам.
> Вижу результаты с индикацией se_mode. Скачиваю файл из rw/ro хранилища.

### US-06: Попытка скачать архивный файл

> Как пользователь, я пытаюсь скачать файл из ar хранилища.
> Вижу модальное окно: "Файл в архиве", file_id (кнопка копирования),
> инструкция "обратитесь к администратору". Копирую file_id.

### US-07: Просмотр метаданных

> Как пользователь, я кликаю на файл в результатах поиска.
> Вижу карточку с полной информацией: имя, размер, теги, checksum,
> дата загрузки, retention, TTL (если temporary), se_mode.

### US-08: Мониторинг операций

> Как пользователь, я смотрю лог операций на dashboard.
> Вижу все API-вызовы: upload, search, download.
> HTTP статусы раскрашены: 200 зелёный, 410 жёлтый, 500 красный.

## Открытые вопросы

1. **Keycloak клиент**: Нужно создать `artstore-demo-client` в Keycloak
   realm с scopes `files:read`, `files:write`. Добавить в тестовый Helm chart.

2. **Gateway маршрутизация**: Нужно добавить HTTPRoute для demo-client
   (отдельный домен `demo.artstore.kryukov.lan` или path `/demo/*`).

3. **Превью изображений**: Отложено на следующую версию (v0.2.0).
   Требует proxy через backend для добавления Authorization header.

## Архитектурные заметки

```
┌─────────────┐
│ Demo Client │  Go + Templ + HTMX
│   (browser) │
└──────┬──────┘
       │ HTTPS
       ▼
┌──────────────┐
│ Demo Client  │  Go HTTP server
│  (backend)   │  SA token management
└──────┬───────┘
       │ HTTPS + JWT
       ▼
┌──────────────┐
│ API Gateway  │  artstore.kryukov.lan
│ (Envoy GW)   │
└──┬─────┬─────┘
   │     │
   ▼     ▼
┌─────┐ ┌─────┐
│ IM  │ │ QM  │  Upload / Search+Download
└─────┘ └─────┘
```

- Demo Client backend — BFF (Backend for Frontend)
- Хранит SA токен in-memory, проксирует запросы к Gateway
- Браузер никогда не видит JWT токен
- Все API-вызовы идут через backend → Gateway

## Следующие шаги

После утверждения требований:

1. `/sc:design` — проектирование архитектуры и API
2. Создание плана разработки по шаблону
3. Фазированная реализация
