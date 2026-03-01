#!/usr/bin/env bash
# ==========================================================================
# test-se-basic.sh — Базовые интеграционные тесты Storage Element
#
# Тесты 1-12: health, info, upload, download, list, delete, mode, readiness
#
# Переменные окружения (из Makefile):
#   SE_EDIT_1_URL, SE_RW_1_URL, SE_RO_URL, SE_AR_URL
#   KC_TOKEN_URL, KC_TEST_USER_CLIENT_ID, KC_TEST_USER_CLIENT_SECRET
#   KC_ADMIN_USERNAME, KC_ADMIN_PASSWORD, KC_VIEWER_USERNAME, KC_VIEWER_PASSWORD
# ==========================================================================

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/lib.sh"

# Проверяем обязательные переменные
: "${SE_EDIT_1_URL:?SE_EDIT_1_URL не задана}"
: "${SE_RW_1_URL:?SE_RW_1_URL не задана}"
: "${SE_RO_URL:?SE_RO_URL не задана}"
: "${SE_AR_URL:?SE_AR_URL не задана}"
: "${KC_TOKEN_URL:?KC_TOKEN_URL не задана}"

# ==========================================================================
# Получение токенов
# ==========================================================================
log_info "Получение токенов из Keycloak..."

# Admin с полным набором scopes для SE операций
ADMIN_TOKEN=$(get_user_token_with_scopes "$KC_TOKEN_URL" \
    "$KC_TEST_USER_CLIENT_ID" "$KC_TEST_USER_CLIENT_SECRET" \
    "$KC_ADMIN_USERNAME" "$KC_ADMIN_PASSWORD" \
    "openid files:read files:write storage:read storage:write")
if [[ -z "$ADMIN_TOKEN" ]]; then
    log_fail "Не удалось получить admin-токен"
    exit 1
fi

# Viewer — только read scopes
VIEWER_TOKEN=$(get_user_token_with_scopes "$KC_TOKEN_URL" \
    "$KC_TEST_USER_CLIENT_ID" "$KC_TEST_USER_CLIENT_SECRET" \
    "$KC_VIEWER_USERNAME" "$KC_VIEWER_PASSWORD" \
    "openid files:read storage:read")
if [[ -z "$VIEWER_TOKEN" ]]; then
    log_fail "Не удалось получить viewer-токен"
    exit 1
fi

log_ok "Токены получены"

# ==========================================================================
# Тест 1: Health — liveness проба
# ==========================================================================
echo ""
log_info "=== Тест 1: Health — liveness проба ==="

response=$(http_get "$SE_EDIT_1_URL" "" "/health/live")
code=$(get_response_code "$response")
body=$(get_response_body "$response")

if [[ "$code" == "200" ]]; then
    version=$(echo "$body" | jq -r '.version // empty')
    if [[ "$version" == v0.3.* ]]; then
        test_pass "Health liveness: 200, version=${version}"
    else
        test_fail "Health liveness: неожиданная версия '${version}'"
    fi
else
    test_fail "Health liveness: ожидался 200, получен ${code}"
fi

# ==========================================================================
# Тест 2: Health — readiness проба
# ==========================================================================
log_info "=== Тест 2: Health — readiness проба ==="

response=$(http_get "$SE_EDIT_1_URL" "" "/health/ready")
code=$(get_response_code "$response")

if [[ "$code" == "200" ]]; then
    test_pass "Health readiness: 200"
else
    test_fail "Health readiness: ожидался 200, получен ${code}"
fi

# ==========================================================================
# Тест 3: Info — stateless (нет полей role, leader_addr)
# ==========================================================================
log_info "=== Тест 3: Info — stateless (нет legacy-полей) ==="

response=$(http_get "$SE_EDIT_1_URL" "$ADMIN_TOKEN" "/api/v1/info")
code=$(get_response_code "$response")
body=$(get_response_body "$response")

