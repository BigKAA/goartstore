#!/usr/bin/env bash
# ==========================================================================
# test-am-service-accounts.sh — Тесты Service Accounts (тесты 10-15)
#
# Проверяет полный CRUD цикл SA: create, list, get, update scopes,
# rotate secret, delete.
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

log_info "=== AM Service Accounts Tests (10-15) ==="

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

# Уникальное имя SA для теста (избежание конфликтов при повторных запусках)
SA_NAME="test-sa-$(date +%s)"

# ---------- Тест 10: POST /api/v1/service-accounts — создание SA ----------
log_info "Тест 10: POST /api/v1/service-accounts (создание SA)"
RESPONSE=$(http_post "$AM_URL" "$ADMIN_TOKEN" "/api/v1/service-accounts" \
    "{\"name\": \"${SA_NAME}\", \"description\": \"Тестовый SA\", \"scopes\": [\"files:read\", \"storage:read\"]}")
CODE=$(get_response_code "$RESPONSE")
BODY=$(get_response_body "$RESPONSE")

if [[ "$CODE" == "201" ]]; then
    SA_ID=$(echo "$BODY" | jq -r '.id')
    CLIENT_ID=$(echo "$BODY" | jq -r '.client_id')
    CLIENT_SECRET=$(echo "$BODY" | jq -r '.client_secret')
    SA_STATUS=$(echo "$BODY" | jq -r '.status')
    SCOPES=$(echo "$BODY" | jq -c '.scopes')
    if [[ -n "$SA_ID" && "$SA_ID" != "null" && -n "$CLIENT_SECRET" && "$CLIENT_SECRET" != "null" && "$SA_STATUS" == "active" ]]; then
        test_pass "Тест 10: SA создан, id=${SA_ID}, client_id=${CLIENT_ID}, status=active"
    else
        test_fail "Тест 10: SA создан, но данные некорректны: id=${SA_ID}, status=${SA_STATUS}"
    fi
else
    test_fail "Тест 10: create SA → HTTP ${CODE} (ожидался 201)"
    if echo "$BODY" | jq . >/dev/null 2>&1; then
        log_fail "  Ответ: $(echo "$BODY" | jq -c '.')"
    fi
fi

# ---------- Тест 11: GET /api/v1/service-accounts — список SA ----------
log_info "Тест 11: GET /api/v1/service-accounts (список)"
RESPONSE=$(http_get "$AM_URL" "$ADMIN_TOKEN" "/api/v1/service-accounts")
CODE=$(get_response_code "$RESPONSE")
BODY=$(get_response_body "$RESPONSE")

if [[ "$CODE" == "200" ]]; then
    TOTAL=$(echo "$BODY" | jq -r '.total')
    # Проверяем, что наш SA есть в списке
    FOUND=$(echo "$BODY" | jq -r --arg name "$SA_NAME" '.items[] | select(.name == $name) | .id')
    if [[ "$TOTAL" -ge 1 && -n "$FOUND" ]]; then
        test_pass "Тест 11: SA list → total=${TOTAL}, наш SA найден"
    else
        test_fail "Тест 11: SA list → total=${TOTAL}, наш SA не найден"
    fi
else
    test_fail "Тест 11: SA list → HTTP ${CODE} (ожидался 200)"
fi

# ---------- Тест 12: GET /api/v1/service-accounts/{id} — получение SA ----------
log_info "Тест 12: GET /api/v1/service-accounts/{id}"
RESPONSE=$(http_get "$AM_URL" "$ADMIN_TOKEN" "/api/v1/service-accounts/${SA_ID}")
CODE=$(get_response_code "$RESPONSE")
BODY=$(get_response_body "$RESPONSE")

