# =============================================================================
# Makefile для интеграционного тестирования Artsore в Kubernetes
#
# Три независимых Helm chart-а (artsore-infra, artsore-se, artsore-apps)
# деплоятся в один namespace artsore-test. Init Job — standalone manifest.
#
# Использование:
#   make help                  — Справка
#   make docker-build          — Собрать и запушить образы AM + SE
#   make test-env-up           — Развернуть всю тестовую среду
#   make port-forward-start    — Запустить port-forward ко всем сервисам
#   make test-env-down         — Удалить тестовую среду
# =============================================================================

# -----------------------------------------------------------------------------
# Переменные
# -----------------------------------------------------------------------------
DOCKER_REGISTRY ?= harbor.kryukov.lan/library
AM_TAG          ?= v0.1.0-1
SE_TAG          ?= v0.1.0-1
NAMESPACE       ?= artsore-test
AM_PORT         ?= 8000
SE_PORT         ?= 8010

# Helm chart-ы и release names
INFRA_CHART     := helm/artsore-infra
INFRA_RELEASE   := artsore-infra
SE_CHART        := helm/artsore-se
SE_RELEASE      := artsore-se
APPS_CHART      := helm/artsore-apps
APPS_RELEASE    := artsore-apps

# Init Job manifest
INIT_JOB_SRC    := helm/init-job/job.yaml

# Директории
AM_ROOT         := ../src/admin-module
SE_ROOT         := ../src/storage-element
REALM_SRC       := ../deploy/keycloak/artsore-realm.json
REALM_DST       := helm/artsore-infra/files/artsore-realm.json
SCRIPTS_DIR     := scripts
PF_PID_DIR      := .port-forward

# Образы
AM_IMAGE        := $(DOCKER_REGISTRY)/admin-module
SE_IMAGE        := $(DOCKER_REGISTRY)/storage-element

# Port-forward: локальный порт → сервис K8s
PF_PG           := 15432:5432
PF_KC           := 18080:8443
PF_AM           := 18000:$(AM_PORT)
PF_SE_EDIT_1    := 18010:$(SE_PORT)
PF_SE_EDIT_2    := 18011:$(SE_PORT)
PF_SE_RW_1      := 18012:$(SE_PORT)
PF_SE_RW_2      := 18013:$(SE_PORT)
PF_SE_RO        := 18014:$(SE_PORT)
PF_SE_AR        := 18015:$(SE_PORT)

# Переменные окружения для тестовых скриптов
export KC_URL         := https://localhost:18080
export KC_TOKEN_URL   := https://localhost:18080/realms/artsore/protocol/openid-connect/token
export AM_URL         := https://localhost:18000
export SE_EDIT_1_URL  := https://localhost:18010
export SE_EDIT_2_URL  := https://localhost:18011
export SE_RW_1_URL    := https://localhost:18012
export SE_RW_2_URL    := https://localhost:18013
export SE_RO_URL      := https://localhost:18014
export SE_AR_URL      := https://localhost:18015
export K8S_NAMESPACE  := $(NAMESPACE)
export CURL_OPTS      := -k -s --connect-timeout 5

.PHONY: help docker-build docker-build-am docker-build-se docker-push-am docker-push-se \
        copy-realm infra-up infra-down se-up se-down apps-up apps-down \
        init-data init-data-clean test-env-up test-env-down \
        test-env-status test-env-logs \
        port-forward-start port-forward-stop port-forward-status \
        port-forward-infra port-forward-se port-forward-apps \
        test-all clean

# -----------------------------------------------------------------------------
# Справка
# -----------------------------------------------------------------------------

