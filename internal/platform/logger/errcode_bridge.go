// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package logger

import (
	"errors"

	"github.com/sqlrush/opendbx/internal/platform/errcode"
)

// errcodeFromErr extracts the structured Code/Message/Hint triple from err
// by walking the Unwrap chain via errors.As (spec-0.6 D-5; codex HIGH-1 +
// claude CRIT-1 contract).
//
// Resolves the 4 forms the spec § 2.4 mandates:
//
//   - Plain errcode.Error                       → direct match
//   - fmt.Errorf("...: %w", errcode.Error)      → As walks Unwrap
//   - errcode.Wrap(code, errcode.Error, ...)    → As binds to outermost
//   - redactedError{wrapped: errcode.Error}     → As walks via Unwrap()
//     (spec-0.5 redactedError gained Unwrap in spec-0.6 D-5)
//
// Non-errcode errors fall back to `code:""` + raw message + empty hint
// (spec § 2.4 Q9 ★A fallback compatible with spec-0.5 current behaviour).
//
// nil err returns three empty strings.
func errcodeFromErr(err error) (code, message, hint string) {
	if err == nil {
		return "", "", ""
	}
	var ec errcode.Error
	if errors.As(err, &ec) {
		return ec.Code(), ec.Message(), ec.Hint()
	}
	return "", err.Error(), ""
}
