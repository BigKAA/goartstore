#!/usr/bin/env bash
# ==========================================================================
# test-qm-health.sh — Интеграционные тесты Query Module: Health и Metrics
#
# Тесты 1-3: health/live, health/ready, metrics
# ==========================================================================

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/lib.sh"

# Проверка обязательных переменных
: "${QM_URL:?QM_URL не задана}"

echo ""
log_info "=========================================="
log_info "  Query Module: Health & Metrics (тесты 1-3)"
log_info "=========================================="
echo ""

# --------------------------------------------------------------------------
# Тест 1: GET /health/live → 200, status=ok, service=query-module
# --------------------------------------------------------------------------
log_info "Тест 1: GET /health/live"
response=$(http_get "$QM_URL" "" "/health/live")
code=$(get_response_code "$response")
body=$(get_response_body "$response")

if [[ "$code" == "200" ]]; then
    status=$(echo "$body" | jq -r '.status // empty')
    service=$(echo "$body" | jq -r '.service // empty')
    if [[ "$status" == "ok" && "$service" == "query-module" ]]; then
        test_pass "Тест 1: /health/live → 200, status=ok, service=query-module"
    else
        test_fail "Тест 1: /health/live → 200, но status=${status}, service=${service}"
    fi
else
    test_fail "Тест 1: /health/live → ожидался 200, получен ${code}"
fi

# --------------------------------------------------------------------------
# Тест 2: GET /health/ready → 200, checks.postgresql.status=ok
# --------------------------------------------------------------------------
log_info "Тест 2: GET /health/ready"
response=$(http_get "$QM_URL" "" "/health/ready")
code=$(get_response_code "$response")
body=$(get_response_body "$response")

if [[ "$code" == "200" ]]; then
    pg_status=$(echo "$body" | jq -r '.checks.postgresql.status // empty')
    if [[ "$pg_status" == "ok" ]]; then
        test_pass "Тест 2: /health/ready → 200, postgresql.status=ok"
    else
        test_fail "Тест 2: /health/ready → 200, но postgresql.status=${pg_status}"
    fi
else
    test_fail "Тест 2: /health/ready → ожидался 200, получен ${code}"
fi

# --------------------------------------------------------------------------
# Тест 3: GET /metrics → 200, содержит go_goroutines
# --------------------------------------------------------------------------
log_info "Тест 3: GET /metrics"
response=$(http_get "$QM_URL" "" "/metrics")
code=$(get_response_code "$response")
body=$(get_response_body "$response")

if [[ "$code" == "200" ]]; then
    if echo "$body" | grep -q "go_goroutines"; then
        test_pass "Тест 3: /metrics → 200, содержит go_goroutines"
    else
        test_fail "Тест 3: /metrics → 200, но не содержит go_goroutines"
    fi
else
    test_fail "Тест 3: /metrics → ожидался 200, получен ${code}"
fi

# --------------------------------------------------------------------------
print_summary
