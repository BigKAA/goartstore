#!/usr/bin/env bash
# ==========================================================================
# test-am-ui.sh — Интеграционные тесты Admin UI (тесты 31-40)
#
# Проверяет доступность UI страниц, SSE endpoint, статические файлы,
# redirect на Keycloak login (без cookie), и корректность ответов
# для аутентифицированных запросов.
#
# Переменные окружения (из Makefile):
#   AM_URL                 — URL Admin Module (http://localhost:18000)
#   KC_TOKEN_URL           — Keycloak token endpoint
#   KC_TEST_USER_CLIENT_ID — Keycloak test user client ID
#   KC_TEST_USER_CLIENT_SECRET — Keycloak test user client secret
#   KC_ADMIN_USERNAME      — Keycloak admin user
#   KC_ADMIN_PASSWORD      — Keycloak admin password
#   KC_VIEWER_USERNAME     — Keycloak viewer user
#   KC_VIEWER_PASSWORD     — Keycloak viewer password
#   CURL_OPTS              — опции curl
# ==========================================================================
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/lib.sh"

: "${AM_URL:=http://localhost:18000}"

log_info "=== AM UI Tests (31-40) ==="

# ---------- Тест 31: Статические файлы доступны ----------
log_info "Тест 31: GET /static/js/htmx.min.js (статические файлы)"
RESPONSE_CODE=$(curl $CURL_OPTS -w "%{http_code}" -o /dev/null "${AM_URL}/static/js/htmx.min.js")

if [[ "$RESPONSE_CODE" == "200" ]]; then
    test_pass "Тест 31: /static/js/htmx.min.js → 200"
else
    test_fail "Тест 31: /static/js/htmx.min.js → HTTP ${RESPONSE_CODE} (ожидался 200)"
fi

# ---------- Тест 32: Статический CSS доступен ----------
log_info "Тест 32: GET /static/css/output.css (скомпилированный Tailwind CSS)"
RESPONSE_CODE=$(curl $CURL_OPTS -w "%{http_code}" -o /dev/null "${AM_URL}/static/css/output.css")

if [[ "$RESPONSE_CODE" == "200" ]]; then
    test_pass "Тест 32: /static/css/output.css → 200"
else
    test_fail "Тест 32: /static/css/output.css → HTTP ${RESPONSE_CODE} (ожидался 200)"
fi

# ---------- Тест 33: Статический Alpine.js доступен ----------
log_info "Тест 33: GET /static/js/alpine.min.js"
RESPONSE_CODE=$(curl $CURL_OPTS -w "%{http_code}" -o /dev/null "${AM_URL}/static/js/alpine.min.js")

if [[ "$RESPONSE_CODE" == "200" ]]; then
    test_pass "Тест 33: /static/js/alpine.min.js → 200"
else
    test_fail "Тест 33: /static/js/alpine.min.js → HTTP ${RESPONSE_CODE} (ожидался 200)"
fi

# ---------- Тест 34: Статический ApexCharts доступен ----------
log_info "Тест 34: GET /static/js/apexcharts.min.js"
RESPONSE_CODE=$(curl $CURL_OPTS -w "%{http_code}" -o /dev/null "${AM_URL}/static/js/apexcharts.min.js")

if [[ "$RESPONSE_CODE" == "200" ]]; then
    test_pass "Тест 34: /static/js/apexcharts.min.js → 200"
else
    test_fail "Тест 34: /static/js/apexcharts.min.js → HTTP ${RESPONSE_CODE} (ожидался 200)"
fi

# ---------- Тест 35: Redirect на login без cookie ----------
log_info "Тест 35: GET /admin/ без cookie → redirect на login"
# -L не следует redirect, проверяем Location header
# UI middleware redirect: /admin/ → /admin/login → Keycloak authorize
RESPONSE=$(curl $CURL_OPTS -w "\n%{http_code}" -D - -o /dev/null "${AM_URL}/admin/")
RESPONSE_CODE=$(echo "$RESPONSE" | tail -1)
LOCATION=$(echo "$RESPONSE" | grep -i "^Location:" | head -1 | tr -d '\r')

if [[ "$RESPONSE_CODE" == "302" ]] || [[ "$RESPONSE_CODE" == "303" ]]; then
    if echo "$LOCATION" | grep -q "openid-connect/auth"; then
        test_pass "Тест 35: /admin/ без cookie → ${RESPONSE_CODE} redirect на Keycloak authorize"
    elif echo "$LOCATION" | grep -q "/admin/login"; then
        test_pass "Тест 35: /admin/ без cookie → ${RESPONSE_CODE} redirect на /admin/login"
    else
        test_fail "Тест 35: /admin/ → ${RESPONSE_CODE}, но Location не указывает на Keycloak или /admin/login: ${LOCATION}"
    fi
