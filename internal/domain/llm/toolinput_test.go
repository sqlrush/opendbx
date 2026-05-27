// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package llm

import (
	"errors"
	"strings"
	"testing"
)

// TestDecodeToolInput moved from anthropic adapter (spec-1.20.1 D-7
// errata-move; behavior unchanged) — now the shared guard for all adapters.
func TestDecodeToolInput(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		raw     string
		wantErr bool
	}{
		{"empty → empty object", "", false},
		{"object", `{"n": 5, "q": "select"}`, false},
		{"nested object", `{"a": {"b": {"c": 1}}}`, false},
		{"non-object array", `[1,2,3]`, true},
		{"non-object scalar", `42`, true},
		{"malformed", `{"a":`, true},
		{"trailing object", `{}{}`, true},
		{"trailing garbage", `{"a":1} x`, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := DecodeToolInput([]byte(tc.raw))
			if tc.wantErr != (err != nil) {
				t.Errorf("DecodeToolInput(%q) err=%v; wantErr=%v", tc.raw, err, tc.wantErr)
			}
			if err != nil && !errors.Is(err, ErrDecodeFailed) {
				t.Errorf("err %v should be ErrDecodeFailed", err)
			}
		})
	}
}

func TestDecodeToolInput_Oversize(t *testing.T) {
	t.Parallel()
	big := `{"x":"` + strings.Repeat("a", MaxToolInputBytes) + `"}`
	if _, err := DecodeToolInput([]byte(big)); !errors.Is(err, ErrDecodeFailed) {
		t.Errorf("oversize input should be ErrDecodeFailed; got %v", err)
	}
}

func TestDecodeToolInput_TooDeep(t *testing.T) {
	t.Parallel()
	deep := strings.Repeat(`{"a":`, MaxToolInputDepth+2) + "1" + strings.Repeat("}", MaxToolInputDepth+2)
	if _, err := DecodeToolInput([]byte(deep)); !errors.Is(err, ErrDecodeFailed) {
		t.Errorf("too-deep input should be ErrDecodeFailed; got %v", err)
	}
}
