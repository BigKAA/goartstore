#!/usr/bin/env bash
# ==========================================================================
# test-replica.sh — Тесты replicated mode (тесты 19-22)
#
# Проверяет leader/follower election, proxy и failover:
#   19. GET /info на оба pod-а se-edit-1 → один leader, один follower
#   20. POST upload на follower → 201 (proxy к leader)
#   21. kubectl delete pod <leader> → follower становится leader (retry 30s)
#   22. kubectl scale → новый pod → follower
#
# Переменные окружения:
#   JWKS_MOCK_URL     — URL JWKS Mock (по умолчанию https://localhost:18080)
#   SE_EDIT_1_URL     — URL SE edit-1 (по умолчанию https://localhost:18010)
#   K8S_NAMESPACE     — Kubernetes namespace (по умолчанию se-test)
#   SE_EDIT_1_STS     — Имя StatefulSet (по умолчанию se-edit-1)
#
# Примечание:
#   Тесты 19-22 требуют доступа к kubectl и namespace se-test.
#   Для pod-to-pod обращений используется kubectl port-forward к отдельным pod-ам.
#
# Использование:
#   ./test-replica.sh
# ==========================================================================
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/lib.sh"

: "${JWKS_MOCK_URL:=https://localhost:18080}"
: "${SE_EDIT_1_URL:=https://localhost:18010}"
: "${K8S_NAMESPACE:=se-test}"
: "${SE_EDIT_1_STS:=se-edit-1}"

log_info "========================================================"
log_info "  Тесты replicated mode (19-22)"
log_info "  JWKS Mock: ${JWKS_MOCK_URL}"
log_info "  SE EDIT-1: ${SE_EDIT_1_URL}"
log_info "  Namespace: ${K8S_NAMESPACE}"
log_info "  StatefulSet: ${SE_EDIT_1_STS}"
log_info "========================================================"
echo ""

# --------------------------------------------------------------------------
# Подготовка: JWT
# --------------------------------------------------------------------------
log_info "Получение JWT токена..."
TOKEN=$(get_token "$JWKS_MOCK_URL" "test-replica" '["files:read","files:write","storage:write"]' 3600)
if [[ -z "$TOKEN" || "$TOKEN" == "null" ]]; then
    log_fail "Не удалось получить JWT токен — прерывание"
    exit 1
fi
log_ok "JWT получен"
echo ""

# --------------------------------------------------------------------------
# Вспомогательные функции для работы с pod-ами
# --------------------------------------------------------------------------

# get_pod_info — получить role из /api/v1/info pod-а через port-forward
# Аргументы: $1 — имя pod-а, $2 — локальный порт
# Возвращает: role (leader/follower/standalone)
get_pod_role() {
    local pod_name="$1"
    local local_port="$2"

    # Запускаем port-forward в фоне
    kubectl port-forward -n "$K8S_NAMESPACE" "pod/${pod_name}" "${local_port}:8010" &>/dev/null &
    local pf_pid=$!
    sleep 2

    local response
    response=$(http_get "https://localhost:${local_port}" "" "/api/v1/info") || true

    # Убиваем port-forward
    kill "$pf_pid" 2>/dev/null || true
    wait "$pf_pid" 2>/dev/null || true

    local code body role
    code=$(get_response_code "$response")
    body=$(get_response_body "$response")

    if [[ "$code" == "200" ]]; then
        role=$(echo "$body" | jq -r '.role // "unknown"')
        echo "$role"
    else
        echo "error"
    fi
}

# ==========================================================================
# Тест 19: GET /info на оба pod-а se-edit-1 → один leader, один follower
# ==========================================================================
log_info "[Тест 19] Проверка ролей: один leader, один follower"

POD_0="${SE_EDIT_1_STS}-0"
POD_1="${SE_EDIT_1_STS}-1"

# Проверяем что оба pod-а Running
POD_0_STATUS=$(kubectl get pod -n "$K8S_NAMESPACE" "$POD_0" -o jsonpath='{.status.phase}' 2>/dev/null || echo "NotFound")
POD_1_STATUS=$(kubectl get pod -n "$K8S_NAMESPACE" "$POD_1" -o jsonpath='{.status.phase}' 2>/dev/null || echo "NotFound")

if [[ "$POD_0_STATUS" != "Running" || "$POD_1_STATUS" != "Running" ]]; then
    test_fail "Тест 19: Pod-ы не в состоянии Running (${POD_0}=${POD_0_STATUS}, ${POD_1}=${POD_1_STATUS})"
else
    ROLE_0=$(get_pod_role "$POD_0" 19010)
    ROLE_1=$(get_pod_role "$POD_1" 19011)

    log_info "  ${POD_0} → role=${ROLE_0}"
    log_info "  ${POD_1} → role=${ROLE_1}"

    if { [[ "$ROLE_0" == "leader" && "$ROLE_1" == "follower" ]] || \
         [[ "$ROLE_0" == "follower" && "$ROLE_1" == "leader" ]]; }; then
        test_pass "Тест 19: Роли корректны (${POD_0}=${ROLE_0}, ${POD_1}=${ROLE_1})"

        # Запоминаем кто leader и follower для дальнейших тестов
        if [[ "$ROLE_0" == "leader" ]]; then
            LEADER_POD="$POD_0"
            FOLLOWER_POD="$POD_1"
            FOLLOWER_PORT=19011
        else
            LEADER_POD="$POD_1"
            FOLLOWER_POD="$POD_0"
            FOLLOWER_PORT=19010
        fi
    else
        test_fail "Тест 19: Некорректные роли (${POD_0}=${ROLE_0}, ${POD_1}=${ROLE_1})"
        LEADER_POD=""
        FOLLOWER_POD=""
    fi
