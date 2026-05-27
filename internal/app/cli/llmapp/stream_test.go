// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package llmapp

import (
	"context"
	"errors"
	"testing"

	"github.com/sqlrush/opendbx/internal/app/cli/render/streaming"
	"github.com/sqlrush/opendbx/internal/domain/llm"
)

func TestMapToRender_AllNine(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   llm.FinishReason
		want streaming.FinishReason
	}{
		{llm.FinishUnset, streaming.FinishUnset},
		{llm.FinishStop, streaming.FinishStop},
		{llm.FinishLength, streaming.FinishLength},
		{llm.FinishToolUse, streaming.FinishStop},
		{llm.FinishStopSequence, streaming.FinishStop},
		{llm.FinishPause, streaming.FinishStop},
		{llm.FinishRefusal, streaming.FinishStop},
		{llm.FinishCancelled, streaming.FinishCancelled},
		{llm.FinishError, streaming.FinishError},
	}
	for _, tc := range cases {
		if got := mapToRender(tc.in); got != tc.want {
			t.Errorf("mapToRender(%v) = %v; want %v", tc.in, got, tc.want)
		}
	}
}

func TestFinishFromErr(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		err     error
		wantFR  llm.FinishReason
		wantErr error
	}{
		{"nil", nil, llm.FinishStop, nil},
		{"canceled", context.Canceled, llm.FinishCancelled, nil},
		{"deadline", context.DeadlineExceeded, llm.FinishError, llm.ErrTimeout},
		{"generic", errors.New("boom"), llm.FinishError, errors.New("boom")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fr, err := finishFromErr(tc.err)
			if fr != tc.wantFR {
				t.Errorf("finishFromErr(%v) fr = %v; want %v", tc.err, fr, tc.wantFR)
			}
			if tc.name == "deadline" && !errors.Is(err, llm.ErrTimeout) {
				t.Errorf("deadline err = %v; want ErrTimeout", err)
			}
			if (tc.wantErr == nil) != (err == nil) && tc.name != "generic" {
				t.Errorf("finishFromErr(%v) err = %v; want nil=%v", tc.err, err, tc.wantErr == nil)
			}
		})
	}
}

// BenchmarkMapToRender targets spec-1.20 § 4.4 (pure map; well under 500 ns/op).
func BenchmarkMapToRender(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = mapToRender(llm.FinishLength)
	}
}

// BenchmarkFinishFromErr targets the terminal-error classifier.
func BenchmarkFinishFromErr(b *testing.B) {
	err := context.Canceled
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = finishFromErr(err)
	}
}
