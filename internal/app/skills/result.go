// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// File result.go — discovery result + error/warning/ignored shapes + error
// classification (spec-2.2 D-4/D-5).
//
// Membership contract (DoD): every valid skill name appears in EXACTLY one of
//   - Active   (one winner), or
//   - Conflicts with Kind==ConflictUnresolvable (no winner).
// ConflictShadowed losers go to Shadowed; the winner is in Active. Conflicts
// are NOT duplicated into Errors — they carry full provenance (Source.Path of
// every shadowed file) in res.Conflicts (review HIGH/CRIT).

package skills

import (
	"errors"

	"github.com/sqlrush/opendbx/internal/platform/errcode"
)

// Sentinel causes for ROOT_UNREADABLE, carried as the PathError root cause so
// the wrapped message is descriptive.
var (
	errSymlinkRoot = errors.New("skill root is a symlink (not followed)")
	errNotDir      = errors.New("skill root is not a directory")
)

// DiscoveryError is one filesystem/parse/validate failure, isolated so the
// rest of discovery proceeds. Err is always normalized to an errcode.Error
// (asErrcode) so ClassifyError can read its Code.
type DiscoveryError struct {
	Path string
	Err  error
}

// DiscoveryWarning carries a non-fatal spec-2.1 Validate warning with the file
// it came from. Collected even when the same file also has a fatal error.
type DiscoveryWarning struct {
	Path    string
	Name    string
	Warning Warning
}

// IgnoredSkill records a skill excluded by configuration (disabled), kept
// visible rather than silently dropped (规则 7).
type IgnoredSkill struct {
	Skill  Skill
	Reason string
}

// DiscoveryResult is the outcome of Discover. All slices are nil when empty.
type DiscoveryResult struct {
	Active    []Skill
	Shadowed  []Skill
	Conflicts []Conflict
	Warnings  []DiscoveryWarning
	Ignored   []IgnoredSkill
	Errors    []DiscoveryError
}

// ClassifyError returns the errcode Code of a DiscoveryError (e.g.
// "SKILL.PARSE_ERROR"), or "" if the error is not an errcode.Error. Every
// DiscoveryError.Err is normalized via asErrcode, so this returns a non-empty
// code for every collected error.
func ClassifyError(de DiscoveryError) string {
	var ec errcode.Error
	if errors.As(de.Err, &ec) {
		return ec.Code()
	}
	return ""
}

// asErrcode normalizes any error into an errcode.Error so downstream
// classification never sees a bare error. Errors already carrying an errcode
// (Parse/Validate/Conflict.Err and the discovery sentinels) pass through;
// anything else is wrapped under SKILL.FILE_UNREADABLE as a conservative
// default.
func asErrcode(err error) error {
	if err == nil {
		return nil
	}
	var ec errcode.Error
	if errors.As(err, &ec) {
		return err
	}
	return errcode.Wrap(ErrFileUnreadable.Code(), err, "", "")
}

// fileUnreadable wraps an os stat/read error under SKILL.FILE_UNREADABLE,
// preserving the OS root cause for errors.Is/As.
func fileUnreadable(err error) error {
	return errcode.Wrap(ErrFileUnreadable.Code(), err, "", "")
}

// rootUnreadable wraps an os error under SKILL.ROOT_UNREADABLE.
func rootUnreadable(err error) error {
	return errcode.Wrap(ErrRootUnreadable.Code(), err, "", "")
}

// tooLargeErr reports a file that exceeds the size cap without reading it.
func tooLargeErr(size int64) error {
	return errcode.Newf(ErrTooLarge.Code(),
		"SKILL.md is %d bytes, exceeds the %d byte limit", size, maxSkillSize)
}

// errcodeTooManyFiles reports a root whose file count exceeded the cap.
func errcodeTooManyFiles(dir string, found, cap int) error {
	return errcode.Newf(ErrRootTooManyFiles.Code(),
		"root %q has %d files, exceeds the %d limit; extra files skipped", dir, found, cap)
}
