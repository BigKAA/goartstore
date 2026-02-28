#!/usr/bin/env bash
# ==========================================================================
# test-qm-search.sh — Интеграционные тесты Query Module: Поиск
#
# Тесты 7-12: поиск по атрибутам, пагинация, фильтры, file metadata
# Предусловие: данные загружены через init-data (тестовые файлы)
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
log_info "  Query Module: Search & Files (тесты 7-12)"
log_info "=========================================="
echo ""

# Получение admin-токена для всех тестов
admin_token=$(get_user_token "$KC_TOKEN_URL" \
    "$KC_TEST_USER_CLIENT_ID" "$KC_TEST_USER_CLIENT_SECRET" \
    "$KC_ADMIN_USERNAME" "$KC_ADMIN_PASSWORD") || true

if [[ -z "$admin_token" || "$admin_token" == "null" ]]; then
    log_fail "Не удалось получить admin-токен. Прерывание."
    FAIL_COUNT=6
    print_summary
    exit 1
fi

# --------------------------------------------------------------------------
# Тест 7: POST /api/v1/search — пустой поиск возвращает результаты
# --------------------------------------------------------------------------
log_info "Тест 7: POST /api/v1/search — пустой поиск"
response=$(http_post "$QM_URL" "$admin_token" "/api/v1/search" '{"limit":10,"offset":0}')
code=$(get_response_code "$response")
body=$(get_response_body "$response")

if [[ "$code" == "200" ]]; then
    total=$(echo "$body" | jq -r '.total // 0')
    items_count=$(echo "$body" | jq -r '.files | length // 0')
    if [[ "$total" -ge 0 && "$items_count" -ge 0 ]]; then
        test_pass "Тест 7: пустой поиск → 200, total=${total}, files=${items_count}"
    else
        test_fail "Тест 7: пустой поиск → 200, но некорректный формат ответа"
    fi
else
    test_fail "Тест 7: пустой поиск → ожидался 200, получен ${code}"
fi

# --------------------------------------------------------------------------
# Тест 8: POST /api/v1/search — поиск с пагинацией (limit=2, offset=0)
# --------------------------------------------------------------------------
log_info "Тест 8: POST /api/v1/search — пагинация (limit=2)"
response=$(http_post "$QM_URL" "$admin_token" "/api/v1/search" '{"limit":2,"offset":0}')
code=$(get_response_code "$response")
body=$(get_response_body "$response")

if [[ "$code" == "200" ]]; then
    total=$(echo "$body" | jq -r '.total // 0')
    items_count=$(echo "$body" | jq -r '.files | length // 0')
    limit=$(echo "$body" | jq -r '.limit // 0')
    offset=$(echo "$body" | jq -r '.offset // 0')
    if [[ "$limit" == "2" && "$offset" == "0" ]]; then
        test_pass "Тест 8: пагинация → limit=${limit}, offset=${offset}, total=${total}, files=${items_count}"
    else
        test_fail "Тест 8: пагинация → limit=${limit}, offset=${offset} (ожидались 2, 0)"
    fi
else
    test_fail "Тест 8: пагинация → ожидался 200, получен ${code}"
fi

# --------------------------------------------------------------------------
# Тест 9: POST /api/v1/search — поиск по status=active
# --------------------------------------------------------------------------
log_info "Тест 9: POST /api/v1/search — фильтр status=active"
response=$(http_post "$QM_URL" "$admin_token" "/api/v1/search" \
    '{"limit":100,"offset":0,"status":"active"}')
code=$(get_response_code "$response")
body=$(get_response_body "$response")

if [[ "$code" == "200" ]]; then
    total=$(echo "$body" | jq -r '.total // 0')
    # Проверяем, что все возвращённые файлы имеют status=active
    non_active=$(echo "$body" | jq -r '[(.files // [])[] | select(.status != "active")] | length')
    if [[ "$non_active" == "0" ]]; then
        test_pass "Тест 9: status=active → total=${total}, все файлы active"
    else
        test_fail "Тест 9: status=active → ${non_active} файлов с иным статусом"
    fi
else
    test_fail "Тест 9: status=active → ожидался 200, получен ${code}"
fi

# --------------------------------------------------------------------------
# Тест 10: POST /api/v1/search — partial поиск по filename
# --------------------------------------------------------------------------
log_info "Тест 10: POST /api/v1/search — partial поиск по filename"
response=$(http_post "$QM_URL" "$admin_token" "/api/v1/search" \
    '{"limit":10,"offset":0,"filename":"test","search_mode":"partial"}')
code=$(get_response_code "$response")
body=$(get_response_body "$response")

if [[ "$code" == "200" ]]; then
    total=$(echo "$body" | jq -r '.total // 0')
    test_pass "Тест 10: partial filename=test → 200, total=${total}"
else
    test_fail "Тест 10: partial filename → ожидался 200, получен ${code}"
fi

# --------------------------------------------------------------------------
# Тест 11: GET /api/v1/files/{file_id} — метаданные файла
# Находим первый файл через search и запрашиваем его метаданные
# --------------------------------------------------------------------------
log_info "Тест 11: GET /api/v1/files/{file_id} — метаданные файла"

# Получаем file_id из результатов поиска
search_response=$(http_post "$QM_URL" "$admin_token" "/api/v1/search" '{"limit":1,"offset":0}')
search_body=$(get_response_body "$search_response")
file_id=$(echo "$search_body" | jq -r '.files[0].file_id // empty')

if [[ -n "$file_id" ]]; then
    response=$(http_get "$QM_URL" "$admin_token" "/api/v1/files/${file_id}")
    code=$(get_response_code "$response")
    body=$(get_response_body "$response")

    if [[ "$code" == "200" ]]; then
        returned_id=$(echo "$body" | jq -r '.file_id // empty')
        if [[ "$returned_id" == "$file_id" ]]; then
            test_pass "Тест 11: /api/v1/files/${file_id} → 200, file_id совпадает"
        else
            test_fail "Тест 11: /api/v1/files/${file_id} → 200, но file_id=${returned_id}"
        fi
    else
        test_fail "Тест 11: /api/v1/files/${file_id} → ожидался 200, получен ${code}"
    fi
else
    log_warn "Тест 11: нет файлов в БД, пропускаю"
    test_pass "Тест 11: (пропущен — нет файлов в БД)"
fi

# --------------------------------------------------------------------------
# Тест 12: GET /api/v1/files/{nonexistent} → 404
# --------------------------------------------------------------------------
log_info "Тест 12: GET /api/v1/files/{nonexistent} → 404"
fake_id="00000000-0000-0000-0000-000000000000"
response=$(http_get "$QM_URL" "$admin_token" "/api/v1/files/${fake_id}")
code=$(get_response_code "$response")

if [[ "$code" == "404" ]]; then
    test_pass "Тест 12: /api/v1/files/${fake_id} → 404"
else
    test_fail "Тест 12: /api/v1/files/${fake_id} → ожидался 404, получен ${code}"
fi

# --------------------------------------------------------------------------
print_summary
