// Package goodpkg is a test fixture for errcode-lint OK paths.
//
// We fake an errcode package under internal/platform/errcode so
// errcode-lint can resolve the same package suffix used by production.
package goodpkg

import (
	"errors"

	ec "example.com/errcode-lint-fixtures/internal/platform/errcode"
)

// GoodErrcodeNew uses errcode.New → OK.
func GoodErrcodeNew() error {
	return ec.New("good: errcode.New")
}

// GoodErrcodeWrap uses errcode.Wrap → OK.
func GoodErrcodeWrap(root error) error {
	return ec.Wrap(root, "good: errcode.Wrap")
}

// GoodHelperCall returns an error from a local helper that itself returns
// errcode; helper-proof analysis must accept it.
func GoodHelperCall() error {
	return helperWrapped()
}

// GoodLocalHelperCall stores a wrapped helper result in a local before
// returning it; reaching-assignment analysis must accept it.
func GoodLocalHelperCall() error {
	err := helperWrapped()
	return err
}

// GoodSelectorHelper calls a method helper; type-aware calledFunc must
// resolve SelectorExpr.Sel and prove the method returns errcode.
func GoodSelectorHelper() error {
	return helperObj{}.Wrapped()
}

// GoodParamReturn returns a function parameter (caller already wrapped) → OK.
func GoodParamReturn(err error) error {
	return err
}

type helperObj struct{}

func (helperObj) Wrapped() error {
	return ec.New("good: method helper")
}

type hidden struct{}

// ExportedMethod returns a bare error but its receiver type is unexported,
// so the method is not a public package boundary.
func (hidden) ExportedMethod() error {
	return errors.New("hidden receiver method")
}

func helperWrapped() error {
	return ec.Newf("good: helper errcode.Newf")
}

// _unused references errors so the import remains used; not exported.
//
//nolint:unused // spec-0.10 D-2: fixture-only helper to keep errors import live
func _unused() error {
	return errors.New("unused")
}
