#!/usr/bin/env bash
# Regression tests for scripts/ci/sync-branch-protection.sh.
#
# These tests fake `gh` so ci-script-check can verify the script's control
# flow without mutating GitHub settings or requiring network access.

set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
SCRIPT="$ROOT/scripts/ci/sync-branch-protection.sh"
TMP="$(mktemp -d -t opendbx-sync-branch-protection-test.XXXXXX)"
trap 'rm -rf "$TMP"' EXIT

cat >"$TMP/gh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail

if [ "${1:-}" = "api" ]; then
  endpoint="${2:-}"
  case "${FAKE_GH_MODE:-ok}" in
    ok)
      case "$endpoint" in
        *required_status_checks)
          printf '{"contexts":["validate"]}\n'
          exit 0
          ;;
      esac
      ;;
    not-protected)
      echo "HTTP 404: Branch not protected" >&2
      exit 1
      ;;
    error)
      echo "proxyconnect tcp: connect: operation not permitted" >&2
      exit 1
      ;;
  esac
fi

echo "unexpected fake gh invocation: $*" >&2
exit 2
EOF
chmod +x "$TMP/gh"

run_case() {
  local mode="$1"
  local want_rc="$2"
  local out="$TMP/$mode.out"
  local err="$TMP/$mode.err"

  set +e
  PATH="$TMP:$PATH" REPO=sqlrush/opendbx FAKE_GH_MODE="$mode" \
    bash "$SCRIPT" --dry-run >"$out" 2>"$err"
  local rc=$?
  set -e

  if [ "$rc" -ne "$want_rc" ]; then
    echo "sync-branch-protection test $mode: want rc=$want_rc got rc=$rc" >&2
    echo "--- stdout ---" >&2
    cat "$out" >&2
    echo "--- stderr ---" >&2
    cat "$err" >&2
    exit 1
  fi
}

run_case ok 0
grep -q '  - validate' "$TMP/ok.out"
grep -q 'would PATCH' "$TMP/ok.out"

run_case not-protected 0
grep -q 'none configured yet, or main not protected' "$TMP/not-protected.out"

run_case error 1
grep -q 'failed to read current required_status_checks' "$TMP/error.err"
grep -q 'operation not permitted' "$TMP/error.err"
if grep -q 'none configured yet' "$TMP/error.out"; then
  echo "sync-branch-protection test error: network failure was mislabeled as unprotected" >&2
  exit 1
fi

echo "sync-branch-protection tests OK"
