#!/usr/bin/env bash
# ==========================================================================
# test-am-errors.sh — Тесты обработки ошибок AM (тесты 27-30)
#
# Проверяет корректность HTTP-ответов при ошибках авторизации:
# нет JWT → 401, нет роли → 403, SA без scope → 403, конфликт → 409.
#
# Переменные окружения (из Makefile):
#   AM_URL, KC_TOKEN_URL, KC_TEST_USER_CLIENT_ID,
#   KC_TEST_USER_CLIENT_SECRET, KC_ADMIN_USERNAME, KC_ADMIN_PASSWORD,
#   KC_VIEWER_USERNAME, KC_VIEWER_PASSWORD, KC_SA_CLIENT_ID, KC_SA_CLIENT_SECRET
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
: "${KC_VIEWER_USERNAME:=viewer}"
: "${KC_VIEWER_PASSWORD:=viewer}"
: "${KC_SA_CLIENT_ID:=artstore-admin-module}"
: "${KC_SA_CLIENT_SECRET:=admin-module-test-secret}"

log_info "=== AM Error Handling Tests (27-30) ==="

# Получаем токены
log_info "Получение токенов..."
VIEWER_TOKEN=$(get_user_token "$KC_TOKEN_URL" "$KC_TEST_USER_CLIENT_ID" \
    "$KC_TEST_USER_CLIENT_SECRET" "$KC_VIEWER_USERNAME" "$KC_VIEWER_PASSWORD")
if [[ -z "$VIEWER_TOKEN" ]]; then
    log_fail "Не удалось получить токен viewer"
    print_summary
    exit 1
fi
log_ok "Токен viewer получен"

SA_TOKEN=$(get_token_from_keycloak "$KC_TOKEN_URL" "$KC_SA_CLIENT_ID" "$KC_SA_CLIENT_SECRET")
if [[ -z "$SA_TOKEN" ]]; then
    log_fail "Не удалось получить SA токен"
    print_summary
    exit 1
fi
log_ok "SA токен получен"

# ---------- Тест 27: Без JWT → 401 ----------
log_info "Тест 27: GET /api/v1/admin-users без JWT → 401"
RESPONSE=$(http_get "$AM_URL" "" "/api/v1/admin-users")
CODE=$(get_response_code "$RESPONSE")
BODY=$(get_response_body "$RESPONSE")

if [[ "$CODE" == "401" ]]; then
    ERROR_CODE=$(echo "$BODY" | jq -r '.error.code // empty')
    if [[ "$ERROR_CODE" == "UNAUTHORIZED" ]]; then
        test_pass "Тест 27: без JWT → 401, code=UNAUTHORIZED"
    else
        test_pass "Тест 27: без JWT → 401 (error_code=${ERROR_CODE})"
    fi
else
    test_fail "Тест 27: без JWT → HTTP ${CODE} (ожидался 401)"
fi

# ---------- Тест 28: Viewer → admin-only endpoint → 403 ----------
log_info "Тест 28: POST /api/v1/service-accounts (viewer) → 403"
RESPONSE=$(http_post "$AM_URL" "$VIEWER_TOKEN" "/api/v1/service-accounts" \
    '{"name": "forbidden-sa", "scopes": ["files:read"]}')
CODE=$(get_response_code "$RESPONSE")
BODY=$(get_response_body "$RESPONSE")

if [[ "$CODE" == "403" ]]; then
    ERROR_CODE=$(echo "$BODY" | jq -r '.error.code // empty')
    if [[ "$ERROR_CODE" == "FORBIDDEN" ]]; then
        test_pass "Тест 28: viewer → admin-only → 403, code=FORBIDDEN"
    else
        test_pass "Тест 28: viewer → admin-only → 403 (error_code=${ERROR_CODE})"
    fi
else
    test_fail "Тест 28: viewer → admin-only → HTTP ${CODE} (ожидался 403)"
fi

# ---------- Тест 29: SA → user-only endpoint → 403 ----------
log_info "Тест 29: GET /api/v1/admin-auth/me (SA) → 403"
RESPONSE=$(http_get "$AM_URL" "$SA_TOKEN" "/api/v1/admin-auth/me")
CODE=$(get_response_code "$RESPONSE")
BODY=$(get_response_body "$RESPONSE")

if [[ "$CODE" == "403" ]]; then
    ERROR_CODE=$(echo "$BODY" | jq -r '.error.code // empty')
    test_pass "Тест 29: SA → user-only endpoint → 403 (code=${ERROR_CODE})"
else
    test_fail "Тест 29: SA → user-only → HTTP ${CODE} (ожидался 403)"
fi

# ---------- Тест 30: Конфликт (duplicate) → 409 ----------
log_info "Тест 30: Конфликт — создание SA с одинаковым именем → 409"

# Получаем admin токен для создания SA
ADMIN_TOKEN=$(get_user_token "$KC_TOKEN_URL" "$KC_TEST_USER_CLIENT_ID" \
    "$KC_TEST_USER_CLIENT_SECRET" "$KC_ADMIN_USERNAME" "$KC_ADMIN_PASSWORD")

# Уникальное имя для теста конфликта
CONFLICT_SA_NAME="conflict-sa-$(date +%s)"

# Создаём первый SA
RESPONSE1=$(http_post "$AM_URL" "$ADMIN_TOKEN" "/api/v1/service-accounts" \
    "{\"name\": \"${CONFLICT_SA_NAME}\", \"scopes\": [\"files:read\"]}")
CODE1=$(get_response_code "$RESPONSE1")
if [[ "$CODE1" != "201" ]]; then
    test_fail "Тест 30: не удалось создать первый SA (HTTP ${CODE1})"
else
    CONFLICT_SA_ID=$(get_response_body "$RESPONSE1" | jq -r '.id')

    # Пробуем создать второй с тем же именем
    RESPONSE2=$(http_post "$AM_URL" "$ADMIN_TOKEN" "/api/v1/service-accounts" \
        "{\"name\": \"${CONFLICT_SA_NAME}\", \"scopes\": [\"files:read\"]}")
    CODE2=$(get_response_code "$RESPONSE2")
    BODY2=$(get_response_body "$RESPONSE2")

    if [[ "$CODE2" == "409" ]]; then
        ERROR_CODE=$(echo "$BODY2" | jq -r '.error.code // empty')
        test_pass "Тест 30: конфликт → 409 (code=${ERROR_CODE})"
    else
        test_fail "Тест 30: дубликат SA → HTTP ${CODE2} (ожидался 409)"
    fi

    # Cleanup: удаляем тестовый SA
    http_delete "$AM_URL" "$ADMIN_TOKEN" "/api/v1/service-accounts/${CONFLICT_SA_ID}" >/dev/null 2>&1 || true
fi

print_summary
