#!/usr/bin/env bash
# ==========================================================================
# test-qm-all.sh — Оркестратор интеграционных тестов Query Module
#
# Запускает все группы тестов QM последовательно:
#   1-3:   Health & Metrics
#   4-6:   Auth
#   7-12:  Search & Files
#   13-16: Download
#
# Аргументы:
#   --skip-download  Пропустить тесты download (13-16)
#
# Использование:
#   ./test-qm-all.sh
#   ./test-qm-all.sh --skip-download
# ==========================================================================

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Разбор аргументов
SKIP_DOWNLOAD=false
for arg in "$@"; do
    case "$arg" in
        --skip-download) SKIP_DOWNLOAD=true ;;
    esac
done

echo ""
echo "============================================================"
echo "  Artstore — Query Module Integration Tests"
echo "============================================================"
echo ""

TOTAL_PASS=0
TOTAL_FAIL=0
RESULTS=()

run_test_group() {
    local name="$1"
    local script="$2"

    echo ""
    echo "------------------------------------------------------------"
    echo "  ${name}"
    echo "------------------------------------------------------------"

    set +e
    output=$("${SCRIPT_DIR}/${script}" 2>&1)
    local exit_code=$?
    set -e

    echo "$output"

    # Извлекаем PASS/FAIL из вывода
    local pass=$(echo "$output" | grep -Eo '[0-9]+ PASS' | tail -1 | grep -Eo '[0-9]+')
    local fail=$(echo "$output" | grep -Eo '[0-9]+ FAIL' | tail -1 | grep -Eo '[0-9]+')
    pass=${pass:-0}
    fail=${fail:-0}

    TOTAL_PASS=$((TOTAL_PASS + pass))
    TOTAL_FAIL=$((TOTAL_FAIL + fail))

    if [[ $exit_code -eq 0 ]]; then
        RESULTS+=("  ✓ ${name}: ${pass} PASS / ${fail} FAIL")
    else
        RESULTS+=("  ✗ ${name}: ${pass} PASS / ${fail} FAIL")
    fi
}

# --- Запуск тестовых групп ---

run_test_group "Health & Metrics (1-3)" "test-qm-health.sh"
run_test_group "Auth (4-6)" "test-qm-auth.sh"
run_test_group "Search & Files (7-12)" "test-qm-search.sh"

if [[ "$SKIP_DOWNLOAD" == "false" ]]; then
    run_test_group "Download (13-16)" "test-qm-download.sh"
else
    echo ""
    echo "  >>> Download тесты (13-16) пропущены (--skip-download)"
fi

# --- Итоговый отчёт ---

echo ""
echo "============================================================"
echo "  Итоговый отчёт: Query Module"
echo "============================================================"
echo ""
for result in "${RESULTS[@]}"; do
    echo "$result"
done
echo ""
echo "  Всего: ${TOTAL_PASS} PASS / ${TOTAL_FAIL} FAIL (итого $((TOTAL_PASS + TOTAL_FAIL)))"
echo ""
echo "============================================================"

if [[ $TOTAL_FAIL -gt 0 ]]; then
    exit 1
fi
