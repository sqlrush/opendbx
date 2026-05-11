// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package logger

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestLevelString(t *testing.T) {
	t.Parallel()
	cases := []struct {
		level Level
		want  string
	}{
		{LevelVerbose, "verbose"},
		{LevelDebug, "debug"},
		{LevelInfo, "info"},
		{LevelWarn, "warn"},
		{LevelError, "error"},
		{Level(99), "unknown"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.want, func(t *testing.T) {
			t.Parallel()
			if got := tc.level.String(); got != tc.want {
				t.Errorf("Level(%d).String() = %q, want %q", tc.level, got, tc.want)
			}
		})
	}
}

func TestLevelOrdering(t *testing.T) {
	t.Parallel()
	// Order is significant: lower value = higher verbosity. CC parity check.
	if !(LevelVerbose < LevelDebug && LevelDebug < LevelInfo &&
		LevelInfo < LevelWarn && LevelWarn < LevelError) {
		t.Fatalf("level ordering broken: verbose=%d debug=%d info=%d warn=%d error=%d",
			LevelVerbose, LevelDebug, LevelInfo, LevelWarn, LevelError)
	}
}

func TestParseLevel(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in      string
		want    Level
		wantErr bool
	}{
		{"verbose", LevelVerbose, false},
		{"DEBUG", LevelDebug, false},
		{"  Info  ", LevelInfo, false},
		{"WARN", LevelWarn, false},
		{"error", LevelError, false},
		{"trace", LevelDebug, true},
		{"", LevelDebug, true},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.in, func(t *testing.T) {
			t.Parallel()
			got, err := ParseLevel(tc.in)
			if tc.wantErr && !errors.Is(err, ErrInvalidLevel) {
				t.Fatalf("ParseLevel(%q) err = %v, want ErrInvalidLevel", tc.in, err)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("ParseLevel(%q) err = %v, want nil", tc.in, err)
			}
			if got != tc.want {
				t.Errorf("ParseLevel(%q) = %d, want %d", tc.in, got, tc.want)
			}
		})
	}
}

// TestLBeforeInitReturnsNoop — L() must be safe to call before Init.
//
// Pre-bootstrap callers (flag parsing, version printing) reference logger.L()
// and emit events. Returning a no-op keeps them panic-free, matching CC.
func TestLBeforeInitReturnsNoop(t *testing.T) {
	resetForTesting(t)
	got := L()
	if _, ok := got.(noopLogger); !ok {
		t.Fatalf("L() before Init = %T, want noopLogger", got)
	}
	// All methods must be silent no-ops.
	got.Verbose("v")
	got.Debug("d")
	got.Info("i", Attr{Key: "k", Value: "v"})
	got.Warn("w")
	got.Error("e")
	_ = got.WithModule("m")
	_ = got.WithAttrs(Attr{Key: "x", Value: 1})
	_ = got.WithContext(context.Background())
}

// TestInitIdempotent — Init must be idempotent via sync.Once.
//
// Second + third calls return nil without re-running doInit. claude LOW-2
// integration; rule 9 race-clean (validated under -race).
func TestInitIdempotent(t *testing.T) {
	resetForTesting(t)
	if err := Init(InitInput{MinLevel: LevelDebug, SessionID: "test-1"}); err != nil {
		t.Fatalf("first Init err = %v", err)
	}
	if err := Init(InitInput{MinLevel: LevelError, SessionID: "test-2"}); err != nil {
		t.Fatalf("second Init err = %v, want nil (idempotent)", err)
	}
	// First-call values should win — second call's MinLevel/SessionID ignored.
	impl := current.Load()
	if impl == nil {
		t.Fatal("current logger nil after Init")
	}
	if impl.minLevel != LevelDebug {
		t.Errorf("minLevel = %v, want LevelDebug (first-call wins)", impl.minLevel)
	}
	if impl.sessionID != "test-1" {
		t.Errorf("sessionID = %q, want test-1 (first-call wins)", impl.sessionID)
	}
}

// TestInitConcurrent — concurrent Init calls must not race.
//
// rule 9 hard requirement. sync.Once guarantees exactly-once doInit.
func TestInitConcurrent(t *testing.T) {
	resetForTesting(t)
	const goroutines = 32
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			_ = Init(InitInput{MinLevel: LevelDebug})
		}()
	}
	wg.Wait()
	if !initDone.Load() {
		t.Fatal("initDone not set after concurrent Init")
	}
	if current.Load() == nil {
		t.Fatal("current logger nil after concurrent Init")
	}
}

