#!/usr/bin/env bash
# ==========================================================================
# test-am-storage-elements.sh — Тесты Storage Elements (тесты 16-20)
#
# Проверяет операции с SE: discover, register (+ full sync), list, update, sync.
# Требует работающие SE в тестовой среде.
#
# Переменные окружения (из Makefile):
#   AM_URL, KC_TOKEN_URL, KC_TEST_USER_CLIENT_ID,
#   KC_TEST_USER_CLIENT_SECRET, KC_ADMIN_USERNAME, KC_ADMIN_PASSWORD,
#   SE_RW_1_URL
# ==========================================================================
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/lib.sh"

: "${AM_URL:=http://localhost:18000}"
: "${KC_TOKEN_URL:=https://localhost:18080/realms/artsore/protocol/openid-connect/token}"
: "${KC_TEST_USER_CLIENT_ID:=artsore-test-user}"
: "${KC_TEST_USER_CLIENT_SECRET:=test-user-secret}"
: "${KC_ADMIN_USERNAME:=admin}"
: "${KC_ADMIN_PASSWORD:=admin}"
: "${SE_RW_1_URL:=https://localhost:18012}"

# SE URL для AM — AM подключается к SE внутри кластера, но discover/register
# используют URL, по которому AM может достучаться до SE.
# В тестовой среде AM и SE в одном namespace, поэтому используем cluster-internal URL.
: "${K8S_NAMESPACE:=artsore-test}"
SE_INTERNAL_URL="https://se-rw-1.${K8S_NAMESPACE}.svc.cluster.local:8010"

log_info "=== AM Storage Elements Tests (16-20) ==="

# Получаем токен admin
log_info "Получение токена admin..."
ADMIN_TOKEN=$(get_user_token "$KC_TOKEN_URL" "$KC_TEST_USER_CLIENT_ID" \
    "$KC_TEST_USER_CLIENT_SECRET" "$KC_ADMIN_USERNAME" "$KC_ADMIN_PASSWORD")
if [[ -z "$ADMIN_TOKEN" ]]; then
    log_fail "Не удалось получить токен admin"
    print_summary
    exit 1
fi
log_ok "Токен admin получен"

# ---------- Тест 16: POST /api/v1/storage-elements/discover — предпросмотр SE ----------
log_info "Тест 16: POST /api/v1/storage-elements/discover"
RESPONSE=$(http_post "$AM_URL" "$ADMIN_TOKEN" "/api/v1/storage-elements/discover" \
    "{\"url\": \"${SE_INTERNAL_URL}\"}")
CODE=$(get_response_code "$RESPONSE")
BODY=$(get_response_body "$RESPONSE")

if [[ "$CODE" == "200" ]]; then
    STORAGE_ID=$(echo "$BODY" | jq -r '.storage_id')
    MODE=$(echo "$BODY" | jq -r '.mode')
    STATUS=$(echo "$BODY" | jq -r '.status')
    if [[ -n "$STORAGE_ID" && "$STORAGE_ID" != "null" && "$STATUS" == "online" ]]; then
        test_pass "Тест 16: discover → storage_id=${STORAGE_ID}, mode=${MODE}, status=online"
    else
        test_fail "Тест 16: discover → storage_id=${STORAGE_ID}, status=${STATUS}"
    fi
else
    test_fail "Тест 16: discover → HTTP ${CODE} (ожидался 200)"
    if echo "$BODY" | jq . >/dev/null 2>&1; then
        log_fail "  Ответ: $(echo "$BODY" | jq -c '.')"
    fi
fi

# ---------- Тест 17: POST /api/v1/storage-elements — регистрация SE ----------
log_info "Тест 17: POST /api/v1/storage-elements (регистрация)"
RESPONSE=$(http_post "$AM_URL" "$ADMIN_TOKEN" "/api/v1/storage-elements" \
    "{\"name\": \"Test SE RW-1\", \"url\": \"${SE_INTERNAL_URL}\"}")
CODE=$(get_response_code "$RESPONSE")
BODY=$(get_response_body "$RESPONSE")

if [[ "$CODE" == "201" ]]; then
    SE_ID=$(echo "$BODY" | jq -r '.id')
    SE_NAME=$(echo "$BODY" | jq -r '.name')
    SE_MODE=$(echo "$BODY" | jq -r '.mode')
    SE_STATUS=$(echo "$BODY" | jq -r '.status')
    if [[ -n "$SE_ID" && "$SE_ID" != "null" && "$SE_NAME" == "Test SE RW-1" ]]; then
        test_pass "Тест 17: SE зарегистрирован, id=${SE_ID}, mode=${SE_MODE}, status=${SE_STATUS}"
    else
        test_fail "Тест 17: SE зарегистрирован, но данные некорректны: id=${SE_ID}, name=${SE_NAME}"
    fi
