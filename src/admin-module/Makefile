# Makefile для Admin Module
# Основные цели: build, test, generate, lint, docker-build, docker-run
# UI цели: templ-generate, css-build, ui-build
# Проверки: lint, lint-docker, lint-helm, security-scan, secrets-scan, check-all

BINARY_NAME := admin-module
MODULE := github.com/bigkaa/goartstore/admin-module
VERSION ?= dev
OPENAPI_SPEC := ../../docs/api-contracts/admin-module-openapi.yaml

# Docker
DOCKER_REGISTRY := harbor.kryukov.lan/library
DOCKER_IMAGE := $(DOCKER_REGISTRY)/$(BINARY_NAME)
TAG ?= $(VERSION)

# UI: пути к инструментам и файлам
TAILWINDCSS := ./tailwindcss
CSS_INPUT := internal/ui/static/css/input.css
CSS_OUTPUT := internal/ui/static/css/output.css

# Каталоги переводов
I18N_DIR := internal/ui/i18n/locales
I18N_EN := $(I18N_DIR)/en.json
I18N_RU := $(I18N_DIR)/ru.json

# Версии инструментов проверки кода
GOLANGCI_LINT_VERSION := v2.10.1
HADOLINT_VERSION := v2.14.0
TRIVY_VERSION := 0.69.1
GITLEAKS_VERSION := v8.30.0

# Пути
ROOT_DIR := $(shell cd ../.. && pwd)
HELM_CHART := charts/$(BINARY_NAME)

.PHONY: build test test-race generate lint lint-docker lint-helm \
        security-scan secrets-scan check-all install-tools \
        docker-build docker-run docker-stop clean help \
        templ-generate css-build css-watch ui-build i18n-check

## help: Показать справку по целям
help:
	@echo "Доступные цели:"
	@echo ""
	@echo "  Сборка:"
	@echo "  build           — Собрать бинарник (включая UI)"
	@echo "  ui-build        — Собрать UI (templ generate + tailwind CSS)"
	@echo "  templ-generate  — Сгенерировать Go-код из .templ файлов"
	@echo "  css-build       — Скомпилировать Tailwind CSS (минифицированный)"
	@echo "  css-watch       — Tailwind CSS в режиме watch (для разработки)"
	@echo ""
	@echo "  Тесты:"
	@echo "  test            — Запустить unit-тесты"
	@echo "  test-race       — Запустить тесты с race detector"
	@echo ""
	@echo "  Кодогенерация:"
	@echo "  generate        — Сгенерировать код из OpenAPI"
	@echo ""
	@echo "  Проверки кода:"
	@echo "  lint            — Запустить golangci-lint (Go-код)"
	@echo "  lint-docker     — Запустить hadolint (Dockerfile)"
	@echo "  lint-helm       — Запустить helm lint (Helm chart)"
	@echo "  security-scan   — Сканирование уязвимостей (trivy: зависимости + образ)"
	@echo "  secrets-scan    — Сканирование на утечку секретов (gitleaks)"
	@echo "  check-all       — Запустить все проверки одной командой"
	@echo ""
	@echo "  Инструменты:"
	@echo "  install-tools   — Установить все инструменты проверки кода"
	@echo ""
	@echo "  i18n:"
	@echo "  i18n-check      — Проверить полноту каталогов переводов (en ↔ ru)"
	@echo ""
	@echo "  Docker:"
	@echo "  docker-build    — Собрать Docker-образ с UI (templ + tailwind в Docker)"
	@echo "  docker-run      — Запустить через docker-compose"
	@echo "  docker-stop     — Остановить docker-compose"
	@echo ""
	@echo "  Прочее:"
	@echo "  clean           — Очистить артефакты сборки"

# =====================================================================
# UI
# =====================================================================

## templ-generate: Сгенерировать Go-код из .templ шаблонов
templ-generate:
	templ generate

