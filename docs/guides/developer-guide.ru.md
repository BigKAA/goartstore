# Руководство разработчика Artstore

> **Язык**: [English](developer-guide.md) | [Русский](developer-guide.ru.md)

## Содержание

- [1. Обзор API](#1-обзор-api)
  - [1.1 Архитектура](#11-архитектура)
  - [1.2 Базовые URL](#12-базовые-url)
  - [1.3 Модули API](#13-модули-api)
  - [1.4 Спецификации OpenAPI](#14-спецификации-openapi)
  - [1.5 Общие соглашения](#15-общие-соглашения)
- [2. Аутентификация](#2-аутентификация)
  - [2.1 Обзор JWT](#21-обзор-jwt)
  - [2.2 Получение токенов — учётные данные пользователя](#22-получение-токенов--учётные-данные-пользователя)
  - [2.3 Получение токенов — Client Credentials (сервисные аккаунты)](#23-получение-токенов--client-credentials-сервисные-аккаунты)
  - [2.4 Scopes и роли](#24-scopes-и-роли)
  - [2.5 Использование токенов в запросах](#25-использование-токенов-в-запросах)
  - [2.6 Обновление токена](#26-обновление-токена)
- [3. Загрузка файлов](#3-загрузка-файлов)
  - [3.1 Процесс загрузки](#31-процесс-загрузки)
  - [3.2 Эндпоинт загрузки](#32-эндпоинт-загрузки)
  - [3.3 Политики хранения](#33-политики-хранения)
  - [3.4 Примеры](#34-примеры)
- [4. Поиск файлов](#4-поиск-файлов)
  - [4.1 Процесс поиска](#41-процесс-поиска)
  - [4.2 Эндпоинт поиска](#42-эндпоинт-поиска)
  - [4.3 Фильтры поиска](#43-фильтры-поиска)
  - [4.4 Пагинация и сортировка](#44-пагинация-и-сортировка)
  - [4.5 Примеры](#45-примеры)
- [5. Скачивание файлов](#5-скачивание-файлов)
  - [5.1 Процесс скачивания](#51-процесс-скачивания)
  - [5.2 Эндпоинт скачивания](#52-эндпоинт-скачивания)
  - [5.3 Range-запросы](#53-range-запросы)
  - [5.4 Архивные файлы](#54-архивные-файлы)
  - [5.5 Примеры](#55-примеры)
- [6. Управление файлами](#6-управление-файлами)
  - [6.1 Получение метаданных файла](#61-получение-метаданных-файла)
  - [6.2 Список файлов](#62-список-файлов)
  - [6.3 Обновление метаданных файла](#63-обновление-метаданных-файла)
  - [6.4 Удаление файла](#64-удаление-файла)
  - [6.5 Регистрация файла (сервисные аккаунты)](#65-регистрация-файла-сервисные-аккаунты)
- [7. Обработка ошибок](#7-обработка-ошибок)
  - [7.1 Формат ответа об ошибке](#71-формат-ответа-об-ошибке)
  - [7.2 Справочник кодов ошибок](#72-справочник-кодов-ошибок)
  - [7.3 HTTP-коды статусов](#73-http-коды-статусов)
  - [7.4 Обработка ошибок в коде](#74-обработка-ошибок-в-коде)

---

## 1. Обзор API

### 1.1 Архитектура

Artstore — распределённое файловое хранилище с микросервисной архитектурой. С точки зрения разработчика, система предоставляет три API-модуля, каждый из которых отвечает за определённый набор операций:

- **Ingester Module (IM)** — загрузка файлов
- **Query Module (QM)** — поиск и скачивание файлов
- **Admin Module (AM)** — реестр файлов, управление хранилищами, сервисные аккаунты

Все модули расположены за **API Gateway** (Envoy Gateway), который обеспечивает терминацию TLS, валидацию JWT и маршрутизацию запросов.

![Диаграмма C4 Container](images/c4-container.png)
*Рис. 1.1 — Архитектура системы: модули, базы данных и потоки коммуникации*

> **Ключевой принцип**: Клиенты взаимодействуют с системой через один домен (`https://artstore.example.com`). API Gateway маршрутизирует запросы к нужному модулю на основе префикса пути URL.

### 1.2 Базовые URL

Все API-запросы проходят через API Gateway по единому базовому URL:

| Окружение | Базовый URL |
|-----------|-------------|
| Production | `https://artstore.example.com` |
| Разработка | `https://artstore.kryukov.lan` |

Маршрутизация путей к модулям:

| Префикс пути | Модуль | Назначение |
|---------------|--------|------------|
| `/upload/*` | Ingester Module | Загрузка файлов |
| `/query/*` | Query Module | Поиск и скачивание |
| `/admin/*` | Admin Module | Реестр файлов, управление SE, сервисные аккаунты |

Например, для загрузки файла отправьте запрос на:

```
POST https://artstore.example.com/upload/api/v1/files/upload
```

### 1.3 Модули API

| Модуль | Диапазон портов | Ключевые эндпоинты | Требуется авторизация |
|--------|----------------|--------------------|-----------------------|
| Ingester Module | 8020-8029 | `POST /api/v1/files/upload` | Да (files:write) |
| Query Module | 8030-8039 | `POST /api/v1/search`, `GET /api/v1/files/{id}/download` | Да (files:read) |
| Admin Module | 8000-8009 | `GET/POST/PUT/DELETE /api/v1/files`, `/api/v1/storage-elements`, `/api/v1/service-accounts` | Да (различные) |

> **Примечание**: Диапазоны портов предназначены для прямого доступа к модулям (отладка, внутренняя коммуникация). Клиенты должны всегда использовать API Gateway.

### 1.4 Спецификации OpenAPI

Полные спецификации OpenAPI 3.0.3 доступны для каждого модуля:

| Модуль | Спецификация |
|--------|-------------|
| Admin Module | [`docs/api-contracts/admin-module-openapi.yaml`](../api-contracts/admin-module-openapi.yaml) |
| Ingester Module | [`docs/api-contracts/ingester-module-openapi.yaml`](../api-contracts/ingester-module-openapi.yaml) |
| Query Module | [`docs/api-contracts/query-module-openapi.yaml`](../api-contracts/query-module-openapi.yaml) |
| Storage Element | [`docs/api-contracts/storage-element-openapi.yaml`](../api-contracts/storage-element-openapi.yaml) |

### 1.5 Общие соглашения

**Формат запросов**:

- JSON для тел запросов и ответов (`Content-Type: application/json`)
- `multipart/form-data` для загрузки файлов
- Все временные метки в формате ISO 8601 (`2026-03-03T10:30:00Z`)
- UUID для всех идентификаторов ресурсов

**Формат ответов**:

- Успешные ответы: `200 OK`, `201 Created`, `204 No Content`
- Пагинированные списки содержат поля `total`, `limit`, `offset`, `has_more`
- Ошибки всегда используют [стандартный формат ошибок](#71-формат-ответа-об-ошибке)

**Аутентификация**:

- Все эндпоинты (кроме `/health/*` и `/metrics`) требуют JWT Bearer-токен
- Токен передаётся в заголовке `Authorization`

---

## 2. Аутентификация

### 2.1 Обзор JWT

Artstore использует **JWT RS256-токены**, выпускаемые **Keycloak** (OIDC-совместимый Identity Provider). Процесс аутентификации:

1. Клиент получает JWT от эндпоинта токенов Keycloak
2. Клиент включает JWT в заголовок `Authorization: Bearer <token>`
3. API Gateway проверяет подпись JWT через JWKS, проверяет срок действия
4. Модуль извлекает claims (роли, scopes, имя пользователя) из валидированного JWT

![Процесс JWT-аутентификации](images/seq-jwt-auth.png)
*Рис. 2.1 — Последовательность JWT-аутентификации: Клиент → Keycloak → Gateway → Модуль*

### 2.2 Получение токенов — учётные данные пользователя

Для пользователей (администраторы, наблюдатели) используйте grant-тип **Resource Owner Password Credentials**:

**curl**:

```bash
# Получение JWT для пользователя
TOKEN=$(curl -s -X POST \
  "https://keycloak.example.com/realms/artstore/protocol/openid-connect/token" \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d "grant_type=password" \
  -d "client_id=artstore-test-user" \
  -d "client_secret=test-user-secret" \
  -d "username=admin" \
  -d "password=admin" \
  | jq -r '.access_token')

echo $TOKEN
```

**Python**:

```python
import requests

token_url = "https://keycloak.example.com/realms/artstore/protocol/openid-connect/token"

response = requests.post(token_url, data={
    "grant_type": "password",
    "client_id": "artstore-test-user",
    "client_secret": "test-user-secret",
    "username": "admin",
    "password": "admin",
})

token = response.json()["access_token"]
```

**Go**:

```go
package main

import (
    "encoding/json"
    "net/http"
    "net/url"
    "strings"
)

func getToken() (string, error) {
    tokenURL := "https://keycloak.example.com/realms/artstore/protocol/openid-connect/token"

    data := url.Values{
        "grant_type":    {"password"},
        "client_id":     {"artstore-test-user"},
        "client_secret": {"test-user-secret"},
        "username":      {"admin"},
        "password":      {"admin"},
    }

    resp, err := http.Post(tokenURL, "application/x-www-form-urlencoded",
        strings.NewReader(data.Encode()))
    if err != nil {
        return "", err
    }
    defer resp.Body.Close()

    var result struct {
        AccessToken string `json:"access_token"`
    }
    if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
        return "", err
    }
    return result.AccessToken, nil
}
```

Ответ содержит:

```json
{
  "access_token": "eyJhbGciOiJSUzI1NiIs...",
  "expires_in": 300,
  "refresh_expires_in": 1800,
  "refresh_token": "eyJhbGciOiJIUzUxMiIs...",
  "token_type": "Bearer",
  "scope": "openid profile email"
}
```

> **Примечание**: Grant-тип Resource Owner Password Credentials используется в основном для тестирования. В production используйте Authorization Code + PKCE для браузерных приложений.

### 2.3 Получение токенов — Client Credentials (сервисные аккаунты)

Для межсервисного (M2M) взаимодействия используйте grant-тип **Client Credentials**:

**curl**:

```bash
# Получение JWT для сервисного аккаунта
TOKEN=$(curl -s -X POST \
  "https://keycloak.example.com/realms/artstore/protocol/openid-connect/token" \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d "grant_type=client_credentials" \
  -d "client_id=artstore-ingester" \
  -d "client_secret=ingester-test-secret" \
  | jq -r '.access_token')
```

**Python**:

```python
response = requests.post(token_url, data={
    "grant_type": "client_credentials",
    "client_id": "artstore-ingester",
    "client_secret": "ingester-test-secret",
})

token = response.json()["access_token"]
```

**Go**:

```go
func getServiceAccountToken(clientID, clientSecret string) (string, error) {
    tokenURL := "https://keycloak.example.com/realms/artstore/protocol/openid-connect/token"

    data := url.Values{
        "grant_type":    {"client_credentials"},
        "client_id":     {clientID},
        "client_secret": {clientSecret},
    }

    resp, err := http.Post(tokenURL, "application/x-www-form-urlencoded",
        strings.NewReader(data.Encode()))
    if err != nil {
        return "", err
    }
    defer resp.Body.Close()

    var result struct {
        AccessToken string `json:"access_token"`
    }
    if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
        return "", err
    }
    return result.AccessToken, nil
}
```

Токены сервисных аккаунтов содержат scopes в payload JWT:

```json
{
  "scope": "files:read files:write storage:read",
  "client_id": "artstore-ingester",
  "typ": "Bearer"
}
```

### 2.4 Scopes и роли

Artstore использует две модели авторизации:

**Роли** (для пользователей через группы Keycloak):

| Роль | Группа | Разрешения |
|------|--------|------------|
| `admin` | `artstore-admins` | Полный доступ: чтение, запись, удаление, управление SE и SA |
| `readonly` | `artstore-viewers` | Только чтение: список файлов, поиск, скачивание |

**Scopes** (для сервисных аккаунтов через конфигурацию клиентов Keycloak):

| Scope | Разрешения |
|-------|------------|
| `files:read` | Чтение метаданных файлов, скачивание, поиск |
| `files:write` | Загрузка файлов, обновление метаданных, удаление, регистрация |
| `storage:read` | Чтение информации и статуса Storage Element |
| `storage:write` | Управление режимами SE, запуск операций обслуживания |
| `admin:read` | Чтение списка пользователей, списка сервисных аккаунтов |
| `admin:write` | Управление пользователями, создание/обновление сервисных аккаунтов |

**Правила авторизации эндпоинтов**:

| Операция | Требуемая роль | Требуемый scope |
|----------|----------------|-----------------|
| Загрузка файла | `admin` | `files:write` |
| Поиск файлов | `admin` или `readonly` | `files:read` |
| Скачивание файла | `admin` или `readonly` | `files:read` |
| Получение метаданных | `admin` или `readonly` | `files:read` |
| Обновление метаданных | `admin` | `files:write` |
| Удаление файла | `admin` | `files:write` |
| Просмотр/управление SE | `admin` | `storage:read` / `storage:write` |
| Управление сервисными аккаунтами | `admin` | `admin:write` |

> **Логика авторизации**: Эндпоинты принимают **либо** роль пользователя, **либо** scope сервисного аккаунта. Запрос со scope `files:read` ИЛИ ролью `readonly` может искать и скачивать файлы.

### 2.5 Использование токенов в запросах

Включайте JWT в заголовок `Authorization` для всех API-запросов:

```bash
curl -H "Authorization: Bearer $TOKEN" \
  "https://artstore.example.com/query/api/v1/files/{file_id}"
```

### 2.6 Обновление токена

Access-токены имеют короткое время жизни (по умолчанию: 5 минут). Используйте refresh-токен для получения нового access-токена без повторной аутентификации:

```bash
NEW_TOKEN=$(curl -s -X POST \
  "https://keycloak.example.com/realms/artstore/protocol/openid-connect/token" \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d "grant_type=refresh_token" \
  -d "client_id=artstore-test-user" \
  -d "client_secret=test-user-secret" \
  -d "refresh_token=$REFRESH_TOKEN" \
  | jq -r '.access_token')
```

> **Примечание**: Токены сервисных аккаунтов (Client Credentials grant) не имеют refresh-токенов. Запрашивайте новый токен по истечении текущего.

---

## 3. Загрузка файлов

### 3.1 Процесс загрузки

Загрузка файлов обрабатывается **Ingester Module**. Процесс загрузки:

1. Клиент отправляет файл через `POST /api/v1/files/upload` (multipart/form-data)
2. Ingester аутентифицирует запрос (валидация JWT)
3. Ingester выбирает Storage Element на основе политики хранения:
   - `temporary` → SE в режиме `edit`
   - `permanent` → SE в режиме `rw`
4. Ingester передаёт файл потоком на выбранный SE
5. SE сохраняет файл и создаёт метаданные `attr.json`
6. Ingester регистрирует файл в Admin Module
7. Клиент получает ответ с метаданными загруженного файла

![Поток данных загрузки файла](images/dataflow-upload.png)
*Рис. 3.1 — Поток данных загрузки: Клиент → Ingester → SE → Admin Module*

![Последовательность загрузки файла](images/seq-file-upload.png)
*Рис. 3.2 — Диаграмма последовательности загрузки с детальным взаимодействием модулей*

### 3.2 Эндпоинт загрузки

```
POST /upload/api/v1/files/upload
Content-Type: multipart/form-data
Authorization: Bearer <token>
```

**Поля формы**:

| Поле | Тип | Обязательное | Описание |
|------|-----|-------------|----------|
| `file` | binary | Да | Загружаемый файл |
| `description` | string | Нет | Описание файла (макс. 1000 символов) |
| `tags` | string | Нет | JSON-массив тегов, например `["logo","draft"]` |
| `retention_policy` | string | Нет | `temporary` (по умолчанию) или `permanent` |
| `ttl_days` | integer | Нет | Дней до истечения, 1-365 (по умолчанию: 30, только для `temporary`) |

**Ответ** (`201 Created`):

```json
{
  "file_id": "550e8400-e29b-41d4-a716-446655440000",
  "original_filename": "report.pdf",
  "content_type": "application/pdf",
  "size": 2048576,
  "checksum": "a1b2c3d4e5f6...",
  "uploaded_by": "admin",
  "uploaded_at": "2026-03-03T10:30:00Z",
  "description": "Q1 Report",
  "tags": ["report", "2026"],
  "status": "active",
  "storage_element_id": "7d3f2a1b-4c5e-6f7a-8b9c-0d1e2f3a4b5c",
  "retention_policy": "temporary",
  "ttl_days": 30,
  "expires_at": "2026-04-02T10:30:00Z"
}
```

### 3.3 Политики хранения

| Политика | Режим SE | TTL | Сценарий использования |
|----------|----------|-----|----------------------|
| `temporary` | `edit` | 1-365 дней (по умолчанию: 30) | Черновики, временные файлы, артефакты CI |
| `permanent` | `rw` | Без ограничения | Рабочие ресурсы, документы |

- Временные файлы автоматически удаляются сборщиком мусора SE после `expires_at`
- Постоянные файлы хранятся до ручного удаления или перехода SE в режим `ar` (архив)

### 3.4 Примеры

**curl — простая загрузка**:

```bash
curl -X POST "https://artstore.example.com/upload/api/v1/files/upload" \
  -H "Authorization: Bearer $TOKEN" \
  -F "file=@/path/to/report.pdf"
```

**curl — загрузка с метаданными**:

```bash
curl -X POST "https://artstore.example.com/upload/api/v1/files/upload" \
  -H "Authorization: Bearer $TOKEN" \
  -F "file=@/path/to/logo.png" \
  -F "description=Company logo v2" \
  -F 'tags=["logo","brand","v2"]' \
  -F "retention_policy=permanent"
```

**curl — временный файл с пользовательским TTL**:

```bash
curl -X POST "https://artstore.example.com/upload/api/v1/files/upload" \
  -H "Authorization: Bearer $TOKEN" \
  -F "file=@/tmp/build-artifact.tar.gz" \
  -F "description=CI build #1234" \
  -F 'tags=["ci","build"]' \
  -F "retention_policy=temporary" \
  -F "ttl_days=7"
```

**Python**:

```python
import requests

url = "https://artstore.example.com/upload/api/v1/files/upload"
headers = {"Authorization": f"Bearer {token}"}

# Простая загрузка
with open("/path/to/report.pdf", "rb") as f:
    response = requests.post(url, headers=headers, files={"file": f})

print(response.json()["file_id"])

# Загрузка с метаданными
import json

with open("/path/to/logo.png", "rb") as f:
    response = requests.post(url, headers=headers,
        files={"file": ("logo.png", f, "image/png")},
        data={
            "description": "Company logo v2",
            "tags": json.dumps(["logo", "brand", "v2"]),
            "retention_policy": "permanent",
        })

print(response.json())
```

**Go**:

```go
package main

import (
    "bytes"
    "encoding/json"
    "fmt"
    "io"
    "mime/multipart"
    "net/http"
    "os"
)

func uploadFile(token, filePath, description string, tags []string,
    retentionPolicy string) (map[string]interface{}, error) {

    file, err := os.Open(filePath)
    if err != nil {
        return nil, err
    }
    defer file.Close()

    body := &bytes.Buffer{}
    writer := multipart.NewWriter(body)

    // Добавление файла
    part, err := writer.CreateFormFile("file", file.Name())
    if err != nil {
        return nil, err
    }
    if _, err := io.Copy(part, file); err != nil {
        return nil, err
    }

    // Добавление метаданных
    if description != "" {
        writer.WriteField("description", description)
    }
    if len(tags) > 0 {
        tagsJSON, _ := json.Marshal(tags)
        writer.WriteField("tags", string(tagsJSON))
    }
    if retentionPolicy != "" {
        writer.WriteField("retention_policy", retentionPolicy)
    }

    writer.Close()

    req, err := http.NewRequest("POST",
        "https://artstore.example.com/upload/api/v1/files/upload", body)
    if err != nil {
        return nil, err
    }
    req.Header.Set("Content-Type", writer.FormDataContentType())
    req.Header.Set("Authorization", "Bearer "+token)

    resp, err := http.DefaultClient.Do(req)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()

    var result map[string]interface{}
    if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
        return nil, err
    }

    if resp.StatusCode != http.StatusCreated {
        return nil, fmt.Errorf("upload failed: %d %v", resp.StatusCode, result)
    }
    return result, nil
}
```

---

## 4. Поиск файлов

### 4.1 Процесс поиска

Поиск файлов обрабатывается **Query Module**. Он обращается к PostgreSQL напрямую (общая база данных с Admin Module) и поддерживает полнотекстовый поиск с множественными фильтрами.

![Поток данных поиска](images/dataflow-search.png)
*Рис. 4.1 — Поток данных поиска: Клиент → Query Module → PostgreSQL FTS*

Query Module поддерживает in-memory LRU-кэш для часто запрашиваемых метаданных (TTL: 30-60 секунд).

### 4.2 Эндпоинт поиска

```
POST /query/api/v1/search
Content-Type: application/json
Authorization: Bearer <token>
```

> **Примечание**: Поиск использует POST (а не GET), потому что критерии фильтрации могут быть сложными и длинными.

**Тело запроса** (`SearchRequest`):

```json
{
  "query": "report",
  "tags": ["2026"],
  "retention_policy": "permanent",
  "sort_by": "uploaded_at",
  "sort_order": "desc",
  "limit": 20,
  "offset": 0
}
```

**Ответ** (`200 OK`):

```json
{
  "items": [
    {
      "file_id": "550e8400-e29b-41d4-a716-446655440000",
      "original_filename": "annual-report-2026.pdf",
      "content_type": "application/pdf",
      "size": 5242880,
      "checksum": "a1b2c3d4e5f6...",
      "uploaded_by": "admin",
      "uploaded_at": "2026-03-01T14:00:00Z",
      "description": "Annual report 2026",
      "tags": ["report", "2026", "annual"],
      "status": "active",
      "retention_policy": "permanent",
      "storage_element_id": "7d3f2a1b-4c5e-6f7a-8b9c-0d1e2f3a4b5c",
      "se_mode": "rw"
    }
  ],
  "total": 1,
  "limit": 20,
  "offset": 0,
  "has_more": false
}
```

### 4.3 Фильтры поиска

Все фильтры необязательны. При указании нескольких фильтров они объединяются логикой AND.

| Фильтр | Тип | Описание |
|--------|-----|----------|
| `query` | string | Поиск подстроки в `original_filename` (без учёта регистра) |
| `filename` | string | Точное или частичное совпадение имени файла |
| `file_extension` | string | Расширение файла без точки (например, `pdf`, `jpg`) |
| `tags` | string[] | Файл должен содержать ВСЕ указанные теги |
| `uploaded_by` | string | Имя пользователя или client_id загрузившего |
| `retention_policy` | string | `temporary` или `permanent` |
| `status` | string | `active` (по умолчанию), `expired` или `deleted` |
| `min_size` | integer | Минимальный размер файла в байтах |
| `max_size` | integer | Максимальный размер файла в байтах |
| `uploaded_after` | string | Нижняя граница даты в формате ISO 8601 |
| `uploaded_before` | string | Верхняя граница даты в формате ISO 8601 |
| `mode` | string | `partial` (по умолчанию, ILIKE) или `exact` (без учёта регистра) |

**Правила валидации**:

- `uploaded_after` не должен быть позже `uploaded_before`
- `min_size` не должен превышать `max_size`
- `min_size` и `max_size` должны быть неотрицательными
- Максимальная длина `query`: 500 символов
- Максимум `tags`: 50 элементов

### 4.4 Пагинация и сортировка

| Параметр | Тип | По умолчанию | Диапазон | Описание |
|----------|-----|-------------|----------|----------|
| `limit` | integer | 20 | 1-1000 | Количество результатов на страницу |
| `offset` | integer | 0 | 0+ | Количество пропускаемых результатов |
| `sort_by` | string | `uploaded_at` | `uploaded_at`, `original_filename`, `size` | Поле сортировки |
| `sort_order` | string | `desc` | `asc`, `desc` | Направление сортировки |

**Пример пагинации** — получение страницы 3 (20 элементов на страницу):

```json
{
  "query": "report",
  "limit": 20,
  "offset": 40
}
```

### 4.5 Примеры

**curl — простой поиск**:

```bash
curl -X POST "https://artstore.example.com/query/api/v1/search" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"query": "report"}'
```

**curl — поиск с фильтрами**:

```bash
curl -X POST "https://artstore.example.com/query/api/v1/search" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "query": "logo",
    "file_extension": "png",
    "tags": ["brand"],
    "retention_policy": "permanent",
    "min_size": 1024,
    "uploaded_after": "2026-01-01T00:00:00Z",
    "sort_by": "size",
    "sort_order": "desc",
    "limit": 10
  }'
```

**Python**:

```python
import requests

url = "https://artstore.example.com/query/api/v1/search"
headers = {
    "Authorization": f"Bearer {token}",
    "Content-Type": "application/json",
}

# Простой поиск
response = requests.post(url, headers=headers, json={"query": "report"})
results = response.json()

print(f"Найдено {results['total']} файлов")
for item in results["items"]:
    print(f"  {item['original_filename']} ({item['size']} байт)")

# Пагинированный поиск
all_files = []
offset = 0
while True:
    response = requests.post(url, headers=headers, json={
        "query": "report",
        "limit": 100,
        "offset": offset,
    })
    data = response.json()
    all_files.extend(data["items"])
    if not data["has_more"]:
        break
    offset += 100

print(f"Всего файлов собрано: {len(all_files)}")
```

**Go**:

```go
package main

import (
    "bytes"
    "encoding/json"
    "fmt"
    "net/http"
)

type SearchRequest struct {
    Query           string   `json:"query,omitempty"`
    Tags            []string `json:"tags,omitempty"`
    RetentionPolicy string   `json:"retention_policy,omitempty"`
    Limit           int      `json:"limit,omitempty"`
    Offset          int      `json:"offset,omitempty"`
    SortBy          string   `json:"sort_by,omitempty"`
    SortOrder       string   `json:"sort_order,omitempty"`
}

type SearchResponse struct {
    Items   []map[string]interface{} `json:"items"`
    Total   int                      `json:"total"`
    Limit   int                      `json:"limit"`
    Offset  int                      `json:"offset"`
    HasMore bool                     `json:"has_more"`
}

func searchFiles(token string, req SearchRequest) (*SearchResponse, error) {
    body, err := json.Marshal(req)
    if err != nil {
        return nil, err
    }

    httpReq, err := http.NewRequest("POST",
        "https://artstore.example.com/query/api/v1/search",
        bytes.NewReader(body))
    if err != nil {
        return nil, err
    }
    httpReq.Header.Set("Content-Type", "application/json")
    httpReq.Header.Set("Authorization", "Bearer "+token)

    resp, err := http.DefaultClient.Do(httpReq)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        return nil, fmt.Errorf("search failed: %d", resp.StatusCode)
    }

    var result SearchResponse
    if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
        return nil, err
    }
    return &result, nil
}
```

---

## 5. Скачивание файлов

### 5.1 Процесс скачивания

Скачивание файлов обрабатывается **Query Module**, который выступает прокси между клиентом и Storage Element.

1. Клиент запрашивает `GET /api/v1/files/{file_id}/download`
2. Query Module аутентифицирует запрос
3. Query Module ищет файл в реестре (PostgreSQL или кэш)
4. Query Module получает файл с соответствующего SE
5. Файл передаётся потоком клиенту

![Поток данных скачивания](images/dataflow-download.png)
*Рис. 5.1 — Поток данных скачивания: Клиент → Query Module → Admin Module → SE*

![Последовательность скачивания](images/seq-file-download.png)
*Рис. 5.2 — Диаграмма последовательности скачивания с кэшированием и взаимодействием с SE*

> **Ленивая очистка**: Если SE возвращает HTTP 404 при скачивании, Query Module автоматически отмечает файл как `deleted` в реестре.

### 5.2 Эндпоинт скачивания

```
GET /query/api/v1/files/{file_id}/download
Authorization: Bearer <token>
```

**Ответ** (`200 OK`):

Бинарное содержимое файла с заголовками:

| Заголовок | Пример | Описание |
|-----------|--------|----------|
| `Content-Type` | `application/pdf` | MIME-тип файла |
| `Content-Length` | `2048576` | Размер файла в байтах |
| `Content-Disposition` | `attachment; filename="report.pdf"` | Предлагаемое имя файла |
| `Accept-Ranges` | `bytes` | Сервер поддерживает range-запросы |
| `ETag` | `"a1b2c3d4e5f6"` | Контрольная сумма файла для кэширования |

### 5.3 Range-запросы

Для больших файлов или возобновляемых загрузок используйте заголовок `Range`:

```bash
# Скачать первый 1 МБ
curl -H "Authorization: Bearer $TOKEN" \
  -H "Range: bytes=0-1048575" \
  "https://artstore.example.com/query/api/v1/files/{file_id}/download" \
  -o partial.bin
```

**Ответ** (`206 Partial Content`):

| Заголовок | Пример | Описание |
|-----------|--------|----------|
| `Content-Range` | `bytes 0-1048575/5242880` | Возвращённый диапазон / общий размер |
| `Content-Length` | `1048576` | Размер частичного ответа |

### 5.4 Архивные файлы

Если файл находится на Storage Element в режиме `ar` (архив), скачивание возвращает:

**Ответ** (`410 Gone`):

```json
{
  "error": {
    "code": "FILE_ARCHIVED",
    "message": "File is in an archived storage element and cannot be downloaded"
  }
}
```

> **Совет**: Используйте поле `se_mode` в [результатах поиска](#42-эндпоинт-поиска) для проверки доступности файла перед попыткой скачивания.

### 5.5 Примеры

**curl — скачивание файла**:

```bash
curl -H "Authorization: Bearer $TOKEN" \
  "https://artstore.example.com/query/api/v1/files/550e8400-e29b-41d4-a716-446655440000/download" \
  -o report.pdf
```

**curl — скачивание с прогрессом**:

```bash
curl -# -H "Authorization: Bearer $TOKEN" \
  "https://artstore.example.com/query/api/v1/files/{file_id}/download" \
  -o large-file.zip
```

**Python**:

```python
import requests

file_id = "550e8400-e29b-41d4-a716-446655440000"
url = f"https://artstore.example.com/query/api/v1/files/{file_id}/download"
headers = {"Authorization": f"Bearer {token}"}

# Простое скачивание
response = requests.get(url, headers=headers, stream=True)
with open("report.pdf", "wb") as f:
    for chunk in response.iter_content(chunk_size=8192):
        f.write(chunk)

# Скачивание с прогрессом
import shutil
response = requests.get(url, headers=headers, stream=True)
total = int(response.headers.get("Content-Length", 0))
with open("report.pdf", "wb") as f:
    downloaded = 0
    for chunk in response.iter_content(chunk_size=8192):
        f.write(chunk)
        downloaded += len(chunk)
        if total > 0:
            print(f"\rПрогресс: {downloaded}/{total} ({100*downloaded//total}%)", end="")
```

**Go**:

```go
package main

import (
    "fmt"
    "io"
    "net/http"
    "os"
)

func downloadFile(token, fileID, outputPath string) error {
    url := fmt.Sprintf(
        "https://artstore.example.com/query/api/v1/files/%s/download", fileID)

    req, err := http.NewRequest("GET", url, nil)
    if err != nil {
        return err
    }
    req.Header.Set("Authorization", "Bearer "+token)

    resp, err := http.DefaultClient.Do(req)
    if err != nil {
        return err
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        return fmt.Errorf("download failed: %d", resp.StatusCode)
    }

    out, err := os.Create(outputPath)
    if err != nil {
        return err
    }
    defer out.Close()

    _, err = io.Copy(out, resp.Body)
    return err
}
```

---

## 6. Управление файлами

Операции управления файлами (CRUD метаданных) обрабатываются **Admin Module**. Эти эндпоинты в основном используются администраторами и сервисными аккаунтами для управления реестром файлов.

### 6.1 Получение метаданных файла

```
GET /admin/api/v1/files/{file_id}
Authorization: Bearer <token>
```

**Требуется**: роль `admin` или `readonly`, или scope `files:read`

**Ответ** (`200 OK`):

```json
{
  "file_id": "550e8400-e29b-41d4-a716-446655440000",
  "original_filename": "report.pdf",
  "content_type": "application/pdf",
  "size": 2048576,
  "checksum": "a1b2c3d4e5f6...",
  "storage_element_id": "7d3f2a1b-4c5e-6f7a-8b9c-0d1e2f3a4b5c",
  "uploaded_by": "admin",
  "uploaded_at": "2026-03-03T10:30:00Z",
  "description": "Q1 Report",
  "tags": ["report", "2026"],
  "status": "active",
  "retention_policy": "temporary",
  "ttl_days": 30,
  "expires_at": "2026-04-02T10:30:00Z",
  "created_at": "2026-03-03T10:30:00Z",
  "updated_at": "2026-03-03T10:30:00Z"
}
```

**curl**:

```bash
curl -H "Authorization: Bearer $TOKEN" \
  "https://artstore.example.com/admin/api/v1/files/550e8400-e29b-41d4-a716-446655440000"
```

> **Примечание**: Метаданные файла также можно получить через эндпоинт Query Module `GET /query/api/v1/files/{file_id}`, который возвращает аналогичные данные и использует in-memory кэш для более быстрых ответов.

### 6.2 Список файлов

```
GET /admin/api/v1/files?limit=20&offset=0
Authorization: Bearer <token>
```

**Требуется**: роль `admin` или `readonly`, или scope `files:read`

**Параметры запроса**:

| Параметр | Тип | По умолчанию | Описание |
|----------|-----|-------------|----------|
| `limit` | integer | 20 | Результатов на страницу (1-1000) |
| `offset` | integer | 0 | Пропускаемых результатов |
| `status` | string | — | Фильтр по статусу: `active`, `deleted`, `archived` |
| `retention_policy` | string | — | Фильтр: `temporary` или `permanent` |
| `storage_element_id` | UUID | — | Фильтр по SE |
| `uploaded_by` | string | — | Фильтр по загрузившему |

**Ответ** (`200 OK`):

```json
{
  "items": [ ... ],
  "total": 150,
  "limit": 20,
  "offset": 0
}
```

**curl**:

```bash
# Список активных временных файлов
curl -H "Authorization: Bearer $TOKEN" \
  "https://artstore.example.com/admin/api/v1/files?status=active&retention_policy=temporary&limit=50"
```

### 6.3 Обновление метаданных файла

```
PUT /admin/api/v1/files/{file_id}
Content-Type: application/json
Authorization: Bearer <token>
```

**Требуется**: роль `admin`, или scope `files:write`

**Изменяемые поля**: `description`, `tags`, `status`

**Неизменяемые поля** (не могут быть изменены): `original_filename`, `content_type`, `size`, `checksum`, `storage_element_id`, `uploaded_by`, `retention_policy`

**Тело запроса**:

```json
{
  "description": "Updated description",
  "tags": ["report", "2026", "final"],
  "status": "active"
}
```

**curl**:

```bash
curl -X PUT "https://artstore.example.com/admin/api/v1/files/{file_id}" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"description": "Final version", "tags": ["report", "final"]}'
```

### 6.4 Удаление файла

```
DELETE /admin/api/v1/files/{file_id}
Authorization: Bearer <token>
```

**Требуется**: роль `admin`, или scope `files:write`

**Ответ**: `204 No Content`

> **Примечание**: Это мягкое удаление. Статус файла устанавливается в `deleted` в реестре. Физическое удаление файла выполняется сборщиком мусора SE.

**curl**:

```bash
curl -X DELETE "https://artstore.example.com/admin/api/v1/files/{file_id}" \
  -H "Authorization: Bearer $TOKEN"
```

### 6.5 Регистрация файла (сервисные аккаунты)

Этот эндпоинт используется Ingester Module (или другими сервисными аккаунтами) для регистрации файла после успешной загрузки на Storage Element.

```
POST /admin/api/v1/files
Content-Type: application/json
Authorization: Bearer <token>
```

**Требуется**: scope `files:write`

**Тело запроса**:

```json
{
  "file_id": "550e8400-e29b-41d4-a716-446655440000",
  "original_filename": "report.pdf",
  "content_type": "application/pdf",
  "size": 2048576,
  "checksum": "a1b2c3d4e5f6...",
  "storage_element_id": "7d3f2a1b-4c5e-6f7a-8b9c-0d1e2f3a4b5c",
  "uploaded_by": "admin",
  "retention_policy": "temporary",
  "ttl_days": 30,
  "description": "Q1 Report",
  "tags": ["report", "2026"]
}
```

**Правила валидации**:

- `file_id`, `original_filename`, `content_type`, `checksum`, `storage_element_id` — обязательны
- `size` должен быть больше 0
- Для `temporary` хранения: `ttl_days` должен быть 1-365
- `uploaded_by` устанавливается из JWT: `preferred_username` для пользователей, `client_id` для сервисных аккаунтов

**Ответ** (`201 Created`): Зарегистрированный объект `FileRecord`.

---

## 7. Обработка ошибок

### 7.1 Формат ответа об ошибке

Все модули Artstore возвращают ошибки в унифицированном JSON-формате:

```json
{
  "error": {
    "code": "ERROR_CODE",
    "message": "Описание ошибки в читаемом виде"
  }
}
```

- `code` — машиночитаемый код ошибки в формате `SCREAMING_SNAKE_CASE`
- `message` — человекочитаемое описание (может меняться, не следует парсить программно)

### 7.2 Справочник кодов ошибок

**Общие ошибки (все модули)**:

| Код | HTTP-статус | Описание |
|-----|-------------|----------|
| `VALIDATION_ERROR` | 400 | Невалидные параметры запроса или тело |
| `UNAUTHORIZED` | 401 | Отсутствующий, просроченный или невалидный JWT-токен |
| `FORBIDDEN` | 403 | Недостаточные роль или scope для операции |
| `NOT_FOUND` | 404 | Запрашиваемый ресурс не существует |
| `INTERNAL_ERROR` | 500 | Неожиданная ошибка сервера |

**Ошибки Ingester Module**:

| Код | HTTP-статус | Описание |
|-----|-------------|----------|
| `FILE_TOO_LARGE` | 413 | Загружаемый файл превышает лимит размера |
| `STORAGE_FULL` | 507 | Нет свободного места на выбранном Storage Element |
| `NO_STORAGE_AVAILABLE` | 502 | Нет доступного SE для запрошенной политики хранения |
| `SE_UPLOAD_FAILED` | 502 | Storage Element вернул ошибку при загрузке |
| `ADMIN_UNAVAILABLE` | 502 | Admin Module недоступен |

**Ошибки Query Module**:

| Код | HTTP-статус | Описание |
|-----|-------------|----------|
| `INVALID_RANGE` | 416 | Некорректный или невыполнимый заголовок `Range` |
| `FILE_ARCHIVED` | 410 | Файл на архивном SE и не может быть скачан |
| `SE_UNAVAILABLE` | 502 | Storage Element недоступен для скачивания |
| `AM_UNAVAILABLE` | 502 | Admin Module недоступен |

**Ошибки Admin Module**:

| Код | HTTP-статус | Описание |
|-----|-------------|----------|
| `CONFLICT` | 409 | Ресурс с таким именем уже существует |
| `MODE_NOT_ALLOWED` | 409 | Невалидный переход режима SE |

### 7.3 HTTP-коды статусов

| Статус | Значение | Когда используется |
|--------|----------|-------------------|
| `200 OK` | Успех | Чтение, обновление, поиск |
| `201 Created` | Ресурс создан | Загрузка, регистрация файла, создание SA |
| `204 No Content` | Успех, без тела | Удаление |
| `206 Partial Content` | Частичная загрузка | Range-запрос |
| `400 Bad Request` | Невалидный ввод | Ошибки валидации |
| `401 Unauthorized` | Требуется авторизация | Отсутствующий/невалидный JWT |
| `403 Forbidden` | Доступ запрещён | Недостаточные разрешения |
| `404 Not Found` | Не найдено | Несуществующий ресурс |
| `409 Conflict` | Конфликт состояния | Дублирование имени, невалидный переход режима |
| `410 Gone` | Ресурс удалён | Архивный файл |
| `413 Payload Too Large` | Файл слишком большой | Лимит размера загрузки |
| `416 Range Not Satisfiable` | Невалидный диапазон | Некорректный заголовок Range |
| `500 Internal Server Error` | Ошибка сервера | Неожиданные сбои |
| `502 Bad Gateway` | Ошибка upstream | SE или AM недоступны |
| `503 Service Unavailable` | Не готов | Модуль запускается |
| `507 Insufficient Storage` | Нет места | Диск SE заполнен |

### 7.4 Обработка ошибок в коде

**Python**:

```python
import requests

response = requests.post(url, headers=headers, json=data)

if response.status_code == 200:
    result = response.json()
elif response.status_code == 401:
    # Токен истёк — обновить и повторить
    token = refresh_token()
    headers["Authorization"] = f"Bearer {token}"
    response = requests.post(url, headers=headers, json=data)
elif response.status_code == 410:
    error = response.json()["error"]
    print(f"Файл в архиве: {error['message']}")
else:
    error = response.json()["error"]
    print(f"Ошибка [{error['code']}]: {error['message']}")
```

**Go**:

```go
type APIError struct {
    Error struct {
        Code    string `json:"code"`
        Message string `json:"message"`
    } `json:"error"`
}

func handleResponse(resp *http.Response) error {
    if resp.StatusCode >= 200 && resp.StatusCode < 300 {
        return nil // успех
    }

    var apiErr APIError
    if err := json.NewDecoder(resp.Body).Decode(&apiErr); err != nil {
        return fmt.Errorf("HTTP %d: не удалось декодировать ошибку", resp.StatusCode)
    }

    switch apiErr.Error.Code {
    case "UNAUTHORIZED":
        return fmt.Errorf("требуется аутентификация: %s", apiErr.Error.Message)
    case "FILE_ARCHIVED":
        return fmt.Errorf("файл в архиве и не может быть скачан")
    case "NO_STORAGE_AVAILABLE":
        return fmt.Errorf("нет доступного хранилища, попробуйте позже")
    default:
        return fmt.Errorf("[%s] %s", apiErr.Error.Code, apiErr.Error.Message)
    }
}
```
