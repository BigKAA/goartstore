#!/usr/bin/env bash
# ==========================================================================
# test-se-parallel.sh — Тесты параллельной записи SE (stateless)
#
# Тесты 1-5: Upload через разные pod-ы, index sync, mode sync
#
# se-edit-1 имеет 2 реплики — Service балансирует запросы.
# Для прямого доступа к конкретному pod-у используем kubectl port-forward.
#
# Переменные окружения (из Makefile):
#   SE_EDIT_1_URL, KC_TOKEN_URL
#   KC_TEST_USER_CLIENT_ID, KC_TEST_USER_CLIENT_SECRET
#   KC_ADMIN_USERNAME, KC_ADMIN_PASSWORD
#   K8S_NAMESPACE
# ==========================================================================

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/lib.sh"

# Проверяем обязательные переменные
: "${SE_EDIT_1_URL:?SE_EDIT_1_URL не задана}"
: "${KC_TOKEN_URL:?KC_TOKEN_URL не задана}"
: "${K8S_NAMESPACE:?K8S_NAMESPACE не задана}"

# ==========================================================================
# Получение токена
# ==========================================================================
log_info "Получение admin-токена из Keycloak..."

ADMIN_TOKEN=$(get_user_token_with_scopes "$KC_TOKEN_URL" \
    "$KC_TEST_USER_CLIENT_ID" "$KC_TEST_USER_CLIENT_SECRET" \
    "$KC_ADMIN_USERNAME" "$KC_ADMIN_PASSWORD" \
    "openid files:read files:write storage:read storage:write")
if [[ -z "$ADMIN_TOKEN" ]]; then
    log_fail "Не удалось получить admin-токен"
    exit 1
fi
log_ok "Токен получен"

# ==========================================================================
# Подготовка: port-forward к каждому pod-у se-edit-1
# ==========================================================================
log_info "Настройка port-forward к pod-ам se-edit-1..."

PODS=($(kubectl get pods -n "$K8S_NAMESPACE" -l app.kubernetes.io/instance=se-edit-1 \
    -o jsonpath='{.items[*].metadata.name}' 2>/dev/null))

