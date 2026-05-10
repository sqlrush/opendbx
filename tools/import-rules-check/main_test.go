// Copyright 2026 opendbx contributors. See LICENSE.
//
// Tests for import-rules-check: 50+ table-driven cases covering layer
// matrix + cluster restrictions + render strict DAG (spec-0.2 § 4.1).
//
// Author: sqlrush
package main

import (
	"strings"
	"testing"

	"github.com/sqlrush/opendbx/tools/import-rules-check/rules"
)

// ---- Layer classification ----

func TestPathToLayer(t *testing.T) {
	cases := []struct {
		path string
		want rules.Layer
	}{
		// stdlib
		{"fmt", rules.LayerStdlib},
		{"encoding/json", rules.LayerStdlib},
		{"os/exec", rules.LayerStdlib},
		{"io", rules.LayerStdlib},
		{"context", rules.LayerStdlib},
		{"strings", rules.LayerStdlib},
		// unknown opendbx-internal path
		{"github.com/sqlrush/opendbx/internal/foo", rules.LayerUnknown},
		// pkg + tests sub-paths
		{"github.com/sqlrush/opendbx/pkg/skillsdk/v2", rules.LayerPkg},
		{"github.com/sqlrush/opendbx/tests/e2e/uitest-visual", rules.LayerTests},
		{"github.com/sqlrush/opendbx/tools/dep-allowlist-check", rules.LayerTools},
		// external
		{"golang.org/x/tools/go/packages", rules.LayerExternal},
		{"github.com/jackc/pgx/v5", rules.LayerExternal},
		{"github.com/sqlrush/somethingelse", rules.LayerExternal},
		// opendbx layers
		{"github.com/sqlrush/opendbx/cmd/opendbx", rules.LayerCmd},
		{"github.com/sqlrush/opendbx/internal/entrypoints", rules.LayerEntrypoints},
		{"github.com/sqlrush/opendbx/internal/entrypoints/admin", rules.LayerEntrypoints},
		{"github.com/sqlrush/opendbx/internal/bootstrap", rules.LayerBootstrap},
		{"github.com/sqlrush/opendbx/internal/bootstrap/wire", rules.LayerBootstrap},
		{"github.com/sqlrush/opendbx/internal/app/cli/tui", rules.LayerApp},
		{"github.com/sqlrush/opendbx/internal/app/services/mcp", rules.LayerApp},
		{"github.com/sqlrush/opendbx/internal/app/cli/render/buffer", rules.LayerApp},
		{"github.com/sqlrush/opendbx/internal/domain/db", rules.LayerDomain},
		{"github.com/sqlrush/opendbx/internal/domain/db/postgres", rules.LayerDomain},
		{"github.com/sqlrush/opendbx/internal/domain/llm/anthropic", rules.LayerDomain},
		{"github.com/sqlrush/opendbx/internal/platform/config", rules.LayerPlatform},
		{"github.com/sqlrush/opendbx/internal/platform/version", rules.LayerPlatform},
		{"github.com/sqlrush/opendbx/internal/platform/migrations", rules.LayerPlatform},
		{"github.com/sqlrush/opendbx/tools/import-rules-check/rules", rules.LayerTools},
		{"github.com/sqlrush/opendbx/pkg/skillsdk", rules.LayerPkg},
		{"github.com/sqlrush/opendbx/tests/integration", rules.LayerTests},
	}
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			got := rules.PathToLayer(tc.path)
			if got != tc.want {
				t.Errorf("PathToLayer(%q) = %v, want %v", tc.path, got, tc.want)
			}
		})
	}
}

// ---- Layer matrix ----

