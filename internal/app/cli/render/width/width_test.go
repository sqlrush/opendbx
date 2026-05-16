// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package width

import (
	"errors"
	"strings"
	"testing"
)

// --- Width -----------------------------------------------------------

func TestWidth(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		s    string
		want int
	}{
		{"empty", "", 0},
		{"ascii", "hello", 5},
		{"ascii_with_space", "hello world", 11},
		{"cjk_each_width_2", "中文", 4},
		{"mixed_ascii_cjk", "中文 hello", 10}, // 中=2 + 文=2 + space=1 + hello=5 = 10
		{"flag_emoji_runewidth_1", "🇨🇳", 1}, // runewidth treats RIs as 1 col under EastAsianWidth=false (no glyph composition)
		{"newline_treated_as_zero", "\n", 0},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := Width(c.s); got != c.want {
				t.Errorf("Width(%q) = %d, want %d", c.s, got, c.want)
			}
		})
	}
}

// --- Wrap ------------------------------------------------------------

func TestWrap(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		s    string
		cols int
		want []string
	}{
		{"empty_input", "", 10, []string{}},
		{"fits_in_one_line", "hello", 10, []string{"hello"}},
		{"exact_fit", "hello", 5, []string{"hello"}},
		{"breaks_basic_ascii", "abcdefghij", 4, []string{"abcd", "efgh", "ij"}},
		{"breaks_cjk_each_2_cols", "中文中文中", 4, []string{"中文", "中文", "中"}},
		{"single_column", "abc", 1, []string{"a", "b", "c"}},
		// T-13 code-reviewer MED-1 + security-reviewer LOW-1: pin the
		// degenerate behavior when a CJK rune is wider than cols. Current
		// spec-0.13 D-2 contract emits an empty line before the rune; if
		// spec-1.3 changes this, update both the test and width.go godoc.
		{"cjk_into_1_col_degenerate", "中文", 1, []string{"", "中", "文"}},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			got := Wrap(c.s, c.cols)
			if len(got) != len(c.want) {
				t.Fatalf("Wrap(%q, %d) = %v (len %d), want %v (len %d)",
					c.s, c.cols, got, len(got), c.want, len(c.want))
			}
			for i := range got {
				if got[i] != c.want[i] {
					t.Errorf("Wrap(%q, %d)[%d] = %q, want %q", c.s, c.cols, i, got[i], c.want[i])
				}
			}
		})
	}
}

func TestWrap_PanicOnNonPositiveCols(t *testing.T) {
	t.Parallel()
	for _, cols := range []int{0, -1, -100} {
		cols := cols
		t.Run("", func(t *testing.T) {
			t.Parallel()
			defer func() {
				r := recover()
				if r == nil {
					t.Fatalf("Wrap(s, %d) did not panic", cols)
				}
				err, ok := r.(error)
				if !ok || !errors.Is(err, ErrInvalidWidth) {
					t.Errorf("Wrap panic recover = %v, want ErrInvalidWidth", r)
				}
			}()
			_ = Wrap("hello", cols)
		})
	}
}

// --- Truncate --------------------------------------------------------

func TestTruncate(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		s    string
		max  int
		want string
	}{
		{"empty_unchanged", "", 5, ""},
		{"fits_exactly", "hello", 5, "hello"},
		{"fits_under", "hi", 5, "hi"},
		{"truncates_ascii_5_to_3", "hello", 3, "he…"},
		{"truncates_cjk_4_to_3", "中文你", 3, "中…"},
		{"max_1_returns_ellipsis_only", "hello", 1, "…"},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := Truncate(c.s, c.max); got != c.want {
				t.Errorf("Truncate(%q, %d) = %q, want %q", c.s, c.max, got, c.want)
			}
		})
	}
}

func TestTruncate_PanicOnNonPositiveMax(t *testing.T) {
	t.Parallel()
	defer func() {
		r := recover()
		if r == nil {
			t.Fatalf("Truncate(s, 0) did not panic")
		}
		err, ok := r.(error)
		if !ok || !errors.Is(err, ErrInvalidWidth) {
			t.Errorf("Truncate panic recover = %v, want ErrInvalidWidth", r)
		}
	}()
	_ = Truncate("hello", 0)
}

// --- Pad -------------------------------------------------------------

func TestPad(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		s      string
		target int
		want   string
	}{
		{"empty_padded", "", 3, "   "},
		{"ascii_right_pad", "hi", 5, "hi   "},
		{"exact_unchanged", "hello", 5, "hello"},
		{"already_wider_unchanged", "hellomore", 5, "hellomore"},
		{"cjk_pad", "中", 5, "中   "}, // 中=2 + 3 spaces = 5
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			got := Pad(c.s, c.target)
			if got != c.want {
				t.Errorf("Pad(%q, %d) = %q, want %q", c.s, c.target, got, c.want)
			}
			// Padded result should reach target width if input fits.
			if Width(c.s) <= c.target && Width(got) != c.target {
				t.Errorf("Pad(%q, %d) → %q width = %d, want target = %d",
					c.s, c.target, got, Width(got), c.target)
			}
		})
	}
}

func TestPad_PanicOnNonPositiveTarget(t *testing.T) {
	t.Parallel()
	defer func() {
		r := recover()
		if r == nil {
			t.Fatalf("Pad(s, 0) did not panic")
		}
	}()
	_ = Pad("hi", 0)
}

// --- ErrInvalidWidth errcode contract -------------------------------

func TestErrInvalidWidth_Errcode(t *testing.T) {
	t.Parallel()
	if ErrInvalidWidth.Code() != "RENDER.INVALID_WIDTH" {
		t.Errorf("ErrInvalidWidth.Code() = %q", ErrInvalidWidth.Code())
	}
	if !strings.Contains(ErrInvalidWidth.Hint(), "cols/max/target") {
		t.Errorf("ErrInvalidWidth.Hint() should mention cols/max/target; got %q", ErrInvalidWidth.Hint())
	}
}
