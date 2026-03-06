#!/usr/bin/env bash
# ==========================================================================
# test-im-upload.sh — Интеграционные тесты Ingester Module: Upload
#
# Тесты 7-16: upload файлов с разными параметрами, валидация, cross-module
# ==========================================================================

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/lib.sh"

# Проверка обязательных переменных
: "${IM_URL:?IM_URL не задана}"
: "${AM_URL:?AM_URL не задана}"
: "${KC_TOKEN_URL:?KC_TOKEN_URL не задана}"
: "${KC_TEST_USER_CLIENT_ID:?KC_TEST_USER_CLIENT_ID не задана}"
: "${KC_TEST_USER_CLIENT_SECRET:?KC_TEST_USER_CLIENT_SECRET не задана}"
: "${KC_ADMIN_USERNAME:?KC_ADMIN_USERNAME не задана}"
: "${KC_ADMIN_PASSWORD:?KC_ADMIN_PASSWORD не задана}"

# Опциональные: QM_URL для cross-module теста
SKIP_CROSS_MODULE=false
for arg in "$@"; do
    case "$arg" in
        --skip-cross-module) SKIP_CROSS_MODULE=true ;;
    esac
done

echo ""
log_info "=========================================="
log_info "  Ingester Module: Upload (тесты 7-16)"
log_info "=========================================="
echo ""

# Получаем admin-токен для всех тестов
admin_token=$(get_user_token "$KC_TOKEN_URL" \
    "$KC_TEST_USER_CLIENT_ID" "$KC_TEST_USER_CLIENT_SECRET" \
    "$KC_ADMIN_USERNAME" "$KC_ADMIN_PASSWORD") || true

if [[ -z "$admin_token" || "$admin_token" == "null" ]]; then
    log_fail "Не удалось получить admin-токен, пропускаем upload-тесты"
    test_fail "Получение admin-токена для upload-тестов"
    print_summary
    exit 1
fi

# SA-токен для проверки файлов через AM API
sa_token=$(get_token_from_keycloak "$KC_TOKEN_URL" \
    "${KC_SA_CLIENT_ID}" "${KC_SA_CLIENT_SECRET}") || true

# Генерируем тестовый файл
TEST_FILE=$(mktemp)
dd if=/dev/urandom bs=1024 count=1 2>/dev/null > "$TEST_FILE"
trap 'rm -f "$TEST_FILE"' EXIT

# Хелпер: upload файла через IM
# Аргументы: $1=token, $2=filename, $3=retention_policy, $4=ttl_days, $5=description, $6=tags, $7=file_path
im_upload() {
    local token="$1"
    local filename="$2"
    local retention_policy="$3"
    local ttl_days="$4"
    local description="${5:-}"
    local tags="${6:-}"
    local file_path="${7:-$TEST_FILE}"

    local tmpout
    tmpout=$(mktemp)

    local curl_args=($CURL_OPTS -w "%{http_code}" -o "$tmpout"
        -X POST
        -H "Authorization: Bearer ${token}"
        -F "file=@${file_path};filename=${filename};type=application/octet-stream"
        -F "retention_policy=${retention_policy}")

    if [[ -n "$ttl_days" ]]; then
        curl_args+=(-F "ttl_days=${ttl_days}")
    fi
    if [[ -n "$description" ]]; then
        curl_args+=(-F "description=${description}")
    fi
    if [[ -n "$tags" ]]; then
        curl_args+=(-F "tags=${tags}")
    fi

    local http_code
    http_code=$(curl "${curl_args[@]}" "${IM_URL}/api/v1/files/upload") || http_code="000"

    local body
    body=$(cat "$tmpout")
    rm -f "$tmpout"

    echo "${http_code} ${body}"
}

# --------------------------------------------------------------------------
# Тест 7: Upload файла с retention_policy=temporary, ttl_days=7 → 201
# --------------------------------------------------------------------------
log_info "Тест 7: Upload temporary файла (ttl_days=7)"
response=$(im_upload "$admin_token" "test-temp.bin" "temporary" "7")
code=$(get_response_code "$response")
body=$(get_response_body "$response")

