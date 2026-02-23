# =============================================================================
# Makefile для интеграционного тестирования Artsore в Kubernetes
#
# Оркестрирует полный цикл: сборка образов → деплой тестовой среды
# (PG + Keycloak + Admin Module + 6 SE) → port-forward → тесты → очистка.
#
# Использование:
#   make help                  — Справка
#   make docker-build          — Собрать и запушить образы AM + SE
#   make test-env-up           — Развернуть тестовую среду в K8s
#   make port-forward-start    — Запустить port-forward ко всем сервисам
#   make test-all              — Запустить все интеграционные тесты
#   make test-env-down         — Удалить тестовую среду
# =============================================================================

# -----------------------------------------------------------------------------
# Переменные
# -----------------------------------------------------------------------------
DOCKER_REGISTRY ?= harbor.kryukov.lan/library
AM_TAG          ?= v2.0.0-1
SE_TAG          ?= v1.0.0-1
NAMESPACE       ?= artsore-test
HELM_CHART      := helm/artsore-test
HELM_RELEASE    := artsore-test

# Директории (относительно этого Makefile)
AM_ROOT         := ../src/admin-module
SE_ROOT         := ../src/storage-element
REALM_SRC       := ../deploy/keycloak/artsore-realm.json
REALM_DST       := helm/artsore-test/files/artsore-realm.json
SCRIPTS_DIR     := scripts
PF_PID_DIR      := .port-forward

# Образы
AM_IMAGE        := $(DOCKER_REGISTRY)/admin-module
SE_IMAGE        := $(DOCKER_REGISTRY)/storage-element

# Port-forward: локальный порт → сервис K8s
# PostgreSQL: 15432 → postgresql:5432
# Keycloak:   18080 → keycloak:8443
# AM:         18000 → admin-module:8000
# se-edit-1:  18010 → se-edit-1:8010
# se-edit-2:  18011 → se-edit-2:8010
# se-rw-1:    18012 → se-rw-1:8010
# se-rw-2:    18013 → se-rw-2:8010
# se-ro:      18014 → se-ro:8010
# se-ar:      18015 → se-ar:8010
PF_PG           := 15432:5432
PF_KC           := 18080:8443
PF_AM           := 18000:8000
PF_SE_EDIT_1    := 18010:8010
PF_SE_EDIT_2    := 18011:8010
PF_SE_RW_1      := 18012:8010
PF_SE_RW_2      := 18013:8010
PF_SE_RO        := 18014:8010
PF_SE_AR        := 18015:8010

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
        copy-realm helm-dep-update test-env-up test-env-status test-env-logs test-env-down \
        port-forward-start port-forward-stop port-forward-status \
        test-all clean

# -----------------------------------------------------------------------------
# Справка
# -----------------------------------------------------------------------------

## help: Показать справку по всем целям
help:
	@echo "============================================================"
	@echo "  Artsore Integration Tests — Makefile"
	@echo "============================================================"
	@echo ""
	@echo "Сборка образов:"
	@echo "  docker-build         Собрать и запушить AM + SE в Harbor"
	@echo "  docker-build-am      Собрать образ Admin Module"
	@echo "  docker-build-se      Собрать образ Storage Element"
	@echo "  docker-push-am       Запушить образ AM в Harbor"
	@echo "  docker-push-se       Запушить образ SE в Harbor"
	@echo ""
	@echo "Тестовая среда:"
	@echo "  copy-realm           Копировать realm.json в Helm files/"
	@echo "  helm-dep-update      Обновить Helm зависимости (Keycloak)"
	@echo "  test-env-up          Развернуть тестовую среду (PG+KC+AM+6SE)"
	@echo "  test-env-status      Показать статус pods, svc, pvc, jobs"
	@echo "  test-env-logs        Показать логи всех контейнеров"
	@echo "  test-env-down        Удалить тестовую среду"
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
	@echo "  make docker-build && make test-env-up && make port-forward-start && make test-all"

# -----------------------------------------------------------------------------
# Сборка Docker-образов
# -----------------------------------------------------------------------------

