// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package terminal

import (
	"os"
	"strings"

	"golang.org/x/term"
)

// Capabilities is the immutable probe result. Advisory: fields are best-
// effort heuristics; spec-1.x render engine will post-init re-query tcell
// for ground truth (see spec-0.12 R-11 COLORTERM heuristic limits).
type Capabilities struct {
	Cols, Rows        int  // initial terminal size (0,0 if not a TTY)
	SupportsTrueColor bool // COLORTERM=truecolor|24bit heuristic
	LocaleUTF8        bool // LANG / LC_CTYPE / LC_ALL contains "UTF-8" / "utf8"
	StdoutIsTTY       bool // os.Stdout is a TTY
	StdinIsTTY        bool // os.Stdin is a TTY
}

// Probe returns the current terminal's capabilities. non-TTY is NOT
// a probe failure; the caller (cmd/opendbx) decides the policy by
// inspecting StdoutIsTTY + StdinIsTTY.
//
// Only returns ErrProbeFailed if the probe itself is unable to run
// (currently: never under stdlib + x/term; reserved for future
// hard-failure modes).
func Probe() (Capabilities, error) {
	caps := Capabilities{
		StdoutIsTTY: term.IsTerminal(int(os.Stdout.Fd())),
		StdinIsTTY:  term.IsTerminal(int(os.Stdin.Fd())),
	}
	if caps.StdoutIsTTY {
		// GetSize is best-effort; failure → leave 0,0 (advisory).
		if cols, rows, err := term.GetSize(int(os.Stdout.Fd())); err == nil {
			caps.Cols = cols
			caps.Rows = rows
		}
	}
	caps.SupportsTrueColor = colortermSupportsTrueColor()
	caps.LocaleUTF8 = localeIsUTF8()
	return caps, nil
}

// IsInteractiveTTY reports whether both stdin AND stdout are TTYs.
// Used by cmd/opendbx runInteractRoot to gate the tui.Run path
// (spec-0.12 R2 M-7: stdin-pipe + stdout-TTY mixed mode unsupported
// in Stage 0).
func IsInteractiveTTY() bool {
	return term.IsTerminal(int(os.Stdout.Fd())) &&
		term.IsTerminal(int(os.Stdin.Fd()))
}

// colortermSupportsTrueColor checks the COLORTERM env per the de-facto
// convention (set by iTerm2 / Alacritty / kitty / WezTerm / etc.).
// spec-0.12 R-11: kitty / GNOME / Terminal.app may not set it; this
// returns false in those cases (advisory only).
func colortermSupportsTrueColor() bool {
	switch strings.ToLower(os.Getenv("COLORTERM")) {
	case "truecolor", "24bit":
		return true
	}
	return false
}

// localeIsUTF8 reports whether any of LC_ALL / LC_CTYPE / LANG indicates
// a UTF-8 locale. Case-insensitive substring match for "UTF-8" or "utf8".
func localeIsUTF8() bool {
	for _, key := range []string{"LC_ALL", "LC_CTYPE", "LANG"} {
		v := strings.ToLower(os.Getenv(key))
		if v == "" {
			continue
		}
		if strings.Contains(v, "utf-8") || strings.Contains(v, "utf8") {
			return true
		}
		// LC_ALL / LC_CTYPE precedence: first set value wins (POSIX).
		return false
	}
	return false
}
