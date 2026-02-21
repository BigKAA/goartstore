# Ревизия API: Ingester Module

**Дата**: 2026-02-21
**Статус**: Draft
**Модуль**: Ingester Module (old_artstore/ingester-module/)

---

## 1. Сводка текущего API

Ingester Module содержит **3 группы endpoints**:

| # | Группа | Prefix | Endpoints | Назначение |
|---|--------|--------|-----------|------------|
| 1 | Upload | `/api/v1/files/*` | 2 (1 stub) | Загрузка файлов в Storage Element |
| 2 | Finalize | `/api/v1/finalize/*` | 2 | Two-Phase Commit финализация temporary → permanent |
| 3 | Health | `/health/*` | 2 | Kubernetes probes |

**Итого: 6 endpoints (из них 1 stub, 2 подлежат удалению)**

### Текущие endpoints

| Метод | Путь | Статус | Назначение |
|-------|------|--------|------------|
| POST | `/api/v1/files/upload` | Рабочий | Загрузка файла |
| GET | `/api/v1/files/` | Stub | Список файлов (не реализован) |
| POST | `/api/v1/finalize/{file_id}` | Рабочий | Запуск финализации (Two-Phase Commit) |
| GET | `/api/v1/finalize/{transaction_id}/status` | Рабочий | Статус транзакции финализации |
| GET | `/health/live` | Рабочий | Liveness probe |
| GET | `/health/ready` | Рабочий | Readiness probe (с подробной диагностикой) |

---

## 2. Выявленные проблемы и несоответствия

### 2.1. Весь Finalize API — это Two-Phase Commit (удаляется)

Согласно `docs/REQUIREMENTS.md`, Two-Phase Commit **убираем из новой архитектуры**. Это означает, что вся группа Finalize (2 endpoints) подлежит удалению:

- `POST /finalize/{file_id}` — запуск финализации
- `GET /finalize/{transaction_id}/status` — polling статуса

**В новой архитектуре**: файлы загружаются сразу в нужный SE на основе `retention_policy`. Нет промежуточной стадии копирования.

**Связанные схемы для удаления:**
- `FinalizeTransactionStatus` (enum: copying, copied, verifying, completed, failed, rolled_back)
- `FinalizeRequest`
- `FinalizeResponse`
- `FinalizeStatus`

### 2.2. Deprecated параметр `storage_mode` в Upload

Endpoint `POST /files/upload` принимает deprecated параметр `storage_mode` наряду с `retention_policy`. В коде есть model_validator, который **перезаписывает** `storage_mode` на основе `retention_policy`:

```python
# Маппинг:
# TEMPORARY → edit SE
# PERMANENT → rw SE
```

**Решение**: В новом API удалить `storage_mode` полностью. Оставить только `retention_policy`.

### 2.3. Stub endpoint GET /files/

Endpoint `GET /api/v1/files/` существует как stub:

```python
return {"message": "Not implemented yet", "user": user.username}
```

**Решение**: Не переносить. Получение списка файлов — задача Query Module.

### 2.4. Тяжёлые Redis-зависимости

Модуль зависит от Redis в нескольких местах:

| Компонент | Redis-зависимость | Статус |
|-----------|-------------------|--------|
| Service Discovery | Redis Pub/Sub для обнаружения SE | Удаляется (REQUIREMENTS.md) |
| Capacity Monitor | Redis для кэша ёмкости + Leader Election | Удаляется (Adaptive Capacity Thresholds) |
| StorageSelector | Redis для получения доступных SE | Заменяется на Admin Module API |
| Health check | Проверка Redis в readiness probe | Не нужен |

**В новой архитектуре**: Ingester узнаёт о доступных SE через API Admin Module (endpoint `GET /api/v1/internal/storage-elements/available`).

### 2.5. Сложный readiness probe

Текущий readiness probe проверяет **6 компонентов** и возвращает развёрнутую диагностику:

- Redis (Service Discovery)
- Admin Module (fallback)
- Capacity Monitor (Leader Election, storage_elements_count)
- Все writable SE (edit/rw) по отдельности
- Edit storage (наличие хотя бы 1 edit SE)
- Data sources (polling_model, admin_module, fallback_chain)

Это чрезмерно для readiness probe. Kubernetes probe должен быть быстрым и надёжным.

**Решение**: Упростить readiness — проверять только критические зависимости:
- Admin Module доступен
- Хотя бы 1 SE для каждой необходимой `retention_policy` доступен

### 2.6. Нет endpoint /metrics

В router отсутствует endpoint для метрик (Prometheus). В Admin Module и Storage Element тоже нет, но для Ingester — основного write path — метрики критичны (throughput, latency, ошибки загрузки).

### 2.7. Upload Response раскрывает внутренние данные

Ответ `POST /files/upload` содержит `storage_element_url` (прямой URL SE):

