// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package block

import "testing"

// Fence parser tests per spec-1.7 § 4.2 fence_test 12+ cases.
// Validates CommonMark backtick fence rules with spec-1.6 R-12 robustness.

func TestFence_Empty(t *testing.T) {
	if got := scanFenceRanges(""); got != nil {
		t.Fatalf("empty text = %+v, want nil", got)
	}
}

func TestFence_BasicBacktick(t *testing.T) {
	ranges := scanFenceRanges("```go\nbody\n```")
	if len(ranges) != 1 {
		t.Fatalf("ranges = %+v, want 1", ranges)
	}
	r := ranges[0]
	if r.Start != 0 || r.End != 2 || r.Lang != "go" || !r.Closed || r.OpenRun != 3 {
		t.Fatalf("range = %+v, want {Start:0 End:2 Lang:go Closed:true OpenRun:3}", r)
	}
}

func TestFence_NoLang(t *testing.T) {
	ranges := scanFenceRanges("```\nbody\n```")
	if len(ranges) != 1 || ranges[0].Lang != "" {
		t.Fatalf("ranges = %+v, want 1 range with empty Lang", ranges)
	}
}

func TestFence_4Backtick_Inner3Backtick(t *testing.T) {
	text := "````go\ninner ``` legal\n````"
	ranges := scanFenceRanges(text)
	if len(ranges) != 1 {
		t.Fatalf("ranges = %+v, want 1 (4-backtick fence containing inner ```)", ranges)
	}
	if ranges[0].OpenRun != 4 {
		t.Errorf("OpenRun = %d, want 4", ranges[0].OpenRun)
	}
}

func TestFence_Unclosed(t *testing.T) {
	ranges := scanFenceRanges("```go\nbody no close")
	if len(ranges) != 1 {
		t.Fatalf("ranges = %+v, want 1", ranges)
	}
	r := ranges[0]
	if r.Closed {
		t.Error("unclosed fence should have Closed=false")
	}
	if r.End != 1 {
		t.Errorf("unclosed End=%d, want 1 (last line index)", r.End)
	}
}

func TestFence_InlineBacktickNotFence(t *testing.T) {
	ranges := scanFenceRanges("this is `inline` code")
	if len(ranges) != 0 {
		t.Fatalf("inline backtick should not be fence; got %+v", ranges)
	}
}

func TestFence_LineStartPadding(t *testing.T) {
	// 2-space padding (valid per CommonMark 0-3).
	ranges := scanFenceRanges("  ```go\ncode\n  ```")
	if len(ranges) != 1 {
		t.Fatalf("2-space padded fence not detected: %+v", ranges)
	}
}

func TestFence_4PlusSpaceIndentNotFence(t *testing.T) {
	// 4-space indent: NOT fence (indented code block per CommonMark).
	ranges := scanFenceRanges("    ```go\nbody\n    ```")
	if len(ranges) != 0 {
		t.Fatalf("4-space indent should NOT be fence; got %+v", ranges)
	}
}

func TestFence_MultipleInText(t *testing.T) {
	text := "```a\nx\n```\n```b\ny\n```"
	ranges := scanFenceRanges(text)
	if len(ranges) != 2 {
		t.Fatalf("multi-fence: got %d, want 2", len(ranges))
	}
	if ranges[0].Lang != "a" || ranges[1].Lang != "b" {
		t.Errorf("langs mismatch: %+v", ranges)
	}
}

func TestFence_LangLabelWithHashAndDot(t *testing.T) {
	// spec-1.7 R2 D6 HIGH-J — # and . should be accepted.
	cases := []struct {
		text string
		lang string
	}{
		{"```c#\ncode\n```", "c#"},
		{"```f#\ncode\n```", "f#"},
		{"```.sh\ncode\n```", ".sh"},
		{"```go\ncode\n```", "go"},
	}
	for _, c := range cases {
		ranges := scanFenceRanges(c.text)
		if len(ranges) != 1 || ranges[0].Lang != c.lang {
			t.Errorf("text=%q lang=%q got %+v", c.text, c.lang, ranges)
		}
	}
}

func TestFence_LangLabelTrailingWhitespace(t *testing.T) {
	ranges := scanFenceRanges("```go   \ncode\n```")
	if len(ranges) != 1 || ranges[0].Lang != "go" {
		t.Fatalf("trailing whitespace after lang label: %+v", ranges)
	}
}

func TestFence_OpeningLineExtraText_NotFence(t *testing.T) {
	// CommonMark: after lang label, only whitespace allowed.
	// Line 0 "```go extra stuff" rejected (extra non-whitespace).
	// Line 2 "```" becomes new (unclosed) fence opener — that's strict
	// CommonMark behavior (any standalone ``` line opens a fence).
	ranges := scanFenceRanges("```go extra stuff\ncode\n```")
	// Should NOT find a fence starting at line 0 (the malformed opener).
	for _, r := range ranges {
		if r.Start == 0 {
			t.Fatalf("malformed opener line 0 should not be a fence; got %+v", r)
		}
	}
}

func TestFence_2BackticksNotFence(t *testing.T) {
	ranges := scanFenceRanges("``go\ncode\n``")
	if len(ranges) != 0 {
		t.Fatalf("2-backtick should NOT be fence; got %+v", ranges)
	}
}

func TestFence_CloseRequiresMatchingRun(t *testing.T) {
	// 4-backtick open, 3-backtick "close" → still unclosed (close needs ≥4).
	text := "````\nbody\n```\nstill in fence"
	ranges := scanFenceRanges(text)
	if len(ranges) != 1 {
		t.Fatalf("ranges=%+v", ranges)
	}
	if ranges[0].Closed {
		t.Error("3-backtick close inside 4-backtick fence: should remain unclosed")
	}
}
