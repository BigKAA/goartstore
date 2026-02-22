#!/usr/bin/env bash
# ==========================================================================
# init-data.sh — инициализация тестовых данных для SE тестовой среды
#
# Запускается как Helm post-install Job в контейнере alpine:3.19.
# Подготавливает тестовую среду: загружает файлы, переводит SE в нужные режимы.
#
# Переменные окружения (устанавливаются в init-job.yaml):
#   JWKS_MOCK_URL  — URL JWKS Mock Server
#   SE_EDIT_1_URL  — URL SE edit-1 (replicated)
#   SE_EDIT_2_URL  — URL SE edit-2 (replicated)
#   SE_RW_1_URL    — URL SE rw-1 (standalone)
#   SE_RW_2_URL    — URL SE rw-2 (standalone)
#   SE_RO_URL      — URL SE ro (standalone, будет переведён в ro)
#   SE_AR_URL      — URL SE ar (standalone, будет переведён в ar)
#   CURL_OPTS      — опции curl (-k -s --connect-timeout 5)
#
# Зависимости: bash, curl, jq (устанавливаются через apk)
# ==========================================================================
set -euo pipefail
source /scripts/lib.sh

log_info "========================================================"
log_info "  SE Test Environment — Инициализация данных"
log_info "========================================================"
log_info ""

# ==========================================================================
# Шаг 1: Дождаться готовности JWKS Mock (до 60 секунд)
# ==========================================================================
log_info "[1/8] Ожидание готовности JWKS Mock..."

JWKS_READY=false
JWKS_TIMEOUT=60
JWKS_ELAPSED=0

while [[ $JWKS_ELAPSED -lt $JWKS_TIMEOUT ]]; do
    HTTP_CODE=$(curl $CURL_OPTS -o /dev/null -w "%{http_code}" \
        "${JWKS_MOCK_URL}/jwks" 2>/dev/null) || HTTP_CODE="000"

    if [[ "$HTTP_CODE" == "200" ]]; then
        JWKS_READY=true
        break
    fi

    sleep 2
    JWKS_ELAPSED=$((JWKS_ELAPSED + 2))
done

if [[ "$JWKS_READY" != "true" ]]; then
    log_fail "JWKS Mock не стал доступен за ${JWKS_TIMEOUT} секунд"
    exit 1
fi
log_ok "JWKS Mock доступен (${JWKS_ELAPSED}с)"

# ==========================================================================
# Шаг 2: Получить JWT токен с полными правами
# ==========================================================================
log_info "[2/8] Получение JWT токена..."

TOKEN=$(get_token "$JWKS_MOCK_URL" "init-job" '["files:read","files:write","storage:write"]' 3600)

if [[ -z "$TOKEN" || "$TOKEN" == "null" ]]; then
    log_fail "Не удалось получить JWT токен"
    exit 1
fi
log_ok "JWT получен (sub=init-job, scopes=[files:read,files:write,storage:write])"

# ==========================================================================
# Шаг 3: Дождаться готовности всех 6 SE экземпляров (до 120 секунд каждый)
# ==========================================================================
log_info "[3/8] Ожидание готовности Storage Elements..."

# Все URL экземпляров SE
SE_NAMES=("SE_EDIT_1" "SE_EDIT_2" "SE_RW_1" "SE_RW_2" "SE_RO" "SE_AR")
SE_URLS=("$SE_EDIT_1_URL" "$SE_EDIT_2_URL" "$SE_RW_1_URL" "$SE_RW_2_URL" "$SE_RO_URL" "$SE_AR_URL")
SE_TIMEOUT=120

ALL_READY=true
for i in "${!SE_NAMES[@]}"; do
    NAME="${SE_NAMES[$i]}"
    URL="${SE_URLS[$i]}"
    log_info "  Ожидание ${NAME}..."

    if wait_ready "$URL" $SE_TIMEOUT; then
        log_ok "  ${NAME} готов"
    else
        log_fail "  ${NAME} не стал доступен за ${SE_TIMEOUT} секунд"
        ALL_READY=false
    fi
done

if [[ "$ALL_READY" != "true" ]]; then
    log_fail "Не все SE экземпляры готовы — прерывание"
    exit 1
fi
log_ok "Все 6 SE экземпляров готовы"

# ==========================================================================
# Шаг 4: Загрузка 4 тестовых файлов в se-ro
# ==========================================================================
log_info "[4/8] Загрузка тестовых файлов в se-ro (4 файла)..."

RO_UPLOADED=0
RO_FILES=("test-ro-doc.bin" "test-ro-image.bin" "test-ro-data.bin" "test-ro-backup.bin")
RO_DESCS=("Тестовый документ" "Тестовое изображение" "Тестовые данные" "Тестовый бэкап")

for i in "${!RO_FILES[@]}"; do
    FILENAME="${RO_FILES[$i]}"
    DESC="${RO_DESCS[$i]}"

    RESPONSE=$(upload_file "$SE_RO_URL" "$TOKEN" "$FILENAME" "application/octet-stream" "$DESC")

    if assert_status "$RESPONSE" 201 "Upload ${FILENAME}"; then
        RO_UPLOADED=$((RO_UPLOADED + 1))
        FILE_ID=$(get_response_body "$RESPONSE" | jq -r '.file_id // .id // "unknown"')
        log_ok "  ${FILENAME} -> ${FILE_ID}"
    else
        log_fail "Не удалось загрузить ${FILENAME} в se-ro"
        exit 1
    fi
