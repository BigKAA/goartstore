# Makefile для Admin Module
# Основные цели: build, test, generate, lint, docker-build, docker-run

BINARY_NAME := admin-module
MODULE := github.com/arturkryukov/artsore/admin-module
VERSION ?= dev
OPENAPI_SPEC := ../../docs/api-contracts/admin-module-openapi.yaml

# Docker
DOCKER_REGISTRY := harbor.kryukov.lan/library
DOCKER_IMAGE := $(DOCKER_REGISTRY)/$(BINARY_NAME)
TAG ?= $(VERSION)

.PHONY: build test test-race generate lint docker-build docker-run docker-stop clean help

## help: Показать справку по целям
help:
	@echo "Доступные цели:"
	@echo "  build        — Собрать бинарник"
	@echo "  test         — Запустить unit-тесты"
	@echo "  test-race    — Запустить тесты с race detector"
	@echo "  generate     — Сгенерировать код из OpenAPI"
	@echo "  lint         — Запустить линтер"
	@echo "  docker-build — Собрать Docker-образ (TAG=v0.1.0-1)"
	@echo "  docker-run   — Запустить через docker-compose"
	@echo "  docker-stop  — Остановить docker-compose"
	@echo "  clean        — Очистить артефакты сборки"

## build: Собрать бинарник admin-module
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
docker-run:
	docker compose up -d

## docker-stop: Остановить docker-compose
docker-stop:
	docker compose down

## clean: Очистить артефакты сборки
clean:
	rm -rf bin/
