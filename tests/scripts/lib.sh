#!/usr/bin/env bash
# ==========================================================================
# lib.sh — общая библиотека функций для Artstore интеграционных тестов
#
# Использование: source scripts/lib.sh
#
# Переменные окружения (задаются в Makefile):
#   KC_TOKEN_URL    — Keycloak token endpoint
#   AM_URL          — Admin Module URL
#   SE_EDIT_1_URL   — SE URLs (edit-1, edit-2, rw-1, rw-2, ro, ar)
#   CURL_OPTS       — опции curl (-k -s --connect-timeout 5)
# ==========================================================================
set -euo pipefail

# Опции curl по умолчанию (self-signed TLS, тихий режим, таймаут)
: "${CURL_OPTS:=-k -s --connect-timeout 5}"

# ---- Логирование (цветной вывод) ----

log_info() { echo -e "\033[0;34m[INFO]\033[0m  $*"; }
log_ok()   { echo -e "\033[0;32m[OK]\033[0m    $*"; }
log_fail() { echo -e "\033[0;31m[FAIL]\033[0m  $*"; }
log_warn() { echo -e "\033[0;33m[WARN]\033[0m  $*"; }

# ---- Счётчики тестов ----

PASS_COUNT=0
FAIL_COUNT=0

test_pass() {
    ((PASS_COUNT++))
    log_ok "PASS: $*"
}

test_fail() {
    ((FAIL_COUNT++))
    log_fail "FAIL: $*"
}

print_summary() {
    local total=$((PASS_COUNT + FAIL_COUNT))
    echo ""
    log_info "========================================================"
    log_info "Результаты: ${PASS_COUNT} PASS / ${FAIL_COUNT} FAIL (всего ${total})"
    log_info "========================================================"
    if [[ $FAIL_COUNT -gt 0 ]]; then
        return 1
    fi
    return 0
}

# ---- Утилиты для работы с ответами ----
# Формат ответа: "<http_code> <response_body>"

get_response_code() { echo "$1" | head -1 | awk '{print $1}'; }
get_response_body() { local first_line; first_line=$(echo "$1" | head -1 | cut -d' ' -f2-); local rest; rest=$(echo "$1" | tail -n +2); if [[ -n "$rest" ]]; then printf '%s\n%s' "$first_line" "$rest"; else printf '%s' "$first_line"; fi; }

# ---- Основные функции ----

# get_token_from_keycloak — получить JWT из Keycloak через Client Credentials flow
# Аргументы: $1=token_endpoint, $2=client_id, $3=client_secret
# Возвращает: access_token через stdout
get_token_from_keycloak() {
    local endpoint="$1"
    local client_id="$2"
    local client_secret="$3"

    local tmpout
    tmpout=$(mktemp)
    local http_code
    http_code=$(curl $CURL_OPTS -w "%{http_code}" -o "$tmpout" \
        -X POST \
        -d "grant_type=client_credentials" \
        -d "client_id=${client_id}" \
        -d "client_secret=${client_secret}" \
        "${endpoint}") || true

    local body
    body=$(cat "$tmpout")
    rm -f "$tmpout"

    if [[ "$http_code" != "200" ]]; then
        log_fail "get_token_from_keycloak: HTTP ${http_code} от ${endpoint}"
        if echo "$body" | jq . >/dev/null 2>&1; then
            log_fail "  Ответ: $(echo "$body" | jq -c '.')"
        fi
        echo ""
        return 1
    fi

    echo "$body" | jq -r '.access_token'
}

# get_user_token — получить JWT пользователя из Keycloak через Resource Owner Password Credentials flow
# Аргументы: $1=token_endpoint, $2=client_id, $3=client_secret, $4=username, $5=password
# Возвращает: access_token через stdout
get_user_token() {
    local endpoint="$1"
    local client_id="$2"
    local client_secret="$3"
    local username="$4"
    local password="$5"

    local tmpout
    tmpout=$(mktemp)
    local http_code
    http_code=$(curl $CURL_OPTS -w "%{http_code}" -o "$tmpout" \
        -X POST \
        -d "grant_type=password" \
        -d "client_id=${client_id}" \
        -d "client_secret=${client_secret}" \
        -d "username=${username}" \
        -d "password=${password}" \
        "${endpoint}") || true

    local body
    body=$(cat "$tmpout")
    rm -f "$tmpout"

    if [[ "$http_code" != "200" ]]; then
        log_fail "get_user_token: HTTP ${http_code} от ${endpoint} (user=${username})"
        if echo "$body" | jq . >/dev/null 2>&1; then
            log_fail "  Ответ: $(echo "$body" | jq -c '.')"
        fi
        echo ""
        return 1
    fi

    echo "$body" | jq -r '.access_token'
}