if [[ "$CODE" == "200" ]]; then
    NAME=$(echo "$BODY" | jq -r '.name')
    # client_secret НЕ должен возвращаться при GET
    SECRET_PRESENT=$(echo "$BODY" | jq 'has("client_secret")')
    SECRET_VAL=$(echo "$BODY" | jq -r '.client_secret // "absent"')
    if [[ "$NAME" == "$SA_NAME" && ("$SECRET_PRESENT" == "false" || "$SECRET_VAL" == "null" || "$SECRET_VAL" == "absent") ]]; then
        test_pass "Тест 12: SA get → name=${NAME}, client_secret скрыт"
    else
        test_fail "Тест 12: SA get → name=${NAME}, secret_present=${SECRET_PRESENT}"
    fi
else
    test_fail "Тест 12: SA get → HTTP ${CODE} (ожидался 200)"
fi

# ---------- Тест 13: PUT /api/v1/service-accounts/{id} — обновление scopes ----------
log_info "Тест 13: PUT /api/v1/service-accounts/{id} (обновление scopes)"
RESPONSE=$(http_put "$AM_URL" "$ADMIN_TOKEN" "/api/v1/service-accounts/${SA_ID}" \
    '{"scopes": ["files:read", "files:write", "storage:read", "storage:write"]}')
CODE=$(get_response_code "$RESPONSE")
BODY=$(get_response_body "$RESPONSE")

if [[ "$CODE" == "200" ]]; then
    SCOPES_COUNT=$(echo "$BODY" | jq '.scopes | length')
    HAS_WRITE=$(echo "$BODY" | jq '.scopes | index("files:write") != null')
    if [[ "$SCOPES_COUNT" -eq 4 && "$HAS_WRITE" == "true" ]]; then
        test_pass "Тест 13: SA scopes обновлены, count=${SCOPES_COUNT}"
    else
        test_fail "Тест 13: SA scopes → count=${SCOPES_COUNT}, has_write=${HAS_WRITE}"
    fi
else
    test_fail "Тест 13: SA update → HTTP ${CODE} (ожидался 200)"
fi

# ---------- Тест 14: POST /api/v1/service-accounts/{id}/rotate-secret — ротация секрета ----------
log_info "Тест 14: POST /api/v1/service-accounts/{id}/rotate-secret"
RESPONSE=$(http_post "$AM_URL" "$ADMIN_TOKEN" "/api/v1/service-accounts/${SA_ID}/rotate-secret" "")
CODE=$(get_response_code "$RESPONSE")
BODY=$(get_response_body "$RESPONSE")

if [[ "$CODE" == "200" ]]; then
    NEW_SECRET=$(echo "$BODY" | jq -r '.client_secret')
    NEW_CLIENT_ID=$(echo "$BODY" | jq -r '.client_id')
    if [[ -n "$NEW_SECRET" && "$NEW_SECRET" != "null" && "$NEW_SECRET" != "$CLIENT_SECRET" ]]; then
        test_pass "Тест 14: rotate-secret → новый секрет получен, отличается от старого"
    else
        test_fail "Тест 14: rotate-secret → new_secret=${NEW_SECRET}"
    fi
else
    test_fail "Тест 14: rotate-secret → HTTP ${CODE} (ожидался 200)"
fi

# ---------- Тест 15: DELETE /api/v1/service-accounts/{id} — удаление SA ----------
log_info "Тест 15: DELETE /api/v1/service-accounts/{id}"
RESPONSE=$(http_delete "$AM_URL" "$ADMIN_TOKEN" "/api/v1/service-accounts/${SA_ID}")
CODE=$(get_response_code "$RESPONSE")

if [[ "$CODE" == "204" ]]; then
    # Проверяем, что SA больше не доступен
    RESPONSE2=$(http_get "$AM_URL" "$ADMIN_TOKEN" "/api/v1/service-accounts/${SA_ID}")
    CODE2=$(get_response_code "$RESPONSE2")
    if [[ "$CODE2" == "404" ]]; then
        test_pass "Тест 15: SA удалён, GET возвращает 404"
    else
        test_fail "Тест 15: SA удалён, но GET возвращает HTTP ${CODE2}"
    fi
else
    test_fail "Тест 15: delete SA → HTTP ${CODE} (ожидался 204)"
fi

print_summary
