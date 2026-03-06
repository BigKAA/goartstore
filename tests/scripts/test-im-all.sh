#!/usr/bin/env bash
# ==========================================================================
# test-im-all.sh — Оркестратор интеграционных тестов Ingester Module
#
# Запускает все группы тестов IM последовательно:
#   1-3:   Health & Metrics
#   4-6:   Auth
#   7-17:  Upload & Delete
#
# Аргументы:
#   --skip-cross-module  Пропустить cross-module тест (16)
#
# Использование:
#   ./test-im-all.sh
#   ./test-im-all.sh --skip-cross-module
# ==========================================================================

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Разбор аргументов
SKIP_CROSS_MODULE=false
UPLOAD_ARGS=""
for arg in "$@"; do
    case "$arg" in
        --skip-cross-module)
            SKIP_CROSS_MODULE=true
            UPLOAD_ARGS="--skip-cross-module"
            ;;
    esac
done

echo ""
echo "============================================================"
echo "  Artstore — Ingester Module Integration Tests"
echo "============================================================"
echo ""

TOTAL_PASS=0
TOTAL_FAIL=0
RESULTS=()

run_test_group() {
    local name="$1"
    local script="$2"
    shift 2

    echo ""
    echo "------------------------------------------------------------"
    echo "  ${name}"
    echo "------------------------------------------------------------"

    set +e
    if [[ $# -gt 0 ]]; then
        output=$("${SCRIPT_DIR}/${script}" "$@" 2>&1)
    else
        output=$("${SCRIPT_DIR}/${script}" 2>&1)
    fi
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

run_test_group "Health & Metrics (1-3)" "test-im-health.sh"
run_test_group "Auth (4-6)" "test-im-auth.sh"

if [[ -n "$UPLOAD_ARGS" ]]; then
    run_test_group "Upload & Delete (7-17)" "test-im-upload.sh" "$UPLOAD_ARGS"
else
    run_test_group "Upload & Delete (7-17)" "test-im-upload.sh"
fi

# --- Итоговый отчёт ---

echo ""
echo "============================================================"
echo "  Итоговый отчёт: Ingester Module"
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