## help: Показать справку
help:
	@echo "============================================================"
	@echo "  Artsore Integration Tests — Makefile"
	@echo "============================================================"
	@echo ""
	@echo "Сборка образов:"
	@echo "  docker-build         Собрать и запушить AM + SE в Harbor"
	@echo "  docker-build-am      Собрать образ Admin Module"
	@echo "  docker-build-se      Собрать образ Storage Element"
	@echo ""
	@echo "Тестовая среда (по слоям):"
	@echo "  infra-up             Деплой инфраструктуры (PG + KC)"
	@echo "  infra-down           Удалить инфраструктуру"
	@echo "  se-up                Деплой Storage Elements (6 SE)"
	@echo "  se-down              Удалить Storage Elements"
	@echo "  apps-up              Деплой приложений (Admin Module)"
	@echo "  apps-down            Удалить приложения"
	@echo "  init-data            Запустить Init Job (загрузка данных)"
	@echo "  init-data-clean      Удалить Init Job (для перезапуска)"
	@echo ""
	@echo "Тестовая среда (всё сразу):"
	@echo "  test-env-up          Развернуть всё: infra → se → apps"
	@echo "  test-env-down        Удалить всё: apps → se → infra → ns"
	@echo "  test-env-status      Показать статус pods, svc, pvc, jobs"
	@echo "  test-env-logs        Показать логи всех контейнеров"
	@echo ""
	@echo "Port-forward:"
	@echo "  port-forward-start   Запустить port-forward ко всем сервисам"
	@echo "  port-forward-stop    Остановить все port-forward"
	@echo "  port-forward-status  Показать статус port-forward"
	@echo ""
	@echo "Тесты (требуют port-forward-start):"
	@echo "  test-all             Запустить все интеграционные тесты"
	@echo ""
	@echo "Очистка:"
	@echo "  clean                Удалить тестовую среду + локальные образы"
	@echo ""
	@echo "Переменные:"
	@echo "  DOCKER_REGISTRY      $(DOCKER_REGISTRY)"
	@echo "  AM_TAG               $(AM_TAG)"
	@echo "  SE_TAG               $(SE_TAG)"
	@echo "  NAMESPACE            $(NAMESPACE)"
	@echo ""
	@echo "Быстрый старт:"
	@echo "  make docker-build && make test-env-up && make port-forward-start"

# -----------------------------------------------------------------------------
# Сборка Docker-образов
# -----------------------------------------------------------------------------

## docker-build: Собрать и запушить оба образа
docker-build: docker-build-am docker-push-am docker-build-se docker-push-se
	@echo ""
	@echo "Образы собраны и запушены:"
	@echo "  $(AM_IMAGE):$(AM_TAG)"
	@echo "  $(SE_IMAGE):$(SE_TAG)"

## docker-build-am: Собрать образ Admin Module
docker-build-am:
	@echo ">>> Сборка Admin Module $(AM_TAG) (linux/amd64)..."
	docker build --platform linux/amd64 --build-arg VERSION=$(AM_TAG) \
		-t $(AM_IMAGE):$(AM_TAG) -f $(AM_ROOT)/Dockerfile $(AM_ROOT)

## docker-push-am: Запушить образ AM в Harbor
docker-push-am:
	@echo ">>> Push $(AM_IMAGE):$(AM_TAG)..."
	docker push $(AM_IMAGE):$(AM_TAG)

## docker-build-se: Собрать образ Storage Element
docker-build-se:
	@echo ">>> Сборка Storage Element $(SE_TAG) (linux/amd64)..."
	docker build --platform linux/amd64 --build-arg VERSION=$(SE_TAG) \
		-t $(SE_IMAGE):$(SE_TAG) -f $(SE_ROOT)/Dockerfile $(SE_ROOT)

## docker-push-se: Запушить образ SE в Harbor
docker-push-se:
	@echo ">>> Push $(SE_IMAGE):$(SE_TAG)..."
	docker push $(SE_IMAGE):$(SE_TAG)

# -----------------------------------------------------------------------------
# Подготовка
# -----------------------------------------------------------------------------

## copy-realm: Копировать realm.json в artsore-infra files/
copy-realm:
	@echo ">>> Копирование realm.json..."
	@mkdir -p $(dir $(REALM_DST))
	@cp $(REALM_SRC) $(REALM_DST)
	@echo "  $(REALM_SRC) -> $(REALM_DST)"

# -----------------------------------------------------------------------------
# Деплой по слоям
# -----------------------------------------------------------------------------

