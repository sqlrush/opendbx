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
	"testing"
)

func TestRedactStringPatterns(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"password key=value", "password=hunter2 next", "password=<REDACTED> next"},
		{"PASSWORD upper-case key", "PASSWORD=secret;more", "PASSWORD=<REDACTED>;more"},
		{"token key=value", "token=abc123def&x=y", "token=<REDACTED>&x=y"},
		{"api_key key=value", "api_key=sk_test_xyz123 something", "api_key=<REDACTED> something"},
		{"apikey no underscore", "apikey=foobar more", "apikey=<REDACTED> more"},
		// Authorization header: only the trailing token is masked; surrounding
		// context (incl. trailing " more") is preserved so debug consumers can
		// still see "an Authorization header was present here".
		{"Authorization header", "Authorization: Bearer abcdefghijklmnopqrst more", "Authorization: Bearer <REDACTED> more"},
		{"Bearer naked", "curl -H 'Bearer aaaabbbbccccdddd'", "curl -H 'Bearer <REDACTED>'"},
		{"sk- key form", "key sk-abc123def456ghi789jkl in code", "key sk-<REDACTED> in code"},
		{"url userinfo", "Connected to postgres://user:s3cret@db.example.com/foo", "Connected to postgres://user:<REDACTED>@db.example.com/foo"},
		{"plain text unchanged", "no secrets here", "no secrets here"},
		{"idempotent", "password=<REDACTED>", "password=<REDACTED>"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := redactString(tc.in); got != tc.want {
				t.Errorf("redactString(%q)\n  got:  %q\n  want: %q", tc.in, got, tc.want)
			}
		})
	}
}

type secretsStruct struct {
	User     string
	Password string `redact:"true"`
	Note     string
}

type nestedStruct struct {
	Public string
	Inner  secretsStruct
}

func TestRedactAttrsReservedKeysUnchanged(t *testing.T) {
	t.Parallel()
	in := []Attr{
		{Key: "event", Value: "tool.call"},
		{Key: "trace_id", Value: "t-1"},
		{Key: "span_id", Value: "s-1"},
	}
	out := redactAttrs(in)
	for i, a := range in {
		if out[i].Value != a.Value {
			t.Errorf("reserved key %q got redacted: %v → %v", a.Key, a.Value, out[i].Value)
		}
	}
}

func TestRedactAttrsSecretKeyName(t *testing.T) {
	t.Parallel()
	in := []Attr{
		{Key: "password", Value: "hunter2"},
		{Key: "apiKey", Value: "abc"},
		{Key: "token", Value: "xyz"},
		{Key: "User", Value: "alice"},
	}
	out := redactAttrs(in)
	for _, a := range out[:3] {
		if a.Value != redactionToken {
			t.Errorf("attr %q value not redacted: %v", a.Key, a.Value)
		}
	}
	if out[3].Value != "alice" {
		t.Errorf("non-secret attr 'User' was redacted: %v", out[3].Value)
	}
}

func TestRedactAttrsStringValuePatternMasked(t *testing.T) {
	t.Parallel()
	in := []Attr{{Key: "log_line", Value: "got password=topsecret in stream"}}
	out := redactAttrs(in)
	got, _ := out[0].Value.(string)
	if !strings.Contains(got, "password=<REDACTED>") || strings.Contains(got, "topsecret") {
		t.Errorf("string value not pattern-masked: %q", got)
	}
}

func TestRedactAttrsStructWithRedactTag(t *testing.T) {
	t.Parallel()
	in := []Attr{
		{Key: "config", Value: secretsStruct{User: "alice", Password: "raw-secret", Note: "ok"}},
	}
	out := redactAttrs(in)
	got, ok := out[0].Value.(secretsStruct)
	if !ok {
		t.Fatalf("redacted value type = %T, want secretsStruct", out[0].Value)
	}
	if got.User != "alice" {
		t.Errorf("untagged field corrupted: User = %q", got.User)
	}
	if got.Password != redactionToken {
		t.Errorf("redact:\"true\" field not masked: Password = %q", got.Password)
	}
	if got.Note != "ok" {
		t.Errorf("plain field corrupted: Note = %q", got.Note)
	}
}

