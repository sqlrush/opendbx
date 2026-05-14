// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const fixedNow = "2026-05-14"

func fixedTime(t *testing.T) time.Time {
	t.Helper()
	tt, err := time.Parse("2006-01-02", fixedNow)
	if err != nil {
		t.Fatalf("parse fixedNow: %v", err)
	}
	return tt
}

// writeAllowlist creates a temp allowlist.json with the given content.
func writeAllowlist(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "allowlist.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write allowlist: %v", err)
	}
	return path
}

// fullExempt produces a complete allowlist entry with all mandatory fields.
// T-7.5 codex HIGH-2: module + reason are now mandatory.
const fullExempt = `{"exemptions":[
	{"osv_id":"GO-2026-4602","module":"stdlib","expiry":"2026-08-14","reason":"r","spec_ref":"s"}
]}`

// --- Path 1: empty stream (no findings) → OK ---------------------------

func TestRun_NoFindings(t *testing.T) {
	t.Parallel()
	allow := writeAllowlist(t, `{"exemptions":[]}`)
	var out bytes.Buffer
	code := run(allow, strings.NewReader(""), &out, fixedTime(t))
	if code != 0 {
		t.Errorf("expected exit 0; got %d; out=%s", code, out.String())
	}
	if !strings.Contains(out.String(), "no called vulnerabilities") {
		t.Errorf("expected no-vuln message; got %q", out.String())
	}
}

// --- Path 2: finding with no function-level trace = not called → OK ----

func TestRun_FindingNotCalled(t *testing.T) {
	t.Parallel()
	allow := writeAllowlist(t, `{"exemptions":[]}`)
	stream := `{"finding":{"osv":"GO-2026-9999","trace":[{"module":"stdlib","version":"v1.23.0"}]}}`
	var out bytes.Buffer
	code := run(allow, strings.NewReader(stream), &out, fixedTime(t))
	if code != 0 {
		t.Errorf("module-only trace must not fail; got %d; out=%s", code, out.String())
	}
}

// --- Path 3: called vuln, no exemption → FAIL --------------------------

func TestRun_CalledNoExemption(t *testing.T) {
	t.Parallel()
	allow := writeAllowlist(t, `{"exemptions":[]}`)
	stream := `{"finding":{"osv":"GO-2026-4602","trace":[
		{"module":"stdlib","function":"os.ReadDir"},
		{"package":"github.com/sqlrush/opendbx/tools/import-rules-check","function":"main"}
	]}}`
	var out bytes.Buffer
	code := run(allow, strings.NewReader(stream), &out, fixedTime(t))
	if code != 1 {
		t.Errorf("called + no exemption must fail; got %d", code)
	}
	if !strings.Contains(out.String(), "GO-2026-4602") || !strings.Contains(out.String(), "no exemption") {
		t.Errorf("expected block reason; got %q", out.String())
	}
}

// --- Path 4: called vuln, valid exemption → OK -------------------------

func TestRun_CalledValidExemption(t *testing.T) {
	t.Parallel()
	allow := writeAllowlist(t, fullExempt)
	stream := `{"finding":{"osv":"GO-2026-4602","trace":[
		{"module":"stdlib","function":"os.ReadDir"},
		{"package":"x","function":"main"}
	]}}`
	var out bytes.Buffer
	code := run(allow, strings.NewReader(stream), &out, fixedTime(t))
	if code != 0 {
		t.Errorf("called + valid exemption must pass; got %d; out=%s", code, out.String())
	}
	if !strings.Contains(out.String(), "[exempt]") {
		t.Errorf("expected exempt mark; got %q", out.String())
	}
	if !strings.Contains(out.String(), "stdlib") {
		t.Errorf("expected module in output; got %q", out.String())
	}
}

// --- Path 5: called vuln, expired exemption → FAIL ---------------------

func TestRun_CalledExpiredExemption(t *testing.T) {
	t.Parallel()
	allow := writeAllowlist(t, `{"exemptions":[
		{"osv_id":"GO-2026-4602","module":"stdlib","expiry":"2026-01-01","reason":"r","spec_ref":"s"}
	]}`)
	stream := `{"finding":{"osv":"GO-2026-4602","trace":[
		{"module":"stdlib","function":"os.ReadDir"},
		{"package":"x","function":"main"}
	]}}`
	var out bytes.Buffer
	code := run(allow, strings.NewReader(stream), &out, fixedTime(t))
	if code != 1 {
		t.Errorf("expired exemption must fail; got %d", code)
	}
	if !strings.Contains(out.String(), "expired on 2026-01-01") {
		t.Errorf("expected expired message; got %q", out.String())
	}
	if !strings.Contains(out.String(), "renew or fix") {
		t.Errorf("expected renew hint; got %q", out.String())
	}
}

// --- Path 5b: T-7.5 codex HIGH-1: end-of-day on expiry date still valid -