else
    test_fail "Тест 35: /admin/ → HTTP ${RESPONSE_CODE} (ожидался 302/303 redirect)"
fi

# ---------- Тест 36: Redirect на login для dashboard ----------
log_info "Тест 36: GET /admin/storage-elements без cookie → redirect"
RESPONSE_CODE=$(curl $CURL_OPTS -w "%{http_code}" -o /dev/null "${AM_URL}/admin/storage-elements")

if [[ "$RESPONSE_CODE" == "302" ]] || [[ "$RESPONSE_CODE" == "303" ]]; then
    test_pass "Тест 36: /admin/storage-elements без cookie → ${RESPONSE_CODE} redirect"
else
    test_fail "Тест 36: /admin/storage-elements → HTTP ${RESPONSE_CODE} (ожидался 302/303)"
fi

# ---------- Тест 37: Login endpoint доступен ----------
log_info "Тест 37: GET /admin/login → redirect на Keycloak"
RESPONSE=$(curl $CURL_OPTS -w "\n%{http_code}" -D - -o /dev/null "${AM_URL}/admin/login")
RESPONSE_CODE=$(echo "$RESPONSE" | tail -1)
LOCATION=$(echo "$RESPONSE" | grep -i "^Location:" | head -1 | tr -d '\r')

if [[ "$RESPONSE_CODE" == "302" ]] || [[ "$RESPONSE_CODE" == "303" ]]; then
    if echo "$LOCATION" | grep -q "openid-connect/auth"; then
        # Проверяем наличие PKCE code_challenge
        if echo "$LOCATION" | grep -q "code_challenge"; then
            test_pass "Тест 37: /admin/login → redirect на Keycloak с PKCE"
        else
            test_fail "Тест 37: /admin/login → redirect на Keycloak, но без PKCE code_challenge"
        fi
    else
        test_fail "Тест 37: /admin/login → ${RESPONSE_CODE}, Location не указывает на Keycloak"
    fi
else
    test_fail "Тест 37: /admin/login → HTTP ${RESPONSE_CODE} (ожидался 302/303)"
fi

# ---------- Тест 38: Callback без параметров → ошибка ----------
log_info "Тест 38: GET /admin/callback без code → ошибка"
RESPONSE_CODE=$(curl $CURL_OPTS -w "%{http_code}" -o /dev/null "${AM_URL}/admin/callback")

if [[ "$RESPONSE_CODE" == "400" ]] || [[ "$RESPONSE_CODE" == "302" ]] || [[ "$RESPONSE_CODE" == "303" ]]; then
    test_pass "Тест 38: /admin/callback без code → HTTP ${RESPONSE_CODE} (ошибка или redirect)"
else
    test_fail "Тест 38: /admin/callback без code → HTTP ${RESPONSE_CODE} (ожидался 400/302/303)"
fi

# ---------- Тест 39: SSE endpoint без cookie → redirect/unauthorized ----------
log_info "Тест 39: GET /admin/events/system-status без cookie"
RESPONSE_CODE=$(curl $CURL_OPTS -w "%{http_code}" -o /dev/null --max-time 3 \
    "${AM_URL}/admin/events/system-status" 2>/dev/null) || RESPONSE_CODE="000"

if [[ "$RESPONSE_CODE" == "302" ]] || [[ "$RESPONSE_CODE" == "303" ]] || [[ "$RESPONSE_CODE" == "401" ]]; then
    test_pass "Тест 39: /admin/events/system-status без cookie → ${RESPONSE_CODE}"
else
    test_fail "Тест 39: /admin/events/system-status → HTTP ${RESPONSE_CODE} (ожидался 302/303/401)"
fi

# ---------- Тест 40: Logout endpoint доступен ----------
log_info "Тест 40: POST /admin/logout без cookie → redirect"
RESPONSE_CODE=$(curl $CURL_OPTS -X POST -w "%{http_code}" -o /dev/null "${AM_URL}/admin/logout")

if [[ "$RESPONSE_CODE" == "302" ]] || [[ "$RESPONSE_CODE" == "303" ]] || [[ "$RESPONSE_CODE" == "200" ]]; then
    test_pass "Тест 40: /admin/logout → HTTP ${RESPONSE_CODE}"
else
    test_fail "Тест 40: /admin/logout → HTTP ${RESPONSE_CODE} (ожидался 302/303/200)"
fi

print_summary
