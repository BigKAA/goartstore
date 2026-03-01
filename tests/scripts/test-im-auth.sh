#!/usr/bin/env bash
# ==========================================================================
# test-im-auth.sh — Интеграционные тесты Ingester Module: Аутентификация
#
# Тесты 4-6: 401 без токена, 403 без scope, авторизация с admin JWT
# ==========================================================================

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/lib.sh"

# Проверка обязательных переменных
: "${IM_URL:?IM_URL не задана}"
: "${KC_TOKEN_URL:?KC_TOKEN_URL не задана}"
: "${KC_TEST_USER_CLIENT_ID:?KC_TEST_USER_CLIENT_ID не задана}"
: "${KC_TEST_USER_CLIENT_SECRET:?KC_TEST_USER_CLIENT_SECRET не задана}"
: "${KC_ADMIN_USERNAME:?KC_ADMIN_USERNAME не задана}"
: "${KC_ADMIN_PASSWORD:?KC_ADMIN_PASSWORD не задана}"

echo ""
log_info "=========================================="
log_info "  Ingester Module: Auth (тесты 4-6)"
log_info "=========================================="
echo ""

# Генерируем минимальный тестовый файл
TEST_FILE=$(mktemp)
dd if=/dev/urandom bs=1024 count=1 2>/dev/null > "$TEST_FILE"
trap 'rm -f "$TEST_FILE"' EXIT

# --------------------------------------------------------------------------
# Тест 4: POST /api/v1/files/upload без JWT → 401
# --------------------------------------------------------------------------
log_info "Тест 4: POST /api/v1/files/upload без JWT → 401"

tmpout=$(mktemp)
http_code=$(curl $CURL_OPTS -w "%{http_code}" -o "$tmpout" \
    -X POST \
    -F "file=@${TEST_FILE};filename=test.bin;type=application/octet-stream" \
    -F "retention_policy=temporary" \
    -F "ttl_days=7" \
    "${IM_URL}/api/v1/files/upload") || http_code="000"
rm -f "$tmpout"

if [[ "$http_code" == "401" ]]; then
    test_pass "Тест 4: /api/v1/files/upload без JWT → 401"
else
    test_fail "Тест 4: /api/v1/files/upload без JWT → ожидался 401, получен ${http_code}"
fi

# --------------------------------------------------------------------------
# Тест 5: POST /api/v1/files/upload с viewer JWT (без scope files:write) → 403
# Viewer имеет роль readonly, но не admin и не scope files:write
# --------------------------------------------------------------------------
log_info "Тест 5: POST /api/v1/files/upload с viewer JWT → 403"
viewer_token=$(get_user_token "$KC_TOKEN_URL" \
    "$KC_TEST_USER_CLIENT_ID" "$KC_TEST_USER_CLIENT_SECRET" \
    "$KC_VIEWER_USERNAME" "$KC_VIEWER_PASSWORD") || true

if [[ -n "$viewer_token" && "$viewer_token" != "null" ]]; then
    tmpout=$(mktemp)
    http_code=$(curl $CURL_OPTS -w "%{http_code}" -o "$tmpout" \
        -X POST \
        -H "Authorization: Bearer ${viewer_token}" \
        -F "file=@${TEST_FILE};filename=test.bin;type=application/octet-stream" \
        -F "retention_policy=temporary" \
        -F "ttl_days=7" \
        "${IM_URL}/api/v1/files/upload") || http_code="000"
    rm -f "$tmpout"

    if [[ "$http_code" == "403" ]]; then
        test_pass "Тест 5: /api/v1/files/upload с viewer JWT → 403"
    else
        test_fail "Тест 5: /api/v1/files/upload с viewer JWT → ожидался 403, получен ${http_code}"
    fi
else
    test_fail "Тест 5: не удалось получить viewer-токен"
fi

# --------------------------------------------------------------------------
# Тест 6: POST /api/v1/files/upload с admin JWT → не 401 и не 403
# (подтверждение что авторизация проходит; полный upload тестируется в upload-группе)
# --------------------------------------------------------------------------
log_info "Тест 6: POST /api/v1/files/upload с admin JWT → не 401, не 403"
admin_token=$(get_user_token "$KC_TOKEN_URL" \
    "$KC_TEST_USER_CLIENT_ID" "$KC_TEST_USER_CLIENT_SECRET" \
    "$KC_ADMIN_USERNAME" "$KC_ADMIN_PASSWORD") || true

if [[ -n "$admin_token" && "$admin_token" != "null" ]]; then
    tmpout=$(mktemp)
    http_code=$(curl $CURL_OPTS -w "%{http_code}" -o "$tmpout" \
        -X POST \
        -H "Authorization: Bearer ${admin_token}" \
        -F "file=@${TEST_FILE};filename=test.bin;type=application/octet-stream" \
        -F "retention_policy=temporary" \
        -F "ttl_days=7" \
        "${IM_URL}/api/v1/files/upload") || http_code="000"
    body=$(cat "$tmpout")
    rm -f "$tmpout"

    if [[ "$http_code" != "401" && "$http_code" != "403" ]]; then
        test_pass "Тест 6: /api/v1/files/upload с admin JWT → ${http_code} (авторизация OK)"
    else
        test_fail "Тест 6: /api/v1/files/upload с admin JWT → получен ${http_code} (ожидалось не 401/403)"
        log_fail "  Ответ: ${body}"
    fi
else
    test_fail "Тест 6: не удалось получить admin-токен"
fi

# --------------------------------------------------------------------------
print_summary
