#!/usr/bin/env bash
# Smoke test suite for dockerz
# Runs critical-path scenarios against tests/test-project/
set -uo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
CYAN='\033[0;36m'
NC='\033[0m'
PASS=0
FAIL=0

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
BIN="$SCRIPT_DIR/../dockerz"
TEST_PROJECT="$SCRIPT_DIR/test-project"

pass()   { echo -e "  ${GREEN}✓${NC} $1"; PASS=$((PASS + 1)); true; }
fail()   { echo -e "  ${RED}✗${NC} $1"; FAIL=$((FAIL + 1)); true; }

heading() { echo ""; echo "━━━ $1 ━━━━━"; }

echo "============================================"
echo "  dockerz — Smoke Test Suite"
echo "============================================"
echo "Binary: $BIN"
echo "Project: $TEST_PROJECT"

if [[ ! -x "$BIN" ]]; then
    echo -e "${RED}Binary not found. Run 'make build' first.${NC}"
    exit 1
fi

# ─── 1. Config Validate (valid) ──────────────────
heading "1. Config Validate (valid)"
"$BIN" config validate --config "$TEST_PROJECT/build.yaml" > /dev/null 2>&1 \
    && pass "valid config" || fail "valid config"

# ─── 2. Config Validate (missing file) ───────────
heading "2. Config Validate (missing file)"
"$BIN" config validate --config /nonexistent/build.yaml > /dev/null 2>&1 \
    && fail "missing file exits 1" || pass "missing file exits 1"

# ─── 3. Init (sample config) ─────────────────────
heading "3. Init (sample config)"
TMPDIR=$(mktemp -d)
pushd "$TMPDIR" > /dev/null
"$BIN" init > /dev/null 2>&1 && pass "init succeeds" || fail "init succeeds"
[[ -f build.yaml ]] && pass "build.yaml created" || fail "build.yaml created"
popd > /dev/null
rm -rf "$TMPDIR"

# ─── 4. Auto-discovery (dry run) ─────────────────
heading "4. Auto-Discovery (dry run)"
OUT=$("$BIN" build --dry-run --config "$TEST_PROJECT/build.yaml" 2>&1)
RC=$?
[[ $RC -eq 0 ]] && pass "exit 0" || fail "exit 0"
echo "$OUT" | grep -q "api "      && pass "finds api"      || fail "finds api"
echo "$OUT" | grep -q "backend "  && pass "finds backend"  || fail "finds backend"
echo "$OUT" | grep -q "frontend " && pass "finds frontend" || fail "finds frontend"
echo "$OUT" | grep -q "shared "   && pass "finds shared"   || fail "finds shared"

# ─── 5. Smart + Git Track (dry run) ──────────────
heading "5. Smart + Git Track (dry run)"
OUT=$("$BIN" build --dry-run --smart --git-track --config "$TEST_PROJECT/build.yaml" 2>&1)
RC=$?
[[ $RC -eq 0 ]] && pass "exit 0" || fail "exit 0"

# ─── 6. Input file (dry run from test-project) ───
heading "6. Input File (dry run)"
pushd "$TEST_PROJECT" > /dev/null
OUT=$("$BIN" build --dry-run --input-changed-services changed.txt 2>&1)
RC=$?
popd > /dev/null
[[ $RC -eq 0 ]] && pass "exit 0" || fail "exit 0"
echo "$OUT" | grep -q "api " && pass "input file api" || fail "input file api"

# ─── 7. Force (dry run) ──────────────────────────
heading "7. Force (dry run)"
OUT=$("$BIN" build --dry-run --force --config "$TEST_PROJECT/build.yaml" 2>&1)
RC=$?
[[ $RC -eq 0 ]] && pass "exit 0" || fail "exit 0"

# ─── 8. Shell completions ────────────────────────
heading "8. Shell Completions"
for shell in bash zsh fish; do
    "$BIN" completion "$shell" > /dev/null 2>&1 \
        && pass "completion $shell" || fail "completion $shell"
done

# ─── 9. Version flag ─────────────────────────────
heading "9. Version"
OUT=$("$BIN" --version 2>&1)
RC=$?
[[ $RC -eq 0 ]] && pass "exit 0" || fail "exit 0"
echo "$OUT" | grep -qi "dockerz" && pass "version string" || fail "version string"

# ─── 10. Help text ───────────────────────────────
heading "10. Help"
OUT=$("$BIN" --help 2>&1)
RC=$?
[[ $RC -eq 0 ]] && pass "exit 0" || fail "exit 0"
for cmd in build init config completion; do
    echo "$OUT" | grep -q "$cmd" && pass "shows $cmd" || fail "shows $cmd"
done

# ─── Summary ─────────────────────────────────────
echo ""
echo "============================================"
echo -e "  ${GREEN}$PASS passed${NC}, ${RED}$FAIL failed${NC}"
echo "============================================"
[[ "$FAIL" -gt 0 ]] && exit 1 || exit 0
