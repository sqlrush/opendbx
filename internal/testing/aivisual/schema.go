// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package aivisual

// Verdict literals. Returned by the AI evaluator after analyzing the
// snapshot. "uncertain" is the fallback when JSON parsing fails or
// the model returns ambiguous output.
const (
	VerdictOK           = "ok"
	VerdictIssuesFound  = "issues-found"
	VerdictUncertain    = "uncertain"
)

// Severity literals.
const (
	SeverityHigh   = "high"
	SeverityMedium = "medium"
	SeverityLow    = "low"
)

// Category literals — the 6 fixed evaluation dimensions per
// spec § D-3 prompt checklist.
const (
	CategoryAlignment = "alignment" // table column / row alignment
	CategoryBorder    = "border"    // box border completeness
	CategoryColor     = "color"     // color contrast / ANSI fidelity
	CategoryCJKWidth  = "cjk-width" // CJK / wide-character width
	CategoryIndent    = "indent"    // indentation consistency
	CategoryANSI      = "ansi"      // ANSI residue / unrendered escapes
)

// Report is the structured output from the AI reviewer.
type Report struct {
	Verdict string  `json:"verdict"`
	Issues  []Issue `json:"issues"`
	Tokens  int     `json:"tokens,omitempty"`
}

// Issue describes a single problem the AI found.
type Issue struct {
	Severity string `json:"severity"`
	Category string `json:"category"`
	Message  string `json:"message"`
	Region   *Box   `json:"region,omitempty"`
}

// Box is an optional bounding box highlighting the issue region in
// the source PNG (pixel coords from top-left).
type Box struct {
	X int `json:"x"`
	Y int `json:"y"`
	W int `json:"w"`
	H int `json:"h"`
}