func TestCheckEdge_LayerMatrix(t *testing.T) {
	const M = "github.com/sqlrush/opendbx/"

	cases := []struct {
		name        string
		from, to    string
		wantOK      bool
		wantContain string // substring required when wantOK==false
	}{
		// stdlib always allowed
		{"app→stdlib", M + "internal/app/cli/tui", "fmt", true, ""},
		{"platform→stdlib", M + "internal/platform/config", "encoding/json", true, ""},

		// cmd allowed
		{"cmd→entrypoints", M + "cmd/opendbx", M + "internal/entrypoints", true, ""},
		{"cmd→stdlib", M + "cmd/opendbx", "io", true, ""},

		// cmd → platform/version (UNIQUE EXCEPTION)
		{"cmd→platform/version", M + "cmd/opendbx", M + "internal/platform/version", true, ""},
		// cmd → platform/version subpkg (NOT covered by exception — exact match only)
		{"cmd→platform/version/sub_FAIL", M + "cmd/opendbx", M + "internal/platform/version/build", false, "cmd may import only"},
		// cmd → other platform = FAIL
		{"cmd→platform/config_FAIL", M + "cmd/opendbx", M + "internal/platform/config", false, "cmd may import only"},
		{"cmd→platform/apperr_FAIL", M + "cmd/opendbx", M + "internal/platform/apperr", false, "cmd may import only"},
		{"cmd→platform/migrations_FAIL", M + "cmd/opendbx", M + "internal/platform/migrations", false, "migrations"},

		// schemas global-read (spec § 2.2 重要细则 #2): any layer may import schemas
		{"platform→schemas_OK", M + "internal/platform/apperr", M + "internal/domain/schemas", true, ""},
		{"cmd→schemas_OK", M + "cmd/opendbx", M + "internal/domain/schemas", true, ""},
		{"app→schemas_OK", M + "internal/app/diagnose", M + "internal/domain/schemas", true, ""},
		{"entrypoints→schemas_OK", M + "internal/entrypoints/admin", M + "internal/domain/schemas", true, ""},
		// schemas sibling path (boundary check): platform → domain/schemas-foo should still fail with the regular layer rule
		{"platform→schemas_sibling_FAIL", M + "internal/platform/apperr", M + "internal/domain/schemas-foo", false, "not allowed"},

		// cmd → other layers = FAIL
		{"cmd→app_FAIL", M + "cmd/opendbx", M + "internal/app/cli/tui", false, "not allowed"},
		{"cmd→domain_FAIL", M + "cmd/opendbx", M + "internal/domain/db", false, "not allowed"},
		{"cmd→bootstrap_FAIL", M + "cmd/opendbx", M + "internal/bootstrap", false, "not allowed"},

		// entrypoints
		{"entrypoints→bootstrap", M + "internal/entrypoints/admin", M + "internal/bootstrap", true, ""},
		{"entrypoints→platform", M + "internal/entrypoints", M + "internal/platform/config", true, ""},
		{"entrypoints→app_FAIL", M + "internal/entrypoints", M + "internal/app/cli/tui", false, "not allowed"},
		{"entrypoints→domain_FAIL", M + "internal/entrypoints", M + "internal/domain/db", false, "not allowed"},

		// bootstrap
		{"bootstrap→app", M + "internal/bootstrap", M + "internal/app/diagnose", true, ""},
		{"bootstrap→domain", M + "internal/bootstrap", M + "internal/domain/db", true, ""},
		{"bootstrap→platform", M + "internal/bootstrap", M + "internal/platform/version", true, ""},
		{"bootstrap→migrations", M + "internal/bootstrap", M + "internal/platform/migrations", true, ""},
		{"bootstrap→cmd_FAIL", M + "internal/bootstrap", M + "cmd/opendbx", false, "not allowed"},

		// app
		{"app→domain", M + "internal/app/diagnose", M + "internal/domain/llm", true, ""},
		{"app→platform", M + "internal/app/diagnose", M + "internal/platform/logger", true, ""},
		{"app→app_same_layer", M + "internal/app/diagnose", M + "internal/app/sentinel", true, ""},
		{"app→bootstrap_FAIL", M + "internal/app/diagnose", M + "internal/bootstrap", false, "not allowed"},
		{"app→entrypoints_FAIL", M + "internal/app/diagnose", M + "internal/entrypoints", false, "not allowed"},
		{"app→cmd_FAIL", M + "internal/app/diagnose", M + "cmd/opendbx", false, "not allowed"},

		// domain
		{"domain→platform", M + "internal/domain/db", M + "internal/platform/logger", true, ""},
		{"domain→domain_same_layer", M + "internal/domain/db", M + "internal/domain/security", true, ""},
		{"domain→app_FAIL", M + "internal/domain/db", M + "internal/app/diagnose", false, "not allowed"},
		{"domain→bootstrap_FAIL", M + "internal/domain/db", M + "internal/bootstrap", false, "not allowed"},

		// platform
		{"platform→platform", M + "internal/platform/config", M + "internal/platform/logger", true, ""},
		{"platform→domain_FAIL", M + "internal/platform/config", M + "internal/domain/db", false, "not allowed"},
		{"platform→app_FAIL", M + "internal/platform/config", M + "internal/app/diagnose", false, "not allowed"},

		// migrations strict gating: only bootstrap may import (any other from-layer fails)
		{"app→migrations_FAIL", M + "internal/app/diagnose", M + "internal/platform/migrations", false, "migrations may only"},
		{"domain→migrations_FAIL", M + "internal/domain/db", M + "internal/platform/migrations", false, "migrations may only"},
		{"platform→migrations_FAIL", M + "internal/platform/version", M + "internal/platform/migrations", false, "migrations may only"},
		{"entrypoints→migrations_FAIL", M + "internal/entrypoints/admin", M + "internal/platform/migrations", false, "migrations may only"},
		{"app→migrations_subpkg_FAIL", M + "internal/app/diagnose", M + "internal/platform/migrations/sql", false, "migrations may only"},
		// migrations sibling path (boundary check): app → platform/migrations2 should be a normal layer rule (which IS allowed app→platform)
		{"app→migrations_sibling_OK", M + "internal/app/diagnose", M + "internal/platform/migrations2", true, ""},

		// tools
		{"tools→stdlib", M + "tools/import-rules-check", "fmt", true, ""},
		{"tools→external", M + "tools/import-rules-check", "golang.org/x/tools/go/packages", true, ""},

		// tests
		{"tests→app", M + "tests/integration", M + "internal/app/diagnose", true, ""},
		{"tests→domain", M + "tests/integration", M + "internal/domain/db", true, ""},
		{"tests→platform", M + "tests/integration", M + "internal/platform/config", true, ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := rules.CheckEdge(tc.from, tc.to)
			if tc.wantOK {
				if got != "" {
					t.Errorf("CheckEdge(%q, %q) = %q, want OK", tc.from, tc.to, got)
				}
				return
			}
			if got == "" {
				t.Errorf("CheckEdge(%q, %q) = OK, want failure containing %q", tc.from, tc.to, tc.wantContain)
				return
			}
			if !strings.Contains(got, tc.wantContain) {
				t.Errorf("CheckEdge(%q, %q) = %q, want containing %q", tc.from, tc.to, got, tc.wantContain)
			}
		})
	}
}