func TestRedactAttrsNestedStruct(t *testing.T) {
	t.Parallel()
	in := []Attr{{Key: "config", Value: nestedStruct{Public: "pub", Inner: secretsStruct{User: "alice", Password: "raw"}}}}
	out := redactAttrs(in)
	got, ok := out[0].Value.(nestedStruct)
	if !ok {
		t.Fatalf("redacted type = %T", out[0].Value)
	}
	if got.Inner.Password != redactionToken {
		t.Errorf("nested redact:\"true\" not masked: %q", got.Inner.Password)
	}
}

func TestRedactAttrsErrorWrapping(t *testing.T) {
	t.Parallel()
	in := []Attr{{Key: "err", Value: errors.New("connect failed: password=topsecret refused")}}
	out := redactAttrs(in)
	got, ok := out[0].Value.(error)
	if !ok {
		t.Fatalf("error not preserved as error type: %T", out[0].Value)
	}
	if strings.Contains(got.Error(), "topsecret") {
		t.Errorf("error message leaks secret: %q", got.Error())
	}
	if !strings.Contains(got.Error(), "password=<REDACTED>") {
		t.Errorf("error message missing redaction token: %q", got.Error())
	}
}

func TestRedactAttrsMapValues(t *testing.T) {
	t.Parallel()
	in := []Attr{{Key: "cfg", Value: map[string]any{
		"user":     "alice",
		"password": "hunter2",
		"note":     "log says password=raw inline",
	}}}
	out := redactAttrs(in)
	got, ok := out[0].Value.(map[string]any)
	if !ok {
		t.Fatalf("map type lost: %T", out[0].Value)
	}
	if got["password"] != redactionToken {
		t.Errorf("map password not redacted: %v", got["password"])
	}
	if note, _ := got["note"].(string); !strings.Contains(note, "<REDACTED>") || strings.Contains(note, "raw") {
		t.Errorf("map string value not pattern-masked: %q", got["note"])
	}
	if got["user"] != "alice" {
		t.Errorf("non-secret map value corrupted: %v", got["user"])
	}
}

// End-to-end: a write that includes a secret in both attrs and msg must not
// leak it in either the main text log or the sidecar JSONL.
func TestLoggerEndToEndNoSecretLeak(t *testing.T) {
	resetForTesting(t)
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	logPath := filepath.Join(tmp, "debug.log")
	setArgvForTesting(t, "opendbx", "--debug-file", logPath)

	if err := Init(InitInput{SessionID: "redact-e2e"}); err != nil {
		t.Fatalf("Init: %v", err)
	}
	L().Info("connecting with password=raw-secret to db",
		Attr{Key: "dsn", Value: "postgres://u:supersecret@host/x"},
		Attr{Key: "Password", Value: "raw"},
	)
	_, sp := StartSpan(context.Background(), "auth")
	sp.RecordError(errors.New("login failed with token=letmein"))
	sp.End()
	if err := Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	mainText, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read main: %v", err)
	}
	sidecar, err := os.ReadFile(filepath.Join(tmp, ".opendbx", "debug", "redact-e2e.events.jsonl"))
	if err != nil {
		t.Fatalf("read sidecar: %v", err)
	}

	for _, leak := range []string{"raw-secret", "raw\"", "supersecret", "letmein"} {
		if strings.Contains(string(mainText), leak) {
			t.Errorf("main text leaks %q:\n%s", leak, mainText)
		}
		if strings.Contains(string(sidecar), leak) {
			t.Errorf("sidecar leaks %q:\n%s", leak, sidecar)
		}
	}
	// And a sanity assertion: redaction token appears at least once.
	if !strings.Contains(string(mainText), redactionToken) {
		t.Errorf("main text missing redaction token (no redaction happened?):\n%s", mainText)
	}
}