```json
{
  "storage_element_url": "http://se-01:8010",
  "storage_element_id": "se-01"
}
```

Клиент не должен знать прямые URL Storage Element. Это внутренняя инфраструктура.

**Решение**: Убрать `storage_element_url`. `storage_element_id` — на усмотрение (может быть полезен для дебага).

### 2.8. Формат ошибок — FastAPI default

Ошибки возвращаются в формате FastAPI по умолчанию:

```json
{"detail": "Error message description"}
```

Но в `upload.py` определена модель `UploadError` с полями `error_code`, `error_message`, `details` — которая **нигде не используется**.

**Решение**: Использовать единый формат ошибок системы (будет определён при унификации).

### 2.9. Compression — решить о сохранении

Upload поддерживает сжатие с двумя алгоритмами:

```
compress: bool (default: false)
compression_algorithm: gzip | brotli
```

Сжатие происходит на стороне Ingester перед отправкой в SE. Это полезная функция, но нужно решить:
- Оставляем ли мы сжатие в Ingester или переносим в SE?
- Нужно ли поддерживать brotli, или достаточно gzip?

### 2.10. Metadata как JSON-строка в multipart form

Параметр `metadata` принимается как JSON-строка в form data и парсится вручную:

```python
parsed_metadata = json.loads(metadata)
```

Это ограничение multipart/form-data — нельзя передать структурированные данные напрямую. Решение корректное, но нужна документация формата.

---

## 3. Анализ по группам

### 3.1. Upload API

#### POST /files/upload — СОХРАНИТЬ, УПРОСТИТЬ

**Текущие параметры (multipart/form-data):**

| Параметр | Тип | Обязательный | Статус |
|----------|-----|--------------|--------|
| `file` | binary | Да | Сохранить |
| `description` | string | Нет | Сохранить |
| `retention_policy` | string | Нет (def: temporary) | Сохранить |
| `ttl_days` | integer | Нет (def: 30) | Сохранить |
| `storage_mode` | string | Нет | **Удалить** (deprecated) |
| `compress` | boolean | Нет (def: false) | Сохранить (обсудить) |
| `compression_algorithm` | string | Нет (def: gzip) | Сохранить (обсудить) |
| `metadata` | string (JSON) | Нет | Сохранить |

**Текущий ответ (UploadResponse):**

| Поле | Тип | Статус |
|------|-----|--------|
| `file_id` | UUID | Сохранить |
| `original_filename` | string | Сохранить |
| `storage_filename` | string | Обсудить (внутренняя деталь?) |
| `file_size` | integer | Сохранить |
| `compressed` | boolean | Сохранить (если оставляем сжатие) |
| `compression_ratio` | float | Сохранить (если оставляем сжатие) |
| `checksum` | string | Сохранить |
| `uploaded_at` | datetime | Сохранить |
| `storage_element_url` | string | **Удалить** (внутренняя деталь) |
| `retention_policy` | string | Сохранить |
| `ttl_expires_at` | datetime | Сохранить |
| `storage_element_id` | string | Обсудить (полезно для дебага) |

#### GET /files/ — УДАЛИТЬ (stub)

Не реализован. Получение списка файлов — задача Query Module.

### 3.2. Finalize API — ПОЛНОСТЬЮ УДАЛИТЬ

Вся группа Finalize — это Two-Phase Commit, который удаляется из новой архитектуры.

| Endpoint | Назначение | Решение |
|----------|------------|---------|
| POST /finalize/{file_id} | Запуск финализации | Удалить |
| GET /finalize/{transaction_id}/status | Polling статуса | Удалить |

**Альтернатива в новой архитектуре**: При загрузке файл сразу попадает в нужный SE. Если нужно сменить `retention_policy` — это операция обновления метаданных, а не двухфазный коммит.

### 3.3. Health API — УПРОСТИТЬ

#### GET /health/live — СОХРАНИТЬ

Минимальный liveness probe. Текущая реализация корректна.

**Текущий ответ:**

```json
{
  "status": "ok",
  "timestamp": "2025-01-27T14:30:45Z",
  "version": "0.1.0",
  "service": "artstore-ingester"
}
```

#### GET /health/ready — СОХРАНИТЬ, УПРОСТИТЬ

Текущая реализация слишком сложная (Redis, Capacity Monitor, Leader Election, Data Sources). После удаления Redis-зависимостей проверять:

1. Admin Module доступен (`/health/live`)
2. Хотя бы 1 SE доступен для каждой retention_policy

**Статусы**: `ok` (200), `degraded` (200), `fail` (503) — оставить.

---

## 4. Удаляемые зависимости и компоненты

### Компоненты для удаления (согласно REQUIREMENTS.md)