// ---- Cluster restrictions ----

func TestCheckCluster(t *testing.T) {
	const M = "github.com/sqlrush/opendbx/"

	cases := []struct {
		name        string
		from, to    string
		wantOK      bool
		wantContain string
	}{
		// services mutual exclusion
		{"services_mcp→auth_FAIL", M + "internal/app/services/mcp", M + "internal/app/services/auth", false, "services must communicate"},
		{"services_costtracker→notifier_FAIL", M + "internal/app/services/costtracker", M + "internal/app/services/notifier", false, "services must communicate"},
		{"services_self_OK", M + "internal/app/services/mcp", M + "internal/app/services/mcp/server", true, ""},
		{"services_self_util→sub_OK", M + "internal/app/services/mcp/util", M + "internal/app/services/mcp/sub", true, ""},
		{"services_to_app_other_OK", M + "internal/app/services/mcp", M + "internal/app/diagnose", true, ""},
		// db driver isolation
		{"db_postgres→mysql_FAIL", M + "internal/domain/db/postgres", M + "internal/domain/db/mysql", false, "DB drivers are isolated"},
		{"db_mysql→oracle_FAIL", M + "internal/domain/db/mysql", M + "internal/domain/db/oracle", false, "DB drivers are isolated"},
		{"db_postgres→postgres_self_OK", M + "internal/domain/db/postgres", M + "internal/domain/db/postgres/util", true, ""},
		{"db_postgres→db_root_OK", M + "internal/domain/db/postgres", M + "internal/domain/db", true, ""},
		// scrollback ↛ components (boundary-safe)
		{"scrollback→components_FAIL", M + "internal/app/cli/render/scrollback", M + "internal/app/cli/components", false, "scrollback is a render"},
		{"scrollback_sub→components_FAIL", M + "internal/app/cli/render/scrollback/internal", M + "internal/app/cli/components", false, "scrollback is a render"},
		// scrollback prefix-but-not-boundary sibling: render/scrollback-extra should NOT trip cluster rule
		{"scrollback_sibling_OK", M + "internal/app/cli/render/scrollback-extra", M + "internal/app/cli/components", true, ""},
		// unrelated edges pass
		{"unrelated_OK", M + "internal/app/diagnose", M + "internal/domain/llm", true, ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := rules.CheckCluster(tc.from, tc.to)
			if tc.wantOK {
				if got != "" {
					t.Errorf("CheckCluster(%q, %q) = %q, want OK", tc.from, tc.to, got)
				}
				return
			}
			if got == "" {
				t.Errorf("CheckCluster(%q, %q) = OK, want failure containing %q", tc.from, tc.to, tc.wantContain)
				return
			}
			if !strings.Contains(got, tc.wantContain) {
				t.Errorf("CheckCluster(%q, %q) = %q, want containing %q", tc.from, tc.to, got, tc.wantContain)
			}
		})
	}
}

