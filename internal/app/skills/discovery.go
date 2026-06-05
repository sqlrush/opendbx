// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// File discovery.go — Discover orchestrates the scan→read→parse→validate→
// resolve pipeline (spec-2.2 D-3). I/O lives here; all format/namespace logic
// is spec-2.1's pure functions.
//
// Order is load-bearing:
//   - size cap is checked from Lstat BEFORE os.ReadFile (no unbounded read).
//   - Validate warnings are collected even when Validate also returns a fatal
//     error (warnings are data; spec-2.1 Q4).
//   - disabled skills go to Ignored (visible), not silently dropped.
//   - Validate runs before Resolve (spec-2.1 Resolve precondition).

package skills

import (
	"io"
	"os"
)

// Discover scans every root, isolates per-file failures into Errors, and
// resolves the surviving skills by precedence. It never aborts on a bad file
// or an absent optional root.
func Discover(opts DiscoverOptions) DiscoveryResult {
	maxFiles := opts.MaxFilesPerRoot
	if maxFiles == 0 {
		maxFiles = defaultMaxFilesPerRoot
	}
	disabled := toSet(opts.Disabled)

	var valid []Skill
	var res DiscoveryResult

	for _, root := range opts.Roots {
		files, rerr := scanRoot(root, maxFiles)
		if rerr != nil {
			// ROOT_TOO_MANY_FILES still returns a truncated file list to process.
			res.Errors = append(res.Errors, DiscoveryError{Path: root.Dir, Err: asErrcode(rerr)})
		}
		for _, path := range files {
			sk, ferr := loadSkill(path, root)
			if ferr != nil {
				res.Errors = append(res.Errors, DiscoveryError{Path: path, Err: asErrcode(ferr)})
				continue
			}
			rep, verr := Validate(sk)
			for _, w := range rep.Warnings {
				res.Warnings = append(res.Warnings, DiscoveryWarning{Path: path, Name: sk.Schema.Name, Warning: w})
			}
			if verr != nil {
				res.Errors = append(res.Errors, DiscoveryError{Path: path, Err: asErrcode(verr)})
				continue
			}
			if disabled[sk.Key()] {
				res.Ignored = append(res.Ignored, IgnoredSkill{Skill: sk, Reason: "disabled"})
				continue
			}
			valid = append(valid, sk)
		}
	}

	r := Resolve(valid) // validate-before-resolve precondition satisfied
	res.Active = r.Active
	res.Shadowed = r.Shadowed
	res.Conflicts = r.Conflicts // NOT duplicated into Errors; provenance lives here
	return res
}

// loadSkill safely reads and parses one skill file (review HIGH: fs safety).
// Defense in depth against symlink/device targets and unbounded reads:
//   - Lstat + IsRegular rejects a symlink/FIFO/socket/device at the path before
//     it is opened (covers DT_UNKNOWN filesystems where the scan's d_type check
//     is unreliable).
//   - After Open, f.Stat + IsRegular re-checks (guards a TOCTOU swap-to-device).
//   - io.LimitReader bounds the read to maxSkillSize+1 so even a TOCTOU swap to
//     a huge regular file cannot OOM — it surfaces as SKILL.TOO_LARGE.
//
// Residual: a TOCTOU swap to a <1 MiB regular file outside the root could be
// read; closing that needs O_NOFOLLOW (non-portable) and is deferred. The scan
// is a one-shot over the user's own dirs, so the race window is negligible.
func loadSkill(path string, root SkillRoot) (Skill, error) {
	info, lerr := os.Lstat(path)
	if lerr != nil {
		return Skill{}, fileUnreadable(lerr)
	}
	if !info.Mode().IsRegular() {
		return Skill{}, notRegularErr(path)
	}
	f, oerr := os.Open(path) //nolint:gosec // spec-2.2 D-3: path is a scanned skill file; Lstat-IsRegular above + f.Stat below + LimitReader bound the read.
	if oerr != nil {
		return Skill{}, fileUnreadable(oerr)
	}
	defer func() { _ = f.Close() }()

	fi, serr := f.Stat()
	if serr != nil {
		return Skill{}, fileUnreadable(serr)
	}
	if !fi.Mode().IsRegular() {
		return Skill{}, notRegularErr(path)
	}
	if fi.Size() > maxSkillSize {
		return Skill{}, tooLargeErr(fi.Size())
	}

	content, rerr := io.ReadAll(io.LimitReader(f, maxSkillSize+1))
	if rerr != nil {
		return Skill{}, fileUnreadable(rerr)
	}
	if int64(len(content)) > maxSkillSize {
		return Skill{}, tooLargeErr(int64(len(content)))
	}

	src := SkillSource{Kind: root.Kind, Precedence: root.Precedence, Path: path, PluginID: root.PluginID}
	return Parse(content, src)
}

// toSet builds a lookup set from a slice (nil-safe).
func toSet(items []string) map[string]bool {
	if len(items) == 0 {
		return nil
	}
	m := make(map[string]bool, len(items))
	for _, it := range items {
		m[it] = true
	}
	return m
}
