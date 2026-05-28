// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package diagnose

import (
	"errors"
	"testing"

	"github.com/sqlrush/opendbx/internal/platform/errcode"
)

// TestErrcodes_Registered guards spec-1.21 D-5: each DIAGNOSE.* sentinel
// MUST be a registered errcode (Code/Message/Hint triple), not a raw
// errors.New string — 规则 7 三件套.
func TestErrcodes_Registered(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		err  error
		code string
	}{
		{"MaxTurns", ErrMaxTurns, "DIAGNOSE.MAX_TURNS"},
		{"TotalTimeout", ErrTotalTimeout, "DIAGNOSE.TOTAL_TIMEOUT"},
		{"ToolUnknown", ErrToolUnknown, "DIAGNOSE.TOOL_UNKNOWN"},
		{"ToolTimeout", ErrToolTimeout, "DIAGNOSE.TOOL_TIMEOUT"},
		{"UnexpectedPause", ErrUnexpectedPause, "DIAGNOSE.UNEXPECTED_PAUSE"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var ec errcode.Error
			if !errors.As(tc.err, &ec) {
				t.Fatalf("%s: not an errcode.Error (规则 7)", tc.name)
			}
			if ec.Code() != tc.code {
				t.Errorf("Code = %q; want %q", ec.Code(), tc.code)
			}
			if ec.Message() == "" || ec.Hint() == "" {
				t.Errorf("%s missing Message or Hint (规则 7 三件套): msg=%q hint=%q",
					tc.name, ec.Message(), ec.Hint())
			}
		})
	}
}

// TestErrcodes_UniqueCodes guards against accidental code collision when
// new DIAGNOSE.* entries are added.
func TestErrcodes_UniqueCodes(t *testing.T) {
	t.Parallel()
	seen := map[string]bool{}
	for _, e := range []error{ErrMaxTurns, ErrTotalTimeout, ErrToolUnknown, ErrToolTimeout, ErrUnexpectedPause} {
		var ec errcode.Error
		if !errors.As(e, &ec) {
			continue
		}
		if seen[ec.Code()] {
			t.Errorf("duplicate code %q", ec.Code())
		}
		seen[ec.Code()] = true
	}
	if len(seen) != 5 {
		t.Errorf("expected 5 distinct DIAGNOSE.* codes; got %d", len(seen))
	}
}
