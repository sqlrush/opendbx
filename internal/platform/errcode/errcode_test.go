// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package errcode

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
)

// register a TEST.* code for use inside this test file. Always called
// inside the test that needs it so the registry stays clean across t.Run
// boundaries.
func registerTest(t *testing.T, code, msg, hint string) Sentinel {
	t.Helper()
	s := Register(code, msg, hint)
	t.Cleanup(func() { unregisterForTesting(code) })
	return s
}

func TestErrorInterfaceSatisfied(t *testing.T) {
	t.Parallel()
	// Compile-time check that *structuredErr satisfies Error.
	var _ Error = (*structuredErr)(nil)
}

func TestStructuredErrError(t *testing.T) {
	t.Parallel()
	e := &structuredErr{code: "TEST.X", message: "boom", hint: "do something"}
	if got := e.Error(); got != "[TEST.X] boom" {
		t.Errorf("Error() = %q, want [TEST.X] boom", got)
	}

	wrapped := &structuredErr{code: "TEST.X", message: "boom", wrapped: errors.New("root")}
	if got := wrapped.Error(); !strings.HasPrefix(got, "[TEST.X] boom: root") {
		t.Errorf("wrapped Error() = %q, want [TEST.X] boom: root prefix", got)
	}
}

func TestStructuredErrNilSafe(t *testing.T) {
	t.Parallel()
	var e *structuredErr
	if e.Error() != "" || e.Code() != "" || e.Message() != "" || e.Hint() != "" || e.Unwrap() != nil {
		t.Error("nil structuredErr should return zero values without panicking")
	}
	if e.Is(errors.New("any")) {
		t.Error("nil structuredErr.Is should return false")
	}
}

func TestNewRequiresRegisteredCode(t *testing.T) {
	t.Parallel()
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("New with unregistered code should panic")
		}
	}()
	_ = New("TEST.UNREGISTERED_NEW", "msg", "hint")
}

func TestNewInheritsRegistryDefaults(t *testing.T) {
	t.Parallel()
	_ = registerTest(t, "TEST.DEFAULTS", "default msg", "default hint")
	e := New("TEST.DEFAULTS", "", "")
	if e.Message() != "default msg" || e.Hint() != "default hint" {
		t.Errorf("New empty overrides did not inherit defaults: %+v", e)
	}
	// Explicit overrides win.
	e2 := New("TEST.DEFAULTS", "override msg", "override hint")
	if e2.Message() != "override msg" || e2.Hint() != "override hint" {
		t.Errorf("New explicit overrides ignored: %+v", e2)
	}
}

func TestNewf(t *testing.T) {
	t.Parallel()
	_ = registerTest(t, "TEST.FORMAT", "x", "the hint")
	e := Newf("TEST.FORMAT", "value=%d", 42)
	if e.Message() != "value=42" {
		t.Errorf("Newf Message() = %q, want value=42", e.Message())
	}
	if e.Hint() != "the hint" {
		t.Errorf("Newf Hint() = %q, want 'the hint' (inherited)", e.Hint())
	}
}

func TestWrapPreservesChain(t *testing.T) {
	t.Parallel()
	root := errors.New("root cause")
	_ = registerTest(t, "TEST.WRAPCHAIN", "outer", "hint")
	wrap := Wrap("TEST.WRAPCHAIN", root, "outer msg", "")
	if !errors.Is(wrap, root) {
		t.Error("errors.Is(wrap, root) should be true via Unwrap chain")
	}
	if !errors.Is(wrap.Unwrap(), root) {
		t.Errorf("Unwrap = %v, want root", wrap.Unwrap())
	}
}

func TestErrorsIsSentinelMatch(t *testing.T) {
	t.Parallel()
	sentinel := registerTest(t, "TEST.SENTINEL", "msg", "hint")
	err := New("TEST.SENTINEL", "runtime", "rt-hint")
	if !errors.Is(err, sentinel) {
		t.Error("errors.Is(err, sentinel) should match when both share Code")
	}
	// Symmetry: errors.Is(sentinel, err) also true (claude CRIT-1 contract).
	if !errors.Is(sentinel, err) {
		t.Error("errors.Is(sentinel, err) should match (symmetry)")
	}
}

