#!/usr/bin/env bash
# ==========================================================================
# lib.sh — общая библиотека функций для SE тестовых скриптов
#
# Используется:
#   - init-data.sh (Helm post-install Job, монтируется в /scripts/lib.sh)
#   - test-*.sh (локальный запуск через port-forward)
#
# Зависимости: bash, curl, jq
# ==========================================================================
set -euo pipefail

# Опции curl по умолчанию (self-signed TLS, тихий режим, таймаут)
: "${CURL_OPTS:=-k -s --connect-timeout 5}"

# ==========================================================================
# Логирование (цветной вывод)
# ==========================================================================

log_info() { echo -e "\033[0;34m[INFO]\033[0m  $*"; }
log_ok()   { echo -e "\033[0;32m[OK]\033[0m    $*"; }
log_fail() { echo -e "\033[0;31m[FAIL]\033[0m  $*"; }
log_warn() { echo -e "\033[0;33m[WARN]\033[0m  $*"; }

# ==========================================================================
# Счётчики тестов (для test-*.sh)
# ==========================================================================

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

# print_summary — вывести итоги тестов
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

# ==========================================================================
# Утилиты для работы с ответами
#
# Все функции, возвращающие HTTP-ответ, используют формат:
#   "<http_code> <response_body>"
# Например: "201 {\"file_id\":\"abc123\",...}"
# ==========================================================================

# get_response_code — извлечь HTTP код из ответа (первое слово первой строки)
# Использование: CODE=$(get_response_code "$RESPONSE")
get_response_code() {
    echo "$1" | head -1 | awk '{print $1}'
}

# get_response_body — извлечь тело ответа без HTTP кода
# Удаляет первое слово (HTTP код) из первой строки, остальные строки без изменений.
# Использование: BODY=$(get_response_body "$RESPONSE")
get_response_body() {
    echo "$1" | sed '1s/^[^ ]* //'
}

# ==========================================================================
# Основные функции
# ==========================================================================

# get_token — получить JWT от JWKS Mock
#
# Аргументы:
#   $1 — URL JWKS Mock (например, https://jwks-mock.se-test.svc.cluster.local:8080)
#   $2 — subject (sub claim)
#   $3 — scopes (JSON-массив строк, например '["files:read","files:write"]')
#   $4 — TTL в секундах (по умолчанию 3600)
#
# Возвращает: JWT токен через stdout
# При ошибке возвращает пустую строку
#
# Пример:
#   TOKEN=$(get_token "$JWKS_MOCK_URL" "test-user" '["files:read","files:write"]' 3600)
get_token() {
    local mock_url="$1"
    local sub="$2"
    local scopes="$3"
    local ttl="${4:-3600}"

    local request_body
    request_body=$(jq -n \
        --arg sub "$sub" \
        --argjson scopes "$scopes" \
        --argjson ttl "$ttl" \
        '{sub: $sub, scopes: $scopes, ttl_seconds: $ttl}')

    local tmpout
    tmpout=$(mktemp)
    local http_code
    http_code=$(curl $CURL_OPTS -w "%{http_code}" -o "$tmpout" \
        -X POST \
        -H "Content-Type: application/json" \
        -d "$request_body" \
        "${mock_url}/token") || true

    local body
    body=$(cat "$tmpout")
    rm -f "$tmpout"

    if [[ "$http_code" != "200" ]]; then
        log_fail "get_token: HTTP ${http_code} от ${mock_url}/token"
        echo ""
        return 1
    fi

    echo "$body" | jq -r '.token'
}

# wait_ready — дождаться готовности сервиса (GET /health/ready → 200)
#
# Аргументы:
#   $1 — базовый URL сервиса
#   $2 — таймаут в секундах (по умолчанию 120)
#
# Возвращает: 0 при успехе, 1 при таймауте
#
# Пример:
#   wait_ready "$SE_RW_1_URL" 120
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

