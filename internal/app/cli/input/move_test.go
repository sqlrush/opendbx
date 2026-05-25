// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package input

import "testing"

// TestMoveCursor covers all 5 MovementKind values × empty / non-empty
// buffers and the MED-9 clamp invariant.
func TestMoveCursor(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		buffer     string
		cursor     int
		movement   MovementKind
		wantCursor int
	}{
		// Empty buffer.
		{"empty + Left → 0", "", 0, MoveLeft, 0},
		{"empty + Right → 0", "", 0, MoveRight, 0},
		{"empty + Home → 0", "", 0, MoveHome, 0},
		{"empty + End → 0", "", 0, MoveEnd, 0},
		{"empty + None → 0", "", 0, MoveNone, 0},

		// Single-rune buffer "a", cursor 0.
		{"'a' c0 + Left → 0 (clamped)", "a", 0, MoveLeft, 0},
		{"'a' c0 + Right → 1", "a", 0, MoveRight, 1},
		{"'a' c0 + Home → 0", "a", 0, MoveHome, 0},
		{"'a' c0 + End → 1", "a", 0, MoveEnd, 1},

		// 3-rune buffer "abc", cursor in middle.
		{"'abc' c1 + Left → 0", "abc", 1, MoveLeft, 0},
		{"'abc' c1 + Right → 2", "abc", 1, MoveRight, 2},
		{"'abc' c1 + Home → 0", "abc", 1, MoveHome, 0},
		{"'abc' c1 + End → 3", "abc", 1, MoveEnd, 3},
		{"'abc' c1 + None → 1", "abc", 1, MoveNone, 1},

		// Cursor at end.
		{"'abc' c3 + Right → 3 (clamped)", "abc", 3, MoveRight, 3},
		{"'abc' c3 + End → 3", "abc", 3, MoveEnd, 3},

		// UTF-8 multi-byte runes.
		{"'中文' c0 + Right → 1", "中文", 0, MoveRight, 1},
		{"'中文' c2 + Left → 1", "中文", 2, MoveLeft, 1},
		{"'中文' c0 + End → 2 (rune count)", "中文", 0, MoveEnd, 2},

		// MED-9 clamp on input cursor.
		{"clamp cursor -1 then End", "abc", -1, MoveEnd, 3},
		{"clamp cursor 999 then Left", "abc", 999, MoveLeft, 2},
		{"clamp cursor -5 then None", "abc", -5, MoveNone, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := MoveCursor(tc.buffer, tc.cursor, tc.movement)
			if got != tc.wantCursor {
				t.Errorf("MoveCursor(%q, %d, %v) = %d; want %d", tc.buffer, tc.cursor, tc.movement, got, tc.wantCursor)
			}
		})
	}
}

// TestMovementKindString covers human-readable names + unknown fallback.
func TestMovementKindString(t *testing.T) {
	t.Parallel()
	cases := []struct {
		m    MovementKind
		want string
	}{
		{MoveNone, "None"},
		{MoveLeft, "Left"},
		{MoveRight, "Right"},
		{MoveHome, "Home"},
		{MoveEnd, "End"},
		{MovementKind(99), "MovementKind(?)"},
	}
	for _, tc := range cases {
		t.Run(tc.want, func(t *testing.T) {
			if got := tc.m.String(); got != tc.want {
				t.Errorf("MovementKind(%d).String() = %q; want %q", int(tc.m), got, tc.want)
			}
		})
	}
}

// BenchmarkMoveCursor targets spec-1.17 § 4.4 < 100 ns/op.
func BenchmarkMoveCursor(b *testing.B) {
	const buf = "Lorem ipsum dolor sit amet, consectetur adipiscing elit"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = MoveCursor(buf, 25, MoveLeft)
	}
}
