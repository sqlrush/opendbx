// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// Version tag grammar + parser. spec-0.7 D-1.
//
// Grammar (CLAUDE.md 规则 15):
//
//		v<MAJOR>.<MINOR>.<PATCH>-stage<S>.<N>[-accepted]
//
//	  - MAJOR=0 until commercial 1.0
//	  - MINOR = global cumulative spec ordinal (spec-registry SSOT — NOT a
//	    formula from spec id; manifest authoritative; spec-0.7 R2.1 Q13 ★B')
//	  - PATCH=0 for spec FROZEN tag; >0 for hotfix bump
//	  - S = stage number (0~9 anticipated)
//	  - N = spec ordinal within stage (matches spec-registry ordinal column)
//	  - "-accepted" marks Stage acceptance tags
//
// Examples:
//
//	v0.1.0-stage0.1               spec-0.1-repo-bootstrap
//	v0.7.0-stage0.7               spec-0.7-version-numbering (this spec)
//	v0.19.0-stage0.19-accepted    spec-0.16-stage0-acceptance (Stage 0 验收;
//	                              spec-0.16 ordinal=19 含子规格全计 Q13 ★B')
//	v0.20.0-stage1.1              spec-1.1 (Stage 1 第一个; ordinal=20 =
//	                              Stage 0 19 ordinal + 1)
package version

import (
	"regexp"
	"strconv"

	"github.com/sqlrush/opendbx/internal/platform/errcode"
)

// VersionPattern is the canonical opendbx version tag pattern.
//
// Capture groups: 1=MAJOR, 2=MINOR, 3=PATCH, 4=Stage, 5=SpecN,
// 6="-accepted" suffix (optional).
//
// Exposed as a string constant (not the compiled regexp) so callers cannot
// mutate shared state — codex LOW (R2). The compiled value lives in the
// unexported versionRegex variable below.
const VersionPattern = `^v(\d+)\.(\d+)\.(\d+)-stage(\d+)\.(\d+)(-accepted)?$`

//nolint:gochecknoglobals // compile-once regex for hot-path Parse calls.
var versionRegex = regexp.MustCompile(VersionPattern)

// MaxStage is the upper bound on the Stage segment. Per the roadmap, stage
// 0~9 is the anticipated range through commercial 1.0; values above that
// fail semantic validation in Parse to prevent typos like "stage99.999".
const MaxStage = 9

// Info is the parsed representation of a version tag.
type Info struct {
	Major    int
	Minor    int
	Patch    int
	Stage    int
	SpecN    int
	Accepted bool
	Raw      string
}

// String reformats Info back to the canonical tag form. Round-trip stable:
// Parse(Info.String()).Equals(Info) holds for any valid Info.
func (i Info) String() string {
	out := "v" + strconv.Itoa(i.Major) + "." + strconv.Itoa(i.Minor) + "." + strconv.Itoa(i.Patch) +
		"-stage" + strconv.Itoa(i.Stage) + "." + strconv.Itoa(i.SpecN)
	if i.Accepted {
		out += "-accepted"
	}
	return out
}

// LookupFunc resolves a (stage, specN) pair to the global spec-registry
// ordinal. Returned ok=false signals "no such spec in registry".
//
// Parse takes a LookupFunc rather than reading the registry file directly
// so it stays pure / has no I/O dependency at call time. T-10 (D-7) wires
// the production lookup; tests inject fakes (see grammar_test.go).
type LookupFunc func(stage, specN int) (ordinal int, ok bool)

// Parse validates a tag string against VersionPattern (grammar) and then
// against the spec-registry (semantic: MINOR == ordinal; Stage <= MaxStage).
//
// Returns ErrTagInvalid via errcode.Newf on any failure. The returned Info
// is the zero value on error.
//
// If lookup is nil, semantic validation against the registry is skipped and
// only the syntactic + Stage <= MaxStage checks run. This allows Parse to
// be used from contexts where the registry is not yet available (e.g.,
// early bootstrap, or tests that focus on grammar alone).
func Parse(tag string, lookup LookupFunc) (Info, error) {
	m := versionRegex.FindStringSubmatch(tag)
	if m == nil {
		return Info{}, errTagInvalid(tag, "does not match VersionPattern")
	}

	// Capture groups are all \d+ except group 6 (literal "-accepted"); the
	// regex guarantees Atoi succeeds — checking error is paranoia for the
	// case where the regex is mutated in future.
	major, err := strconv.Atoi(m[1])
	if err != nil {
		return Info{}, errTagInvalid(tag, "MAJOR not integer")
	}
	minor, err := strconv.Atoi(m[2])
	if err != nil {
		return Info{}, errTagInvalid(tag, "MINOR not integer")
	}
	patch, err := strconv.Atoi(m[3])
	if err != nil {
		return Info{}, errTagInvalid(tag, "PATCH not integer")
	}
	stage, err := strconv.Atoi(m[4])
	if err != nil {
		return Info{}, errTagInvalid(tag, "Stage not integer")
	}
	specN, err := strconv.Atoi(m[5])
	if err != nil {
		return Info{}, errTagInvalid(tag, "SpecN not integer")
	}

	info := Info{
		Major:    major,
		Minor:    minor,
		Patch:    patch,
		Stage:    stage,
		SpecN:    specN,
		Accepted: m[6] == "-accepted",
		Raw:      tag,
	}

	// Semantic: Stage upper bound (catches "stage99.999" typos).
	if info.Stage > MaxStage {
		return Info{}, errTagInvalid(tag, "Stage "+strconv.Itoa(info.Stage)+" exceeds MaxStage "+strconv.Itoa(MaxStage))
	}

	// Semantic: MINOR == spec-registry ordinal for (Stage, SpecN).
	// Skipped when lookup is nil (grammar-only mode).
	if lookup != nil {
		ordinal, ok := lookup(info.Stage, info.SpecN)
		if !ok {
			return Info{}, errTagInvalid(tag, "(stage="+strconv.Itoa(info.Stage)+", specN="+strconv.Itoa(info.SpecN)+") not in spec-registry")
		}
		if info.Minor != ordinal {
			return Info{}, errTagInvalid(tag, "MINOR "+strconv.Itoa(info.Minor)+" must equal spec-registry ordinal "+strconv.Itoa(ordinal)+" for (stage="+strconv.Itoa(info.Stage)+", specN="+strconv.Itoa(info.SpecN)+")")
		}
		// SpecN itself must equal ordinal — spec-0.7 § 2.1 grammar comment:
		// "N = spec ordinal within stage (matches spec-registry ordinal)".
		if info.SpecN != ordinal {
			return Info{}, errTagInvalid(tag, "SpecN "+strconv.Itoa(info.SpecN)+" must equal spec-registry ordinal "+strconv.Itoa(ordinal))
		}
	}

	return info, nil
}

// errTagInvalid is a small helper to construct the canonical ErrTagInvalid
// with the caller-supplied reason appended to the message. Used only inside
// Parse — callers should errors.Is against ErrTagInvalid, not match the
// reason text (the reason is for human diagnostics only).
func errTagInvalid(tag, reason string) error {
	return errcode.Newf(ErrTagInvalid.Code(), "version tag %q invalid: %s", tag, reason)
}
