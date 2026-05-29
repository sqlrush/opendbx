// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package llmapp

import (
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

// finishFromErr was retired in spec-1.21 T-8: the diagnose.Loop owns
// terminal classification end-to-end (classifyTerminal / classifyEmitErr
// / classifyToolErr) and surfaces the already-classified
// FinishReason+Err on EventFinish. The llmapp adapter never re-classifies
// — it just renders. So no TestFinishFromErr / BenchmarkFinishFromErr.

// BenchmarkMapToRender targets spec-1.20 § 4.4 (pure map; well under 500 ns/op).
func BenchmarkMapToRender(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = mapToRender(llm.FinishLength)
	}
}
