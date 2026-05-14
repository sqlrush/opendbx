// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// Package main implements errcode-lint, a custom checker enforcing the
// spec-0.6 D-4 contract: public API functions returning error MUST go
// through `internal/platform/errcode.Newf` / `errcode.Wrap`, not bare
// `errors.New` / `fmt.Errorf`. spec-0.10 D-2 / T-4.
//
// Why this exists (spec-0.6 D-4 forward):
//
// spec-0.6 introduced the Code/Message/Hint Error contract for opendbx.
// 18 boundary fmt.Errorf sites were migrated to errcode.Wrap in T-8.
// Without permanent lint enforcement, new boundary errors silently regress
// to bare fmt.Errorf. errcode-lint enforces the contract on every PR.
//
// Boundary judgment (spec-0.10 § 2.1 R2 deterministic rules):
//
//  1. *ast.FuncDecl (including methods) with exported name
//  2. ResultList contains a position whose type is assignable to `error`
//  3. File path not in skip list (_test.go / vendor/** / testdata/**)
//  4. Package path not in exempt list (internal/platform/errcode/** /
//     internal/entrypoints/** / tools/errcode-lint/** /
//     tools/coverage-gate/** etc — packages whose primary purpose is
//     building errors, parsing config CLI, or being lint tools themselves)
//
// Return expression conservative proof model (spec-0.10 § 2.1 R2):
//
//   - direct call errcode.New/Newf/Wrap                           → OK
//   - direct return of function parameter (caller already wrapped) → OK
//   - return local var whose reaching assignment is errcode.X     → OK
//   - return errors.New(...) / fmt.Errorf(...) bare construction  → EC-1
//   - return fmt.Errorf("...: %w", root) without errcode outer     → EC-2
//   - line above carries `// errcode-lint:exempt -- spec-X.Y D-N:` → skip
//
// Usage:
//
//	go run ./tools/errcode-lint [-v] [path...]
//
// Default path is `./...` (module-wide). Exit codes: 0 OK / 1 violations
// / 2 parse error.
package main

import (
	"flag"
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"io"
	"os"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"
)

// errcodePkg is the canonical opendbx error package path. Calls to
// errcode.New/Newf/Wrap from this path are considered OK constructions.
const errcodePkg = "github.com/sqlrush/opendbx/internal/platform/errcode"

// exemptPrefixes lists package import path prefixes whose subtree is
// exempt from public API errcode enforcement. These packages either:
//   - implement the errcode contract itself (internal/platform/errcode);
//   - are stub relays not yet implemented (internal/entrypoints);
//   - are lint tools themselves (tools/*).
//
// Subpackages of these prefixes are also exempt (e.g. tools/import-rules-
// check/rules inherits from tools/import-rules-check).
//
//nolint:gochecknoglobals // spec-0.10 D-2: deterministic exempt config table
var exemptPrefixes = []string{
	"github.com/sqlrush/opendbx/internal/platform/errcode",
	"github.com/sqlrush/opendbx/internal/entrypoints",
	"github.com/sqlrush/opendbx/tools/",
	"github.com/sqlrush/opendbx/cmd/opendbx",
	"github.com/sqlrush/opendbx/cmd/tools/",
}

// isExempt reports whether pkgPath is in the exempt subtree.
func isExempt(pkgPath string) bool {
	for _, p := range exemptPrefixes {
		if pkgPath == strings.TrimSuffix(p, "/") || strings.HasPrefix(pkgPath, p) {
			return true
		}
	}
	return false
}

// Code enumerates errcode-lint violation classes.
type Code string

// Violation codes.
const (
	EC1 Code = "EC-1" // exported func returns bare errors.New / fmt.Errorf
	EC2 Code = "EC-2" // exported func wraps with fmt.Errorf but outer is not errcode
)

// Violation describes a single rule transgression.
type Violation struct {
	Pkg      string
	File     string
	Line     int
	Function string
	Code     Code
	Message  string
}

// String renders a violation for stderr output.
func (v Violation) String() string {
	return fmt.Sprintf("  [%s] %s:%d %s.%s — %s",
		v.Code, v.File, v.Line, v.Pkg, v.Function, v.Message)
}

