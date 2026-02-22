#!/usr/bin/env bash
# ==========================================================================
# test-files.sh — Тесты файловых операций (тесты 4-11)
#
# Проверяет полный цикл работы с файлами на SE в режиме edit:
#   4.  POST upload (1KB) → 201, file_id != ""
#   5.  GET download → 200, Content-Type == "application/octet-stream"
#   6.  GET download Range: bytes=0-99 → 206, Content-Range header
#   7.  GET download If-None-Match: {etag} → 304
#   8.  GET /files?limit=2&offset=0 → 200, items length <= 2
#   9.  GET /files/{id} → 200, metadata
#   10. PATCH /files/{id} → 200, description обновлён
#   11. DELETE /files/{id} → 204, повторный GET → deleted
#
# Переменные окружения:
#   JWKS_MOCK_URL   — URL JWKS Mock (по умолчанию https://localhost:18080)
#   SE_EDIT_1_URL   — URL SE edit-1 (по умолчанию https://localhost:18010)
#
# Использование:
#   ./test-files.sh
# ==========================================================================
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/lib.sh"

: "${JWKS_MOCK_URL:=https://localhost:18080}"
: "${SE_EDIT_1_URL:=https://localhost:18010}"

log_info "========================================================"
log_info "  Тесты файловых операций (4-11)"
log_info "  JWKS Mock: ${JWKS_MOCK_URL}"
log_info "  SE: ${SE_EDIT_1_URL}"
log_info "========================================================"
echo ""

# --------------------------------------------------------------------------
# Подготовка: получить JWT с полными правами
# --------------------------------------------------------------------------
log_info "Получение JWT токена..."
TOKEN=$(get_token "$JWKS_MOCK_URL" "test-files" '["files:read","files:write","storage:write"]' 3600)
if [[ -z "$TOKEN" || "$TOKEN" == "null" ]]; then
    log_fail "Не удалось получить JWT токен — прерывание"
    exit 1
fi
log_ok "JWT получен"
echo ""

# ==========================================================================
# Тест 4: POST upload (1KB) → 201, file_id != ""
# ==========================================================================
log_info "[Тест 4] POST /api/v1/files/upload → 201, file_id != \"\""

RESPONSE=$(upload_file "$SE_EDIT_1_URL" "$TOKEN" "test-upload.bin" "application/octet-stream" "Тестовый файл для интеграционных тестов")
CODE=$(get_response_code "$RESPONSE")
BODY=$(get_response_body "$RESPONSE")

if [[ "$CODE" == "201" ]]; then
    FILE_ID=$(echo "$BODY" | jq -r '.file_id // empty')
    if [[ -n "$FILE_ID" ]]; then
        test_pass "Тест 4: Upload → 201, file_id=${FILE_ID}"
    else
        test_fail "Тест 4: Upload → 201, но file_id отсутствует"
        FILE_ID=""
    fi
else
    test_fail "Тест 4: Upload → HTTP ${CODE} (ожидался 201)"
    FILE_ID=""
fi

# Прерываем если upload не удался — остальные тесты зависят от file_id
if [[ -z "$FILE_ID" ]]; then
    log_fail "Нет file_id — пропускаем тесты 5-11"
    print_summary
    exit 1
fi

# ==========================================================================
# Тест 5: GET download → 200, Content-Type == "application/octet-stream"
# ==========================================================================
log_info "[Тест 5] GET /api/v1/files/${FILE_ID}/download → 200"

# Используем curl напрямую чтобы получить заголовки ответа
TMPOUT=$(mktemp)
TMPHEADERS=$(mktemp)
HTTP_CODE=$(curl $CURL_OPTS -w "%{http_code}" -o "$TMPOUT" -D "$TMPHEADERS" \
    -H "Authorization: Bearer ${TOKEN}" \
    "${SE_EDIT_1_URL}/api/v1/files/${FILE_ID}/download") || HTTP_CODE="000"

