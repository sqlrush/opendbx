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

import "os"

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

// loadSkill stat-checks the size cap, reads the file, and parses it. Returns an
// errcode error on stat/size/read/parse failure.
func loadSkill(path string, root SkillRoot) (Skill, error) {
	info, lerr := os.Lstat(path)
	if lerr != nil {
		return Skill{}, fileUnreadable(lerr)
	}
	if info.Size() > maxSkillSize {
		return Skill{}, tooLargeErr(info.Size()) // size cap BEFORE read
	}
	content, ferr := os.ReadFile(path) //nolint:gosec // spec-2.2 D-3: path is a scanned, non-symlink, size-capped (stat-before-read) skill file
	if ferr != nil {
		return Skill{}, fileUnreadable(ferr)
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
