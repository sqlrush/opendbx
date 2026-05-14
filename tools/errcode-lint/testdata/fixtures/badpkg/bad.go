// Package badpkg is a test fixture for errcode-lint EC-1 / EC-2.
package badpkg

import (
	"errors"
	"fmt"
)

// BadBareErrors is exported, returns error, uses errors.New → EC-1.
func BadBareErrors() error {
	return errors.New("bad: bare errors.New")
}

// BadFmtErrorf is exported, returns error, uses fmt.Errorf → EC-2.
func BadFmtErrorf() error {
	return fmt.Errorf("bad: bare fmt.Errorf")
}

// BadFmtWrap wraps with %w but outer is fmt.Errorf, not errcode → EC-2.
func BadFmtWrap(root error) error {
	return fmt.Errorf("bad: %w", root)
}

// privateBareErrors is unexported → must be skipped (returns error).
func privateBareErrors() error {
	return errors.New("private: ok to skip")
}

// NoErrorReturn does not return error → must be skipped.
func NoErrorReturn() string {
	return "no error return"
}

// ExemptComment carries the exempt directive on the line above the
// return; errcode-lint must skip.
func ExemptComment() error {
	// errcode-lint:exempt -- spec-0.10 D-2: fixture skip line
	return errors.New("exempt: should skip")
}