// ---- Render strict DAG ----

func TestCheckRenderDAG(t *testing.T) {
	const R = "github.com/sqlrush/opendbx/internal/app/cli/render/"

	cases := []struct {
		name        string
		from, to    string
		wantOK      bool
		wantContain string
	}{
		// forward (downward in list, idx_from < idx_to) — OK
		{"terminal→buffer", R + "terminal", R + "buffer", true, ""},
		{"terminal→layout", R + "terminal", R + "layout", true, ""},
		{"terminal→width_long_jump", R + "terminal", R + "width", true, ""},
		{"buffer→layout", R + "buffer", R + "layout", true, ""},
		{"layout→optimizer", R + "layout", R + "optimizer", true, ""},
		{"optimizer→scheduler", R + "optimizer", R + "scheduler", true, ""},
		{"scheduler→scrollback", R + "scheduler", R + "scrollback", true, ""},
		{"scrollback→streaming", R + "scrollback", R + "streaming", true, ""},
		{"streaming→block", R + "streaming", R + "block", true, ""},
		{"block→style", R + "block", R + "style", true, ""},
		{"style→width", R + "style", R + "width", true, ""},

		// reverse (upward, idx_from >= idx_to) — FAIL
		{"buffer→terminal_FAIL", R + "buffer", R + "terminal", false, "render-DAG"},
		{"layout→buffer_FAIL", R + "layout", R + "buffer", false, "render-DAG"},
		{"block→scheduler_FAIL", R + "block", R + "scheduler", false, "render-DAG"},
		{"width→style_FAIL", R + "width", R + "style", false, "render-DAG"},
		{"width→terminal_FAIL", R + "width", R + "terminal", false, "render-DAG"},
		{"streaming→scheduler_FAIL", R + "streaming", R + "scheduler", false, "render-DAG"},

		// edges outside render/ are ignored
		{"non_render_from_diagnose", "github.com/sqlrush/opendbx/internal/app/diagnose", R + "buffer", true, ""},
		{"non_render_to_stdlib", R + "buffer", "fmt", true, ""},
		{"both_non_render", "fmt", "io", true, ""},

		// edges into render-with-subpkg also classify
		{"terminal_subpkg→buffer", R + "terminal/sub", R + "buffer", true, ""},
		{"buffer_subpkg→terminal_FAIL", R + "buffer/sub", R + "terminal", false, "render-DAG"},

		// unknown render subpackage hard-fails (must be added to DAG first)
		{"unknown_render_subpkg_from_FAIL", R + "newpkg/foo", R + "buffer", false, "not in RenderOrder"},
		{"unknown_render_subpkg_to_FAIL", R + "buffer", R + "futurepkg", false, "not in RenderOrder"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := rules.CheckRenderDAG(tc.from, tc.to)
			if tc.wantOK {
				if got != "" {
					t.Errorf("CheckRenderDAG(%q, %q) = %q, want OK", tc.from, tc.to, got)
				}
				return
			}
			if got == "" {
				t.Errorf("CheckRenderDAG(%q, %q) = OK, want failure containing %q", tc.from, tc.to, tc.wantContain)
				return
			}
			if !strings.Contains(got, tc.wantContain) {
				t.Errorf("CheckRenderDAG(%q, %q) = %q, want containing %q", tc.from, tc.to, got, tc.wantContain)
			}
		})
	}
}

// ---- End-to-end: scan the actual opendbx repo ----

func TestScan_RealRepo(t *testing.T) {
	// Locate opendbx repo root: this test file is at tools/import-rules-check/main_test.go.
	// Walk up two levels.
	root := "../../"
	violations, scanned, err := scan(root)
	if err != nil {
		t.Fatalf("scan(%q): %v", root, err)
	}
	if scanned < 50 {
		t.Errorf("scanned only %d packages, expected ≥ 50 (after scaffold)", scanned)
	}
	if len(violations) > 0 {
		t.Errorf("repo has %d violations:\n  %s", len(violations), strings.Join(violations, "\n  "))
	}
}
