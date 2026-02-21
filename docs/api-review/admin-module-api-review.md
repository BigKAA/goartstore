# Ревизия API: Admin Module

**Дата**: 2026-02-21
**Статус**: Draft
**Модуль**: Admin Module (old_artstore/admin-module/)

---

## 1. Сводка текущего API

Admin Module содержит **9 групп endpoints** и **1 систему событий**:

| # | Группа | Prefix | Endpoints | Назначение |
|---|--------|--------|-----------|------------|
| 1 | OAuth 2.0 | `/auth/token` | 1 | Токены для Service Accounts (M2M) |
| 2 | Admin Auth | `/admin-auth/*` | 5 | Аутентификация администраторов (H2M) |
| 3 | Admin Users | `/admin-users/*` | 6 | CRUD администраторов |
| 4 | Service Accounts | `/service-accounts/*` | 6 | CRUD сервисных аккаунтов |
| 5 | Storage Elements | `/storage-elements/*` | 8 | CRUD + discovery + sync SE |
| 6 | JWT Keys | `/jwt-keys/*` | 4 | Управление ротацией JWT ключей |
| 7 | Files | `/files/*` | 5 | Реестр файлов (internal) |
| 8 | Internal | `/internal/*` | 2 | Fallback API для межсервисного взаимодействия |
| 9 | Health | `/health/*` | 4 | Kubernetes probes + metrics |
| 10 | Events | (Redis Pub/Sub) | 3 типа | Синхронизация file metadata с Query Module |

**Итого: ~41 endpoint + 3 типа событий**

---

## 2. Выявленные проблемы и несоответствия

### 2.1. Непоследовательная пагинация

Используются **две разные схемы** пагинации:

| Группа | Схема | Параметры | Ответ |
|--------|-------|-----------|-------|
| Admin Users | Page-based | `page`, `page_size` | `items`, `total`, `page`, `page_size`, `total_pages` |
| Service Accounts | Offset-based | `skip`, `limit` | `items`, `total`, `skip`, `limit` |
| Storage Elements | Offset-based | `skip`, `limit` | `items`, `total`, `skip`, `limit` |
| Files | Page-based | `page`, `page_size` | `files`, `total`, `page`, `page_size`, `total_pages` |

**Проблемы:**

- Два разных стиля пагинации в одном модуле
- Разные имена коллекции в ответе: `items` vs `files`
- У offset-based нет `total_pages`

### 2.2. Непоследовательный формат ошибок

| Контекст | Формат |
|----------|--------|
| Общие ошибки | `{"detail": "message"}` |
| OAuth 2.0 ошибки | `{"error": "code", "error_description": "message"}` (RFC 6749) |
| Валидация | FastAPI default (массив ошибок) |

### 2.3. Дублирование схем

- `AdminUserResponse` определён **дважды**: в `admin_auth.py` (для `/me`) и `admin_user.py` (для CRUD) с **разным набором полей**
- `LoginRequest` и `TokenResponse` в `auth.py` — **не используются** (legacy). Реально используются `OAuth2TokenRequest`/`OAuth2TokenResponse` и `AdminLoginRequest`/`AdminTokenResponse`
- `PasswordResetRequest`, `PasswordResetConfirm` в `auth.py` — **не используются**, endpoint не реализован

### 2.4. Двойные идентификаторы Storage Element

- `id` (int) — database primary key
- `element_id` (string) — строковый идентификатор для Redis Registry

В Internal API используется `element_id`, в Storage Elements API — `id` (int). Это создаёт путаницу.

### 2.5. Несоответствие OAuth 2.0 спецификации RFC 6749

- Content-Type должен быть `application/x-www-form-urlencoded` (RFC 6749 §4.4.2), но используется `application/json`
- Поле `grant_type` в схеме присутствует, но в API.md не упоминается
- Refresh token выдаётся, но **нет endpoint** для его использования сервисными аккаунтами (только для админов)

### 2.6. Непоследовательный регистр enum-значений

| Enum | Значения в коде | Отображение в API.md |
|------|-----------------|---------------------|
| AdminRole | `super_admin`, `admin`, `readonly` | В нижнем регистре |
| ServiceAccountRole | `admin`, `user`, `auditor`, `readonly` | В ВЕРХНЕМ регистре (ADMIN, USER...) |
| ServiceAccountStatus | `active`, `suspended`, `expired`, `deleted` | В ВЕРХНЕМ регистре |
| StorageMode | `edit`, `rw`, `ro`, `ar` | В нижнем регистре |
| StorageStatus | `online`, `offline`, `degraded`, `maintenance` | В нижнем регистре |