if [[ ${#PODS[@]} -lt 2 ]]; then
    log_fail "se-edit-1 имеет ${#PODS[@]} pod-ов (ожидалось 2). Пропуск тестов параллельной записи."
    test_fail "se-edit-1 должен иметь 2 реплики"
    print_summary
    exit 0
fi

POD1="${PODS[0]}"
POD2="${PODS[1]}"
PORT1=19010
PORT2=19011

log_info "  Pod 1: ${POD1} → localhost:${PORT1}"
log_info "  Pod 2: ${POD2} → localhost:${PORT2}"

# Запускаем port-forward к каждому pod-у
kubectl port-forward -n "$K8S_NAMESPACE" "pod/${POD1}" "${PORT1}:8010" >/dev/null 2>&1 &
PF_PID1=$!
kubectl port-forward -n "$K8S_NAMESPACE" "pod/${POD2}" "${PORT2}:8010" >/dev/null 2>&1 &
PF_PID2=$!

# Cleanup при выходе
cleanup() {
    kill "$PF_PID1" 2>/dev/null || true
    kill "$PF_PID2" 2>/dev/null || true
}
trap cleanup EXIT

sleep 3

POD1_URL="https://localhost:${PORT1}"
POD2_URL="https://localhost:${PORT2}"

# Проверяем доступность
for pod_url in "$POD1_URL" "$POD2_URL"; do
    code=$(curl $CURL_OPTS -o /dev/null -w "%{http_code}" "${pod_url}/health/live" 2>/dev/null) || code="000"
    if [[ "$code" != "200" ]]; then
        log_fail "Pod ${pod_url} недоступен (HTTP ${code})"
        test_fail "Подготовка: pod ${pod_url} недоступен"
        print_summary
        exit 0
    fi
done
log_ok "Оба pod-а доступны"

# ==========================================================================
# Тест 1: Upload через pod-1, файл виден через pod-1 сразу
# ==========================================================================
echo ""
log_info "=== Тест 1: Upload через pod-1 → виден на pod-1 ==="

response=$(upload_file "$POD1_URL" "$ADMIN_TOKEN" \
    "parallel-test-1.txt" "text/plain" "Файл загружен через pod-1")
code=$(get_response_code "$response")
body=$(get_response_body "$response")
FILE_ID_1=$(echo "$body" | jq -r '.file_id // empty')

if [[ "$code" == "201" && -n "$FILE_ID_1" ]]; then
    # Проверяем доступность на pod-1 сразу
    meta_response=$(http_get "$POD1_URL" "$ADMIN_TOKEN" "/api/v1/files/${FILE_ID_1}")
    meta_code=$(get_response_code "$meta_response")
    if [[ "$meta_code" == "200" ]]; then
        test_pass "Upload pod-1 + metadata pod-1: OK (file_id=${FILE_ID_1})"
    else
        test_fail "Upload pod-1: 201, но metadata pod-1 → ${meta_code}"
    fi
else
    test_fail "Upload pod-1: ожидался 201, получен ${code}"
fi

# ==========================================================================
# Тест 2: Файл от pod-1 виден через pod-2 после index sync
# ==========================================================================
log_info "=== Тест 2: Файл от pod-1 виден через pod-2 (index sync) ==="

if [[ -n "${FILE_ID_1:-}" ]]; then
    # indexSyncInterval = 15s, ждём до 30s
    max_wait=30
    interval=5
    elapsed=0
    found=false

    while [[ $elapsed -lt $max_wait ]]; do
        meta_response=$(http_get "$POD2_URL" "$ADMIN_TOKEN" "/api/v1/files/${FILE_ID_1}")
        meta_code=$(get_response_code "$meta_response")
        if [[ "$meta_code" == "200" ]]; then
            found=true
            break
        fi
        sleep $interval
        elapsed=$((elapsed + interval))
    done

    if $found; then
        test_pass "Index sync: файл от pod-1 виден через pod-2 за ${elapsed}s"
    else
        test_fail "Index sync: файл от pod-1 НЕ виден через pod-2 за ${max_wait}s"
    fi
else
    test_fail "Index sync: пропущен (upload не удался)"
fi

# ==========================================================================
# Тест 3: Upload через pod-2, файл виден через pod-1 после sync
# ==========================================================================
echo ""
log_info "=== Тест 3: Upload через pod-2 → виден на pod-1 (index sync) ==="

response=$(upload_file "$POD2_URL" "$ADMIN_TOKEN" \
    "parallel-test-2.txt" "text/plain" "Файл загружен через pod-2")
code=$(get_response_code "$response")
body=$(get_response_body "$response")
FILE_ID_2=$(echo "$body" | jq -r '.file_id // empty')

if [[ "$code" == "201" && -n "$FILE_ID_2" ]]; then
    max_wait=30
    interval=5
    elapsed=0
    found=false

    while [[ $elapsed -lt $max_wait ]]; do
        meta_response=$(http_get "$POD1_URL" "$ADMIN_TOKEN" "/api/v1/files/${FILE_ID_2}")
        meta_code=$(get_response_code "$meta_response")
        if [[ "$meta_code" == "200" ]]; then
            found=true
            break
        fi
        sleep $interval
        elapsed=$((elapsed + interval))
    done

    if $found; then
        test_pass "Upload pod-2 → pod-1 sync: OK за ${elapsed}s (file_id=${FILE_ID_2})"
    else
        test_fail "Upload pod-2 → pod-1: НЕ виден за ${max_wait}s"
    fi
else
    test_fail "Upload pod-2: ожидался 201, получен ${code}"
fi

# ==========================================================================
# Тест 4: Одновременный upload через оба pod-а (разные файлы)
# ==========================================================================
echo ""
log_info "=== Тест 4: Одновременный upload через оба pod-а ==="

# Создаём временные файлы
TMP1=$(mktemp)
TMP2=$(mktemp)
dd if=/dev/urandom bs=1024 count=2 2>/dev/null > "$TMP1"
dd if=/dev/urandom bs=1024 count=2 2>/dev/null > "$TMP2"

# Загружаем одновременно
RESP_FILE1=$(mktemp)
RESP_FILE2=$(mktemp)

(upload_file "$POD1_URL" "$ADMIN_TOKEN" "concurrent-a.bin" "application/octet-stream" "" "$TMP1" > "$RESP_FILE1") &
UPLOAD_PID1=$!
(upload_file "$POD2_URL" "$ADMIN_TOKEN" "concurrent-b.bin" "application/octet-stream" "" "$TMP2" > "$RESP_FILE2") &
UPLOAD_PID2=$!

wait "$UPLOAD_PID1" || true
wait "$UPLOAD_PID2" || true

RESP1=$(cat "$RESP_FILE1")
RESP2=$(cat "$RESP_FILE2")
rm -f "$TMP1" "$TMP2" "$RESP_FILE1" "$RESP_FILE2"

CODE1=$(get_response_code "$RESP1")
CODE2=$(get_response_code "$RESP2")
FID_A=$(get_response_body "$RESP1" | jq -r '.file_id // empty')
FID_B=$(get_response_body "$RESP2" | jq -r '.file_id // empty')

if [[ "$CODE1" == "201" && "$CODE2" == "201" && -n "$FID_A" && -n "$FID_B" ]]; then
    test_pass "Concurrent upload: оба 201 (file_a=${FID_A}, file_b=${FID_B})"
else
    test_fail "Concurrent upload: pod-1=${CODE1}, pod-2=${CODE2}"
fi

# ==========================================================================
# Тест 5: Delete через pod-1, проверка через pod-2
# ==========================================================================
echo ""
log_info "=== Тест 5: Delete через pod-1 → отсутствие на pod-2 ==="

if [[ -n "${FILE_ID_1:-}" ]]; then
    # Удаляем через pod-1
    del_response=$(http_delete "$POD1_URL" "$ADMIN_TOKEN" "/api/v1/files/${FILE_ID_1}")
    del_code=$(get_response_code "$del_response")

    if [[ "$del_code" == "200" || "$del_code" == "204" ]]; then
        # Ждём index sync на pod-2
        # SE делает hard delete — файл физически удалён, после index rebuild → 404
        max_wait=45
        interval=5
        elapsed=0
        gone=false

        while [[ $elapsed -lt $max_wait ]]; do
            meta_response=$(http_get "$POD2_URL" "$ADMIN_TOKEN" "/api/v1/files/${FILE_ID_1}")
            meta_code=$(get_response_code "$meta_response")
            # 404 = файл физически удалён
            if [[ "$meta_code" == "404" ]]; then
                gone=true
                break
            fi
            sleep $interval
            elapsed=$((elapsed + interval))
        done

        if $gone; then
            test_pass "Delete sync: файл физически удалён на pod-2 за ${elapsed}s"
        else
            test_fail "Delete sync: файл всё ещё виден на pod-2 через ${max_wait}s"
        fi
    else
        test_fail "Delete pod-1: ожидался 200/204, получен ${del_code}"
    fi
else
    test_fail "Delete sync: пропущен (upload не удался)"
fi

# ==========================================================================
# Очистка тестовых файлов
# ==========================================================================
log_info "Очистка тестовых файлов..."
for fid in "${FILE_ID_2:-}" "${FID_A:-}" "${FID_B:-}"; do
    if [[ -n "$fid" ]]; then
        http_delete "$POD1_URL" "$ADMIN_TOKEN" "/api/v1/files/${fid}" >/dev/null 2>&1 || true
    fi
done

# ==========================================================================
# Итого
# ==========================================================================
print_summary
