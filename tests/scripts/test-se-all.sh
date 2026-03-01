#!/usr/bin/env bash
# ==========================================================================
# test-se-all.sh — Оркестратор интеграционных тестов Storage Element
#
# Запускает все группы тестов SE последовательно:
#   1-12:  Basic (health, info, upload, download, delete, auth)
#   13-18: Locks (Lock API, DELETE 409)
#   19-23: Parallel (параллельная запись, index sync)
#
# Аргументы:
#   --skip-parallel  Пропустить тесты параллельной записи (требуют 2 реплики)
#
# Использование:
#   ./test-se-all.sh
#   ./test-se-all.sh --skip-parallel
# ==========================================================================

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Разбор аргументов
SKIP_PARALLEL=false
for arg in "$@"; do
    case "$arg" in
        --skip-parallel)
            SKIP_PARALLEL=true
            ;;
    esac
done

# Счётчики
TOTAL_PASS=0
TOTAL_FAIL=0
RESULTS=()

# Запуск группы тестов
run_test_group() {
    local name="$1"
    local script="$2"
    shift 2

    echo ""
    echo "╔══════════════════════════════════════════════════════════╗"
    echo "║  ${name}"
    echo "╚══════════════════════════════════════════════════════════╝"
    echo ""

    set +e
    output=$("${SCRIPT_DIR}/${script}" "$@" 2>&1)
    local exit_code=$?
    set -e

    echo "$output"

    # Парсим результаты
    local pass
    pass=$(echo "$output" | grep -Eo '[0-9]+ PASS' | tail -1 | grep -Eo '[0-9]+') || pass=0
    local fail
    fail=$(echo "$output" | grep -Eo '[0-9]+ FAIL' | tail -1 | grep -Eo '[0-9]+') || fail=0

    TOTAL_PASS=$((TOTAL_PASS + pass))
    TOTAL_FAIL=$((TOTAL_FAIL + fail))

    if [[ $exit_code -eq 0 ]]; then
        RESULTS+=("  ✓ ${name}: ${pass} PASS / ${fail} FAIL")
    else
        RESULTS+=("  ✗ ${name}: ${pass} PASS / ${fail} FAIL")
    fi
}

# ==========================================================================
# Запуск тестов
# ==========================================================================

echo ""
echo "╔══════════════════════════════════════════════════════════╗"
echo "║  Storage Element — Интеграционные тесты (Stateless)     ║"
echo "╚══════════════════════════════════════════════════════════╝"

# Группа 1: Basic
run_test_group "SE Basic (health, info, upload, download, delete)" "test-se-basic.sh"

# Группа 2: Locks
run_test_group "SE Locks (Lock API, DELETE 409)" "test-se-locks.sh"

# Группа 3: Parallel
if $SKIP_PARALLEL; then
    echo ""
    echo ">>> Тесты параллельной записи пропущены (--skip-parallel)"
    RESULTS+=("  – SE Parallel: ПРОПУЩЕН")
else
    run_test_group "SE Parallel (параллельная запись, index sync)" "test-se-parallel.sh"
fi

# ==========================================================================
# Итого
# ==========================================================================
echo ""
echo "╔══════════════════════════════════════════════════════════╗"
echo "║  Результаты SE тестов                                   ║"
echo "╚══════════════════════════════════════════════════════════╝"
echo ""
for result in "${RESULTS[@]}"; do
    echo "$result"
done
echo ""
echo "  ИТОГО: ${TOTAL_PASS} PASS / ${TOTAL_FAIL} FAIL"
echo ""

if [[ $TOTAL_FAIL -gt 0 ]]; then
    exit 1
fi