Документация описывает ServiceAccount enum-ы в UPPERCASE, но в коде они lowercase.

### 2.7. Rate limiting без enforcement

- Rate limit хранится в JWT payload и в модели ServiceAccount
- Middleware для проверки **не реализован**
- Смысл хранить в токене — неясен без enforcement

### 2.8. Logout без server-side revocation

- `POST /admin-auth/logout` возвращает `{"success": true}`, но **не инвалидирует** токен
- JWT ID (`jti`) добавлен «для будущего», но blacklist не реализован
- Logout — чисто клиентская операция (удаление токена из UI)

---

## 3. Анализ по группам: сохранить / упростить / убрать

### 3.1. OAuth 2.0 — СОХРАНИТЬ (с корректировками)

| Endpoint | Решение | Комментарий |
|----------|---------|-------------|
| `POST /auth/token` | Сохранить | Основной механизм M2M аутентификации |

**Рекомендации:**

- Оставить `application/json` (отступление от RFC 6749, но удобнее)
- Убрать `refresh_token` из ответа (SA всегда могут заново запросить token по credentials)
- Убрать поле `grant_type` из запроса (у нас только один grant type)
- Переименовать `issued_at` → не нужен (есть `exp` и `expires_in`)

### 3.2. Admin Auth — СОХРАНИТЬ (с упрощениями)

| Endpoint | Решение | Комментарий |
|----------|---------|-------------|
| `POST /admin-auth/login` | Сохранить | Основной вход для Admin UI |
| `POST /admin-auth/refresh` | Сохранить | Продление сессии |
| `POST /admin-auth/logout` | Убрать | Клиентская операция, нет серверной логики |
| `GET /admin-auth/me` | Сохранить | Информация о текущем пользователе |
| `POST /admin-auth/change-password` | Сохранить | Смена пароля |

**Рекомендации:**

- Убрать `logout` — клиент просто удаляет токен
- Убрать `confirm_password` из change-password (валидация на клиенте)
- Убрать password history (over-engineering)
- Упростить блокировку: оставить лимит попыток, но без таймера unlock (ручной unlock через SUPER_ADMIN)

### 3.3. Admin Users CRUD — СОХРАНИТЬ (как есть)

| Endpoint | Решение | Комментарий |
|----------|---------|-------------|
| `POST /admin-users/` | Сохранить | Создание администратора |
| `GET /admin-users/` | Сохранить | Список администраторов |
| `GET /admin-users/{id}` | Сохранить | Детали администратора |
| `PUT /admin-users/{id}` | Сохранить | Обновление администратора |
| `DELETE /admin-users/{id}` | Сохранить | Удаление администратора |
| `POST /admin-users/{id}/reset-password` | Сохранить | Сброс пароля |

**Рекомендации:**

- Унифицировать пагинацию (см. раздел 4)
- Убрать `is_system` (системный admin создаётся при init, но не нужен как отдельный флаг; защита через логику, а не через поле)

### 3.4. Service Accounts CRUD — СОХРАНИТЬ (с упрощениями)

| Endpoint | Решение | Комментарий |
|----------|---------|-------------|
| `POST /service-accounts/` | Сохранить | Создание SA |
| `GET /service-accounts/` | Сохранить | Список SA |
| `GET /service-accounts/{id}` | Сохранить | Детали SA |
| `PUT /service-accounts/{id}` | Сохранить | Обновление SA |
| `DELETE /service-accounts/{id}` | Сохранить | Удаление SA |
| `POST /service-accounts/{id}/rotate-secret` | Сохранить | Ротация secret |

**Рекомендации:**

- Убрать `environment` (prod/staging/dev) — это инфраструктурный аспект, не бизнес-логика
- Убрать `is_system` (аналогично Admin Users)
- Убрать `rate_limit` из модели SA (rate limiting — задача API gateway / middleware, а не auth)
- Убрать `secret_history` (over-engineering)
- Упростить `client_id` — не нужен сложный формат `sa_<env>_<name>_<random>`, достаточно UUID
- Убрать `days_until_expiry` и `requires_rotation_warning` — вычисляемые поля, клиент может рассчитать сам

