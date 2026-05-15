// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package aivisual

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// --- Prompt frozen sha256 test ------------------------------------

func TestPromptFrozen(t *testing.T) {
	t.Parallel()
	// R5 spec § D-3 + 11.3: prompt.txt sha256 守门. T-5 commits the
	// initial template; this test记录 the resulting hash so future
	// commits that mutate it must explicitly update the hash + BREAKING
	// commit (spec § 11.3).
	p := Prompt()
	if p == "" {
		t.Fatal("Prompt() returned empty; embed failed")
	}
	if !strings.Contains(p, "alignment") || !strings.Contains(p, "cjk-width") {
		t.Error("prompt.txt should contain the 6 evaluation dimensions")
	}
	// spec-0.11.5 T-13 errata (codex HIGH-5 + claude MED-2): real freeze
	// guard. Update wantHash via BREAKING commit (feat!: + BREAKING CHANGE
	// body per § 11.3) when prompt.txt is intentionally changed.
	const wantHash = "9202da09d59d085299e24bf82f11d29e4968b4c5f001c98017e5301262a06ed9"
	hash := sha256.Sum256([]byte(p))
	hashHex := fmt.Sprintf("%x", hash)
	if hashHex != wantHash {
		t.Fatalf("prompt.txt SHA-256 drift: want %s got %s — update wantHash via BREAKING commit", wantHash, hashHex)
	}
}

// --- happy path via httptest -----------------------------------

func TestReview_HappyPath(t *testing.T) {
	t.Parallel()
	wantReport := Report{
		Verdict: VerdictOK,
		Issues:  []Issue{},
		Tokens:  42,
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Echo a synthetic OpenAI-compatible response with the
		// canned report embedded as JSON-in-string.
		reportJSON, _ := json.Marshal(wantReport)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"content": string(reportJSON)}},
			},
			"usage": map[string]int{"total_tokens": 42},
		})
	}))
	defer srv.Close()

	reviewer := &Reviewer{Endpoint: srv.URL, Model: "test-model", Timeout: 5 * time.Second}
	got, err := reviewer.Review(context.Background(), []byte("\x89PNG\r\n\x1a\n"), "table alignment")
	if err != nil {
		t.Fatalf("Review: %v", err)
	}
	if got.Verdict != VerdictOK {
		t.Errorf("verdict = %q, want %q", got.Verdict, VerdictOK)
	}
	if got.Tokens != 42 {
		t.Errorf("tokens = %d, want 42", got.Tokens)
	}
}

func TestReview_IssuesFound(t *testing.T) {
	t.Parallel()
	wantReport := Report{
		Verdict: VerdictIssuesFound,
		Issues: []Issue{
			{Severity: SeverityHigh, Category: CategoryAlignment, Message: "header column drift"},
		},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reportJSON, _ := json.Marshal(wantReport)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"content": string(reportJSON)}},
			},
		})
	}))
	defer srv.Close()

	reviewer := &Reviewer{Endpoint: srv.URL}
	got, err := reviewer.Review(context.Background(), []byte("png"), "")
	if err != nil {
		t.Fatalf("Review: %v", err)
	}
	if got.Verdict != VerdictIssuesFound {
		t.Errorf("verdict = %q, want %q", got.Verdict, VerdictIssuesFound)
	}
	if len(got.Issues) != 1 || got.Issues[0].Category != CategoryAlignment {
		t.Errorf("expected 1 alignment issue; got %+v", got.Issues)
	}
}

// --- failure paths ------------------------------------------------

func TestReview_EndpointDown(t *testing.T) {
	t.Parallel()
	reviewer := &Reviewer{Endpoint: "http://127.0.0.1:1/v1", Timeout: 200 * time.Millisecond}
	_, err := reviewer.Review(context.Background(), []byte("png"), "")
	if !errors.Is(err, ErrEndpointDown) {
		t.Errorf("expected ErrEndpointDown; got %v", err)
	}
}

// spec-0.11.5 T-13 errata (security LOW-1): non-http schemes are rejected
// before request construction so misconfig produces a clear error.
func TestReview_InvalidScheme(t *testing.T) {
	t.Parallel()
	for _, sch := range []string{"file:///etc/passwd", "ftp://x/y", "ws://x/y"} {
		reviewer := &Reviewer{Endpoint: sch}
		_, err := reviewer.Review(context.Background(), []byte("png"), "")
		if err == nil || !strings.Contains(err.Error(), "invalid endpoint scheme") {
			t.Errorf("scheme %q should reject; got %v", sch, err)
		}
	}
}

