# =============================================================================
# Makefile для интеграционного тестирования Storage Element в Kubernetes
#
# Оркестрирует полный цикл: сборка образов → деплой тестовой среды →
# port-forward → запуск 30 интеграционных тестов → очистка.
#
# Использование:
#   make help                  — Справка
#   make docker-build          — Собрать и запушить образы SE + JWKS Mock
#   make test-env-up           — Развернуть тестовую среду в K8s
#   make port-forward-start    — Запустить port-forward ко всем сервисам
#   make test-all              — Запустить все 30 тестов
#   make test-env-down         — Удалить тестовую среду
# =============================================================================

# -----------------------------------------------------------------------------
# Переменные
# -----------------------------------------------------------------------------
DOCKER_REGISTRY ?= harbor.kryukov.lan/library
SE_TAG          ?= v1.0.0-1
MOCK_TAG        ?= v1.0.0-1
NAMESPACE       ?= se-test
HELM_CHART      := helm/se-test
HELM_RELEASE    := se-test

# Директории (относительно этого Makefile)
SE_ROOT         := ..
SCRIPTS_DIR     := scripts
PF_PID_DIR      := .port-forward

# Образы
SE_IMAGE        := $(DOCKER_REGISTRY)/storage-element
MOCK_IMAGE      := $(DOCKER_REGISTRY)/jwks-mock

# Port-forward: локальный порт → сервис K8s
# JWKS Mock:  18080 → jwks-mock:8080
# se-edit-1:  18010 → se-edit-1:8010
# se-edit-2:  18011 → se-edit-2:8010
# se-rw-1:    18012 → se-rw-1:8010
# se-rw-2:    18013 → se-rw-2:8010
# se-ro:      18014 → se-ro:8010
# se-ar:      18015 → se-ar:8010
PF_JWKS_MOCK    := 18080:8080
PF_SE_EDIT_1    := 18010:8010
PF_SE_EDIT_2    := 18011:8010
PF_SE_RW_1      := 18012:8010
PF_SE_RW_2      := 18013:8010
PF_SE_RO        := 18014:8010
PF_SE_AR        := 18015:8010

# Переменные окружения для тестовых скриптов
export JWKS_MOCK_URL  := https://localhost:18080
export SE_EDIT_1_URL  := https://localhost:18010
export SE_EDIT_2_URL  := https://localhost:18011
export SE_RW_1_URL    := https://localhost:18012
export SE_RW_2_URL    := https://localhost:18013
export SE_RO_URL      := https://localhost:18014
export SE_AR_URL      := https://localhost:18015
export K8S_NAMESPACE  := $(NAMESPACE)
export CURL_OPTS      := -k -s --connect-timeout 5

.PHONY: help docker-build docker-build-se docker-build-mock docker-push-se docker-push-mock \
        test-env-up test-env-status test-env-logs test-env-down \
        port-forward-start port-forward-stop port-forward-status \
        test-smoke test-files test-modes test-replica test-data test-gc test-errors test-all \
        clean

# -----------------------------------------------------------------------------
# Справка
# -----------------------------------------------------------------------------

