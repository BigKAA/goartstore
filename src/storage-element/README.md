# Storage Element

Модуль физического хранения файлов системы Artsore. Обеспечивает атомарные
файловые операции с Write-Ahead Log (WAL), attribute-first метаданные
(`*.attr.json`), lifecycle режимы (`edit` / `rw` / `ro` / `ar`) и
leader/follower репликацию через NFS flock.

## Архитектура

```text
                          ┌──────────────────┐
                          │   Storage Element│
                          │    (Go, HTTPS)   │
                          └──────┬───────────┘
                                 │
              ┌──────────────────┼──────────────────┐
              │                  │                  │
         ┌────▼────┐       ┌────▼────┐        ┌────▼────┐
         │  /data  │       │  /wal   │        │  /certs │
         │ (PVC)   │       │ (PVC)   │        │ (Secret)│
         └─────────┘       └─────────┘        └─────────┘
```

- **/data** — файлы + `*.attr.json` метаданные (единственный источник истины)
- **/wal** — Write-Ahead Log для атомарности операций
- **/certs** — TLS сертификаты (cert-manager)

### Режимы работы

| Режим | Upload | Download | Delete | Update | Описание |
|-------|--------|----------|--------|--------|----------|
| `edit` | + | + | + | + | Полный доступ, изолированный цикл |
| `rw` | + | + | - | + | Чтение/запись, без удаления |
| `ro` | - | + | - | + | Только чтение + обновление метаданных |
| `ar` | - | - | - | + | Архив, только метаданные (list/update) |

Допустимые переходы: `edit` (изолирован), `rw` → `ro` → `ar` (необратимые).
Обратные переходы (`ro` → `rw`) требуют `confirm: true`.

### Режимы деплоя

- **standalone** — один pod (Deployment), отдельные PVC для data и WAL
- **replicated** — N pod'ов (StatefulSet), общий data PVC (RWX, NFS),
  WAL per-pod (RWO). Leader election через `flock()` на
  `/data/.leader.lock`. Follower проксирует write-операции к leader.

## Быстрый старт

### Предварительные требования

- Kubernetes кластер с cert-manager (`ClusterIssuer`)
- StorageClass с поддержкой RWO (standalone) или RWX/NFS (replicated)
- Docker registry (Harbor)

### Сборка образа

```bash
cd src/storage-element
docker build \
  --platform linux/amd64 \
  --build-arg VERSION=v0.1.0 \
  -t harbor.kryukov.lan/library/storage-element:v0.1.0 \
  -f Dockerfile .
docker push harbor.kryukov.lan/library/storage-element:v0.1.0
```

### Деплой standalone SE

```bash
helm install se-rw-01 charts/storage-element \
  --namespace artsore \
  --create-namespace \
  --set elementId=se-rw-01 \
  --set mode=rw \
  --set replicaMode=standalone \
  --set tag=v0.1.0 \
  --set jwksUrl="https://admin-module.artsore.svc.cluster.local:8000/api/v1/auth/jwks" \
  --set tls.clusterIssuer=dev-ca-issuer
```

### Деплой replicated SE

```bash
helm install se-edit-01 charts/storage-element \
  --namespace artsore \
  --set elementId=se-edit-01 \
  --set mode=edit \
  --set replicaMode=replicated \
  --set replicas=2 \
  --set tag=v0.1.0 \
  --set jwksUrl="https://admin-module.artsore.svc.cluster.local:8000/api/v1/auth/jwks" \
  --set tls.clusterIssuer=dev-ca-issuer \
  --set storageClass=nfs-client
```

## Конфигурация

Все параметры настраиваются через переменные окружения или Helm values.

### Основные параметры

| Helm value | Env var | По умолчанию | Описание |
|------------|---------|--------------|----------|
| `elementId` | `SE_STORAGE_ID` | `se-01` | Уникальный ID экземпляра |
| `port` | `SE_PORT` | `8010` | Порт HTTPS-сервера |
| `mode` | `SE_MODE` | `rw` | Начальный режим: edit/rw/ro/ar |
| `replicaMode` | `SE_REPLICA_MODE` | `standalone` | standalone или replicated |
| `replicas` | — | `2` | Количество реплик (replicated) |
| `maxFileSize` | `SE_MAX_FILE_SIZE` | `1073741824` | Макс. размер файла (байт) |
| `logLevel` | `SE_LOG_LEVEL` | `info` | Уровень логирования |
| `logFormat` | `SE_LOG_FORMAT` | `json` | Формат: json или text |

### JWT/JWKS

| Helm value | Env var | Описание |
|------------|---------|----------|
| `jwksUrl` | `SE_JWKS_URL` | URL JWKS endpoint для валидации JWT |
| `jwksCaCert` | `SE_JWKS_CA_CERT` | CA-сертификат для JWKS (опционально) |

### Фоновые процессы

| Helm value | Env var | По умолчанию | Описание |
|------------|---------|--------------|----------|
| `gcInterval` | `SE_GC_INTERVAL` | `1h` | Интервал сборки мусора |
| `reconcileInterval` | `SE_RECONCILE_INTERVAL` | `6h` | Интервал reconciliation |
| `shutdownTimeout` | `SE_SHUTDOWN_TIMEOUT` | `5s` | Таймаут graceful shutdown |

### Replicated mode

