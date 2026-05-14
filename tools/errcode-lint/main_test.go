// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package main

import (
	"bytes"
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"strings"
	"testing"
)

// --- Path 1: isExempt prefix logic -----------------------------------

func TestIsExempt(t *testing.T) {
	t.Parallel()
	cases := []struct {
		pkg  string
		want bool
	}{
		{"github.com/sqlrush/opendbx/internal/platform/errcode", true},
		{"github.com/sqlrush/opendbx/internal/platform/errcode/sub", true},
		{"github.com/sqlrush/opendbx/internal/entrypoints", true},
		{"github.com/sqlrush/opendbx/tools/errcode-lint", true},
		{"github.com/sqlrush/opendbx/tools/import-rules-check", true},
		{"github.com/sqlrush/opendbx/tools/import-rules-check/rules", true},
		{"github.com/sqlrush/opendbx/cmd/opendbx", true},
		{"github.com/sqlrush/opendbx/cmd/tools/gen-error-codes", true},
		{"github.com/sqlrush/opendbx/internal/platform/logger", false},
		{"github.com/sqlrush/opendbx/internal/platform/config", false},
		{"some/other/module", false},
	}
	for _, c := range cases {
		c := c
		t.Run(c.pkg, func(t *testing.T) {
			t.Parallel()
			if got := isExempt(c.pkg); got != c.want {
				t.Errorf("isExempt(%q) = %v; want %v", c.pkg, got, c.want)
			}
		})
	}
}

// --- Path 2: isErrorType detects predeclared error ------------------

func TestIsErrorType(t *testing.T) {
	t.Parallel()
	// Construct predeclared "error" type via the universe scope.
	errType := types.Universe.Lookup("error").Type()
	if !isErrorType(errType) {
		t.Errorf("predeclared error must be detected; got false")
	}
	if isErrorType(types.Typ[types.Int]) {
		t.Errorf("int must not be detected as error")
	}
	if isErrorType(nil) {
		t.Errorf("nil type must not be detected")
	}
}

// --- Path 3: callExprName extracts qualified name -------------------

func TestCallExprName(t *testing.T) {
	t.Parallel()
	cases := []struct {
		src  string
		want string
	}{
		{`package x; func _(){ Foo() }`, "Foo"},
		{`package x; import "errors"; func _(){ errors.New("x") }`, "errors.New"},
		{`package x; func _(){ pkg.Sub.Method() }`, "Method"},
	}
	for _, c := range cases {
		c := c
		t.Run(c.want, func(t *testing.T) {
			t.Parallel()
			ce := findFirstCall(t, c.src)
			if got := callExprName(ce); got != c.want {
				t.Errorf("got %q want %q", got, c.want)
			}
		})
	}
}

// findFirstCall returns the first *ast.CallExpr inside src.
func findFirstCall(t *testing.T, src string) *ast.CallExpr {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "x.go", src, 0)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var found *ast.CallExpr
	ast.Inspect(f, func(n ast.Node) bool {
		if found != nil {
			return false
		}
		if ce, ok := n.(*ast.CallExpr); ok {
			found = ce
			return false
		}
		return true
	})
	if found == nil {
		t.Fatalf("no call expr in src")
	}
	return found
}

// --- Path 4: isErrcodeConstructor / isBareErrorConstructor ----------

func TestIsErrcodeConstructor(t *testing.T) {
	t.Parallel()
	for _, src := range []string{
		`package x; import "errcode"; func _(){ errcode.New() }`,
		`package x; import "errcode"; func _(){ errcode.Newf() }`,
		`package x; import "errcode"; func _(){ errcode.Wrap() }`,
	} {
		ce := findFirstCall(t, src)
		if !isErrcodeConstructor(ce) {
			t.Errorf("expected true for %q", src)
		}
	}
	// Negative
	ce := findFirstCall(t, `package x; import "errors"; func _(){ errors.New("x") }`)
	if isErrcodeConstructor(ce) {
		t.Errorf("errors.New must not be errcode constructor")
	}
}

