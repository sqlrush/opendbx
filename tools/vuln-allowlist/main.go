// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// Package main implements vuln-allowlist, a wrapper around govulncheck JSON
// output that filters findings against an OSV-ID allowlist with expiry.
// spec-0.9 D-2.5 / T-3.5.
//
// Why this exists (R2 codex HIGH-3):
//
// govulncheck does not natively support inline `// govulncheck:exempt` or
// any source-level suppression. The official tool either fixes the
// vulnerability or fails. For production teams that must defer upgrades
// (e.g., Stage 0 locked Go 1.23 stdlib vs Go 1.25.8 fix for GO-2026-4602),
// a wrapper with explicit OSV-ID + expiry allowlist provides the only safe
// suppression path: every exemption has an expiry date and required
// spec_ref, so allowlists cannot rot silently.
//
// Pipeline:
//
//	govulncheck -json -test ./... | go run ./tools/vuln-allowlist
//
// or wired into `make vuln-check` (spec-0.9 D-2.5 / T-3.5).
//
// Allowlist file: tools/vuln-allowlist/allowlist.json
//
//	{
//	  "exemptions": [
//	    {
//	      "osv_id": "GO-2026-4602",
//	      "module": "stdlib",
//	      "expiry": "2026-08-01",
//	      "reason": "Go 1.25.8 fix; Stage 0 locks 1.23",
//	      "spec_ref": "spec-0.9-ci-github-actions.md § 3.1 T-3.5"
//	    }
//	  ]
//	}
//
// Exit codes:
//
//	0  no called vulnerabilities, OR all called vulns covered by valid (non-expired) exemptions
//	1  one or more called vulns lack a valid exemption (or exemption expired)
//	2  malformed allowlist / malformed govulncheck JSON
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"time"
)

// vulnFinding mirrors a "finding" entry in govulncheck's JSON stream.
// Trace is the call chain from user code to the vulnerable symbol; a
// trace with len > 1 (or with any function-level entry) indicates the
// vulnerability is actually reachable from this code.
type vulnFinding struct {
	OSV          string       `json:"osv"`
	FixedVersion string       `json:"fixed_version,omitempty"`
	Trace        []traceEntry `json:"trace,omitempty"`
}

type traceEntry struct {
	Module   string `json:"module,omitempty"`
	Version  string `json:"version,omitempty"`
	Package  string `json:"package,omitempty"`
	Function string `json:"function,omitempty"`
}

// govulnMessage is one line of the govulncheck JSON stream. Only one of
// the optional fields is non-nil per line.
type govulnMessage struct {
	Finding *vulnFinding `json:"finding,omitempty"`
}

// allowlistFile is the on-disk allowlist schema.
type allowlistFile struct {
	Exemptions []exemption `json:"exemptions"`
}

type exemption struct {
	OSVID   string `json:"osv_id"`
	Module  string `json:"module"`
	Expiry  string `json:"expiry"` // YYYY-MM-DD
	Reason  string `json:"reason"`
	SpecRef string `json:"spec_ref"`

	// expiryParsed is cached from Expiry during loadAllowlist (T-7.5
	// codex HIGH-1 + go-reviewer MED-2 修): single parse, no silent
	// `_ =` discard at classify time.
	expiryParsed time.Time
}

// loadAllowlist reads and validates the allowlist file.
//
// T-7.5 codex HIGH-2 修: module + reason are mandatory per spec § 1.1 D-2.5;
// previous version only required osv_id / expiry / spec_ref.
func loadAllowlist(path string) (map[string]exemption, error) {
	raw, err := os.ReadFile(path) // #nosec G304 -- spec-0.9 D-2.5: operator-supplied allowlist path
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var f allowlistFile
	if err := json.Unmarshal(raw, &f); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	out := make(map[string]exemption, len(f.Exemptions))
	for i, e := range f.Exemptions {
		if e.OSVID == "" {
			return nil, fmt.Errorf("exemption[%d]: osv_id required", i)
		}
		if e.Module == "" {
			return nil, fmt.Errorf("exemption[%d] %s: module required (spec § 1.1 D-2.5)", i, e.OSVID)
		}
		if e.Expiry == "" {
			return nil, fmt.Errorf("exemption[%d] %s: expiry required (YYYY-MM-DD)", i, e.OSVID)
		}
		if e.Reason == "" {
			return nil, fmt.Errorf("exemption[%d] %s: reason required (no anonymous exemptions)", i, e.OSVID)
		}
		if e.SpecRef == "" {
			return nil, fmt.Errorf("exemption[%d] %s: spec_ref required (no anonymous exemptions)", i, e.OSVID)
		}
		// T-13 codex MED-1 + go-reviewer MED: parse expiry in UTC explicitly
		// (else default Parse returns UTC but compared against local-zone now,
		// causing premature expiry in UTC-negative timezones — e.g.
		// America/Los_Angeles flips at start of expiry day instead of end).
		expiryParsed, err := time.ParseInLocation("2006-01-02", e.Expiry, time.UTC)
		if err != nil {
			return nil, fmt.Errorf("exemption[%d] %s: invalid expiry %q (want YYYY-MM-DD): %w", i, e.OSVID, e.Expiry, err)
		}
		if _, dup := out[e.OSVID]; dup {
			return nil, fmt.Errorf("exemption[%d] %s: duplicate OSV ID", i, e.OSVID)
		}
		e.expiryParsed = expiryParsed
		out[e.OSVID] = e
	}
	return out, nil
}

