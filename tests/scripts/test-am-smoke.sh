#!/usr/bin/env bash
# ==========================================================================
# test-am-smoke.sh — Smoke-тесты Admin Module (тесты 1-3)
#
# Проверяет базовую доступность AM: health live, health ready, metrics.
# Не требует авторизации.
#
# Переменные окружения (из Makefile):
#   AM_URL       — URL Admin Module (https://localhost:18000)
#   CURL_OPTS    — опции curl
# ==========================================================================
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/lib.sh"

: "${AM_URL:=https://localhost:18000}"

log_info "=== AM Smoke Tests (1-3) ==="

# ---------- Тест 1: GET /health/live ----------
log_info "Тест 1: GET /health/live"
RESPONSE=$(http_get "$AM_URL" "" "/health/live")
CODE=$(get_response_code "$RESPONSE")
BODY=$(get_response_body "$RESPONSE")

if [[ "$CODE" == "200" ]]; then
    STATUS=$(echo "$BODY" | jq -r '.status')
    SERVICE=$(echo "$BODY" | jq -r '.service')
    if [[ "$STATUS" == "ok" && "$SERVICE" == "admin-module" ]]; then
        test_pass "Тест 1: health/live → 200, status=ok, service=admin-module"
    else
        test_fail "Тест 1: health/live → 200, но status=${STATUS}, service=${SERVICE}"
    fi
else
    test_fail "Тест 1: health/live → HTTP ${CODE} (ожидался 200)"
fi

# ---------- Тест 2: GET /health/ready ----------
log_info "Тест 2: GET /health/ready"
RESPONSE=$(http_get "$AM_URL" "" "/health/ready")
CODE=$(get_response_code "$RESPONSE")
BODY=$(get_response_body "$RESPONSE")

if [[ "$CODE" == "200" ]]; then
    STATUS=$(echo "$BODY" | jq -r '.status')
    PG_STATUS=$(echo "$BODY" | jq -r '.checks.postgresql.status')
    KC_STATUS=$(echo "$BODY" | jq -r '.checks.keycloak.status')
    if [[ "$STATUS" == "ok" && "$PG_STATUS" == "ok" && "$KC_STATUS" == "ok" ]]; then
        test_pass "Тест 2: health/ready → 200, все зависимости ok"
    else
        test_fail "Тест 2: health/ready → status=${STATUS}, pg=${PG_STATUS}, kc=${KC_STATUS}"
    fi
else
    test_fail "Тест 2: health/ready → HTTP ${CODE} (ожидался 200)"
fi

# ---------- Тест 3: GET /metrics ----------
log_info "Тест 3: GET /metrics"
RESPONSE=$(http_get "$AM_URL" "" "/metrics")
CODE=$(get_response_code "$RESPONSE")
BODY=$(get_response_body "$RESPONSE")

if [[ "$CODE" == "200" ]]; then
    # Проверяем наличие типичных Prometheus-метрик
    if echo "$BODY" | grep -q "go_goroutines"; then
        test_pass "Тест 3: metrics → 200, Prometheus-формат"
    else
        test_fail "Тест 3: metrics → 200, но не содержит go_goroutines"
    fi
else
    test_fail "Тест 3: metrics → HTTP ${CODE} (ожидался 200)"
fi

print_summary