## css-build: Скомпилировать Tailwind CSS (минифицированный output)
css-build:
	$(TAILWINDCSS) -i $(CSS_INPUT) -o $(CSS_OUTPUT) --minify

## css-watch: Tailwind CSS в режиме watch (для разработки)
css-watch:
	$(TAILWINDCSS) -i $(CSS_INPUT) -o $(CSS_OUTPUT) --watch

## ui-build: Собрать весь UI (templ + CSS)
ui-build: templ-generate css-build

# =====================================================================
# Сборка
# =====================================================================

## build: Собрать бинарник admin-module (с предварительной сборкой UI)
build: ui-build
	go build -ldflags "-X $(MODULE)/internal/config.Version=$(VERSION)" \
		-o bin/$(BINARY_NAME) ./cmd/$(BINARY_NAME)/

# =====================================================================
# Тесты
# =====================================================================

## test: Запустить unit-тесты
test:
	go test ./... -count=1

## test-race: Запустить тесты с детектором гонок
test-race:
	go test -race ./... -count=1

# =====================================================================
# Кодогенерация
# =====================================================================

## generate: Сгенерировать Go-код из OpenAPI спецификации
generate:
	oapi-codegen --config oapi-codegen-types.yaml $(OPENAPI_SPEC)
	oapi-codegen --config oapi-codegen-server.yaml $(OPENAPI_SPEC)

# =====================================================================
# Проверки кода
# =====================================================================

## lint: Запустить golangci-lint (используется конфиг из корня проекта)
lint:
	golangci-lint run ./...

## lint-docker: Проверить Dockerfile с помощью hadolint
lint-docker:
	@echo "=== hadolint: Dockerfile ==="
	hadolint --config $(ROOT_DIR)/.hadolint.yaml Dockerfile

## lint-helm: Проверить Helm chart
lint-helm:
	@echo "=== helm lint: $(HELM_CHART) ==="
	helm lint $(HELM_CHART)

## security-scan: Сканирование уязвимостей в зависимостях и Docker-образе
security-scan:
	@echo "=== trivy: сканирование Go-зависимостей ==="
	trivy fs --scanners vuln --severity HIGH,CRITICAL .
	@echo ""
	@if docker image inspect $(DOCKER_IMAGE):$(TAG) >/dev/null 2>&1; then \
		echo "=== trivy: сканирование Docker-образа $(DOCKER_IMAGE):$(TAG) ==="; \
		trivy image --severity HIGH,CRITICAL $(DOCKER_IMAGE):$(TAG); \
	else \
		echo "Docker-образ $(DOCKER_IMAGE):$(TAG) не найден, пропускаем сканирование образа"; \
		echo "Соберите образ: make docker-build TAG=$(TAG)"; \
	fi

## secrets-scan: Сканирование на утечку секретов
secrets-scan:
	@echo "=== gitleaks: сканирование репозитория ==="
	cd $(ROOT_DIR) && gitleaks detect --config .gitleaks.toml --source . -v

## check-all: Запустить все проверки
check-all: lint lint-docker lint-helm security-scan secrets-scan
	@echo ""
	@echo "========================================="
	@echo "  Все проверки завершены успешно"
	@echo "========================================="

# =====================================================================
# Установка инструментов
# =====================================================================