// isErrorType reports whether typ is the predeclared error type (or
// assignable to it, e.g. an interface that embeds error).
func isErrorType(typ types.Type) bool {
	if typ == nil {
		return false
	}
	// The predeclared error type is *types.Named "error" in the universe scope.
	named, ok := typ.(*types.Named)
	if ok && named.Obj() != nil && named.Obj().Name() == "error" && named.Obj().Pkg() == nil {
		return true
	}
	// Or any type that implements error.
	if iface, ok := typ.Underlying().(*types.Interface); ok {
		// Check whether the interface declares Error() string.
		for i := 0; i < iface.NumMethods(); i++ {
			m := iface.Method(i)
			if m.Name() == "Error" {
				sig, ok := m.Type().(*types.Signature)
				if ok && sig.Params().Len() == 0 && sig.Results().Len() == 1 {
					return true
				}
			}
		}
	}
	return false
}

// funcReturnsError reports whether the FuncDecl's signature has any
// result whose type is assignable to error.
func funcReturnsError(info *types.Info, fn *ast.FuncDecl) bool {
	if fn.Type.Results == nil {
		return false
	}
	for _, field := range fn.Type.Results.List {
		if info == nil {
			// Fallback: textual check
			if ident, ok := field.Type.(*ast.Ident); ok && ident.Name == "error" {
				return true
			}
			continue
		}
		t := info.TypeOf(field.Type)
		if isErrorType(t) {
			return true
		}
	}
	return false
}

// callExprName returns "pkg.Func" or "Func" for a CallExpr's call target,
// or "" if it's not a recognizable call.
func callExprName(ce *ast.CallExpr) string {
	switch fn := ce.Fun.(type) {
	case *ast.Ident:
		return fn.Name
	case *ast.SelectorExpr:
		if pkgIdent, ok := fn.X.(*ast.Ident); ok {
			return pkgIdent.Name + "." + fn.Sel.Name
		}
		return fn.Sel.Name
	}
	return ""
}

// isErrcodeConstructor returns true if call is errcode.New / errcode.Newf
// / errcode.Wrap. We use textual match on the selector (errcode pkg may
// be aliased but convention in opendbx is unaliased import).
func isErrcodeConstructor(ce *ast.CallExpr) bool {
	name := callExprName(ce)
	return name == "errcode.New" || name == "errcode.Newf" || name == "errcode.Wrap"
}

// isBareErrorConstructor returns true if call is errors.New or fmt.Errorf
// (without errcode being the outer wrapper). For fmt.Errorf we treat all
// uses inside an exported error-returning function as suspect unless
// wrapped by errcode at the caller.
func isBareErrorConstructor(ce *ast.CallExpr) (Code, bool) {
	name := callExprName(ce)
	if name == "errors.New" {
		return EC1, true
	}
	if name == "fmt.Errorf" {
		return EC2, true // could be %w wrapping; flag as EC-2
	}
	return "", false
}

// hasExemptComment scans comment groups above lineN for an
// `errcode-lint:exempt` directive.
func hasExemptComment(file *ast.File, fset *token.FileSet, lineN int) bool {
	for _, cg := range file.Comments {
		for _, c := range cg.List {
			if !strings.Contains(c.Text, "errcode-lint:exempt") {
				continue
			}
			pos := fset.Position(c.Pos())
			// Accept the directive when it appears on the same line, on
			// the previous line, or two lines above (covering `// ...\n
			// // ...\n return ...` patterns).
			if pos.Line == lineN || pos.Line == lineN-1 || pos.Line == lineN-2 {
				return true
			}
		}
	}
	return false
}

