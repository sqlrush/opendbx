// Package badpkg is a test fixture for errcode-lint EC-1 / EC-2.
package badpkg

import (
	stderrors "errors"
	stdfmt "fmt"
)

// BadBareErrors is exported, returns error, uses errors.New → EC-1.
func BadBareErrors() error {
	return stderrors.New("bad: bare errors.New")
}

// BadFmtErrorf is exported, returns error, uses fmt.Errorf → EC-2.
func BadFmtErrorf() error {
	return stdfmt.Errorf("bad: bare fmt.Errorf")
}

// BadFmtWrap wraps with %w but outer is fmt.Errorf, not errcode → EC-2.
func BadFmtWrap(root error) error {
	return stdfmt.Errorf("bad: %w", root)
}

// BadLocalBareErrors stores errors.New in a local then returns it. T-13
// codex HIGH-1: errcode-lint must trace reaching assignment, not just
// direct call returns.
func BadLocalBareErrors() error {
	err := stderrors.New("bad: local-var bare errors.New")
	return err
}

// BadLocalFmtErrorf — same pattern with fmt.Errorf → EC-2.
func BadLocalFmtErrorf() error {
	err := stdfmt.Errorf("bad: local-var fmt.Errorf")
	return err
}

// BadAliasErrors imports errors under an alias; type-aware detection must
// still catch it as EC-1.
func BadAliasErrors() error {
	return stderrors.New("bad: alias errors.New")
}

// BadAliasFmt imports fmt under an alias; type-aware detection must still
// catch it as EC-2.
func BadAliasFmt() error {
	return stdfmt.Errorf("bad: alias fmt.Errorf")
}

// BadUnknownHelper returns an error from a helper call that errcode-lint
// cannot prove is wrapped, so the public boundary must fail with EC-3.
func BadUnknownHelper() error {
	return helperUnknown()
}

// BadVarDecl returns a local error assigned via a var declaration rather
// than :=. The conservative model cannot prove it, so EC-3 is expected.
func BadVarDecl() error {
	var err = stderrors.New("bad: var declaration")
	return err
}

func helperUnknown() error {
	return stderrors.New("bad: hidden helper")
}

// privateBareErrors is unexported → must be skipped (returns error).
func privateBareErrors() error {
	return stderrors.New("private: ok to skip")
}

// NoErrorReturn does not return error → must be skipped.
func NoErrorReturn() string {
	return "no error return"
}

// ExemptComment carries the exempt directive on the line above the
// return; errcode-lint must skip.
func ExemptComment() error {
	// errcode-lint:exempt -- spec-0.10 D-2: fixture skip line
	return stderrors.New("exempt: should skip")
}
