// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package uiinvariant

import (
	"fmt"
	"strconv"
	"strings"
	"testing"
)

// sgrState tracks active attributes during SGR stream parse.
// Spec-0.11.5 § 2.2 finite-state SGR validator (replaced R1's broken
// open/close stack model after codex HIGH-5 + claude HIGH-4 review).
//
// fg/bg: -1 = default; 0-7 standard; 8-15 bright; 16+ for 256-color
// and 24-bit (encoded with a sentinel offset; presence/absence is
// what matters for residual-at-stream-end check).
type sgrState struct {
	fg, bg                              int
	bold, italic, underline, reverse bool
}

func newSGRState() sgrState {
	return sgrState{fg: -1, bg: -1}
}

// isDefault reports whether state has any active attribute (any fg/bg
// color set or any flag).
func (s sgrState) hasActive() bool {
	return s.fg != -1 || s.bg != -1 || s.bold || s.italic || s.underline || s.reverse
}

// CheckANSI validates an ANSI-encoded byte stream. Failures:
//   - malformed CSI escape (missing terminator / unknown intermediate)
//   - truncated escape (ESC [ ... without final byte)
//   - active SGR attributes at stream end (residual fg/bg/bold/etc.)
//   - invalid SGR parameter (overflow, unknown subcode)
//
// Targeted resets (22m 23m 24m 27m 39m 49m) are valid regardless of
// prior opens. 0m is the universal reset.
//
// spec-0.11.5 D-1 (state-based, not stack-based). R5 covers 256-color
// (38;5;N + 48;5;N) and 24-bit (38;2;R;G;B + 48;2;R;G;B).
func CheckANSI(t testing.TB, raw []byte) {
	t.Helper()
	state := newSGRState()
	i := 0
	for i < len(raw) {
		c := raw[i]
		if c != 0x1B { // ESC
			i++
			continue
		}
		// Look for CSI: ESC [
		if i+1 >= len(raw) {
			t.Fatalf("CheckANSI: truncated ESC at byte %d", i)
			return
		}
		if raw[i+1] != '[' {
			// Non-CSI escape (e.g., OSC ESC ]); skip the next byte
			// conservatively — opendbx output should be CSI-only.
			i += 2
			continue
		}
		// Parse CSI parameters until final byte.
		// Param byte: 0x30-0x3F (digits, ; : < = > ?).
		// Intermediate byte: 0x20-0x2F (space + ! " # $ % & ' ( ) * + , - . /).
		// Final byte: 0x40-0x7E.
		// Any byte outside these classes is malformed.
		paramStart := i + 2
		end := paramStart
		for end < len(raw) {
			b := raw[end]
			if b >= 0x40 && b <= 0x7E {
				break // final byte found
			}
			if (b >= 0x30 && b <= 0x3F) || (b >= 0x20 && b <= 0x2F) {
				end++
				continue
			}
			t.Fatalf("CheckANSI: malformed CSI param byte 0x%02X at byte %d (raw: %q)",
				b, end, raw[i:end+1])
			return
		}
		if end >= len(raw) {
			t.Fatalf("CheckANSI: truncated CSI at byte %d (no final byte)", i)
			return
		}
		final := raw[end]
		params := string(raw[paramStart:end])
		switch final {
		case 'm':
			if err := applySGR(&state, params); err != nil {
				t.Fatalf("CheckANSI: SGR at byte %d: %v (raw: %q)",
					i, err, raw[i:end+1])
				return
			}
		case 'A', 'B', 'C', 'D', 'E', 'F', 'G', 'H', 'J', 'K', 'f', 'n', 's', 'u', 'h', 'l':
			// Cursor / erase / mode CSIs — not SGR; ignore.
		default:
			// Unknown final byte; tolerate (vt10x may emit OSC / other
			// CSI that uiinvariant doesn't model).
		}
		i = end + 1
	}
	if state.hasActive() {
		t.Fatalf("CheckANSI: residual active SGR at stream end: %+v "+
			"(missing reset \\x1b[0m)", state)
	}
}

func applySGR(state *sgrState, params string) error {
	if params == "" {
		// "ESC [ m" with no params is treated as ESC [ 0 m (reset).
		*state = newSGRState()
		return nil
	}
	parts := strings.Split(params, ";")
	nums := make([]int, len(parts))
	for k, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return fmt.Errorf("malformed CSI param %q", p)
		}
		if n < 0 || n > 65535 {
			return fmt.Errorf("CSI param out of range: %d", n)
		}
		nums[k] = n
	}
	// Index loop (not range) so 38;5;N and 38;2;R;G;B can consume
	// following params atomically.
	for idx := 0; idx < len(nums); idx++ {
		n := nums[idx]
		switch {
		case n == 0:
			*state = newSGRState()
		case n == 1:
			state.bold = true
		case n == 3:
			state.italic = true
		case n == 4:
			state.underline = true
		case n == 7:
			state.reverse = true
		case n == 22:
			state.bold = false
		case n == 23:
			state.italic = false
		case n == 24:
			state.underline = false
		case n == 27:
			state.reverse = false
		case n == 39:
			state.fg = -1
		case n == 49:
			state.bg = -1
		case n >= 30 && n <= 37:
			state.fg = n - 30
		case n >= 40 && n <= 47:
			state.bg = n - 40
		case n >= 90 && n <= 97:
			state.fg = n - 90 + 8
		case n >= 100 && n <= 107:
			state.bg = n - 100 + 8
		case n == 38 || n == 48:
			// Extended color: 38;5;N (256) or 38;2;R;G;B (24-bit).
			if idx+1 >= len(nums) {
				return fmt.Errorf("CSI %d missing mode subparam", n)
			}
			mode := nums[idx+1]
			switch mode {
			case 5:
				if idx+2 >= len(nums) {
					return fmt.Errorf("CSI %d;5 missing color index", n)
				}
				color := nums[idx+2]
				if color < 0 || color > 255 {
					return fmt.Errorf("CSI %d;5 color out of [0,255]: %d", n, color)
				}
				if n == 38 {
					state.fg = 16 + color
				} else {
					state.bg = 16 + color
				}
				idx += 2
			case 2:
				if idx+4 >= len(nums) {
					return fmt.Errorf("CSI %d;2 missing R;G;B", n)
				}
				r, g, b := nums[idx+2], nums[idx+3], nums[idx+4]
				if r > 255 || g > 255 || b > 255 {
					return fmt.Errorf("CSI %d;2 RGB out of [0,255]: %d,%d,%d", n, r, g, b)
				}
				// Encode RGB into a sentinel int (>= 0x10000).
				if n == 38 {
					state.fg = 0x10000 | (r << 16) | (g << 8) | b
				} else {
					state.bg = 0x10000 | (r << 16) | (g << 8) | b
				}
				idx += 4
			default:
				return fmt.Errorf("CSI %d unknown mode subparam: %d", n, mode)
			}
		default:
			// Unknown SGR code — tolerate (vt10x may emit ones uiinvariant
			// doesn't track; not all unknown codes are errors).
		}
	}
	return nil
}
