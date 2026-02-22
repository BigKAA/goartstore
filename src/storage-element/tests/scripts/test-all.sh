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
# Определение тестовых групп
# ==========================================================================
declare -a GROUPS=()
declare -a GROUP_NAMES=()
declare -a GROUP_TESTS=()

GROUPS+=("${SCRIPT_DIR}/test-smoke.sh")
GROUP_NAMES+=("Smoke")
GROUP_TESTS+=("1-3")

GROUPS+=("${SCRIPT_DIR}/test-files.sh")
GROUP_NAMES+=("Files")
GROUP_TESTS+=("4-11")

GROUPS+=("${SCRIPT_DIR}/test-modes.sh")
GROUP_NAMES+=("Modes")
GROUP_TESTS+=("12-18")

if [[ "$SKIP_REPLICA" == "false" ]]; then
    GROUPS+=("${SCRIPT_DIR}/test-replica.sh")
    GROUP_NAMES+=("Replica")
    GROUP_TESTS+=("19-22")
fi

GROUPS+=("${SCRIPT_DIR}/test-data.sh")
GROUP_NAMES+=("Data")
GROUP_TESTS+=("23-24")

if [[ "$SKIP_GC" == "false" ]]; then
    GROUPS+=("${SCRIPT_DIR}/test-gc-reconcile.sh")
    GROUP_NAMES+=("GC/Reconcile")
    GROUP_TESTS+=("25-26")
fi

GROUPS+=("${SCRIPT_DIR}/test-errors.sh")
GROUP_NAMES+=("Errors")
GROUP_TESTS+=("27-30")

# ==========================================================================
# Запуск тестов
# ==========================================================================
echo -e "${BLUE}========================================================${NC}"
echo -e "${BLUE}  SE Integration Tests — Полный набор${NC}"
echo -e "${BLUE}========================================================${NC}"
echo ""

TOTAL_GROUPS=${#GROUPS[@]}
PASSED_GROUPS=0
FAILED_GROUPS=0
FAILED_LIST=()

for i in "${!GROUPS[@]}"; do
    SCRIPT="${GROUPS[$i]}"
    NAME="${GROUP_NAMES[$i]}"
    TESTS="${GROUP_TESTS[$i]}"

    echo -e "${BLUE}────────────────────────────────────────────────────────${NC}"
    echo -e "${BLUE}  [$(( i + 1 ))/${TOTAL_GROUPS}] ${NAME} (тесты ${TESTS})${NC}"
    echo -e "${BLUE}────────────────────────────────────────────────────────${NC}"
    echo ""

    if [[ ! -x "$SCRIPT" ]]; then
        chmod +x "$SCRIPT"
    fi

    if "$SCRIPT"; then
        PASSED_GROUPS=$((PASSED_GROUPS + 1))
        echo -e "${GREEN}  >>> ${NAME}: PASS${NC}"
    else
        FAILED_GROUPS=$((FAILED_GROUPS + 1))
        FAILED_LIST+=("${NAME} (тесты ${TESTS})")
        echo -e "${RED}  >>> ${NAME}: FAIL${NC}"
    fi
    echo ""
done

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

if [[ ${#FAILED_LIST[@]} -gt 0 ]]; then
    echo -e "${RED}Провалившиеся группы:${NC}"
    for f in "${FAILED_LIST[@]}"; do
        echo -e "  ${RED}- ${f}${NC}"
    done
    echo ""
fi

if [[ $FAILED_GROUPS -gt 0 ]]; then
    echo -e "${RED}РЕЗУЛЬТАТ: FAIL${NC}"
    exit 1
else
    echo -e "${GREEN}РЕЗУЛЬТАТ: ALL PASS${NC}"
    exit 0
fi
