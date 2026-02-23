#!/usr/bin/env bash
# ==========================================================================
# test-am-files.sh — Тесты Files Registry (тесты 21-24)
#
# Проверяет операции с файловым реестром: register, list, update metadata,
# soft delete. Использует SE, зарегистрированный в test-am-storage-elements.sh.
#
# Переменные окружения (из Makefile):
#   AM_URL, KC_TOKEN_URL, KC_TEST_USER_CLIENT_ID,
#   KC_TEST_USER_CLIENT_SECRET, KC_ADMIN_USERNAME, KC_ADMIN_PASSWORD
# ==========================================================================
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/lib.sh"

: "${AM_URL:=http://localhost:18000}"
: "${KC_TOKEN_URL:=https://localhost:18080/realms/artstore/protocol/openid-connect/token}"
: "${KC_TEST_USER_CLIENT_ID:=artstore-test-user}"
: "${KC_TEST_USER_CLIENT_SECRET:=test-user-secret}"
: "${KC_ADMIN_USERNAME:=admin}"
: "${KC_ADMIN_PASSWORD:=admin}"

log_info "=== AM Files Registry Tests (21-24) ==="

# Получаем токен admin
log_info "Получение токена admin..."
ADMIN_TOKEN=$(get_user_token "$KC_TOKEN_URL" "$KC_TEST_USER_CLIENT_ID" \
    "$KC_TEST_USER_CLIENT_SECRET" "$KC_ADMIN_USERNAME" "$KC_ADMIN_PASSWORD")
if [[ -z "$ADMIN_TOKEN" ]]; then
    log_fail "Не удалось получить токен admin"
    print_summary
    exit 1
fi
log_ok "Токен admin получен"

# Находим зарегистрированный SE для привязки файла
log_info "Поиск зарегистрированного SE..."
RESPONSE=$(http_get "$AM_URL" "$ADMIN_TOKEN" "/api/v1/storage-elements?limit=1")
SE_BODY=$(get_response_body "$RESPONSE")
SE_ID=$(echo "$SE_BODY" | jq -r '.items[0].id // empty')
if [[ -z "$SE_ID" ]]; then
    log_fail "Нет зарегистрированных SE — тесты файлов невозможны"
    log_fail "Запустите сначала test-am-storage-elements.sh"
    print_summary
    exit 1
fi
log_ok "Используем SE: ${SE_ID}"

# Генерируем уникальный file_id
FILE_ID=$(python3 -c "import uuid; print(uuid.uuid4())" 2>/dev/null || uuidgen | tr '[:upper:]' '[:lower:]')

# ---------- Тест 21: POST /api/v1/files — регистрация файла ----------
log_info "Тест 21: POST /api/v1/files (регистрация файла)"
RESPONSE=$(http_post "$AM_URL" "$ADMIN_TOKEN" "/api/v1/files" \
    "{
        \"file_id\": \"${FILE_ID}\",
        \"original_filename\": \"test-photo.jpg\",
        \"content_type\": \"image/jpeg\",
        \"size\": 5242880,
        \"checksum\": \"sha256:abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890\",
        \"storage_element_id\": \"${SE_ID}\",
        \"description\": \"Тестовый файл для интеграционных тестов\",
        \"tags\": [\"test\", \"integration\"],
        \"retention_policy\": \"permanent\"
    }")
CODE=$(get_response_code "$RESPONSE")
BODY=$(get_response_body "$RESPONSE")

if [[ "$CODE" == "201" ]]; then
    REGISTERED_ID=$(echo "$BODY" | jq -r '.file_id')
    STATUS=$(echo "$BODY" | jq -r '.status')
    FILENAME=$(echo "$BODY" | jq -r '.original_filename')
    UPLOADED_BY=$(echo "$BODY" | jq -r '.uploaded_by')
    if [[ "$REGISTERED_ID" == "$FILE_ID" && "$STATUS" == "active" && "$FILENAME" == "test-photo.jpg" ]]; then
        test_pass "Тест 21: файл зарегистрирован, file_id=${FILE_ID}, status=active, uploaded_by=${UPLOADED_BY}"
    else
        test_fail "Тест 21: файл зарегистрирован, но данные некорректны: id=${REGISTERED_ID}, status=${STATUS}"
    fi
