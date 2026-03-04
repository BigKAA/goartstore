# Demo Client — Makefile
# Паттерн аналогичен Admin Module
# Проверки: lint, lint-docker, lint-helm, security-scan, secrets-scan, check-all

BINARY_NAME := demo-client
MODULE := github.com/bigkaa/goartstore/demo-client
VERSION ?= dev

DOCKER_REGISTRY := harbor.kryukov.lan/library
DOCKER_IMAGE := $(DOCKER_REGISTRY)/$(BINARY_NAME)
TAG ?= $(VERSION)

# Директории
BIN_DIR := bin
CMD_DIR := cmd/$(BINARY_NAME)

# Инструменты
TEMPL := templ
TAILWIND := npx tailwindcss

# CSS пути
CSS_INPUT := internal/ui/static/css/input.css
CSS_OUTPUT := internal/ui/static/css/output.css

# Версии инструментов проверки кода
GOLANGCI_LINT_VERSION := v2.10.1
HADOLINT_VERSION := v2.14.0
TRIVY_VERSION := 0.69.1
GITLEAKS_VERSION := v8.30.0

# Пути
ROOT_DIR := $(shell cd ../.. && pwd)
HELM_CHART := charts/$(BINARY_NAME)

# Каталоги переводов
I18N_DIR := internal/ui/i18n/locales
I18N_EN := $(I18N_DIR)/en.json
I18N_RU := $(I18N_DIR)/ru.json

.PHONY: all build test test-race lint lint-docker lint-helm \
	security-scan secrets-scan check-all install-tools clean \
	templ-generate css-build css-watch ui-build \
	docker-build docker-run docker-stop \
	i18n-check help

## --- Сборка ---

# Полная сборка (templ → CSS → Go)
all: ui-build build

# Сборка Go-бинарника с инъекцией версии через ldflags
build:
	@echo "==> Сборка $(BINARY_NAME) $(VERSION)..."
	go build -ldflags "-X $(MODULE)/internal/config.Version=$(VERSION)" \
		-o $(BIN_DIR)/$(BINARY_NAME) ./$(CMD_DIR)/

## --- UI ---

# Генерация Go-кода из .templ файлов
templ-generate:
	@echo "==> Генерация templ..."
	$(TEMPL) generate

# Компиляция Tailwind CSS
css-build:
	@echo "==> Сборка Tailwind CSS..."
	$(TAILWIND) -i $(CSS_INPUT) -o $(CSS_OUTPUT) --minify

# Tailwind CSS в режиме watch (для разработки)
css-watch:
	$(TAILWIND) -i $(CSS_INPUT) -o $(CSS_OUTPUT) --watch

# Полная сборка UI: templ → CSS
ui-build: templ-generate css-build

## --- Тесты ---

# Запуск unit-тестов
test:
	@echo "==> Запуск тестов..."
	go test ./... -count=1

# Тесты с race detector
test-race:
	@echo "==> Запуск тестов (race)..."
	go test -race ./... -count=1

## --- Проверки кода ---

# Запуск golangci-lint (используется конфиг из корня проекта)
lint:
	@echo "==> golangci-lint..."
	golangci-lint run ./...

# Проверка Dockerfile с помощью hadolint
lint-docker:
	@echo "=== hadolint: Dockerfile ==="
	hadolint --config $(ROOT_DIR)/.hadolint.yaml Dockerfile

# Проверка Helm chart
lint-helm:
	@echo "=== helm lint: $(HELM_CHART) ==="
	helm lint $(HELM_CHART)

# Сканирование уязвимостей в зависимостях и Docker-образе (trivy)
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

# Сканирование на утечку секретов (gitleaks)
secrets-scan:
	@echo "=== gitleaks: сканирование репозитория ==="
	cd $(ROOT_DIR) && gitleaks detect --config .gitleaks.toml --source . -v

# Запуск всех проверок
check-all: lint lint-docker lint-helm security-scan secrets-scan i18n-check
	@echo ""
	@echo "========================================="
	@echo "  Все проверки завершены успешно"
	@echo "========================================="

## --- Установка инструментов ---

# Установка всех инструментов проверки кода
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

## --- Docker ---

# Сборка Docker-образа
docker-build:
	@echo "==> Сборка Docker-образа $(DOCKER_IMAGE):$(TAG)..."
	docker build --build-arg VERSION=$(TAG) \
		-t $(DOCKER_IMAGE):$(TAG) -f Dockerfile .

# Запуск через docker-compose
docker-run:
	docker compose up -d

# Остановка через docker-compose
docker-stop:
	docker compose down

## --- i18n ---

# Проверка полноты каталогов переводов (en.json ↔ ru.json)
i18n-check:
	@echo "==> Проверка i18n ключей..."
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

## --- Очистка ---

clean:
	@echo "==> Очистка..."
	rm -rf bin/
	rm -f $(CSS_OUTPUT)
	find . -name '*_templ.go' -delete

## --- Справка ---

help:
	@echo "Доступные цели:"
	@echo ""
	@echo "  Сборка:"
	@echo "  all             — Полная сборка (templ → CSS → Go)"
	@echo "  build           — Сборка Go-бинарника"
	@echo "  ui-build        — templ + CSS"
	@echo "  templ-generate  — Генерация Go-кода из .templ"
	@echo "  css-build       — Компиляция Tailwind CSS"
	@echo "  css-watch       — Tailwind CSS watch (разработка)"
	@echo ""
	@echo "  Тесты:"
	@echo "  test            — Unit-тесты"
	@echo "  test-race       — Тесты с race detector"
	@echo ""
	@echo "  Проверки кода:"
	@echo "  lint            — golangci-lint (Go-код)"
	@echo "  lint-docker     — hadolint (Dockerfile)"
	@echo "  lint-helm       — helm lint (Helm chart)"
	@echo "  security-scan   — Сканирование уязвимостей (trivy: зависимости + образ)"
	@echo "  secrets-scan    — Сканирование на утечку секретов (gitleaks)"
	@echo "  check-all       — Все проверки одной командой"
	@echo ""
	@echo "  Инструменты:"
	@echo "  install-tools   — Установить инструменты проверки кода"
	@echo ""
	@echo "  i18n:"
	@echo "  i18n-check      — Проверка полноты каталогов переводов (en ↔ ru)"
	@echo ""
	@echo "  Docker:"
	@echo "  docker-build    — Сборка Docker-образа"
	@echo "  docker-run      — Запуск через docker-compose"
	@echo "  docker-stop     — Остановка через docker-compose"
	@echo ""
	@echo "  Прочее:"
	@echo "  clean           — Очистка артефактов сборки"