if [[ "$HTTP_CODE" == "200" ]]; then
    CT=$(grep -i "^content-type:" "$TMPHEADERS" | tr -d '\r' | awk '{print $2}' | head -1)
    if echo "$CT" | grep -qi "application/octet-stream"; then
        test_pass "Тест 5: Download → 200, Content-Type=${CT}"
    else
        test_fail "Тест 5: Download → 200, но Content-Type=\"${CT}\" (ожидался application/octet-stream)"
    fi
    # Сохраняем ETag для теста 7
    ETAG=$(grep -i "^etag:" "$TMPHEADERS" | tr -d '\r' | awk '{print $2}' | head -1)
else
    test_fail "Тест 5: Download → HTTP ${HTTP_CODE} (ожидался 200)"
    ETAG=""
fi
rm -f "$TMPOUT" "$TMPHEADERS"

# ==========================================================================
# Тест 6: GET download Range: bytes=0-99 → 206, Content-Range header
# ==========================================================================
log_info "[Тест 6] GET /api/v1/files/${FILE_ID}/download Range: bytes=0-99 → 206"

TMPOUT=$(mktemp)
TMPHEADERS=$(mktemp)
HTTP_CODE=$(curl $CURL_OPTS -w "%{http_code}" -o "$TMPOUT" -D "$TMPHEADERS" \
    -H "Authorization: Bearer ${TOKEN}" \
    -H "Range: bytes=0-99" \
    "${SE_EDIT_1_URL}/api/v1/files/${FILE_ID}/download") || HTTP_CODE="000"

if [[ "$HTTP_CODE" == "206" ]]; then
    CONTENT_RANGE=$(grep -i "^content-range:" "$TMPHEADERS" | tr -d '\r' | head -1)
    if [[ -n "$CONTENT_RANGE" ]]; then
        test_pass "Тест 6: Range request → 206, ${CONTENT_RANGE}"
    else
        test_fail "Тест 6: Range request → 206, но Content-Range header отсутствует"
    fi
else
    test_fail "Тест 6: Range request → HTTP ${HTTP_CODE} (ожидался 206)"
fi
rm -f "$TMPOUT" "$TMPHEADERS"

# ==========================================================================
# Тест 7: GET download If-None-Match: {etag} → 304
# ==========================================================================
log_info "[Тест 7] GET /api/v1/files/${FILE_ID}/download If-None-Match → 304"

if [[ -n "${ETAG:-}" ]]; then
    TMPOUT=$(mktemp)
    HTTP_CODE=$(curl $CURL_OPTS -w "%{http_code}" -o "$TMPOUT" \
        -H "Authorization: Bearer ${TOKEN}" \
        -H "If-None-Match: ${ETAG}" \
        "${SE_EDIT_1_URL}/api/v1/files/${FILE_ID}/download") || HTTP_CODE="000"

    if [[ "$HTTP_CODE" == "304" ]]; then
        test_pass "Тест 7: If-None-Match → 304 (ETag=${ETAG})"
    else
        test_fail "Тест 7: If-None-Match → HTTP ${HTTP_CODE} (ожидался 304)"
    fi
    rm -f "$TMPOUT"
else
    test_fail "Тест 7: ETag не получен из теста 5 — пропуск"
fi

# ==========================================================================
# Тест 8: GET /files?limit=2&offset=0 → 200, items length <= 2
# ==========================================================================
log_info "[Тест 8] GET /api/v1/files?limit=2&offset=0 → 200, items <= 2"

RESPONSE=$(http_get "$SE_EDIT_1_URL" "$TOKEN" "/api/v1/files?limit=2&offset=0")
CODE=$(get_response_code "$RESPONSE")
BODY=$(get_response_body "$RESPONSE")

if [[ "$CODE" == "200" ]]; then
    ITEMS_LEN=$(echo "$BODY" | jq '.items | length')
    TOTAL=$(echo "$BODY" | jq '.total // 0')
    if [[ "$ITEMS_LEN" -le 2 ]]; then
        test_pass "Тест 8: List → 200, items=${ITEMS_LEN}, total=${TOTAL}"
    else
        test_fail "Тест 8: List → 200, но items=${ITEMS_LEN} (ожидалось <= 2)"
    fi
else
    test_fail "Тест 8: List → HTTP ${CODE} (ожидался 200)"
fi

# ==========================================================================
# Тест 9: GET /files/{id} → 200, metadata
# ==========================================================================
log_info "[Тест 9] GET /api/v1/files/${FILE_ID} → 200, metadata"

