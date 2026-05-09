.PHONY: all build test gate lint fmt clean help hooks-install hooks-status import-check dep-check golden golden-update

BIN_DIR := bin
BIN_NAME := opendbx
GO := go
GO_BUILD_FLAGS := -trimpath -ldflags="-s -w -X github.com/sqlrush/opendbx/internal/platform/version.Version=$(shell git describe --tags --always --dirty 2>/dev/null || echo 'dev')"

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
gate: import-check dep-check golden ## Local layer-2 gate (must pass before push)
	@echo "=== Layer-2 Gate ==="
	gofmt -l . | tee /tmp/opendbx-fmt.txt && [ ! -s /tmp/opendbx-fmt.txt ] || (echo "gofmt failed" && exit 1)
	$(GO) vet ./...
	$(GO) mod tidy && git diff --exit-code go.mod go.sum 2>/dev/null || (echo "go.mod/go.sum dirty after tidy" && exit 1)
	@command -v golangci-lint >/dev/null 2>&1 && golangci-lint run --timeout 5m || echo "golangci-lint not installed (skip in bootstrap)"
	CGO_ENABLED=0 $(GO) build ./...
	$(GO) test -race ./...
	@echo "=== Layer-2 Gate PASSED ==="

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
