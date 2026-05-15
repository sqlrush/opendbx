// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package must

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sqlrush/opendbx/internal/platform/errcode"
)

// Register test-only error codes once at package init. Identical re-
// registration is a no-op (Register godoc), so this is safe even if
// errcode tests run before us.
//
//nolint:gochecknoinits // spec-0.11 D-2: test fixture errcode registration.
func init() {
	_ = errcode.Register("MUSTTEST.MATCH", "msg", "hint")
	_ = errcode.Register("MUSTTEST.NOERR_CHAIN", "msg-here", "hint-here")
	_ = errcode.Register("MUSTTEST.GOT", "msg", "hint")
	_ = errcode.Register("MUSTTEST.WANT", "msg", "hint")
}

// --- File -----------------------------------------------------------

func TestFile_Happy(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o600); err != nil {
		t.Fatalf("setup: %v", err)
	}
	got := File(t, path)
	if string(got) != "hello" {
		t.Errorf("got %q, want %q", got, "hello")
	}
}

func TestFile_NotFound(t *testing.T) {
	t.Parallel()
	mt := &mockT{}
	File(mt, "/definitely/not/a/path/xyz.txt")
	if !mt.fatalCalled || !strings.Contains(mt.fatalMsg, "must.File") {
		t.Errorf("expected fatal 'must.File'; got %q", mt.fatalMsg)
	}
}

// --- WriteTemp ------------------------------------------------------

