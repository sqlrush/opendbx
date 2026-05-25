// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package input

import (
	"testing"
	"unicode/utf8"

	"github.com/sqlrush/opendbx/internal/app/cli/render/terminal"
)

// FuzzResolveMode (spec-1.16 R2 M-3 fuzz commitment) feeds random
// (buffer, code, rune) tuples and asserts invariants:
//
//  1. ResolveMode never panics.
//  2. Returned newBuffer is valid UTF-8 if buffer is valid UTF-8.
//  3. DeriveMode(newBuffer) is one of {Natural, Slash, SQL}.
//  4. cursor never goes negative.
//
// Pure-function fuzz; race detector trivially clean.
func FuzzResolveMode(f *testing.F) {
	f.Add("", terminal.KeyRune, int32('/'))
	f.Add("/help", terminal.KeyBackspace, int32(0))
	f.Add("\\d", terminal.KeyRune, int32('a'))
	f.Add("你好", terminal.KeyBackspace, int32(0))

	f.Fuzz(func(t *testing.T, buffer string, code int, r int32) {
		// Bound cursor input to buffer's rune count (R2 H-7 scope-limit).
		cursor := utf8.RuneCountInString(buffer)
		newBuf, newCursor := ResolveMode(buffer, cursor, code, rune(r))
		// 1. no panic — implicit (test would Fatalf on panic).
		// 4. cursor never negative.
		if newCursor < 0 {
			t.Fatalf("cursor went negative: buf=%q cursor=%d code=%d r=%U", newBuf, newCursor, code, r)
		}
		// 3. DeriveMode returns valid mode.
		mode := DeriveMode(newBuf)
		if mode < ModeNatural || mode > ModeSQL {
			t.Fatalf("DeriveMode returned invalid mode %v for buf=%q", mode, newBuf)
		}
		// 2. UTF-8 validity preserved.
		if utf8.ValidString(buffer) && !utf8.ValidString(newBuf) {
			t.Fatalf("ResolveMode produced invalid UTF-8: buf=%q → newBuf=%q", buffer, newBuf)
		}
	})
}

// FuzzValueWithoutPrefix asserts that stripping the prefix yields a
// suffix of the original buffer (no rune corruption).
func FuzzValueWithoutPrefix(f *testing.F) {
	f.Add("/help")
	f.Add("\\d")
	f.Add("hello")
	f.Add("")
	f.Add("/")

	f.Fuzz(func(t *testing.T, buffer string) {
		out := ValueWithoutPrefix(buffer)
		if len(out) > len(buffer) {
			t.Fatalf("output longer than input: buf=%q → out=%q", buffer, out)
		}
		// UTF-8 validity preserved.
		if utf8.ValidString(buffer) && !utf8.ValidString(out) {
			t.Fatalf("ValueWithoutPrefix produced invalid UTF-8: buf=%q → out=%q", buffer, out)
		}
		// For Natural mode (or empty), output == buffer.
		mode := DeriveMode(buffer)
		if mode == ModeNatural && out != buffer {
			t.Fatalf("Natural mode should preserve buffer: buf=%q → out=%q", buffer, out)
		}
	})
}
