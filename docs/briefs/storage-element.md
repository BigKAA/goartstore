# Storage Element — Бриф модуля

**Версия**: 1.0.0
**Дата**: 2026-02-21
**Статус**: Draft
**Порты**: 8010-8019

---

## 1. Назначение модуля

Storage Element (SE) — модуль физического хранения файлов системы Artsore.
Каждый экземпляр SE управляет локальной файловой системой: принимает, хранит,
отдаёт и удаляет файлы. SE — автономная единица хранения, способная работать
удалённо (вне кластера Kubernetes, через WAN).

### Ключевые концепции

**Attribute-First Storage Model** — каждый файл сопровождается файлом атрибутов
`*.attr.json`, который является единственным источником истины для метаданных.
PostgreSQL файловый реестр (Admin Module) — вторичный индекс,
восстанавливаемый из `attr.json` файлов.

**WAL Protocol** — все операции записи проходят через Write-Ahead Log для
обеспечения атомарности: WAL entry &rarr; запись файла &rarr; создание
`attr.json` &rarr; WAL commit.

**Режимы работы** — SE работает в одном из четырёх режимов (`edit`, `rw`,
`ro`, `ar`), определяющих допустимые операции (см. раздел 4).
Переход между режимами — однонаправленный жизненный цикл:
`edit` &rarr; `rw` &rarr; `ro` &rarr; `ar`. Смена режима выполняется
через API (runtime, без restart).

**Retention Policy** — файлы хранятся с одной из двух политик:

- `temporary` — файлы с TTL (1-365 дней), автоматически удаляются встроенным
  GC по истечении срока
- `permanent` — файлы без срока хранения

**Встроенный GC** — SE самостоятельно удаляет expired файлы по TTL
из `attr.json`. Физическое удаление — soft delete (статус `deleted`
в `attr.json`), затем очистка GC.

**In-memory индекс метаданных** — при старте SE сканирует все `*.attr.json`
и строит в памяти `map[file_id]FileMetadata`. Индекс обеспечивает быструю
фильтрацию, пагинацию и подсчёт `total` для `GET /api/v1/files` без
сканирования диска на каждый запрос. При операциях записи (upload, update,
delete) индекс обновляется синхронно вместе с `attr.json`. Индекс —
производный кэш, полностью пересобирается при старте и reconciliation.
Не персистентный, не требует внешних зависимостей (без SQLite/BoltDB).

**Встроенный Reconciliation** — периодическая сверка `attr.json` с файловой
системой. Выявляет orphaned files, missing files, несоответствия checksum
и размера. Пересобирает in-memory индекс. Запускается автоматически
по интервалу и вручную через API.

---

## 2. Топология

Storage Element располагается **вне кластера Kubernetes** (remote).
Может работать на bare metal, в другом кластере или на edge-локации.

```text
┌─────────────────────────────────────┐
│         Kubernetes кластер          │
│                                     │
│  ┌──────────┐  ┌──────────────────┐ │
│  │  Admin    │  │  Ingester /      │ │
│  │  Module   │  │  Query Module    │ │
│  └────┬─────┘  └───────┬──────────┘ │
│       │                │            │
└───────┼────────────────┼────────────┘
        │   WAN / TLS    │
        ▼                ▼
   ┌──────────┐    ┌──────────┐
   │ SE #1    │    │ SE #2    │
   │ (remote) │    │ (remote) │
   └──────────┘    └──────────┘
```

**Следствия:**

- Все соединения к SE — через TLS (потенциально WAN, ненадёжная сеть)
- SE должен быть устойчив к сбоям сети
- Аутентификация — JWT RS256, публичный ключ получается от Admin Module
  через JWKS endpoint

---

## 3. Зависимости

### Инфраструктурные

| Зависимость | Назначение |
|-------------|------------|
| Local filesystem | Хранение файлов и `attr.json` (единственное хранилище, без БД) |
| TLS сертификаты | Шифрование соединений (cert-manager / ручные) |

SE не использует PostgreSQL, Redis или другие внешние хранилища данных.
Вся информация хранится в файловой системе: файлы данных + `*.attr.json`.

### Межмодульные

