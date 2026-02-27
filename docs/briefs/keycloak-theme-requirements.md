# Кастомная тема Keycloak для Artstore — Требования

## 1. Цель

Создать кастомную тему Keycloak для realm `artstore`, визуально соответствующую
дизайн-системе Admin Module (тёмная зелёная тема). Пользователь при переходе на
страницы аутентификации Keycloak должен видеть единый стиль с Admin Module UI.

## 2. Контекст

- **Текущее состояние**: Keycloak использует стандартную тему `keycloak.v2` (PatternFly 5,
  светлая + тёмная). Визуальный разрыв с Admin Module при редиректе на логин.
- **Admin Module UI**: тёмная зелёная тема (Tailwind CSS), шрифты Inter + JetBrains Mono,
  акцентный цвет `#22c55e`, фон `#0a0f0a`.
- **Keycloak версия**: 26.1 (quay.io/keycloak/keycloak:26.1).

## 3. Функциональные требования

### FR-1: Кастомный Docker-образ Keycloak

- Создать Dockerfile на базе `quay.io/keycloak/keycloak:26.1`.
- Тема встроена в образ (COPY в `/opt/keycloak/themes/artstore/`).
- Образ хранится в `harbor.kryukov.lan/library/keycloak-artstore`.
- Dockerfile размещается в `deploy/keycloak/Dockerfile`.
- **Все** конфигурации проекта (Helm charts, docker-compose) переходят на новый образ.

### FR-2: Подход — CSS-only кастомизация

- Тема наследует `keycloak.v2` (`parent=keycloak.v2` в `theme.properties`).
- HTML-структура страниц **не переопределяется** (без кастомных .ftl файлов, кроме
  `template.ftl` для логотипа).
- Переопределяется только `template.ftl` — для добавления логотипа Artstore и
  подключения кастомного CSS.
- Вся визуальная кастомизация выполняется через CSS (переопределение CSS-переменных
  и PatternFly 5 классов).

### FR-3: Визуальное соответствие Admin Module

Цветовая палитра (из Admin Module дизайн-системы):

| Элемент | Значение |
|---------|----------|
| Фон страницы | `#0a0f0a` (bg-base) |
| Фон карточки/формы | `#111c11` (bg-surface) |
| Фон полей ввода | `#1a2e1a` (bg-elevated) |
| Hover состояние | `#243824` (bg-hover) |
| Основной текст | `#e8f5e8` (text-primary) |
| Вторичный текст | `#9cb89c` (text-secondary) |
| Приглушённый текст | `#5a7a5a` (text-muted) |
| Акцентный цвет (кнопки, ссылки) | `#22c55e` (accent-primary) |
| Акцент hover | `#4ade80` (accent-light) |
| Границы | `#1a2e1a` / `#243824` (border-subtle / border-default) |
| Ошибка | `#ef4444` (status-error) |
| Предупреждение | `#eab308` (status-warning) |
| Информация | `#3b82f6` (status-info) |
| Успех | `#22c55e` (status-success) |

Шрифты:

- Основной: Inter (400, 500, 600, 700) через Google Fonts
- Моноширинный: JetBrains Mono (400, 500) через Google Fonts

### FR-4: Логотип

- Текстовый логотип "Artstore" с SVG-иконкой (как в sidebar Admin Module).
- Размещается в центре над формой логина.
- Цвет логотипа: `#22c55e` (акцентный зелёный).

### FR-5: Охват страниц (полный набор)

Все пользовательские страницы Keycloak должны стилизоваться через CSS:

| Категория | Страницы |
|-----------|----------|
| Аутентификация | login, login-username, login-password |
| Регистрация | register |
| Пароль | login-reset-password, login-update-password |
| 2FA/OTP | login-otp, login-config-totp, login-reset-otp, select-authenticator |
| Recovery | login-recovery-authn-code-config, login-recovery-authn-code-input |
| WebAuthn | webauthn-authenticate, webauthn-register, webauthn-error |
| Email | login-verify-email, update-email |
| Профиль | login-update-profile, idp-review-user-profile |
| OAuth | login-oauth-grant, login-oauth2-device-verify-user-code |
| Ошибки/статусы | error, login-page-expired, info |
| Прочее | terms, logout-confirm, code, delete-account-confirm, delete-credential |

Поскольку используется CSS-only подход с наследованием `keycloak.v2`, все эти
страницы стилизуются единым CSS-файлом без необходимости переопределять каждый
шаблон.