# wait_ready — дождаться готовности сервиса (GET /health/ready → 200)
# Аргументы: $1=url, $2=timeout(секунды)
wait_ready() {
    local url="$1"
    local timeout="${2:-120}"
    local elapsed=0
    local interval=2

    while [[ $elapsed -lt $timeout ]]; do
        local http_code
        http_code=$(curl $CURL_OPTS -o /dev/null -w "%{http_code}" \
            "${url}/health/ready" 2>/dev/null) || http_code="000"

        if [[ "$http_code" == "200" ]]; then
            return 0
        fi

        sleep $interval
        elapsed=$((elapsed + interval))
    done

    return 1
}

# upload_file — загрузить файл в SE (multipart/form-data)
# Аргументы: $1=se_url, $2=token, $3=filename, $4=content_type, $5=description, $6=file_path
# Если $6 не указан — генерирует случайный файл ~1KB
# Возвращает: "<http_code> <response_body>"
upload_file() {
    local se_url="$1"
    local token="$2"
    local filename="$3"
    local content_type="${4:-application/octet-stream}"
    local description="${5:-}"
    local file_path="${6:-}"

    local tmp_generated=""
    if [[ -z "$file_path" ]]; then
        file_path=$(mktemp)
        dd if=/dev/urandom bs=1024 count=1 2>/dev/null > "$file_path"
        tmp_generated="$file_path"
    fi

    local tmpout
    tmpout=$(mktemp)
    local http_code

    local curl_args=($CURL_OPTS -w "%{http_code}" -o "$tmpout"
        -X POST
        -H "Authorization: Bearer ${token}"
        -F "file=@${file_path};filename=${filename};type=${content_type}")

    if [[ -n "$description" ]]; then
        curl_args+=(-F "description=${description}")
    fi

    http_code=$(curl "${curl_args[@]}" "${se_url}/api/v1/files/upload") || http_code="000"

    local body
    body=$(cat "$tmpout")
    rm -f "$tmpout"
    [[ -n "$tmp_generated" ]] && rm -f "$tmp_generated"

    echo "${http_code} ${body}"
}

# transition_mode — сменить режим SE
# Аргументы: $1=se_url, $2=token, $3=target_mode, $4=confirm(true/false)
# Возвращает: "<http_code> <response_body>"
transition_mode() {
    local se_url="$1"
    local token="$2"
    local target_mode="$3"
    local confirm="${4:-false}"

    local request_body
    request_body=$(jq -n \
        --arg mode "$target_mode" \
        --argjson confirm "$confirm" \
        '{target_mode: $mode, confirm: $confirm}')

    local tmpout
    tmpout=$(mktemp)
    local http_code
    http_code=$(curl $CURL_OPTS -w "%{http_code}" -o "$tmpout" \
        -X POST \
        -H "Authorization: Bearer ${token}" \
        -H "Content-Type: application/json" \
        -d "$request_body" \
        "${se_url}/api/v1/mode/transition") || http_code="000"

    local body
    body=$(cat "$tmpout")
    rm -f "$tmpout"

    echo "${http_code} ${body}"
}

# assert_status — проверить HTTP статус ответа
# Аргументы: $1=response, $2=expected_code, $3=message
assert_status() {
    local response="$1"
    local expected="$2"
    local message="${3:-}"

    local actual
    actual=$(get_response_code "$response")

    if [[ "$actual" == "$expected" ]]; then
        return 0
    else
        if [[ -n "$message" ]]; then
            log_fail "${message}: ожидался HTTP ${expected}, получен ${actual}"
        fi
        local body
        body=$(get_response_body "$response")
        if [[ -n "$body" ]] && echo "$body" | jq . >/dev/null 2>&1; then
            log_fail "  Ответ: $(echo "$body" | jq -c '.error // .')"
        fi
        return 1
    fi
}

