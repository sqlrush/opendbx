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
//   - return an unproved error expression/helper/local             → EC-3
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

const (
	errcodePackagePath = "github.com/sqlrush/opendbx/internal/platform/errcode"
	errorsPackagePath  = "errors"
	fmtPackagePath     = "fmt"
)

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
	EC3 Code = "EC-3" // exported func returns unproved error expression
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
	errorType := types.Universe.Lookup("error").Type()
	if types.AssignableTo(typ, errorType) {
		return true
	}
	errorIface, ok := errorType.Underlying().(*types.Interface)
	return ok && types.Implements(typ, errorIface)
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

// selectorFunc reports the selected function's import path and name.
// When type information is available, it uses the resolved object, so
// aliases such as `e "errors"` still resolve to package path "errors".
// Without type information it falls back to the textual selector prefix.
func selectorFunc(ce *ast.CallExpr, info *types.Info) (pkgPath string, name string, ok bool) {
	sel, ok := ce.Fun.(*ast.SelectorExpr)
	if !ok {
		return "", callExprName(ce), false
	}
	if info != nil {
		if fn, ok := info.Uses[sel.Sel].(*types.Func); ok {
			if pkg := fn.Pkg(); pkg != nil {
				return pkg.Path(), fn.Name(), true
			}
		}
	}
	if ident, ok := sel.X.(*ast.Ident); ok {
		return ident.Name, sel.Sel.Name, false
	}
	return "", sel.Sel.Name, false
}

func calledFunc(ce *ast.CallExpr, info *types.Info) *types.Func {
	if info == nil {
		return nil
	}
	switch fn := ce.Fun.(type) {
	case *ast.Ident:
		if obj, ok := info.Uses[fn].(*types.Func); ok {
			return obj
		}
	case *ast.SelectorExpr:
		if obj, ok := info.Uses[fn.Sel].(*types.Func); ok {
			return obj
		}
	}
	return nil
}

// isErrcodeConstructor returns true if call is errcode.New / errcode.Newf
// / errcode.Wrap from the canonical errcode package. Fixture modules may
// use the same internal/platform/errcode suffix.
func isErrcodeConstructor(ce *ast.CallExpr, info *types.Info) bool {
	pkgPath, name, resolved := selectorFunc(ce, info)
	if name != "New" && name != "Newf" && name != "Wrap" {
		return false
	}
	if resolved {
		return pkgPath == errcodePackagePath || strings.HasSuffix(pkgPath, "/internal/platform/errcode")
	}
	return pkgPath == "errcode"
}

// isBareErrorConstructor returns true if call is errors.New or fmt.Errorf
// (without errcode being the outer wrapper). For fmt.Errorf we treat all
// uses inside an exported error-returning function as suspect unless
// wrapped by errcode at the caller.
func isBareErrorConstructor(ce *ast.CallExpr, info *types.Info) (Code, bool) {
	pkgPath, name, resolved := selectorFunc(ce, info)
	if resolved && pkgPath == errorsPackagePath && name == "New" {
		return EC1, true
	}
	if resolved && pkgPath == fmtPackagePath && name == "Errorf" {
		return EC2, true // could be %w wrapping; flag as EC-2
	}
	if !resolved && pkgPath == "errors" && name == "New" {
		return EC1, true
	}
	if !resolved && pkgPath == "fmt" && name == "Errorf" {
		return EC2, true // could be %w wrapping; flag as EC-2
	}
	return "", false
}

func isErrcodeType(typ types.Type) bool {
	if typ == nil {
		return false
	}
	hasMethod := func(name string) bool {
		obj, _, _ := types.LookupFieldOrMethod(typ, true, nil, name)
		fn, ok := obj.(*types.Func)
		if !ok {
			return false
		}
		sig, ok := fn.Type().(*types.Signature)
		if !ok || sig.Params().Len() != 0 {
			return false
		}
		switch name {
		case "Code", "Message", "Hint", "Error":
			return sig.Results().Len() == 1 && sig.Results().At(0).Type() == types.Typ[types.String]
		case "Unwrap":
			return sig.Results().Len() == 1 && isErrorType(sig.Results().At(0).Type())
		default:
			return false
		}
	}
	return hasMethod("Error") && hasMethod("Code") && hasMethod("Message") &&
		hasMethod("Hint") && hasMethod("Unwrap")
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

// reachingAssignSource returns the *ast.CallExpr that is the most recent
// reaching assignment to the identifier `name` within the function body
// before position `before`. Returns nil if not found or if the value was
// reassigned in non-call ways (e.g. control flow merge). T-13 codex
// HIGH-1: conservative reaching-assignment analysis to catch the common
// pattern `err := errors.New("x"); return err`.
//
// The analysis is intentionally simple: it walks the function body
// linearly and tracks the latest single assignment per identifier. It is
// safe (false-negatives possible if control flow is complex) but never
// reports an EC-3 violation when it cannot prove the origin; callers can
// use `errcode-lint:exempt` with a spec reference for intentional cases.
func reachingAssignSource(fn *ast.FuncDecl, name string, before token.Pos) *ast.CallExpr {
	if fn.Body == nil {
		return nil
	}
	var found *ast.CallExpr
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		if n == nil || n.Pos() >= before {
			return false
		}
		switch s := n.(type) {
		case *ast.AssignStmt:
			// Look for `name := <call>` or `name = <call>` patterns.
			for i, lhs := range s.Lhs {
				ident, ok := lhs.(*ast.Ident)
				if !ok || ident.Name != name {
					continue
				}
				if i >= len(s.Rhs) {
					continue
				}
				if ce, ok := s.Rhs[i].(*ast.CallExpr); ok {
					found = ce
				}
			}
		}
		return true
	})
	return found
}