done

log_ok "Загружено ${RO_UPLOADED} файлов в se-ro"

# ==========================================================================
# Шаг 5: Загрузка 200 тестовых файлов в se-ar
# ==========================================================================
log_info "[5/8] Загрузка тестовых файлов в se-ar (200 файлов, ~1KB каждый)..."

AR_UPLOADED=0
AR_FAILED=0

for i in $(seq 1 200); do
    FILENAME="test-ar-$(printf '%03d' "$i").bin"

    RESPONSE=$(upload_file "$SE_AR_URL" "$TOKEN" "$FILENAME" "application/octet-stream" "Архивный файл ${i}/200")

    if assert_status "$RESPONSE" 201 ""; then
        AR_UPLOADED=$((AR_UPLOADED + 1))
    else
        AR_FAILED=$((AR_FAILED + 1))
        log_warn "  Ошибка при загрузке ${FILENAME}"
    fi

    # Прогресс каждые 50 файлов
    if (( i % 50 == 0 )); then
        log_info "  Прогресс: ${i}/200 (загружено: ${AR_UPLOADED}, ошибок: ${AR_FAILED})"
    fi
done

if [[ $AR_UPLOADED -ne 200 ]]; then
    log_fail "Загружено ${AR_UPLOADED}/200 файлов в se-ar (${AR_FAILED} ошибок)"
    exit 1
fi
log_ok "Загружено ${AR_UPLOADED} файлов в se-ar"

# ==========================================================================
# Шаг 6: Перевод se-ro в режим ro (rw → ro)
# ==========================================================================
log_info "[6/8] Перевод se-ro в режим ro..."

RESPONSE=$(transition_mode "$SE_RO_URL" "$TOKEN" "ro" "false")

if assert_status "$RESPONSE" 200 "Transition se-ro rw->ro"; then
    BODY=$(get_response_body "$RESPONSE")
    PREV=$(echo "$BODY" | jq -r '.previous_mode // "unknown"')
    CURR=$(echo "$BODY" | jq -r '.current_mode // "unknown"')
    log_ok "se-ro: ${PREV} -> ${CURR}"
else
    log_fail "Не удалось перевести se-ro в режим ro"
    exit 1
fi

# ==========================================================================
# Шаг 7: Перевод se-ar в режим ar (rw → ro → ar)
# ==========================================================================
log_info "[7/8] Перевод se-ar в режим ar (rw -> ro -> ar)..."

# Шаг 7a: rw → ro
RESPONSE=$(transition_mode "$SE_AR_URL" "$TOKEN" "ro" "false")

if assert_status "$RESPONSE" 200 "Transition se-ar rw->ro"; then
    BODY=$(get_response_body "$RESPONSE")
    PREV=$(echo "$BODY" | jq -r '.previous_mode // "unknown"')
    CURR=$(echo "$BODY" | jq -r '.current_mode // "unknown"')
    log_ok "se-ar: ${PREV} -> ${CURR}"
else
    log_fail "Не удалось перевести se-ar в режим ro"
    exit 1
fi

# Шаг 7b: ro → ar
RESPONSE=$(transition_mode "$SE_AR_URL" "$TOKEN" "ar" "false")

if assert_status "$RESPONSE" 200 "Transition se-ar ro->ar"; then
    BODY=$(get_response_body "$RESPONSE")
    PREV=$(echo "$BODY" | jq -r '.previous_mode // "unknown"')
    CURR=$(echo "$BODY" | jq -r '.current_mode // "unknown"')
    log_ok "se-ar: ${PREV} -> ${CURR}"
else
    log_fail "Не удалось перевести se-ar в режим ar"
    exit 1
fi

# Удаление физических файлов данных из se-ar (оставляем только *.attr.json).
# SE_AR_DATA_DIR — путь к PVC se-ar-data, монтируется в init job.
log_info "  Удаление физических файлов из se-ar (оставляем только *.attr.json)..."

if [[ -d "${SE_AR_DATA_DIR:-}" ]]; then
    DELETED_COUNT=$(find "$SE_AR_DATA_DIR" -maxdepth 1 -type f \
        ! -name '*.attr.json' ! -name '.*' -delete -print | wc -l)
    log_ok "  Удалено ${DELETED_COUNT} файлов данных, attr.json сохранены"
else
    log_warn "  SE_AR_DATA_DIR не смонтирован — пропуск очистки"
fi

# ==========================================================================
# Шаг 8: Итоговый отчёт
# ==========================================================================
log_info ""
log_info "========================================================"
log_info "  Инициализация завершена успешно"
log_info "========================================================"
log_ok "se-edit-1 : режим edit, replicated (2 реплики)"
log_ok "se-edit-2 : режим edit, replicated (2 реплики)"
log_ok "se-rw-1   : режим rw, standalone"
log_ok "se-rw-2   : режим rw, standalone"
log_ok "se-ro     : режим ro, standalone, ${RO_UPLOADED} файлов"
log_ok "se-ar     : режим ar, standalone, ${AR_UPLOADED} attr.json (только метаданные)"
log_info "========================================================"
