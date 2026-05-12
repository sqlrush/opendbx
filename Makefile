.PHONY: all build test gate lint fmt clean help hooks-install hooks-status import-check dep-check golden golden-update gen-docs cc-help-diff

BIN_DIR := bin
BIN_NAME := opendbx
GO := go

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

all: build

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

bench: ## Run benchmarks
	$(GO) test -bench=. -benchmem -run=^$$ -count=1 ./...

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
	$(MAKE) coverage-gate
	@echo "=== Layer-2 Gate PASSED ==="

# spec-0.8 D-1 / T-4: enforce CLAUDE.md 规则 8 per-package coverage thresholds.
#
# Tiers (R2 用户拍板 CRIT-A):
#   core (≥85%): errcode / logger / version
#   other (≥75%): everything not core/exempt
#   exempt: entrypoints / tools/* / cmd/opendbx / config / rpc
#   total (≥80%): aggregated over non-exempt packages
#
# coverage-gate runs `go test -race -coverprofile=...` internally — gate
# uses this as the unit-test step too, so the regular `go test -race` line
# was removed from the recipe above.
#
# Emergency override: `COVERAGE_GATE_SKIP=1 make coverage-gate` (Q11 ★A;
# usage MUST be noted in CHANGELOG).
COVERAGE_PROFILE := /tmp/opendbx-coverage.out
.PHONY: coverage-gate
coverage-gate: ## Run go test -coverprofile + enforce per-package thresholds (spec-0.8 D-1)
	@echo "=== gate: coverage-gate ==="
	$(GO) test -race -coverprofile=$(COVERAGE_PROFILE) ./...
	$(GO) run ./tools/coverage-gate -profile=$(COVERAGE_PROFILE)

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

gen-docs: ## Regenerate opendbrb docs/error-codes.md from live errcode registry
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
