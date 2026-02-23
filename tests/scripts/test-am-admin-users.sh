#!/usr/bin/env bash
# ==========================================================================
# test-am-admin-users.sh — Тесты Admin Users (тесты 5-9)
#
# Проверяет CRUD операции с пользователями: list, get, set role override,
# verify effective role, delete override.
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
: "${KC_VIEWER_USERNAME:=viewer}"
: "${KC_VIEWER_PASSWORD:=viewer}"

log_info "=== AM Admin Users Tests (5-9) ==="

# Получаем токены
log_info "Получение токенов..."
ADMIN_TOKEN=$(get_user_token "$KC_TOKEN_URL" "$KC_TEST_USER_CLIENT_ID" \
    "$KC_TEST_USER_CLIENT_SECRET" "$KC_ADMIN_USERNAME" "$KC_ADMIN_PASSWORD")
if [[ -z "$ADMIN_TOKEN" ]]; then
    log_fail "Не удалось получить токен admin"
    print_summary
    exit 1
fi
log_ok "Токен admin получен"

VIEWER_TOKEN=$(get_user_token "$KC_TOKEN_URL" "$KC_TEST_USER_CLIENT_ID" \
    "$KC_TEST_USER_CLIENT_SECRET" "$KC_VIEWER_USERNAME" "$KC_VIEWER_PASSWORD")
if [[ -z "$VIEWER_TOKEN" ]]; then
    log_fail "Не удалось получить токен viewer"
    print_summary
    exit 1
fi
log_ok "Токен viewer получен"

# ---------- Тест 5: GET /api/v1/admin-users — список пользователей ----------
log_info "Тест 5: GET /api/v1/admin-users"
RESPONSE=$(http_get "$AM_URL" "$ADMIN_TOKEN" "/api/v1/admin-users")
CODE=$(get_response_code "$RESPONSE")
BODY=$(get_response_body "$RESPONSE")

if [[ "$CODE" == "200" ]]; then
    TOTAL=$(echo "$BODY" | jq -r '.total')
    HAS_ITEMS=$(echo "$BODY" | jq 'has("items")')
    if [[ "$HAS_ITEMS" == "true" && "$TOTAL" -ge 2 ]]; then
        test_pass "Тест 5: admin-users list → total=${TOTAL}, items есть"
    else
        test_fail "Тест 5: admin-users list → total=${TOTAL}, has_items=${HAS_ITEMS}"
    fi
else
    test_fail "Тест 5: admin-users list → HTTP ${CODE} (ожидался 200)"
fi

# Находим ID пользователя viewer для дальнейших тестов
VIEWER_ID=$(echo "$BODY" | jq -r '.items[] | select(.username == "viewer") | .id')
if [[ -z "$VIEWER_ID" || "$VIEWER_ID" == "null" ]]; then
    log_warn "Viewer не найден в списке, пробуем через admin-auth/me"
    RESPONSE_ME=$(http_get "$AM_URL" "$VIEWER_TOKEN" "/api/v1/admin-auth/me")
    VIEWER_ID=$(get_response_body "$RESPONSE_ME" | jq -r '.id')
fi
log_info "Viewer ID: ${VIEWER_ID}"

# ---------- Тест 6: GET /api/v1/admin-users/{id} — получение пользователя ----------
log_info "Тест 6: GET /api/v1/admin-users/{id}"
RESPONSE=$(http_get "$AM_URL" "$ADMIN_TOKEN" "/api/v1/admin-users/${VIEWER_ID}")
CODE=$(get_response_code "$RESPONSE")
BODY=$(get_response_body "$RESPONSE")

if [[ "$CODE" == "200" ]]; then
    USERNAME=$(echo "$BODY" | jq -r '.username')
    IDP_ROLE=$(echo "$BODY" | jq -r '.idp_role')
    if [[ "$USERNAME" == "viewer" && "$IDP_ROLE" == "readonly" ]]; then
        test_pass "Тест 6: admin-users get → username=viewer, idp_role=readonly"
    else
        test_fail "Тест 6: admin-users get → username=${USERNAME}, idp_role=${IDP_ROLE}"
    fi
