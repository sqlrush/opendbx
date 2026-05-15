// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package uiinvariant

import (
	"fmt"
	"strings"
	"testing"
)

// --- CheckRowWidth -------------------------------------------------

func TestCheckRowWidth_HappyAscii(t *testing.T) {
	t.Parallel()
	grid := []string{"hello", "world", strings.Repeat("a", 10)}
	CheckRowWidth(t, grid, 80)
}

func TestCheckRowWidth_HappyCJK(t *testing.T) {
	t.Parallel()
	grid := []string{"中文", "日本語", "한국어 hello"} // each CJK rune width 2
	CheckRowWidth(t, grid, 80)
}

func TestCheckRowWidth_OverwidthAscii(t *testing.T) {
	t.Parallel()
	mt := &mockT{}
	grid := []string{strings.Repeat("a", 81)}
	CheckRowWidth(mt, grid, 80)
	if !mt.fatalCalled || !strings.Contains(mt.fatalMsg, "width 81 > cols 80") {
		t.Errorf("expected fatal 'width 81 > cols 80'; got %q", mt.fatalMsg)
	}
}

func TestCheckRowWidth_OverwidthCJK(t *testing.T) {
	t.Parallel()
	mt := &mockT{}
	grid := []string{strings.Repeat("中", 41)} // 41 × 2 = 82 > 80
	CheckRowWidth(mt, grid, 80)
	if !mt.fatalCalled || !strings.Contains(mt.fatalMsg, "width 82") {
		t.Errorf("expected fatal 'width 82'; got %q", mt.fatalMsg)
	}
}

func TestCheckRowWidth_ZeroCols(t *testing.T) {
	t.Parallel()
	mt := &mockT{}
	CheckRowWidth(mt, []string{"a"}, 0)
	if !mt.fatalCalled {
		t.Error("expected fatal for cols=0")
	}
}

// --- CheckANSI happy / state-based ----------------------------------

func TestCheckANSI_PlainText(t *testing.T) {
	t.Parallel()
	CheckANSI(t, []byte("hello world"))
}

func TestCheckANSI_UniversalReset(t *testing.T) {
	t.Parallel()
	// "ESC [ 0 m" universal reset.
	CheckANSI(t, []byte("\x1b[31mred\x1b[0m back to default"))
}

func TestCheckANSI_OverwriteWithoutClose(t *testing.T) {
	t.Parallel()
	// fg 31 then fg 32: state.fg is set both times; final reset clears.
	CheckANSI(t, []byte("\x1b[31m red \x1b[32m green \x1b[0m"))
}

func TestCheckANSI_TargetedReset(t *testing.T) {
	t.Parallel()
	// 1m bold + 22m bold-off (no universal reset needed).
	CheckANSI(t, []byte("\x1b[1m bold \x1b[22m no-bold"))
}

func TestCheckANSI_RepeatedSet(t *testing.T) {
	t.Parallel()
	// Idempotent: setting the same attribute repeatedly is valid.
	CheckANSI(t, []byte("\x1b[31m red \x1b[31m still red \x1b[0m"))
}

// --- CheckANSI failure modes ----------------------------------------

func TestCheckANSI_ResidualFg(t *testing.T) {
	t.Parallel()
	mt := &mockT{}
	CheckANSI(mt, []byte("\x1b[31m hello no reset"))
	if !mt.fatalCalled || !strings.Contains(mt.fatalMsg, "residual active SGR") {
		t.Errorf("expected residual SGR fatal; got %q", mt.fatalMsg)
	}
}

func TestCheckANSI_ResidualBold(t *testing.T) {
	t.Parallel()
	mt := &mockT{}
	CheckANSI(mt, []byte("\x1b[1m bold no reset"))
	if !mt.fatalCalled {
		t.Error("expected fatal for residual bold")
	}
}

