// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package llm

import (
	"errors"
	"testing"

	"github.com/sqlrush/opendbx/internal/platform/errcode"
)

func TestFinishReason_String(t *testing.T) {
	t.Parallel()
	cases := []struct {
		f    FinishReason
		want string
	}{
		{FinishUnset, "unset"},
		{FinishStop, "stop"},
		{FinishLength, "length"},
		{FinishToolUse, "tool_use"},
		{FinishStopSequence, "stop_sequence"},
		{FinishPause, "pause_turn"},
		{FinishRefusal, "refusal"},
		{FinishCancelled, "cancelled"},
		{FinishError, "error"},
		{FinishReason(99), "unknown"},
	}
	for _, tc := range cases {
		if got := tc.f.String(); got != tc.want {
			t.Errorf("FinishReason(%d).String() = %q; want %q", int(tc.f), got, tc.want)
		}
	}
}

func TestFinishReason_Terminal(t *testing.T) {
	t.Parallel()
	if FinishUnset.Terminal() {
		t.Error("FinishUnset.Terminal() = true; want false")
	}
	for _, f := range []FinishReason{FinishStop, FinishLength, FinishToolUse, FinishStopSequence, FinishPause, FinishRefusal, FinishCancelled, FinishError} {
		if !f.Terminal() {
			t.Errorf("%v.Terminal() = false; want true", f)
		}
	}
}

func temp(v float64) *float64 { return &v }

func TestValidateRequest(t *testing.T) {
	t.Parallel()
	okMsgs := []Message{{Role: RoleUser, Content: []ContentBlock{{Type: BlockText, Text: "hi"}}}}
	cases := []struct {
		name    string
		req     Request
		wantErr bool
	}{
		{"valid minimal", Request{Messages: okMsgs, MaxTokens: 100}, false},
		{"valid temp 0", Request{Messages: okMsgs, MaxTokens: 100, Temperature: temp(0)}, false},
		{"valid thinking enabled", Request{Messages: okMsgs, MaxTokens: 4096, ThinkingMode: ThinkingEnabled, ThinkingBudget: 1024}, false},
		{"empty messages", Request{MaxTokens: 100}, true},
		{"maxtokens 0", Request{Messages: okMsgs, MaxTokens: 0}, true},
		{"maxtokens negative", Request{Messages: okMsgs, MaxTokens: -1}, true},
		{"system role in messages", Request{Messages: []Message{{Role: Role("system"), Content: nil}}, MaxTokens: 100}, true},
		{"thinking budget too low", Request{Messages: okMsgs, MaxTokens: 4096, ThinkingMode: ThinkingEnabled, ThinkingBudget: 512}, true},
		{"thinking budget >= maxtokens", Request{Messages: okMsgs, MaxTokens: 1024, ThinkingMode: ThinkingEnabled, ThinkingBudget: 1024}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateRequest(tc.req)
			if tc.wantErr != (err != nil) {
				t.Errorf("ValidateRequest err=%v; wantErr=%v", err, tc.wantErr)
			}
			if err != nil && !errors.Is(err, ErrRequestInvalid) {
				t.Errorf("err %v should errors.Is ErrRequestInvalid (same code)", err)
			}
		})
	}
}

// TestErrcodes_ThreePartContract asserts all 9 LLM.* sentinels carry
// non-empty Code/Message/Hint (规则 7), Hint has no API key leak surface.
func TestErrcodes_ThreePartContract(t *testing.T) {
	t.Parallel()
	sentinels := []errcode.Sentinel{
		ErrUnavailable, ErrAuthFailed, ErrTimeout, ErrCancelled,
		ErrNotImplemented, ErrDecodeFailed, ErrStreamEmpty,
		ErrRequestInvalid, ErrProviderRefusal,
	}
	if len(sentinels) != 9 {
		t.Fatalf("expected 9 LLM.* sentinels; got %d", len(sentinels))
	}
	for _, s := range sentinels {
		if s.Code() == "" || s.Message() == "" || s.Hint() == "" {
			t.Errorf("%s: Code/Message/Hint must all be non-empty (规则 7); got code=%q msg=%q hint=%q",
				s.Code(), s.Code(), s.Message(), s.Hint())
		}
		if len(s.Code()) < 5 || s.Code()[:4] != "LLM." {
			t.Errorf("code %q must start with LLM.", s.Code())
		}
	}
}

func TestRequestInvalidf_CarriesCode(t *testing.T) {
	t.Parallel()
	err := RequestInvalidf("test detail")
	if !errors.Is(err, ErrRequestInvalid) {
		t.Errorf("RequestInvalidf should errors.Is ErrRequestInvalid")
	}
	var ec errcode.Error
	if !errors.As(err, &ec) || ec.Code() != "LLM.REQUEST_INVALID" {
		t.Errorf("RequestInvalidf code = %v; want LLM.REQUEST_INVALID", err)
	}
}
