// Package goodpkg is a test fixture for errcode-lint OK paths.
//
// We fake an errcode package by using a local alias; in production
// opendbx use internal/platform/errcode. errcode-lint matches by
// selector name `errcode.New` / `errcode.Newf` / `errcode.Wrap` so
// any package aliased `errcode` works for OK detection.
package goodpkg

import (
	"errors"

	errcode "example.com/errcode-lint-fixtures/goodpkg/errcode"
)

// GoodErrcodeNew uses errcode.New → OK.
func GoodErrcodeNew() error {
	return errcode.New("good: errcode.New")
}

// GoodErrcodeWrap uses errcode.Wrap → OK.
func GoodErrcodeWrap(root error) error {
	return errcode.Wrap(root, "good: errcode.Wrap")
}

// GoodParamReturn returns a function parameter (caller already wrapped) → OK.
func GoodParamReturn(err error) error {
	return err
}

// _unused references errors so the import remains used; not exported.
//
//nolint:unused // spec-0.10 D-2: fixture-only helper to keep errors import live
func _unused() error {
	return errors.New("unused")
}
