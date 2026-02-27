# План реализации: Кастомная тема Keycloak для Artstore

> **Требования**: `docs/briefs/keycloak-theme-requirements.md`
> **Статус**: Завершён
> **Дата**: 2026-02-27

## Обзор

Создание кастомной темы Keycloak в стиле Admin Module (тёмная зелёная тема) и
упаковка в Docker-образ для использования во всех окружениях.

**Подход**: CSS-only, наследование от `keycloak.v2`, единственный `.ftl` —
`template.ftl` для логотипа.

---

## Фаза 1: Создание темы и Docker-образа

> Один контекст AI. Результат: собранный и запущенный кастомный образ Keycloak
> с темой Artstore.

### 1.1 Структура директорий

- [x] Создать `deploy/keycloak/themes/artstore/login/`
- [x] Создать вложенные директории: `messages/`, `resources/css/`, `resources/img/`

### 1.2 theme.properties

- [x] Создать `deploy/keycloak/themes/artstore/login/theme.properties`:

```properties
parent=keycloak.v2
import=common/keycloak

styles=css/styles.css css/artstore.css
darkMode=true

locales=en,ru
```

### 1.3 Кастомный CSS (artstore.css)

- [x] Создать `deploy/keycloak/themes/artstore/login/resources/css/artstore.css`
- [x] Переопределить CSS-переменные PatternFly 5 на палитру Admin Module:

| PatternFly переменная | Значение Artstore | Назначение |
|-----------------------|-------------------|------------|
| `--pf-t--global--background--color--primary--default` | `#0a0f0a` | Фон страницы |
| `--pf-t--global--background--color--secondary--default` | `#111c11` | Фон карточки/формы |
| `--pf-t--global--background--color--fill--secondary` | `#1a2e1a` | Фон полей ввода |
| `--pf-t--global--background--color--fill--hover` | `#243824` | Hover |
| `--pf-t--global--text--color--regular` | `#e8f5e8` | Текст |
| `--pf-t--global--text--color--subtle` | `#9cb89c` | Вторичный текст |
| `--pf-t--global--text--color--placeholder` | `#5a7a5a` | Placeholder |
| `--pf-t--global--color--brand--default` | `#22c55e` | Акцент (кнопки, ссылки) |
| `--pf-t--global--color--brand--hover` | `#4ade80` | Акцент hover |
| `--pf-t--global--border--color--default` | `#1a2e1a` | Границы |
| `--pf-t--global--color--status--danger--default` | `#ef4444` | Ошибка |
| `--pf-t--global--color--status--warning--default` | `#eab308` | Предупреждение |
| `--pf-t--global--color--status--success--default` | `#22c55e` | Успех |
| `--pf-t--global--color--status--info--default` | `#3b82f6` | Информация |

- [x] Подключить шрифт Inter через `@import` из Google Fonts
- [x] Задать `font-family: 'Inter', system-ui, sans-serif` для body
- [x] Стилизовать кнопку submit: фон `#22c55e`, цвет текста `#0a0f0a`
- [x] Стилизовать поля ввода: фон `#1a2e1a`, граница `#243824`, текст `#e8f5e8`
- [x] Стилизовать ссылки: цвет `#22c55e`, hover `#4ade80`
- [x] Стилизовать алерты (error, warning, info, success)
- [x] Добавить мягкие переходы (`transition: all 150ms ease`)

### 1.4 Логотип (template.ftl)

- [x] Создать `deploy/keycloak/themes/artstore/login/resources/img/logo.svg`:
  - SVG-иконка цилиндра хранилища (из sidebar Admin Module)
  - Цвет: `#22c55e`
  - Текст "Artstore" рядом с иконкой

- [x] Создать `deploy/keycloak/themes/artstore/login/template.ftl`:
  - Скопировать `template.ftl` из `keycloak.v2` как базу
  - Заменить стандартный заголовок на логотип Artstore (SVG + текст)
  - Минимальные изменения для совместимости при обновлениях

### 1.5 Локализация

- [x] Создать `deploy/keycloak/themes/artstore/login/messages/messages_en.properties`:

```properties
loginTitleHtml=Sign in to <strong>Artstore</strong>
loginTitle=Sign in to Artstore
```

- [x] Создать `deploy/keycloak/themes/artstore/login/messages/messages_ru.properties`:

```properties
loginTitleHtml=\u0412\u0445\u043E\u0434 \u0432 <strong>Artstore</strong>
loginTitle=\u0412\u0445\u043E\u0434 \u0432 Artstore
```

### 1.6 Dockerfile

- [x] Создать `deploy/keycloak/Dockerfile`:

```dockerfile
FROM quay.io/keycloak/keycloak:26.1

# Копируем кастомную тему
COPY themes/artstore /opt/keycloak/themes/artstore
```

### 1.7 Сборка и push образа

- [x] Собрать образ:

```bash
docker build --platform linux/amd64 \
  -t harbor.kryukov.lan/library/keycloak-artstore:v26.1-1 \
  deploy/keycloak/
```

- [x] Запушить в Harbor:

```bash
docker push harbor.kryukov.lan/library/keycloak-artstore:v26.1-1
```

### 1.8 Makefile targets

- [x] Добавить в `tests/Makefile` переменные и targets:

```makefile
KC_IMAGE     := $(DOCKER_REGISTRY)/keycloak-artstore
KC_TAG       ?= v26.1-1
KC_ROOT      := ../deploy/keycloak

## docker-build-kc: Собрать образ Keycloak с темой Artstore
docker-build-kc:
	@echo ">>> Сборка Keycloak $(KC_TAG) (linux/amd64)..."
	docker build --platform linux/amd64 \
		-t $(KC_IMAGE):$(KC_TAG) -f $(KC_ROOT)/Dockerfile $(KC_ROOT)

## docker-push-kc: Запушить образ Keycloak в Harbor
docker-push-kc:
	@echo ">>> Push $(KC_IMAGE):$(KC_TAG)..."
	docker push $(KC_IMAGE):$(KC_TAG)
```

- [x] Добавить `docker-build-kc docker-push-kc` в общий target `docker-build`

---

## Фаза 2: Интеграция с инфраструктурой

> Один контекст AI. Результат: кастомный образ используется в Helm chart и
> docker-compose, тема активирована в realm.

### 2.1 Обновить Helm chart (artstore-infra)

- [x] `tests/helm/artstore-infra/values.yaml` — заменить образ Keycloak:

```yaml
keycloak:
  image: harbor.kryukov.lan/library/keycloak-artstore:v26.1-1
```

### 2.2 Обновить docker-compose (Admin Module)

- [x] `src/admin-module/docker-compose.yaml` — заменить образ Keycloak:

```yaml
keycloak:
  image: harbor.kryukov.lan/library/keycloak-artstore:v26.1-1
```

### 2.3 Активировать тему в realm

- [x] `deploy/keycloak/artstore-realm.json` — добавить в корень:

```json
"loginTheme": "artstore"
```

- [x] `tests/helm/artstore-infra/files/artstore-realm.json` — аналогично добавить `loginTheme`

### 2.4 Обновить документацию

- [x] `deploy/keycloak/README.md` — добавить раздел про кастомную тему:
  - Описание структуры темы
  - Как собрать образ
  - Как обновить при смене версии Keycloak

---

## Фаза 3: Деплой и визуальная верификация

> Один контекст AI. Результат: тема работает в кластере, визуально проверена.

### 3.1 Деплой в тестовый кластер

- [x] `make infra-down` — удалить текущую инфраструктуру
- [x] `make infra-up` — задеплоить с новым образом
- [x] Проверить pod keycloak запустился: `kubectl get pods -n artstore-test`
- [x] Проверить логи: `kubectl logs -n artstore-test deploy/artstore-infra-keycloak`

### 3.2 Визуальная верификация

- [x] Открыть `https://artstore.kryukov.lan/realms/artstore/account/` — проверить
  редирект на страницу логина с кастомной темой
- [x] Проверить визуально:
  - Тёмный фон (`#0a0f0a`)
  - Логотип "Artstore" над формой
  - Зелёная кнопка "Sign In" (`#22c55e`)
  - Тёмные поля ввода
  - Шрифт Inter
- [x] Проверить логин с admin/admin — успешный вход
- [x] Проверить страницу ошибки (неверный пароль) — стилизованная ошибка
- [x] Проверить переключение языка EN/RU

### 3.3 Проверить Admin Module flow

- [x] `make apps-up` — задеплоить Admin Module
- [x] Открыть `https://artstore.kryukov.lan/admin/` — должен произойти редирект
  на стилизованную страницу логина
- [x] Залогиниться → редирект обратно в Admin Module
- [x] Нажать Logout → редирект на стилизованную страницу Keycloak

### 3.4 Скриншоты для документации

- [x] Сделать скриншот страницы логина
- [x] Сделать скриншот страницы ошибки
- [x] Сохранить в `docs/` для справки

---

## Чеклист завершения

- [x] Тема визуально соответствует Admin Module
- [x] Все страницы Keycloak стилизованы (логин, ошибки, сброс пароля)
- [x] Логотип Artstore отображается корректно
- [x] Локализация EN/RU работает
- [x] Docker-образ собран и запушен в Harbor
- [x] Helm chart использует новый образ
- [x] docker-compose использует новый образ
- [x] Realm JSON содержит `loginTheme: artstore`
- [x] Документация обновлена
- [x] Admin Module flow работает (логин → UI → логаут)
- [x] Интеграционные тесты AM проходят (`make test-am`)

---

## Файлы, которые будут созданы

| Файл | Назначение |
|------|------------|
| `deploy/keycloak/Dockerfile` | Кастомный образ Keycloak |
| `deploy/keycloak/themes/artstore/login/theme.properties` | Конфигурация темы |
| `deploy/keycloak/themes/artstore/login/template.ftl` | Логотип (единственный .ftl) |
| `deploy/keycloak/themes/artstore/login/resources/css/artstore.css` | Кастомные стили |
| `deploy/keycloak/themes/artstore/login/resources/img/logo.svg` | SVG логотип |
| `deploy/keycloak/themes/artstore/login/messages/messages_en.properties` | EN строки |
| `deploy/keycloak/themes/artstore/login/messages/messages_ru.properties` | RU строки |

## Файлы, которые будут изменены

| Файл | Изменение |
|------|-----------|
| `deploy/keycloak/artstore-realm.json` | Добавить `loginTheme` |
| `tests/helm/artstore-infra/files/artstore-realm.json` | Добавить `loginTheme` |
| `tests/helm/artstore-infra/values.yaml` | Новый образ Keycloak |
| `src/admin-module/docker-compose.yaml` | Новый образ Keycloak |
| `tests/Makefile` | Targets для сборки KC образа |
| `deploy/keycloak/README.md` | Документация по теме |
