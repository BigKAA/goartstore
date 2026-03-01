#!/usr/bin/env bash
# ==========================================================================
# test-im-health.sh — Интеграционные тесты Ingester Module: Health и Metrics
#
# Тесты 1-3: health/live, health/ready, metrics
# ==========================================================================

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/lib.sh"

# Проверка обязательных переменных
: "${IM_URL:?IM_URL не задана}"

echo ""
log_info "=========================================="
log_info "  Ingester Module: Health & Metrics (тесты 1-3)"
log_info "=========================================="
echo ""

# --------------------------------------------------------------------------
# Тест 1: GET /health/live → 200, status=ok, service=ingester-module
# --------------------------------------------------------------------------
log_info "Тест 1: GET /health/live"
response=$(http_get "$IM_URL" "" "/health/live")
code=$(get_response_code "$response")
body=$(get_response_body "$response")

if [[ "$code" == "200" ]]; then
    status=$(echo "$body" | jq -r '.status // empty')
    service=$(echo "$body" | jq -r '.service // empty')
    if [[ "$status" == "ok" && "$service" == "ingester-module" ]]; then
        test_pass "Тест 1: /health/live → 200, status=ok, service=ingester-module"
    else
        test_fail "Тест 1: /health/live → 200, но status=${status}, service=${service}"
    fi
else
    test_fail "Тест 1: /health/live → ожидался 200, получен ${code}"
fi

# --------------------------------------------------------------------------
# Тест 2: GET /health/ready → 200, checks.admin_module.status=ok
# --------------------------------------------------------------------------
log_info "Тест 2: GET /health/ready"
response=$(http_get "$IM_URL" "" "/health/ready")
code=$(get_response_code "$response")
body=$(get_response_body "$response")

if [[ "$code" == "200" ]]; then
    am_status=$(echo "$body" | jq -r '.checks.admin_module.status // empty')
    jwks_status=$(echo "$body" | jq -r '.checks.jwks.status // empty')
    if [[ "$am_status" == "ok" && "$jwks_status" == "ok" ]]; then
        test_pass "Тест 2: /health/ready → 200, admin_module.status=ok, jwks.status=ok"
    else
        test_fail "Тест 2: /health/ready → 200, но admin_module.status=${am_status}, jwks.status=${jwks_status}"
    fi
else
    test_fail "Тест 2: /health/ready → ожидался 200, получен ${code}"
fi

# --------------------------------------------------------------------------
# Тест 3: GET /metrics → 200, содержит go_goroutines и im_http_requests_total
# --------------------------------------------------------------------------
log_info "Тест 3: GET /metrics"
response=$(http_get "$IM_URL" "" "/metrics")
code=$(get_response_code "$response")
body=$(get_response_body "$response")

if [[ "$code" == "200" ]]; then
    if echo "$body" | grep -q "go_goroutines" && echo "$body" | grep -q "im_http_requests_total"; then
        test_pass "Тест 3: /metrics → 200, содержит go_goroutines и im_http_requests_total"
    elif echo "$body" | grep -q "go_goroutines"; then
        test_fail "Тест 3: /metrics → 200, содержит go_goroutines, но не содержит im_http_requests_total"
    else
        test_fail "Тест 3: /metrics → 200, но не содержит go_goroutines"
    fi
else
    test_fail "Тест 3: /metrics → ожидался 200, получен ${code}"
fi

# --------------------------------------------------------------------------
print_summary