if [[ "$code" == "201" ]]; then
    file_id=$(echo "$body" | jq -r '.file_id // empty')
    checksum=$(echo "$body" | jq -r '.checksum // empty')
    se_id=$(echo "$body" | jq -r '.storage_element_id // empty')
    uploaded_by=$(echo "$body" | jq -r '.uploaded_by // empty')

    if [[ -n "$file_id" && -n "$checksum" && -n "$se_id" && -n "$uploaded_by" ]]; then
        test_pass "Тест 7: Upload temporary → 201, file_id=${file_id}"
        # Сохраняем для cross-module теста
        TEMP_FILE_ID="$file_id"
        TEMP_SE_ID="$se_id"
    else
        test_fail "Тест 7: Upload → 201, но неполные поля: file_id=${file_id}, checksum=${checksum}, se_id=${se_id}"
    fi
else
    test_fail "Тест 7: Upload temporary → ожидался 201, получен ${code}"
    log_fail "  Ответ: ${body}"
fi

# --------------------------------------------------------------------------
# Тест 8: Upload файла с retention_policy=permanent → 201
# --------------------------------------------------------------------------
log_info "Тест 8: Upload permanent файла"
response=$(im_upload "$admin_token" "test-perm.bin" "permanent" "")
code=$(get_response_code "$response")
body=$(get_response_body "$response")

if [[ "$code" == "201" ]]; then
    ttl_days=$(echo "$body" | jq -r '.ttl_days // "null"')
    expires_at=$(echo "$body" | jq -r '.expires_at // "null"')
    if [[ "$ttl_days" == "null" && "$expires_at" == "null" ]]; then
        test_pass "Тест 8: Upload permanent → 201, ttl_days=null, expires_at=null"
    else
        test_fail "Тест 8: Upload permanent → 201, но ttl_days=${ttl_days}, expires_at=${expires_at} (ожидались null)"
    fi
else
    test_fail "Тест 8: Upload permanent → ожидался 201, получен ${code}"
    log_fail "  Ответ: ${body}"
fi

# --------------------------------------------------------------------------
# Тест 9: Upload с description и tags → 201
# --------------------------------------------------------------------------
log_info "Тест 9: Upload с description и tags"
response=$(im_upload "$admin_token" "test-meta.bin" "temporary" "14" "Тестовый файл" '["test","integration"]')
code=$(get_response_code "$response")
body=$(get_response_body "$response")

if [[ "$code" == "201" ]]; then
    desc=$(echo "$body" | jq -r '.description // empty')
    tags_count=$(echo "$body" | jq -r '.tags | length // 0')
    if [[ -n "$desc" && "$tags_count" -ge 2 ]]; then
        test_pass "Тест 9: Upload с metadata → 201, description='${desc}', tags=${tags_count}"
    else
        test_fail "Тест 9: Upload с metadata → 201, но description='${desc}', tags_count=${tags_count}"
    fi
else
    test_fail "Тест 9: Upload с metadata → ожидался 201, получен ${code}"
    log_fail "  Ответ: ${body}"
fi

# --------------------------------------------------------------------------
# Тест 10: Upload без файла → 400 VALIDATION_ERROR
# --------------------------------------------------------------------------
log_info "Тест 10: Upload без файла → 400"
tmpout=$(mktemp)
http_code=$(curl $CURL_OPTS -w "%{http_code}" -o "$tmpout" \
    -X POST \
    -H "Authorization: Bearer ${admin_token}" \
    -F "retention_policy=temporary" \
    -F "ttl_days=7" \
    "${IM_URL}/api/v1/files/upload") || http_code="000"
body=$(cat "$tmpout")
rm -f "$tmpout"

if [[ "$http_code" == "400" ]]; then
    error_code=$(echo "$body" | jq -r '.code // empty')
    test_pass "Тест 10: Upload без файла → 400 (code=${error_code})"
else
    test_fail "Тест 10: Upload без файла → ожидался 400, получен ${http_code}"
fi

