// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package logger

import "context"

// noopLogger is returned by L() before Init has been called. All methods are
// silent no-ops; this matches CC's pre-bootstrap behaviour where logger
// references are valid but emit nothing.
//
// Rationale: code paths that reference logger.L() during early init (e.g.
// flag parsing, version printing) should not need to check whether the
// logger has been initialised. The noop fallback makes L() safe to call
// from anywhere.
type noopLogger struct{}

func (noopLogger) Verbose(string, ...Attr)            {}
func (noopLogger) Debug(string, ...Attr)              {}
func (noopLogger) Info(string, ...Attr)               {}
func (noopLogger) Warn(string, ...Attr)               {}
func (noopLogger) Error(string, ...Attr)              {}
func (noopLogger) WithModule(string) Logger           { return noopLogger{} }
func (noopLogger) WithAttrs(...Attr) Logger           { return noopLogger{} }
func (noopLogger) WithContext(context.Context) Logger { return noopLogger{} }