// spec-0.11.5 T-13 errata (security MED-1): 4xx responses surface as
// ErrEndpointDown so auth/rate-limit misconfig fails loud rather than
// silently returning uncertain verdict.
func TestReview_4xx_MapsToEndpointDown(t *testing.T) {
	t.Parallel()
	for _, code := range []int{401, 403, 429} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(code)
		}))
		reviewer := &Reviewer{Endpoint: srv.URL}
		_, err := reviewer.Review(context.Background(), []byte("png"), "")
		srv.Close()
		if !errors.Is(err, ErrEndpointDown) {
			t.Errorf("HTTP %d should map to ErrEndpointDown; got %v", code, err)
		}
	}
}

func TestReview_500_MapsToEndpointDown(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	reviewer := &Reviewer{Endpoint: srv.URL}
	_, err := reviewer.Review(context.Background(), []byte("png"), "")
	if !errors.Is(err, ErrEndpointDown) {
		t.Errorf("HTTP 500 should map to ErrEndpointDown; got %v", err)
	}
}

func TestReview_InvalidJSON_FallsBackToUncertain(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return invalid OpenAI envelope (not JSON).
		_, _ = w.Write([]byte("not json"))
	}))
	defer srv.Close()

	reviewer := &Reviewer{Endpoint: srv.URL}
	got, err := reviewer.Review(context.Background(), []byte("png"), "")
	if err != nil {
		t.Fatalf("invalid JSON should fall back, not error: %v", err)
	}
	if got.Verdict != VerdictUncertain {
		t.Errorf("verdict = %q, want %q", got.Verdict, VerdictUncertain)
	}
}

func TestReview_EmptyChoices_FallsBackToUncertain(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{}})
	}))
	defer srv.Close()

	reviewer := &Reviewer{Endpoint: srv.URL}
	got, err := reviewer.Review(context.Background(), []byte("png"), "")
	if err != nil {
		t.Fatalf("empty choices should fall back, not error: %v", err)
	}
	if got.Verdict != VerdictUncertain {
		t.Errorf("verdict = %q, want %q", got.Verdict, VerdictUncertain)
	}
}

func TestReview_ContentNotJSON_FallsBackToUncertain(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"content": "not a json report"}},
			},
		})
	}))
	defer srv.Close()

	reviewer := &Reviewer{Endpoint: srv.URL}
	got, err := reviewer.Review(context.Background(), []byte("png"), "")
	if err != nil {
		t.Fatalf("non-JSON content should fall back, not error: %v", err)
	}
	if got.Verdict != VerdictUncertain {
		t.Errorf("verdict = %q, want %q", got.Verdict, VerdictUncertain)
	}
}

// --- ChatMessage marshaling shapes ------------------------------

func TestChatMessage_StringContent(t *testing.T) {
	t.Parallel()
	m := chatMessage{Role: "system", Content: "hi"}
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(b), `"content":"hi"`) {
		t.Errorf("expected string content; got %s", b)
	}
}

func TestChatMessage_ArrayContent(t *testing.T) {
	t.Parallel()
	m := chatMessage{Role: "user", ContentArr: []contentPart{
		{Type: "text", Text: "hi"},
		{Type: "image_url", ImageURL: &imageURL{URL: "data:..."}},
	}}
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(b), `"image_url"`) {
		t.Errorf("expected image_url content part; got %s", b)
	}
}

// --- Schema constants tested for stability ------------------------

func TestSchemaConstants(t *testing.T) {
	t.Parallel()
	if VerdictOK != "ok" || VerdictIssuesFound != "issues-found" || VerdictUncertain != "uncertain" {
		t.Error("Verdict constants changed; schema BREAKING")
	}
	if SeverityHigh != "high" || SeverityMedium != "medium" || SeverityLow != "low" {
		t.Error("Severity constants changed; schema BREAKING")
	}
	if CategoryAlignment != "alignment" || CategoryCJKWidth != "cjk-width" {
		t.Error("Category constants changed; schema BREAKING")
	}
}
