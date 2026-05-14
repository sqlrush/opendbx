// Package errcode is a fake errcode for fixtures.
package errcode

// New constructs a sentinel error.
func New(msg string) error { return &E{msg: msg} }

// Wrap constructs a wrapping error.
func Wrap(_ error, msg string) error { return &E{msg: msg} }

// E is the fake error implementation.
type E struct{ msg string }

// Error implements error.
func (e *E) Error() string { return e.msg }
