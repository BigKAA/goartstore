#!/usr/bin/env bash
# ==========================================================================
# test-smoke.sh — Smoke-тесты (тесты 1-3)
#
# Проверяет базовую работоспособность SE:
#   1. GET /health/live → 200, status == "ok"
#   2. GET /health/ready → 200, checks.filesystem.status == "ok"
#   3. GET /api/v1/info → 200, storage_id + mode; GET /metrics → "se_http_requests_total"
#
# Переменные окружения:
#   SE_RW_1_URL  — URL одного из SE экземпляров (по умолчанию https://localhost:18012)
#
# Использование:
#   ./test-smoke.sh
# ==========================================================================
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/lib.sh"

# URL экземпляра для smoke-тестов (standalone rw)
: "${SE_RW_1_URL:=https://localhost:18012}"

log_info "========================================================"
log_info "  Smoke-тесты (1-3)"
log_info "  SE: ${SE_RW_1_URL}"
log_info "========================================================"
echo ""

# ==========================================================================
# Тест 1: GET /health/live → 200, status == "ok"
# ==========================================================================
log_info "[Тест 1] GET /health/live → 200, status == \"ok\""

RESPONSE=$(http_get "$SE_RW_1_URL" "" "/health/live")
CODE=$(get_response_code "$RESPONSE")
BODY=$(get_response_body "$RESPONSE")

if [[ "$CODE" == "200" ]]; then
    STATUS=$(echo "$BODY" | jq -r '.status // empty')
    if [[ "$STATUS" == "ok" ]]; then
        test_pass "Тест 1: /health/live → 200, status=ok"
    else
        test_fail "Тест 1: /health/live → 200, но status=\"${STATUS}\" (ожидался \"ok\")"
    fi
else
    test_fail "Тест 1: /health/live → HTTP ${CODE} (ожидался 200)"
fi

# ==========================================================================
# Тест 2: GET /health/ready → 200, checks.filesystem.status == "ok"
# ==========================================================================
log_info "[Тест 2] GET /health/ready → 200, checks.filesystem.status == \"ok\""

RESPONSE=$(http_get "$SE_RW_1_URL" "" "/health/ready")
CODE=$(get_response_code "$RESPONSE")
BODY=$(get_response_body "$RESPONSE")

if [[ "$CODE" == "200" ]]; then
    FS_STATUS=$(echo "$BODY" | jq -r '.checks.filesystem.status // empty')
    if [[ "$FS_STATUS" == "ok" ]]; then
        test_pass "Тест 2: /health/ready → 200, filesystem=ok"
    else
        test_fail "Тест 2: /health/ready → 200, но filesystem.status=\"${FS_STATUS}\" (ожидался \"ok\")"
    fi
else
    test_fail "Тест 2: /health/ready → HTTP ${CODE} (ожидался 200)"
fi

# ==========================================================================
# Тест 3: GET /api/v1/info → 200, storage_id + mode; /metrics → se_http_requests_total
# ==========================================================================
log_info "[Тест 3] GET /api/v1/info → 200, storage_id, mode; /metrics → se_http_requests_total"

RESPONSE=$(http_get "$SE_RW_1_URL" "" "/api/v1/info")
CODE=$(get_response_code "$RESPONSE")
BODY=$(get_response_body "$RESPONSE")

TEST3_PASS=true

if [[ "$CODE" == "200" ]]; then
    STORAGE_ID=$(echo "$BODY" | jq -r '.storage_id // empty')
    MODE=$(echo "$BODY" | jq -r '.mode // empty')

    if [[ -z "$STORAGE_ID" ]]; then
        test_fail "Тест 3a: /api/v1/info → 200, но storage_id отсутствует"
        TEST3_PASS=false
    fi

    if [[ -z "$MODE" ]]; then
        test_fail "Тест 3a: /api/v1/info → 200, но mode отсутствует"
        TEST3_PASS=false
    fi

    if [[ "$TEST3_PASS" == "true" ]]; then
        log_ok "  /api/v1/info → storage_id=${STORAGE_ID}, mode=${MODE}"
    fi
else
    test_fail "Тест 3a: /api/v1/info → HTTP ${CODE} (ожидался 200)"
    TEST3_PASS=false
fi

# Проверка /metrics
METRICS_RESPONSE=$(http_get "$SE_RW_1_URL" "" "/metrics")
METRICS_CODE=$(get_response_code "$METRICS_RESPONSE")
METRICS_BODY=$(get_response_body "$METRICS_RESPONSE")

if [[ "$METRICS_CODE" == "200" ]]; then
    if echo "$METRICS_BODY" | grep -q "se_http_requests_total"; then
        log_ok "  /metrics → содержит se_http_requests_total"
    else
        test_fail "Тест 3b: /metrics → 200, но не содержит se_http_requests_total"
        TEST3_PASS=false
    fi
else
    test_fail "Тест 3b: /metrics → HTTP ${METRICS_CODE} (ожидался 200)"
    TEST3_PASS=false
fi

if [[ "$TEST3_PASS" == "true" ]]; then
    test_pass "Тест 3: /api/v1/info и /metrics корректны"
fi

# ==========================================================================
# Итоги
# ==========================================================================
echo ""
print_summary