## help: Показать справку по всем целям
help:
	@echo "============================================================"
	@echo "  SE Integration Tests — Makefile"
	@echo "============================================================"
	@echo ""
	@echo "Сборка образов:"
	@echo "  docker-build         Собрать и запушить SE + JWKS Mock в Harbor"
	@echo "  docker-build-se      Собрать образ Storage Element"
	@echo "  docker-build-mock    Собрать образ JWKS Mock Server"
	@echo "  docker-push-se       Запушить образ SE в Harbor"
	@echo "  docker-push-mock     Запушить образ JWKS Mock в Harbor"
	@echo ""
	@echo "Тестовая среда:"
	@echo "  test-env-up          Развернуть тестовую среду (helm install)"
	@echo "  test-env-status      Показать статус pods, svc, pvc"
	@echo "  test-env-logs        Показать логи всех контейнеров"
	@echo "  test-env-down        Удалить тестовую среду"
	@echo ""
	@echo "Port-forward:"
	@echo "  port-forward-start   Запустить port-forward ко всем сервисам"
	@echo "  port-forward-stop    Остановить все port-forward"
	@echo "  port-forward-status  Показать статус port-forward"
	@echo ""
	@echo "Тесты (требуют port-forward-start):"
	@echo "  test-smoke           Тесты 1-3:   Health, info, metrics"
	@echo "  test-files           Тесты 4-11:  Upload, download, CRUD"
	@echo "  test-modes           Тесты 12-18: Режимы и переходы"
	@echo "  test-replica         Тесты 19-22: Leader/follower, failover"
	@echo "  test-data            Тесты 23-24: Инициализированные данные"
	@echo "  test-gc              Тесты 25-26: GC и reconciliation"
	@echo "  test-errors          Тесты 27-30: Ошибки авторизации"
	@echo "  test-all             Все 30 тестов"
	@echo ""
	@echo "Очистка:"
	@echo "  clean                Удалить тестовую среду + локальные образы"
	@echo ""
	@echo "Переменные:"
	@echo "  DOCKER_REGISTRY      $(DOCKER_REGISTRY)"
	@echo "  SE_TAG               $(SE_TAG)"
	@echo "  MOCK_TAG             $(MOCK_TAG)"
	@echo "  NAMESPACE            $(NAMESPACE)"
	@echo ""
	@echo "Быстрый старт:"
	@echo "  make docker-build && make test-env-up && make port-forward-start && make test-all"

# -----------------------------------------------------------------------------
# Сборка Docker-образов
# -----------------------------------------------------------------------------

## docker-build: Собрать и запушить оба образа
docker-build: docker-build-se docker-push-se docker-build-mock docker-push-mock
	@echo ""
	@echo "Образы собраны и запушены:"
	@echo "  $(SE_IMAGE):$(SE_TAG)"
	@echo "  $(MOCK_IMAGE):$(MOCK_TAG)"

## docker-build-se: Собрать образ Storage Element (linux/amd64 для K8s кластера)
docker-build-se:
	@echo ">>> Сборка Storage Element $(SE_TAG) (linux/amd64)..."
	docker build \
		--platform linux/amd64 \
		--build-arg VERSION=$(SE_TAG) \
		-t $(SE_IMAGE):$(SE_TAG) \
		-f $(SE_ROOT)/Dockerfile $(SE_ROOT)

## docker-push-se: Запушить образ SE в Harbor
docker-push-se:
	@echo ">>> Push $(SE_IMAGE):$(SE_TAG)..."
	docker push $(SE_IMAGE):$(SE_TAG)

## docker-build-mock: Собрать образ JWKS Mock Server (linux/amd64 для K8s кластера)
docker-build-mock:
	@echo ">>> Сборка JWKS Mock $(MOCK_TAG) (linux/amd64)..."
	docker build \
		--platform linux/amd64 \
		-t $(MOCK_IMAGE):$(MOCK_TAG) \
		-f jwks-mock/Dockerfile jwks-mock/

## docker-push-mock: Запушить образ JWKS Mock в Harbor
docker-push-mock:
	@echo ">>> Push $(MOCK_IMAGE):$(MOCK_TAG)..."
	docker push $(MOCK_IMAGE):$(MOCK_TAG)

# -----------------------------------------------------------------------------
# Тестовая среда в Kubernetes
# -----------------------------------------------------------------------------

## test-env-up: Развернуть тестовую среду через Helm
test-env-up:
	@echo ">>> Деплой тестовой среды в namespace $(NAMESPACE)..."
	helm upgrade --install $(HELM_RELEASE) $(HELM_CHART) \
		--namespace $(NAMESPACE) \
		--create-namespace \
		--set seTag=$(SE_TAG) \
		--set mockTag=$(MOCK_TAG) \
		--wait \
		--timeout 5m
	@echo ""
	@echo ">>> Тестовая среда развёрнута. Ожидание init job..."
	@kubectl wait --for=condition=complete job/se-test-init \
		-n $(NAMESPACE) --timeout=300s 2>/dev/null || \
		echo "WARN: Init job ещё не завершён или не найден. Проверьте: make test-env-status"
	@echo ""
	@$(MAKE) --no-print-directory test-env-status