else
    test_fail "Тест 21: register file → HTTP ${CODE} (ожидался 201)"
    if echo "$BODY" | jq . >/dev/null 2>&1; then
        log_fail "  Ответ: $(echo "$BODY" | jq -c '.')"
    fi
fi

# ---------- Тест 22: GET /api/v1/files — список файлов ----------
log_info "Тест 22: GET /api/v1/files (список)"
RESPONSE=$(http_get "$AM_URL" "$ADMIN_TOKEN" "/api/v1/files")
CODE=$(get_response_code "$RESPONSE")
BODY=$(get_response_body "$RESPONSE")

if [[ "$CODE" == "200" ]]; then
    TOTAL=$(echo "$BODY" | jq -r '.total')
    # Проверяем, что наш файл есть в списке
    FOUND=$(echo "$BODY" | jq -r --arg fid "$FILE_ID" '.items[] | select(.file_id == $fid) | .file_id')
    if [[ "$TOTAL" -ge 1 && "$FOUND" == "$FILE_ID" ]]; then
        test_pass "Тест 22: files list → total=${TOTAL}, наш файл найден"
    else
        test_fail "Тест 22: files list → total=${TOTAL}, файл не найден"
    fi
else
    test_fail "Тест 22: files list → HTTP ${CODE} (ожидался 200)"
fi

# ---------- Тест 23: PUT /api/v1/files/{file_id} — обновление метаданных ----------
log_info "Тест 23: PUT /api/v1/files/${FILE_ID} (обновление метаданных)"
RESPONSE=$(http_put "$AM_URL" "$ADMIN_TOKEN" "/api/v1/files/${FILE_ID}" \
    '{"description": "Обновлённое описание", "tags": ["test", "updated"]}')
CODE=$(get_response_code "$RESPONSE")
BODY=$(get_response_body "$RESPONSE")

if [[ "$CODE" == "200" ]]; then
    NEW_DESC=$(echo "$BODY" | jq -r '.description')
    HAS_UPDATED_TAG=$(echo "$BODY" | jq '.tags | index("updated") != null')
    if [[ "$NEW_DESC" == "Обновлённое описание" && "$HAS_UPDATED_TAG" == "true" ]]; then
        test_pass "Тест 23: файл обновлён, description и tags корректны"
    else
        test_fail "Тест 23: файл обновлён, но desc=${NEW_DESC}, has_updated=${HAS_UPDATED_TAG}"
    fi
else
    test_fail "Тест 23: file update → HTTP ${CODE} (ожидался 200)"
fi

# ---------- Тест 24: DELETE /api/v1/files/{file_id} — soft delete ----------
log_info "Тест 24: DELETE /api/v1/files/${FILE_ID} (soft delete)"
RESPONSE=$(http_delete "$AM_URL" "$ADMIN_TOKEN" "/api/v1/files/${FILE_ID}")
CODE=$(get_response_code "$RESPONSE")

if [[ "$CODE" == "204" ]]; then
    # Проверяем, что статус стал deleted
    RESPONSE2=$(http_get "$AM_URL" "$ADMIN_TOKEN" "/api/v1/files/${FILE_ID}")
    CODE2=$(get_response_code "$RESPONSE2")
    BODY2=$(get_response_body "$RESPONSE2")
    if [[ "$CODE2" == "200" ]]; then
        FILE_STATUS=$(echo "$BODY2" | jq -r '.status')
        if [[ "$FILE_STATUS" == "deleted" ]]; then
            test_pass "Тест 24: файл soft deleted, status=deleted"
        else
            test_fail "Тест 24: файл после delete → status=${FILE_STATUS} (ожидался deleted)"
        fi
    elif [[ "$CODE2" == "404" ]]; then
        # Некоторые реализации скрывают удалённые файлы
        test_pass "Тест 24: файл soft deleted (GET → 404)"
    else
        test_fail "Тест 24: после delete GET → HTTP ${CODE2}"
    fi
else
    test_fail "Тест 24: delete file → HTTP ${CODE} (ожидался 204)"
fi

print_summary
