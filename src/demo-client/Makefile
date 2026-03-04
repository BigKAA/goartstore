# Demo Client — Makefile
# Паттерн аналогичен Admin Module

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

.PHONY: all build test test-race lint clean \
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

## --- Линтер ---

lint:
	@echo "==> Линтер..."
	golangci-lint run ./...

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

# Проверка наличия всех ключей в обоих локалях
i18n-check:
	@echo "==> Проверка i18n ключей..."
	@python3 -c "\
	import json, sys; \
	ru = set(json.load(open('internal/ui/i18n/locales/ru.json')).keys()); \
	en = set(json.load(open('internal/ui/i18n/locales/en.json')).keys()); \
	missing_en = ru - en; missing_ru = en - ru; \
	[print(f'Отсутствует в en.json: {k}') for k in sorted(missing_en)]; \
	[print(f'Отсутствует в ru.json: {k}') for k in sorted(missing_ru)]; \
	sys.exit(1) if missing_en or missing_ru else print('OK: все ключи синхронизированы')"

## --- Очистка ---

clean:
	@echo "==> Очистка..."
	rm -rf $(BIN_DIR)
	rm -f $(CSS_OUTPUT)

## --- Справка ---

help:
	@echo "Доступные targets:"
	@echo "  all            — Полная сборка (templ → CSS → Go)"
	@echo "  build          — Сборка Go-бинарника"
	@echo "  templ-generate — Генерация Go-кода из .templ"
	@echo "  css-build      — Компиляция Tailwind CSS"
	@echo "  css-watch      — Tailwind CSS watch (разработка)"
	@echo "  ui-build       — templ + CSS"
	@echo "  test           — Unit-тесты"
	@echo "  test-race      — Тесты с race detector"
	@echo "  lint           — golangci-lint"
	@echo "  docker-build   — Сборка Docker-образа"
	@echo "  docker-run     — Запуск через docker-compose"
	@echo "  docker-stop    — Остановка через docker-compose"
	@echo "  i18n-check     — Проверка i18n ключей"
	@echo "  clean          — Очистка артефактов"