func TestIsBareErrorConstructor(t *testing.T) {
	t.Parallel()
	cases := []struct {
		src  string
		want Code
		ok   bool
	}{
		{`package x; import "errors"; func _(){ errors.New("x") }`, EC1, true},
		{`package x; import "fmt"; func _(){ fmt.Errorf("x") }`, EC2, true},
		{`package x; import "errcode"; func _(){ errcode.New("x") }`, "", false},
	}
	for _, c := range cases {
		ce := findFirstCall(t, c.src)
		got, ok := isBareErrorConstructor(ce)
		if got != c.want || ok != c.ok {
			t.Errorf("src=%q got (%q, %v); want (%q, %v)", c.src, got, ok, c.want, c.ok)
		}
	}
}

// --- Path 5: hasExemptComment -------------------------------------

func TestHasExemptComment(t *testing.T) {
	t.Parallel()
	src := `package x
// errcode-lint:exempt -- spec-0.10 D-2: test
var X = 1
`
	fset := token.NewFileSet()
	f, _ := parser.ParseFile(fset, "x.go", src, parser.ParseComments)
	// The comment is on line 2; the var decl is on line 3.
	if !hasExemptComment(f, fset, 3) {
		t.Errorf("exempt comment 1 line above should be detected")
	}
	if hasExemptComment(f, fset, 10) {
		t.Errorf("exempt comment too far should not be detected")
	}
}

// --- Path 6: Violation.String() ------------------------------------

func TestViolationString(t *testing.T) {
	t.Parallel()
	v := Violation{
		Pkg: "pkg/foo", File: "foo.go", Line: 42, Function: "Bar",
		Code: EC1, Message: "bare errors.New",
	}
	got := v.String()
	for _, want := range []string{"[EC-1]", "foo.go:42", "pkg/foo.Bar", "bare errors.New"} {
		if !strings.Contains(got, want) {
			t.Errorf("expected %q in %q", want, got)
		}
	}
}

// --- Path 7: end-to-end Lint on testdata fixtures -------------------

func TestLint_Production(t *testing.T) {
	t.Parallel()
	vs, err := Lint("../..", []string{"./..."})
	if err != nil {
		t.Skipf("production lint unavailable in test env: %v", err)
	}
	if len(vs) != 0 {
		t.Logf("production violations:")
		for _, v := range vs {
			t.Logf("  %s", v)
		}
		t.Errorf("production scan must pass (current state expected clean); got %d", len(vs))
	}
}

// --- Path 8: realMain default + verbose + bad-flag ------------------

func TestRealMain_Default(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	code := realMain([]string{"-dir", "../..", "./..."}, &out)
	if code == 2 {
		t.Errorf("default scan must not error; got 2; out=%s", out.String())
	}
}

func TestRealMain_Verbose(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	code := realMain([]string{"-v", "-dir", "../..", "./..."}, &out)
	if code == 2 {
		t.Errorf("verbose must not error; got 2")
	}
	if !strings.Contains(out.String(), "scanned patterns") {
		t.Errorf("expected verbose summary; got %q", out.String())
	}
}

func TestRealMain_BadFlag(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	code := realMain([]string{"--no-such-flag"}, &out)
	if code != 2 {
		t.Errorf("bad flag must exit 2; got %d", code)
	}
}

func TestRealMain_BadPattern(t *testing.T) {
	t.Parallel()
	// Use a path that doesn't exist as a Go module pattern.
	var out bytes.Buffer
	code := realMain([]string{"/no/such/pkg/exists"}, &out)
	// packages.Load is forgiving; usually returns empty package list w/o
	// hard error. Accept both 0 (no pkgs scanned) and 2 (load failure).
	if code == 1 {
		t.Errorf("non-existent pattern should not flag fake violations; got 1: %s", out.String())
	}
}

// --- Path 9: end-to-end Lint on testdata fixtures (synthetic module) ---

func TestLint_BadFixture(t *testing.T) {
	t.Parallel()
	vs, err := Lint("testdata/fixtures", []string{"./badpkg"})
	if err != nil {
		t.Skipf("fixture load unavailable: %v", err)
	}
	// T-13 codex HIGH-1: now also catches BadLocalBareErrors + BadLocalFmtErrorf.
	if len(vs) != 5 {
		t.Errorf("expected 5 violations; got %d", len(vs))
		for _, v := range vs {
			t.Logf("  %s", v)
		}
	}
	wantByFn := map[string]Code{
		"BadBareErrors":      EC1,
		"BadFmtErrorf":       EC2,
		"BadFmtWrap":         EC2,
		"BadLocalBareErrors": EC1,
		"BadLocalFmtErrorf":  EC2,
	}
	for _, v := range vs {
		want, ok := wantByFn[v.Function]
		if !ok {
			t.Errorf("unexpected violation function %s", v.Function)
			continue
		}
		if v.Code != want {
			t.Errorf("function %s: got code %s, want %s", v.Function, v.Code, want)
		}
		delete(wantByFn, v.Function)
	}
	for fn := range wantByFn {
		t.Errorf("missing expected violation for %s", fn)
	}
}

