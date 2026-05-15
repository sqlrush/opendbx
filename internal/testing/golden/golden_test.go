// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package golden

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- Update() flag behavior -----------------------------------------

func TestUpdate_RegisteredFalseByDefault(t *testing.T) {
	t.Parallel()
	// At test runtime, -update is registered (by init); test binary did
	// not pass -update, so value is false.
	if Update() {
		t.Error("Update() must be false when -update not provided")
	}
}

func TestUpdate_FlagIsRegistered(t *testing.T) {
	t.Parallel()
	if flag.Lookup("update") == nil {
		t.Fatal("-update flag must be registered by init")
	}
}

// --- Compare: missing golden + read error --------------------------

func TestCompare_MissingGolden(t *testing.T) {
	t.Parallel()
	mt := &mockT{}
	Compare(mt, "missing-fixture-xyz", []byte("anything"))
	if !mt.fatalCalled || !strings.Contains(mt.fatalMsg, "run with -update to create") {
		t.Errorf("expected fatal 'run with -update to create'; got %q", mt.fatalMsg)
	}
}

// --- Compare: match + mismatch via temp fixture ---------------------

func TestCompare_Match(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	want := []byte("expected payload")
	path := filepath.Join(dir, "fixture.golden")
	if err := os.WriteFile(path, want, 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	// Use compareAt directly with explicit path so we don't depend on
	// t.Name() / testdata directory resolution.
	mt := &mockT{}
	compareAt(mt, path, want)
	if mt.fatalCalled || mt.errorfCalled {
		t.Errorf("expected no fail; got fatal=%v errorf=%v msg=%q",
			mt.fatalCalled, mt.errorfCalled, firstMsg(mt))
	}
}

func TestCompare_Mismatch(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	want := []byte("expected payload\nline two\nline three\n")
	path := filepath.Join(dir, "fixture.golden")
	if err := os.WriteFile(path, want, 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	got := []byte("expected payload\nline TWO\nline three\n")
	mt := &mockT{}
	compareAt(mt, path, got)
	if !mt.errorfCalled {
		t.Fatal("expected errorf (mismatch)")
	}
	if !strings.Contains(mt.errorfMsg, "first difference at byte") {
		t.Errorf("expected 'first difference' in msg; got %q", mt.errorfMsg)
	}
}

// --- mismatchSummary edge cases ------------------------------------

func TestMismatchSummary_Empty(t *testing.T) {
	t.Parallel()
	got := mismatchSummary([]byte("abc"), []byte("abc"), 200)
	if got != "(no diff)" {
		t.Errorf("equal bytes should report '(no diff)'; got %q", got)
	}
}

func TestMismatchSummary_DifferentLength(t *testing.T) {
	t.Parallel()
	got := mismatchSummary([]byte("abc"), []byte("ab"), 200)
	if !strings.Contains(got, "want 3 bytes, got 2 bytes") {
		t.Errorf("expected byte counts; got %q", got)
	}
}

func TestMismatchSummary_LineNumber(t *testing.T) {
	t.Parallel()
	a := []byte("a\nb\nc\nd")
	b := []byte("a\nb\nX\nd")
	got := mismatchSummary(a, b, 200)
	if !strings.Contains(got, "line 3") {
		t.Errorf("expected line 3 reference; got %q", got)
	}
}

// --- Binary safety --------------------------------------------------

func TestCompare_BinaryRoundTrip(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	want := []byte{0x00, 0xFF, 0xC3, 0xA9, 0x01, 0x7F}
	path := filepath.Join(dir, "blob.golden")
	if err := os.WriteFile(path, want, 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	mt := &mockT{}
	compareAt(mt, path, want)
	if mt.fatalCalled || mt.errorfCalled {
		t.Errorf("binary should match; got fatal=%v errorf=%v",
			mt.fatalCalled, mt.errorfCalled)
	}
}

// --- CompareString convenience -------------------------------------

// --- CompareString / CompareFile entry points ---------------------

func TestCompareString_Match_PublicEntry(t *testing.T) {
	// Use real Compare/CompareString to cover the public path. We supply
	// matching golden via -update write then immediate match read.
	prev := updateOracle
	updateOracle = func() bool { return true }
	defer func() { updateOracle = prev }()

	CompareString(t, "compare_string_match", "hi")

	updateOracle = func() bool { return false }
	CompareString(t, "compare_string_match", "hi")
	t.Cleanup(func() {
		_ = os.RemoveAll(filepath.Join("testdata", "golden", t.Name()))
	})
}

func TestCompareFile_Match(t *testing.T) {
	// Write fixture relative to this test file's dir then use public
	// CompareFile so the function gets coverage.
	dir := filepath.Join("testdata", t.Name())
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(filepath.Join("testdata", t.Name())) })
	rel := filepath.Join("testdata", t.Name(), "rel-fixture.txt")
	if err := os.WriteFile(rel, []byte("hi"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	CompareFile(t, rel, []byte("hi"))
}

func TestCompareFile_Mismatch(t *testing.T) {
	dir := filepath.Join("testdata", t.Name())
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(filepath.Join("testdata", t.Name())) })
	rel := filepath.Join("testdata", t.Name(), "fixture.txt")
	if err := os.WriteFile(rel, []byte("want"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	mt := &mockT{}
	// Compute absolute via the same callerDir logic CompareFile would use
	// by directly calling compareAt with the prepared abs path.
	abs, _ := filepath.Abs(rel)
	compareAt(mt, abs, []byte("different"))
	if !mt.errorfCalled {
		t.Fatal("expected mismatch errorf")
	}
}

// --- Update branches -----------------------------------------------

func TestUpdate_NonGetterValue(t *testing.T) {
	// Replace -update flag value temporarily with a non-Getter value
	// (deliberately broken) and assert Update() returns false.
	// NOT t.Parallel — mutates global flag state.
	prev := flag.Lookup("update")
	if prev == nil {
		t.Fatal("setup: no update flag")
	}
	// Save and restore real flag.
	prevVal := prev.Value
	t.Cleanup(func() {
		f := flag.Lookup("update")
		if f != nil {
			f.Value = prevVal
		}
	})
	// Swap to non-Getter Value.
	prev.Value = nonGetterValue{}
	if Update() {
		t.Error("Update() should be false for non-Getter Value")
	}
}

type nonGetterValue struct{}

func (nonGetterValue) String() string   { return "non-getter" }
func (nonGetterValue) Set(string) error { return nil }

// nonBoolGetter implements flag.Getter but Get() returns non-bool — Update should panic.
type nonBoolGetter struct{}

func (nonBoolGetter) String() string   { return "0" }
func (nonBoolGetter) Set(string) error { return nil }
func (nonBoolGetter) Get() any         { return 42 } // int, not bool

func TestUpdate_PanicsOnNonBool(t *testing.T) {
	// NOT t.Parallel — mutates global flag.
	prev := flag.Lookup("update").Value
	flag.Lookup("update").Value = nonBoolGetter{}
	t.Cleanup(func() { flag.Lookup("update").Value = prev })

	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for non-bool -update")
		}
	}()
	Update()
}

// Write path: WriteFile fails (read-only dir).
func TestCompareAt_WriteFails(t *testing.T) {
	prev := updateOracle
	updateOracle = func() bool { return true }
	t.Cleanup(func() { updateOracle = prev })

	dir := t.TempDir()
	// Make dir read-only after creation.
	subdir := filepath.Join(dir, "ro")
	if err := os.MkdirAll(subdir, 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := os.Chmod(subdir, 0o500); err != nil { // r-x: cannot write
		t.Fatalf("chmod ro: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(subdir, 0o755) })
	path := filepath.Join(subdir, "x.golden")

	mt := &mockT{}
	compareAt(mt, path, []byte("payload"))
	if !mt.fatalCalled || !strings.Contains(mt.fatalMsg, "write") {
		t.Errorf("expected write fatal; got %q", mt.fatalMsg)
	}
}

// --- callerDir fallback path --------------------------------------

// (Hard to trigger callerDir fallback synthetically; the loop walking
// caller frames will always find a non-golden frame in normal go test
// invocation. Coverage of the fallback line is acceptable to omit.)

// --- existing tests below -----------------------------------------

func TestCompareString_Mismatch(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "s.golden")
	if err := os.WriteFile(path, []byte("expected"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	mt := &mockT{}
	compareAt(mt, path, []byte("different"))
	if !mt.errorfCalled {
		t.Fatal("expected errorf on mismatch")
	}
}

// --- helper utilities under test -----------------------------------

func TestLineAt_PastEnd(t *testing.T) {
	t.Parallel()
	if got := lineAt([]byte("abc"), 100); got != "" {
		t.Errorf("got %q, want empty for past-end pos", got)
	}
}

func TestLineNumber_Beyond(t *testing.T) {
	t.Parallel()
	// pos > len treated as scanning whole buffer.
	if got := lineNumber([]byte("a\nb"), 100); got != 2 {
		t.Errorf("got %d, want 2", got)
	}
}

func TestTruncate_Short(t *testing.T) {
	t.Parallel()
	if got := truncate("abc", 10); got != "abc" {
		t.Errorf("got %q, want %q", got, "abc")
	}
}

func TestTruncate_Long(t *testing.T) {
	t.Parallel()
	long := strings.Repeat("x", 250)
	got := truncate(long, 200)
	if len(got) != 203 || !strings.HasSuffix(got, "...") {
		t.Errorf("got len %d / suffix %v", len(got), strings.HasSuffix(got, "..."))
	}
}

func TestTail_Short(t *testing.T) {
	t.Parallel()
	if got := tail([]byte("abc"), 10); string(got) != "abc" {
		t.Errorf("got %q, want %q", got, "abc")
	}
}

func TestTail_Truncates(t *testing.T) {
	t.Parallel()
	long := []byte(strings.Repeat("x", 300))
	got := tail(long, 100)
	if len(got) != 100 {
		t.Errorf("got len %d, want 100", len(got))
	}
}

// --- Update write path (covered separately to avoid global -update mutation) -

func TestUpdate_WritePath(t *testing.T) {
	// NOT t.Parallel — overrides updateOracle (package-level test seam).
	// Swap oracle to force "update=true" without mutating real flag.
	prev := updateOracle
	updateOracle = func() bool { return true }
	t.Cleanup(func() { updateOracle = prev })

	dir := t.TempDir()
	path := filepath.Join(dir, "subdir", "deep.golden")
	got := []byte("new fixture content")
	mt := &mockT{}
	compareAt(mt, path, got)
	if mt.fatalCalled {
		t.Fatalf("write path should not fail: %q", mt.fatalMsg)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(data) != "new fixture content" {
		t.Errorf("got %q, want 'new fixture content'", data)
	}
}

func TestUpdate_WritePath_MkdirFails(t *testing.T) {
	// NOT t.Parallel for same reason as above.
	prev := updateOracle
	updateOracle = func() bool { return true }
	t.Cleanup(func() { updateOracle = prev })

	// Use a path under a regular file — MkdirAll will fail with ENOTDIR.
	dir := t.TempDir()
	regularFile := filepath.Join(dir, "blocker")
	if err := os.WriteFile(regularFile, []byte("x"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	bad := filepath.Join(regularFile, "subdir", "x.golden") // /blocker/subdir/...
	mt := &mockT{}
	compareAt(mt, bad, []byte("anything"))
	if !mt.fatalCalled || !strings.Contains(mt.fatalMsg, "mkdir") {
		t.Errorf("expected mkdir fatal; got %q", mt.fatalMsg)
	}
}

// --- goldenPath uses caller test name ------------------------------

func TestGoldenPath_UsesTestName(t *testing.T) {
	t.Parallel()
	path := goldenPath(t, "")
	if !strings.Contains(path, "TestGoldenPath_UsesTestName") {
		t.Errorf("expected test name in path %q", path)
	}
	if !strings.HasSuffix(path, ".golden") {
		t.Errorf("expected .golden suffix; got %q", path)
	}
}

func TestGoldenPath_WithSubname(t *testing.T) {
	t.Parallel()
	path := goldenPath(t, "sub")
	if !strings.HasSuffix(path, filepath.Join("TestGoldenPath_WithSubname", "sub.golden")) {
		t.Errorf("unexpected subname path: %q", path)
	}
}

// --- mockT helper ---------------------------------------------------

type mockT struct {
	testing.TB
	fatalCalled  bool
	fatalMsg     string
	errorfCalled bool
	errorfMsg    string
}

func (m *mockT) Helper() {}

func (m *mockT) Name() string { return "MockTest" }

func (m *mockT) Fatalf(format string, args ...any) {
	m.fatalCalled = true
	m.fatalMsg = fmt.Sprintf(format, args...)
}

func (m *mockT) Errorf(format string, args ...any) {
	m.errorfCalled = true
	m.errorfMsg = fmt.Sprintf(format, args...)
}

func firstMsg(m *mockT) string {
	if m.fatalCalled {
		return m.fatalMsg
	}
	return m.errorfMsg
}