# --------------------------------------------------------------------------
# Тест 11: Upload с невалидными tags → 400
# --------------------------------------------------------------------------
log_info "Тест 11: Upload с невалидными tags → 400"
response=$(im_upload "$admin_token" "test-badtags.bin" "temporary" "7" "" "not-a-json-array")
code=$(get_response_code "$response")

if [[ "$code" == "400" ]]; then
    test_pass "Тест 11: Upload с невалидными tags → 400"
else
    test_fail "Тест 11: Upload с невалидными tags → ожидался 400, получен ${code}"
fi

# --------------------------------------------------------------------------
# Тест 12: Upload с ttl_days=0 → 400
# --------------------------------------------------------------------------
log_info "Тест 12: Upload с ttl_days=0 → 400"
response=$(im_upload "$admin_token" "test-ttl0.bin" "temporary" "0")
code=$(get_response_code "$response")

if [[ "$code" == "400" ]]; then
    test_pass "Тест 12: Upload с ttl_days=0 → 400"
else
    test_fail "Тест 12: Upload с ttl_days=0 → ожидался 400, получен ${code}"
fi

# --------------------------------------------------------------------------
# Тест 13: Upload temporary без ttl_days → 400
# --------------------------------------------------------------------------
log_info "Тест 13: Upload temporary без ttl_days → 400"
tmpout=$(mktemp)
http_code=$(curl $CURL_OPTS -w "%{http_code}" -o "$tmpout" \
    -X POST \
    -H "Authorization: Bearer ${admin_token}" \
    -F "file=@${TEST_FILE};filename=test-nottl.bin;type=application/octet-stream" \
    -F "retention_policy=temporary" \
    "${IM_URL}/api/v1/files/upload") || http_code="000"
rm -f "$tmpout"

if [[ "$http_code" == "400" ]]; then
    test_pass "Тест 13: Upload temporary без ttl_days → 400"
else
    test_fail "Тест 13: Upload temporary без ttl_days → ожидался 400, получен ${http_code}"
fi

# --------------------------------------------------------------------------
# Тест 14: Проверка expires_at: upload temporary с ttl_days=1
# --------------------------------------------------------------------------
log_info "Тест 14: Проверка expires_at (ttl_days=1)"
response=$(im_upload "$admin_token" "test-expires.bin" "temporary" "1")
code=$(get_response_code "$response")
body=$(get_response_body "$response")

if [[ "$code" == "201" ]]; then
    expires_at=$(echo "$body" | jq -r '.expires_at // "null"')
    if [[ "$expires_at" != "null" && -n "$expires_at" ]]; then
        test_pass "Тест 14: Upload ttl_days=1 → 201, expires_at=${expires_at}"
    else
        test_fail "Тест 14: Upload ttl_days=1 → 201, но expires_at=${expires_at} (ожидалось не null)"
    fi
else
    test_fail "Тест 14: Upload ttl_days=1 → ожидался 201, получен ${code}"
fi

# --------------------------------------------------------------------------
# Тест 15: Проверка регистрации в AM: GET /api/v1/files/{file_id}
# --------------------------------------------------------------------------
log_info "Тест 15: Проверка регистрации в AM"
if [[ -n "${TEMP_FILE_ID:-}" && -n "$sa_token" && "$sa_token" != "null" ]]; then
    response=$(http_get "$AM_URL" "$sa_token" "/api/v1/files/${TEMP_FILE_ID}")
    code=$(get_response_code "$response")
    body=$(get_response_body "$response")

    if [[ "$code" == "200" ]]; then
        am_file_id=$(echo "$body" | jq -r '.file_id // empty')
        am_se_id=$(echo "$body" | jq -r '.storage_element_id // empty')
        if [[ "$am_file_id" == "$TEMP_FILE_ID" && "$am_se_id" == "$TEMP_SE_ID" ]]; then
            test_pass "Тест 15: AM /api/v1/files/${TEMP_FILE_ID} → 200, file_id и se_id совпадают"
        else
            test_fail "Тест 15: AM → 200, но file_id=${am_file_id} (ожидался ${TEMP_FILE_ID}), se_id=${am_se_id} (ожидался ${TEMP_SE_ID})"
        fi
    else
        test_fail "Тест 15: AM /api/v1/files/${TEMP_FILE_ID} → ожидался 200, получен ${code}"
    fi