elif [[ "$CODE" == "409" ]]; then
    # SE уже зарегистрирован — ищем его в списке
    log_warn "SE уже зарегистрирован (409), ищем в списке..."
    RESPONSE2=$(http_get "$AM_URL" "$ADMIN_TOKEN" "/api/v1/storage-elements")
    BODY2=$(get_response_body "$RESPONSE2")
    SE_ID=$(echo "$BODY2" | jq -r --arg url "$SE_INTERNAL_URL" '.items[] | select(.url == $url) | .id')
    if [[ -n "$SE_ID" && "$SE_ID" != "null" ]]; then
        test_pass "Тест 17: SE уже зарегистрирован (409), id=${SE_ID}"
    else
        test_fail "Тест 17: SE 409 конфликт, но не найден в списке"
    fi
else
    test_fail "Тест 17: register SE → HTTP ${CODE} (ожидался 201 или 409)"
    if echo "$BODY" | jq . >/dev/null 2>&1; then
        log_fail "  Ответ: $(echo "$BODY" | jq -c '.')"
    fi
fi

# ---------- Тест 18: GET /api/v1/storage-elements — список SE ----------
log_info "Тест 18: GET /api/v1/storage-elements (список)"
RESPONSE=$(http_get "$AM_URL" "$ADMIN_TOKEN" "/api/v1/storage-elements")
CODE=$(get_response_code "$RESPONSE")
BODY=$(get_response_body "$RESPONSE")

if [[ "$CODE" == "200" ]]; then
    TOTAL=$(echo "$BODY" | jq -r '.total')
    if [[ "$TOTAL" -ge 1 ]]; then
        test_pass "Тест 18: SE list → total=${TOTAL}"
    else
        test_fail "Тест 18: SE list → total=${TOTAL} (ожидался >= 1)"
    fi
else
    test_fail "Тест 18: SE list → HTTP ${CODE} (ожидался 200)"
fi

# ---------- Тест 19: PUT /api/v1/storage-elements/{id} — обновление SE ----------
log_info "Тест 19: PUT /api/v1/storage-elements/{id} (обновление имени)"
RESPONSE=$(http_put "$AM_URL" "$ADMIN_TOKEN" "/api/v1/storage-elements/${SE_ID}" \
    '{"name": "Test SE RW-1 Updated"}')
CODE=$(get_response_code "$RESPONSE")
BODY=$(get_response_body "$RESPONSE")

if [[ "$CODE" == "200" ]]; then
    NEW_NAME=$(echo "$BODY" | jq -r '.name')
    if [[ "$NEW_NAME" == "Test SE RW-1 Updated" ]]; then
        test_pass "Тест 19: SE name обновлён → ${NEW_NAME}"
    else
        test_fail "Тест 19: SE name → ${NEW_NAME} (ожидался 'Test SE RW-1 Updated')"
    fi
else
    test_fail "Тест 19: SE update → HTTP ${CODE} (ожидался 200)"
fi

# ---------- Тест 20: POST /api/v1/storage-elements/{id}/sync — ручная синхронизация ----------
log_info "Тест 20: POST /api/v1/storage-elements/{id}/sync"
RESPONSE=$(http_post "$AM_URL" "$ADMIN_TOKEN" "/api/v1/storage-elements/${SE_ID}/sync" "")
CODE=$(get_response_code "$RESPONSE")
BODY=$(get_response_body "$RESPONSE")

if [[ "$CODE" == "200" ]]; then
    HAS_SE=$(echo "$BODY" | jq 'has("storage_element")')
    HAS_SYNC=$(echo "$BODY" | jq 'has("file_sync")')
    if [[ "$HAS_SE" == "true" && "$HAS_SYNC" == "true" ]]; then
        FILES_ON_SE=$(echo "$BODY" | jq -r '.file_sync.files_on_se')
        test_pass "Тест 20: sync → storage_element и file_sync присутствуют, files_on_se=${FILES_ON_SE}"
    else
        test_fail "Тест 20: sync → has_se=${HAS_SE}, has_sync=${HAS_SYNC}"
    fi
else
    test_fail "Тест 20: sync → HTTP ${CODE} (ожидался 200)"
    if echo "$BODY" | jq . >/dev/null 2>&1; then
        log_fail "  Ответ: $(echo "$BODY" | jq -c '.')"
    fi
fi

print_summary
