# Makefile для Admin Module
# Основные цели: build, test, generate, lint, docker-build, docker-run
# UI цели: templ-generate, css-build, ui-build

BINARY_NAME := admin-module
MODULE := github.com/arturkryukov/artstore/admin-module
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

.PHONY: build test test-race generate lint docker-build docker-run docker-stop clean help \
        templ-generate css-build css-watch ui-build

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
	@echo "  Линтер:"
	@echo "  lint            — Запустить golangci-lint"
	@echo ""
	@echo "  Docker:"
	@echo "  docker-build    — Собрать Docker-образ с UI (templ + tailwind в Docker)"
	@echo "  docker-run      — Запустить через docker-compose"
	@echo "  docker-stop     — Остановить docker-compose"
	@echo ""
	@echo "  Прочее:"
	@echo "  clean           — Очистить артефакты сборки"

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

## build: Собрать бинарник admin-module (с предварительной сборкой UI)
build: ui-build
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

## clean: Очистить артефакты сборки
clean:
	rm -rf bin/
	rm -f $(CSS_OUTPUT)
	find . -name '*_templ.go' -delete
