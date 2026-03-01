#!/usr/bin/env bash
# ==========================================================================
# test-se-locks.sh — Интеграционные тесты Lock API Storage Element
#
# Тесты 1-6: Lock API (GET /locks, POST /locks/cleanup),
#             DELETE при upload (409 Conflict)
#
# Переменные окружения (из Makefile):
#   SE_EDIT_1_URL, KC_TOKEN_URL
#   KC_TEST_USER_CLIENT_ID, KC_TEST_USER_CLIENT_SECRET
#   KC_ADMIN_USERNAME, KC_ADMIN_PASSWORD
#   KC_VIEWER_USERNAME, KC_VIEWER_PASSWORD
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
# Получение токенов
# ==========================================================================
log_info "Получение токенов из Keycloak..."

# Admin с полным набором scopes
ADMIN_TOKEN=$(get_user_token_with_scopes "$KC_TOKEN_URL" \
    "$KC_TEST_USER_CLIENT_ID" "$KC_TEST_USER_CLIENT_SECRET" \
    "$KC_ADMIN_USERNAME" "$KC_ADMIN_PASSWORD" \
    "openid files:read files:write storage:read storage:write")
if [[ -z "$ADMIN_TOKEN" ]]; then
    log_fail "Не удалось получить admin-токен"
    exit 1
fi

# Viewer — без storage:write (для проверки 403 на locks)
VIEWER_TOKEN=$(get_user_token_with_scopes "$KC_TOKEN_URL" \
    "$KC_TEST_USER_CLIENT_ID" "$KC_TEST_USER_CLIENT_SECRET" \
    "$KC_VIEWER_USERNAME" "$KC_VIEWER_PASSWORD" \
    "openid files:read storage:read")
if [[ -z "$VIEWER_TOKEN" ]]; then
    log_fail "Не удалось получить viewer-токен"
    exit 1
fi

log_ok "Токены получены"

# Предварительная очистка: удаляем все lock-и (force) для чистого старта
log_info "Предварительная очистка lock-ов..."
http_post "$SE_EDIT_1_URL" "$ADMIN_TOKEN" "/api/v1/locks/cleanup?force=true" "" >/dev/null 2>&1 || true

# ==========================================================================
# Тест 1: GET /api/v1/locks — пустой список (нет активных lock-ов)
# ==========================================================================
echo ""
log_info "=== Тест 1: GET /api/v1/locks — пустой список ==="

response=$(http_get "$SE_EDIT_1_URL" "$ADMIN_TOKEN" "/api/v1/locks")
code=$(get_response_code "$response")
body=$(get_response_body "$response")

if [[ "$code" == "200" ]]; then
    total=$(echo "$body" | jq -r '.total // -1')
    active=$(echo "$body" | jq -r '.active // -1')
    if [[ "$total" == "0" && "$active" == "0" ]]; then
        test_pass "GET /locks: 200, total=0, active=0"
    else
        test_fail "GET /locks: 200, но total=${total}, active=${active} (ожидались 0)"
    fi
else
    test_fail "GET /locks: ожидался 200, получен ${code}"
fi

# ==========================================================================
# Тест 2: GET /api/v1/locks — viewer → 403 (нужен scope storage:write)
# ==========================================================================
log_info "=== Тест 2: GET /api/v1/locks — viewer → 403 ==="

response=$(http_get "$SE_EDIT_1_URL" "$VIEWER_TOKEN" "/api/v1/locks")
code=$(get_response_code "$response")

if [[ "$code" == "403" ]]; then
    test_pass "GET /locks (viewer): 403 Forbidden"
else
    test_fail "GET /locks (viewer): ожидался 403, получен ${code}"
fi

# ==========================================================================
# Тест 3: POST /api/v1/locks/cleanup — нет expired lock-ов → cleaned=0
# ==========================================================================
log_info "=== Тест 3: POST /api/v1/locks/cleanup — нет expired ==="

response=$(http_post "$SE_EDIT_1_URL" "$ADMIN_TOKEN" "/api/v1/locks/cleanup" "")
code=$(get_response_code "$response")
body=$(get_response_body "$response")

if [[ "$code" == "200" ]]; then
    cleaned=$(echo "$body" | jq -r '.cleaned // -1')
    if [[ "$cleaned" == "0" ]]; then
        test_pass "POST /locks/cleanup: 200, cleaned=0"
    else
        test_fail "POST /locks/cleanup: 200, но cleaned=${cleaned} (ожидался 0)"
    fi
else
    test_fail "POST /locks/cleanup: ожидался 200, получен ${code}"
fi

# ==========================================================================
# Тест 4: Создание lock-файла вручную и проверка GET /locks
# Создаём lock-файл через kubectl exec для имитации upload-in-progress
# ==========================================================================
echo ""
log_info "=== Тест 4: Создание lock-файла → GET /locks ==="

# Получим имя одного из pod-ов se-edit-1
SE_POD=$(kubectl get pods -n "$K8S_NAMESPACE" -l app.kubernetes.io/instance=se-edit-1 \
    -o jsonpath='{.items[0].metadata.name}' 2>/dev/null)

if [[ -z "$SE_POD" ]]; then
    test_fail "Lock-файл: не удалось найти pod se-edit-1"
