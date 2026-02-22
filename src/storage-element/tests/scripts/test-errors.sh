#!/usr/bin/env bash
# ==========================================================================
# test-errors.sh — Тесты ошибок (тесты 27-30)
#
# Проверяет обработку ошибок аутентификации/авторизации и лимитов:
#   27. GET /files без Authorization → 401
#   28. GET /files с невалидным JWT → 401
#   29. POST upload с JWT без scope files:write → 403
#   30. POST upload файл > 10MB → 413
#
# Переменные окружения:
#   JWKS_MOCK_URL  — URL JWKS Mock (по умолчанию https://localhost:18080)
#   SE_RW_1_URL    — URL SE rw-1 (по умолчанию https://localhost:18012)
#
# Использование:
#   ./test-errors.sh
# ==========================================================================
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/lib.sh"

: "${JWKS_MOCK_URL:=https://localhost:18080}"
: "${SE_RW_1_URL:=https://localhost:18012}"

log_info "========================================================"
log_info "  Тесты ошибок (27-30)"
log_info "  JWKS Mock: ${JWKS_MOCK_URL}"
log_info "  SE RW-1: ${SE_RW_1_URL}"
log_info "========================================================"
echo ""

# ==========================================================================
# Тест 27: GET /files без Authorization → 401
# ==========================================================================
log_info "[Тест 27] GET /api/v1/files без Authorization → 401"

RESPONSE=$(http_get "$SE_RW_1_URL" "" "/api/v1/files")
CODE=$(get_response_code "$RESPONSE")
BODY=$(get_response_body "$RESPONSE")

if [[ "$CODE" == "401" ]]; then
    ERR_CODE=$(echo "$BODY" | jq -r '.error.code // empty')
    if [[ "$ERR_CODE" == "UNAUTHORIZED" ]]; then
        test_pass "Тест 27: Без JWT → 401 UNAUTHORIZED"
    else
        test_pass "Тест 27: Без JWT → 401 (code=${ERR_CODE})"
    fi
else
    test_fail "Тест 27: Без JWT → HTTP ${CODE} (ожидался 401)"
fi

# ==========================================================================
# Тест 28: GET /files с невалидным JWT → 401
# ==========================================================================
log_info "[Тест 28] GET /api/v1/files с невалидным JWT → 401"

FAKE_TOKEN="eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiJmYWtlIiwic2NvcGVzIjpbImZpbGVzOnJlYWQiXSwiZXhwIjo5OTk5OTk5OTk5fQ.invalid_signature_here"

RESPONSE=$(http_get "$SE_RW_1_URL" "$FAKE_TOKEN" "/api/v1/files")
CODE=$(get_response_code "$RESPONSE")
BODY=$(get_response_body "$RESPONSE")

if [[ "$CODE" == "401" ]]; then
    ERR_CODE=$(echo "$BODY" | jq -r '.error.code // empty')
    if [[ "$ERR_CODE" == "UNAUTHORIZED" ]]; then
        test_pass "Тест 28: Невалидный JWT → 401 UNAUTHORIZED"
    else
        test_pass "Тест 28: Невалидный JWT → 401 (code=${ERR_CODE})"
    fi
else
    test_fail "Тест 28: Невалидный JWT → HTTP ${CODE} (ожидался 401)"
fi

# ==========================================================================
# Тест 29: POST upload с JWT без scope files:write → 403
# ==========================================================================
log_info "[Тест 29] POST upload с JWT без scope files:write → 403"

# Получаем токен только с files:read (без files:write)
log_info "  Получение JWT с ограниченными scopes..."
READ_ONLY_TOKEN=$(get_token "$JWKS_MOCK_URL" "test-readonly" '["files:read"]' 3600)
if [[ -z "$READ_ONLY_TOKEN" || "$READ_ONLY_TOKEN" == "null" ]]; then
    test_fail "Тест 29: Не удалось получить read-only JWT"
else
    RESPONSE=$(upload_file "$SE_RW_1_URL" "$READ_ONLY_TOKEN" "test-forbidden.bin")
    CODE=$(get_response_code "$RESPONSE")
    BODY=$(get_response_body "$RESPONSE")

    if [[ "$CODE" == "403" ]]; then
        ERR_CODE=$(echo "$BODY" | jq -r '.error.code // empty')
        if [[ "$ERR_CODE" == "FORBIDDEN" ]]; then
            test_pass "Тест 29: Upload без files:write → 403 FORBIDDEN"
        else
            test_pass "Тест 29: Upload без files:write → 403 (code=${ERR_CODE})"
        fi
    else
        test_fail "Тест 29: Upload без files:write → HTTP ${CODE} (ожидался 403)"
    fi
fi

# ==========================================================================
# Тест 30: POST upload файл > 10MB → 413
#
# SE_MAX_FILE_SIZE = 10485760 (10MB) в тестовой среде.
# Генерируем файл ~11MB.
# ==========================================================================
log_info "[Тест 30] POST upload файл > 10MB → 413"

# Получаем полноценный токен
log_info "  Получение JWT с полными правами..."
FULL_TOKEN=$(get_token "$JWKS_MOCK_URL" "test-large" '["files:read","files:write"]' 3600)
if [[ -z "$FULL_TOKEN" || "$FULL_TOKEN" == "null" ]]; then
    test_fail "Тест 30: Не удалось получить JWT"
else
    # Генерируем файл ~11MB
    LARGE_FILE=$(mktemp)
    dd if=/dev/urandom bs=1048576 count=11 2>/dev/null > "$LARGE_FILE"

    RESPONSE=$(upload_file "$SE_RW_1_URL" "$FULL_TOKEN" "test-too-large.bin" "application/octet-stream" "" "$LARGE_FILE")
    CODE=$(get_response_code "$RESPONSE")
    BODY=$(get_response_body "$RESPONSE")

    rm -f "$LARGE_FILE"

    if [[ "$CODE" == "413" ]]; then
        ERR_CODE=$(echo "$BODY" | jq -r '.error.code // empty')
        if [[ "$ERR_CODE" == "FILE_TOO_LARGE" ]]; then
            test_pass "Тест 30: Файл > 10MB → 413 FILE_TOO_LARGE"
        else
            test_pass "Тест 30: Файл > 10MB → 413 (code=${ERR_CODE})"
        fi
    else
        test_fail "Тест 30: Файл > 10MB → HTTP ${CODE} (ожидался 413)"
    fi
fi

# ==========================================================================
# Итоги
# ==========================================================================
echo ""
print_summary
