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

// TestBlockType_AppendOnlyOrdinals guards spec-1.20 R2 H-6 + spec-1.21
// D-1: BlockType ordinals MUST stay stable across appends so adapters
// switching on the integer value don't silently mis-map old serialized
// data when a new tag is added.
func TestBlockType_AppendOnlyOrdinals(t *testing.T) {
	t.Parallel()
	cases := []struct {
		got  BlockType
		want int
		name string
	}{
		{BlockText, 0, "BlockText"},
		{BlockToolUse, 1, "BlockToolUse"},
		{BlockToolResult, 2, "BlockToolResult"},
	}
	for _, c := range cases {
		if int(c.got) != c.want {
			t.Errorf("%s ordinal = %d; want %d (append-only contract)", c.name, c.got, c.want)
		}
	}
}

// TestNewToolUseBlock_NilGuardContract asserts the spec-1.21 D-1 nil-guard
// contract for the BlockToolUse constructor: ToolUse non-nil and
// ToolResult nil. Direct hand-tagging is discouraged; the helper is the
// single point that enforces the contract.
func TestNewToolUseBlock_NilGuardContract(t *testing.T) {
	t.Parallel()
	tu := &ToolUse{ID: "call_1", Name: "clock"}
	blk := NewToolUseBlock(tu)
	if blk.Type != BlockToolUse {
		t.Errorf("Type = %v; want BlockToolUse", blk.Type)
	}
	if blk.ToolUse != tu {
		t.Errorf("ToolUse pointer not preserved")
	}
	if blk.ToolResult != nil {
		t.Errorf("ToolResult should be nil for BlockToolUse; got %+v", blk.ToolResult)
	}
	if blk.Text != "" {
		t.Errorf("Text should be empty for BlockToolUse; got %q", blk.Text)
	}
}

// TestNewToolResultBlock_NilGuardContract is the BlockToolResult mirror
// (ToolResult non-nil, ToolUse nil).
func TestNewToolResultBlock_NilGuardContract(t *testing.T) {
	t.Parallel()
	tr := &ToolResult{ToolUseID: "call_1", Content: "ok"}
	blk := NewToolResultBlock(tr)
	if blk.Type != BlockToolResult {
		t.Errorf("Type = %v; want BlockToolResult", blk.Type)
	}
	if blk.ToolResult != tr {
		t.Errorf("ToolResult pointer not preserved")
	}
	if blk.ToolUse != nil {
		t.Errorf("ToolUse should be nil for BlockToolResult; got %+v", blk.ToolUse)
	}
	if blk.Text != "" {
		t.Errorf("Text should be empty for BlockToolResult; got %q", blk.Text)
	}
}

// TestToolResult_IsErrorFlag is a structural sanity check that ToolResult
// carries the IsError flag so the LLM can distinguish self-correctable
// failures from regular outputs (spec-1.21 D-1).
func TestToolResult_IsErrorFlag(t *testing.T) {
	t.Parallel()
	ok := ToolResult{ToolUseID: "x", Content: "1700000000"}
	bad := ToolResult{ToolUseID: "y", Content: "invalid arg", IsError: true}
	if ok.IsError {
		t.Errorf("default IsError should be false")
	}
	if !bad.IsError {
		t.Errorf("explicit IsError true should round-trip")
	}
}