else
    test_fail "Тест 6: admin-users get → HTTP ${CODE} (ожидался 200)"
fi

# ---------- Тест 7: POST /api/v1/admin-users/{id}/role-override — установка role override ----------
log_info "Тест 7: POST /api/v1/admin-users/{id}/role-override"
RESPONSE=$(http_post "$AM_URL" "$ADMIN_TOKEN" "/api/v1/admin-users/${VIEWER_ID}/role-override" \
    '{"role": "admin"}')
CODE=$(get_response_code "$RESPONSE")
BODY=$(get_response_body "$RESPONSE")

if [[ "$CODE" == "200" ]]; then
    EFF_ROLE=$(echo "$BODY" | jq -r '.effective_role')
    OVERRIDE=$(echo "$BODY" | jq -r '.role_override')
    if [[ "$EFF_ROLE" == "admin" && "$OVERRIDE" == "admin" ]]; then
        test_pass "Тест 7: role-override → effective_role=admin, role_override=admin"
    else
        test_fail "Тест 7: role-override → effective_role=${EFF_ROLE}, role_override=${OVERRIDE}"
    fi
else
    test_fail "Тест 7: role-override → HTTP ${CODE} (ожидался 200)"
fi

# ---------- Тест 8: GET /api/v1/admin-users/{id} — проверка effective role ----------
log_info "Тест 8: Проверка effective_role после override"
RESPONSE=$(http_get "$AM_URL" "$ADMIN_TOKEN" "/api/v1/admin-users/${VIEWER_ID}")
CODE=$(get_response_code "$RESPONSE")
BODY=$(get_response_body "$RESPONSE")

if [[ "$CODE" == "200" ]]; then
    EFF_ROLE=$(echo "$BODY" | jq -r '.effective_role')
    IDP_ROLE=$(echo "$BODY" | jq -r '.idp_role')
    OVERRIDE=$(echo "$BODY" | jq -r '.role_override')
    if [[ "$EFF_ROLE" == "admin" && "$IDP_ROLE" == "readonly" && "$OVERRIDE" == "admin" ]]; then
        test_pass "Тест 8: effective_role=admin (override), idp_role=readonly"
    else
        test_fail "Тест 8: effective=${EFF_ROLE}, idp=${IDP_ROLE}, override=${OVERRIDE}"
    fi
else
    test_fail "Тест 8: admin-users get → HTTP ${CODE} (ожидался 200)"
fi

# ---------- Тест 9: DELETE /api/v1/admin-users/{id} — удаление role override ----------
log_info "Тест 9: DELETE /api/v1/admin-users/{id} (удаление role override)"
RESPONSE=$(http_delete "$AM_URL" "$ADMIN_TOKEN" "/api/v1/admin-users/${VIEWER_ID}")
CODE=$(get_response_code "$RESPONSE")

if [[ "$CODE" == "204" ]]; then
    # Проверяем, что override удалён
    RESPONSE2=$(http_get "$AM_URL" "$ADMIN_TOKEN" "/api/v1/admin-users/${VIEWER_ID}")
    BODY2=$(get_response_body "$RESPONSE2")
    EFF_ROLE=$(echo "$BODY2" | jq -r '.effective_role')
    OVERRIDE=$(echo "$BODY2" | jq -r '.role_override')
    if [[ "$EFF_ROLE" == "readonly" && ("$OVERRIDE" == "null" || -z "$OVERRIDE") ]]; then
        test_pass "Тест 9: role override удалён, effective_role=readonly"
    else
        test_fail "Тест 9: после удаления override → effective=${EFF_ROLE}, override=${OVERRIDE}"
    fi
else
    test_fail "Тест 9: delete override → HTTP ${CODE} (ожидался 204)"
fi

print_summary
