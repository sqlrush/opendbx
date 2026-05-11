// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package errcode

import (
	"errors"
	"fmt"
)

// Error is the opendbx error contract (spec § 2.1). Every public API
// returning error MUST return an instance of this interface (or wrap one
// via fmt.Errorf("%w") / errcode.Wrap).
type Error interface {
	error
	Code() string
	Message() string
	Hint() string
	Unwrap() error
}

// structuredErr is the concrete implementation of Error. The Sentinel
// values returned from Register are typed as Sentinel interface but backed
// by *structuredErr at runtime — this is what makes errors.Is symmetry
// work in both directions (sentinel ↔ structuredErr; spec § 1.3 + § 2.2
// pseudocode).
type structuredErr struct {
	code    string
	message string
	hint    string
	wrapped error
}

// Error renders the structured error as "[CODE] message". Useful for logs
// and stderr; the JSONL sidecar reads Code/Message/Hint separately.
func (e *structuredErr) Error() string {
	if e == nil {
		return ""
	}
	if e.wrapped != nil {
		return "[" + e.code + "] " + e.message + ": " + e.wrapped.Error()
	}
	return "[" + e.code + "] " + e.message
}

// Code returns the machine-readable identifier (e.g. "LOGGER.WRITER_CLOSED").
func (e *structuredErr) Code() string {
	if e == nil {
		return ""
	}
	return e.code
}

// Message returns the human-readable single sentence.
func (e *structuredErr) Message() string {
	if e == nil {
		return ""
	}
	return e.message
}

// Hint returns the remediation suggestion.
func (e *structuredErr) Hint() string {
	if e == nil {
		return ""
	}
	return e.hint
}

// Unwrap returns the wrapped root error (or nil). Required by errors.As
// chain traversal and by sidecar marshalling that walks the chain via
// errors.As to find the deepest errcode.Error.
func (e *structuredErr) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.wrapped
}

// Is implements the errors.Is hook (spec § 1.3 errors.Is semantics).
// Matches when target is any Error with the same Code. This makes
// `errors.Is(err, ErrSentinel)` work for both:
//
//   - err = structuredErr from New/Newf/Wrap (constructor path)
//   - err = Sentinel from Register (declaration path)
//
// symmetrically — both sides only need Code() to participate.
func (e *structuredErr) Is(target error) bool {
	if e == nil || target == nil {
		return false
	}
	if t, ok := target.(Error); ok {
		return e.code == t.Code()
	}
	return false
}

// New constructs an Error from a registered code.
//
// Panics if code is not registered — callers must Register at file scope
// before constructing errors against that code. The msg and hint here
// override registry defaults; pass "" to inherit the registered values.
func New(code, msg, hint string) Error {
	def, ok := Lookup(code)
	if !ok {
		panic("errcode: New called with unregistered code: " + code)
	}
	if msg == "" {
		msg = def.Message
	}
	if hint == "" {
		hint = def.Hint
	}
	return &structuredErr{code: code, message: msg, hint: hint}
}

// Newf is sprintf-style for the Message. Hint inherits registry default.
// Panics if code is unregistered (same contract as New).
func Newf(code, format string, args ...any) Error {
	def, ok := Lookup(code)
	if !ok {
		panic("errcode: Newf called with unregistered code: " + code)
	}
	return &structuredErr{
		code:    code,
		message: fmt.Sprintf(format, args...),
		hint:    def.Hint,
	}
}

// Wrap attaches a structured layer atop an existing root error.
//
// errors.Is(wrap, root) → true via stdlib chain traversal of Unwrap.
// errors.Is(wrap, sentinel-with-same-code) → true via structuredErr.Is.
//
// Panics if code is unregistered. Pass "" msg/hint to inherit registry
// defaults. If err is nil, behaves identically to New.
func Wrap(code string, err error, msg, hint string) Error {
	def, ok := Lookup(code)
	if !ok {
		panic("errcode: Wrap called with unregistered code: " + code)
	}
	if msg == "" {
		msg = def.Message
	}
	if hint == "" {
		hint = def.Hint
	}
	return &structuredErr{code: code, message: msg, hint: hint, wrapped: err}
}

// As is a thin convenience wrapper around stdlib errors.As. Callers may
// use either form (spec § 2.5).
//
//	var ec errcode.Error
//	if errcode.As(err, &ec) {
//	    fmt.Println(ec.Code(), ec.Hint())
//	}
func As(err error, target *Error) bool {
	return errors.As(err, target)
}

// Is is a thin convenience wrapper around stdlib errors.Is. Callers may
// use either form.
func Is(err, target error) bool {
	return errors.Is(err, target)
}