# upload_file — загрузить файл в Storage Element
#
# Аргументы:
#   $1 — базовый URL SE
#   $2 — JWT токен
#   $3 — имя файла (filename в multipart)
#   $4 — Content-Type (по умолчанию application/octet-stream)
#   $5 — описание файла (по умолчанию пусто)
#   $6 — путь к файлу (по умолчанию пусто — генерируется случайный файл ~1KB)
#
# Возвращает: "<http_code> <response_body>"
#
# Пример:
#   RESPONSE=$(upload_file "$SE_URL" "$TOKEN" "test.bin" "application/octet-stream" "Тестовый файл")
#   assert_status "$RESPONSE" 201 "Загрузка файла"
upload_file() {
    local se_url="$1"
    local token="$2"
    local filename="$3"
    local content_type="${4:-application/octet-stream}"
    local description="${5:-}"
    local file_path="${6:-}"

    # Если путь к файлу не указан — генерируем случайный файл ~1KB
    local tmp_generated=""
    if [[ -z "$file_path" ]]; then
        file_path=$(mktemp)
        dd if=/dev/urandom bs=1024 count=1 2>/dev/null > "$file_path"
        tmp_generated="$file_path"
    fi

    local tmpout
    tmpout=$(mktemp)
    local http_code

    # Формируем curl-запрос
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

# transition_mode — сменить режим работы Storage Element
#
# Аргументы:
#   $1 — базовый URL SE
#   $2 — JWT токен
#   $3 — целевой режим (rw, ro, ar)
#   $4 — confirm (true/false, по умолчанию false; нужен для ro→rw)
#
# Возвращает: "<http_code> <response_body>"
#
# Пример:
#   RESPONSE=$(transition_mode "$SE_URL" "$TOKEN" "ro" "false")
#   assert_status "$RESPONSE" 200 "Переход в ro"
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
#
# Аргументы:
#   $1 — ответ в формате "<http_code> <body>"
#   $2 — ожидаемый HTTP код
#   $3 — описание проверки (для лога)
#
# Возвращает: 0 при совпадении, 1 при несовпадении
#
# Пример:
#   if assert_status "$RESPONSE" 201 "Upload файла"; then
#       echo "Успех"
#   fi
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

# ==========================================================================
# Вспомогательные функции для тестовых скриптов
# ==========================================================================

# http_get — выполнить GET запрос с авторизацией
#
# Аргументы:
#   $1 — базовый URL
#   $2 — JWT токен (пусто для запросов без авторизации)
#   $3 — путь (например, /api/v1/files)
#
# Возвращает: "<http_code> <response_body>"
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

# http_post — выполнить POST запрос с JSON телом
#
# Аргументы:
#   $1 — базовый URL
#   $2 — JWT токен
#   $3 — путь
#   $4 — JSON тело запроса
#
# Возвращает: "<http_code> <response_body>"
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

# http_patch — выполнить PATCH запрос с JSON телом
#
# Аргументы:
#   $1 — базовый URL
#   $2 — JWT токен
#   $3 — путь
#   $4 — JSON тело запроса
#
# Возвращает: "<http_code> <response_body>"
http_patch() {
    local base_url="$1"
    local token="$2"
    local path="$3"
    local data="$4"

    local tmpout
    tmpout=$(mktemp)
    local http_code

    http_code=$(curl $CURL_OPTS -w "%{http_code}" -o "$tmpout" \
        -X PATCH \
        -H "Authorization: Bearer ${token}" \
        -H "Content-Type: application/json" \
        -d "$data" \
        "${base_url}${path}") || http_code="000"

    local body
    body=$(cat "$tmpout")
    rm -f "$tmpout"

    echo "${http_code} ${body}"
}

# http_delete — выполнить DELETE запрос
#
# Аргументы:
#   $1 — базовый URL
#   $2 — JWT токен
#   $3 — путь
#
# Возвращает: "<http_code> <response_body>"
http_delete() {
    local base_url="$1"
    local token="$2"
    local path="$3"

    local tmpout
    tmpout=$(mktemp)
    local http_code

    http_code=$(curl $CURL_OPTS -w "%{http_code}" -o "$tmpout" \
        -X DELETE \
        -H "Authorization: Bearer ${token}" \
        "${base_url}${path}") || http_code="000"

    local body
    body=$(cat "$tmpout")
    rm -f "$tmpout"

    echo "${http_code} ${body}"
}

# http_download — выполнить GET запрос для скачивания файла
#
# Аргументы:
#   $1 — базовый URL
#   $2 — JWT токен
#   $3 — путь (например, /api/v1/files/{id}/download)
#   $4 — дополнительные заголовки (опционально, например "Range: bytes=0-99")
#
# Возвращает: "<http_code> <headers>" (тело сохраняется во временный файл)
http_download() {
    local base_url="$1"
    local token="$2"
    local path="$3"
    local extra_header="${4:-}"

    local tmpout
    tmpout=$(mktemp)
    local tmpheaders
    tmpheaders=$(mktemp)
    local http_code

    local curl_args=($CURL_OPTS -w "%{http_code}" -o "$tmpout" -D "$tmpheaders"
        -H "Authorization: Bearer ${token}")
    if [[ -n "$extra_header" ]]; then
        curl_args+=(-H "$extra_header")
    fi

    http_code=$(curl "${curl_args[@]}" "${base_url}${path}") || http_code="000"

    local headers
    headers=$(cat "$tmpheaders")
    rm -f "$tmpout" "$tmpheaders"

    echo "${http_code} ${headers}"
}
