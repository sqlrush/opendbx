# opendbx top-level Makefile
# ============================
#
# spec-0.8 D-7 / T-11 — doc block satisfies binary criterion: ≥ 3 categories
# + ≥ 1 cross-repo dependency + ≥ 1 GNU make / bash requirement.
#
# Categories:
#
#   用户日常:  build / test / gate / gate-fast / fmt / lint / bench
#   CI 投影:   import-check / dep-check / golden / coverage-gate / makefile-check
#   release:   tag-spec (cross-repo dual-tag) / release (stub → spec-5.1)
#   维护:      hooks-install / hooks-status / gen-docs / cc-help-diff / clean
#
# Cross-repo dependencies:
#   tag-spec / gen-docs / cc-help-diff / makefile-check 都需要 sibling
#   $(OPENDBRB_DIR) (default ../opendbrb; override via env).
#
# Platform:
#   Requires GNU make + bash 3.2+ + git. macOS / Linux only; Windows not
#   supported (spec-0.7 § 2.3). All shell recipes avoid bash 4+ features
#   (no globstar `**`) — macOS bash 3.2 broke spec-0.7 T-14 dogfood on `**`.

# T-10a fix per spec-0.8 R2 MED-1: .PHONY split across multiple single-line
# declarations (makefile-check tool rejects backslash continuation).
.PHONY: all build test test-cover gate gate-fast lint fmt bench clean help
.PHONY: hooks-install hooks-status import-check dep-check
.PHONY: golden golden-update gen-docs cc-help-diff
.PHONY: coverage-gate makefile-check tag-spec release registry-drift-check
.PHONY: vuln-check ci-script-check sync-branch-protection

BIN_DIR := bin
BIN_NAME := opendbx
GO := go

# spec-0.8 D-3 / T-6: cross-repo sibling path for tag-spec / sync-registry.
# Default ../opendbrb; override with `make tag-spec OPENDBRB_DIR=~/work/opendbrb ...`
# when clone path differs.
OPENDBRB_DIR ?= ../opendbrb

# spec-0.7 D-3 / T-5: linker -X 注入 4 字段构建元数据 (Version / Commit /
# BuildDate / Dirty). Supported platforms: linux, darwin (POSIX shell + git +
# date -u). Windows not a target for ldflag metadata injection.
#
# Key decisions (claude MED-3 + codex MED 整合):
# - VERSION: `git describe --tags --always` -- 去掉 --dirty 后缀, 让 Version
#   保持 Parse-able tag 字符串; dirty 状态由 DIRTY 独立变量携带 (MED-3).
# - COMMIT: `git rev-parse --short=12 HEAD` -- 显式 12-char short hash.
# - BUILD_DATE: ISO8601 UTC (matches spec-0.5 logger sidecar timestamp 风格).
# - DIRTY: `git status --porcelain` -- 捕获 tracked + untracked changes
#   (diff-index 漏 untracked, codex MED).
VERSION    := $(shell git describe --tags --always 2>/dev/null || echo 'dev')
COMMIT     := $(shell git rev-parse --short=12 HEAD 2>/dev/null || echo 'unknown')
BUILD_DATE := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
DIRTY      := $(shell git status --porcelain 2>/dev/null | awk 'END { if (NR>0) print "dirty"; else print "" }')

VERSION_PKG := github.com/sqlrush/opendbx/internal/platform/version
LDFLAGS := -s -w \
    -X $(VERSION_PKG).Version=$(VERSION) \
    -X $(VERSION_PKG).Commit=$(COMMIT) \
    -X $(VERSION_PKG).BuildDate=$(BUILD_DATE) \
    -X $(VERSION_PKG).Dirty=$(DIRTY)
GO_BUILD_FLAGS := -trimpath -ldflags="$(LDFLAGS)"

all: build ## Default: build the opendbx binary

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*## ' $(MAKEFILE_LIST) | awk -F':.*## ' '{printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2}'

build: ## Build opendbx binary into bin/
	@mkdir -p $(BIN_DIR)
	$(GO) build $(GO_BUILD_FLAGS) -o $(BIN_DIR)/$(BIN_NAME) ./cmd/opendbx

test: ## Run all tests
	$(GO) test -race ./...

