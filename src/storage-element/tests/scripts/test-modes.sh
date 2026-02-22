#!/usr/bin/env bash
# ==========================================================================
# test-modes.sh — Тесты режимов работы (тесты 12-18)
#
# Проверяет ограничения операций по режимам и переходы между ними:
#   12. POST upload на se-ro → 409 MODE_NOT_ALLOWED
#   13. GET download на se-ar → 409 MODE_NOT_ALLOWED
#   14. DELETE на se-rw-1 → 409 MODE_NOT_ALLOWED (delete только в edit)
#   15. POST transition se-rw-2 {target_mode: "ro"} → 200
#   16. POST transition se-rw-2 (ro) {target_mode: "rw"} без confirm → 409 CONFIRMATION_REQUIRED
#   17. POST transition se-rw-2 {target_mode: "rw", confirm: true} → 200
#   18. POST transition se-edit-1 {target_mode: "rw"} → 409 INVALID_TRANSITION
#
# Переменные окружения:
#   JWKS_MOCK_URL   — URL JWKS Mock (по умолчанию https://localhost:18080)
#   SE_RO_URL       — URL SE ro (по умолчанию https://localhost:18014)
#   SE_AR_URL       — URL SE ar (по умолчанию https://localhost:18015)
#   SE_RW_1_URL     — URL SE rw-1 (по умолчанию https://localhost:18012)
#   SE_RW_2_URL     — URL SE rw-2 (по умолчанию https://localhost:18013)
#   SE_EDIT_1_URL   — URL SE edit-1 (по умолчанию https://localhost:18010)
#
# Использование:
#   ./test-modes.sh
# ==========================================================================
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/lib.sh"

: "${JWKS_MOCK_URL:=https://localhost:18080}"
: "${SE_RO_URL:=https://localhost:18014}"
: "${SE_AR_URL:=https://localhost:18015}"
: "${SE_RW_1_URL:=https://localhost:18012}"
: "${SE_RW_2_URL:=https://localhost:18013}"
: "${SE_EDIT_1_URL:=https://localhost:18010}"

log_info "========================================================"
log_info "  Тесты режимов работы (12-18)"
log_info "  JWKS Mock: ${JWKS_MOCK_URL}"
log_info "  SE RO: ${SE_RO_URL}"
log_info "  SE AR: ${SE_AR_URL}"
log_info "  SE RW-1: ${SE_RW_1_URL}"
log_info "  SE RW-2: ${SE_RW_2_URL}"
log_info "  SE EDIT-1: ${SE_EDIT_1_URL}"
log_info "========================================================"
echo ""

# --------------------------------------------------------------------------
# Подготовка: JWT
# --------------------------------------------------------------------------
log_info "Получение JWT токена..."
TOKEN=$(get_token "$JWKS_MOCK_URL" "test-modes" '["files:read","files:write","storage:write"]' 3600)
if [[ -z "$TOKEN" || "$TOKEN" == "null" ]]; then
    log_fail "Не удалось получить JWT токен — прерывание"
    exit 1
fi
log_ok "JWT получен"
echo ""

# ==========================================================================
# Тест 12: POST upload на se-ro → 409 MODE_NOT_ALLOWED
# ==========================================================================
log_info "[Тест 12] POST upload на se-ro → 409 MODE_NOT_ALLOWED"

RESPONSE=$(upload_file "$SE_RO_URL" "$TOKEN" "test-blocked.bin")
CODE=$(get_response_code "$RESPONSE")
BODY=$(get_response_body "$RESPONSE")

if [[ "$CODE" == "409" ]]; then
    ERR_CODE=$(echo "$BODY" | jq -r '.error.code // empty')
    if [[ "$ERR_CODE" == "MODE_NOT_ALLOWED" ]]; then
        test_pass "Тест 12: Upload на ro → 409 MODE_NOT_ALLOWED"
    else
        test_fail "Тест 12: Upload на ro → 409, но code=\"${ERR_CODE}\" (ожидался MODE_NOT_ALLOWED)"
    fi
else
    test_fail "Тест 12: Upload на ro → HTTP ${CODE} (ожидался 409)"