// dateOnly truncates t to YYYY-MM-DD 00:00 UTC, dropping time-of-day and
// normalizing to UTC. T-7.5 codex HIGH-1 修 + T-13 codex MED-1 + go-reviewer
// MED 修: expiry contract is "expiry < today" by *calendar date*. Both sides
// MUST be expressed in the same timezone or the comparison drifts by ±24h
// depending on host zone. Allowlist expiries are loaded with
// ParseInLocation(..., time.UTC), so today must also be UTC-normalized.
// Earlier implementation preserved t.Location() and caused premature expiry
// in UTC-negative zones (LA flipped at start of expiry day rather than end).
func dateOnly(t time.Time) time.Time {
	u := t.UTC()
	return time.Date(u.Year(), u.Month(), u.Day(), 0, 0, 0, 0, time.UTC)
}

// isCalled returns true when the trace shows a function-level entry,
// indicating the vulnerable symbol is actually invoked from user code.
// trace with only module-level frames is govulncheck's "vuln exists in
// dep but not reachable" record — those do not fail CI.
func isCalled(f *vulnFinding) bool {
	for _, t := range f.Trace {
		if t.Function != "" {
			return true
		}
	}
	return false
}

// calledFinding pairs an OSV with the top-of-trace module (vulnerable
// frame). T-7.5 codex HIGH-2 修: classifier matches by both OSV and
// module so an allowlist entry for stdlib does not silently exempt the
// same OSV showing up in a third-party module re-publish.
type calledFinding struct {
	OSV    string
	Module string
}

// findingModule returns the module of the top-most (vulnerable) frame
// in the trace. trace[0] is the deepest vulnerable symbol per
// govulncheck schema; later frames are the call chain up to user code.
// Empty if no module info present.
func findingModule(f *vulnFinding) string {
	if len(f.Trace) == 0 {
		return ""
	}
	return f.Trace[0].Module
}

// uniqueCalledFindings reads the govulncheck JSON stream and returns
// each called-vuln (OSV, module) pair seen, in stable order. Govulncheck
// emits one finding per trace; we collapse duplicates per (OSV, module).
func uniqueCalledFindings(r io.Reader) ([]calledFinding, error) {
	dec := json.NewDecoder(r)
	seen := make(map[string]bool)
	var order []calledFinding
	for {
		var msg govulnMessage
		if err := dec.Decode(&msg); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, fmt.Errorf("decode govulncheck stream: %w", err)
		}
		if msg.Finding == nil {
			continue
		}
		if !isCalled(msg.Finding) {
			continue
		}
		cf := calledFinding{OSV: msg.Finding.OSV, Module: findingModule(msg.Finding)}
		key := cf.OSV + "\x00" + cf.Module
		if seen[key] {
			continue
		}
		seen[key] = true
		order = append(order, cf)
	}
	sort.Slice(order, func(i, j int) bool {
		if order[i].OSV != order[j].OSV {
			return order[i].OSV < order[j].OSV
		}
		return order[i].Module < order[j].Module
	})
	return order, nil
}

// classify pairs each called finding with its allowlist verdict.
type verdict int

const (
	verdictOK      verdict = iota // exempted, non-expired
	verdictBlocked                // no exemption / expired / module mismatch
)

type result struct {
	OSV      string
	Module   string // module reported by govulncheck (vulnerable frame)
	Verdict  verdict
	Reason   string // populated when blocked
	Exempt   exemption
	ExpiryAt time.Time
}