func TestCheckANSI_TruncatedEscape(t *testing.T) {
	t.Parallel()
	mt := &mockT{}
	CheckANSI(mt, []byte("\x1b")) // ESC alone, no [
	if !mt.fatalCalled || !strings.Contains(mt.fatalMsg, "truncated ESC") {
		t.Errorf("expected 'truncated ESC'; got %q", mt.fatalMsg)
	}
}

func TestCheckANSI_TruncatedCSI(t *testing.T) {
	t.Parallel()
	mt := &mockT{}
	CheckANSI(mt, []byte("\x1b[31")) // CSI started, no terminator
	if !mt.fatalCalled || !strings.Contains(mt.fatalMsg, "truncated CSI") {
		t.Errorf("expected 'truncated CSI'; got %q", mt.fatalMsg)
	}
}

func TestCheckANSI_MalformedParamByte(t *testing.T) {
	t.Parallel()
	mt := &mockT{}
	// 0x07 (BEL) is outside param/intermediate/final byte ranges — malformed.
	CheckANSI(mt, []byte("\x1b[1\x07m"))
	if !mt.fatalCalled || !strings.Contains(mt.fatalMsg, "malformed CSI param byte") {
		t.Errorf("expected 'malformed CSI param byte'; got %q", mt.fatalMsg)
	}
}

func TestCheckANSI_ParamOverflow(t *testing.T) {
	t.Parallel()
	mt := &mockT{}
	CheckANSI(mt, []byte("\x1b[99999999m"))
	if !mt.fatalCalled {
		t.Error("expected fatal for param overflow")
	}
}

// --- CheckANSI extended color (256-color + 24-bit) -----------------

func TestCheckANSI_256ColorFg(t *testing.T) {
	t.Parallel()
	// 38;5;196 → bright red (256-color index 196).
	CheckANSI(t, []byte("\x1b[38;5;196m red \x1b[0m"))
}

func TestCheckANSI_256ColorBg(t *testing.T) {
	t.Parallel()
	CheckANSI(t, []byte("\x1b[48;5;240m gray \x1b[0m"))
}

func TestCheckANSI_24BitFg(t *testing.T) {
	t.Parallel()
	// 38;2;R;G;B → 24-bit color.
	CheckANSI(t, []byte("\x1b[38;2;255;100;50m custom \x1b[0m"))
}

func TestCheckANSI_24BitBg(t *testing.T) {
	t.Parallel()
	CheckANSI(t, []byte("\x1b[48;2;10;20;30m custom-bg \x1b[0m"))
}

func TestCheckANSI_256ColorMissingIndex(t *testing.T) {
	t.Parallel()
	mt := &mockT{}
	CheckANSI(mt, []byte("\x1b[38;5m"))
	if !mt.fatalCalled || !strings.Contains(mt.fatalMsg, "missing color index") {
		t.Errorf("expected 'missing color index' fatal; got %q", mt.fatalMsg)
	}
}

func TestCheckANSI_256ColorOutOfRange(t *testing.T) {
	t.Parallel()
	mt := &mockT{}
	CheckANSI(mt, []byte("\x1b[38;5;300m"))
	if !mt.fatalCalled || !strings.Contains(mt.fatalMsg, "out of [0,255]") {
		t.Errorf("expected 'out of [0,255]' fatal; got %q", mt.fatalMsg)
	}
}

func TestCheckANSI_24BitMissingRGB(t *testing.T) {
	t.Parallel()
	mt := &mockT{}
	CheckANSI(mt, []byte("\x1b[38;2;255;100m")) // missing B
	if !mt.fatalCalled || !strings.Contains(mt.fatalMsg, "missing R;G;B") {
		t.Errorf("expected 'missing R;G;B'; got %q", mt.fatalMsg)
	}
}

func TestCheckANSI_24BitRGBOutOfRange(t *testing.T) {
	t.Parallel()
	mt := &mockT{}
	CheckANSI(mt, []byte("\x1b[38;2;256;0;0m"))
	if !mt.fatalCalled || !strings.Contains(mt.fatalMsg, "RGB out of") {
		t.Errorf("expected 'RGB out of' fatal; got %q", mt.fatalMsg)
	}
}

