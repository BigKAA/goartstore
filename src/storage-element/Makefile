# Makefile для Storage Element
# Основные цели: build, test, generate, lint, docker-build, docker-run, dev-certs
# Проверки: lint, lint-docker, lint-helm, security-scan, secrets-scan, check-all

BINARY_NAME := storage-element
MODULE := github.com/bigkaa/goartstore/storage-element
VERSION ?= dev
OPENAPI_SPEC := ../../docs/api-contracts/storage-element-openapi.yaml

# Docker
DOCKER_REGISTRY := harbor.kryukov.lan/library
DOCKER_IMAGE := $(DOCKER_REGISTRY)/$(BINARY_NAME)
TAG ?= $(VERSION)

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
        docker-build docker-run docker-stop dev-certs clean help

## help: Показать справку по целям
help:
	@echo "Доступные цели:"
	@echo ""
	@echo "  Сборка:"
	@echo "  build           — Собрать бинарник"
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
	@echo "  Docker:"
	@echo "  docker-build    — Собрать Docker-образ (TAG=v0.1.0-1)"
	@echo "  docker-run      — Запустить через docker-compose"
	@echo "  docker-stop     — Остановить docker-compose"
	@echo ""
	@echo "  Прочее:"
	@echo "  dev-certs       — Сгенерировать self-signed TLS сертификаты"
	@echo "  clean           — Очистить артефакты сборки"

# =====================================================================
# Сборка
# =====================================================================

## build: Собрать бинарник storage-element
build:
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
	@echo "=== hadolint: tests/jwks-mock/Dockerfile ==="
	hadolint --config $(ROOT_DIR)/.hadolint.yaml tests/jwks-mock/Dockerfile

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

## docker-build: Собрать Docker-образ
docker-build:
	docker build \
		--build-arg VERSION=$(TAG) \
		-t $(DOCKER_IMAGE):$(TAG) \
		-f Dockerfile .

## docker-run: Запустить через docker-compose
docker-run: dev-certs
	docker compose up -d

## docker-stop: Остановить docker-compose
docker-stop:
	docker compose down

# =====================================================================
# Утилиты
# =====================================================================

## dev-certs: Сгенерировать self-signed TLS сертификаты для разработки
dev-certs:
	@mkdir -p certs
	@if [ ! -f certs/tls.crt ]; then \
		openssl req -x509 -newkey rsa:4096 -nodes \
			-keyout certs/tls.key \
			-out certs/tls.crt \
			-days 365 \
			-subj "/CN=storage-element/O=Artstore/C=RU" \
			-addext "subjectAltName=DNS:localhost,DNS:storage-element,IP:127.0.0.1" 2>/dev/null; \
		echo "TLS сертификаты сгенерированы в certs/"; \
	else \
		echo "TLS сертификаты уже существуют в certs/"; \
	fi

## clean: Очистить артефакты сборки
clean:
	rm -rf bin/
	rm -rf certs/