## infra-up: Деплой artsore-infra (PG + KC)
infra-up: copy-realm
	@echo ">>> Деплой artsore-infra в namespace $(NAMESPACE)..."
	helm upgrade --install $(INFRA_RELEASE) $(INFRA_CHART) \
		--namespace $(NAMESPACE) \
		--create-namespace \
		--wait --timeout 10m
	@echo ">>> Ожидание готовности PostgreSQL..."
	@kubectl wait --for=condition=ready pod -l app.kubernetes.io/name=postgresql \
		-n $(NAMESPACE) --timeout=120s
	@echo ">>> Ожидание готовности Keycloak..."
	@kubectl wait --for=condition=ready pod -l app.kubernetes.io/name=keycloak \
		-n $(NAMESPACE) --timeout=300s
	@echo ">>> artsore-infra развёрнут"

## infra-down: Удалить artsore-infra
infra-down:
	@echo ">>> Удаление artsore-infra..."
	@helm uninstall $(INFRA_RELEASE) -n $(NAMESPACE) 2>/dev/null || true
	@echo ">>> artsore-infra удалён"

## se-up: Деплой artsore-se (6 SE)
se-up:
	@echo ">>> Деплой artsore-se в namespace $(NAMESPACE)..."
	helm upgrade --install $(SE_RELEASE) $(SE_CHART) \
		--namespace $(NAMESPACE) \
		--set seTag=$(SE_TAG) \
		--wait --timeout 10m
	@echo ">>> Ожидание готовности SE pods..."
	@kubectl wait --for=condition=ready pod -l app.kubernetes.io/name=storage-element \
		-n $(NAMESPACE) --timeout=300s
	@echo ">>> artsore-se развёрнут (6 SE)"

## se-down: Удалить artsore-se
se-down:
	@echo ">>> Удаление artsore-se..."
	@helm uninstall $(SE_RELEASE) -n $(NAMESPACE) 2>/dev/null || true
	@echo ">>> artsore-se удалён"

## apps-up: Деплой artsore-apps (AM)
apps-up:
	@echo ">>> Деплой artsore-apps в namespace $(NAMESPACE)..."
	helm upgrade --install $(APPS_RELEASE) $(APPS_CHART) \
		--namespace $(NAMESPACE) \
		--set amTag=$(AM_TAG) \
		--wait --timeout 10m
	@echo ">>> Ожидание готовности Admin Module..."
	@kubectl wait --for=condition=ready pod -l app.kubernetes.io/name=admin-module \
		-n $(NAMESPACE) --timeout=300s
	@echo ">>> artsore-apps развёрнут"

## apps-down: Удалить artsore-apps
apps-down:
	@echo ">>> Удаление artsore-apps..."
	@helm uninstall $(APPS_RELEASE) -n $(NAMESPACE) 2>/dev/null || true
	@echo ">>> artsore-apps удалён"

## init-data: Запустить Init Job (загрузка данных, mode transitions)
init-data:
	@echo ">>> Запуск Init Job..."
	@export NAMESPACE=$(NAMESPACE) INFRA_RELEASE=$(INFRA_RELEASE) \
		AM_PORT=$(AM_PORT) SE_PORT=$(SE_PORT); \
		envsubst '$$NAMESPACE $$INFRA_RELEASE $$AM_PORT $$SE_PORT' \
		< $(INIT_JOB_SRC) | kubectl apply -f -
	@echo ">>> Ожидание завершения Init Job (до 600с)..."
	@kubectl wait --for=condition=complete job/artsore-init \
		-n $(NAMESPACE) --timeout=600s
	@echo ">>> Init Job завершён"

## init-data-clean: Удалить Init Job (для перезапуска)
init-data-clean:
	@echo ">>> Удаление Init Job ресурсов..."
	@kubectl delete job artsore-init -n $(NAMESPACE) 2>/dev/null || true
	@kubectl delete configmap artsore-init-lib artsore-init-data -n $(NAMESPACE) 2>/dev/null || true
	@echo ">>> Init Job ресурсы удалены"

# -----------------------------------------------------------------------------
# Тестовая среда (всё сразу)
# -----------------------------------------------------------------------------

## test-env-up: Развернуть всё — infra → se → apps (последовательно)
test-env-up: infra-up se-up apps-up
	@echo ""
	@echo ">>> Тестовая среда полностью развёрнута"
	@$(MAKE) --no-print-directory test-env-status

