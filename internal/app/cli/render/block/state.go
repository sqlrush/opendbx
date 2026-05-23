// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// File state.go — ToolUseState visual indicator placeholder (spec-1.9
// D-7). R2.1.3 MED-2: fixture-derived, no pre-capture numerical lock.
// CC source uses BLACK_CIRCLE / ToolUseLoader / MessageResponse
// composition rather than a fixed glyph table; opendbx TUI needs
// **some** visual prefix for state discrimination; the rune + style
// chosen here are PLACEHOLDER until spec-1.9 T-2.5 CC fixture lands.

package block

// Indicator carries the visual prefix for a ToolUseState row. Style
// is StyleNormal/Dimmed/Warning per the spec § 1.4 cite of CC
// AssistantToolUseMessage.tsx; the rune is currently a placeholder
// per R2.1.3 MED-2 (CC has no fixed glyph table — uses composition).
type Indicator struct {
	Rune  rune
	Style StyleKind
}

// stateIndicator maps ToolUseState → visual Indicator.
//
// **PLACEHOLDER per spec-1.9 R2.1.3 MED-2**: rune values below are
// chosen for opendbx TUI legibility; spec-1.9 T-2.5 CC fixture
// capture will lock the canonical form (or replace with text-only).
//
// Mapping rationale (CC source cited):
//   - Queued '⏸' + StyleDimmed     — CC: not rendered (text-only via
//     QueuedRenderer "Waiting…"); opendbx adds glyph for TUI legibility
//   - Running '⏵' + StyleNormal    — CC: progress text via
//     ProgressRenderer; opendbx glyph for state distinction
//   - WaitingPermission '⏷' + StyleWarning — CC: dim "Waiting for
//     permission…" second row (AssistantToolUseMessage.tsx:240)
//   - Resolved '⏺' + StyleNormal   — CC: final header rendered
//   - Error '✗' + StyleWarning     — CC: erroredToolUseIDs path
func stateIndicator(s ToolUseState) Indicator {
	switch s {
	case StateQueued:
		return Indicator{Rune: '⏸', Style: StyleDimmed}
	case StateRunning:
		return Indicator{Rune: '⏵', Style: StyleNormal}
	case StateWaitingPermission:
		return Indicator{Rune: '⏷', Style: StyleWarning}
	case StateResolved:
		return Indicator{Rune: '⏺', Style: StyleNormal}
	case StateError:
		return Indicator{Rune: '✗', Style: StyleWarning}
	}
	// Unknown state: dim '?' (defensive; should never trigger in
	// production since caller controls State enum).
	return Indicator{Rune: '?', Style: StyleDimmed}
}

// resultIndicator maps ToolResultState → visual Indicator (spec-1.9b D-5).
//
// **PLACEHOLDER per spec-1.9b R2 absorbing spec-1.9 R2.1.3 MED-2**: rune
// values are chosen for opendbx TUI legibility; spec-1.9b T-9 R3 CC
// fixture capture will lock the canonical form.
//
// Mapping rationale:
//   - Success '✓' Normal        — generic check mark
//   - Error '✗' Warning         — generic error glyph
//   - Rejected '⊘' Warning      — rejected / forbidden
//   - Canceled '⏹' Dimmed       — stop / interrupted
func resultIndicator(s ToolResultState) Indicator {
	switch s {
	case ResultSuccess:
		return Indicator{Rune: '✓', Style: StyleNormal}
	case ResultError:
		return Indicator{Rune: '✗', Style: StyleWarning}
	case ResultRejected:
		return Indicator{Rune: '⊘', Style: StyleWarning}
	case ResultCanceled:
		return Indicator{Rune: '⏹', Style: StyleDimmed}
	}
	return Indicator{Rune: '?', Style: StyleDimmed}
}