fi

# ==========================================================================
# Тест 13: GET download на se-ar → 409 MODE_NOT_ALLOWED
#
# se-ar содержит attr.json, но физические файлы удалены (init-data.sh).
# Режим ar запрещает download.
# ==========================================================================
log_info "[Тест 13] GET download на se-ar → 409 MODE_NOT_ALLOWED"

# Получаем file_id из se-ar (из init данных)
RESPONSE=$(http_get "$SE_AR_URL" "$TOKEN" "/api/v1/files?limit=1&offset=0")
CODE=$(get_response_code "$RESPONSE")
BODY=$(get_response_body "$RESPONSE")

if [[ "$CODE" == "200" ]]; then
    AR_FILE_ID=$(echo "$BODY" | jq -r '.items[0].file_id // empty')
else
    AR_FILE_ID=""
fi

if [[ -n "$AR_FILE_ID" ]]; then
    RESPONSE=$(http_get "$SE_AR_URL" "$TOKEN" "/api/v1/files/${AR_FILE_ID}/download")
    CODE=$(get_response_code "$RESPONSE")
    BODY=$(get_response_body "$RESPONSE")

    if [[ "$CODE" == "409" ]]; then
        ERR_CODE=$(echo "$BODY" | jq -r '.error.code // empty')
        if [[ "$ERR_CODE" == "MODE_NOT_ALLOWED" ]]; then
            test_pass "Тест 13: Download на ar → 409 MODE_NOT_ALLOWED"
        else
            test_fail "Тест 13: Download на ar → 409, но code=\"${ERR_CODE}\" (ожидался MODE_NOT_ALLOWED)"
        fi
    else
        test_fail "Тест 13: Download на ar → HTTP ${CODE} (ожидался 409)"
    fi
else
    test_fail "Тест 13: Не удалось получить file_id из se-ar для теста"
fi

# ==========================================================================
# Тест 14: DELETE на se-rw-1 → 409 MODE_NOT_ALLOWED
#
# DELETE доступен только в режиме edit. В rw — запрещён.
# ==========================================================================
log_info "[Тест 14] DELETE на se-rw-1 → 409 MODE_NOT_ALLOWED"

# Загружаем файл в se-rw-1 для удаления
RESPONSE=$(upload_file "$SE_RW_1_URL" "$TOKEN" "test-rw-delete.bin")
CODE=$(get_response_code "$RESPONSE")
BODY=$(get_response_body "$RESPONSE")

if [[ "$CODE" == "201" ]]; then
    RW_FILE_ID=$(echo "$BODY" | jq -r '.file_id // empty')
else
    RW_FILE_ID=""
fi

if [[ -n "$RW_FILE_ID" ]]; then
    RESPONSE=$(http_delete "$SE_RW_1_URL" "$TOKEN" "/api/v1/files/${RW_FILE_ID}")
    CODE=$(get_response_code "$RESPONSE")
    BODY=$(get_response_body "$RESPONSE")

    if [[ "$CODE" == "409" ]]; then
        ERR_CODE=$(echo "$BODY" | jq -r '.error.code // empty')
        if [[ "$ERR_CODE" == "MODE_NOT_ALLOWED" ]]; then
            test_pass "Тест 14: DELETE на rw → 409 MODE_NOT_ALLOWED"
        else
            test_fail "Тест 14: DELETE на rw → 409, но code=\"${ERR_CODE}\" (ожидался MODE_NOT_ALLOWED)"
        fi
    else
        test_fail "Тест 14: DELETE на rw → HTTP ${CODE} (ожидался 409)"
    fi
else
    test_fail "Тест 14: Не удалось загрузить файл в se-rw-1 для теста"
fi

# ==========================================================================
# Тест 15: POST transition se-rw-2 {target_mode: "ro"} → 200
# ==========================================================================
log_info "[Тест 15] POST transition se-rw-2 rw→ro → 200"

RESPONSE=$(transition_mode "$SE_RW_2_URL" "$TOKEN" "ro" "false")
CODE=$(get_response_code "$RESPONSE")
BODY=$(get_response_body "$RESPONSE")