func TestCheckANSI_UnknownExtendedMode(t *testing.T) {
	t.Parallel()
	mt := &mockT{}
	CheckANSI(mt, []byte("\x1b[38;9;1m")) // mode 9 unknown
	if !mt.fatalCalled || !strings.Contains(mt.fatalMsg, "unknown mode") {
		t.Errorf("expected 'unknown mode'; got %q", mt.fatalMsg)
	}
}

func TestCheckANSI_StandardBg(t *testing.T) {
	t.Parallel()
	CheckANSI(t, []byte("\x1b[42m bg green \x1b[49m back to default"))
}

func TestCheckANSI_BrightFg(t *testing.T) {
	t.Parallel()
	CheckANSI(t, []byte("\x1b[91m bright red \x1b[0m"))
}

func TestCheckANSI_BrightBg(t *testing.T) {
	t.Parallel()
	CheckANSI(t, []byte("\x1b[101m bright bg \x1b[0m"))
}

func TestCheckANSI_ItalicUnderlineReverse(t *testing.T) {
	t.Parallel()
	CheckANSI(t, []byte("\x1b[3;4;7m all \x1b[23;24;27m off"))
}

func TestCheckANSI_UnknownSGRTolerated(t *testing.T) {
	t.Parallel()
	// Unknown SGR code 99 — tolerated (no state change, no fatal).
	CheckANSI(t, []byte("\x1b[99m unknown but valid \x1b[0m"))
}

func TestCheckANSI_NonCSIEscapeSkipped(t *testing.T) {
	t.Parallel()
	// ESC P ... (DCS) — non-CSI; we just advance 2 bytes.
	CheckANSI(t, []byte("\x1bPDCS not-csi"))
}

func TestCheckANSI_24BitMissingMode(t *testing.T) {
	t.Parallel()
	mt := &mockT{}
	CheckANSI(mt, []byte("\x1b[38m")) // 38 alone, no ;5/;2 follow-up
	if !mt.fatalCalled || !strings.Contains(mt.fatalMsg, "missing mode subparam") {
		t.Errorf("expected 'missing mode subparam'; got %q", mt.fatalMsg)
	}
}

func TestCheckANSI_ParamMalformedString(t *testing.T) {
	t.Parallel()
	mt := &mockT{}
	// "3;a;1" — middle param 'a' (CSI param bytes 0x30-0x3F include
	// digits + : ; < = > ? but not letters). After our byte filter
	// blocks 'a' at end-scan, so this test path requires a special
	// crafted input: use 0x3F (?) which is a valid param byte but
	// strconv.Atoi rejects.
	CheckANSI(mt, []byte("\x1b[3;?;1m"))
	if !mt.fatalCalled || !strings.Contains(mt.fatalMsg, "malformed CSI param") {
		t.Errorf("expected 'malformed CSI param'; got %q", mt.fatalMsg)
	}
}

// --- CheckANSI non-SGR CSI tolerance -------------------------------

func TestCheckANSI_CursorCSIIgnored(t *testing.T) {
	t.Parallel()
	// CSI A is cursor-up; we tolerate without inspection.
	CheckANSI(t, []byte("\x1b[5A move up 5"))
}

func TestCheckANSI_EmptySGRReset(t *testing.T) {
	t.Parallel()
	// "ESC [ m" with no params is treated as reset.
	CheckANSI(t, []byte("\x1b[31m red \x1b[m back"))
}

// --- mockT for negative paths --------------------------------------

type mockT struct {
	testing.TB
	fatalCalled bool
	fatalMsg    string
}

func (m *mockT) Helper() {}

func (m *mockT) Fatalf(format string, args ...any) {
	m.fatalCalled = true
	m.fatalMsg = fmt.Sprintf(format, args...)
}
