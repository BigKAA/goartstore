#!/usr/bin/env bash
# ==========================================================================
# test-am-admin-auth.sh — Тесты Admin Auth (тест 4)
#
# Проверяет GET /api/v1/admin-auth/me для пользователя admin.
#
# Переменные окружения (из Makefile):
#   AM_URL, KC_TOKEN_URL, KC_TEST_USER_CLIENT_ID,
#   KC_TEST_USER_CLIENT_SECRET, KC_ADMIN_USERNAME, KC_ADMIN_PASSWORD
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

log_info "=== AM Admin Auth Tests (4) ==="

# Получаем токен пользователя admin
log_info "Получение токена пользователя admin..."
ADMIN_TOKEN=$(get_user_token "$KC_TOKEN_URL" "$KC_TEST_USER_CLIENT_ID" \
    "$KC_TEST_USER_CLIENT_SECRET" "$KC_ADMIN_USERNAME" "$KC_ADMIN_PASSWORD")
if [[ -z "$ADMIN_TOKEN" ]]; then
    log_fail "Не удалось получить токен admin"
    print_summary
    exit 1
fi
log_ok "Токен admin получен"

# ---------- Тест 4: GET /api/v1/admin-auth/me ----------
log_info "Тест 4: GET /api/v1/admin-auth/me"
RESPONSE=$(http_get "$AM_URL" "$ADMIN_TOKEN" "/api/v1/admin-auth/me")
CODE=$(get_response_code "$RESPONSE")
BODY=$(get_response_body "$RESPONSE")

if [[ "$CODE" == "200" ]]; then
    USERNAME=$(echo "$BODY" | jq -r '.username')
    EFF_ROLE=$(echo "$BODY" | jq -r '.effective_role')
    IDP_ROLE=$(echo "$BODY" | jq -r '.idp_role')
    if [[ "$USERNAME" == "admin" && "$EFF_ROLE" == "admin" && "$IDP_ROLE" == "admin" ]]; then
        test_pass "Тест 4: admin-auth/me → username=admin, effective_role=admin"
    else
        test_fail "Тест 4: admin-auth/me → username=${USERNAME}, effective_role=${EFF_ROLE}, idp_role=${IDP_ROLE}"
    fi
else
    test_fail "Тест 4: admin-auth/me → HTTP ${CODE} (ожидался 200)"
    BODY=$(get_response_body "$RESPONSE")
    if echo "$BODY" | jq . >/dev/null 2>&1; then
        log_fail "  Ответ: $(echo "$BODY" | jq -c '.')"
    fi
fi

print_summary