test-cover: ## Run tests with coverage report
	$(GO) test -race -coverprofile=coverage.out ./...
	$(GO) tool cover -func=coverage.out | tail -1

lint: ## Run golangci-lint
	golangci-lint run --timeout 5m

fmt: ## Format code
	gofmt -w .
	$(GO) mod tidy

# spec-0.8 D-2 / T-5: bench WARN-mode in gate.
#
# BENCH_TIMEOUT defaults to 2m (Q10 ★A); override via env: `BENCH_TIMEOUT=5m make bench`.
# Output captured to BENCH_OUTPUT for spec-0.11 baseline comparison.
#
# WARN signal format (claude MED-5 + Q2 ★A): any anomaly (timeout, parse
# error, no benchmarks) emits `BENCH_WARN: <reason>` to stderr and exits 0.
# spec-0.11 will grep `BENCH_WARN:` to flip to FAIL semantics with baselines.
# T-13c codex MED-2/LOW-2: BENCH_OUTPUT default aligned with spec D-2 (`bench.out`).
# `*.out` is in .gitignore so this won't pollute the repo. Env override
# allowed: `BENCH_OUTPUT=/tmp/foo.out make bench`.
BENCH_TIMEOUT ?= 2m
BENCH_OUTPUT  ?= bench.out

# T-13c codex MED-1: previous `| tee $(BENCH_OUTPUT)` made `rc=$$?` capture
# tee's exit (≈ always 0), masking genuine `go test` failures. Redirect
# first, then `cat` for visibility; rc now reflects `go test` truthfully.
bench: ## Run benchmarks -> bench.out (WARN; spec-0.11 -> FAIL)
	@echo "=== bench (BENCH_TIMEOUT=$(BENCH_TIMEOUT)) ==="
	@set +e; \
	$(GO) test -bench=. -benchmem -run=^$$$$ -count=1 -timeout=$(BENCH_TIMEOUT) ./... > $(BENCH_OUTPUT) 2>&1; \
	rc=$$?; \
	cat $(BENCH_OUTPUT); \
	if [ $$rc -ne 0 ]; then \
		echo "BENCH_WARN: bench exit code $$rc (timeout / parse error / runtime panic); see $(BENCH_OUTPUT)" >&2; \
		exit 0; \
	fi; \
	if ! grep -q '^Benchmark' $(BENCH_OUTPUT); then \
		echo "BENCH_WARN: no benchmarks present in this codebase (Stage 0 expected; spec-1.4+ adds perf baselines)" >&2; \
	fi

# gate-fast: skip the expensive coverage + bench steps for quick dev iteration.
# It does NOT replace push-time `make gate`; spec-0.9 CI should use the full gate.
.PHONY: gate-fast
gate-fast: import-check dep-check golden ## Fast dev gate (skip coverage + bench; not for push)
	@echo "=== gate-fast (no coverage / no bench) ==="
	gofmt -l . | tee /tmp/opendbx-fmt.txt && [ ! -s /tmp/opendbx-fmt.txt ] || (echo "gofmt failed" && exit 1)
	$(GO) vet ./...
	CGO_ENABLED=0 $(GO) build ./...
	$(GO) test -race ./...
	@echo "=== gate-fast PASSED (push 前请跑 make gate 全套) ==="