func TestLint_GoodFixture(t *testing.T) {
	t.Parallel()
	vs, err := Lint("testdata/fixtures", []string{"./goodpkg"})
	if err != nil {
		t.Skipf("fixture load unavailable: %v", err)
	}
	if len(vs) != 0 {
		t.Errorf("good fixture must pass; got %d violations", len(vs))
		for _, v := range vs {
			t.Logf("  %s", v)
		}
	}
}

// --- Path 9c: realMain end-to-end with bad fixture ------------------

func TestRealMain_BadFixtureViolations(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	code := realMain([]string{"-dir", "testdata/fixtures", "./badpkg"}, &out)
	if code != 1 {
		t.Errorf("bad fixture must exit 1; got %d; out=%s", code, out.String())
	}
	if !strings.Contains(out.String(), "FAIL") || !strings.Contains(out.String(), "violation") {
		t.Errorf("expected FAIL output; got %q", out.String())
	}
}

// --- Path 9d: isErrorType interface branch coverage -----------------

func TestIsErrorType_CustomInterface(t *testing.T) {
	t.Parallel()
	// Construct an interface type that declares Error() string. This
	// exercises the iface.NumMethods branch of isErrorType.
	src := `package x
type MyErr interface { Error() string }
`
	fset := token.NewFileSet()
	f, _ := parser.ParseFile(fset, "x.go", src, 0)
	// Use go/types to typecheck.
	conf := types.Config{Importer: nil}
	info := &types.Info{Defs: map[*ast.Ident]types.Object{}}
	pkg, err := conf.Check("x", fset, []*ast.File{f}, info)
	if err != nil {
		t.Skipf("typecheck unavailable: %v", err)
	}
	obj := pkg.Scope().Lookup("MyErr")
	if obj == nil {
		t.Skip("MyErr not found in scope")
	}
	if !isErrorType(obj.Type()) {
		t.Errorf("custom interface with Error() string must be detected as error type")
	}
}

// --- Path 10: funcReturnsError text-based fallback ------------------

// TestAuditManifest_Consistency cross-checks tools/errcode-lint/testdata/
// audit-errcode-sites.json against the live errcode-lint behavior on the
// opendbx module (T-13 codex MED-1).
//
// The manifest declares expected_violations_today; this test asserts the
// production scan reports exactly that count, providing a machine gate
// against silent regression of the spec-0.6 D-4 contract.
func TestAuditManifest_Consistency(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("testdata/audit-errcode-sites.json")
	if err != nil {
		t.Skipf("manifest unavailable: %v", err)
	}
	var manifest struct {
		InvariantCheck struct {
			ExpectedViolations int `json:"expected_violations_today"`
		} `json:"invariant_check"`
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatalf("parse manifest: %v", err)
	}
	vs, err := Lint("../..", []string{"./..."})
	if err != nil {
		t.Skipf("production lint unavailable: %v", err)
	}
	if len(vs) != manifest.InvariantCheck.ExpectedViolations {
		t.Errorf("audit manifest expects %d violations; got %d",
			manifest.InvariantCheck.ExpectedViolations, len(vs))
		for _, v := range vs {
			t.Logf("  %s", v)
		}
	}
}

func TestFuncReturnsError_TextFallback(t *testing.T) {
	t.Parallel()
	src := `package x

func Foo() error { return nil }
func Bar() (int, error) { return 0, nil }
func Baz() {}
`
	fset := token.NewFileSet()
	f, _ := parser.ParseFile(fset, "x.go", src, 0)
	got := map[string]bool{}
	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		got[fn.Name.Name] = funcReturnsError(nil, fn)
	}
	if !got["Foo"] || !got["Bar"] || got["Baz"] {
		t.Errorf("returnsError fallback wrong; got %v", got)
	}
}