// inspectFunction walks fn body looking for return statements whose
// error-position expressions violate the contract.
func inspectFunction(pkg *packages.Package, file *ast.File, fn *ast.FuncDecl) []Violation {
	if fn.Body == nil {
		return nil
	}
	var vs []Violation
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		ret, ok := n.(*ast.ReturnStmt)
		if !ok {
			return true
		}
		// For each return expression at an error-typed position, classify.
		for _, expr := range ret.Results {
			// Determine if the expression's type is error.
			if pkg.TypesInfo != nil {
				if t := pkg.TypesInfo.TypeOf(expr); t != nil && !isErrorType(t) {
					continue
				}
			}
			ce, ok := expr.(*ast.CallExpr)
			if !ok {
				// Local var or function param. Conservative model: skip
				// (errcode reaching-assignment proof inferred via types
				// info — out of scope for v1, exempted via boundary
				// caller pattern). spec-0.10 § 2.1 R2: "return local
				// 变量，reaching assignment 是 errcode 构造器之一 → OK".
				continue
			}
			if isErrcodeConstructor(ce) {
				continue
			}
			code, ok := isBareErrorConstructor(ce)
			if !ok {
				// Unknown call (some library wrapper); skip — boundary
				// detection by exemption comment for those rare paths.
				continue
			}
			pos := pkg.Fset.Position(ce.Pos())
			if hasExemptComment(file, pkg.Fset, pos.Line) {
				continue
			}
			msg := "exported function returns bare errors.New(...)"
			if code == EC2 {
				msg = "exported function uses fmt.Errorf(...) for boundary error; use errcode.Wrap"
			}
			vs = append(vs, Violation{
				Pkg:      pkg.PkgPath,
				File:     pos.Filename,
				Line:     pos.Line,
				Function: fn.Name.Name,
				Code:     code,
				Message:  msg,
			})
		}
		return true
	})
	return vs
}

// inspectPackage walks every exported FuncDecl returning error.
func inspectPackage(pkg *packages.Package) []Violation {
	if isExempt(pkg.PkgPath) {
		return nil
	}
	var vs []Violation
	for _, file := range pkg.Syntax {
		// Skip test files at the AST level too (packages already filters).
		if pkg.CompiledGoFiles != nil {
			fn := pkg.Fset.Position(file.Pos()).Filename
			if strings.HasSuffix(fn, "_test.go") {
				continue
			}
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Name == nil || !fn.Name.IsExported() {
				continue
			}
			if !funcReturnsError(pkg.TypesInfo, fn) {
				continue
			}
			vs = append(vs, inspectFunction(pkg, file, fn)...)
		}
	}
	return vs
}

// Lint runs errcode-lint on the given package patterns and returns all
// violations. dir is the working directory for packages.Load (empty = cwd).
func Lint(dir string, patterns []string) ([]Violation, error) {
	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedSyntax |
			packages.NeedTypes | packages.NeedTypesInfo | packages.NeedDeps |
			packages.NeedImports | packages.NeedModule,
		Tests: false,
		Dir:   dir,
	}
	pkgs, err := packages.Load(cfg, patterns...)
	if err != nil {
		return nil, fmt.Errorf("load packages: %w", err)
	}
	var all []Violation
	for _, p := range pkgs {
		if len(p.Errors) > 0 {
			// Don't fail-fast on load errors; aggregate.
			for _, e := range p.Errors {
				_, _ = fmt.Fprintf(os.Stderr, "errcode-lint: load error in %s: %s\n", p.PkgPath, e)
			}
		}
		all = append(all, inspectPackage(p)...)
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].File != all[j].File {
			return all[i].File < all[j].File
		}
		return all[i].Line < all[j].Line
	})
	return all, nil
}

// realMain wraps flag parsing for test coverage.
func realMain(args []string, w io.Writer) int {
	fs := flag.NewFlagSet("errcode-lint", flag.ContinueOnError)
	fs.SetOutput(w)
	verbose := fs.Bool("v", false, "verbose: print scanned package count")
	dir := fs.String("dir", "", "working directory for package loading (default cwd)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	patterns := fs.Args()
	if len(patterns) == 0 {
		patterns = []string{"./..."}
	}
	vs, err := Lint(*dir, patterns)
	if err != nil {
		_, _ = fmt.Fprintf(w, "errcode-lint: %v\n", err)
		return 2
	}
	if *verbose {
		_, _ = fmt.Fprintf(w, "errcode-lint: scanned patterns %v; %d violation(s)\n",
			patterns, len(vs))
	}
	if len(vs) == 0 {
		_, _ = fmt.Fprintln(w, "errcode-lint OK: all exported error-returning functions use errcode")
		return 0
	}
	_, _ = fmt.Fprintf(w, "errcode-lint FAIL: %d violation(s)\n", len(vs))
	for _, v := range vs {
		_, _ = fmt.Fprintln(w, v)
	}
	_, _ = fmt.Fprintln(w, "hint: use errcode.Newf / errcode.Wrap (spec-0.6 D-4 contract)")
	_, _ = fmt.Fprintln(w, "      to suppress: // errcode-lint:exempt -- spec-X.Y D-N: <reason>")
	return 1
}

func main() {
	os.Exit(realMain(os.Args[1:], os.Stderr))
}