// classifyFindings cross-references called findings against the allowlist
// using `now` as today. T-7.5 modifications:
//   - codex HIGH-1: date-only comparison; expiry valid until end-of-day.
//   - codex HIGH-2: match by (OSV, module) tuple; module mismatch blocks.
//   - go-reviewer MED-2: use cached expiryParsed; no second silent _ parse.
func classifyFindings(called []calledFinding, list map[string]exemption, now time.Time) []result {
	today := dateOnly(now)
	out := make([]result, 0, len(called))
	for _, f := range called {
		ex, ok := list[f.OSV]
		if !ok {
			out = append(out, result{OSV: f.OSV, Module: f.Module, Verdict: verdictBlocked, Reason: "no exemption in allowlist"})
			continue
		}
		if ex.Module != f.Module {
			out = append(out, result{
				OSV: f.OSV, Module: f.Module, Verdict: verdictBlocked,
				Reason: fmt.Sprintf("module mismatch (allowlist=%q, finding=%q)", ex.Module, f.Module),
				Exempt: ex,
			})
			continue
		}
		if ex.expiryParsed.Before(today) {
			out = append(out, result{
				OSV: f.OSV, Module: f.Module, Verdict: verdictBlocked,
				Reason:   fmt.Sprintf("exemption expired on %s", ex.Expiry),
				Exempt:   ex,
				ExpiryAt: ex.expiryParsed,
			})
			continue
		}
		out = append(out, result{OSV: f.OSV, Module: f.Module, Verdict: verdictOK, Exempt: ex, ExpiryAt: ex.expiryParsed})
	}
	return out
}

// report prints a verdict summary to w and returns the desired exit code.
func report(results []result, w io.Writer) int {
	blocked := 0
	for _, r := range results {
		if r.Verdict == verdictBlocked {
			blocked++
		}
	}
	if len(results) == 0 {
		_, _ = fmt.Fprintln(w, "vuln-allowlist OK: no called vulnerabilities")
		return 0
	}
	if blocked == 0 {
		_, _ = fmt.Fprintf(w, "vuln-allowlist OK: %d called vuln(s) all covered by valid exemption(s)\n", len(results))
		for _, r := range results {
			_, _ = fmt.Fprintf(w, "  [exempt] %s @ %s — expires %s — %s (%s)\n",
				r.OSV, r.Module, r.Exempt.Expiry, r.Exempt.Reason, r.Exempt.SpecRef)
		}
		return 0
	}
	_, _ = fmt.Fprintf(w, "vuln-allowlist FAIL: %d of %d called vuln(s) blocked\n", blocked, len(results))
	for _, r := range results {
		switch r.Verdict {
		case verdictOK:
			_, _ = fmt.Fprintf(w, "  [exempt] %s @ %s — expires %s — %s\n",
				r.OSV, r.Module, r.Exempt.Expiry, r.Exempt.SpecRef)
		case verdictBlocked:
			if r.Exempt.OSVID != "" {
				_, _ = fmt.Fprintf(w, "  [BLOCK]  %s @ %s — %s (was exempted via %s; renew or fix)\n",
					r.OSV, r.Module, r.Reason, r.Exempt.SpecRef)
			} else {
				_, _ = fmt.Fprintf(w, "  [BLOCK]  %s @ %s — %s\n", r.OSV, r.Module, r.Reason)
			}
		}
	}
	_, _ = fmt.Fprintln(w, "  hint: edit tools/vuln-allowlist/allowlist.json to add/renew an exemption")
	_, _ = fmt.Fprintln(w, "        each exemption requires: osv_id, module, expiry (YYYY-MM-DD), reason, spec_ref")
	return 1
}

// run is the testable entry point.
func run(allowlistPath string, input io.Reader, w io.Writer, now time.Time) int {
	list, err := loadAllowlist(allowlistPath)
	if err != nil {
		_, _ = fmt.Fprintf(w, "vuln-allowlist: %v\n", err)
		return 2
	}
	called, err := uniqueCalledFindings(input)
	if err != nil {
		_, _ = fmt.Fprintf(w, "vuln-allowlist: %v\n", err)
		return 2
	}
	return report(classifyFindings(called, list, now), w)
}

func main() {
	allowlistPath := flag.String("allowlist", "tools/vuln-allowlist/allowlist.json",
		"path to OSV allowlist JSON file")
	flag.Parse()
	os.Exit(run(*allowlistPath, os.Stdin, os.Stderr, time.Now()))
}