# spec-0.8 D-3 / T-6: tag-spec wrapper.
#
# Forwards to ../opendbrb/scripts/release/tag-spec.sh (spec-0.7 D-2 SSOT).
# Both repos get this target (Q3 ★A); opendbrb side ships in T-8 / D-5.
#
# Env vars (read by underlying script): DRY_RUN / OPENDBX_TAG_REPAIR
# Make vars → script flags: STAGE_ACCEPTED / FORCE_DIRTY / REPAIR_PEER
#
# Examples:
#   make tag-spec SPEC=spec-0.8-makefile-build
#   DRY_RUN=1 make tag-spec SPEC=spec-0.8-makefile-build
#   make tag-spec SPEC=spec-0.16-stage0-acceptance STAGE_ACCEPTED=1
#   make tag-spec SPEC=spec-0.8-... REPAIR_PEER=1 OPENDBX_TAG_REPAIR=1
.PHONY: tag-spec
tag-spec: ## FROZEN-tag a spec via opendbrb tag-spec.sh (SPEC=... req)
	@[ -n "$(SPEC)" ] || (echo "ERR: SPEC= required (e.g. make tag-spec SPEC=spec-0.8-makefile-build)" >&2; exit 1)
	@[ -d "$(OPENDBRB_DIR)" ] || (echo "ERR: $(OPENDBRB_DIR) not found; clone opendbrb sibling or override OPENDBRB_DIR=..." >&2; exit 1)
	@[ -x "$(OPENDBRB_DIR)/scripts/release/tag-spec.sh" ] || (echo "ERR: $(OPENDBRB_DIR)/scripts/release/tag-spec.sh missing or not executable" >&2; exit 1)
	@DRY_RUN=$(DRY_RUN) OPENDBX_TAG_REPAIR=$(OPENDBX_TAG_REPAIR) \
		$(OPENDBRB_DIR)/scripts/release/tag-spec.sh $(SPEC) \
		$(if $(filter 1,$(STAGE_ACCEPTED)),--stage-accepted,) \
		$(if $(filter 1,$(FORCE_DIRTY)),--force-dirty,) \
		$(if $(filter 1,$(REPAIR_PEER)),--repair-missing-peer,)

# spec-0.8 D-4 / T-7: release pipeline placeholder.
#
# Real release flow (GoReleaser + multi-arch + GitHub Release body) lands
# in spec-5.1. Until then, this target exits 1 with a clear error so:
#   - CI never accidentally "succeeds" a release that didn't actually run
#   - Local users see explicit pointer to current alternative (make build)
#   - spec-0.9 ci-github-actions MUST NOT call this target (CI 调用必须红)
#
# Q4 ★A (R2): stub-fail vs no-op vs scaffold; stub-fail picked because
# silent no-op breaks the worst (CI thinks release pipeline ran).
.PHONY: release
release: ## STUB - release lands in spec-5.1; CI must not call
	@echo "ERROR: 'make release' is a stub. The real release pipeline" >&2
	@echo "       (GoReleaser + multi-arch binary + GitHub Release body)" >&2
	@echo "       lands in spec-5.1-release-pipeline." >&2
	@echo "" >&2
	@echo "  For local single-binary build:" >&2
	@echo "    make build                                    # → bin/opendbx" >&2
	@echo "" >&2
	@echo "  For tagging a spec FROZEN (spec-0.7 dual-repo automation):" >&2
	@echo "    make tag-spec SPEC=spec-X.Y-<slug>" >&2
	@echo "" >&2
	@echo "  spec-0.9 ci-github-actions: do NOT invoke 'make release'." >&2
	@exit 1

# Layer-2 gate: 所有这些命令必须 PASS 才允许 push
# 详见设计仓 docs/cicd-and-methodology.md § 2
# Layer-2 gate runs cheap checks first (fmt/vet/tidy/lint/import/dep/golden/
# build), then the expensive coverage-gate step (which itself runs
# `go test -race -coverprofile=...` and enforces CLAUDE.md 规则 8 thresholds).
# spec-0.8 D-1 / T-4.
#
# Prereqs run before recipe (Make semantics), so import-check / dep-check /
# golden run FIRST. The recipe then runs fmt/vet/tidy/lint/build inline,
# and finally invokes coverage-gate (which subsumes the prior `go test -race`).
gate: import-check dep-check golden ## Local layer-2 gate (must pass before push)
	@echo "=== Layer-2 Gate ==="
	gofmt -l . | tee /tmp/opendbx-fmt.txt && [ ! -s /tmp/opendbx-fmt.txt ] || (echo "gofmt failed" && exit 1)
	$(GO) vet ./...
	$(GO) mod tidy && git diff --exit-code go.mod go.sum 2>/dev/null || (echo "go.mod/go.sum dirty after tidy" && exit 1)
	@command -v golangci-lint >/dev/null 2>&1 && golangci-lint run --timeout 5m || echo "golangci-lint not installed (skip in bootstrap)"
	CGO_ENABLED=0 $(GO) build ./...
	$(MAKE) makefile-check
	$(MAKE) registry-drift-check
	$(MAKE) ci-script-check
	$(MAKE) coverage-gate
	$(MAKE) bench
	@echo "=== Layer-2 Gate PASSED ==="

