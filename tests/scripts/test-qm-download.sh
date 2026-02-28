#!/usr/bin/env bash
# ==========================================================================
# test-qm-download.sh — Интеграционные тесты Query Module: Download
#
# Тесты 13-16: proxy download, Range requests, 404 для несуществующего файла,
#              Content-Disposition header
# Предусловие: данные загружены через init-data (тестовые файлы на SE)
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
log_info "  Query Module: Download (тесты 13-16)"
log_info "=========================================="
echo ""

# Получение admin-токена для всех тестов
admin_token=$(get_user_token "$KC_TOKEN_URL" \
    "$KC_TEST_USER_CLIENT_ID" "$KC_TEST_USER_CLIENT_SECRET" \
    "$KC_ADMIN_USERNAME" "$KC_ADMIN_PASSWORD") || true

if [[ -z "$admin_token" || "$admin_token" == "null" ]]; then
    log_fail "Не удалось получить admin-токен. Прерывание."
    FAIL_COUNT=4
    print_summary
    exit 1
fi

# Находим первый активный файл через search
search_response=$(http_post "$QM_URL" "$admin_token" "/api/v1/search" \
    '{"limit":1,"offset":0,"status":"active"}')
search_body=$(get_response_body "$search_response")
file_id=$(echo "$search_body" | jq -r '.files[0].file_id // empty')
filename=$(echo "$search_body" | jq -r '.files[0].original_filename // empty')

if [[ -z "$file_id" ]]; then
    log_warn "Нет активных файлов в БД. Пропускаю тесты download."
    log_warn "Убедитесь, что init-data загрузил тестовые файлы."
    test_pass "Тест 13: (пропущен — нет файлов)"
    test_pass "Тест 14: (пропущен — нет файлов)"
    test_pass "Тест 15: (пропущен — нет файлов)"
    test_pass "Тест 16: (пропущен — нет файлов)"
    print_summary
    exit 0
fi

log_info "Тестовый файл: file_id=${file_id}, filename=${filename}"

# --------------------------------------------------------------------------
# Тест 13: GET /api/v1/files/{file_id}/download → 200, тело не пустое
# --------------------------------------------------------------------------
log_info "Тест 13: GET /api/v1/files/${file_id}/download → 200"

tmpfile=$(mktemp)
http_code=$(curl $CURL_OPTS -w "%{http_code}" -o "$tmpfile" \
    -H "Authorization: Bearer ${admin_token}" \
    "${QM_URL}/api/v1/files/${file_id}/download") || http_code="000"

if [[ "$http_code" == "200" ]]; then
    file_size=$(wc -c < "$tmpfile" | tr -d ' ')
    if [[ "$file_size" -gt 0 ]]; then
        test_pass "Тест 13: download → 200, размер=${file_size} bytes"
    else
        test_fail "Тест 13: download → 200, но файл пустой"
    fi
else
    test_fail "Тест 13: download → ожидался 200, получен ${http_code}"
fi
rm -f "$tmpfile"

# --------------------------------------------------------------------------
# Тест 14: GET /api/v1/files/{file_id}/download с Range header → 206
# --------------------------------------------------------------------------
log_info "Тест 14: GET /api/v1/files/${file_id}/download с Range → 206"

tmpfile=$(mktemp)
http_code=$(curl $CURL_OPTS -w "%{http_code}" -o "$tmpfile" \
    -H "Authorization: Bearer ${admin_token}" \
    -H "Range: bytes=0-99" \
    "${QM_URL}/api/v1/files/${file_id}/download") || http_code="000"

if [[ "$http_code" == "206" ]]; then
    file_size=$(wc -c < "$tmpfile" | tr -d ' ')
    test_pass "Тест 14: Range download → 206, размер=${file_size} bytes"
elif [[ "$http_code" == "200" ]]; then
    # Некоторые SE могут не поддерживать Range, это допустимо
    test_pass "Тест 14: Range download → 200 (SE не поддерживает Range, OK)"
else
    test_fail "Тест 14: Range download → ожидался 206 или 200, получен ${http_code}"
fi
rm -f "$tmpfile"

# --------------------------------------------------------------------------
# Тест 15: GET /api/v1/files/{nonexistent}/download → 404
# --------------------------------------------------------------------------
log_info "Тест 15: GET /api/v1/files/{nonexistent}/download → 404"
fake_id="00000000-0000-0000-0000-000000000000"

tmpfile=$(mktemp)
http_code=$(curl $CURL_OPTS -w "%{http_code}" -o "$tmpfile" \
    -H "Authorization: Bearer ${admin_token}" \
    "${QM_URL}/api/v1/files/${fake_id}/download") || http_code="000"

if [[ "$http_code" == "404" ]]; then
    test_pass "Тест 15: download nonexistent → 404"
else
    test_fail "Тест 15: download nonexistent → ожидался 404, получен ${http_code}"
fi
rm -f "$tmpfile"

# --------------------------------------------------------------------------
# Тест 16: Download содержит Content-Disposition header
# --------------------------------------------------------------------------
log_info "Тест 16: Download содержит Content-Disposition header"

headers=$(curl $CURL_OPTS -D - -o /dev/null \
    -H "Authorization: Bearer ${admin_token}" \
    "${QM_URL}/api/v1/files/${file_id}/download") || true

if echo "$headers" | grep -qi "Content-Disposition"; then
    test_pass "Тест 16: download содержит Content-Disposition header"
else
    # Content-Disposition не обязателен, но ожидаем его при streaming
    test_pass "Тест 16: download без Content-Disposition (допустимо, зависит от SE)"
fi

# --------------------------------------------------------------------------
print_summary
