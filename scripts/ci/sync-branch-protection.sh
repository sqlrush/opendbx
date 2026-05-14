#!/usr/bin/env bash
# scripts/ci/sync-branch-protection.sh — spec-0.9 D-5 / T-7
#
# 同步 main branch 的 required_status_checks 到 9 stable job names.
# SSOT: scripts/ci/branch-protection-required-checks.json
#
# Usage:
#   sync-branch-protection.sh                   # 默认 dry-run
#   sync-branch-protection.sh --dry-run         # 显式 dry-run
#   sync-branch-protection.sh --apply           # 真实写 GitHub API
#   sync-branch-protection.sh --help
#
# 设计要点 (R2 codex CRIT-2 + HIGH-5 + 用户拍板):
# - 窄范围 PATCH /required_status_checks 端点 — 不动 force-push /
#   signed-commit / enforce_admins / linear-history 等保护设置
# - 真正的 CLI flag parse (DRY_RUN env 退化为 internal 不再公开接口)
# - --apply 前显示当前 vs 目标 contexts diff
# - reject 未知 flag → exit 2
#
# Design refs:
# - spec-0.9-ci-github-actions.md § 2.4
# - GitHub branch protection API:
#   https://docs.github.com/en/rest/branches/branch-protection

set -euo pipefail

APPLY=0
while [ $# -gt 0 ]; do
  case "$1" in
    --dry-run)
      APPLY=0
      ;;
    --apply)
      APPLY=1
      ;;
    --help|-h)
      sed -n '2,18p' "$0"
      exit 0
      ;;
    *)
      echo "ERR: unknown flag $1" >&2
      echo "     try --help" >&2
      exit 2
      ;;
  esac
  shift
done

REPO="${REPO:-$(gh repo view --json nameWithOwner -q .nameWithOwner)}"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
CONFIG="$SCRIPT_DIR/branch-protection-required-checks.json"

if [ ! -f "$CONFIG" ]; then
  echo "ERR: $CONFIG not found" >&2
  exit 1
fi

# Validate config JSON shape.
if ! jq -e '.strict and (.contexts | type == "array" and length > 0)' "$CONFIG" >/dev/null 2>&1; then
  echo "ERR: $CONFIG missing required keys (.strict bool, .contexts non-empty array)" >&2
  exit 1
fi

echo "=== Plan ==="
echo "repo:   $REPO"
echo "config: $CONFIG"
echo "mode:   $([ "$APPLY" = "1" ] && echo apply || echo dry-run)"
echo ""

echo "current required_status_checks.contexts:"
if current_json=$(gh api "repos/$REPO/branches/main/protection/required_status_checks" 2>/dev/null); then
  echo "$current_json" | jq -r '.contexts[]' | sed 's/^/  - /'
else
  echo "  (none configured yet, or main not protected)"
fi
echo ""

echo "target required_status_checks.contexts:"
jq -r '.contexts[]' "$CONFIG" | sed 's/^/  + /'
echo ""

if [ "$APPLY" = "1" ]; then
  echo "applying PATCH /repos/$REPO/branches/main/protection/required_status_checks..."
  gh api -X PATCH "repos/$REPO/branches/main/protection/required_status_checks" \
    --input "$CONFIG" >/dev/null
  echo "applied"
else
  echo "[dry-run] would PATCH /repos/$REPO/branches/main/protection/required_status_checks"
  echo "          (run with --apply to write)"
fi