# http_get — GET запрос с авторизацией
# Аргументы: $1=base_url, $2=token, $3=path
http_get() {
    local base_url="$1"
    local token="$2"
    local path="$3"

    local tmpout
    tmpout=$(mktemp)
    local http_code

    local curl_args=($CURL_OPTS -w "%{http_code}" -o "$tmpout")
    if [[ -n "$token" ]]; then
        curl_args+=(-H "Authorization: Bearer ${token}")
    fi

    http_code=$(curl "${curl_args[@]}" "${base_url}${path}") || http_code="000"

    local body
    body=$(cat "$tmpout")
    rm -f "$tmpout"

    echo "${http_code} ${body}"
}

# http_post — POST запрос с JSON телом
# Аргументы: $1=base_url, $2=token, $3=path, $4=json_data
http_post() {
    local base_url="$1"
    local token="$2"
    local path="$3"
    local data="$4"

    local tmpout
    tmpout=$(mktemp)
    local http_code

    local curl_args=($CURL_OPTS -w "%{http_code}" -o "$tmpout"
        -X POST
        -H "Content-Type: application/json")
    if [[ -n "$token" ]]; then
        curl_args+=(-H "Authorization: Bearer ${token}")
    fi
    if [[ -n "$data" ]]; then
        curl_args+=(-d "$data")
    fi

    http_code=$(curl "${curl_args[@]}" "${base_url}${path}") || http_code="000"

    local body
    body=$(cat "$tmpout")
    rm -f "$tmpout"

    echo "${http_code} ${body}"
}

# http_patch — PATCH запрос с JSON телом
# Аргументы: $1=base_url, $2=token, $3=path, $4=json_data
http_patch() {
    local base_url="$1"
    local token="$2"
    local path="$3"
    local data="$4"

    local tmpout
    tmpout=$(mktemp)
    local http_code

    local curl_args=($CURL_OPTS -w "%{http_code}" -o "$tmpout"
        -X PATCH
        -H "Content-Type: application/json")
    if [[ -n "$token" ]]; then
        curl_args+=(-H "Authorization: Bearer ${token}")
    fi
    if [[ -n "$data" ]]; then
        curl_args+=(-d "$data")
    fi

    http_code=$(curl "${curl_args[@]}" "${base_url}${path}") || http_code="000"

    local body
    body=$(cat "$tmpout")
    rm -f "$tmpout"

    echo "${http_code} ${body}"
}

# http_put — PUT запрос с JSON телом
# Аргументы: $1=base_url, $2=token, $3=path, $4=json_data
http_put() {
    local base_url="$1"
    local token="$2"
    local path="$3"
    local data="$4"

    local tmpout
    tmpout=$(mktemp)
    local http_code

    local curl_args=($CURL_OPTS -w "%{http_code}" -o "$tmpout"
        -X PUT
        -H "Content-Type: application/json")
    if [[ -n "$token" ]]; then
        curl_args+=(-H "Authorization: Bearer ${token}")
    fi
    if [[ -n "$data" ]]; then
        curl_args+=(-d "$data")
    fi

    http_code=$(curl "${curl_args[@]}" "${base_url}${path}") || http_code="000"

    local body
    body=$(cat "$tmpout")
    rm -f "$tmpout"

    echo "${http_code} ${body}"
}

# http_delete — DELETE запрос с авторизацией
# Аргументы: $1=base_url, $2=token, $3=path
http_delete() {
    local base_url="$1"
    local token="$2"
    local path="$3"

    local tmpout
    tmpout=$(mktemp)
    local http_code

    local curl_args=($CURL_OPTS -w "%{http_code}" -o "$tmpout" -X DELETE)
    if [[ -n "$token" ]]; then
        curl_args+=(-H "Authorization: Bearer ${token}")
    fi

    http_code=$(curl "${curl_args[@]}" "${base_url}${path}") || http_code="000"

    local body
    body=$(cat "$tmpout")
    rm -f "$tmpout"

    echo "${http_code} ${body}"
}