func collectFunctionDecls(pkg *packages.Package) map[*types.Func]*ast.FuncDecl {
	out := make(map[*types.Func]*ast.FuncDecl)
	if pkg.TypesInfo == nil {
		return out
	}
	for _, file := range pkg.Syntax {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Name == nil {
				continue
			}
			if obj, ok := pkg.TypesInfo.Defs[fn.Name].(*types.Func); ok {
				out[obj] = fn
			}
		}
	}
	return out
}

func helperReturnsWrapped(pkg *packages.Package, fn *ast.FuncDecl,
	helpers map[*types.Func]*ast.FuncDecl, seen map[*types.Func]bool) bool {
	if fn == nil || fn.Body == nil {
		return false
	}
	paramNames := map[string]bool{}
	if fn.Type.Params != nil {
		for _, p := range fn.Type.Params.List {
			for _, n := range p.Names {
				paramNames[n.Name] = true
			}
		}
	}
	proved := true
	sawErrorReturn := false
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		if !proved {
			return false
		}
		ret, ok := n.(*ast.ReturnStmt)
		if !ok {
			return true
		}
		for _, expr := range ret.Results {
			if pkg.TypesInfo != nil {
				if t := pkg.TypesInfo.TypeOf(expr); t != nil && !isErrorType(t) {
					continue
				}
			}
			sawErrorReturn = true
			if !isProvedReturnExpr(pkg, fn, paramNames, expr, helpers, seen) {
				proved = false
				return false
			}
		}
		return true
	})
	return sawErrorReturn && proved
}

func callReturnsWrapped(pkg *packages.Package, ce *ast.CallExpr,
	helpers map[*types.Func]*ast.FuncDecl, seen map[*types.Func]bool) bool {
	obj := calledFunc(ce, pkg.TypesInfo)
	if obj == nil {
		return false
	}
	decl := helpers[obj]
	if decl == nil {
		return false
	}
	if seen[obj] {
		return false
	}
	seen[obj] = true
	ok := helperReturnsWrapped(pkg, decl, helpers, seen)
	delete(seen, obj)
	return ok
}

func isProvedReturnExpr(pkg *packages.Package, fn *ast.FuncDecl, paramNames map[string]bool,
	expr ast.Expr, helpers map[*types.Func]*ast.FuncDecl, seen map[*types.Func]bool) bool {
	if ident, ok := expr.(*ast.Ident); ok && ident.Name == "nil" {
		return true
	}
	if pkg.TypesInfo != nil && isErrcodeType(pkg.TypesInfo.TypeOf(expr)) {
		return true
	}
	if ce, ok := expr.(*ast.CallExpr); ok {
		if isErrcodeConstructor(ce, pkg.TypesInfo) {
			return true
		}
		if _, ok := isBareErrorConstructor(ce, pkg.TypesInfo); ok {
			return false
		}
		return callReturnsWrapped(pkg, ce, helpers, seen)
	}
	if ident, ok := expr.(*ast.Ident); ok {
		if paramNames[ident.Name] {
			return true
		}
		src := reachingAssignSource(fn, ident.Name, ident.Pos())
		if src == nil {
			return false
		}
		if isErrcodeConstructor(src, pkg.TypesInfo) {
			return true
		}
		if _, ok := isBareErrorConstructor(src, pkg.TypesInfo); ok {
			return false
		}
		return callReturnsWrapped(pkg, src, helpers, seen)
	}
	return false
}

// inspectFunction walks fn body looking for return statements whose
// error-position expressions violate the contract. T-13 codex HIGH-1:
// handle local-var return via reaching-assignment analysis.
func inspectFunction(pkg *packages.Package, file *ast.File, fn *ast.FuncDecl,
	helpers map[*types.Func]*ast.FuncDecl) []Violation {
	if fn.Body == nil {
		return nil
	}
	// Collect names of function parameters; returning a parameter is OK
	// (caller already constructed / wrapped the error).
	paramNames := map[string]bool{}
	if fn.Type.Params != nil {
		for _, p := range fn.Type.Params.List {
			for _, n := range p.Names {
				paramNames[n.Name] = true
			}
		}
	}

	var vs []Violation
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		ret, ok := n.(*ast.ReturnStmt)
		if !ok {
			return true
		}
		for _, expr := range ret.Results {
			// Determine if the expression's type is error.
			if pkg.TypesInfo != nil {
				if t := pkg.TypesInfo.TypeOf(expr); t != nil && !isErrorType(t) {
					continue
				}
			}
			vs = append(vs, classifyReturnExpr(pkg, file, fn, paramNames, expr, helpers)...)
		}
		return true
	})
	return vs
}