## docker-build: Собрать и запушить оба образа
docker-build: docker-build-am docker-push-am docker-build-se docker-push-se
	@echo ""
	@echo "Образы собраны и запушены:"
	@echo "  $(AM_IMAGE):$(AM_TAG)"
	@echo "  $(SE_IMAGE):$(SE_TAG)"

## docker-build-am: Собрать образ Admin Module (linux/amd64)
docker-build-am:
	@echo ">>> Сборка Admin Module $(AM_TAG) (linux/amd64)..."
	docker build \
		--platform linux/amd64 \
		--build-arg VERSION=$(AM_TAG) \
		-t $(AM_IMAGE):$(AM_TAG) \
		-f $(AM_ROOT)/Dockerfile $(AM_ROOT)

## docker-push-am: Запушить образ AM в Harbor
docker-push-am:
	@echo ">>> Push $(AM_IMAGE):$(AM_TAG)..."
	docker push $(AM_IMAGE):$(AM_TAG)

## docker-build-se: Собрать образ Storage Element (linux/amd64)
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

# -----------------------------------------------------------------------------
# Подготовка Helm Chart
# -----------------------------------------------------------------------------

## copy-realm: Копировать realm.json в Helm chart files/
copy-realm:
	@echo ">>> Копирование realm.json..."
	@mkdir -p $(dir $(REALM_DST))
	@cp $(REALM_SRC) $(REALM_DST)
	@echo "  $(REALM_SRC) -> $(REALM_DST)"

## helm-dep-update: Обновить Helm зависимости (Keycloak subchart)
helm-dep-update:
	@echo ">>> Обновление Helm зависимостей..."
	helm dependency update $(HELM_CHART)

# -----------------------------------------------------------------------------
# Тестовая среда в Kubernetes
# -----------------------------------------------------------------------------

## test-env-up: Развернуть тестовую среду через Helm
test-env-up: copy-realm helm-dep-update
	@echo ">>> Деплой тестовой среды в namespace $(NAMESPACE)..."
	helm upgrade --install $(HELM_RELEASE) $(HELM_CHART) \
		--namespace $(NAMESPACE) \
		--create-namespace \
		--set amTag=$(AM_TAG) \
		--set seTag=$(SE_TAG) \
		--wait \
		--timeout 10m
	@echo ""
	@echo ">>> Тестовая среда развёрнута. Ожидание init job..."
	@kubectl wait --for=condition=complete job/artsore-test-init \
		-n $(NAMESPACE) --timeout=600s 2>/dev/null || \
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
	@# PostgreSQL
	@kubectl port-forward -n $(NAMESPACE) svc/postgresql $(PF_PG) >/dev/null 2>&1 & \
		echo $$! > $(PF_PID_DIR)/postgresql.pid
	@# Keycloak (Bitnami: <release>-keycloak)
	@kubectl port-forward -n $(NAMESPACE) svc/$(HELM_RELEASE)-keycloak $(PF_KC) >/dev/null 2>&1 & \
		echo $$! > $(PF_PID_DIR)/keycloak.pid
	@# Admin Module
	@kubectl port-forward -n $(NAMESPACE) svc/admin-module $(PF_AM) >/dev/null 2>&1 & \
		echo $$! > $(PF_PID_DIR)/admin-module.pid
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
	@echo "    PostgreSQL:   localhost:15432 → postgresql:5432"
	@echo "    Keycloak:     localhost:18080 → keycloak:8443"
	@echo "    Admin Module: localhost:18000 → admin-module:8000"
	@echo "    se-edit-1:    localhost:18010 → se-edit-1:8010"
	@echo "    se-edit-2:    localhost:18011 → se-edit-2:8010"
	@echo "    se-rw-1:      localhost:18012 → se-rw-1:8010"
	@echo "    se-rw-2:      localhost:18013 → se-rw-2:8010"
	@echo "    se-ro:        localhost:18014 → se-ro:8010"
	@echo "    se-ar:        localhost:18015 → se-ar:8010"

# -----------------------------------------------------------------------------
# Интеграционные тесты
# -----------------------------------------------------------------------------

## test-all: Запустить все интеграционные тесты
test-all:
	@echo ">>> Интеграционные тесты пока не реализованы"
	@echo "  Реализация будет в Phase 6 Admin Module"

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