### 3.5. Storage Elements — УПРОСТИТЬ

| Endpoint | Решение | Комментарий |
|----------|---------|-------------|
| `POST /storage-elements/discover` | Сохранить | Предпросмотр SE перед регистрацией |
| `POST /storage-elements/` | Сохранить | Регистрация SE с auto-discovery |
| `GET /storage-elements/` | Сохранить | Список SE |
| `GET /storage-elements/{id}` | Сохранить | Детали SE |
| `PUT /storage-elements/{id}` | Сохранить | Обновление SE |
| `DELETE /storage-elements/{id}` | Сохранить | Удаление SE |
| `POST /storage-elements/sync/{id}` | Сохранить | Ручная синхронизация |
| `POST /storage-elements/sync-all` | Сохранить | Массовая синхронизация |
| `GET /storage-elements/stats/summary` | Убрать | Клиент может рассчитать из списка |

**Рекомендации:**

- Убрать двойную идентификацию: использовать только один `id` (UUID или string)
- Убрать `is_replicated`, `replica_count` — в упрощённой архитектуре нет репликации
- Убрать `priority` — Sequential Fill Algorithm удалён
- Вычисляемые поля (`capacity_gb`, `used_gb`, `usage_percent`, `is_available`, `is_writable`) — клиент рассчитает сам из `capacity_bytes`, `used_bytes`, `mode`, `status`
- Переименовать `api_key` → `token` или убрать (аутентификация через OAuth)

### 3.6. JWT Keys — УПРОСТИТЬ ЗНАЧИТЕЛЬНО

| Endpoint | Решение | Комментарий |
|----------|---------|-------------|
| `GET /jwt-keys/status` | Сохранить | Полезно для мониторинга |
| `GET /jwt-keys/active` | Убрать | Дублирует информацию из status |
| `POST /jwt-keys/rotate` | Сохранить | Ручная ротация полезна |
| `GET /jwt-keys/history` | Убрать | Избыточно для нашей задачи |

**Рекомендации:**

- Убрать multi-version key support (database-backed keys + file fallback)
- Простой подход: один активный ключ в файле, ротация через перезапуск или endpoint
- При ротации — graceful period: старый ключ валиден ещё N минут для уже выданных токенов

### 3.7. Files Registry — ПЕРЕОСМЫСЛИТЬ

| Endpoint | Решение | Комментарий |
|----------|---------|-------------|
| `POST /files` | Сохранить | Регистрация файла (от Ingester) |
| `GET /files/{file_id}` | Сохранить | Получение метаданных |
| `PUT /files/{file_id}` | Упростить | Финализация без Two-Phase Commit |
| `DELETE /files/{file_id}` | Сохранить | Soft delete |
| `GET /files` | Сохранить | Список файлов |

**Рекомендации:**

- Убрать поля связанные с Two-Phase Commit: `finalized_at`, `is_finalized`
- Убрать `compressed`, `compression_algorithm`, `original_size` (метаданные сжатия — дело Storage Element)
- Убрать `upload_source_ip` (дело Ingester, не хранить в registry)
- Упростить: файл либо `temporary` (есть TTL), либо `permanent` (без TTL)
- Убрать `ttl_days` из запроса — вычислять из `ttl_expires_at`

### 3.8. Internal API — ОБЪЕДИНИТЬ со Storage Elements

| Endpoint | Решение | Комментарий |
|----------|---------|-------------|
| `GET /internal/storage-elements/available` | Объединить | Добавить query-параметры в `GET /storage-elements/` |
| `GET /internal/storage-elements/{element_id}` | Убрать | Дублирует `GET /storage-elements/{id}` |

**Рекомендации:**

- Internal API существовал как fallback при недоступности Redis
- Поскольку Redis убираем — это становится основной API
- Логику выбора доступных SE добавить как фильтры в `GET /storage-elements/`

### 3.9. Health — СОХРАНИТЬ (с корректировками)

| Endpoint | Решение | Комментарий |
|----------|---------|-------------|
| `GET /health/live` | Сохранить | Kubernetes liveness probe |
| `GET /health/ready` | Сохранить | Kubernetes readiness probe |
| `GET /health/startup` | Сохранить | Kubernetes startup probe |
| `GET /health/metrics` | Переместить | Prometheus metrics → `/metrics` (стандарт) |

