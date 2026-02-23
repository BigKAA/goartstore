#!/usr/bin/env bash
# ==========================================================================
# test-am-all.sh — Запуск всех тестов Admin Module (тесты 1-40)
#
# Последовательно запускает все тестовые группы, собирает итоги.
# Exit code: 0 если все группы пройдены, 1 при наличии ошибок.
#
# Переменные окружения — наследуются из вызывающей среды (Makefile):
#   AM_URL, KC_TOKEN_URL, KC_TEST_USER_CLIENT_ID,
#   KC_TEST_USER_CLIENT_SECRET, KC_ADMIN_USERNAME, KC_ADMIN_PASSWORD,
#   KC_VIEWER_USERNAME, KC_VIEWER_PASSWORD, KC_SA_CLIENT_ID,
#   KC_SA_CLIENT_SECRET, SE_RW_1_URL, K8S_NAMESPACE, CURL_OPTS
#
# Использование:
#   ./test-am-all.sh                  # Все группы
#   ./test-am-all.sh --skip-se        # Без тестов SE (16-20, требуют SE pods)
#   ./test-am-all.sh --skip-files     # Без тестов Files (21-24, зависят от SE)
# ==========================================================================
set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# ==========================================================================
# Разбор аргументов
# ==========================================================================
SKIP_SE=false
SKIP_FILES=false
SKIP_UI=false

for arg in "$@"; do
    case "$arg" in
        --skip-se)    SKIP_SE=true ;;
        --skip-files) SKIP_FILES=true ;;
        --skip-ui)    SKIP_UI=true ;;
        --help|-h)
            echo "Использование: ./test-am-all.sh [--skip-se] [--skip-files] [--skip-ui]"
            echo ""
            echo "Опции:"
            echo "  --skip-se      Пропустить тесты Storage Elements (16-20)"
            echo "  --skip-files   Пропустить тесты Files (21-24, зависят от SE)"
            echo "  --skip-ui      Пропустить тесты Admin UI (31-40)"
            exit 0
            ;;
    esac
done

# Если пропускаем SE, автоматически пропускаем Files (зависят от SE)
if [[ "$SKIP_SE" == "true" ]]; then
    SKIP_FILES=true
fi

# ==========================================================================
# Цвета для вывода
# ==========================================================================
RED='\033[0;31m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[0;33m'
NC='\033[0m'

# ==========================================================================
# Запуск тестовой группы
# Аргументы: $1 — скрипт, $2 — название, $3 — номера тестов
# ==========================================================================
TOTAL_GROUPS=0
PASSED_GROUPS=0
FAILED_GROUPS=0
FAILED_LIST=""
GROUP_NUM=0

run_group() {
    local script="$1"
    local name="$2"
    local tests="$3"

    GROUP_NUM=$((GROUP_NUM + 1))
    TOTAL_GROUPS=$((TOTAL_GROUPS + 1))

    echo -e "${BLUE}────────────────────────────────────────────────────────${NC}"
    echo -e "${BLUE}  [${GROUP_NUM}] ${name} (тесты ${tests})${NC}"
    echo -e "${BLUE}────────────────────────────────────────────────────────${NC}"
    echo ""

    chmod +x "$script" 2>/dev/null || true

    if "$script"; then
        PASSED_GROUPS=$((PASSED_GROUPS + 1))
        echo -e "${GREEN}  >>> ${name}: PASS${NC}"
    else
        FAILED_GROUPS=$((FAILED_GROUPS + 1))
        FAILED_LIST="${FAILED_LIST}\n  - ${name} (тесты ${tests})"
        echo -e "${RED}  >>> ${name}: FAIL${NC}"
    fi
    echo ""
}

# ==========================================================================
# Запуск тестов
# ==========================================================================
echo -e "${BLUE}========================================================${NC}"
echo -e "${BLUE}  AM Integration Tests — Полный набор${NC}"
echo -e "${BLUE}========================================================${NC}"
echo ""

run_group "${SCRIPT_DIR}/test-am-smoke.sh"             "Smoke"             "1-3"
run_group "${SCRIPT_DIR}/test-am-admin-auth.sh"        "Admin Auth"        "4"
run_group "${SCRIPT_DIR}/test-am-admin-users.sh"       "Admin Users"       "5-9"
run_group "${SCRIPT_DIR}/test-am-service-accounts.sh"  "Service Accounts"  "10-15"

if [[ "$SKIP_SE" == "false" ]]; then
    run_group "${SCRIPT_DIR}/test-am-storage-elements.sh" "Storage Elements" "16-20"
fi

if [[ "$SKIP_FILES" == "false" ]]; then
    run_group "${SCRIPT_DIR}/test-am-files.sh"            "Files Registry"   "21-24"
fi

run_group "${SCRIPT_DIR}/test-am-idp.sh"               "IdP Status"        "25-26"
run_group "${SCRIPT_DIR}/test-am-errors.sh"            "Error Handling"    "27-30"

if [[ "$SKIP_UI" == "false" ]]; then
    run_group "${SCRIPT_DIR}/test-am-ui.sh"              "Admin UI"          "31-40"
fi

# ==========================================================================
# Итоговый отчёт
# ==========================================================================
echo -e "${BLUE}========================================================${NC}"
echo -e "${BLUE}  ИТОГИ${NC}"
echo -e "${BLUE}========================================================${NC}"
echo ""

SKIPPED=""
if [[ "$SKIP_SE" == "true" ]]; then
    SKIPPED="${SKIPPED} SE(16-20)"
fi
if [[ "$SKIP_FILES" == "true" ]]; then
    SKIPPED="${SKIPPED} Files(21-24)"
fi
if [[ "$SKIP_UI" == "true" ]]; then
    SKIPPED="${SKIPPED} UI(31-40)"
fi

echo -e "  Групп: ${TOTAL_GROUPS}"
echo -e "  ${GREEN}PASS: ${PASSED_GROUPS}${NC}"
echo -e "  ${RED}FAIL: ${FAILED_GROUPS}${NC}"
if [[ -n "$SKIPPED" ]]; then
    echo -e "  ${YELLOW}SKIP:${SKIPPED}${NC}"
fi
echo ""

if [[ -n "$FAILED_LIST" ]]; then
    echo -e "${RED}Провалившиеся группы:${NC}"
    echo -e "${RED}${FAILED_LIST}${NC}"
    echo ""
fi

if [[ $FAILED_GROUPS -gt 0 ]]; then
    echo -e "${RED}РЕЗУЛЬТАТ: FAIL${NC}"
    exit 1
else
    echo -e "${GREEN}РЕЗУЛЬТАТ: ALL PASS${NC}"
    exit 0
fi