| Компонент | Причина удаления |
|-----------|-----------------|
| Two-Phase Commit (Finalize API) | Убираем из архитектуры |
| Redis Service Discovery | Заменяется Admin Module API |
| Capacity Monitor (Leader Election) | Убираем Adaptive Capacity Thresholds |
| StorageSelector (Redis-based) | Заменяется на простой запрос к Admin Module |
| HealthReporter (Redis push) | Удалён ещё в Sprint 19 |

### Перечисления для удаления

| Enum | Причина |
|------|---------|
| `FinalizeTransactionStatus` | Two-Phase Commit удалён |
| `StorageMode` (в ingester) | Дублирует Admin Module, заменён на `retention_policy` |

### Схемы для удаления

| Схема | Причина |
|-------|---------|
| `FinalizeRequest` | Two-Phase Commit |
| `FinalizeResponse` | Two-Phase Commit |
| `FinalizeStatus` | Two-Phase Commit |
| `UploadProgress` | Не используется в endpoint (определена, но не подключена) |
| `UploadError` | Не используется (определена, но ошибки через HTTPException) |

---

## 5. Рекомендации для нового API

### 5.1. Таблица endpoints нового Ingester Module

| Метод | Путь | Назначение |
|-------|------|------------|
| POST | `/api/v1/files/upload` | Загрузка файла |
| GET | `/health/live` | Liveness probe |
| GET | `/health/ready` | Readiness probe |

**Итого: 3 endpoints** (было 6, из них 1 stub)

### 5.2. Новый UploadRequest (multipart/form-data)

| Параметр | Тип | Обязательный | Default | Описание |
|----------|-----|--------------|---------|----------|
| `file` | binary | Да | — | Файл для загрузки |
| `description` | string | Нет | null | Описание файла (max 1000) |
| `retention_policy` | string | Нет | `temporary` | `temporary` или `permanent` |
| `ttl_days` | integer | Нет | 30 | TTL для temporary (1-365) |
| `compress` | boolean | Нет | false | Включить сжатие |
| `compression_algorithm` | string | Нет | `gzip` | `gzip` или `brotli` |
| `metadata` | string (JSON) | Нет | null | Пользовательские метаданные |

### 5.3. Новый UploadResponse

| Поле | Тип | Описание |
|------|-----|----------|
| `file_id` | UUID | ID загруженного файла |
| `original_filename` | string | Оригинальное имя файла |
| `file_size` | integer | Размер в байтах |
| `compressed` | boolean | Был ли файл сжат |
| `compression_ratio` | float/null | Коэффициент сжатия |
| `checksum` | string | SHA256 checksum |
| `uploaded_at` | datetime | Timestamp загрузки |
| `retention_policy` | string | Политика хранения |
| `ttl_expires_at` | datetime/null | Дата истечения TTL |

**Удалено из ответа:**
- `storage_filename` — внутренняя деталь SE
- `storage_element_url` — внутренняя деталь инфраструктуры
- `storage_element_id` — можно вернуть если нужен для дебага

### 5.4. Внутренний workflow загрузки (без Two-Phase Commit)

```
Клиент → POST /files/upload (retention_policy=temporary|permanent)
  → Ingester определяет целевой SE через Admin Module API
  → Ingester загружает файл в SE (streaming)
  → Ingester регистрирует файл в Admin Module file registry
  → Клиент получает UploadResponse
```

Для `temporary`: файл загружается в Edit SE с TTL. GC Storage Element удалит файл по истечении TTL.
Для `permanent`: файл загружается напрямую в RW SE. Нет TTL, нет финализации.

---

## 6. Открытые вопросы

1. **Сжатие**: Оставляем в Ingester или переносим логику сжатия в SE?
   Если в Ingester — дополнительная нагрузка на CPU. Если в SE — дополнительная нагрузка на SE и увеличение сетевого трафика.

2. **Brotli**: Нужен ли brotli, или достаточно gzip? Brotli лучше сжимает, но медленнее.

3. **storage_filename в ответе**: Раскрывать ли клиенту внутреннее имя файла в SE?
   В текущей реализации формат: `{name}_{timestamp}_{uuid_prefix}.{ext}`

4. **storage_element_id в ответе**: Полезно для дебага, но раскрывает внутреннюю топологию.

5. **Смена retention_policy**: Если пользователь хочет перевести temporary → permanent (бывший Finalize), какой endpoint использовать? Варианты:
   - PATCH на файле в Admin Module (обновление метаданных)
   - Отдельный endpoint в Ingester (с физическим копированием файла)
   - Не поддерживать (загружать заново)

6. **Streaming upload progress**: Схема `UploadProgress` определена но не используется. Нужен ли WebSocket/SSE для отслеживания прогресса больших файлов?

7. **Максимальный размер файла**: В текущем API нет явного ограничения. Нужно ли вводить лимит?
