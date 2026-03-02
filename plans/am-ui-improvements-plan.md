# План разработки: Улучшения Admin Module UI — Архивные файлы и SE Priority

## Метаданные

- **Версия плана**: 1.0.0
- **Дата создания**: 2026-03-02
- **Последнее обновление**: 2026-03-02
- **Статус**: Pending

---

## История версий

- **v1.0.0** (2026-03-02): Начальная версия плана

---

## Текущий статус

- **Активная фаза**: Phase 4
- **Активный подпункт**: 4.1
- **Последнее обновление**: 2026-03-02
- **Примечание**: Две независимые доработки AM UI + изменения QM

---

## Оглавление

- [x] [Phase 1: SE Priority — ограничение для ro/ar режимов](#phase-1-se-priority--ограничение-для-roar-режимов)
- [x] [Phase 2: Файлы в SE Archive — виртуальный статус "В архиве"](#phase-2-файлы-в-se-archive--виртуальный-статус-в-архиве)
- [x] [Phase 3: Query Module — обработка архивных файлов](#phase-3-query-module--обработка-архивных-файлов)
- [ ] [Phase 4: Сборка, деплой и тестирование](#phase-4-сборка-деплой-и-тестирование)

---

## Phase 1: SE Priority — ограничение для ro/ar режимов

**Dependencies**: None
**Status**: Done

### Описание

Поле write priority имеет смысл только для SE в режимах `edit` и `rw` (куда возможна запись).
Для SE в режимах `ro` (read-only) и `ar` (archive) priority не имеет практического значения,
т.к. Ingester Module не записывает в такие SE. Скрываем priority из UI для ro/ar,
сохраняя значение в БД (на случай возврата SE в writable mode).

### Подпункты

- [x] **1.1 UI: скрыть priority в таблице SE для ro/ar**
  - **Dependencies**: None
  - **Description**: В `se_table.templ` — для SE с mode `ro` или `ar` отображать `—` вместо числового priority. SE mode уже доступен как `se.Mode` в `SEListItem`.
  - **Файлы**:
    - `src/admin-module/internal/ui/pages/partials/se_table.templ` — строка 86: заменить `strconv.Itoa(se.Priority)` на условие: если `se.Mode == "ro" || se.Mode == "ar"` → `"—"`, иначе число

- [x] **1.2 UI: скрыть priority на странице деталей SE для ro/ar**
  - **Dependencies**: None
  - **Description**: В `se_detail.templ` — скрыть блок Priority (строки 203-209) если `data.Mode == "ro" || data.Mode == "ar"`. SE mode уже доступен как `data.Mode` в `SEDetailData`.
  - **Файлы**:
    - `src/admin-module/internal/ui/pages/se_detail.templ` — обернуть блок priority в `if data.Mode == "edit" || data.Mode == "rw"`

- [x] **1.3 UI: скрыть/задизейблить поле priority в форме редактирования SE для ro/ar**
  - **Dependencies**: None
  - **Description**: В `se_edit.templ` — функция `SEEditForm(id, name, url string, priority int)` не принимает mode. Нужно: (a) добавить параметр `mode string` в `SEEditForm`, (b) скрыть поле priority если mode=ro/ar, (c) обновить вызов `SEEditForm` в handler `HandleEditForm` — передать mode.
  - **Файлы**:
    - `src/admin-module/internal/ui/pages/partials/se_edit.templ` — добавить параметр mode, скрыть input priority для ro/ar
    - `src/admin-module/internal/ui/handlers/storage_elements.go` — метод `HandleEditForm`: получить SE через сервис (mode уже есть), передать mode в SEEditForm

- [x] **1.4 API: валидация — отклонять обновление priority для ro/ar SE**
  - **Dependencies**: None
  - **Description**: В service layer `StorageElementService.Update()` — если SE в mode ro/ar и priority != nil → возвращать ошибку. В API handler — маппить в HTTP 422 Unprocessable Entity с сообщением "Приоритет записи нельзя изменить для SE в режиме ro/ar".
  - **Файлы**:
    - `src/admin-module/internal/service/storage_elements.go` — метод `Update`: после получения SE проверить mode, если ro/ar и priority != nil → вернуть domain error
    - `src/admin-module/internal/api/handlers/storage_elements.go` — маппинг новой ошибки в HTTP 422

### Критерии завершения Phase 1

- [x] Все подпункты завершены (1.1, 1.2, 1.3, 1.4)
- [x] В таблице SE: для ro/ar отображается `—` вместо числа priority
- [x] На странице деталей SE: для ro/ar блок priority скрыт
- [x] В форме редактирования SE: для ro/ar поле priority скрыто
- [x] API отклоняет обновление priority для ro/ar с HTTP 422
- [x] Unit-тесты для валидации в service layer (существующие тесты проходят)
- [x] `templ generate` и `go build` проходят без ошибок

---

## Phase 2: Файлы в SE Archive — виртуальный статус "В архиве"

**Dependencies**: None (параллельно с Phase 1)
**Status**: Done

### Описание

Файлы, находящиеся в SE с mode=`ar` (archive), содержат только метаданные — binary файлов физически нет.
Такие файлы нельзя скачать, нельзя редактировать или удалить. В UI они должны отображаться с виртуальным
статусом "В архиве" (badge), а не "Active". Статус `archived` не вводится в БД — вычисляется на лету
из `se.mode == "ar"`.

### Подпункты

- [x] **2.1 Model: добавить SEMode в структуры данных файлов**
  - **Dependencies**: None
  - **Description**: Добавить поле `SEMode string` в `FileListItem` и `FileDetailData`. Это виртуальное поле, заполняемое handler'ом (не из БД).
  - **Файлы**:
    - `src/admin-module/internal/ui/pages/file_list.templ` — добавить `SEMode string` в struct `FileListItem`
    - `src/admin-module/internal/ui/pages/partials/file_detail.templ` — добавить `SEMode string` в struct `FileDetailData`

- [x] **2.2 Handler: передавать SEMode вместе с SEName**
  - **Dependencies**: 2.1
  - **Description**: В `files.go` handler уже загружает все SE через `getSENames()` и строит map `id→name`. Расширить: (a) создать struct `seInfo{Name, Mode string}`, (b) переименовать/расширить `getSENames()` → `getSEInfo()`, (c) при маппинге файлов заполнять `SEMode` из seInfo map.
  - **Файлы**:
    - `src/admin-module/internal/ui/handlers/files.go` — расширить `getSENames` → `getSEInfo`, возвращать map[string]seInfo; обновить маппинг в HandleList, HandleDetailModal, HandleTablePartial

- [x] **2.3 Badge: добавить вариант для "В архиве"**
  - **Dependencies**: None
  - **Description**: Добавить новый `BadgeVariant` (например `BadgeArchived`) в badge.templ с нейтрально-синим стилем (отличающимся от существующих). Добавить helper-функцию `isArchivedFile(status, seMode string) bool` → true если status=="active" && seMode=="ar".
  - **Файлы**:
    - `src/admin-module/internal/ui/components/badge.templ` — добавить `BadgeArchived BadgeVariant`, стили (bg-mode-ar/20 text-mode-ar или свой)

- [x] **2.4 i18n: добавить ключи для "В архиве"**
  - **Dependencies**: None
  - **Description**: Добавить локализованные ключи для нового статуса.
  - **Файлы**:
    - `src/admin-module/internal/ui/i18n/locales/ru.json` — добавить `"files.status.archived": "В архиве"`, `"files.filter.archived": "В архиве"`, `"file_detail.archived_notice": "Файл находится в архивном Storage Element. Скачивание и редактирование недоступны."`
    - `src/admin-module/internal/ui/i18n/locales/en.json` — аналогичные ключи на английском

- [x] **2.5 UI: отображение виртуального статуса в таблице файлов**
  - **Dependencies**: 2.1, 2.2, 2.3, 2.4
  - **Description**: В `file_table.templ` — изменить логику badge статуса: если `f.Status == "active" && f.SEMode == "ar"` → показать badge "В архиве" (BadgeArchived). Скрыть кнопки Edit/Delete для таких файлов (условие: `data.Role == "admin" && f.Status == "active" && f.SEMode != "ar"`).
  - **Файлы**:
    - `src/admin-module/internal/ui/pages/partials/file_table.templ` — обновить badge и условие кнопок
    - `src/admin-module/internal/ui/pages/file_list.templ` — обновить `fileStatusLabel()` и `fileStatusVariant()` (добавить параметр seMode или создать новую функцию)

- [x] **2.6 UI: отображение в деталях файла**
  - **Dependencies**: 2.1, 2.2, 2.3, 2.4
  - **Description**: В `file_detail.templ` — аналогичная логика: (a) badge "В архиве" вместо "Active", (b) скрыть кнопки Edit/Delete, (c) показать info-блок с пояснением "Файл в архивном SE, скачивание недоступно".
  - **Файлы**:
    - `src/admin-module/internal/ui/pages/partials/file_detail.templ` — обновить badge, скрыть кнопки, добавить info notice

- [x] **2.7 UI: фильтр "В архиве" на странице файлов**
  - **Dependencies**: 2.2
  - **Description**: Добавить опцию "В архиве" в dropdown фильтра по статусу на странице file_list. При выборе фильтра "archived" — handler фильтрует: status=active + se.mode=ar. Потребуется расширить `FileListFilters` в service/repository.
  - **Файлы**:
    - `src/admin-module/internal/ui/pages/file_list.templ` — добавить option "В архиве" (value="archived") в select фильтра статуса
    - `src/admin-module/internal/ui/handlers/files.go` — при status="archived": передать в сервис фильтр по SE mode
    - `src/admin-module/internal/service/file_registry.go` — расширить `FileListFilters`: добавить `SEMode *string`
    - `src/admin-module/internal/repository/file_registry.go` — расширить SQL: если SEMode задан → JOIN storage_elements + WHERE se.mode = $N

### Критерии завершения Phase 2

- [x] Все подпункты завершены (2.1 — 2.7)
- [x] Файлы в SE-AR отображаются с badge "В архиве" (не "Active")
- [x] Кнопки Edit/Delete скрыты для файлов в SE-AR
- [x] Info-блок в detail modal поясняет недоступность скачивания
- [x] Фильтр "В архиве" работает на странице файлов
- [x] `templ generate` и `go build` проходят без ошибок
- [ ] Визуальная проверка в UI (ручная)

---

## Phase 3: Query Module — обработка архивных файлов

**Dependencies**: Phase 2 (понимание контракта)
**Status**: Done

### Описание

QM должен корректно обрабатывать запросы к файлам из архивных SE:
- Search: возвращать метаданные файлов из SE-AR в результатах поиска
- Download: возвращать ошибку при попытке скачать файл из SE-AR (файл физически отсутствует)

### Подпункты

- [x] **3.1 QM Download: проверка SE mode перед скачиванием**
  - **Dependencies**: None
  - **Description**: В `DownloadService.Download()` — после получения SE info из Admin Module, проверить SE mode. Если mode=ar → вернуть HTTP 410 Gone с сообщением "Файл находится в архивном хранилище и недоступен для скачивания". QM уже получает SE info через `adminClient.GetStorageElement()` — нужно проверить, содержит ли ответ поле mode.
  - **Файлы**:
    - `src/query-module/internal/service/download.go` — добавить проверку SE mode после GetStorageElement
    - `src/query-module/internal/service/adminclient/client.go` — проверить что StorageElementResponse содержит mode (если нет — добавить)
    - `src/query-module/internal/api/errors/` — добавить ошибку для archived file (410 Gone)

- [x] **3.2 QM Search: пометка архивных файлов в результатах**
  - **Dependencies**: None
  - **Description**: QM Search возвращает файлы из SE-AR в результатах. Нужно добавить поле `is_archived` (или `se_mode`) в ответ search API, чтобы клиент мог отображать статус. Проверить: (a) есть ли в search response информация о SE, (b) если нет — нужен JOIN или post-query enrichment.
  - **Файлы**:
    - `docs/api-contracts/query-module-openapi.yaml` — расширить SearchResult: добавить `se_mode` поле
    - `src/query-module/internal/api/generated/` — перегенерировать типы
    - `src/query-module/internal/service/search.go` — обогащение результатов SE mode (через cached SE info)
    - `src/query-module/internal/repository/file_repository.go` — опционально: JOIN с storage_elements для получения mode

### Критерии завершения Phase 3

- [x] Все подпункты завершены (3.1, 3.2)
- [x] QM Download возвращает 410 Gone для файлов из SE-AR
- [x] QM Search возвращает se_mode в результатах
- [x] Unit-тесты для новой логики
- [x] `go build` проходит без ошибок

---

## Phase 4: Сборка, деплой и тестирование

**Dependencies**: Phase 1, Phase 2, Phase 3
**Status**: Pending

### Описание

Сборка Docker-образов Admin Module и Query Module с изменениями,
деплой в тестовый кластер K8s и проведение интеграционного тестирования.

### Подпункты

- [ ] **4.0 Проверка linter для AM и QM**
  - **Dependencies**: None
  - **Description**: Запустить golangci-lint для Admin Module и Query Module, исправить все замечания. Проверить что md-файлы (план, OpenAPI описание) соответствуют linter.
  - **Команды**:
    - `cd src/admin-module && golangci-lint run ./...`
    - `cd src/query-module && golangci-lint run ./...`

- [ ] **4.1 Сборка Docker-образов AM и QM**
  - **Dependencies**: 4.0
  - **Description**: Собрать новые Docker образы для Admin Module и Query Module. Тег: инкремент текущей версии.
  - **Файлы**:
    - `src/admin-module/Dockerfile`
    - `src/query-module/Dockerfile`

- [ ] **4.2 Деплой в тестовый кластер**
  - **Dependencies**: 4.1
  - **Description**: Обновить Helm values с новыми тегами образов, задеплоить в namespace `artstore-test`.
  - **Файлы**:
    - `tests/helm/artstore-apps/` — обновить image tags

- [ ] **4.3 Интеграционное тестирование AM**
  - **Dependencies**: 4.2
  - **Description**: Запустить `make test-am`. Дополнительно: ручная проверка UI — файлы в SE-AR отображаются корректно, priority скрыт для ro/ar.
  - **Файлы**:
    - `tests/scripts/` — при необходимости добавить новые тест-кейсы

- [ ] **4.4 Интеграционное тестирование QM**
  - **Dependencies**: 4.2
  - **Description**: Запустить `make test-qm`. Добавить тест-кейсы: download из SE-AR → 410 Gone.
  - **Файлы**:
    - `tests/scripts/` — добавить тесты для QM download archived files

### Критерии завершения Phase 4

- [ ] Все подпункты завершены (4.0 — 4.4)
- [ ] golangci-lint проходит без ошибок для AM и QM
- [ ] Docker образы AM и QM собраны и запушены в Harbor
- [ ] Все существующие интеграционные тесты проходят (`make test-am`, `make test-qm`)
- [ ] Новые тест-кейсы для archived files проходят
- [ ] Ручная проверка UI подтверждает корректность

---

## Примечания

- **Phase 1 и Phase 2 независимы** — могут выполняться параллельно в разных контекстах AI
- **SE mode не хранится в file_registry** — вычисляется через JOIN/lookup к storage_elements
- **Виртуальный статус** "В архиве" не вводит новый enum в БД — только UI логика
- **Значение priority сохраняется в БД** при переходе SE в ro/ar — при возврате в edit/rw оно восстановится
- **QM adminclient** — проверить, что response `/api/v1/storage-elements/{id}` содержит поле `mode`

### Затронутые модули и файлы (сводка)

**Admin Module (src/admin-module/):**
- `internal/ui/pages/partials/se_table.templ` — Phase 1
- `internal/ui/pages/se_detail.templ` — Phase 1
- `internal/ui/pages/partials/se_edit.templ` — Phase 1
- `internal/ui/handlers/storage_elements.go` — Phase 1
- `internal/service/storage_elements.go` — Phase 1
- `internal/api/handlers/storage_elements.go` — Phase 1
- `internal/ui/pages/file_list.templ` — Phase 2
- `internal/ui/pages/partials/file_table.templ` — Phase 2
- `internal/ui/pages/partials/file_detail.templ` — Phase 2
- `internal/ui/handlers/files.go` — Phase 2
- `internal/ui/components/badge.templ` — Phase 2
- `internal/ui/i18n/locales/ru.json` — Phase 2
- `internal/ui/i18n/locales/en.json` — Phase 2
- `internal/service/file_registry.go` — Phase 2
- `internal/repository/file_registry.go` — Phase 2

**Query Module (src/query-module/):**
- `internal/service/download.go` — Phase 3
- `internal/service/search.go` — Phase 3
- `internal/service/adminclient/client.go` — Phase 3
- `internal/repository/file_repository.go` — Phase 3
- `docs/api-contracts/query-module-openapi.yaml` — Phase 3

---

**План готов к использованию.**
