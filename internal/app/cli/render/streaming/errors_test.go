// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package streaming

import (
	"errors"
	"strings"
	"testing"

	"github.com/sqlrush/opendbx/internal/platform/errcode"
)

// TestFinishReason_EnumValues verifies the 7-value enum (R2 D1: includes
// FinishCancelled for ctx.Done path).
func TestFinishReason_EnumValues(t *testing.T) {
	cases := []struct {
		name string
		fr   FinishReason
		want int
	}{
		{"Unset", FinishUnset, 0},
		{"Stop", FinishStop, 1},
		{"Length", FinishLength, 2},
		{"ToolUse", FinishToolUse, 3},
		{"ContentFilter", FinishContentFilter, 4},
		{"Error", FinishError, 5},
		{"Cancelled", FinishCancelled, 6},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if int(tc.fr) != tc.want {
				t.Fatalf("FinishReason %s = %d, want %d", tc.name, int(tc.fr), tc.want)
			}
		})
	}
}

// TestFinishReason_String — readable name for debug/log.
func TestFinishReason_String(t *testing.T) {
	cases := []struct {
		fr   FinishReason
		want string
	}{
		{FinishUnset, "unset"},
		{FinishStop, "stop"},
		{FinishLength, "length"},
		{FinishToolUse, "tool_use"},
		{FinishContentFilter, "content_filter"},
		{FinishError, "error"},
		{FinishCancelled, "cancelled"},
	}
	for _, tc := range cases {
		if got := tc.fr.String(); got != tc.want {
			t.Errorf("FinishReason(%d).String()=%q, want %q", int(tc.fr), got, tc.want)
		}
	}
}

// TestErrStreamTruncated_Triplet verifies errcode 三件套 (Code/Message/Hint
// all non-empty + actionable) per CLAUDE rule 7 + R2 D10.
func TestErrStreamTruncated_Triplet(t *testing.T) {
	if ErrStreamTruncated.Code() != "RENDER.STREAM_TRUNCATED" {
		t.Errorf("Code()=%q, want RENDER.STREAM_TRUNCATED", ErrStreamTruncated.Code())
	}
	if ErrStreamTruncated.Message() == "" {
		t.Error("Message empty")
	}
	if !strings.Contains(ErrStreamTruncated.Hint(), "拆分") &&
		!strings.Contains(ErrStreamTruncated.Hint(), "max_tokens") {
		t.Errorf("Hint not actionable; got %q", ErrStreamTruncated.Hint())
	}
}

// TestErrStreamFiltered_Triplet.
func TestErrStreamFiltered_Triplet(t *testing.T) {
	if ErrStreamFiltered.Code() != "RENDER.STREAM_FILTERED" {
		t.Errorf("Code()=%q, want RENDER.STREAM_FILTERED", ErrStreamFiltered.Code())
	}
	if ErrStreamFiltered.Message() == "" {
		t.Error("Message empty")
	}
	if !strings.Contains(ErrStreamFiltered.Hint(), "过滤") &&
		!strings.Contains(ErrStreamFiltered.Hint(), "重新表述") {
		t.Errorf("Hint not actionable; got %q", ErrStreamFiltered.Hint())
	}
}

// TestErrStreamCancelled_Triplet (R2 D1 surface).
func TestErrStreamCancelled_Triplet(t *testing.T) {
	if ErrStreamCancelled.Code() != "RENDER.STREAM_CANCELLED" {
		t.Errorf("Code()=%q, want RENDER.STREAM_CANCELLED", ErrStreamCancelled.Code())
	}
	if ErrStreamCancelled.Message() == "" {
		t.Error("Message empty")
	}
	if !strings.Contains(ErrStreamCancelled.Hint(), "ctx") &&
		!strings.Contains(ErrStreamCancelled.Hint(), "取消") &&
		!strings.Contains(ErrStreamCancelled.Hint(), "context") {
		t.Errorf("Hint not actionable; got %q", ErrStreamCancelled.Hint())
	}
}

// TestErrcodeSentinelsAreErrcodeError — errors.As works on each sentinel.
func TestErrcodeSentinelsAreErrcodeError(t *testing.T) {
	cases := []errcode.Error{ErrStreamTruncated, ErrStreamFiltered, ErrStreamCancelled}
	for _, e := range cases {
		var ec errcode.Error
		if !errors.As(error(e), &ec) {
			t.Errorf("sentinel %q not errcode.Error", e.Code())
		}
	}
}