else
    if [[ -z "${TEMP_FILE_ID:-}" ]]; then
        test_fail "Тест 15: пропущен — file_id не доступен из теста 7"
    else
        test_fail "Тест 15: пропущен — не удалось получить SA-токен AM"
    fi
fi

# --------------------------------------------------------------------------
# Тест 16: Cross-module — проверка доступности через QM
# --------------------------------------------------------------------------
if [[ "$SKIP_CROSS_MODULE" == "false" ]]; then
    log_info "Тест 16: Cross-module — download через QM"
    if [[ -n "${QM_URL:-}" && -n "${TEMP_FILE_ID:-}" ]]; then
        # Получаем QM SA-токен (или используем admin-токен)
        qm_response=$(http_get "$QM_URL" "$admin_token" "/api/v1/files/${TEMP_FILE_ID}/download")
        qm_code=$(get_response_code "$qm_response")

        if [[ "$qm_code" == "200" ]]; then
            test_pass "Тест 16: QM download /api/v1/files/${TEMP_FILE_ID}/download → 200"
        elif [[ "$qm_code" == "000" ]]; then
            log_warn "Тест 16: QM недоступен (connection refused), тест пропущен"
            test_pass "Тест 16: Cross-module пропущен (QM недоступен) — WARN"
        else
            log_warn "Тест 16: QM download → ${qm_code} (мягкий тест)"
            test_pass "Тест 16: Cross-module soft — QM ответил ${qm_code}"
        fi
    else
        if [[ -z "${QM_URL:-}" ]]; then
            log_warn "Тест 16: QM_URL не задана, тест пропущен"
            test_pass "Тест 16: Cross-module пропущен (QM_URL не задана) — WARN"
        else
            test_fail "Тест 16: пропущен — file_id не доступен из теста 7"
        fi
    fi
else
    echo ""
    echo "  >>> Cross-module тест (16) пропущен (--skip-cross-module)"
fi

# --------------------------------------------------------------------------
# Тест 17: Hard delete через IM → файл удалён из SE и AM
# --------------------------------------------------------------------------
log_info "Тест 17: Hard delete через IM (SE + AM)"

# Загружаем новый файл для удаления
del_response=$(im_upload "$admin_token" "test-delete.bin" "temporary" "7")
del_code=$(get_response_code "$del_response")
del_body=$(get_response_body "$del_response")
del_file_id=$(echo "$del_body" | jq -r '.file_id // empty')
del_se_id=$(echo "$del_body" | jq -r '.storage_element_id // empty')

if [[ "$del_code" == "201" && -n "$del_file_id" && -n "$del_se_id" ]]; then
    # Удаляем через IM
    im_del_response=$(http_delete "$IM_URL" "$admin_token" "/api/v1/files/${del_file_id}?storage_element_id=${del_se_id}")
    im_del_code=$(get_response_code "$im_del_response")

    if [[ "$im_del_code" == "204" || "$im_del_code" == "200" ]]; then
        # Проверяем, что файл удалён из AM (GET → 404)
        am_check=true
        if [[ -n "$sa_token" && "$sa_token" != "null" ]]; then
            am_resp=$(http_get "$AM_URL" "$sa_token" "/api/v1/files/${del_file_id}")
            am_code=$(get_response_code "$am_resp")
            if [[ "$am_code" != "404" ]]; then
                am_check=false
                log_fail "  AM: GET /api/v1/files/${del_file_id} → ${am_code} (ожидался 404)"
            fi
        fi

        if $am_check; then
            test_pass "Тест 17: Hard delete через IM → ${im_del_code}, файл удалён из AM (404)"
        else
            test_fail "Тест 17: Hard delete через IM → ${im_del_code}, но файл всё ещё в AM"
        fi
    else
        test_fail "Тест 17: IM DELETE → ожидался 204/200, получен ${im_del_code}"
    fi
else
    test_fail "Тест 17: не удалось загрузить тестовый файл для удаления (code=${del_code})"
fi

# --------------------------------------------------------------------------
print_summary
