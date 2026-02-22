#!/usr/bin/env bash
# ==========================================================================
# test-all.sh — Запуск всех тестовых групп (тесты 1-30)
#
# Последовательно запускает все тестовые скрипты, собирает итоги.
# Exit code: 0 если все группы пройдены, 1 при наличии ошибок.
#
# Переменные окружения — наследуются из вызывающей среды:
#   JWKS_MOCK_URL, SE_EDIT_1_URL, SE_EDIT_2_URL, SE_RW_1_URL, SE_RW_2_URL,
#   SE_RO_URL, SE_AR_URL, K8S_NAMESPACE, CURL_OPTS
#
# Использование:
#   ./test-all.sh                          # Все группы
#   ./test-all.sh --skip-replica           # Без тестов replica (19-22)
#   ./test-all.sh --skip-gc               # Без GC тестов (25-26, экономит ~45s)
# ==========================================================================
set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# ==========================================================================
# Разбор аргументов
# ==========================================================================
SKIP_REPLICA=false
SKIP_GC=false

for arg in "$@"; do
    case "$arg" in
        --skip-replica) SKIP_REPLICA=true ;;
        --skip-gc)      SKIP_GC=true ;;
        --help|-h)
            echo "Использование: ./test-all.sh [--skip-replica] [--skip-gc]"
            echo ""
            echo "Опции:"
            echo "  --skip-replica  Пропустить тесты replica (19-22)"
            echo "  --skip-gc       Пропустить тесты GC (25-26, экономит ~45s ожидания)"
            exit 0
            ;;
    esac
done

# ==========================================================================
# Цвета для вывода
# ==========================================================================
RED='\033[0;31m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[0;33m'
NC='\033[0m' # No Color

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
echo -e "${BLUE}  SE Integration Tests — Полный набор${NC}"
echo -e "${BLUE}========================================================${NC}"
echo ""

run_group "${SCRIPT_DIR}/test-smoke.sh"        "Smoke"          "1-3"
run_group "${SCRIPT_DIR}/test-files.sh"         "Files"          "4-11"
run_group "${SCRIPT_DIR}/test-modes.sh"         "Modes"          "12-18"

if [[ "$SKIP_REPLICA" == "false" ]]; then
    run_group "${SCRIPT_DIR}/test-replica.sh"   "Replica"        "19-22"
fi

run_group "${SCRIPT_DIR}/test-data.sh"          "Data"           "23-24"

if [[ "$SKIP_GC" == "false" ]]; then
    run_group "${SCRIPT_DIR}/test-gc-reconcile.sh" "GC/Reconcile" "25-26"
fi

run_group "${SCRIPT_DIR}/test-errors.sh"        "Errors"         "27-30"

# ==========================================================================
# Итоговый отчёт
# ==========================================================================
echo -e "${BLUE}========================================================${NC}"
echo -e "${BLUE}  ИТОГИ${NC}"
echo -e "${BLUE}========================================================${NC}"
echo ""

SKIPPED=""
if [[ "$SKIP_REPLICA" == "true" ]]; then
    SKIPPED="${SKIPPED} Replica(19-22)"
fi
if [[ "$SKIP_GC" == "true" ]]; then
    SKIPPED="${SKIPPED} GC(25-26)"
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