## test-env-status: Показать статус тестовой среды
test-env-status:
	@echo ">>> Pods:"
	@kubectl get pods -n $(NAMESPACE) -o wide 2>/dev/null || echo "Namespace $(NAMESPACE) не найден"
	@echo ""
	@echo ">>> Services:"
	@kubectl get svc -n $(NAMESPACE) 2>/dev/null || true
	@echo ""
	@echo ">>> PVC:"
	@kubectl get pvc -n $(NAMESPACE) 2>/dev/null || true
	@echo ""
	@echo ">>> Jobs:"
	@kubectl get jobs -n $(NAMESPACE) 2>/dev/null || true

## test-env-logs: Показать логи всех контейнеров
test-env-logs:
	@echo ">>> Логи всех контейнеров в namespace $(NAMESPACE):"
	@for pod in $$(kubectl get pods -n $(NAMESPACE) -o jsonpath='{.items[*].metadata.name}' 2>/dev/null); do \
		echo ""; \
		echo "════════ $$pod ════════"; \
		kubectl logs -n $(NAMESPACE) "$$pod" --all-containers --tail=50 2>/dev/null || true; \
	done

## test-env-down: Удалить тестовую среду
test-env-down: port-forward-stop
	@echo ">>> Удаление Helm release $(HELM_RELEASE)..."
	@helm uninstall $(HELM_RELEASE) -n $(NAMESPACE) 2>/dev/null || true
	@echo ">>> Удаление namespace $(NAMESPACE)..."
	@kubectl delete namespace $(NAMESPACE) --wait=false 2>/dev/null || true
	@echo ">>> Тестовая среда удалена"

# -----------------------------------------------------------------------------
# Port-forward
# -----------------------------------------------------------------------------

## port-forward-start: Запустить port-forward ко всем сервисам
port-forward-start: port-forward-stop
	@mkdir -p $(PF_PID_DIR)
	@echo ">>> Запуск port-forward..."
	@# JWKS Mock
	@kubectl port-forward -n $(NAMESPACE) svc/jwks-mock $(PF_JWKS_MOCK) >/dev/null 2>&1 & \
		echo $$! > $(PF_PID_DIR)/jwks-mock.pid
	@# SE instances
	@kubectl port-forward -n $(NAMESPACE) svc/se-edit-1 $(PF_SE_EDIT_1) >/dev/null 2>&1 & \
		echo $$! > $(PF_PID_DIR)/se-edit-1.pid
	@kubectl port-forward -n $(NAMESPACE) svc/se-edit-2 $(PF_SE_EDIT_2) >/dev/null 2>&1 & \
		echo $$! > $(PF_PID_DIR)/se-edit-2.pid
	@kubectl port-forward -n $(NAMESPACE) svc/se-rw-1 $(PF_SE_RW_1) >/dev/null 2>&1 & \
		echo $$! > $(PF_PID_DIR)/se-rw-1.pid
	@kubectl port-forward -n $(NAMESPACE) svc/se-rw-2 $(PF_SE_RW_2) >/dev/null 2>&1 & \
		echo $$! > $(PF_PID_DIR)/se-rw-2.pid
	@kubectl port-forward -n $(NAMESPACE) svc/se-ro $(PF_SE_RO) >/dev/null 2>&1 & \
		echo $$! > $(PF_PID_DIR)/se-ro.pid
	@kubectl port-forward -n $(NAMESPACE) svc/se-ar $(PF_SE_AR) >/dev/null 2>&1 & \
		echo $$! > $(PF_PID_DIR)/se-ar.pid
	@sleep 2
	@$(MAKE) --no-print-directory port-forward-status