if [[ "$code" == "200" ]]; then
    role=$(echo "$body" | jq -r '.role // empty')
    leader=$(echo "$body" | jq -r '.leader_addr // empty')
    replica_mode=$(echo "$body" | jq -r '.replica_mode // empty')
    storage_id=$(echo "$body" | jq -r '.storage_id // empty')
    mode=$(echo "$body" | jq -r '.mode // empty')

    if [[ -z "$role" && -z "$leader" && -z "$replica_mode" && "$storage_id" == "se-edit-1" && "$mode" == "edit" ]]; then
        test_pass "Info: stateless (нет role/leader/replica_mode), storage_id=${storage_id}, mode=${mode}"
    else
        test_fail "Info: legacy-поля присутствуют (role=${role}, leader=${leader}, replica_mode=${replica_mode})"
    fi
else
    test_fail "Info: ожидался 200, получен ${code}"
fi

# ==========================================================================
# Тест 4: Auth — запрос к защищённому endpoint без токена → 401
# ==========================================================================
log_info "=== Тест 4: Auth — upload без токена → 401 ==="

response=$(upload_file "$SE_EDIT_1_URL" "" "noauth-test.txt" "text/plain" "")
code=$(get_response_code "$response")

if [[ "$code" == "401" ]]; then
    test_pass "Auth: upload без токена → 401"
else
    test_fail "Auth: upload без токена → ожидался 401, получен ${code}"
fi

# ==========================================================================
# Тест 5: Upload + Download цикл на edit SE
# ==========================================================================
echo ""
log_info "=== Тест 5: Upload файла на edit SE ==="

UPLOAD_RESPONSE=$(upload_file "$SE_EDIT_1_URL" "$ADMIN_TOKEN" \
    "test-basic.txt" "text/plain" "Тестовый файл для basic тестов")
UPLOAD_CODE=$(get_response_code "$UPLOAD_RESPONSE")
UPLOAD_BODY=$(get_response_body "$UPLOAD_RESPONSE")

if [[ "$UPLOAD_CODE" == "201" ]]; then
    FILE_ID=$(echo "$UPLOAD_BODY" | jq -r '.file_id // empty')
    if [[ -n "$FILE_ID" ]]; then
        test_pass "Upload: 201, file_id=${FILE_ID}"
    else
        test_fail "Upload: 201, но file_id пустой"
    fi
else
    test_fail "Upload: ожидался 201, получен ${UPLOAD_CODE}"
    UPLOAD_BODY_ERR=$(get_response_body "$UPLOAD_RESPONSE")
    log_fail "  Ответ: ${UPLOAD_BODY_ERR}"
    FILE_ID=""
fi

# ==========================================================================
# Тест 6: Download загруженного файла
# ==========================================================================
log_info "=== Тест 6: Download загруженного файла ==="

if [[ -n "${FILE_ID:-}" ]]; then
    tmpfile=$(mktemp)
    http_code=$(curl $CURL_OPTS -w "%{http_code}" -o "$tmpfile" \
        -H "Authorization: Bearer ${ADMIN_TOKEN}" \
        "${SE_EDIT_1_URL}/api/v1/files/${FILE_ID}/download") || http_code="000"

    if [[ "$http_code" == "200" ]]; then
        fsize=$(wc -c < "$tmpfile" | tr -d ' ')
        if [[ "$fsize" -gt 0 ]]; then
            test_pass "Download: 200, size=${fsize} bytes"
        else
            test_fail "Download: 200, но файл пустой"
        fi
    else
        test_fail "Download: ожидался 200, получен ${http_code}"
    fi
    rm -f "$tmpfile"
else
    test_fail "Download: пропущен (upload не удался)"
fi

# ==========================================================================
# Тест 7: List файлов (должен содержать загруженный файл)
# ==========================================================================
log_info "=== Тест 7: List файлов ==="

response=$(http_get "$SE_EDIT_1_URL" "$ADMIN_TOKEN" "/api/v1/files")
code=$(get_response_code "$response")
body=$(get_response_body "$response")

if [[ "$code" == "200" ]]; then
    count=$(echo "$body" | jq -r '.total // 0')
    if [[ "$count" -ge 1 ]]; then
        test_pass "List: 200, total=${count}"
    else
        test_fail "List: 200, но total=0 (ожидался >= 1)"
    fi
else
    test_fail "List: ожидался 200, получен ${code}"