| Helm value | Env var | По умолчанию | Описание |
|------------|---------|--------------|----------|
| `indexRefreshInterval` | `SE_INDEX_REFRESH_INTERVAL` | `30s` | Интервал обновления индекса follower |
| `electionRetryInterval` | `SE_ELECTION_RETRY_INTERVAL` | `5s` | Интервал retry захвата flock |

### Хранилище

| Helm value | По умолчанию | Описание |
|------------|--------------|----------|
| `storageClass` | `nfs-client` | StorageClass для PVC |
| `dataSize` | `2Gi` | Размер PVC для данных |
| `walSize` | `1Gi` | Размер PVC для WAL |

### TLS

| Helm value | По умолчанию | Описание |
|------------|--------------|----------|
| `tls.enabled` | `true` | Создать Certificate (cert-manager) |
| `tls.clusterIssuer` | `dev-ca-issuer` | ClusterIssuer для сертификатов |
| `tls.existingSecret` | — | Использовать существующий Secret |

## API

Все endpoints используют HTTPS. Авторизация через JWT Bearer token.

### Публичные endpoints (без авторизации)

| Метод | Путь | Описание |
|-------|------|----------|
| GET | `/health/live` | Liveness probe |
| GET | `/health/ready` | Readiness probe |
| GET | `/api/v1/info` | Информация об экземпляре |
| GET | `/metrics` | Prometheus метрики |

### Файловые операции (требуют JWT)

| Метод | Путь | Scopes | Описание |
|-------|------|--------|----------|
| POST | `/api/v1/files/upload` | `files:write` | Загрузка файла (multipart) |
| GET | `/api/v1/files/{id}/download` | `files:read` | Скачивание файла |
| GET | `/api/v1/files` | `files:read` | Список файлов (limit/offset) |
| GET | `/api/v1/files/{id}` | `files:read` | Метаданные файла |
| PATCH | `/api/v1/files/{id}` | `files:write` | Обновление метаданных |
| DELETE | `/api/v1/files/{id}` | `files:write` | Мягкое удаление (edit mode) |

### Управление

| Метод | Путь | Scopes | Описание |
|-------|------|--------|----------|
| POST | `/api/v1/mode/transition` | `storage:write` | Смена режима |
| POST | `/maintenance/reconcile` | `storage:write` | Ручной reconcile |

## Интеграционные тесты

30 тестов покрывают все аспекты работы SE.

### Запуск тестов

```bash
cd src/storage-element/tests

# Полный цикл
make docker-build        # Сборка SE + JWKS Mock → Harbor
make test-env-up         # Деплой тестовой среды (helm install + init job)
make port-forward-start  # Port-forward ко всем сервисам
make test-all            # Запуск всех 30 тестов

# Очистка
make port-forward-stop
make test-env-down
```

### Группы тестов

| Группа | Тесты | Цель | Команда |
|--------|-------|------|---------|
| Smoke | 1-3 | Health, info, metrics | `make test-smoke` |
| Files | 4-11 | Upload, download, CRUD | `make test-files` |
| Modes | 12-18 | Режимы и переходы | `make test-modes` |
| Replica | 19-22 | Leader/follower, failover | `make test-replica` |
| Data | 23-24 | Инициализированные данные | `make test-data` |
| GC | 25-26 | GC и reconciliation | `make test-gc` |
| Errors | 27-30 | Авторизация и лимиты | `make test-errors` |

### Тестовая среда

Тестовая среда разворачивается через Helm chart `tests/helm/se-test/`:

- **6 SE экземпляров**: se-edit-1 (replicated, 2 реплики), se-edit-2
  (replicated, 2 реплики), se-rw-1, se-rw-2, se-ro, se-ar
- **JWKS Mock** — генерация JWT для тестов
- **Init Job** — загрузка тестовых данных и настройка режимов

## Graceful Shutdown и Failover

### Порядок shutdown

1. `election.Stop()` — освобождение NFS flock (leader lock)
2. HTTP server shutdown — завершение активных запросов
3. Остановка фоновых процессов (GC, reconcile)

### Параметры для failover

- `SE_SHUTDOWN_TIMEOUT` должен быть **меньше** K8s
  `terminationGracePeriodSeconds` (30s по умолчанию)
- `SE_ELECTION_RETRY_INTERVAL` влияет на скорость failover
  (follower проверяет flock с этим интервалом)
- При корректном graceful shutdown failover занимает ~3-5 секунд
- При SIGKILL (без graceful shutdown) — NFS v4 lease timeout (~90s)

## Структура проекта

```text
src/storage-element/
├── cmd/storage-element/         # Точка входа
├── internal/
│   ├── config/                  # Конфигурация (env vars)
│   ├── server/                  # HTTP-сервер, роутинг, middleware
│   ├── handler/                 # HTTP-обработчики (files, mode, health)
│   ├── storage/                 # Файловое хранилище, attr.json, WAL
│   ├── replica/                 # Leader election, proxy, index refresh
│   ├── gc/                      # Garbage Collector
│   ├── reconcile/               # Reconciliation (data ↔ index)
│   ├── dephealth/               # Dependency health checks
│   └── auth/                    # JWT валидация, JWKS клиент
├── charts/storage-element/      # Production Helm chart
├── tests/
│   ├── helm/se-test/            # Тестовый Helm chart
│   ├── scripts/                 # Bash-скрипты тестов
│   ├── jwks-mock/               # JWKS Mock Server
│   └── Makefile                 # Оркестрация тестов
└── Dockerfile
```