RESPONSE=$(http_get "$SE_EDIT_1_URL" "$TOKEN" "/api/v1/files/${FILE_ID}")
CODE=$(get_response_code "$RESPONSE")
BODY=$(get_response_body "$RESPONSE")

if [[ "$CODE" == "200" ]]; then
    GOT_ID=$(echo "$BODY" | jq -r '.file_id // empty')
    GOT_FILENAME=$(echo "$BODY" | jq -r '.original_filename // empty')
    if [[ "$GOT_ID" == "$FILE_ID" && -n "$GOT_FILENAME" ]]; then
        test_pass "Тест 9: Metadata → 200, file_id=${GOT_ID}, filename=${GOT_FILENAME}"
    else
        test_fail "Тест 9: Metadata → 200, но file_id=\"${GOT_ID}\" или filename пуст"
    fi
else
    test_fail "Тест 9: Metadata → HTTP ${CODE} (ожидался 200)"
fi

# ==========================================================================
# Тест 10: PATCH /files/{id} → 200, description обновлён
# ==========================================================================
log_info "[Тест 10] PATCH /api/v1/files/${FILE_ID} → 200, description обновлён"

NEW_DESC="Обновлённое описание (тест 10)"
PATCH_DATA=$(jq -n --arg desc "$NEW_DESC" '{description: $desc}')
RESPONSE=$(http_patch "$SE_EDIT_1_URL" "$TOKEN" "/api/v1/files/${FILE_ID}" "$PATCH_DATA")
CODE=$(get_response_code "$RESPONSE")
BODY=$(get_response_body "$RESPONSE")

if [[ "$CODE" == "200" ]]; then
    GOT_DESC=$(echo "$BODY" | jq -r '.description // empty')
    if [[ "$GOT_DESC" == "$NEW_DESC" ]]; then
        test_pass "Тест 10: Update → 200, description обновлён"
    else
        test_fail "Тест 10: Update → 200, но description=\"${GOT_DESC}\" (ожидался \"${NEW_DESC}\")"
    fi
else
    test_fail "Тест 10: Update → HTTP ${CODE} (ожидался 200)"
fi

# ==========================================================================
# Тест 11: DELETE /files/{id} → 204, повторный GET → deleted
# ==========================================================================
log_info "[Тест 11] DELETE /api/v1/files/${FILE_ID} → 204, повторный GET → deleted"

RESPONSE=$(http_delete "$SE_EDIT_1_URL" "$TOKEN" "/api/v1/files/${FILE_ID}")
CODE=$(get_response_code "$RESPONSE")

TEST11_PASS=true

if [[ "$CODE" == "204" || "$CODE" == "200" ]]; then
    log_ok "  DELETE → HTTP ${CODE}"
else
    test_fail "Тест 11a: DELETE → HTTP ${CODE} (ожидался 204)"
    TEST11_PASS=false
fi

# Проверяем что файл в статусе deleted
if [[ "$TEST11_PASS" == "true" ]]; then
    RESPONSE=$(http_get "$SE_EDIT_1_URL" "$TOKEN" "/api/v1/files/${FILE_ID}")
    CODE=$(get_response_code "$RESPONSE")
    BODY=$(get_response_body "$RESPONSE")

    if [[ "$CODE" == "200" ]]; then
        GOT_STATUS=$(echo "$BODY" | jq -r '.status // empty')
        if [[ "$GOT_STATUS" == "deleted" ]]; then
            test_pass "Тест 11: DELETE → ${CODE}, повторный GET → status=deleted"
        else
            test_fail "Тест 11b: После DELETE, GET → status=\"${GOT_STATUS}\" (ожидался \"deleted\")"
        fi
    elif [[ "$CODE" == "404" ]]; then
        # Файл может быть полностью удалён — тоже допустимо
        test_pass "Тест 11: DELETE → файл удалён (GET → 404)"
    else
        test_fail "Тест 11b: После DELETE, GET → HTTP ${CODE} (ожидался 200 или 404)"
    fi
fi

# ==========================================================================
# Итоги
# ==========================================================================
echo ""
print_summary
