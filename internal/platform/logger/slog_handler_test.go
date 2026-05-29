// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package logger

import (
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"
)

func TestForceFileBypassesDebugGateAndStderr(t *testing.T) {
	// NOT t.Parallel: mutates logger globals and os.Stderr.
	resetForTesting(t)
	logPath := t.TempDir() + "/force.log"

	oldStderr := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stderr = w
	defer func() { os.Stderr = oldStderr }()

	if err := Init(InitInput{SessionID: "force", LogPath: logPath, DebugToStderr: true}); err != nil {
		t.Fatalf("Init: %v", err)
	}
	WarnForceFile("frame budget overshoot", "cols", 120)

	if err := w.Close(); err != nil {
		t.Fatalf("close stderr pipe writer: %v", err)
	}
	stderrRaw, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read stderr pipe: %v", err)
	}
	if strings.Contains(string(stderrRaw), "frame budget overshoot") {
		t.Fatalf("WarnForceFile wrote to stderr: %q", stderrRaw)
	}

	raw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read force log: %v", err)
	}
	got := string(raw)
	if !strings.Contains(got, "[WARN] frame budget overshoot") || !strings.Contains(got, "cols=120") {
		t.Fatalf("force log missing message/attrs:\n%s", got)
	}
}

func TestSlogHandlerLevelMapping(t *testing.T) {
	// NOT t.Parallel: mutates logger globals.
	resetForTesting(t)
	logPath := t.TempDir() + "/slog.log"
	if err := Init(InitInput{SessionID: "slog-map", LogPath: logPath, DebugEnabled: true, DisableSidecar: true}); err != nil {
		t.Fatalf("Init: %v", err)
	}
	l := slog.New(NewSlogHandler()).With("component", "scheduler")
	l.Debug("debug event", "n", 1)
	l.Info("info event", "n", 2)
	l.Warn("warn event", "n", 3)
	l.Error("error event", "n", 4)
	if err := Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	raw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read slog log: %v", err)
	}
	got := string(raw)
	for _, want := range []string{
		"[DEBUG] debug event",
		"[INFO] info event",
		"[WARN] warn event component=scheduler n=3",
		"[ERROR] error event component=scheduler n=4",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("log missing %q:\n%s", want, got)
		}
	}
}

type testLogValuer struct{}

func (testLogValuer) LogValue() slog.Value { return slog.StringValue("value") }

func TestSlogHandlerGroupsAndKVForms(t *testing.T) {
	// NOT t.Parallel: mutates logger globals.
	resetForTesting(t)
	logPath := t.TempDir() + "/groups.log"
	if err := Init(InitInput{SessionID: "groups", LogPath: logPath}); err != nil {
		t.Fatalf("Init: %v", err)
	}
	ErrorForceFile("direct error", slog.String("path", "/tmp/x"), Attr{Key: "user", Value: "alice"}, "missing")
	slog.New(NewSlogHandler()).WithGroup("outer").Error(
		"grouped error",
		slog.Group("inner", slog.String("id", "42")),
		"lv", testLogValuer{},
	)

	raw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read groups log: %v", err)
	}
	got := string(raw)
	for _, want := range []string{
		"[ERROR] direct error path=/tmp/x user=alice missing=<missing>",
		"[ERROR] grouped error outer.inner=map[id:42] outer.lv=value",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("log missing %q:\n%s", want, got)
		}
	}
}

func TestSlogHandlerPreInitDiscardWarnsOnceAfterInit(t *testing.T) {
	// NOT t.Parallel: mutates logger globals.
	resetForTesting(t)
	h := NewSlogHandler()
	l := slog.New(h)
	l.Warn("before init one")
	l.Error("before init two")

	logPath := t.TempDir() + "/preinit.log"
	if err := Init(InitInput{SessionID: "preinit", LogPath: logPath}); err != nil {
		t.Fatalf("Init: %v", err)
	}
	l.Warn("after init")
	l.Warn("after init again")

	raw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read preinit log: %v", err)
	}
	got := string(raw)
	if strings.Contains(got, "before init one") || strings.Contains(got, "before init two") {
		t.Fatalf("pre-init records were replayed instead of discarded:\n%s", got)
	}
	if c := strings.Count(got, "slog records discarded before logger.Init"); c != 1 {
		t.Fatalf("startup discard warning count = %d, want 1:\n%s", c, got)
	}
	if !strings.Contains(got, "count=2") || !strings.Contains(got, "after init") {
		t.Fatalf("post-init log missing discard count or active warn:\n%s", got)
	}
}
