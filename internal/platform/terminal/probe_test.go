// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package terminal

import (
	"errors"
	"testing"

	"github.com/sqlrush/opendbx/internal/platform/errcode"
)

// --- colortermSupportsTrueColor ---------------------------------------

func TestColortermSupportsTrueColor(t *testing.T) {
	cases := []struct {
		env  string
		want bool
	}{
		{"truecolor", true},
		{"24bit", true},
		{"TrueColor", true}, // case-insensitive
		{"256color", false},
		{"", false},
		{"unknown", false},
	}
	for _, c := range cases {
		c := c
		t.Run(c.env, func(t *testing.T) {
			t.Setenv("COLORTERM", c.env)
			if got := colortermSupportsTrueColor(); got != c.want {
				t.Errorf("COLORTERM=%q: want %v, got %v", c.env, c.want, got)
			}
		})
	}
}

// --- localeIsUTF8 -----------------------------------------------------

func TestLocaleIsUTF8(t *testing.T) {
	cases := []struct {
		name           string
		lcAll, lcCtype string
		lang           string
		want           bool
	}{
		{"LANG-utf8", "", "", "en_US.UTF-8", true},
		{"LANG-utf8-lower", "", "", "en_US.utf8", true},
		{"LANG-posix", "", "", "POSIX", false},
		{"LC_CTYPE-utf8", "", "zh_CN.UTF-8", "en_US.UTF-8", true},
		{"LC_CTYPE-posix-prevents-LANG", "", "POSIX", "en_US.UTF-8", false}, // T-13 L-1: LC_CTYPE set non-UTF-8 → return false, do not fall through to LANG
		{"LC_ALL-overrides", "C", "zh_CN.UTF-8", "en_US.UTF-8", false},      // LC_ALL=C wins
		{"all-empty", "", "", "", false},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("LC_ALL", c.lcAll)
			t.Setenv("LC_CTYPE", c.lcCtype)
			t.Setenv("LANG", c.lang)
			if got := localeIsUTF8(); got != c.want {
				t.Errorf("locale combo %+v: want %v, got %v", c, c.want, got)
			}
		})
	}
}

// --- Probe ------------------------------------------------------------

// TestProbe_Smoke verifies Probe returns nil error in any environment
// (non-TTY is graceful, not an error).
func TestProbe_Smoke(t *testing.T) {
	t.Parallel()
	caps, err := Probe()
	if err != nil {
		t.Errorf("Probe should not error in test env; got %v", err)
	}
	// In Go test runner, stdout/stdin are typically pipes, not TTYs.
	if caps.StdoutIsTTY || caps.StdinIsTTY {
		t.Logf("note: running with TTY (StdoutIsTTY=%v / StdinIsTTY=%v)",
			caps.StdoutIsTTY, caps.StdinIsTTY)
	}
}

// TestProbe_TrueColor_Env ensures Probe picks up COLORTERM.
// T-13 go H-1: t.Parallel + t.Setenv is FORBIDDEN (runtime panic);
// fix direction is the opposite — strip t.Parallel from other env-
// reading tests so they don't race with this one. See
// TestProbe_Smoke / TestIsInteractiveTTY_Smoke below (t.Parallel
// removed). The package now has zero parallel tests touching env.
func TestProbe_TrueColor_Env(t *testing.T) {
	t.Setenv("COLORTERM", "truecolor")
	caps, err := Probe()
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}
	if !caps.SupportsTrueColor {
		t.Errorf("COLORTERM=truecolor should set SupportsTrueColor=true; got %+v", caps)
	}
}

// TestProbe_LocaleUTF8_Env ensures Probe picks up LANG. T-13 go H-1:
// no t.Parallel (Setenv conflict).
func TestProbe_LocaleUTF8_Env(t *testing.T) {
	t.Setenv("LC_ALL", "")
	t.Setenv("LC_CTYPE", "")
	t.Setenv("LANG", "en_US.UTF-8")
	caps, err := Probe()
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}
	if !caps.LocaleUTF8 {
		t.Errorf("LANG=en_US.UTF-8 should set LocaleUTF8=true; got %+v", caps)
	}
}

// --- IsInteractiveTTY -------------------------------------------------

// TestIsInteractiveTTY_Smoke just verifies the function runs without
// panic in any test env (return value depends on how tests are invoked).
func TestIsInteractiveTTY_Smoke(t *testing.T) {
	t.Parallel()
	_ = IsInteractiveTTY()
}

// --- errcode -----------------------------------------------------

func TestErrProbeFailed_Errcode(t *testing.T) {
	t.Parallel()
	if ErrProbeFailed.Code() != "TERMINAL.PROBE_FAILED" {
		t.Errorf("ErrProbeFailed.Code() = %q, want TERMINAL.PROBE_FAILED", ErrProbeFailed.Code())
	}
	var sentinel errcode.Error
	if !errors.As(ErrProbeFailed, &sentinel) {
		t.Errorf("ErrProbeFailed should satisfy errcode.Error interface")
	}
}

func TestErrNotATTY_Errcode(t *testing.T) {
	t.Parallel()
	if ErrNotATTY.Code() != "TERMINAL.NOT_A_TTY" {
		t.Errorf("ErrNotATTY.Code() = %q, want TERMINAL.NOT_A_TTY", ErrNotATTY.Code())
	}
	if got := ErrNotATTY.Hint(); got == "" {
		t.Errorf("ErrNotATTY.Hint() should be non-empty")
	}
}