// TestEnableDebugLogging — runtime toggle MUST NOT be memoised; flips are
// observed immediately by subsequent isDebugRuntimeEnabled calls.
//
// claude HIGH-2 contract.
func TestEnableDebugLogging(t *testing.T) {
	resetForTesting(t)
	if isDebugRuntimeEnabled() {
		t.Fatal("runtime debug initially true, want false")
	}
	prev := EnableDebugLogging()
	if prev {
		t.Errorf("EnableDebugLogging first call returned %v, want false", prev)
	}
	if !isDebugRuntimeEnabled() {
		t.Fatal("isDebugRuntimeEnabled false after Enable, want true")
	}
	prev = EnableDebugLogging()
	if !prev {
		t.Errorf("EnableDebugLogging second call returned %v, want true", prev)
	}
}

// TestSetHasFormattedOutput — multi-line guard is package-level atomic.Bool.
//
// codex HIGH-1 + claude HIGH-1 contract; spec-1.12 tcell-bootstrap will
// SetHasFormattedOutput(true) on TUI start.
func TestSetHasFormattedOutput(t *testing.T) {
	resetForTesting(t)
	if isFormattedOutput() {
		t.Fatal("hasFormattedOutput initially true, want false")
	}
	SetHasFormattedOutput(true)
	if !isFormattedOutput() {
		t.Fatal("isFormattedOutput false after SetHasFormattedOutput(true)")
	}
	SetHasFormattedOutput(false)
	if isFormattedOutput() {
		t.Fatal("isFormattedOutput true after SetHasFormattedOutput(false)")
	}
}

// TestWithModuleAttrsContext — derived loggers must not mutate the parent.
func TestWithModuleAttrsContext(t *testing.T) {
	resetForTesting(t)
	if err := Init(InitInput{MinLevel: LevelDebug}); err != nil {
		t.Fatalf("Init err = %v", err)
	}
	root := L()
	a := root.WithModule("alpha")
	b := a.WithAttrs(Attr{Key: "k1", Value: "v1"})
	c := b.WithContext(context.Background())

	// a must have module "alpha"; root must NOT (immutability check).
	rootImpl := root.(*loggerImpl)
	if rootImpl.module != "" {
		t.Errorf("root.module = %q, want empty (immutability broken)", rootImpl.module)
	}
	aImpl := a.(*loggerImpl)
	if aImpl.module != "alpha" {
		t.Errorf("a.module = %q, want alpha", aImpl.module)
	}
	// b must inherit module + add attrs.
	bImpl := b.(*loggerImpl)
	if bImpl.module != "alpha" || len(bImpl.attrs) != 1 || bImpl.attrs[0].Key != "k1" {
		t.Errorf("b state wrong: module=%q attrs=%+v", bImpl.module, bImpl.attrs)
	}
	// c must inherit module + attrs + bind ctx.
	cImpl := c.(*loggerImpl)
	if cImpl.module != "alpha" || len(cImpl.attrs) != 1 || cImpl.ctx == nil {
		t.Errorf("c state wrong: module=%q attrs=%+v ctx=%v", cImpl.module, cImpl.attrs, cImpl.ctx)
	}

	// WithModule chain replaces (not appends).
	d := a.WithModule("beta")
	if d.(*loggerImpl).module != "beta" {
		t.Errorf("WithModule chain replace: got %q, want beta", d.(*loggerImpl).module)
	}
}

// TestWithAttrsAppend — successive WithAttrs accumulate, not replace.
func TestWithAttrsAppend(t *testing.T) {
	resetForTesting(t)
	if err := Init(InitInput{MinLevel: LevelDebug}); err != nil {
		t.Fatalf("Init err = %v", err)
	}
	root := L()
	a := root.WithAttrs(Attr{Key: "k1", Value: 1})
	b := a.WithAttrs(Attr{Key: "k2", Value: 2})
	bImpl := b.(*loggerImpl)
	if len(bImpl.attrs) != 2 {
		t.Fatalf("WithAttrs append: got %d attrs, want 2", len(bImpl.attrs))
	}
	if bImpl.attrs[0].Key != "k1" || bImpl.attrs[1].Key != "k2" {
		t.Errorf("WithAttrs order wrong: %+v", bImpl.attrs)
	}
	// Original a must still have only 1 attr (immutability).
	aImpl := a.(*loggerImpl)
	if len(aImpl.attrs) != 1 {
		t.Errorf("a.attrs len = %d after b derived, want 1 (immutability)", len(aImpl.attrs))
	}
}

// TestCloseBeforeInit — Close on an uninitialised logger returns ErrNotInitialised.
func TestCloseBeforeInit(t *testing.T) {
	resetForTesting(t)
	if err := Close(); !errors.Is(err, ErrNotInitialised) {
		t.Errorf("Close before Init = %v, want ErrNotInitialised", err)
	}
}