# T-13c codex HIGH-1 / claude HIGH-1: sibling-aware delegation to opendbrb's
# registry-drift-check (data-row comparison between SSOT and hook-local copy).
# Silently skipped if sibling absent — gate remains useful for opendbx-only
# clones. spec-0.8 D-5 explicitly requires this in opendbx gate.
registry-drift-check: ## Detect drift vs opendbrb/specs/spec-registry.txt SSOT
	@echo "=== gate: registry-drift-check ==="
	@if [ -f "$(OPENDBRB_DIR)/Makefile" ]; then \
		$(MAKE) -C "$(OPENDBRB_DIR)" registry-drift-check OPENDBX_DIR="$(CURDIR)"; \
	else \
		echo "skip (sibling $(OPENDBRB_DIR) not present)"; \
	fi

# spec-0.8 D-1 / T-4: enforce CLAUDE.md 规则 8 per-package coverage thresholds.
#
# Tiers (R2 用户拍板 CRIT-A + T-13 tool-tier errata):
#   core (≥85%): errcode / logger / version
#   tool (≥90%): coverage-gate / makefile-check
#   other (≥75%): everything not core/exempt
#   exempt: entrypoints / import-rules-check / dep-allowlist-check /
#           import-rules-check/rules / cmd/opendbx / config / rpc
#   total (≥80%): aggregated over non-exempt packages
#
# coverage-gate runs `go test -race -coverprofile=...` internally — gate
# uses this as the unit-test step too, so the regular `go test -race` line
# was removed from the recipe above.
#
# Emergency override: `COVERAGE_GATE_SKIP=1 make coverage-gate` (Q11 ★A;
# usage MUST be noted in CHANGELOG).
COVERAGE_PROFILE := /tmp/opendbx-coverage.out
coverage-gate: ## Coverage gate: per-package thresholds (spec-0.8 D-1)
	@echo "=== gate: coverage-gate ==="
	$(GO) test -race -coverprofile=$(COVERAGE_PROFILE) ./...
	$(GO) run ./tools/coverage-gate -profile=$(COVERAGE_PROFILE)

# spec-0.8 D-6 / T-10b: Makefile lint. Scans this Makefile + sibling
# opendbrb/Makefile (if present) for the 5 conventions defined in
# tools/makefile-check (help comment / .PHONY / kebab-lower / no-dup /
# doc-block + .PHONY no-continuation). Sibling skip is silent — gate
# remains useful for opendbx-only clones.
makefile-check: ## Lint top-level Makefile(s) + sibling if present
	@echo "=== gate: makefile-check ==="
	@files="Makefile"; \
	if [ -f "$(OPENDBRB_DIR)/Makefile" ]; then files="$$files $(OPENDBRB_DIR)/Makefile"; fi; \
	$(GO) run ./tools/makefile-check $$files

# spec-0.9 D-2.5 / T-3.5: govulncheck + OSV allowlist 包装 (R2 codex HIGH-3 修).
#
# govulncheck 本身无 inline 豁免机制; 直 fail on first finding 不可接受 (Stage 0
# Go 1.23 lock vs Go 1.25.8 fix for GO-2026-4602 stdlib vuln). 包装脚本读
# tools/vuln-allowlist/allowlist.json (OSV ID + expiry + spec_ref) 过滤已知豁免.
# 过期或未豁免 finding 强制 fail.
GOVULN_VERSION ?= v1.1.4
vuln-check: ## Run govulncheck filtered by OSV allowlist (spec-0.9 D-2.5)
	@command -v govulncheck >/dev/null 2>&1 || $(GO) install golang.org/x/vuln/cmd/govulncheck@$(GOVULN_VERSION)
	@govulncheck -json -test ./... | $(GO) run ./tools/vuln-allowlist

# spec-0.9 D-5 / T-7: ci.yml ↔ branch-protection JSON 1:1 drift check.
ci-script-check: ## Detect ci.yml vs branch-protection JSON drift (D-5)
	@$(GO) run ./tools/ci-protection-check

# spec-0.9 D-5 / T-7: PATCH /required_status_checks 窄端点同步.
sync-branch-protection: ## Sync branch protection contexts (dry-run; APPLY=1)
	@bash scripts/ci/sync-branch-protection.sh $(if $(filter 1,$(APPLY)),--apply,--dry-run)