func TestErrorsIsDifferentCodeNoMatch(t *testing.T) {
	t.Parallel()
	a := registerTest(t, "TEST.AAA", "", "")
	b := registerTest(t, "TEST.BBB", "", "")
	if errors.Is(a, b) {
		t.Error("errors.Is should not match different codes")
	}
}

func TestErrorsAsTypeAssert(t *testing.T) {
	t.Parallel()
	_ = registerTest(t, "TEST.ASTYPE", "msg", "hint")
	err := Newf("TEST.ASTYPE", "x=%s", "y")
	var ec Error
	if !As(err, &ec) {
		t.Fatal("errcode.As should succeed for structuredErr")
	}
	if ec.Code() != "TEST.ASTYPE" {
		t.Errorf("ec.Code() = %q, want TEST.ASTYPE", ec.Code())
	}
}

func TestErrorsAsWalksWrappedChain(t *testing.T) {
	t.Parallel()
	_ = registerTest(t, "TEST.DEEPWRAP", "inner", "h")
	deep := Newf("TEST.DEEPWRAP", "deep")
	mid := fmt.Errorf("middle: %w", deep)
	outer := fmt.Errorf("outer: %w", mid)

	var ec Error
	if !As(outer, &ec) {
		t.Fatal("As should walk Unwrap chain to reach structuredErr")
	}
	if ec.Code() != "TEST.DEEPWRAP" {
		t.Errorf("ec.Code() = %q, want TEST.DEEPWRAP", ec.Code())
	}
}

func TestErrorsAsViaErrCodeWrap(t *testing.T) {
	t.Parallel()
	_ = registerTest(t, "TEST.INNER", "inner msg", "inner hint")
	_ = registerTest(t, "TEST.OUTER", "outer msg", "outer hint")
	inner := New("TEST.INNER", "", "")
	outer := Wrap("TEST.OUTER", inner, "", "")

	// errors.Is(outer, INNER) → true via Unwrap chain.
	var ec Error
	if !As(outer, &ec) {
		t.Fatal("As should hit outer first")
	}
	if ec.Code() != "TEST.OUTER" {
		t.Errorf("As should bind to outermost errcode.Error first: got %q", ec.Code())
	}
	if !errors.Is(outer, inner) {
		t.Error("errors.Is(outer, inner) should walk chain and match")
	}
}

func TestRegisterRejectsBadNaming(t *testing.T) {
	t.Parallel()
	cases := []string{
		"lower.case",   // lowercase
		"NO_DOT",       // missing dot
		"LLM_CLIENT.X", // underscore in module — too long
		"X.Y",          // segments too short
		"A.B_C",        // module too short (1 letter only after first)
		"FOO.bar",      // lowercase segment
		"FOO.",         // empty segment
		".BAR",         // empty module
		"FOO.BAR-BAZ",  // hyphen in segment
		"",             // empty
	}
	for _, c := range cases {
		c := c
		t.Run(c, func(t *testing.T) {
			t.Parallel()
			defer func() {
				if r := recover(); r == nil {
					t.Errorf("Register(%q) should have panicked", c)
				}
			}()
			_ = Register(c, "m", "h")
		})
	}
}

func TestRegisterAcceptsValidNaming(t *testing.T) {
	t.Parallel()
	cases := []string{
		"TEST.VALID_ONE",
		"TEST.WITH_NUMBERS_42",
		"TEST.MULTI.DOT.SEG", // multi-dot allowed per spec § 1.3
		"LOGGER.WRITER_CLOSED",
		"LLM.STREAM_EMPTY_CONTENT",
		"PG.SQLSTATE_42P01",
	}
	for _, c := range cases {
		c := c
		t.Run(c, func(t *testing.T) {
			// Cannot t.Parallel + share unique codes; serial in subtest.
			_ = registerTest(t, c, "msg", "hint")
		})
	}
}

func TestRegisterDuplicateConflictPanics(t *testing.T) {
	t.Parallel()
	_ = registerTest(t, "TEST.DUPCONFLICT", "msg-A", "hint-A")
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("duplicate Register with different msg/hint should panic")
		}
	}()
	_ = Register("TEST.DUPCONFLICT", "msg-B", "hint-B")
}