func TestLoggerWritesMainTextPath(t *testing.T) {
	resetForTesting(t)
	tmp := t.TempDir()
	logPath := filepath.Join(tmp, "debug.log")
	setArgvForTesting(t, "opendbx", "--debug-file", logPath)

	if err := Init(InitInput{SessionID: "main-text"}); err != nil {
		t.Fatalf("Init err = %v", err)
	}
	L().Info("api: connected")
	if err := Close(); err != nil {
		t.Fatalf("Close err = %v", err)
	}

	raw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read debug log: %v", err)
	}
	got := string(raw)
	if !strings.Contains(got, " [INFO] api: connected\n") {
		t.Fatalf("debug log missing CC text event:\n%s", got)
	}
}

func TestLoggerDefaultMinLevelIsDebug(t *testing.T) {
	resetForTesting(t)
	tmp := t.TempDir()
	logPath := filepath.Join(tmp, "debug.log")
	setArgvForTesting(t, "opendbx", "--debug-file", logPath)
	t.Setenv("OPENDBX_DEBUG_LOG_LEVEL", "")

	if err := Init(InitInput{SessionID: "default-level"}); err != nil {
		t.Fatalf("Init err = %v", err)
	}
	L().Verbose("api: verbose hidden")
	L().Debug("api: debug shown")
	if err := Close(); err != nil {
		t.Fatalf("Close err = %v", err)
	}

	raw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read debug log: %v", err)
	}
	got := string(raw)
	if strings.Contains(got, "verbose hidden") {
		t.Fatalf("default min level should filter verbose:\n%s", got)
	}
	if !strings.Contains(got, "debug shown") {
		t.Fatalf("default min level should include debug:\n%s", got)
	}
}

func TestLoggerMinLevelEnvOverride(t *testing.T) {
	resetForTesting(t)
	tmp := t.TempDir()
	logPath := filepath.Join(tmp, "debug.log")
	setArgvForTesting(t, "opendbx", "--debug-file", logPath)
	t.Setenv("OPENDBX_DEBUG_LOG_LEVEL", "info")

	if err := Init(InitInput{SessionID: "env-level"}); err != nil {
		t.Fatalf("Init err = %v", err)
	}
	L().Debug("api: debug hidden")
	L().Info("api: info shown")
	if err := Close(); err != nil {
		t.Fatalf("Close err = %v", err)
	}

	raw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read debug log: %v", err)
	}
	got := string(raw)
	if strings.Contains(got, "debug hidden") {
		t.Fatalf("OPENDBX_DEBUG_LOG_LEVEL=info should filter debug:\n%s", got)
	}
	if !strings.Contains(got, "info shown") {
		t.Fatalf("OPENDBX_DEBUG_LOG_LEVEL=info should include info:\n%s", got)
	}
}

func TestLoggerDebugFilterAppliesToModuleAndMessage(t *testing.T) {
	resetForTesting(t)
	tmp := t.TempDir()
	logPath := filepath.Join(tmp, "debug.log")
	setArgvForTesting(t, "opendbx", "--debug=api", "--debug-file", logPath)

	if err := Init(InitInput{SessionID: "filter"}); err != nil {
		t.Fatalf("Init err = %v", err)
	}
	L().Info("other: hidden")
	L().Info("api: shown")
	L().WithModule("api").Info("plain module message")
	if err := Close(); err != nil {
		t.Fatalf("Close err = %v", err)
	}

	raw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read debug log: %v", err)
	}
	got := string(raw)
	if strings.Contains(got, "other: hidden") {
		t.Fatalf("--debug=api should hide non-api messages:\n%s", got)
	}
	if !strings.Contains(got, "api: shown") || !strings.Contains(got, "plain module message") {
		t.Fatalf("--debug=api should include message and WithModule categories:\n%s", got)
	}
}

func TestLoggerRuntimeEnableStartsWriting(t *testing.T) {
	resetForTesting(t)
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	setArgvForTesting(t, "opendbx", "interact")

	if err := Init(InitInput{SessionID: "runtime"}); err != nil {
		t.Fatalf("Init err = %v", err)
	}
	L().Info("api: before hidden")
	EnableDebugLogging()
	L().Info("api: after shown")
	if err := Close(); err != nil {
		t.Fatalf("Close err = %v", err)
	}

	logPath := filepath.Join(tmp, ".opendbx", "debug", "runtime.txt")
	raw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read debug log: %v", err)
	}
	got := string(raw)
	if strings.Contains(got, "before hidden") {
		t.Fatalf("logger should not backfill events before EnableDebugLogging:\n%s", got)
	}
	if !strings.Contains(got, "after shown") {
		t.Fatalf("EnableDebugLogging should make later events visible:\n%s", got)
	}
}

// resetForTesting clears all package-level state so each test starts fresh.
//
// Hidden helper exposed only to *_test.go files. Re-creates the once
// primitives so subsequent Init / Close calls take effect.
func resetForTesting(t *testing.T) {
	t.Helper()
	initOnce = sync.Once{}
	closeOnce = sync.Once{}
	initErr = nil
	initDone.Store(false)
	current.Store(nil)
	runtimeDebugEnabled.Store(false)
	hasFormattedOutput.Store(false)
}