## install-tools: Установить все инструменты проверки кода
install-tools:
	@echo "=== Установка golangci-lint $(GOLANGCI_LINT_VERSION) ==="
	@if command -v golangci-lint >/dev/null 2>&1 && golangci-lint --version 2>&1 | grep -q "$(subst v,,$(GOLANGCI_LINT_VERSION))"; then \
		echo "golangci-lint $(GOLANGCI_LINT_VERSION) уже установлен"; \
	else \
		curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/HEAD/install.sh | sh -s -- -b $$(go env GOPATH)/bin $(GOLANGCI_LINT_VERSION); \
	fi
	@echo ""
	@echo "=== Установка hadolint $(HADOLINT_VERSION) ==="
	@if command -v hadolint >/dev/null 2>&1; then \
		echo "hadolint уже установлен"; \
	else \
		echo "Установите hadolint вручную:"; \
		echo "  brew install hadolint  (macOS)"; \
		echo "  или скачайте: https://github.com/hadolint/hadolint/releases/tag/$(HADOLINT_VERSION)"; \
	fi
	@echo ""
	@echo "=== Установка trivy $(TRIVY_VERSION) ==="
	@if command -v trivy >/dev/null 2>&1; then \
		echo "trivy уже установлен"; \
	else \
		echo "Установите trivy вручную:"; \
		echo "  brew install trivy  (macOS)"; \
		echo "  или скачайте: https://github.com/aquasecurity/trivy/releases/tag/v$(TRIVY_VERSION)"; \
	fi
	@echo ""
	@echo "=== Установка gitleaks $(GITLEAKS_VERSION) ==="
	@if command -v gitleaks >/dev/null 2>&1; then \
		echo "gitleaks уже установлен"; \
	else \
		echo "Установите gitleaks вручную:"; \
		echo "  brew install gitleaks  (macOS)"; \
		echo "  или скачайте: https://github.com/gitleaks/gitleaks/releases/tag/$(GITLEAKS_VERSION)"; \
	fi

# =====================================================================
# Docker
# =====================================================================

## docker-build: Собрать Docker-образ (включает templ generate + tailwind compile)
docker-build:
	docker build \
		--build-arg VERSION=$(TAG) \
		-t $(DOCKER_IMAGE):$(TAG) \
		-f Dockerfile .

## docker-run: Запустить через docker-compose
docker-run:
	docker compose up -d

## docker-stop: Остановить docker-compose
docker-stop:
	docker compose down

# =====================================================================
# i18n
# =====================================================================

## i18n-check: Проверить полноту каталогов переводов (en.json ↔ ru.json)
## Сравнивает ключи между каталогами и выводит отсутствующие.
i18n-check:
	@echo "Проверка полноты каталогов переводов..."
	@EN_KEYS=$$(jq -r 'keys[]' $(I18N_EN) | grep -v '^_' | sort); \
	RU_KEYS=$$(jq -r 'keys[]' $(I18N_RU) | grep -v '^_' | sort); \
	EN_FILE=$$(mktemp); RU_FILE=$$(mktemp); \
	echo "$$EN_KEYS" > "$$EN_FILE"; echo "$$RU_KEYS" > "$$RU_FILE"; \
	MISSING_RU=$$(comm -23 "$$EN_FILE" "$$RU_FILE"); \
	MISSING_EN=$$(comm -13 "$$EN_FILE" "$$RU_FILE"); \
	rm -f "$$EN_FILE" "$$RU_FILE"; \
	EN_COUNT=$$(echo "$$EN_KEYS" | wc -l | tr -d ' '); \
	RU_COUNT=$$(echo "$$RU_KEYS" | wc -l | tr -d ' '); \
	echo "  en.json: $$EN_COUNT ключей"; \
	echo "  ru.json: $$RU_COUNT ключей"; \
	FAIL=0; \
	if [ -n "$$MISSING_RU" ]; then \
		echo ""; \
		echo "ОШИБКА: Ключи отсутствуют в ru.json:"; \
		echo "$$MISSING_RU" | sed 's/^/  - /'; \
		FAIL=1; \
	fi; \
	if [ -n "$$MISSING_EN" ]; then \
		echo ""; \
		echo "ОШИБКА: Ключи отсутствуют в en.json:"; \
		echo "$$MISSING_EN" | sed 's/^/  - /'; \
		FAIL=1; \
	fi; \
	if [ $$FAIL -eq 0 ]; then \
		echo "  Каталоги синхронизированы ✓"; \
	else \
		exit 1; \
	fi

# =====================================================================
# Утилиты
# =====================================================================

## clean: Очистить артефакты сборки
clean:
	rm -rf bin/
	rm -f $(CSS_OUTPUT)
	find . -name '*_templ.go' -delete
