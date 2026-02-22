# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Обзор проекта

Artsore — переработка проекта ArtStore (распределённое файловое хранилище с микросервисной архитектурой). Старый код на Python/FastAPI находится в `old_artstore/` как справочный материал. Новая разработка ведётся на **Go**, каждый модуль — отдельный независимый проект в своей директории.

### Цель переработки (из task.md)

- Разделить модули на отдельные независимые проекты
- Провести ревизию API каждого модуля, унифицировать контракты
- Перевести на Go
- Начать разработку каждого модуля с чистого листа, опираясь на API-контракт и документацию

### Модули системы (порты из old_artstore)

| Модуль | Порты | Назначение |
|--------|-------|------------|
| Admin Module | 8000-8009 | OAuth 2.0 JWT (RS256), управление, Saga координация |
| Storage Element | 8010-8019 | Физическое хранение файлов, WAL, attr.json |
| Ingester Module | 8020-8029 | Streaming upload, валидация, сжатие |
| Query Module | 8030-8039 | Поиск (PostgreSQL FTS), multi-level caching |
| Admin UI | 4200 | Веб-интерфейс (Angular) |

## Общие правила

- Общаться на русском языке
- Комментарии и документацию вести на русском языке
- Подробные комментарии в коде
- Если не знаешь ответа — останови выполнение, спроси у пользователя
- Все md файлы проверять linter
- Разработку, отладку и тестирование вести **только через Docker или Kubernetes**
- Доменные имена для тестов/разработки добавлять в файл hosts (просить пользователя о ручном добавлении)

## Инструменты

- `kubectl` — настроен на работу с тестовым кластером Kubernetes
- `helm` — для работы с helm charts
- `docker` — работа с контейнерами

## Хранилище контейнеров

Harbor: https://harbor.kryukov.lan — admin/password. Использовать публичный проект `library`.

## Тестовый кластер Kubernetes

- Gateway API (Envoy Gateway, gatewayClassName: `eg`)
- MetalLB (LoadBalancer, IP pool: 192.168.218.180-190)
- Ingress controller отсутствует
- cert-manager (ClusterIssuer: `dev-ca-issuer`)
- Домены для отладки: `test1.kryukov.lan`, `test2.kryukov.lan`, `test3.kryukov.lan` → 192.168.218.180
- Домен проекта: `artsore.kryukov.lan` → 192.168.218.180 (внешний доступ к API Gateway)
- DNS сервер: 192.168.218.9

## Git Workflow (GitHub Flow + Semver Tags)

**Обязательно** следовать при любых изменениях файлов проекта. Полное описание: `GIT-WORKFLOW.md`.

### Ветки

Основная ветка — `master` (всегда deployable). Feature branches от master:
- `feature/`, `bugfix/`, `docs/`, `refactor/`, `test/`, `hotfix/`

### Commits — Conventional Commits

```
<type>(<scope>): <subject>
```
Типы: `feat`, `fix`, `docs`, `style`, `refactor`, `test`, `chore`

### Workflow

1. `git checkout master && git pull` → `git checkout -b <type>/<description>`
2. Работа в ветке
3. Спросить пользователя: «Создать commit?»
4. После commit — предложить: локальный merge (`--no-ff`) или GitHub PR
5. Удалить ветку после merge

### Теги образов

- При разработке: `vX.Y.Z-N` (суффикс инкрементируется)
- При релизе: `vX.Y.Z` (суффикс убирается, patch увеличивается)

## Планы разработки

- Шаблон: `.plantemplates/DEVELOPMENT_PLAN_TEMPLATE.md`
- План должен быть подробным, разбит на фазы
- Одна фаза — один контекст AI
- После выполнения — отметить как завершённый
- Завершённые планы переносить в `plans/archive/`

## Структура директорий

```
.plantemplates/    — Шаблоны для планов разработки
old_artstore/      — Старый проект (Python/FastAPI) как справочный материал
src/               — Исходные коды (будет создана)
plans/             — Планы разработки и тестирования
docs/              — Документация
```

## Справка по старому проекту (old_artstore/)

Ключевые архитектурные концепции из старого проекта, которые должны быть перенесены:

- **Attribute-First Storage Model**: `*.attr.json` — единственный источник истины для метаданных
- **WAL Protocol**: Write-Ahead Log для атомарности операций с файлами
- **Service Discovery**: Redis Pub/Sub, fallback на Admin Module API
- **Sequential Fill Algorithm**: выбор Storage Element по priority и capacity
- **Storage Element Modes**: edit → rw → ro → ar (жизненный цикл хранилища)
- **Two-Phase Commit Finalization**: перенос файлов из edit SE в rw SE
- **Adaptive Capacity Thresholds**: динамические пороги ёмкости
- **Circuit Breaker**: для всех inter-service communications

Подробная документация: `old_artstore/README.md`, `old_artstore/CLAUDE.md`, модульные README.