**Рекомендации:**

- Убрать Redis из проверки ready (Redis убран)
- Метрики — отдельный endpoint `/metrics` (без prefix `/health`)

### 3.10. Events (Redis Pub/Sub) — УБРАТЬ

| Event | Решение | Комментарий |
|-------|---------|-------------|
| `file:created` | Убрать | Redis убран, Query Module будет опрашивать БД напрямую |
| `file:updated` | Убрать | То же |
| `file:deleted` | Убрать | То же |

**Рекомендации:**

- Вместо событийной модели — Query Module напрямую читает из PostgreSQL
- Если потребуется асинхронность в будущем — рассмотреть PostgreSQL LISTEN/NOTIFY

---

## 4. Рекомендации по унификации

### 4.1. Единая пагинация

Рекомендуемая схема: **page-based** (проще для клиента):

**Параметры запроса:**

```
?page=1&per_page=20
```

- `page` — номер страницы (начиная с 1, default: 1)
- `per_page` — размер страницы (default: 20, max: 100)

**Ответ:**

```json
{
  "items": [...],
  "total": 150,
  "page": 1,
  "per_page": 20,
  "total_pages": 8
}
```

### 4.2. Единый формат ошибок

```json
{
  "error": {
    "code": "not_found",
    "message": "Storage Element with id '123' not found"
  }
}
```

Для OAuth 2.0 endpoint — оставить RFC 6749 формат (`error`, `error_description`), он стандартный.

### 4.3. Единые enum-значения — snake_case

Все enum-значения в **snake_case** (lowercase):

```
admin_role:      super_admin | admin | readonly
sa_role:         admin | user | auditor | readonly
sa_status:       active | suspended | expired | deleted
storage_mode:    edit | rw | ro | ar
storage_status:  online | offline | degraded | maintenance
retention:       temporary | permanent
```

### 4.4. Идентификаторы — UUID

Все сущности используют UUID (string) как primary key:

- Admin Users — UUID (уже)
- Service Accounts — UUID (уже)
- Storage Elements — UUID (заменить `int id` + `string element_id`)
- Files — UUID (уже)

### 4.5. Timestamps — RFC 3339

Все timestamps в формате RFC 3339 (ISO 8601 с timezone):

```
"2026-02-21T14:30:45Z"
```

### 4.6. Версионирование API

- Prefix: `/api/v1/`
- Версия API фиксирована в URL (как сейчас)

---

## 5. Итоговая таблица endpoints (новый API)

### Аутентификация (4 endpoints)

| Метод | Endpoint | Назначение |
|-------|----------|------------|
| POST | `/api/v1/auth/token` | OAuth 2.0 token для Service Accounts |
| POST | `/api/v1/admin-auth/login` | Вход администратора |
| POST | `/api/v1/admin-auth/refresh` | Обновление access token |
| GET | `/api/v1/admin-auth/me` | Текущий администратор |
| POST | `/api/v1/admin-auth/change-password` | Смена пароля |

### Admin Users (6 endpoints)

| Метод | Endpoint | Назначение | RBAC |
|-------|----------|------------|------|
| POST | `/api/v1/admin-users` | Создать администратора | SUPER_ADMIN |
| GET | `/api/v1/admin-users` | Список администраторов | Любая роль |
| GET | `/api/v1/admin-users/{id}` | Получить администратора | Любая роль |
| PUT | `/api/v1/admin-users/{id}` | Обновить администратора | SUPER_ADMIN |
| DELETE | `/api/v1/admin-users/{id}` | Удалить администратора | SUPER_ADMIN |
| POST | `/api/v1/admin-users/{id}/reset-password` | Сброс пароля | SUPER_ADMIN |

### Service Accounts (6 endpoints)

| Метод | Endpoint | Назначение | RBAC |
|-------|----------|------------|------|
| POST | `/api/v1/service-accounts` | Создать SA | SUPER_ADMIN |
| GET | `/api/v1/service-accounts` | Список SA | Любая роль |
| GET | `/api/v1/service-accounts/{id}` | Получить SA | Любая роль |
| PUT | `/api/v1/service-accounts/{id}` | Обновить SA | SUPER_ADMIN |
| DELETE | `/api/v1/service-accounts/{id}` | Удалить SA | SUPER_ADMIN |
| POST | `/api/v1/service-accounts/{id}/rotate-secret` | Ротация secret | SUPER_ADMIN |