func TestRun_ExpiryDateStillValid(t *testing.T) {
	t.Parallel()
	allow := writeAllowlist(t, `{"exemptions":[
		{"osv_id":"GO-2026-4602","module":"stdlib","expiry":"2026-08-14","reason":"r","spec_ref":"s"}
	]}`)
	stream := `{"finding":{"osv":"GO-2026-4602","trace":[
		{"module":"stdlib","function":"f"},{"function":"main"}
	]}}`
	// On the expiry date itself at any time of day → still valid.
	now := time.Date(2026, 8, 14, 23, 59, 59, 0, time.UTC)
	var out bytes.Buffer
	code := run(allow, strings.NewReader(stream), &out, now)
	if code != 0 {
		t.Errorf("expiry date itself must remain valid; got %d; out=%s", code, out.String())
	}
}

// --- Path 5c: T-7.5 codex HIGH-1: one day after expiry → expired -------

func TestRun_DayAfterExpiry(t *testing.T) {
	t.Parallel()
	allow := writeAllowlist(t, `{"exemptions":[
		{"osv_id":"GO-2026-4602","module":"stdlib","expiry":"2026-08-14","reason":"r","spec_ref":"s"}
	]}`)
	stream := `{"finding":{"osv":"GO-2026-4602","trace":[
		{"module":"stdlib","function":"f"},{"function":"main"}
	]}}`
	now := time.Date(2026, 8, 15, 0, 0, 1, 0, time.UTC)
	var out bytes.Buffer
	code := run(allow, strings.NewReader(stream), &out, now)
	if code != 1 {
		t.Errorf("day after expiry must fail; got %d", code)
	}
}

// --- Path 5d: T-7.5 codex HIGH-2: module mismatch blocks --------------

func TestRun_ModuleMismatch(t *testing.T) {
	t.Parallel()
	// Allowlist exempts OSV for stdlib, but finding reports a different module.
	allow := writeAllowlist(t, fullExempt)
	stream := `{"finding":{"osv":"GO-2026-4602","trace":[
		{"module":"github.com/other/repo","function":"Bad"},
		{"function":"main"}
	]}}`
	var out bytes.Buffer
	code := run(allow, strings.NewReader(stream), &out, fixedTime(t))
	if code != 1 {
		t.Errorf("module mismatch must fail; got %d; out=%s", code, out.String())
	}
	if !strings.Contains(out.String(), "module mismatch") {
		t.Errorf("expected module mismatch reason; got %q", out.String())
	}
}

// --- Path 6: 2 called vulns (1 exempt + 1 not) → FAIL (mixed) ----------

func TestRun_MixedExemption(t *testing.T) {
	t.Parallel()
	allow := writeAllowlist(t, `{"exemptions":[
		{"osv_id":"GO-2026-AAAA","module":"stdlib","expiry":"2026-08-14","reason":"r","spec_ref":"s"}
	]}`)
	stream := strings.Join([]string{
		`{"finding":{"osv":"GO-2026-AAAA","trace":[{"module":"stdlib","function":"f"},{"function":"main"}]}}`,
		`{"finding":{"osv":"GO-2026-BBBB","trace":[{"module":"stdlib","function":"f"},{"function":"main"}]}}`,
	}, "\n")
	var out bytes.Buffer
	code := run(allow, strings.NewReader(stream), &out, fixedTime(t))
	if code != 1 {
		t.Errorf("mixed must fail; got %d", code)
	}
	if !strings.Contains(out.String(), "GO-2026-BBBB") {
		t.Errorf("expected BBBB blocked; got %q", out.String())
	}
}

// --- Path 7: duplicate finding lines per OSV → dedup -------------------

func TestRun_DuplicateFindings(t *testing.T) {
	t.Parallel()
	allow := writeAllowlist(t, `{"exemptions":[
		{"osv_id":"GO-2026-CCCC","module":"stdlib","expiry":"2026-08-14","reason":"r","spec_ref":"s"}
	]}`)
	stream := strings.Repeat(
		`{"finding":{"osv":"GO-2026-CCCC","trace":[{"module":"stdlib","function":"f"},{"function":"main"}]}}`+"\n",
		3)
	var out bytes.Buffer
	code := run(allow, strings.NewReader(stream), &out, fixedTime(t))
	if code != 0 {
		t.Errorf("duplicate findings dedup must pass; got %d", code)
	}
	if strings.Count(out.String(), "[exempt]") != 1 {
		t.Errorf("expected 1 [exempt] line; got %q", out.String())
	}
}

// --- Path 8: malformed govulncheck JSON → exit 2 ----------------------

func TestRun_MalformedStream(t *testing.T) {
	t.Parallel()
	allow := writeAllowlist(t, `{"exemptions":[]}`)
	var out bytes.Buffer
	code := run(allow, strings.NewReader("{ this is not json"), &out, fixedTime(t))
	if code != 2 {
		t.Errorf("malformed JSON must exit 2; got %d", code)
	}
}

// --- Path 9: allowlist schema violations → exit 2 ---------------------

