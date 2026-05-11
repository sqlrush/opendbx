// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package version

import (
	"errors"
	"strings"
	"testing"
)

// fakeRegistry mimics the production spec-registry lookup (D-7) for unit
// tests. The data here mirrors spec-0.7 § 2.6 sample table — Stage 0 has
// 19 ordinals (16 main + 1 sub + 3 survey + acceptance overlay); Stage 1
// starts at ordinal 20.
//
// Map shape: (stage, specN) → ordinal.
var fakeRegistry = map[[2]int]int{
	// Stage 0 — first 6 specs are co-incidence MINOR=ordinal=specN.
	{0, 1}:  1,
	{0, 2}:  2,
	{0, 3}:  3,
	{0, 4}:  4,
	{0, 5}:  5,
	{0, 6}:  6,
	{0, 7}:  7, // spec-0.7 — this spec
	{0, 8}:  8,
	{0, 9}:  9,
	{0, 10}: 10,
	{0, 11}: 11,
	{0, 12}: 12, // spec-0.11.5 sub — N=12 not 11.5 (registry is SSOT, integer)
	{0, 13}: 13, // spec-0.12
	{0, 14}: 14, // spec-0.13
	{0, 15}: 15, // spec-0.14
	{0, 16}: 16, // spec-0.15a survey (tag_policy=skip but ordinal occupied)
	{0, 17}: 17, // spec-0.15b survey
	{0, 18}: 18, // spec-0.15c survey
	{0, 19}: 19, // spec-0.16 acceptance
	// Stage 1 — ordinal continues from 20.
	{1, 20}: 20, // spec-1.1
}

func fakeLookup(stage, specN int) (int, bool) {
	o, ok := fakeRegistry[[2]int{stage, specN}]
	return o, ok
}

// --- Valid fixtures (≥ 6 required per spec § 4) -------------------------

func TestParseValidFixtures(t *testing.T) {
	t.Parallel()
	cases := []struct {
		tag  string
		want Info
	}{
		{
			tag:  "v0.1.0-stage0.1",
			want: Info{Major: 0, Minor: 1, Patch: 0, Stage: 0, SpecN: 1, Accepted: false, Raw: "v0.1.0-stage0.1"},
		},
		{
			tag:  "v0.6.0-stage0.6",
			want: Info{Major: 0, Minor: 6, Patch: 0, Stage: 0, SpecN: 6, Accepted: false, Raw: "v0.6.0-stage0.6"},
		},
		{
			tag:  "v0.7.0-stage0.7", // this spec
			want: Info{Major: 0, Minor: 7, Patch: 0, Stage: 0, SpecN: 7, Accepted: false, Raw: "v0.7.0-stage0.7"},
		},
		{
			tag:  "v0.19.0-stage0.19-accepted", // spec-0.16 stage 0 acceptance
			want: Info{Major: 0, Minor: 19, Patch: 0, Stage: 0, SpecN: 19, Accepted: true, Raw: "v0.19.0-stage0.19-accepted"},
		},
		{
			tag:  "v0.20.0-stage1.20", // spec-1.1 ordinal = 20 (Stage 0 19 ordinal + 1)
			want: Info{Major: 0, Minor: 20, Patch: 0, Stage: 1, SpecN: 20, Accepted: false, Raw: "v0.20.0-stage1.20"},
		},
		{
			tag:  "v0.7.1-stage0.7", // PATCH bump for spec-0.7 hotfix
			want: Info{Major: 0, Minor: 7, Patch: 1, Stage: 0, SpecN: 7, Accepted: false, Raw: "v0.7.1-stage0.7"},
		},
	}
	for _, c := range cases {
		c := c
		t.Run(c.tag, func(t *testing.T) {
			t.Parallel()
			got, err := Parse(c.tag, fakeLookup)
			if err != nil {
				t.Fatalf("Parse(%q) returned error: %v", c.tag, err)
			}
			if got != c.want {
				t.Errorf("Parse(%q) = %+v, want %+v", c.tag, got, c.want)
			}
		})
	}
}

// --- Round-trip (Info.String) ------------------------------------------

func TestInfoStringRoundTrip(t *testing.T) {
	t.Parallel()
	tags := []string{
		"v0.1.0-stage0.1",
		"v0.7.0-stage0.7",
		"v0.7.1-stage0.7",
		"v0.19.0-stage0.19-accepted",
		"v0.20.0-stage1.20",
	}
	for _, tag := range tags {
		tag := tag
		t.Run(tag, func(t *testing.T) {
			t.Parallel()
			info, err := Parse(tag, fakeLookup)
			if err != nil {
				t.Fatalf("Parse(%q): %v", tag, err)
			}
			if got := info.String(); got != tag {
				t.Errorf("round-trip: Parse(%q).String() = %q, want %q", tag, got, tag)
			}
		})
	}
}