### Storage Elements (7 endpoints)

| Метод | Endpoint | Назначение | RBAC |
|-------|----------|------------|------|
| POST | `/api/v1/storage-elements/discover` | Предпросмотр SE | Любая роль |
| POST | `/api/v1/storage-elements` | Регистрация SE | SUPER_ADMIN |
| GET | `/api/v1/storage-elements` | Список SE (+ фильтры для internal) | Любая роль / SA |
| GET | `/api/v1/storage-elements/{id}` | Получить SE | Любая роль / SA |
| PUT | `/api/v1/storage-elements/{id}` | Обновить SE | SUPER_ADMIN |
| DELETE | `/api/v1/storage-elements/{id}` | Удалить SE | SUPER_ADMIN |
| POST | `/api/v1/storage-elements/{id}/sync` | Синхронизация SE | ADMIN |

### JWT Keys (2 endpoints)

| Метод | Endpoint | Назначение | RBAC |
|-------|----------|------------|------|
| GET | `/api/v1/jwt-keys/status` | Статус ключей | ADMIN |
| POST | `/api/v1/jwt-keys/rotate` | Ротация ключей | ADMIN |

### Files Registry (5 endpoints)

| Метод | Endpoint | Назначение | Аутентификация |
|-------|----------|------------|----------------|
| POST | `/api/v1/files` | Регистрация файла | Service Account |
| GET | `/api/v1/files` | Список файлов | Service Account |
| GET | `/api/v1/files/{id}` | Метаданные файла | Service Account |
| PUT | `/api/v1/files/{id}` | Обновление метаданных | Service Account |
| DELETE | `/api/v1/files/{id}` | Soft delete | Service Account (ADMIN) |

### Health (3 endpoints + metrics)

| Метод | Endpoint | Назначение |
|-------|----------|------------|
| GET | `/health/live` | Liveness probe |
| GET | `/health/ready` | Readiness probe |
| GET | `/health/startup` | Startup probe |
| GET | `/metrics` | Prometheus metrics |

### Итого: ~34 endpoints (было ~41)

---

## 6. Что убрано (сводка)

| Что убрано | Причина |
|------------|---------|
| `POST /admin-auth/logout` | Клиентская операция, нет серверной логики |
| `GET /jwt-keys/active` | Дублирует `/jwt-keys/status` |
| `GET /jwt-keys/history` | Избыточно |
| `GET /storage-elements/stats/summary` | Клиент вычислит из списка |
| `POST /storage-elements/sync-all` | Потенциально опасная операция; синхронизировать по одному |
| `GET /internal/storage-elements/available` | Объединено с `GET /storage-elements/` через фильтры |
| `GET /internal/storage-elements/{element_id}` | Дублирует `GET /storage-elements/{id}` |
| Redis Pub/Sub Events | Redis убран из архитектуры |
| `confirm_password` из change-password | Валидация на клиенте |
| `refresh_token` для Service Accounts | SA заново запросят token по credentials |
| `environment` в Service Account | Инфраструктурный аспект |
| `is_system` в Admin Users / SA | Защита через логику, не через поле |
| `rate_limit` в Service Account | Задача API gateway |
| `priority` в Storage Element | Sequential Fill Algorithm убран |
| `is_replicated`, `replica_count` | Репликация убрана |
| Вычисляемые поля (capacity_gb и т.д.) | Клиент рассчитает |
| `secret_history` | Over-engineering |
| `password_history` | Over-engineering |
| Сжатие в File Registry | Дело Storage Element |
| `upload_source_ip` в File Registry | Дело Ingester |

---

## 7. Открытые вопросы

1. **`sync-all`** — убрать совсем или оставить? Может быть полезно для первоначальной настройки.
2. **Формат `client_id`** — оставить читаемый `sa_<name>_<random>` или перейти на UUID?
3. **Secret expiration** — оставить автоматическое истечение через 90 дней или убрать?
4. **Account locking** — оставить механизм блокировки после N попыток? Если да, как разблокировать (таймер или ручной)?
5. **RBAC для Storage Elements** — Service Accounts тоже должны иметь доступ к `GET /storage-elements/` (для Ingester). Как разграничить доступ Admin Users и SA к одному endpoint?
6. **`/health/metrics`** vs **`/metrics`** — какой path для Prometheus?