// classifyReturnExpr decides whether a single return expression in an
// exported error-returning function is OK or a violation. Handles four
// shapes:
//   - direct errcode constructor call → OK
//   - direct errors.New / fmt.Errorf call → EC-1 / EC-2
//   - bare identifier (local var or param) → check reaching assignment
//     or treat parameter as OK (caller-wrapped)
//   - anything else → EC-3 unless exempt
func classifyReturnExpr(pkg *packages.Package, file *ast.File, fn *ast.FuncDecl,
	paramNames map[string]bool, expr ast.Expr, helpers map[*types.Func]*ast.FuncDecl) []Violation {
	if ident, ok := expr.(*ast.Ident); ok && ident.Name == "nil" {
		return nil
	}
	if pkg.TypesInfo != nil && isErrcodeType(pkg.TypesInfo.TypeOf(expr)) {
		return nil
	}
	// Direct call expression case.
	if ce, ok := expr.(*ast.CallExpr); ok {
		if isErrcodeConstructor(ce, pkg.TypesInfo) {
			return nil
		}
		if code, ok := isBareErrorConstructor(ce, pkg.TypesInfo); ok {
			return emitIfNotExempt(pkg, file, fn, ce.Pos(), code)
		}
		if callReturnsWrapped(pkg, ce, helpers, map[*types.Func]bool{}) {
			return nil
		}
		return emitIfNotExempt(pkg, file, fn, ce.Pos(), EC3)
	}
	// Bare identifier (local var or parameter) — trace its assignment.
	if ident, ok := expr.(*ast.Ident); ok {
		if paramNames[ident.Name] {
			return nil // returning a parameter — caller wrapped already
		}
		src := reachingAssignSource(fn, ident.Name, ident.Pos())
		if src == nil {
			return emitIfNotExempt(pkg, file, fn, ident.Pos(), EC3)
		}
		if isErrcodeConstructor(src, pkg.TypesInfo) {
			return nil
		}
		if code, ok := isBareErrorConstructor(src, pkg.TypesInfo); ok {
			return emitIfNotExempt(pkg, file, fn, src.Pos(), code)
		}
		if callReturnsWrapped(pkg, src, helpers, map[*types.Func]bool{}) {
			return nil
		}
		return emitIfNotExempt(pkg, file, fn, src.Pos(), EC3)
	}
	return emitIfNotExempt(pkg, file, fn, expr.Pos(), EC3)
}

// emitIfNotExempt emits a violation unless an `errcode-lint:exempt`
// comment is present near pos.
func emitIfNotExempt(pkg *packages.Package, file *ast.File, fn *ast.FuncDecl,
	pos token.Pos, code Code) []Violation {
	p := pkg.Fset.Position(pos)
	if hasExemptComment(file, pkg.Fset, p.Line) {
		return nil
	}
	msg := "exported function returns bare errors.New(...)"
	switch code {
	case EC2:
		msg = "exported function uses fmt.Errorf(...) for boundary error; use errcode.Wrap"
	case EC3:
		msg = "exported function returns unproved error expression; wrap with errcode or add an errcode-lint exemption"
	}
	return []Violation{{
		Pkg:      pkg.PkgPath,
		File:     p.Filename,
		Line:     p.Line,
		Function: fn.Name.Name,
		Code:     code,
		Message:  msg,
	}}
}

// inspectPackage walks every exported FuncDecl returning error.
func inspectPackage(pkg *packages.Package) []Violation {
	if isExempt(pkg.PkgPath) {
		return nil
	}
	helpers := collectFunctionDecls(pkg)
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
			if !ok || !isPublicFuncDecl(fn) {
				continue
			}
			if !funcReturnsError(pkg.TypesInfo, fn) {
				continue
			}
			vs = append(vs, inspectFunction(pkg, file, fn, helpers)...)
		}
	}
	return vs
}

func isPublicFuncDecl(fn *ast.FuncDecl) bool {
	if fn == nil || fn.Name == nil || !fn.Name.IsExported() {
		return false
	}
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return true
	}
	name := receiverBaseName(fn.Recv.List[0].Type)
	return name == "" || ast.IsExported(name)
}

func receiverBaseName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return receiverBaseName(t.X)
	case *ast.SelectorExpr:
		return t.Sel.Name
	case *ast.IndexExpr:
		return receiverBaseName(t.X)
	case *ast.IndexListExpr:
		return receiverBaseName(t.X)
	default:
		return ""
	}
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