### FR-6: Локализация

- Поддержка двух языков: EN и RU (как в Admin Module).
- Переопределение строк через `messages_en.properties` и `messages_ru.properties`.
- Минимум: заголовок страницы логина, подпись, текст приветствия (если требуется).

### FR-7: Активация темы в realm

- Тема `artstore` устанавливается как login theme для realm `artstore`.
- Обновить `artstore-realm.json`: добавить `"loginTheme": "artstore"`.

## 4. Нефункциональные требования

### NFR-1: Совместимость при обновлении Keycloak

- CSS-only подход минимизирует поломки при обновлении Keycloak.
- Единственный переопределённый шаблон — `template.ftl` (для логотипа).
- При обновлении Keycloak необходимо проверить совместимость `template.ftl`.

### NFR-2: Структура файлов

```
deploy/keycloak/
├── Dockerfile                              # Кастомный образ
├── artstore-realm.json                     # Realm конфигурация (уже есть)
├── README.md                               # Документация (уже есть)
└── themes/
    └── artstore/
        └── login/
            ├── theme.properties            # parent=keycloak.v2, darkMode=true
            ├── template.ftl                # Логотип Artstore
            ├── messages/
            │   ├── messages_en.properties  # EN строки
            │   └── messages_ru.properties  # RU строки
            └── resources/
                ├── css/
                │   └── artstore.css        # Кастомные стили
                └── img/
                    ├── logo.svg            # SVG логотип
                    └── favicon.ico         # Иконка вкладки
```

### NFR-3: Docker-образ

- Базовый образ: `quay.io/keycloak/keycloak:26.1`.
- Registry: `harbor.kryukov.lan/library/keycloak-artstore`.
- Тегирование: `v26.1-1` (версия KC + номер сборки темы).
- Makefile target для сборки и push.

### NFR-4: Интеграция с Helm charts

Обновить конфигурации для использования нового образа:

- `tests/helm/artstore-infra/values.yaml` — образ Keycloak.
- `src/admin-module/docker-compose.yaml` — образ Keycloak для локальной разработки.
- Realm JSON — добавить `loginTheme`.

## 5. User Stories

### US-1: Брендированный логин

> Как администратор Artstore, я хочу видеть страницу логина в едином стиле
> с Admin Module, чтобы интерфейс выглядел профессионально и целостно.

**Acceptance Criteria:**

- Страница логина имеет тёмный фон (`#0a0f0a`).
- Логотип "Artstore" отображается над формой.
- Кнопка "Sign In" зелёного цвета (`#22c55e`).
- Поля ввода в тёмном стиле.
- Шрифт Inter.

### US-2: Единый стиль всех страниц

> Как пользователь, при любом взаимодействии с Keycloak (ошибка, сброс пароля,
> 2FA) я вижу тот же визуальный стиль, что и на странице логина.

**Acceptance Criteria:**

- Все страницы аутентификации используют тёмную зелёную тему.
- Сообщения об ошибках стилизованы (красный для ошибок, жёлтый для
  предупреждений).
- Ссылки и акценты зелёного цвета.

### US-3: Локализация

> Как русскоязычный пользователь, я хочу видеть страницы логина на русском языке.

**Acceptance Criteria:**

- Переключение языка работает (EN/RU).
- Ключевые строки переведены на русский.

### US-4: Кастомный Docker-образ

> Как DevOps, я хочу использовать единый кастомный образ Keycloak с встроенной
> темой, чтобы не монтировать файлы темы отдельно.

**Acceptance Criteria:**

- Образ `harbor.kryukov.lan/library/keycloak-artstore:v26.1-1` собирается и
  пушится.
- Helm chart и docker-compose используют новый образ.
- Тема активируется автоматически через realm config.

## 6. Открытые вопросы

1. **Favicon**: Использовать текущий favicon из Admin Module или создать новый?
2. **Страница Account Console**: Нужна ли кастомизация account console
   (личный кабинет пользователя в Keycloak), или только login theme?
3. **Email templates**: Нужна ли стилизация email-писем от Keycloak
   (сброс пароля, верификация email) в едином стиле?

## 7. Следующие шаги

После утверждения требований:

1. `/sc:design` — проектирование CSS-переменных и маппинга PatternFly 5 → Artstore
2. `/sc:workflow` — план реализации (Dockerfile, тема, интеграция, тестирование)