func TestRegisterDuplicateIdenticalNoOp(t *testing.T) {
	t.Parallel()
	a := registerTest(t, "TEST.DUPIDENTICAL", "msg", "hint")
	// Identical re-registration must NOT panic (codex MED-2 / claude LOW-1).
	b := Register("TEST.DUPIDENTICAL", "msg", "hint")
	if a.Code() != b.Code() {
		t.Errorf("identical re-register produced mismatched Code: %q vs %q", a.Code(), b.Code())
	}
}

func TestLookup(t *testing.T) {
	t.Parallel()
	_ = registerTest(t, "TEST.LOOKUP", "m", "h")
	def, ok := Lookup("TEST.LOOKUP")
	if !ok {
		t.Fatal("Lookup hit miss for registered code")
	}
	if def.Code != "TEST.LOOKUP" || def.Message != "m" || def.Hint != "h" || def.Module != "TEST" {
		t.Errorf("Lookup returned wrong def: %+v", def)
	}
	if _, ok := Lookup("TEST.NEVERREG"); ok {
		t.Error("Lookup should miss unregistered code")
	}
}

func TestAllExcludesTestPrefix(t *testing.T) {
	t.Parallel()
	_ = registerTest(t, "TEST.HIDDEN", "m", "h")
	for _, def := range All() {
		if strings.HasPrefix(def.Code, testPrefix) {
			t.Errorf("All() leaked TEST.* code: %s", def.Code)
		}
	}
}

func TestAllSortedByCode(t *testing.T) {
	t.Parallel()
	defs := All()
	for i := 1; i < len(defs); i++ {
		if defs[i-1].Code > defs[i].Code {
			t.Errorf("All() not sorted: %q > %q", defs[i-1].Code, defs[i].Code)
		}
	}
}

func TestBuiltinCodesPresent(t *testing.T) {
	t.Parallel()
	// Builtin codes are registered at package init via builtin.go.
	wanted := []string{
		"ERRCODE.INVALID_ARGUMENT",
		"ERRCODE.NOT_FOUND",
		"ERRCODE.NOT_IMPLEMENTED",
		"ERRCODE.INTERNAL",
	}
	for _, code := range wanted {
		if _, ok := Lookup(code); !ok {
			t.Errorf("builtin code %q not registered", code)
		}
	}
}

func TestRegisterConcurrent(t *testing.T) {
	// Hammer Register from many goroutines with distinct codes — must not
	// race under -race. claude HIGH-1 + go-reviewer H-2 R2 alignment:
	// t.Cleanup MUST be registered from the test's own goroutine, not from
	// the spawned ones (testing package contract). Pre-collect codes here.
	const n = 32
	codes := make([]string, n)
	for i := 0; i < n; i++ {
		codes[i] = fmt.Sprintf("TEST.CONCURRENT_%02d", i)
	}
	for _, code := range codes {
		code := code
		t.Cleanup(func() { unregisterForTesting(code) })
	}

	var wg sync.WaitGroup
	wg.Add(n)
	for _, code := range codes {
		code := code
		go func() {
			defer wg.Done()
			_ = Register(code, "msg", "hint")
		}()
	}
	wg.Wait()
}

func TestIsHelperFunc(t *testing.T) {
	t.Parallel()
	a := registerTest(t, "TEST.ISHELPER", "m", "h")
	err := New("TEST.ISHELPER", "", "")
	if !Is(err, a) {
		t.Error("errcode.Is helper should match Code")
	}
}

func TestUnregisterForTestingRemovesFromAll(t *testing.T) {
	t.Parallel()
	code := "TEST.UNREG_REMOVES"
	_ = Register(code, "m", "h")
	if _, ok := Lookup(code); !ok {
		t.Fatal("seed Register failed")
	}
	unregisterForTesting(code)
	if _, ok := Lookup(code); ok {
		t.Error("Lookup should miss after unregisterForTesting")
	}
}

func TestModuleFromCode(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   string
		want string
	}{
		{"LOGGER.WRITER_CLOSED", "LOGGER"},
		{"LLM.STREAM.EMPTY_CONTENT", "LLM"},
		{"NODOT", "NODOT"},
		{"", ""},
	}
	for _, tc := range cases {
		if got := moduleFromCode(tc.in); got != tc.want {
			t.Errorf("moduleFromCode(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