fi

# ==========================================================================
# Тест 8: Get metadata
# ==========================================================================
log_info "=== Тест 8: Get metadata ==="

if [[ -n "${FILE_ID:-}" ]]; then
    response=$(http_get "$SE_EDIT_1_URL" "$ADMIN_TOKEN" "/api/v1/files/${FILE_ID}")
    code=$(get_response_code "$response")
    body=$(get_response_body "$response")

    if [[ "$code" == "200" ]]; then
        fname=$(echo "$body" | jq -r '.original_filename // empty')
        if [[ "$fname" == "test-basic.txt" ]]; then
            test_pass "Metadata: 200, original_filename=${fname}"
        else
            test_fail "Metadata: 200, но original_filename='${fname}' != 'test-basic.txt'"
        fi
    else
        test_fail "Metadata: ожидался 200, получен ${code}"
    fi
else
    test_fail "Metadata: пропущен (upload не удался)"
fi

# ==========================================================================
# Тест 9: Delete файла на edit SE
# ==========================================================================
log_info "=== Тест 9: Delete файла ==="

if [[ -n "${FILE_ID:-}" ]]; then
    response=$(http_delete "$SE_EDIT_1_URL" "$ADMIN_TOKEN" "/api/v1/files/${FILE_ID}")
    code=$(get_response_code "$response")

    if [[ "$code" == "200" || "$code" == "204" ]]; then
        test_pass "Delete: ${code}"
    else
        test_fail "Delete: ожидался 200/204, получен ${code}"
    fi
else
    test_fail "Delete: пропущен (upload не удался)"
fi

# ==========================================================================
# Тест 10: Viewer — read-only операции доступны
# ==========================================================================
echo ""
log_info "=== Тест 10: Viewer — read-only доступ ==="

response=$(http_get "$SE_EDIT_1_URL" "$VIEWER_TOKEN" "/api/v1/files")
code=$(get_response_code "$response")

if [[ "$code" == "200" ]]; then
    test_pass "Viewer: list файлов → 200"
else
    test_fail "Viewer: list файлов → ожидался 200, получен ${code}"
fi

# ==========================================================================
# Тест 11: Viewer — upload запрещён (403)
# ==========================================================================
log_info "=== Тест 11: Viewer — upload запрещён ==="

response=$(upload_file "$SE_EDIT_1_URL" "$VIEWER_TOKEN" "viewer-test.txt" "text/plain" "")
code=$(get_response_code "$response")

if [[ "$code" == "403" ]]; then
    test_pass "Viewer: upload → 403 (Forbidden)"
else
    test_fail "Viewer: upload → ожидался 403, получен ${code}"
fi

# ==========================================================================
# Тест 12: Info — разные SE, разные mode
# ==========================================================================
log_info "=== Тест 12: Info — проверка mode у разных SE ==="

# Используем массивы вместо строкового парсинга (URL содержат ":")
SE_NAMES=("se-rw-1" "se-ro" "se-ar")
SE_URLS=("$SE_RW_1_URL" "$SE_RO_URL" "$SE_AR_URL")
SE_MODES=("rw" "ro" "ar")

ok=true
for i in "${!SE_NAMES[@]}"; do
    name="${SE_NAMES[$i]}"
    url="${SE_URLS[$i]}"
    expected_mode="${SE_MODES[$i]}"

    response=$(http_get "$url" "$ADMIN_TOKEN" "/api/v1/info")
    code=$(get_response_code "$response")
    body=$(get_response_body "$response")

    if [[ "$code" == "200" ]]; then
        actual_mode=$(echo "$body" | jq -r '.mode // empty')
        if [[ "$actual_mode" != "$expected_mode" ]]; then
            log_fail "  ${name}: mode='${actual_mode}', ожидался '${expected_mode}'"
            ok=false
        fi
    else
        log_fail "  ${name}: HTTP ${code}"
        ok=false
    fi
done

if $ok; then
    test_pass "Info: все SE в корректных режимах (rw, ro, ar)"
else
    test_fail "Info: некорректные режимы у SE"
fi

# ==========================================================================
# Итого
# ==========================================================================
print_summary
