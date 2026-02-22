#!/usr/bin/env bash
# ==========================================================================
# test-data.sh — Тесты инициализированных данных (тесты 23-24)
#
# Проверяет что init-data.sh корректно подготовил данные:
#   23. GET /files на se-ro → total >= 3
#   24. GET /files на se-ar → total == 200
#
# Переменные окружения:
#   JWKS_MOCK_URL  — URL JWKS Mock (по умолчанию https://localhost:18080)
#   SE_RO_URL      — URL SE ro (по умолчанию https://localhost:18014)
#   SE_AR_URL      — URL SE ar (по умолчанию https://localhost:18015)
#
# Использование:
#   ./test-data.sh
# ==========================================================================
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/lib.sh"

: "${JWKS_MOCK_URL:=https://localhost:18080}"
: "${SE_RO_URL:=https://localhost:18014}"
: "${SE_AR_URL:=https://localhost:18015}"

log_info "========================================================"
log_info "  Тесты инициализированных данных (23-24)"
log_info "  JWKS Mock: ${JWKS_MOCK_URL}"
log_info "  SE RO: ${SE_RO_URL}"
log_info "  SE AR: ${SE_AR_URL}"
log_info "========================================================"
echo ""

# --------------------------------------------------------------------------
# Подготовка: JWT (files:read достаточно для list)
# --------------------------------------------------------------------------
log_info "Получение JWT токена..."
TOKEN=$(get_token "$JWKS_MOCK_URL" "test-data" '["files:read"]' 3600)
if [[ -z "$TOKEN" || "$TOKEN" == "null" ]]; then
    log_fail "Не удалось получить JWT токен — прерывание"
    exit 1
fi
log_ok "JWT получен"
echo ""

# ==========================================================================
# Тест 23: GET /files на se-ro → total >= 3
#
# Init-data.sh загрузил 4 файла в se-ro.
# Проверяем что в индексе >= 3 файлов (с запасом).
# ==========================================================================
log_info "[Тест 23] GET /api/v1/files на se-ro → total >= 3"

RESPONSE=$(http_get "$SE_RO_URL" "$TOKEN" "/api/v1/files?limit=1&offset=0")
CODE=$(get_response_code "$RESPONSE")
BODY=$(get_response_body "$RESPONSE")

if [[ "$CODE" == "200" ]]; then
    TOTAL=$(echo "$BODY" | jq '.total // 0')
    if [[ "$TOTAL" -ge 3 ]]; then
        test_pass "Тест 23: se-ro содержит ${TOTAL} файлов (ожидалось >= 3)"
    else
        test_fail "Тест 23: se-ro содержит ${TOTAL} файлов (ожидалось >= 3)"
    fi
else
    test_fail "Тест 23: GET /files на se-ro → HTTP ${CODE} (ожидался 200)"
fi

# ==========================================================================
# Тест 24: GET /files на se-ar → total == 200
#
# Init-data.sh загрузил 200 файлов в se-ar.
# Физические данные удалены, но attr.json остались → индекс содержит 200 записей.
# ==========================================================================
log_info "[Тест 24] GET /api/v1/files на se-ar → total == 200"

RESPONSE=$(http_get "$SE_AR_URL" "$TOKEN" "/api/v1/files?limit=1&offset=0")
CODE=$(get_response_code "$RESPONSE")
BODY=$(get_response_body "$RESPONSE")

if [[ "$CODE" == "200" ]]; then
    TOTAL=$(echo "$BODY" | jq '.total // 0')
    if [[ "$TOTAL" -eq 200 ]]; then
        test_pass "Тест 24: se-ar содержит ${TOTAL} файлов (ожидалось 200)"
    else
        test_fail "Тест 24: se-ar содержит ${TOTAL} файлов (ожидалось 200)"
    fi
else
    test_fail "Тест 24: GET /files на se-ar → HTTP ${CODE} (ожидался 200)"
fi

# ==========================================================================
# Итоги
# ==========================================================================
echo ""
print_summary
