#!/usr/bin/env bash
# ==========================================================================
# test-am-idp.sh — Тесты IdP Status (тесты 25-26)
#
# Проверяет статус подключения к Keycloak и принудительную
# синхронизацию Service Accounts.
#
# Переменные окружения (из Makefile):
#   AM_URL, KC_TOKEN_URL, KC_TEST_USER_CLIENT_ID,
#   KC_TEST_USER_CLIENT_SECRET, KC_ADMIN_USERNAME, KC_ADMIN_PASSWORD
# ==========================================================================
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/lib.sh"

: "${AM_URL:=http://localhost:18000}"
: "${KC_TOKEN_URL:=https://localhost:18080/realms/artstore/protocol/openid-connect/token}"
: "${KC_TEST_USER_CLIENT_ID:=artstore-test-user}"
: "${KC_TEST_USER_CLIENT_SECRET:=test-user-secret}"
: "${KC_ADMIN_USERNAME:=admin}"
: "${KC_ADMIN_PASSWORD:=admin}"

log_info "=== AM IdP Tests (25-26) ==="

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

# ---------- Тест 25: GET /api/v1/idp/status ----------
log_info "Тест 25: GET /api/v1/idp/status"
RESPONSE=$(http_get "$AM_URL" "$ADMIN_TOKEN" "/api/v1/idp/status")
CODE=$(get_response_code "$RESPONSE")
BODY=$(get_response_body "$RESPONSE")

if [[ "$CODE" == "200" ]]; then
    CONNECTED=$(echo "$BODY" | jq -r '.connected')
    REALM=$(echo "$BODY" | jq -r '.realm')
    USERS_COUNT=$(echo "$BODY" | jq -r '.users_count // 0')
    CLIENTS_COUNT=$(echo "$BODY" | jq -r '.clients_count // 0')
    if [[ "$CONNECTED" == "true" && "$REALM" == "artstore" ]]; then
        test_pass "Тест 25: idp/status → connected=true, realm=artstore, users=${USERS_COUNT}, clients=${CLIENTS_COUNT}"
    else
        test_fail "Тест 25: idp/status → connected=${CONNECTED}, realm=${REALM}"
    fi
else
    test_fail "Тест 25: idp/status → HTTP ${CODE} (ожидался 200)"
    if echo "$BODY" | jq . >/dev/null 2>&1; then
        log_fail "  Ответ: $(echo "$BODY" | jq -c '.')"
    fi
fi

# ---------- Тест 26: POST /api/v1/idp/sync-sa ----------
log_info "Тест 26: POST /api/v1/idp/sync-sa"
RESPONSE=$(http_post "$AM_URL" "$ADMIN_TOKEN" "/api/v1/idp/sync-sa" "")
CODE=$(get_response_code "$RESPONSE")
BODY=$(get_response_body "$RESPONSE")

if [[ "$CODE" == "200" ]]; then
    TOTAL_LOCAL=$(echo "$BODY" | jq -r '.total_local')
    TOTAL_KC=$(echo "$BODY" | jq -r '.total_keycloak')
    SYNCED_AT=$(echo "$BODY" | jq -r '.synced_at')
    if [[ -n "$SYNCED_AT" && "$SYNCED_AT" != "null" ]]; then
        test_pass "Тест 26: sync-sa → total_local=${TOTAL_LOCAL}, total_keycloak=${TOTAL_KC}, synced_at=${SYNCED_AT}"
    else
        test_fail "Тест 26: sync-sa → synced_at отсутствует"
    fi
else
    test_fail "Тест 26: sync-sa → HTTP ${CODE} (ожидался 200)"
    if echo "$BODY" | jq . >/dev/null 2>&1; then
        log_fail "  Ответ: $(echo "$BODY" | jq -c '.')"
    fi
fi

print_summary