if [[ "$CODE" == "200" ]]; then
    PREV=$(echo "$BODY" | jq -r '.previous_mode // empty')
    CURR=$(echo "$BODY" | jq -r '.current_mode // empty')
    if [[ "$CURR" == "ro" ]]; then
        test_pass "Тест 15: Transition rw→ro → 200, ${PREV}→${CURR}"
    else
        test_fail "Тест 15: Transition → 200, но current_mode=\"${CURR}\" (ожидался \"ro\")"
    fi
else
    test_fail "Тест 15: Transition rw→ro → HTTP ${CODE} (ожидался 200)"
fi

# ==========================================================================
# Тест 16: POST transition se-rw-2 (ro) → rw без confirm → 409 CONFIRMATION_REQUIRED
# ==========================================================================
log_info "[Тест 16] POST transition se-rw-2 ro→rw без confirm → 409 CONFIRMATION_REQUIRED"

RESPONSE=$(transition_mode "$SE_RW_2_URL" "$TOKEN" "rw" "false")
CODE=$(get_response_code "$RESPONSE")
BODY=$(get_response_body "$RESPONSE")

if [[ "$CODE" == "409" ]]; then
    ERR_CODE=$(echo "$BODY" | jq -r '.error.code // empty')
    if [[ "$ERR_CODE" == "CONFIRMATION_REQUIRED" ]]; then
        test_pass "Тест 16: Transition ro→rw без confirm → 409 CONFIRMATION_REQUIRED"
    else
        test_fail "Тест 16: Transition ro→rw без confirm → 409, но code=\"${ERR_CODE}\" (ожидался CONFIRMATION_REQUIRED)"
    fi
else
    test_fail "Тест 16: Transition ro→rw без confirm → HTTP ${CODE} (ожидался 409)"
fi

# ==========================================================================
# Тест 17: POST transition se-rw-2 {target_mode: "rw", confirm: true} → 200
# ==========================================================================
log_info "[Тест 17] POST transition se-rw-2 ro→rw с confirm=true → 200"

RESPONSE=$(transition_mode "$SE_RW_2_URL" "$TOKEN" "rw" "true")
CODE=$(get_response_code "$RESPONSE")
BODY=$(get_response_body "$RESPONSE")

if [[ "$CODE" == "200" ]]; then
    PREV=$(echo "$BODY" | jq -r '.previous_mode // empty')
    CURR=$(echo "$BODY" | jq -r '.current_mode // empty')
    if [[ "$CURR" == "rw" ]]; then
        test_pass "Тест 17: Transition ro→rw с confirm → 200, ${PREV}→${CURR}"
    else
        test_fail "Тест 17: Transition → 200, но current_mode=\"${CURR}\" (ожидался \"rw\")"
    fi
else
    test_fail "Тест 17: Transition ro→rw с confirm → HTTP ${CODE} (ожидался 200)"
fi

# ==========================================================================
# Тест 18: POST transition se-edit-1 {target_mode: "rw"} → 409 INVALID_TRANSITION
#
# edit → rw — недопустимый переход (edit — изолированный цикл).
# ==========================================================================
log_info "[Тест 18] POST transition se-edit-1 edit→rw → 409 INVALID_TRANSITION"

RESPONSE=$(transition_mode "$SE_EDIT_1_URL" "$TOKEN" "rw" "false")
CODE=$(get_response_code "$RESPONSE")
BODY=$(get_response_body "$RESPONSE")

if [[ "$CODE" == "409" ]]; then
    ERR_CODE=$(echo "$BODY" | jq -r '.error.code // empty')
    if [[ "$ERR_CODE" == "INVALID_TRANSITION" ]]; then
        test_pass "Тест 18: Transition edit→rw → 409 INVALID_TRANSITION"
    else
        test_fail "Тест 18: Transition edit→rw → 409, но code=\"${ERR_CODE}\" (ожидался INVALID_TRANSITION)"
    fi
else
    test_fail "Тест 18: Transition edit→rw → HTTP ${CODE} (ожидался 409)"
fi

# ==========================================================================
# Итоги
# ==========================================================================
echo ""
print_summary
