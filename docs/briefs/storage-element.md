# Storage Element — Бриф модуля

**Версия**: 0.1.0
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

## 2. Топология и модели развёртывания

### 2.1. Общая топология

Admin Module, Ingester Module и Query Module работают в кластере Kubernetes.
Storage Element может работать **в любом окружении**: bare metal, отдельный
кластер K8s, edge-локация, Docker-контейнер на хосте.

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

### 2.2. Модели развёртывания

SE поддерживает две модели развёртывания:

**Standalone** — один экземпляр SE с локальным хранилищем (или выделенным
NFS-томом). Простейшая конфигурация, подходит для большинства сценариев.

```text
┌──────────────┐
│  SE (один)   │
│  local disk  │
│  или NFS PV  │
└──────────────┘
```

**Replicated** — несколько реплик одного SE, работающих с общей файловой
системой (NFS). Обеспечивает высокую доступность (HA). Может работать
в Kubernetes (StatefulSet + NFS PVC ReadWriteMany) или как контейнеры
на нескольких хостах с общим NFS-монтированием.

```text
┌──────────────┐    ┌──────────────┐
│  SE Leader   │    │  SE Follower  │
│  (read+write)│    │  (read only)  │
└──────┬───────┘    └──────┬───────┘
       │                   │
       └─────────┬─────────┘
                 ▼
        NFS (ReadWriteMany)
        /data/se-shared/
```

Подробности replicated-режима — в разделе 11 «Горизонтальное масштабирование».

### 2.3. Общие следствия

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

### Два жизненных цикла

SE создаётся в одном из двух стартовых режимов, определяющих его жизненный
цикл навсегда:

**Цикл 1: Temporary storage (edit)**

```text
edit  (замкнутый, без переходов)
```

Режим `edit` — изолированный. SE в режиме `edit` предназначен для черновиков
и temporary-файлов. Переход в другие режимы **невозможен**. Обратный переход
из других режимов в `edit` также запрещён.

**Цикл 2: Permanent storage (rw → ro → ar)**

```text
rw ──► ro ──► ar
         ◄──
      (rollback,
    confirm=true)
```

Штатные переходы: `rw` → `ro` → `ar`. Пропуск шагов не допускается
(например, `rw` → `ar` запрещён).

**Откат `ro` → `rw`** — единственный разрешённый обратный переход.
Предназначен для исправления ошибочного перевода SE в read-only.
Требует явного подтверждения (`confirm: true` в запросе). Без подтверждения
возвращается ошибка `409 Conflict` с кодом `CONFIRMATION_REQUIRED`.
Откат логируется с указанием субъекта (`sub` из JWT) и времени.

### Запрещённые переходы

| Переход | Статус |
|---------|--------|
| `edit` → `rw`/`ro`/`ar` | ❌ запрещён (edit изолирован) |
| `rw`/`ro`/`ar` → `edit` | ❌ запрещён |
| `ar` → `ro` | ❌ запрещён |
| `ar` → `rw` | ❌ запрещён |
| `rw` → `ar` | ❌ запрещён (пропуск шага) |

### API перехода

Переход выполняется через `POST /api/v1/mode/transition` (runtime,
без restart), вызывается Admin Module.

```json
// Штатный переход rw → ro
{"target_mode": "ro"}

// Откат ro → rw (требует подтверждения)
{"target_mode": "rw", "confirm": true}
```

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
| `SE_REPLICA_MODE` | нет | `standalone` | Режим развёртывания: `standalone` или `replicated` |
| `SE_INDEX_REFRESH_INTERVAL` | нет | `30s` | Интервал обновления индекса на follower (Go duration, только для replicated) |
| `SE_DEPHEALTH_CHECK_INTERVAL` | нет | `15s` | Интервал проверки зависимостей topologymetrics (Go duration) |

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
docker build -t harbor.kryukov.lan/library/storage-element:v0.1.0 \
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
  harbor.kryukov.lan/library/storage-element:v0.1.0
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

---

## 11. Горизонтальное масштабирование (Replicated Mode)

При `SE_REPLICA_MODE=replicated` несколько экземпляров SE работают
с общей файловой системой (NFS v4+). Используется паттерн
**Leader / Follower**.

### 11.1. Leader Election

Выбор лидера реализуется через **file lock** на общей файловой системе:

1. При старте SE пытается захватить `flock()` на файл
   `{SE_DATA_DIR}/.leader.lock`
2. Если блокировка получена — SE становится **leader**
3. Если нет — SE становится **follower** и периодически повторяет попытку
4. При падении leader-процесса — блокировка освобождается ОС,
   один из follower захватывает её и становится новым leader

