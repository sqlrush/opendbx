// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package tablerun

import (
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
)

// --- mustExtractName: failure modes ---------------------------------

func TestMustExtractName_NotStruct(t *testing.T) {
	t.Parallel()
	mt := &mockT{}
	mustExtractName(mt, 0, "not a struct")
	if !mt.fatalCalled || !strings.Contains(mt.fatalMsg, "not a struct") {
		t.Errorf("expected Fatalf with 'not a struct'; got fatal=%v msg=%q", mt.fatalCalled, mt.fatalMsg)
	}
}

func TestMustExtractName_MissingName(t *testing.T) {
	t.Parallel()
	type noname struct{ Other string }
	mt := &mockT{}
	mustExtractName(mt, 3, noname{Other: "x"})
	if !mt.fatalCalled || !strings.Contains(mt.fatalMsg, "missing required Name") {
		t.Errorf("expected fatal with 'missing required Name'; got %q", mt.fatalMsg)
	}
	if !strings.Contains(mt.fatalMsg, "case 3") {
		t.Errorf("fatal msg must mention case idx 3; got %q", mt.fatalMsg)
	}
}

func TestMustExtractName_NameWrongType(t *testing.T) {
	t.Parallel()
	type badtype struct{ Name int }
	mt := &mockT{}
	mustExtractName(mt, 0, badtype{Name: 42})
	if !mt.fatalCalled || !strings.Contains(mt.fatalMsg, "want string") {
		t.Errorf("expected fatal 'want string'; got %q", mt.fatalMsg)
	}
}

func TestMustExtractName_EmptyName(t *testing.T) {
	t.Parallel()
	type empty struct{ Name string }
	mt := &mockT{}
	mustExtractName(mt, 0, empty{Name: ""})
	if !mt.fatalCalled || !strings.Contains(mt.fatalMsg, "empty Name") {
		t.Errorf("expected fatal 'empty Name'; got %q", mt.fatalMsg)
	}
}

func TestMustExtractName_Happy(t *testing.T) {
	t.Parallel()
	type good struct{ Name string }
	got := mustExtractName(t, 0, good{Name: "abc"})
	if got != "abc" {
		t.Errorf("got %q, want %q", got, "abc")
	}
}

func TestMustExtractName_PointerStruct(t *testing.T) {
	t.Parallel()
	type good struct{ Name string }
	got := mustExtractName(t, 0, &good{Name: "via-pointer"})
	if got != "via-pointer" {
		t.Errorf("got %q, want %q", got, "via-pointer")
	}
}

// --- Run: serial execution + Name dispatch --------------------------

type basicCase struct {
	Name string
	In   int
	Want int
}

func TestRun_SerialOrder(t *testing.T) {
	cases := []basicCase{
		{Name: "first", In: 1, Want: 2},
		{Name: "second", In: 2, Want: 4},
		{Name: "third", In: 3, Want: 6},
	}
	var seen []string
	Run(t, cases, func(t *testing.T, c basicCase) {
		seen = append(seen, c.Name)
		if c.In*2 != c.Want {
			t.Errorf("%s: %d*2 != %d", c.Name, c.In, c.Want)
		}
	})
	if strings.Join(seen, ",") != "first,second,third" {
		t.Errorf("serial order broken; got %v", seen)
	}
}

// --- RunParallel: invokes t.Parallel inside subtests ----------------

func TestRunParallel_Invokes(t *testing.T) {
	cases := []basicCase{
		{Name: "a", In: 1, Want: 1},
		{Name: "b", In: 2, Want: 2},
	}
	var count atomic.Int32
	// Wrap in t.Run so parent blocks until all parallel siblings finish.
	t.Run("group", func(t *testing.T) {
		RunParallel(t, cases, func(t *testing.T, c basicCase) {
			count.Add(1)
			if c.In != c.Want {
				t.Errorf("%s: in=%d want=%d", c.Name, c.In, c.Want)
			}
		})
	})
	if got := count.Load(); got != 2 {
		t.Errorf("RunParallel saw %d cases; want 2", got)
	}
}

// --- Skippable: opt-out via SkipReason ------------------------------

type skipCase struct {
	Name string
	Skip string
}

func (c skipCase) SkipReason() string { return c.Skip }

func TestRun_SkippableNonEmpty(t *testing.T) {
	cases := []skipCase{
		{Name: "ran", Skip: ""},
		{Name: "skipped", Skip: "intentional"},
	}
	var ran []string
	Run(t, cases, func(t *testing.T, c skipCase) {
		ran = append(ran, c.Name)
	})
	if strings.Join(ran, ",") != "ran" {
		t.Errorf("expected only 'ran' to execute; got %v", ran)
	}
}

func TestRunParallel_SkippableNonEmpty(t *testing.T) {
	cases := []skipCase{
		{Name: "ran-p", Skip: ""},
		{Name: "skipped-p", Skip: "parallel skip"},
	}
	var count atomic.Int32
	t.Run("group", func(t *testing.T) {
		RunParallel(t, cases, func(t *testing.T, c skipCase) {
			count.Add(1)
		})
	})
	if got := count.Load(); got != 1 {
		t.Errorf("RunParallel ran %d cases; want 1 (other skipped)", got)
	}
}

// --- mockT: minimal *testing.T stand-in for negative-path coverage --

// mockT only stubs Helper + Fatalf since mustExtractName is what we test.
// The real *testing.T can't be used negatively because Fatalf marks the
// test failed — we want to verify the call was made without failing the
// outer test.
type mockT struct {
	testing.TB
	helperCalled bool
	fatalCalled  bool
	fatalMsg     string
}

func (m *mockT) Helper() { m.helperCalled = true }

func (m *mockT) Fatalf(format string, args ...any) {
	m.fatalCalled = true
	m.fatalMsg = fmt.Sprintf(format, args...)
}