# spec-0.2 governance gates (D-5 / D-6 / D-3) — see docs/cicd-and-methodology.md
import-check: ## Run import-rules-check (spec-0.2 D-5)
	$(GO) run ./tools/import-rules-check -v .

dep-check: ## Run dep-allowlist-check (spec-0.2 D-6)
	$(GO) run ./tools/dep-allowlist-check -v .

golden: ## Run CLI text golden tests (spec-0.2 D-3)
	$(GO) test -race -run 'TestGolden|TestSubcommandStubs' ./cmd/opendbx/...

golden-update: ## Regenerate CLI golden files
	TEST_UPDATE_GOLDEN=1 $(GO) test -run TestGolden ./cmd/opendbx/...
	@echo "goldens updated. Review with 'git diff cmd/opendbx/testdata/golden/'"

gen-docs: ## Regenerate opendbrb docs/error-codes.md from registry
	$(GO) run cmd/tools/gen-error-codes/main.go --out=../opendbrb/docs/error-codes.md

# spec-0.3 D-6: drift check vs CC v2.1.138 baseline. Doesn't fail; surfaces
# a unified diff that humans review (ad hoc when CC upgrades) per user D8 +
# D13 decisions.
CC_HELP_BASELINE := ../opendbrb/docs/cc-help-baseline-v2.1.138.txt
cc-help-diff: build ## Diff opendbx --help against the CC help baseline
	@echo "=== opendbx --help vs $(CC_HELP_BASELINE) ==="
	@if [ ! -f "$(CC_HELP_BASELINE)" ]; then \
		echo "ERR: baseline not found: $(CC_HELP_BASELINE)"; \
		echo "     (per user D3 + D8: lock to local CC version; do not chase latest)"; \
		exit 1; \
	fi
	@./$(BIN_DIR)/$(BIN_NAME) --help > /tmp/opendbx-help.txt 2>&1
	@echo "(exit code below is from diff: 0=identical, 1=differences exist, 2=error)"
	@diff -u $(CC_HELP_BASELINE) /tmp/opendbx-help.txt || true
	@echo ""
	@echo "Differences are expected (DB-flavored adaptations + opendbx-specific commands)."
	@echo "Curated rationale: ../opendbrb/docs/cc-vs-opendbx-help-diff.md"

clean: ## Remove build artifacts
	rm -rf $(BIN_DIR) coverage.out *.prof

# ===== git hooks (spec-0.1 D-8) =====

GIT_HOOKS_DIR := $(shell git rev-parse --git-dir 2>/dev/null)/hooks
SRC_HOOKS := $(wildcard git-hooks/*)

hooks-install: ## Install repo git hooks into .git/hooks/ (idempotent)
	@if [ -z "$(GIT_HOOKS_DIR)" ]; then \
		echo "ERR: not in a git repo"; exit 1; \
	fi
	@for h in $(SRC_HOOKS); do \
		name=$$(basename "$$h"); \
		dest="$(GIT_HOOKS_DIR)/$$name"; \
		ln -sf "$(CURDIR)/$$h" "$$dest"; \
		echo "linked $$h -> $$dest"; \
	done
	@echo ""
	@echo "git hooks installed."
	@echo "next: build commit-lint binary so the hook can find it:"
	@echo "  cd ../opendbrb/scripts/opendbrb-commit-lint && go install ."
	@echo "  (or build to /tmp/opendbrb-commit-lint as a fallback)"

hooks-status: ## Show installed git hooks
	@if [ -z "$(GIT_HOOKS_DIR)" ]; then \
		echo "ERR: not in a git repo"; exit 1; \
	fi
	@for h in $(SRC_HOOKS); do \
		name=$$(basename "$$h"); \
		dest="$(GIT_HOOKS_DIR)/$$name"; \
		if [ -L "$$dest" ]; then \
			target=$$(readlink "$$dest"); \
			echo "OK    $$name -> $$target"; \
		elif [ -e "$$dest" ]; then \
			echo "FILE  $$name (not symlink — manually managed?)"; \
		else \
			echo "MISS  $$name (run 'make hooks-install')"; \
		fi; \
	done
