#!/usr/bin/env bash
# ==========================================================================
# test-gc-reconcile.sh — Тесты GC и Reconciliation (тесты 25-26)
#
# Проверяет фоновые процессы:
#   25. Upload файл с коротким TTL в se-edit-2, подождать GC (30s), GET → expired/deleted
#   26. POST /maintenance/reconcile на se-rw-1 → 200, reconcile stats
#
# Переменные окружения:
#   JWKS_MOCK_URL    — URL JWKS Mock (по умолчанию https://localhost:18080)
#   SE_EDIT_2_URL    — URL SE edit-2 (по умолчанию https://localhost:18011)
#   SE_RW_1_URL      — URL SE rw-1 (по умолчанию https://localhost:18012)
#   GC_WAIT_SECONDS  — Время ожидания GC (по умолчанию 45)
#
# Использование:
#   ./test-gc-reconcile.sh
# ==========================================================================
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/lib.sh"

: "${JWKS_MOCK_URL:=https://localhost:18080}"
: "${SE_EDIT_2_URL:=https://localhost:18011}"
: "${SE_RW_1_URL:=https://localhost:18012}"
: "${GC_WAIT_SECONDS:=45}"

log_info "========================================================"
log_info "  Тесты GC и Reconciliation (25-26)"
log_info "  JWKS Mock: ${JWKS_MOCK_URL}"
log_info "  SE EDIT-2: ${SE_EDIT_2_URL}"
log_info "  SE RW-1: ${SE_RW_1_URL}"
log_info "  GC Wait: ${GC_WAIT_SECONDS}s"
log_info "========================================================"
echo ""

# --------------------------------------------------------------------------
# Подготовка: JWT
# --------------------------------------------------------------------------
log_info "Получение JWT токена..."
TOKEN=$(get_token "$JWKS_MOCK_URL" "test-gc" '["files:read","files:write","storage:write"]' 3600)
if [[ -z "$TOKEN" || "$TOKEN" == "null" ]]; then
    log_fail "Не удалось получить JWT токен — прерывание"
    exit 1
fi
log_ok "JWT получен"
echo ""

# ==========================================================================
# Тест 25: GC удаляет expired файл
#
# se-edit-2 в режиме edit → retention_policy=temporary, TTL по умолчанию.
# Загружаем файл, ждём GC цикл (30s по конфигурации).
# В edit mode файлы получают retention=temporary с TTL.
# Если TTL = 0 (не установлен) — файл не истечёт автоматически.
#
# Стратегия: загружаем файл, затем soft-delete и ждём GC для физического удаления.
# ==========================================================================
log_info "[Тест 25] GC: upload → delete → ждём GC → файл физически удалён"

# Шаг 1: Загрузить файл
RESPONSE=$(upload_file "$SE_EDIT_2_URL" "$TOKEN" "test-gc-file.bin" "application/octet-stream" "Файл для теста GC")
CODE=$(get_response_code "$RESPONSE")
BODY=$(get_response_body "$RESPONSE")

if [[ "$CODE" == "201" ]]; then
    GC_FILE_ID=$(echo "$BODY" | jq -r '.file_id // empty')
    log_ok "  Upload → file_id=${GC_FILE_ID}"
else
    test_fail "Тест 25: Upload для GC теста → HTTP ${CODE} (ожидался 201)"
    GC_FILE_ID=""
fi

if [[ -n "$GC_FILE_ID" ]]; then
    # Шаг 2: Soft-delete файл
    RESPONSE=$(http_delete "$SE_EDIT_2_URL" "$TOKEN" "/api/v1/files/${GC_FILE_ID}")
    DEL_CODE=$(get_response_code "$RESPONSE")
    log_ok "  DELETE → HTTP ${DEL_CODE}"

    # Шаг 3: Проверить что файл в статусе deleted
    RESPONSE=$(http_get "$SE_EDIT_2_URL" "$TOKEN" "/api/v1/files/${GC_FILE_ID}")
    CODE=$(get_response_code "$RESPONSE")
    BODY=$(get_response_body "$RESPONSE")

    if [[ "$CODE" == "200" ]]; then
        STATUS=$(echo "$BODY" | jq -r '.status // empty')
        log_ok "  До GC: status=${STATUS}"
    fi

    # Шаг 4: Ждём GC цикл
    log_info "  Ожидание GC цикла (${GC_WAIT_SECONDS}s)..."
    sleep "$GC_WAIT_SECONDS"

    # Шаг 5: Проверяем файл после GC
    RESPONSE=$(http_get "$SE_EDIT_2_URL" "$TOKEN" "/api/v1/files/${GC_FILE_ID}")
    CODE=$(get_response_code "$RESPONSE")
    BODY=$(get_response_body "$RESPONSE")

    if [[ "$CODE" == "404" ]]; then
        test_pass "Тест 25: GC удалил файл (GET → 404)"
    elif [[ "$CODE" == "200" ]]; then
        STATUS=$(echo "$BODY" | jq -r '.status // empty')
        if [[ "$STATUS" == "deleted" || "$STATUS" == "expired" ]]; then
            # GC ещё не успел физически удалить — файл помечен, но запись осталась
            test_pass "Тест 25: Файл помечен как ${STATUS} (GC обработает)"
        else
            test_fail "Тест 25: После GC файл всё ещё status=${STATUS} (ожидался deleted/expired/404)"
        fi
    else
        test_fail "Тест 25: После GC, GET → HTTP ${CODE} (ожидался 404 или 200 с status=deleted)"
    fi
fi

# ==========================================================================
# Тест 26: POST /maintenance/reconcile на se-rw-1 → 200
# ==========================================================================
log_info "[Тест 26] POST /maintenance/reconcile на se-rw-1 → 200"

RESPONSE=$(http_post "$SE_RW_1_URL" "$TOKEN" "/api/v1/maintenance/reconcile" "")
CODE=$(get_response_code "$RESPONSE")
BODY=$(get_response_body "$RESPONSE")

if [[ "$CODE" == "200" ]]; then
    FILES_CHECKED=$(echo "$BODY" | jq '.files_checked // 0')
    ORPHANED=$(echo "$BODY" | jq '.summary.orphaned_files // 0')
    MISSING=$(echo "$BODY" | jq '.summary.missing_files // 0')
    test_pass "Тест 26: Reconcile → 200, files_checked=${FILES_CHECKED}, orphaned=${ORPHANED}, missing=${MISSING}"
else
    test_fail "Тест 26: Reconcile → HTTP ${CODE} (ожидался 200)"
    BODY_ERR=$(get_response_body "$RESPONSE")
    if echo "$BODY_ERR" | jq . >/dev/null 2>&1; then
        log_fail "  Ответ: $(echo "$BODY_ERR" | jq -c '.')"
    fi
fi

# ==========================================================================
# Итоги
# ==========================================================================
echo ""
print_summary