else
    # Создаём lock-файл с TTL 120s
    FAKE_FILE_ID="test-lock-$(date +%s)"
    LOCK_JSON=$(jq -n \
        --arg holder "$SE_POD" \
        --arg file_id "$FAKE_FILE_ID" \
        --arg acquired_at "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
        --argjson ttl_seconds 120 \
        '{holder: $holder, file_id: $file_id, acquired_at: $acquired_at, ttl_seconds: $ttl_seconds}')

    # Создаём директорию .locks/ и записываем lock-файл
    kubectl exec -n "$K8S_NAMESPACE" "$SE_POD" -- \
        sh -c "mkdir -p /data/.locks && echo '${LOCK_JSON}' > /data/.locks/${FAKE_FILE_ID}.lock" 2>/dev/null

    # Проверяем GET /locks
    response=$(http_get "$SE_EDIT_1_URL" "$ADMIN_TOKEN" "/api/v1/locks")
    code=$(get_response_code "$response")
    body=$(get_response_body "$response")

    if [[ "$code" == "200" ]]; then
        active=$(echo "$body" | jq -r '.active // 0')
        found=$(echo "$body" | jq -r --arg fid "$FAKE_FILE_ID" \
            '[.locks[] | select(.file_id == $fid)] | length')
        if [[ "$active" -ge 1 && "$found" == "1" ]]; then
            test_pass "GET /locks: active=${active}, найден lock для ${FAKE_FILE_ID}"
        else
            test_fail "GET /locks: active=${active}, found=${found} (ожидался ≥1, found=1)"
        fi
    else
        test_fail "GET /locks: ожидался 200, получен ${code}"
    fi
fi

# ==========================================================================
# Тест 5: POST /api/v1/locks/cleanup?force=true — принудительная очистка
# ==========================================================================
log_info "=== Тест 5: POST /api/v1/locks/cleanup?force=true ==="

response=$(http_post "$SE_EDIT_1_URL" "$ADMIN_TOKEN" "/api/v1/locks/cleanup?force=true" "")
code=$(get_response_code "$response")
body=$(get_response_body "$response")

if [[ "$code" == "200" ]]; then
    cleaned=$(echo "$body" | jq -r '.cleaned // -1')
    if [[ "$cleaned" -ge 1 ]]; then
        test_pass "POST /locks/cleanup?force=true: 200, cleaned=${cleaned}"
    else
        test_fail "POST /locks/cleanup?force=true: 200, но cleaned=${cleaned} (ожидался ≥1)"
    fi
else
    test_fail "POST /locks/cleanup?force=true: ожидался 200, получен ${code}"
fi

# Проверяем что lock-и очищены
response=$(http_get "$SE_EDIT_1_URL" "$ADMIN_TOKEN" "/api/v1/locks")
body=$(get_response_body "$response")
remaining=$(echo "$body" | jq -r '.active // -1')
if [[ "$remaining" == "0" ]]; then
    log_ok "  Подтверждено: active=0 после force cleanup"
else
    log_warn "  После force cleanup: active=${remaining}"
fi

# ==========================================================================
# Тест 6: DELETE при активном lock → 409 Conflict
# ==========================================================================
echo ""
log_info "=== Тест 6: DELETE при активном lock → 409 Conflict ==="

# Загружаем файл, чтобы получить реальный file_id
UPLOAD_RESPONSE=$(upload_file "$SE_EDIT_1_URL" "$ADMIN_TOKEN" \
    "test-lock-delete.txt" "text/plain" "Файл для теста lock+delete")
UPLOAD_CODE=$(get_response_code "$UPLOAD_RESPONSE")
UPLOAD_BODY=$(get_response_body "$UPLOAD_RESPONSE")
LOCK_FILE_ID=$(echo "$UPLOAD_BODY" | jq -r '.file_id // empty')

if [[ "$UPLOAD_CODE" == "201" && -n "$LOCK_FILE_ID" ]]; then
    # Создаём lock-файл для этого file_id (имитация повторного upload)
    LOCK_JSON=$(jq -n \
        --arg holder "$SE_POD" \
        --arg file_id "$LOCK_FILE_ID" \
        --arg acquired_at "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
        --argjson ttl_seconds 120 \
        '{holder: $holder, file_id: $file_id, acquired_at: $acquired_at, ttl_seconds: $ttl_seconds}')

    kubectl exec -n "$K8S_NAMESPACE" "$SE_POD" -- \
        sh -c "mkdir -p /data/.locks && echo '${LOCK_JSON}' > /data/.locks/${LOCK_FILE_ID}.lock" 2>/dev/null

    # Попытка DELETE при активном lock → ожидаем 409
    response=$(http_delete "$SE_EDIT_1_URL" "$ADMIN_TOKEN" "/api/v1/files/${LOCK_FILE_ID}")
    code=$(get_response_code "$response")
    body=$(get_response_body "$response")

    if [[ "$code" == "409" ]]; then
        error_code=$(echo "$body" | jq -r '.error.code // empty')
        if [[ "$error_code" == "FILE_UPLOAD_IN_PROGRESS" ]]; then
            test_pass "DELETE при lock: 409 Conflict, code=FILE_UPLOAD_IN_PROGRESS"
        else
            test_pass "DELETE при lock: 409 Conflict (code=${error_code})"
        fi
    else
        test_fail "DELETE при lock: ожидался 409, получен ${code}"
    fi

    # Очистка: удаляем lock и файл
    kubectl exec -n "$K8S_NAMESPACE" "$SE_POD" -- \
        rm -f "/data/.locks/${LOCK_FILE_ID}.lock" 2>/dev/null || true
    http_delete "$SE_EDIT_1_URL" "$ADMIN_TOKEN" "/api/v1/files/${LOCK_FILE_ID}" >/dev/null 2>&1 || true
else
    test_fail "DELETE при lock: upload не удался (${UPLOAD_CODE})"
fi

# ==========================================================================
# Итого
# ==========================================================================
print_summary