func TestRun_AllowlistMissingExpiry(t *testing.T) {
	t.Parallel()
	allow := writeAllowlist(t, `{"exemptions":[
		{"osv_id":"GO-2026-XXXX","module":"stdlib","reason":"r","spec_ref":"s"}
	]}`)
	var out bytes.Buffer
	code := run(allow, strings.NewReader(""), &out, fixedTime(t))
	if code != 2 {
		t.Errorf("missing expiry must exit 2; got %d", code)
	}
	if !strings.Contains(out.String(), "expiry required") {
		t.Errorf("expected expiry error; got %q", out.String())
	}
}

func TestRun_AllowlistMissingModule(t *testing.T) {
	t.Parallel()
	// T-7.5 codex HIGH-2: module mandatory.
	allow := writeAllowlist(t, `{"exemptions":[
		{"osv_id":"GO-2026-XXXX","expiry":"2026-08-14","reason":"r","spec_ref":"s"}
	]}`)
	var out bytes.Buffer
	code := run(allow, strings.NewReader(""), &out, fixedTime(t))
	if code != 2 {
		t.Errorf("missing module must exit 2; got %d", code)
	}
	if !strings.Contains(out.String(), "module required") {
		t.Errorf("expected module error; got %q", out.String())
	}
}

func TestRun_AllowlistMissingReason(t *testing.T) {
	t.Parallel()
	// T-7.5 codex HIGH-2: reason mandatory.
	allow := writeAllowlist(t, `{"exemptions":[
		{"osv_id":"GO-2026-XXXX","module":"stdlib","expiry":"2026-08-14","spec_ref":"s"}
	]}`)
	var out bytes.Buffer
	code := run(allow, strings.NewReader(""), &out, fixedTime(t))
	if code != 2 {
		t.Errorf("missing reason must exit 2; got %d", code)
	}
	if !strings.Contains(out.String(), "reason required") {
		t.Errorf("expected reason error; got %q", out.String())
	}
}

func TestRun_AllowlistMissingSpecRef(t *testing.T) {
	t.Parallel()
	allow := writeAllowlist(t, `{"exemptions":[
		{"osv_id":"GO-2026-XXXX","module":"stdlib","expiry":"2026-08-14","reason":"r"}
	]}`)
	var out bytes.Buffer
	code := run(allow, strings.NewReader(""), &out, fixedTime(t))
	if code != 2 {
		t.Errorf("missing spec_ref must exit 2; got %d", code)
	}
	if !strings.Contains(out.String(), "spec_ref required") {
		t.Errorf("expected spec_ref error; got %q", out.String())
	}
}

func TestRun_AllowlistBadDate(t *testing.T) {
	t.Parallel()
	allow := writeAllowlist(t, `{"exemptions":[
		{"osv_id":"GO-2026-XXXX","module":"stdlib","expiry":"08-14-2026","reason":"r","spec_ref":"s"}
	]}`)
	var out bytes.Buffer
	code := run(allow, strings.NewReader(""), &out, fixedTime(t))
	if code != 2 {
		t.Errorf("bad date format must exit 2; got %d", code)
	}
	if !strings.Contains(out.String(), "invalid expiry") {
		t.Errorf("expected invalid expiry; got %q", out.String())
	}
}

func TestRun_AllowlistDuplicate(t *testing.T) {
	t.Parallel()
	allow := writeAllowlist(t, `{"exemptions":[
		{"osv_id":"GO-2026-XXXX","module":"stdlib","expiry":"2026-08-14","reason":"r","spec_ref":"s"},
		{"osv_id":"GO-2026-XXXX","module":"stdlib","expiry":"2026-09-14","reason":"r2","spec_ref":"s2"}
	]}`)
	var out bytes.Buffer
	code := run(allow, strings.NewReader(""), &out, fixedTime(t))
	if code != 2 {
		t.Errorf("duplicate OSV must exit 2; got %d", code)
	}
}

// --- Path 10: missing allowlist file → exit 2 -------------------------

func TestRun_AllowlistNotFound(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	code := run("/no/such/allowlist-xyz.json", strings.NewReader(""), &out, fixedTime(t))
	if code != 2 {
		t.Errorf("missing allowlist must exit 2; got %d", code)
	}
}

// --- Path 11: shipped allowlist.json sanity --------------------------

func TestShippedAllowlist_Parses(t *testing.T) {
	t.Parallel()
	list, err := loadAllowlist("allowlist.json")
	if err != nil {
		t.Fatalf("shipped allowlist must parse: %v", err)
	}
	if _, ok := list["GO-2026-4602"]; !ok {
		t.Errorf("shipped allowlist must include GO-2026-4602 (Go 1.23 stdlib os.FileInfo escape; T-3.5)")
	}
	// T-7.5 codex HIGH-1 + claude HIGH: use date-only comparison so on the
	// expiry date itself the test does not falsely flag entries as expired.
	today := dateOnly(time.Now())
	for id, ex := range list {
		if ex.SpecRef == "" {
			t.Errorf("%s missing spec_ref", id)
		}
		if ex.expiryParsed.Before(today) {
			t.Errorf("%s expiry %s is already in the past — renew or remove", id, ex.Expiry)
		}
	}
}