## test-env-down: Удалить всё — apps → se → infra → namespace
test-env-down: port-forward-stop init-data-clean apps-down se-down infra-down
	@echo ">>> Удаление namespace $(NAMESPACE)..."
	@kubectl delete namespace $(NAMESPACE) --wait=false 2>/dev/null || true
	@echo ">>> Тестовая среда полностью удалена"

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
	@echo ""
	@echo ">>> Helm releases:"
	@helm list -n $(NAMESPACE) 2>/dev/null || true

## test-env-logs: Показать логи всех контейнеров
test-env-logs:
	@echo ">>> Логи всех контейнеров в namespace $(NAMESPACE):"
	@for pod in $$(kubectl get pods -n $(NAMESPACE) -o jsonpath='{.items[*].metadata.name}' 2>/dev/null); do \
		echo ""; \
		echo "════════ $$pod ════════"; \
		kubectl logs -n $(NAMESPACE) "$$pod" --all-containers --tail=50 2>/dev/null || true; \
	done

# -----------------------------------------------------------------------------
# Port-forward
# -----------------------------------------------------------------------------

## port-forward-start: Запустить port-forward ко всем сервисам
port-forward-start: port-forward-stop port-forward-infra port-forward-se port-forward-apps
	@sleep 2
	@$(MAKE) --no-print-directory port-forward-status

## port-forward-infra: Port-forward для PG + KC
port-forward-infra:
	@mkdir -p $(PF_PID_DIR)
	@echo ">>> Port-forward: infra (PG + KC)..."
	@kubectl port-forward -n $(NAMESPACE) svc/postgresql $(PF_PG) >/dev/null 2>&1 & \
		echo $$! > $(PF_PID_DIR)/postgresql.pid
	@kubectl port-forward -n $(NAMESPACE) svc/$(INFRA_RELEASE)-keycloak $(PF_KC) >/dev/null 2>&1 & \
		echo $$! > $(PF_PID_DIR)/keycloak.pid

## port-forward-se: Port-forward для SE
port-forward-se:
	@mkdir -p $(PF_PID_DIR)
	@echo ">>> Port-forward: SE (6 экземпляров)..."
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

## port-forward-apps: Port-forward для AM
port-forward-apps:
	@mkdir -p $(PF_PID_DIR)
	@echo ">>> Port-forward: apps (AM)..."
	@kubectl port-forward -n $(NAMESPACE) svc/admin-module $(PF_AM) >/dev/null 2>&1 & \
		echo $$! > $(PF_PID_DIR)/admin-module.pid

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
	@echo "    PostgreSQL:   localhost:15432 → postgresql:5432"
	@echo "    Keycloak:     localhost:18080 → keycloak:8443"
	@echo "    Admin Module: localhost:18000 → admin-module:$(AM_PORT)"
	@echo "    se-edit-1:    localhost:18010 → se-edit-1:$(SE_PORT)"
	@echo "    se-edit-2:    localhost:18011 → se-edit-2:$(SE_PORT)"
	@echo "    se-rw-1:      localhost:18012 → se-rw-1:$(SE_PORT)"
	@echo "    se-rw-2:      localhost:18013 → se-rw-2:$(SE_PORT)"
	@echo "    se-ro:        localhost:18014 → se-ro:$(SE_PORT)"
	@echo "    se-ar:        localhost:18015 → se-ar:$(SE_PORT)"

# -----------------------------------------------------------------------------
# Интеграционные тесты
# -----------------------------------------------------------------------------

## test-all: Запустить все интеграционные тесты
test-all:
	@echo ">>> Интеграционные тесты пока не реализованы"
	@echo "  Реализация будет в Phase 7"

# -----------------------------------------------------------------------------
# Очистка
# -----------------------------------------------------------------------------

## clean: Полная очистка — тестовая среда + локальные образы
clean: test-env-down
	@echo ">>> Удаление локальных образов..."
	@docker rmi $(AM_IMAGE):$(AM_TAG) 2>/dev/null || true
	@docker rmi $(SE_IMAGE):$(SE_TAG) 2>/dev/null || true
	@rm -rf $(PF_PID_DIR)
	@rm -f $(REALM_DST)
	@echo ">>> Очистка завершена"