func TestWriteTemp_RoundTrip(t *testing.T) {
	t.Parallel()
	path := WriteTemp(t, "foo.txt", "bar")
	if !strings.HasSuffix(path, "foo.txt") {
		t.Errorf("path %q must end with foo.txt", path)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(got) != "bar" {
		t.Errorf("got %q, want %q", got, "bar")
	}
}

// --- WriteFile ------------------------------------------------------

func TestWriteFile_BinarySafe(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	binary := []byte{0x00, 0xFF, 0xC3, 0xA9, 0x01}
	path := WriteFile(t, dir, "blob.bin", binary)
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if !bytesEqual(got, binary) {
		t.Errorf("binary round-trip failed: got %v want %v", got, binary)
	}
}

func TestWriteFile_FailsOnNonExistentDir(t *testing.T) {
	t.Parallel()
	mt := &mockT{}
	WriteFile(mt, "/definitely/not/a/dir/xyz", "name.txt", []byte("x"))
	if !mt.fatalCalled || !strings.Contains(mt.fatalMsg, "must.WriteFile") {
		t.Errorf("expected fatal 'must.WriteFile'; got %q", mt.fatalMsg)
	}
}

// --- JSON -----------------------------------------------------------

func TestJSON_Happy(t *testing.T) {
	t.Parallel()
	var v struct{ A int }
	JSON(t, []byte(`{"A":42}`), &v)
	if v.A != 42 {
		t.Errorf("got %d, want 42", v.A)
	}
}

func TestJSON_Malformed(t *testing.T) {
	t.Parallel()
	mt := &mockT{}
	var v struct{ A int }
	JSON(mt, []byte(`{"A":}`), &v)
	if !mt.fatalCalled || !strings.Contains(mt.fatalMsg, "must.JSON") {
		t.Errorf("expected fatal 'must.JSON'; got %q", mt.fatalMsg)
	}
}

// --- NoErr ----------------------------------------------------------

func TestNoErr_Nil(t *testing.T) {
	t.Parallel()
	NoErr(t, nil)
	// no fatal expected
}

func TestNoErr_PlainError(t *testing.T) {
	t.Parallel()
	mt := &mockT{}
	NoErr(mt, errors.New("plain"))
	if !mt.fatalCalled || !strings.Contains(mt.fatalMsg, "plain") {
		t.Errorf("expected fatal mentioning 'plain'; got %q", mt.fatalMsg)
	}
}

func TestNoErr_ErrcodeChain(t *testing.T) {
	t.Parallel()
	mt := &mockT{}
	ec := errcode.New("MUSTTEST.NOERR_CHAIN", "msg-here", "hint-here")
	NoErr(mt, ec)
	if !mt.fatalCalled {
		t.Fatal("expected fatal")
	}
	for _, want := range []string{"MUSTTEST.NOERR_CHAIN", "msg-here", "hint-here"} {
		if !strings.Contains(mt.fatalMsg, want) {
			t.Errorf("fatal msg missing %q; got %q", want, mt.fatalMsg)
		}
	}
}

// --- ErrCode --------------------------------------------------------

func TestErrCode_Match(t *testing.T) {
	t.Parallel()
	ec := errcode.New("MUSTTEST.MATCH", "msg", "hint")
	ErrCode(t, ec, "MUSTTEST.MATCH")
}

func TestErrCode_NilErr(t *testing.T) {
	t.Parallel()
	mt := &mockT{}
	ErrCode(mt, nil, "MUSTTEST.NIL")
	if !mt.fatalCalled || !strings.Contains(mt.fatalMsg, "got nil error") {
		t.Errorf("expected fatal 'got nil error'; got %q", mt.fatalMsg)
	}
}

func TestErrCode_NotErrcodeType(t *testing.T) {
	t.Parallel()
	mt := &mockT{}
	ErrCode(mt, errors.New("plain"), "MUSTTEST.WANT")
	if !mt.fatalCalled || !strings.Contains(mt.fatalMsg, "is not an errcode.Error") {
		t.Errorf("expected fatal 'is not an errcode.Error'; got %q", mt.fatalMsg)
	}
}

func TestErrCode_WrongCode(t *testing.T) {
	t.Parallel()
	mt := &mockT{}
	ec := errcode.New("MUSTTEST.GOT", "msg", "hint")
	ErrCode(mt, ec, "MUSTTEST.WANT")
	if !mt.fatalCalled || !strings.Contains(mt.fatalMsg, "MUSTTEST.GOT") || !strings.Contains(mt.fatalMsg, "MUSTTEST.WANT") {
		t.Errorf("expected fatal mentioning both codes; got %q", mt.fatalMsg)
	}
}

// --- firstN ---------------------------------------------------------

func TestFirstN_Shorter(t *testing.T) {
	t.Parallel()
	got := firstN([]byte("hi"), 80)
	if string(got) != "hi" {
		t.Errorf("got %q, want %q", got, "hi")
	}
}

func TestFirstN_Truncates(t *testing.T) {
	t.Parallel()
	got := firstN(make([]byte, 200), 80)
	if len(got) != 80 {
		t.Errorf("got len %d, want 80", len(got))
	}
}

// --- mockT helper ---------------------------------------------------

type mockT struct {
	testing.TB
	fatalCalled bool
	fatalMsg    string
	tempDirs    []string // T-13 claude MED-5: drained by CleanupDirs
}

func (m *mockT) Helper() {}

func (m *mockT) Fatalf(format string, args ...any) {
	m.fatalCalled = true
	m.fatalMsg = fmt.Sprintf(format, args...)
}

// TempDir is required because WriteTemp calls it; we route to a fresh
// os.MkdirTemp and track the path so the outer test (which created the
// mockT) can defer cleanup. T-13 claude MED-5: previously leaked temp
// dirs across negative-path runs.
func (m *mockT) TempDir() string {
	dir, err := os.MkdirTemp("", "must-mock-")
	if err != nil {
		panic("mockT.TempDir: " + err.Error())
	}
	m.tempDirs = append(m.tempDirs, dir)
	return dir
}

// CleanupDirs is called by tests after they exercise mockT to drop the
// tempdirs that the mock allocated. (Outer testing.T doesn't see them.)
func (m *mockT) CleanupDirs() {
	for _, d := range m.tempDirs {
		_ = os.RemoveAll(d)
	}
	m.tempDirs = nil
}

// bytesEqual avoids importing bytes for a single comparison.
func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i, v := range a {
		if v != b[i] {
			return false
		}
	}
	return true
}

func TestWriteFileAt_HappyAndMkdir(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "nested", "f.txt")
	got := WriteFileAt(t, path, []byte("hello"))
	if got != path {
		t.Errorf("got %q, want %q", got, path)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(body) != "hello" {
		t.Errorf("body %q, want %q", body, "hello")
	}
}

func TestWriteFileAt_FailsOnBlockingFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatalf("setup: %v", err)
	}
	// Path under a regular file → MkdirAll ENOTDIR.
	mt := &mockT{}
	WriteFileAt(mt, filepath.Join(blocker, "sub", "x.txt"), []byte("payload"))
	if !mt.fatalCalled || !strings.Contains(mt.fatalMsg, "WriteFileAt") {
		t.Errorf("expected fatal 'WriteFileAt'; got %q", mt.fatalMsg)
	}
}
