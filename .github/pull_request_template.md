## Spec / Feature

- **Spec id**: spec-X.Y-... (or `N/A` for chore/ci/review-only PRs)
- **Catalog id** (cross-reference master-test-catalog if business spec): L-N / S-N / ST-N / U-N / D-N / E-N / P-N / C-N

## Cross-repo link (双仓 PR 协议 § 1.4)

- 配对的 opendbx 代码 PR: TODO（如有）
- 配对的 opendbrb 设计 PR: 自身

## Checklist

- [ ] CHANGELOG.md 已更新（type ∈ {feat, fix, refactor, test, perf, spec} 必；新增行含 spec id）
- [ ] development-roadmap.md 状态从 ⬜ → 🟡 / ✅（spec-typed PR 必）
- [ ] master-test-catalog id 已 cross-reference 在 spec § 4 / § 11（业务 spec）或写 `N/A: meta spec`（meta spec）
- [ ] 共用组件改动（render/block/* / components/* / scrollback/* / streaming/*）列出所有调用方 + 各调用方 5 层 UI Review 结果（CLAUDE 规则 21 + § 3.9）
- [ ] Claude review trace 在 PR comment（claude）
- [ ] codex review trace 在 PR comment（codex）

## Test plan

- [ ] unit 测试（go test -race，≥ N table-driven cases）
- [ ] integration 测试（如适用）
- [ ] E2E 测试（如适用）
- [ ] perf benchmark vs baseline（如适用）

## 备注

TODO

---
🤖 Generated with [Claude Code](https://claude.com/claude-code)
