#!/usr/bin/env bash
# ==========================================================================
# test-qm-auth.sh — Интеграционные тесты Query Module: Аутентификация
#
# Тесты 4-6: 401 без токена, 403 без scope, 200 с валидным JWT
# ==========================================================================

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/lib.sh"

# Проверка обязательных переменных
: "${QM_URL:?QM_URL не задана}"
: "${KC_TOKEN_URL:?KC_TOKEN_URL не задана}"
: "${KC_TEST_USER_CLIENT_ID:?KC_TEST_USER_CLIENT_ID не задана}"
: "${KC_TEST_USER_CLIENT_SECRET:?KC_TEST_USER_CLIENT_SECRET не задана}"
: "${KC_ADMIN_USERNAME:?KC_ADMIN_USERNAME не задана}"
: "${KC_ADMIN_PASSWORD:?KC_ADMIN_PASSWORD не задана}"

echo ""
log_info "=========================================="
log_info "  Query Module: Auth (тесты 4-6)"
log_info "=========================================="
echo ""

# --------------------------------------------------------------------------
# Тест 4: POST /api/v1/search без JWT → 401
# --------------------------------------------------------------------------
log_info "Тест 4: POST /api/v1/search без JWT → 401"
response=$(http_post "$QM_URL" "" "/api/v1/search" '{"limit":10,"offset":0}')
code=$(get_response_code "$response")

if [[ "$code" == "401" ]]; then
    test_pass "Тест 4: /api/v1/search без JWT → 401"
else
    test_fail "Тест 4: /api/v1/search без JWT → ожидался 401, получен ${code}"
fi

# --------------------------------------------------------------------------
# Тест 5: POST /api/v1/search с SA-токеном QM (без user-scope) → 403
# QM SA имеет scope files:read, но тестируем с client_credentials SA
# который не имеет роли admin/readonly и scope files:read
# Используем artstore-test-init — у него нет scope files:read
# --------------------------------------------------------------------------
log_info "Тест 5: POST /api/v1/search с токеном без нужных прав → 403"
init_token=$(get_token_from_keycloak "$KC_TOKEN_URL" "artstore-test-init" "test-init-secret") || true

if [[ -n "$init_token" && "$init_token" != "null" ]]; then
    response=$(http_post "$QM_URL" "$init_token" "/api/v1/search" '{"limit":10,"offset":0}')
    code=$(get_response_code "$response")

    if [[ "$code" == "403" ]]; then
        test_pass "Тест 5: /api/v1/search с токеном без прав → 403"
    else
        test_fail "Тест 5: /api/v1/search с токеном без прав → ожидался 403, получен ${code}"
    fi
else
    test_fail "Тест 5: не удалось получить init-токен"
fi

# --------------------------------------------------------------------------
# Тест 6: POST /api/v1/search с валидным JWT (admin) → 200
# --------------------------------------------------------------------------
log_info "Тест 6: POST /api/v1/search с валидным JWT (admin) → 200"
admin_token=$(get_user_token "$KC_TOKEN_URL" \
    "$KC_TEST_USER_CLIENT_ID" "$KC_TEST_USER_CLIENT_SECRET" \
    "$KC_ADMIN_USERNAME" "$KC_ADMIN_PASSWORD") || true

if [[ -n "$admin_token" && "$admin_token" != "null" ]]; then
    response=$(http_post "$QM_URL" "$admin_token" "/api/v1/search" '{"limit":10,"offset":0}')
    code=$(get_response_code "$response")

    if [[ "$code" == "200" ]]; then
        test_pass "Тест 6: /api/v1/search с admin JWT → 200"
    else
        test_fail "Тест 6: /api/v1/search с admin JWT → ожидался 200, получен ${code}"
        body=$(get_response_body "$response")
        log_fail "  Ответ: ${body}"
    fi
else
    test_fail "Тест 6: не удалось получить admin-токен"
fi

# --------------------------------------------------------------------------
print_summary