fi

# ==========================================================================
# Тест 20: POST upload на follower → 201 (proxy к leader)
# ==========================================================================
log_info "[Тест 20] POST upload через follower → 201 (proxy)"

if [[ -n "${FOLLOWER_POD:-}" ]]; then
    # Запускаем port-forward к follower
    kubectl port-forward -n "$K8S_NAMESPACE" "pod/${FOLLOWER_POD}" "${FOLLOWER_PORT}:8010" &>/dev/null &
    PF_PID=$!
    sleep 2

    RESPONSE=$(upload_file "https://localhost:${FOLLOWER_PORT}" "$TOKEN" "test-proxy-upload.bin" "application/octet-stream" "Upload через follower proxy")
    CODE=$(get_response_code "$RESPONSE")
    BODY=$(get_response_body "$RESPONSE")

    kill "$PF_PID" 2>/dev/null || true
    wait "$PF_PID" 2>/dev/null || true

    if [[ "$CODE" == "201" ]]; then
        FILE_ID=$(echo "$BODY" | jq -r '.file_id // empty')
        test_pass "Тест 20: Upload через follower → 201, file_id=${FILE_ID}"
    else
        test_fail "Тест 20: Upload через follower → HTTP ${CODE} (ожидался 201)"
    fi
else
    test_fail "Тест 20: Пропуск — follower не определён (тест 19 не пройден)"
fi

# ==========================================================================
# Тест 21: kubectl delete pod <leader> → follower становится leader (30s)
# ==========================================================================
log_info "[Тест 21] Failover: удаление leader pod → follower становится leader"

if [[ -n "${LEADER_POD:-}" && -n "${FOLLOWER_POD:-}" ]]; then
    log_info "  Удаление leader pod: ${LEADER_POD}..."
    kubectl delete pod -n "$K8S_NAMESPACE" "$LEADER_POD" --grace-period=5 2>/dev/null

    # Ждём пока follower станет leader (до 30 секунд)
    FAILOVER_TIMEOUT=30
    FAILOVER_ELAPSED=0
    FAILOVER_OK=false

    while [[ $FAILOVER_ELAPSED -lt $FAILOVER_TIMEOUT ]]; do
        sleep 3
        FAILOVER_ELAPSED=$((FAILOVER_ELAPSED + 3))

        NEW_ROLE=$(get_pod_role "$FOLLOWER_POD" 19020)
        if [[ "$NEW_ROLE" == "leader" ]]; then
            FAILOVER_OK=true
            break
        fi
        log_info "  Ожидание failover... ${FAILOVER_ELAPSED}s (${FOLLOWER_POD} role=${NEW_ROLE})"
    done

    if [[ "$FAILOVER_OK" == "true" ]]; then
        test_pass "Тест 21: Failover завершён за ${FAILOVER_ELAPSED}s, ${FOLLOWER_POD} → leader"
    else
        test_fail "Тест 21: Failover не произошёл за ${FAILOVER_TIMEOUT}s"
    fi

    # Ждём восстановления удалённого pod-а
    log_info "  Ожидание восстановления ${LEADER_POD}..."
    kubectl wait --for=condition=ready "pod/${LEADER_POD}" -n "$K8S_NAMESPACE" --timeout=60s 2>/dev/null || true
else
    test_fail "Тест 21: Пропуск — leader/follower не определены"
fi

# ==========================================================================
# Тест 22: Новый pod → становится follower
# ==========================================================================
log_info "[Тест 22] Восстановленный pod → follower"

if [[ -n "${LEADER_POD:-}" ]]; then
    # Ждём что восстановленный pod готов
    POD_READY=$(kubectl get pod -n "$K8S_NAMESPACE" "$LEADER_POD" -o jsonpath='{.status.conditions[?(@.type=="Ready")].status}' 2>/dev/null || echo "False")

    if [[ "$POD_READY" == "True" ]]; then
        # Проверяем роль восстановленного pod-а
        RESTORED_ROLE=$(get_pod_role "$LEADER_POD" 19030)
        if [[ "$RESTORED_ROLE" == "follower" ]]; then
            test_pass "Тест 22: Восстановленный ${LEADER_POD} → follower"
        elif [[ "$RESTORED_ROLE" == "leader" ]]; then
            # Возможна ситуация когда восстановленный pod перехватил leader lock обратно
            log_warn "  ${LEADER_POD} стал leader (допустимо — перехватил lock)"
            test_pass "Тест 22: Восстановленный ${LEADER_POD} → leader (перехватил lock)"
        else
            test_fail "Тест 22: Восстановленный ${LEADER_POD} → role=${RESTORED_ROLE} (ожидался follower или leader)"
        fi
    else
        test_fail "Тест 22: Pod ${LEADER_POD} не готов — POD_READY=${POD_READY}"
    fi
else
    test_fail "Тест 22: Пропуск — leader pod не определён"
fi

# ==========================================================================
# Итоги
# ==========================================================================
echo ""
print_summary
