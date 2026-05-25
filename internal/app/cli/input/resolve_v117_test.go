// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package input

import (
	"strings"
	"testing"

	"github.com/sqlrush/opendbx/internal/app/cli/render/terminal"
)

// --- spec-1.17 R2 D-2 — cursor任意位置 + KeyDelete (T1-14..T1-25) ---

// TestResolveMode_InsertMiddle covers KeyRune insert at a non-end
// cursor position — the spec-1.16 H-7 scope-limit retirement.
func TestResolveMode_InsertMiddle(t *testing.T) {
	t.Parallel()
	// buffer "ac", cursor=1, insert 'b' → "abc" cursor=2
	buf, cur := ResolveMode("ac", 1, terminal.KeyRune, 'b')
	if buf != "abc" || cur != 2 {
		t.Errorf("insert mid got (%q, %d); want (\"abc\", 2)", buf, cur)
	}
}

// TestResolveMode_InsertStart covers cursor=0 insert (prepend).
func TestResolveMode_InsertStart(t *testing.T) {
	t.Parallel()
	buf, cur := ResolveMode("bc", 0, terminal.KeyRune, 'a')
	if buf != "abc" || cur != 1 {
		t.Errorf("insert start got (%q, %d); want (\"abc\", 1)", buf, cur)
	}
}

// TestResolveMode_InsertMiddle_UTF8 verifies rune-position math holds
// for multi-byte runes (3-byte CJK chars).
func TestResolveMode_InsertMiddle_UTF8(t *testing.T) {
	t.Parallel()
	// "你好" cursor=1 (between the two runes), insert '中' → "你中好" cursor=2
	buf, cur := ResolveMode("你好", 1, terminal.KeyRune, '中')
	if buf != "你中好" || cur != 2 {
		t.Errorf("UTF-8 mid insert got (%q, %d); want (\"你中好\", 2)", buf, cur)
	}
}

// TestResolveMode_BackspaceMiddle covers Backspace at a non-end cursor.
// "abc" cursor=2, Backspace → "ac" cursor=1 (rune at cursor-1 removed).
func TestResolveMode_BackspaceMiddle(t *testing.T) {
	t.Parallel()
	buf, cur := ResolveMode("abc", 2, terminal.KeyBackspace, 0)
	if buf != "ac" || cur != 1 {
		t.Errorf("backspace mid got (%q, %d); want (\"ac\", 1)", buf, cur)
	}
}

// TestResolveMode_KeyDelete_End_NoOp asserts KeyDelete at the end of
// buffer is a no-op (nothing to forward-delete).
func TestResolveMode_KeyDelete_End_NoOp(t *testing.T) {
	t.Parallel()
	buf, cur := ResolveMode("abc", 3, terminal.KeyDelete, 0)
	if buf != "abc" || cur != 3 {
		t.Errorf("Delete at end got (%q, %d); want (\"abc\", 3)", buf, cur)
	}
}

// TestResolveMode_KeyDelete_Start removes the first rune.
func TestResolveMode_KeyDelete_Start(t *testing.T) {
	t.Parallel()
	buf, cur := ResolveMode("abc", 0, terminal.KeyDelete, 0)
	if buf != "bc" || cur != 0 {
		t.Errorf("Delete at start got (%q, %d); want (\"bc\", 0)", buf, cur)
	}
}

// TestResolveMode_KeyDelete_Middle removes the rune AT cursor (cursor unchanged).
func TestResolveMode_KeyDelete_Middle(t *testing.T) {
	t.Parallel()
	buf, cur := ResolveMode("abc", 1, terminal.KeyDelete, 0)
	if buf != "ac" || cur != 1 {
		t.Errorf("Delete mid got (%q, %d); want (\"ac\", 1)", buf, cur)
	}
}