Механизм универсальный: работает в Kubernetes, на bare metal, в Docker —
везде, где есть общий NFS v4+. Не требует внешних зависимостей (etcd,
Redis и т.д.).

### 11.2. Разделение обязанностей

| Операция | Leader | Follower |
|----------|:------:|:--------:|
| Upload | ✅ | ❌ proxy к leader |
| Download | ✅ | ✅ (чтение с NFS) |
| List / Metadata | ✅ | ✅ (свой in-memory индекс) |
| Update metadata | ✅ | ❌ proxy к leader |
| Delete | ✅ | ❌ proxy к leader |
| GC | ✅ | ❌ |
| Reconciliation | ✅ | ❌ |
| Mode transition | ✅ | ❌ proxy к leader |
| WAL | ✅ | ❌ (нет операций записи) |

### 11.3. In-memory индекс на follower

Follower строит свой in-memory индекс при старте (сканирование `attr.json`
файлов на NFS). Индекс обновляется периодически по таймеру
(`SE_INDEX_REFRESH_INTERVAL`, default 30 секунд). Это означает, что
метаданные на follower могут отставать от leader на величину интервала
обновления.

### 11.4. Mode на общем NFS

Текущий режим SE хранится в файле `{SE_DATA_DIR}/mode.json` на NFS.
Leader записывает файл при переходе режима. Все реплики читают
этот файл при старте и при обновлении индекса.

### 11.5. Proxy write-запросов

Когда follower получает write-запрос (upload, update, delete,
mode transition), он проксирует его к leader-поду. Follower определяет
адрес leader через файл `{SE_DATA_DIR}/.leader.info`, который leader
обновляет при захвате блокировки (содержит `host:port` leader-а).

### 11.6. Развёртывание в Kubernetes

```yaml
# StatefulSet с NFS PVC (ReadWriteMany)
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: se-moscow-01
spec:
  replicas: 2
  # ...
  volumeClaimTemplates: []   # используется общий PVC
  template:
    spec:
      volumes:
        - name: data
          persistentVolumeClaim:
            claimName: se-moscow-01-data   # NFS ReadWriteMany
      containers:
        - name: storage-element
          env:
            - name: SE_REPLICA_MODE
              value: "replicated"
            - name: SE_STORAGE_ID
              value: "se-moscow-01"
```

### 11.7. Развёртывание на хостах (Docker)

```bash
# На хосте 1 (потенциальный leader)
docker run -d \
  -v /mnt/nfs/se-data:/data \
  -e SE_REPLICA_MODE=replicated \
  -e SE_STORAGE_ID=se-moscow-01 \
  storage-element:v0.1.0

# На хосте 2 (потенциальный follower)
docker run -d \
  -v /mnt/nfs/se-data:/data \
  -e SE_REPLICA_MODE=replicated \
  -e SE_STORAGE_ID=se-moscow-01 \
  storage-element:v0.1.0
```

Оба контейнера монтируют один и тот же NFS-том. Leader определяется
автоматически через file lock.

---

## 12. Мониторинг зависимостей (topologymetrics)

SE интегрируется с SDK
[topologymetrics](https://github.com/BigKAA/topologymetrics)
для мониторинга здоровья внешних зависимостей через Prometheus-метрики.

### 12.1. Отслеживаемые зависимости

| Зависимость | Тип проверки | Критичность |
|-------------|-------------|:-----------:|
| Admin Module JWKS | HTTP (GET) | да |

### 12.2. Экспортируемые метрики

| Метрика | Тип | Описание |
|---------|-----|----------|
| `app_dependency_health` | Gauge | 1 = доступен, 0 = недоступен |
| `app_dependency_latency_seconds` | Histogram | Время проверки |
| `app_dependency_status` | Gauge | Категория результата (ok, timeout, error...) |
| `app_dependency_status_detail` | Gauge | Детальная причина |

Метрики доступны на endpoint `/metrics` вместе с остальными
Prometheus-метриками SE.

### 12.3. Labels

Все метрики имеют обязательные labels: `name` (storage-element),
`group` (artsore), `dependency`, `type`, `host`, `port`, `critical`.

### 12.4. Интеграция в коде

```go
import (
    "github.com/BigKAA/topologymetrics/sdk-go/dephealth"
    _ "github.com/BigKAA/topologymetrics/sdk-go/dephealth/checks"
)

dh, err := dephealth.New("storage-element", "artsore",
    dephealth.WithCheckInterval(cfg.DephealthCheckInterval),
    dephealth.HTTP("admin-jwks",
        dephealth.FromURL(cfg.JWKSUrl),
        dephealth.Critical(true),
    ),
)
dh.Start(ctx)
defer dh.Stop()
```