## port-forward-stop: Остановить все port-forward
port-forward-stop:
	@if [ -d "$(PF_PID_DIR)" ]; then \
		echo ">>> Остановка port-forward..."; \
		for pidfile in $(PF_PID_DIR)/*.pid; do \
			if [ -f "$$pidfile" ]; then \
				pid=$$(cat "$$pidfile"); \
				kill "$$pid" 2>/dev/null || true; \
				rm -f "$$pidfile"; \
			fi; \
		done; \
		rmdir $(PF_PID_DIR) 2>/dev/null || true; \
		echo ">>> Port-forward остановлен"; \
	fi

## port-forward-status: Показать статус port-forward
port-forward-status:
	@echo ">>> Port-forward статус:"
	@if [ -d "$(PF_PID_DIR)" ]; then \
		all_ok=true; \
		for pidfile in $(PF_PID_DIR)/*.pid; do \
			if [ -f "$$pidfile" ]; then \
				name=$$(basename "$$pidfile" .pid); \
				pid=$$(cat "$$pidfile"); \
				if kill -0 "$$pid" 2>/dev/null; then \
					echo "  $$name: OK (pid $$pid)"; \
				else \
					echo "  $$name: DEAD (pid $$pid)"; \
					all_ok=false; \
				fi; \
			fi; \
		done; \
		if [ "$$all_ok" = "false" ]; then \
			echo ""; \
			echo "WARN: Некоторые port-forward не работают. Перезапустите: make port-forward-start"; \
		fi; \
	else \
		echo "  Port-forward не запущен. Запустите: make port-forward-start"; \
	fi
	@echo ""
	@echo "  Маппинг портов:"
	@echo "    JWKS Mock:  localhost:18080 → jwks-mock:8080"
	@echo "    se-edit-1:  localhost:18010 → se-edit-1:8010"
	@echo "    se-edit-2:  localhost:18011 → se-edit-2:8010"
	@echo "    se-rw-1:    localhost:18012 → se-rw-1:8010"
	@echo "    se-rw-2:    localhost:18013 → se-rw-2:8010"
	@echo "    se-ro:      localhost:18014 → se-ro:8010"
	@echo "    se-ar:      localhost:18015 → se-ar:8010"

# -----------------------------------------------------------------------------
# Интеграционные тесты
# -----------------------------------------------------------------------------

## test-smoke: Тесты 1-3 — Health, info, metrics
test-smoke:
	@chmod +x $(SCRIPTS_DIR)/test-smoke.sh
	@$(SCRIPTS_DIR)/test-smoke.sh

## test-files: Тесты 4-11 — Upload, download, list, metadata, update, delete
test-files:
	@chmod +x $(SCRIPTS_DIR)/test-files.sh
	@$(SCRIPTS_DIR)/test-files.sh

## test-modes: Тесты 12-18 — Режимы работы и переходы
test-modes:
	@chmod +x $(SCRIPTS_DIR)/test-modes.sh
	@$(SCRIPTS_DIR)/test-modes.sh

## test-replica: Тесты 19-22 — Leader/follower election, proxy, failover
test-replica:
	@chmod +x $(SCRIPTS_DIR)/test-replica.sh
	@$(SCRIPTS_DIR)/test-replica.sh

## test-data: Тесты 23-24 — Проверка инициализированных данных
test-data:
	@chmod +x $(SCRIPTS_DIR)/test-data.sh
	@$(SCRIPTS_DIR)/test-data.sh

## test-gc: Тесты 25-26 — GC и reconciliation
test-gc:
	@chmod +x $(SCRIPTS_DIR)/test-gc-reconcile.sh
	@$(SCRIPTS_DIR)/test-gc-reconcile.sh

## test-errors: Тесты 27-30 — Ошибки авторизации и лимиты
test-errors:
	@chmod +x $(SCRIPTS_DIR)/test-errors.sh
	@$(SCRIPTS_DIR)/test-errors.sh

## test-all: Запустить все 30 тестов
test-all:
	@chmod +x $(SCRIPTS_DIR)/test-all.sh
	@$(SCRIPTS_DIR)/test-all.sh

# -----------------------------------------------------------------------------
# Очистка
# -----------------------------------------------------------------------------

## clean: Полная очистка — тестовая среда + локальные образы
clean: test-env-down
	@echo ">>> Удаление локальных образов..."
	@docker rmi $(SE_IMAGE):$(SE_TAG) 2>/dev/null || true
	@docker rmi $(MOCK_IMAGE):$(MOCK_TAG) 2>/dev/null || true
	@rm -rf $(PF_PID_DIR)
	@echo ">>> Очистка завершена"