// TestResolveMode_KeyDelete_UTF8 verifies forward delete of a multi-byte rune.
func TestResolveMode_KeyDelete_UTF8(t *testing.T) {
	t.Parallel()
	// "你好" cursor=0, Delete → "好" cursor=0
	buf, cur := ResolveMode("你好", 0, terminal.KeyDelete, 0)
	if buf != "好" || cur != 0 {
		t.Errorf("Delete UTF-8 start got (%q, %d); want (\"好\", 0)", buf, cur)
	}
}

// TestResolveMode_KeyDelete_EmptyNoOp asserts Delete on empty buffer
// is a no-op.
func TestResolveMode_KeyDelete_EmptyNoOp(t *testing.T) {
	t.Parallel()
	buf, cur := ResolveMode("", 0, terminal.KeyDelete, 0)
	if buf != "" || cur != 0 {
		t.Errorf("Delete on empty got (%q, %d); want (\"\", 0)", buf, cur)
	}
}

// TestResolveMode_ClampInvariant covers the spec-1.17 R2 MED-9 clamp
// invariant: cursor out of range is clamped, no panic, behavior matches
// the clamped position.
func TestResolveMode_ClampInvariant(t *testing.T) {
	t.Parallel()
	// cursor=-1 with KeyRune 'a' on "bc" → clamps to 0, inserts at start.
	buf, cur := ResolveMode("bc", -1, terminal.KeyRune, 'a')
	if buf != "abc" || cur != 1 {
		t.Errorf("clamp -1 KeyRune got (%q, %d); want (\"abc\", 1)", buf, cur)
	}

	// cursor=999 with KeyDelete on "abc" → clamps to 3 → no-op.
	buf, cur = ResolveMode("abc", 999, terminal.KeyDelete, 0)
	if buf != "abc" || cur != 3 {
		t.Errorf("clamp 999 KeyDelete got (%q, %d); want (\"abc\", 3)", buf, cur)
	}

	// cursor=-5 with KeyBackspace on "abc" → clamps to 0 → no-op.
	buf, cur = ResolveMode("abc", -5, terminal.KeyBackspace, 0)
	if buf != "abc" || cur != 0 {
		t.Errorf("clamp -5 KeyBackspace got (%q, %d); want (\"abc\", 0)", buf, cur)
	}
}

// TestResolveMode_InsertEnd_ForwardCompat covers the spec-1.16 H-7
// cursor==end behavior — must still hold under the upgraded code path
// (real subset; no behavior change for old callers).
func TestResolveMode_InsertEnd_ForwardCompat(t *testing.T) {
	t.Parallel()
	buf, cur := "", 0
	for _, r := range "/help" {
		buf, cur = ResolveMode(buf, cur, terminal.KeyRune, r)
	}
	if buf != "/help" || cur != 5 {
		t.Errorf("end-only insert got (%q, %d); want (\"/help\", 5)", buf, cur)
	}
}

// TestResolveMode_LongInsertStress runs 600-rune middle inserts to make
// sure the rune-slice copy path doesn't have an off-by-one under load.
func TestResolveMode_LongInsertStress(t *testing.T) {
	t.Parallel()
	src := strings.Repeat("a", 300)
	tail := strings.Repeat("b", 300)
	buf := src + tail
	// Insert 'X' between the a's and b's — cursor=300.
	buf, cur := ResolveMode(buf, 300, terminal.KeyRune, 'X')
	if cur != 301 {
		t.Errorf("cursor after insert = %d; want 301", cur)
	}
	if got := []rune(buf)[300]; got != 'X' {
		t.Errorf("inserted rune at position 300 = %q; want 'X'", got)
	}
	if r := len([]rune(buf)); r != 601 {
		t.Errorf("rune count after insert = %d; want 601", r)
	}
}

// BenchmarkResolveMode_CursorMid targets spec-1.17 § 4.4 < 200 ns/op
// (cursor任意位置 含 rune slice insert; vs spec-1.16 cursor==end fast path).
func BenchmarkResolveMode_CursorMid(b *testing.B) {
	buf := strings.Repeat("a", 40)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ResolveMode(buf, 20, terminal.KeyRune, 'x')
	}
}
