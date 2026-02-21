# Makefile для Storage Element
# Основные цели: build, test, generate, lint, docker-build, docker-run, dev-certs

BINARY_NAME := storage-element
MODULE := github.com/arturkryukov/artsore/storage-element
VERSION ?= dev
OPENAPI_SPEC := ../../docs/api-contracts/storage-element-openapi.yaml

# Docker
DOCKER_REGISTRY := harbor.kryukov.lan/library
DOCKER_IMAGE := $(DOCKER_REGISTRY)/$(BINARY_NAME)
TAG ?= $(VERSION)

# Линтер
GOLANGCI_LINT_VERSION := v1.64.8

.PHONY: build test generate lint docker-build docker-run dev-certs clean help

## help: Показать справку по целям
help:
	@echo "Доступные цели:"
	@echo "  build        — Собрать бинарник"
	@echo "  test         — Запустить unit-тесты"
	@echo "  test-race    — Запустить тесты с race detector"
	@echo "  generate     — Сгенерировать код из OpenAPI"
	@echo "  lint         — Запустить линтер"
	@echo "  docker-build — Собрать Docker-образ (TAG=v1.0.0-1)"
	@echo "  docker-run   — Запустить через docker-compose"
	@echo "  dev-certs    — Сгенерировать self-signed TLS сертификаты"
	@echo "  clean        — Очистить артефакты сборки"

## build: Собрать бинарник storage-element
build:
	go build -ldflags "-X $(MODULE)/internal/config.Version=$(VERSION)" \
		-o bin/$(BINARY_NAME) ./cmd/$(BINARY_NAME)/

## test: Запустить unit-тесты
test:
	go test ./... -count=1

## test-race: Запустить тесты с детектором гонок
test-race:
	go test -race ./... -count=1

## generate: Сгенерировать Go-код из OpenAPI спецификации
generate:
	oapi-codegen --config oapi-codegen-types.yaml $(OPENAPI_SPEC)
	oapi-codegen --config oapi-codegen-server.yaml $(OPENAPI_SPEC)

## lint: Запустить golangci-lint
lint:
	golangci-lint run ./...

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

## dev-certs: Сгенерировать self-signed TLS сертификаты для разработки
dev-certs:
	@mkdir -p certs
	@if [ ! -f certs/tls.crt ]; then \
		openssl req -x509 -newkey rsa:4096 -nodes \
			-keyout certs/tls.key \
			-out certs/tls.crt \
			-days 365 \
			-subj "/CN=storage-element/O=Artsore/C=RU" \
			-addext "subjectAltName=DNS:localhost,DNS:storage-element,IP:127.0.0.1" 2>/dev/null; \
		echo "TLS сертификаты сгенерированы в certs/"; \
	else \
		echo "TLS сертификаты уже существуют в certs/"; \
	fi

## clean: Очистить артефакты сборки
clean:
	rm -rf bin/
	rm -rf certs/
