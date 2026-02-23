# Git Workflow (GitHub Flow + Semver Tags)

## ОБЯЗАТЕЛЬНОЕ ПРАВИЛО

При выполнении любых задач, связанных с изменением файлов проекта, ВСЕГДА следовать этому workflow.

## Структура веток

```txt
main                              ← основная ветка, всегда deployable
 ├── feature/...                  ← новая функциональность
 ├── bugfix/...                   ← исправления багов
 ├── docs/...                     ← документация
 ├── refactor/...                 ← рефакторинг
 ├── test/...                     ← тесты
 └── hotfix/...                   ← критические production fixes
```

- **`main`** — единственная постоянная ветка. Всегда стабильна и готова к деплою.
- **Feature branches** — короткоживущие ветки от `main`, мерджатся обратно в `main`.
- **Релизы** — git tags `vX.Y.Z` на `main`. CI собирает Docker-образ и создаёт GitHub Release.
- **Теги образов при разработке** — `v0.Y.Z-N` (например `v0.1.0-1`, `v0.1.0-2`). Суффикс `-N` инкрементируется при каждой сборке.
- **Теги образов при релизе** — `v0.Y.Z` (например `v0.1.1`). Суффикс убирается, patch-версия увеличивается.

### Версионирование модулей (Semver)

Все модули на стадии разработки используют версии `0.x.y`:

- **Major = 0** — проект в разработке, API нестабилен
- **Minor (Y)** — увеличивается при завершении milestone/фазы. **Только с разрешения пользователя!**
- **Patch (Z)** — баг-фиксы внутри текущей версии
- **v1.0.0** — первый production-ready release (все модули интегрированы и протестированы)

| Действие | Результат | Нужно разрешение? |
|----------|-----------|-------------------|
| Баг-фикс | patch: `0.1.0` → `0.1.1` | Нет |
| Новая фаза/milestone | minor: `0.1.0` → `0.2.0` | **Да** |
| Production release | major: `0.x.y` → `1.0.0` | **Да** |

## Workflow

### 1. Перед началом работы

```bash
git checkout main
git pull origin main

# Создать feature branch от main
git checkout -b <type>/<short-description>
```

**Branch naming:**

- `feature/` — новая функциональность
- `bugfix/` — исправления багов
- `docs/` — документация
- `refactor/` — рефакторинг без изменения функционала
- `test/` — добавление/улучшение тестов
- `hotfix/` — критические production fixes

**Примеры:**

- `feature/admin-auth-oauth2`
- `docs/auth-mechanics-documentation`
- `bugfix/storage-element-wal-race-condition`

### 2. Выполнение работы

- Делать изменения в созданной ветке
- Можно делать промежуточные commits при необходимости
- **Быстрые правки** (опечатки, мелкие фиксы) можно коммитить напрямую в `main`

### 3. По завершении задачи — предложить commit

**Спросить пользователя:**
> Работа завершена. Создать commit?

**Commit message format (Conventional Commits):**

```txt
<type>(<scope>): <subject>

[optional body]
```

**Types:**

- `feat`: новая функциональность
- `fix`: исправление бага
- `docs`: документация
- `style`: форматирование
- `refactor`: рефакторинг
- `test`: тесты
- `chore`: maintenance

### 4. После commit — merge в main

**Спросить пользователя:**

> Commit создан. Выберите способ merge в `main`:
>
> **[A] Локальный merge:**
>
> ```bash
> git checkout main
> git merge --no-ff <branch-name>
> git push origin main
> ```
>
> **[B] GitHub PR:**
>
> ```bash
> git push origin <branch-name>
> gh pr create --base main --fill
> ```

### 5. После merge — удалить временную ветку

```bash
git branch -d <branch-name>
git push origin --delete <branch-name>
```

### 6. Сборка образа при разработке

При каждом изменении собирать образ с тегом `v0.Y.Z-N`, где `N` инкрементируется:

```bash
# Первая сборка версии 0.1.0
make docker-build TAG=v0.1.0-1

# Следующая сборка
make docker-build TAG=v0.1.0-2
```

### 7. Выпуск релиза — создание тега

Когда набрана функциональность для релиза:

1. Убрать суффикс и увеличить patch-версию
2. Собрать финальный образ и создать git tag

```bash
git checkout main
git pull origin main

# Финальная сборка образа
make docker-build TAG=v0.1.1

# Git tag
git tag -a v0.1.1 -m "Release v0.1.1"
git push origin v0.1.1
```

CI автоматически:

- Собирает Docker-образ с тегом `vX.Y.Z`
- Создаёт GitHub Release

### 8. Поддержка старой версии (при необходимости)

Если нужно выпустить patch для старой версии:

```bash
# Создать release-ветку от тега
git checkout -b release/v0.1 v0.1.0

# Cherry-pick нужные фиксы
git cherry-pick <commit-hash>

# Выпустить patch
git tag -a v0.1.1 -m "Release v0.1.1"
git push origin v0.1.1
```

## Важные правила

1. **`main` всегда deployable** — не мерджить сломанный код
2. **Короткоживущие ветки** — merge как можно скорее
3. **Conventional Commits** — всегда использовать правильный формат
4. **Удалять ветки после merge** — не оставлять мусор
5. **Релизы через теги** — не через ветки
6. **PR для значимых изменений** — code review перед merge

## Пример полного цикла

```bash
# 1. Обновить main
git checkout main && git pull

# 2. Создать feature-ветку
git checkout -b docs/update-readme-authentication

# 3. Работа... (изменения файлов)

# 4. Commit
git add .
git commit -m "docs(admin-module): add authentication documentation"

# 5. Merge в main
git checkout main
git merge --no-ff docs/update-readme-authentication
git push origin main

# 6. Cleanup
git branch -d docs/update-readme-authentication

# 7. Когда готов релиз
git tag -a v0.2.0 -m "Release v0.2.0"
git push origin v0.2.0
```
