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
}

// loadAllowlist reads and validates the allowlist file.
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
		if e.Expiry == "" {
			return nil, fmt.Errorf("exemption[%d] %s: expiry required (YYYY-MM-DD)", i, e.OSVID)
		}
		if e.SpecRef == "" {
			return nil, fmt.Errorf("exemption[%d] %s: spec_ref required (no anonymous exemptions)", i, e.OSVID)
		}
		if _, err := time.Parse("2006-01-02", e.Expiry); err != nil {
			return nil, fmt.Errorf("exemption[%d] %s: invalid expiry %q (want YYYY-MM-DD)", i, e.OSVID, e.Expiry)
		}
		if _, dup := out[e.OSVID]; dup {
			return nil, fmt.Errorf("exemption[%d] %s: duplicate OSV ID", i, e.OSVID)
		}
		out[e.OSVID] = e
	}
	return out, nil
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

// uniqueCalledFindings reads the govulncheck JSON stream and returns
// each called-vuln OSV ID seen, in stable order. Govulncheck emits one
// finding per trace; we collapse duplicates per OSV.
func uniqueCalledFindings(r io.Reader) ([]string, error) {
	dec := json.NewDecoder(r)
	seen := make(map[string]bool)
	var order []string
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
		if seen[msg.Finding.OSV] {
			continue
		}
		seen[msg.Finding.OSV] = true
		order = append(order, msg.Finding.OSV)
	}
	sort.Strings(order) // determinism
	return order, nil
}

// classify pairs each called finding with its allowlist verdict.
type verdict int

const (
	verdictOK      verdict = iota // exempted, non-expired
	verdictBlocked                // no exemption / expired exemption
)

type result struct {
	OSV      string
	Verdict  verdict
	Reason   string // populated when blocked
	Exempt   exemption
	ExpiryAt time.Time
}

// classifyFindings cross-references called findings against the allowlist
// using `now` as today.
func classifyFindings(called []string, list map[string]exemption, now time.Time) []result {
	out := make([]result, 0, len(called))
	for _, osv := range called {
		ex, ok := list[osv]
		if !ok {
			out = append(out, result{OSV: osv, Verdict: verdictBlocked, Reason: "no exemption in allowlist"})
			continue
		}
		expiry, _ := time.Parse("2006-01-02", ex.Expiry)
		if now.After(expiry) {
			out = append(out, result{
				OSV: osv, Verdict: verdictBlocked,
				Reason:   fmt.Sprintf("exemption expired on %s", ex.Expiry),
				Exempt:   ex,
				ExpiryAt: expiry,
			})
			continue
		}
		out = append(out, result{OSV: osv, Verdict: verdictOK, Exempt: ex, ExpiryAt: expiry})
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
			_, _ = fmt.Fprintf(w, "  [exempt] %s — expires %s — %s (%s)\n",
				r.OSV, r.Exempt.Expiry, r.Exempt.Reason, r.Exempt.SpecRef)
		}
		return 0
	}
	_, _ = fmt.Fprintf(w, "vuln-allowlist FAIL: %d of %d called vuln(s) blocked\n", blocked, len(results))
	for _, r := range results {
		switch r.Verdict {
		case verdictOK:
			_, _ = fmt.Fprintf(w, "  [exempt] %s — expires %s — %s\n", r.OSV, r.Exempt.Expiry, r.Exempt.SpecRef)
		case verdictBlocked:
			if r.Exempt.OSVID != "" {
				_, _ = fmt.Fprintf(w, "  [BLOCK]  %s — %s (was exempted via %s; renew or fix)\n",
					r.OSV, r.Reason, r.Exempt.SpecRef)
			} else {
				_, _ = fmt.Fprintf(w, "  [BLOCK]  %s — %s\n", r.OSV, r.Reason)
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