| Модуль | Направление | Назначение |
|--------|-------------|------------|
| Admin Module | SE &larr; Admin | JWT JWKS endpoint — получение публичного ключа для валидации токенов |
| Admin Module | SE &larr; Admin | Регистрация SE, синхронизация файлового реестра, смена режима |
| Ingester Module | SE &larr; Ingester | Загрузка файлов через `POST /api/v1/files/upload` |
| Query Module | SE &larr; Query | Скачивание файлов через `GET /api/v1/files/{file_id}/download` |

---

## 4. Режимы работы

| Режим | Upload | Download | Update | Delete | List/Meta | Описание |
|-------|:------:|:--------:|:------:|:------:|:---------:|----------|
| `edit` | да | да | да | да | да | Полный доступ (черновики, temporary файлы) |
| `rw` | да | да | да | нет | да | Чтение и запись (permanent файлы) |
| `ro` | нет | да | нет | нет | да | Только чтение |
| `ar` | нет | нет | нет | нет | да | Архив (только метаданные и листинг) |

Переходы между режимами — **однонаправленные**:

```text
edit ──► rw ──► ro ──► ar
```

Обратные переходы и пропуск шагов не допускаются. Переход выполняется
через `POST /api/v1/mode/transition` (runtime, без restart), вызывается
Admin Module.

---

## 5. API endpoints

12 endpoints, сгруппированных по назначению. Полная спецификация —
[storage-element-openapi.yaml](../api-contracts/storage-element-openapi.yaml).

### File Operations (6 endpoints)

| Метод | Endpoint | Назначение | Режимы |
|-------|----------|------------|--------|
| `POST` | `/api/v1/files/upload` | Загрузка файла (multipart/form-data) | `edit`, `rw` |
| `GET` | `/api/v1/files` | Список файлов (пагинация, фильтр по status) | все |
| `GET` | `/api/v1/files/{file_id}` | Метаданные файла | все |
| `PATCH` | `/api/v1/files/{file_id}` | Обновление метаданных (description, tags) | `edit`, `rw` |
| `DELETE` | `/api/v1/files/{file_id}` | Удаление файла (soft delete) | `edit` |
| `GET` | `/api/v1/files/{file_id}/download` | Скачивание файла (streaming, Range requests) | `edit`, `rw`, `ro` |

### System (1 endpoint)

| Метод | Endpoint | Назначение | Аутентификация |
|-------|----------|------------|----------------|
| `GET` | `/api/v1/info` | Информация о SE (discovery, capacity, mode) | без аутентификации |

### Mode (1 endpoint)

| Метод | Endpoint | Назначение | Аутентификация |
|-------|----------|------------|----------------|
| `POST` | `/api/v1/mode/transition` | Смена режима работы (runtime) | JWT `storage:write` |

### Maintenance (1 endpoint)

| Метод | Endpoint | Назначение | Аутентификация |
|-------|----------|------------|----------------|
| `POST` | `/api/v1/maintenance/reconcile` | Ручная сверка attr.json с filesystem | JWT `storage:write` |

### Health (3 endpoints)

| Метод | Endpoint | Назначение | Аутентификация |
|-------|----------|------------|----------------|
| `GET` | `/health/live` | Liveness probe (процесс жив) | без аутентификации |
| `GET` | `/health/ready` | Readiness probe (filesystem, WAL, индекс) | без аутентификации |
| `GET` | `/metrics` | Prometheus metrics | без аутентификации |

---

## 6. Аутентификация

JWT RS256 токены, выданные Admin Module.

**Публичные endpoints** (без аутентификации):

- `/health/live`, `/health/ready`, `/metrics` — Kubernetes probes и мониторинг
- `/api/v1/info` — discovery и регистрация SE

**Защищённые endpoints** — требуют JWT Bearer token с соответствующим scope:

| Scope | Операции |
|-------|----------|
| `files:read` | Список файлов, метаданные, скачивание |
| `files:write` | Загрузка, обновление метаданных, удаление |
| `storage:read` | Чтение информации о SE (зарезервирован) |
| `storage:write` | Смена режима работы, запуск reconciliation |

Валидация JWT:

- Алгоритм: RS256
- Публичный ключ: получается через JWKS endpoint Admin Module
- Claims: `sub` (идентификатор субъекта), `scopes` (массив scopes)

---

## 7. Конфигурация

Все параметры задаются через переменные окружения.

| Переменная | Обязательная | По умолчанию | Описание |
|------------|:------------:|--------------|----------|
| `SE_PORT` | нет | `8010` | Порт HTTP-сервера (диапазон 8010-8019) |
| `SE_STORAGE_ID` | да | — | Уникальный идентификатор SE (например, `se-moscow-01`) |
| `SE_DATA_DIR` | да | — | Путь к директории хранения файлов |
| `SE_WAL_DIR` | да | — | Путь к директории WAL |
| `SE_MODE` | нет | `edit` | Начальный режим работы (`edit`, `rw`, `ro`, `ar`) |
| `SE_MAX_FILE_SIZE` | нет | `1073741824` | Максимальный размер файла в байтах (default 1 GB) |
| `SE_GC_INTERVAL` | нет | `1h` | Интервал запуска GC (Go duration: `30m`, `1h`, `24h`) |
| `SE_RECONCILE_INTERVAL` | нет | `6h` | Интервал автоматической сверки (Go duration) |
| `SE_JWKS_URL` | да | — | URL JWKS endpoint Admin Module для валидации JWT |
| `SE_TLS_CERT` | да | — | Путь к TLS сертификату |
| `SE_TLS_KEY` | да | — | Путь к TLS приватному ключу |
| `SE_LOG_LEVEL` | нет | `info` | Уровень логирования (`debug`, `info`, `warn`, `error`) |
| `SE_LOG_FORMAT` | нет | `json` | Формат логов (`json` — production, `text` — development) |

---

## 8. Синхронизация файлового реестра

SE предоставляет endpoint `GET /api/v1/files` для синхронизации файлового
реестра Admin Module. `attr.json` — единственный источник истины.

### Как это работает

1. Admin Module периодически (по таймеру, default 1 час) опрашивает каждый
   зарегистрированный SE
2. Постранично вычитывает `GET /api/v1/files` (пагинация: `limit`/`offset`)
3. Для каждого файла из ответа SE:
   - Если файл есть в реестре — обновить метаданные
   - Если файла нет в реестре — добавить запись
4. Файлы в реестре, привязанные к этому SE, но отсутствующие в ответе SE —
   пометить как `deleted`

### Восстановление после сбоя PostgreSQL

1. Восстановить PostgreSQL из backup (или развернуть чистый)
2. Административные данные — из backup
3. Файловый реестр — автоматический full sync при подключении каждого SE
4. Система полностью работоспособна после завершения sync всех SE

---

## 9. Сборка и запуск

### Docker

```bash
# Сборка образа
docker build -t harbor.kryukov.lan/library/storage-element:v1.0.0 \
  -f storage-element/Dockerfile .

# Запуск контейнера
docker run -d \
  --name storage-element \
  -p 8010:8010 \
  -v /data/storage:/data \
  -v /data/wal:/wal \
  -e SE_STORAGE_ID=se-local-01 \
  -e SE_DATA_DIR=/data \
  -e SE_WAL_DIR=/wal \
  -e SE_JWKS_URL=https://admin.kryukov.lan/api/v1/auth/jwks \
  -e SE_TLS_CERT=/certs/tls.crt \
  -e SE_TLS_KEY=/certs/tls.key \
  -v /path/to/certs:/certs:ro \
  harbor.kryukov.lan/library/storage-element:v1.0.0
```

### Kubernetes (Helm)

```bash
# Установка через Helm chart
helm install se-moscow-01 ./storage-element/chart \
  --set storageId=se-moscow-01 \
  --set dataDir=/data \
  --set walDir=/wal \
  --set jwksUrl=https://admin.kryukov.lan/api/v1/auth/jwks \
  --set tls.certManager.issuer=dev-ca-issuer
```

---

## 10. Порты

| Порт | Назначение |
|------|------------|
| 8010-8019 | HTTP/TLS API Storage Element (по одному порту на экземпляр) |

По умолчанию используется порт `8010`. При запуске нескольких экземпляров
на одном хосте — назначаются последующие порты из диапазона.
