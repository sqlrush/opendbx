// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package logger

import (
	"fmt"
	"os"
	"os/signal"
	"runtime/debug"
	"sync"
	"syscall"
)

// RegisterSignalCleanup arms SIGINT / SIGTERM handlers that flush the global
// logger before letting the default Go runtime exit behaviour take over.
//
// Contract (spec § 3 dispose path + claude HIGH-4):
//   - Idempotent (sync.Once): subsequent calls are no-ops.
//   - Handler runs Close() then re-raises the signal so the runtime exits
//     with the expected non-zero status (e.g. 130 for SIGINT).
//   - Best-effort: if Close errors, we still re-raise — log loss is preferred
//     over swallowing the signal.
//
// Callers (typically cmd/opendbx/main after logger.Init) invoke this once.
// We deliberately do NOT auto-register from Init because that would override
// caller-controlled signal disposition.
func RegisterSignalCleanup() {
	signalCleanupOnce.Do(func() {
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
		go func() {
			sig := <-ch
			_ = Close()
			// Re-raise to default behaviour. signal.Reset puts the disposition
			// back to default, then we re-send the signal to ourselves.
			signal.Reset(sig)
			if s, ok := sig.(syscall.Signal); ok {
				_ = syscall.Kill(syscall.Getpid(), s)
			}
		}()
	})
}

var signalCleanupOnce sync.Once

// GuardPanic invokes fn with panic recovery that:
//
//  1. emits a `process.panic` sidecar event including the panic value and
//     a runtime stack trace (Q13 ★D = A+B integration)
//  2. flushes both writers via Close (errors.Join — claude HIGH-4)
//  3. re-panics so callers see the original semantics
//
// Typical use:
//
//	func main() {
//	    logger.Init(...)
//	    defer logger.Close()
//	    logger.RegisterSignalCleanup()
//	    logger.GuardPanic(func() {
//	        cmd.Execute()
//	    })
//	}
func GuardPanic(fn func()) {
	defer func() {
		r := recover()
		if r == nil {
			return
		}
		stack := string(debug.Stack())
		L().Error(
			"process.panic",
			Attr{Key: "event", Value: "process.panic"},
			Attr{Key: "value", Value: fmt.Sprint(r)},
			Attr{Key: "stack", Value: stack},
		)
		_ = Close()
		panic(r)
	}()
	fn()
}