// --- Invalid fixtures (≥ 10 required per spec § 4) ----------------------

func TestParseInvalidFixtures(t *testing.T) {
	t.Parallel()
	// Each case: tag string + a substring expected in the error message
	// (for human-diagnostic verification; callers should errors.Is against
	// ErrTagInvalid, not match this text).
	cases := []struct {
		name    string
		tag     string
		wantMsg string
	}{
		// 1. typo: missing 'v' prefix
		{"missing-v", "0.7.0-stage0.7", "VersionPattern"},
		// 2. too many MAJOR.MINOR.PATCH segments
		{"extra-segment", "v0.7.0.0-stage0.7", "VersionPattern"},
		// 3. uppercase 'V'
		{"uppercase-v", "V0.7.0-stage0.7", "VersionPattern"},
		// 4. empty string
		{"empty", "", "VersionPattern"},
		// 5. whitespace in tag
		{"has-space", "v0.7.0-stage0.7 ", "VersionPattern"},
		// 6. -rc pre-release suffix (spec § 1.2 ❌-5)
		{"rc-suffix", "v0.7.0-stage0.7-rc.1", "VersionPattern"},
		// 7. -accepted with extra suffix
		{"accepted-extra", "v0.19.0-stage0.19-accepted-extra", "VersionPattern"},
		// 8. R2 claude HIGH-1: MINOR-S mismatch — MINOR=7 but registry says
		//    (stage=3, specN=7) is not registered.
		{"minor-stage-mismatch", "v0.7.0-stage3.7", "not in spec-registry"},
		// 9. CRIT-1 negative: acceptance tag missing dot-N
		{"accepted-missing-dotN", "v0.16.0-stage0-accepted", "VersionPattern"},
		// 10. R-4: Stage exceeds MaxStage (catches typos like stage99.999)
		{"stage-out-of-range", "v0.7.0-stage99.7", "exceeds MaxStage"},
		// 11. Unknown (stage, specN) — registry miss
		{"unknown-spec", "v0.99.0-stage0.99", "not in spec-registry"},
		// 12. MINOR != registry ordinal (e.g. typo: spec-0.7 → MINOR=8)
		{"minor-not-ordinal", "v0.8.0-stage0.7", "must equal spec-registry ordinal"},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			_, err := Parse(c.tag, fakeLookup)
			if err == nil {
				t.Fatalf("Parse(%q) expected error, got nil", c.tag)
			}
			if !errors.Is(err, ErrTagInvalid) {
				t.Errorf("Parse(%q) error not errors.Is(ErrTagInvalid): %v", c.tag, err)
			}
			if !strings.Contains(err.Error(), c.wantMsg) {
				t.Errorf("Parse(%q) error message %q does not contain %q", c.tag, err.Error(), c.wantMsg)
			}
		})
	}
}

// --- Nil lookup: grammar-only mode -------------------------------------

func TestParseNilLookupSkipsSemantic(t *testing.T) {
	t.Parallel()
	// With nil lookup, MINOR-vs-ordinal check is skipped. A tag that would
	// fail semantic validation (MINOR=8 for spec-0.7) still parses.
	tag := "v0.8.0-stage0.7"
	info, err := Parse(tag, nil)
	if err != nil {
		t.Fatalf("Parse(%q, nil) = %v, want nil", tag, err)
	}
	if info.Minor != 8 || info.SpecN != 7 {
		t.Errorf("Parse(%q, nil) = %+v, want Minor=8 SpecN=7", tag, info)
	}
}

func TestParseNilLookupStillEnforcesStageBound(t *testing.T) {
	t.Parallel()
	// Stage <= MaxStage is checked even in grammar-only mode (it's the only
	// invariant that does not depend on registry data).
	_, err := Parse("v0.7.0-stage99.7", nil)
	if err == nil {
		t.Fatal("Parse with stage>MaxStage should fail even with nil lookup")
	}
	if !errors.Is(err, ErrTagInvalid) {
		t.Errorf("error not errors.Is(ErrTagInvalid): %v", err)
	}
}

// --- VersionPattern exported as string constant (codex LOW) -------------

func TestVersionPatternIsString(t *testing.T) {
	t.Parallel()
	// Compile-time guarantee: VersionPattern is a `const string`, not a
	// *regexp.Regexp. Callers cannot mutate it.
	var _ string = VersionPattern
	if VersionPattern == "" {
		t.Error("VersionPattern must not be empty")
	}
	if !strings.HasPrefix(VersionPattern, "^v") {
		t.Errorf("VersionPattern must start with ^v, got %q", VersionPattern)
	}
}
