// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package skills

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// twoRoots returns a low- and high-precedence root over two temp dirs.
func twoRoots(t *testing.T) (lowDir, highDir string, opts DiscoverOptions) {
	t.Helper()
	lowDir, highDir = t.TempDir(), t.TempDir()
	opts = DiscoverOptions{Roots: []SkillRoot{
		{Kind: SourceUserGlobal, Precedence: 1, Dir: lowDir},
		{Kind: SourceProject, Precedence: 2, Dir: highDir},
	}}
	return
}

func TestDiscover_Empty(t *testing.T) {
	t.Parallel()
	res := Discover(DiscoverOptions{})
	if len(res.Active) != 0 || len(res.Errors) != 0 {
		t.Errorf("empty options should yield empty result: %+v", res)
	}
}

func TestDiscover_ShadowByPrecedence(t *testing.T) {
	t.Parallel()
	low, high, opts := twoRoots(t)
	writeSkill(t, filepath.Join(low, "top.md"), "top-sql")
	writeSkill(t, filepath.Join(high, "top.md"), "top-sql")
	res := Discover(opts)
	if len(res.Active) != 1 || res.Active[0].Source.Precedence != 2 {
		t.Fatalf("higher precedence should win: %+v", res.Active)
	}
	if len(res.Shadowed) != 1 || res.Shadowed[0].Source.Precedence != 1 {
		t.Fatalf("lower precedence shadowed: %+v", res.Shadowed)
	}
	// Conflicts carry provenance; they are NOT duplicated into Errors.
	if len(res.Errors) != 0 {
		t.Errorf("shadow is not an error: %+v", res.Errors)
	}
}

// TestDiscover_BadFileIsolation — each bad-file class is isolated to Errors
// while a good skill in the same scan still becomes Active.
func TestDiscover_BadFileIsolation(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeSkill(t, filepath.Join(dir, "good.md"), "good")
	// bad YAML
	_ = os.WriteFile(filepath.Join(dir, "badyaml.md"), []byte("---\nname: : :\n---\n"), 0o644)
	// missing required field (no description) → validate failure
	_ = os.WriteFile(filepath.Join(dir, "noval.md"), []byte("---\nname: noval\n---\nbody\n"), 0o644)
	// no frontmatter
	_ = os.WriteFile(filepath.Join(dir, "nofm.md"), []byte("# just markdown\n"), 0o644)

	res := Discover(DiscoverOptions{Roots: []SkillRoot{root(dir)}})

	if len(res.Active) != 1 || res.Active[0].Schema.Name != "good" {
		t.Fatalf("good skill should survive isolation: %+v", res.Active)
	}
	codes := map[string]bool{}
	for _, de := range res.Errors {
		codes[ClassifyError(de)] = true
	}
	for _, want := range []string{ErrParseError.Code(), ErrValidationFailed.Code(), ErrNoFrontmatter.Code()} {
		if !codes[want] {
			t.Errorf("missing isolated error %s; got %v", want, codes)
		}
	}
}

// TestDiscover_SizeCapBeforeRead — an over-limit file is rejected via stat,
// never fully read (review HIGH: size cap before ReadFile).
func TestDiscover_SizeCapBeforeRead(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	big := filepath.Join(dir, "big.md")
	if err := os.WriteFile(big, make([]byte, maxSkillSize+1), 0o644); err != nil {
		t.Fatal(err)
	}
	writeSkill(t, filepath.Join(dir, "ok.md"), "ok")
	res := Discover(DiscoverOptions{Roots: []SkillRoot{root(dir)}})
	if len(res.Active) != 1 {
		t.Fatalf("ok skill should survive: %+v", res.Active)
	}
	found := false
	for _, de := range res.Errors {
		if ClassifyError(de) == ErrTooLarge.Code() {
			found = true
		}
	}
	if !found {
		t.Errorf("over-limit file should produce TOO_LARGE: %+v", res.Errors)
	}
}

// TestDiscover_WarningsCollected — non-fatal warnings (non-kebab name, unknown
// field) surface in DiscoveryResult.Warnings, not dropped.
func TestDiscover_WarningsCollected(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	content := "---\nname: Not_Kebab\ndescription: d\nopendbx_extra: hi\n---\nbody\n"
	_ = os.WriteFile(filepath.Join(dir, "w.md"), []byte(content), 0o644)
	res := Discover(DiscoverOptions{Roots: []SkillRoot{root(dir)}})
	if len(res.Active) != 1 {
		t.Fatalf("valid-but-warned skill should be active: %+v", res.Active)
	}
	if len(res.Warnings) < 2 {
		t.Errorf("expected non-kebab + unknown-field warnings, got %+v", res.Warnings)
	}
}

func TestDiscover_DisabledToIgnored(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeSkill(t, filepath.Join(dir, "a.md"), "keep")
	writeSkill(t, filepath.Join(dir, "b.md"), "drop")
	res := Discover(DiscoverOptions{Roots: []SkillRoot{root(dir)}, Disabled: []string{"drop"}})
	if len(res.Active) != 1 || res.Active[0].Schema.Name != "keep" {
		t.Fatalf("disabled skill must not be active: %+v", res.Active)
	}
	if len(res.Ignored) != 1 || res.Ignored[0].Skill.Schema.Name != "drop" {
		t.Errorf("disabled skill should be visible in Ignored: %+v", res.Ignored)
	}
}

// TestDiscover_MembershipContract — every valid name is Active XOR
// unresolvable; no name is both.
func TestDiscover_MembershipContract(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	// two files, same name, SAME root → Unresolvable
	_ = os.WriteFile(filepath.Join(dir, "x1.md"), []byte("---\nname: dup\ndescription: d\n---\nb\n"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "x2.md"), []byte("---\nname: dup\ndescription: d\n---\nb\n"), 0o644)
	writeSkill(t, filepath.Join(dir, "solo.md"), "solo")
	res := Discover(DiscoverOptions{Roots: []SkillRoot{root(dir)}})

	active := map[string]bool{}
	for _, s := range res.Active {
		active[s.Schema.Name] = true
	}
	unresolvable := map[string]bool{}
	for _, c := range res.Conflicts {
		if c.Kind == ConflictUnresolvable {
			unresolvable[c.Name] = true
		}
	}
	if !active["solo"] || active["dup"] {
		t.Errorf("solo active, dup not active: active=%v", active)
	}
	if !unresolvable["dup"] {
		t.Errorf("dup should be unresolvable: %v", unresolvable)
	}
	if active["dup"] && unresolvable["dup"] {
		t.Error("dup is both active and unresolvable (contract violation)")
	}
}

// TestDiscover_ExtraRoundTrip — discovery preserves Schema.Extra byte-faithful
// through Parse + Resolve (2.1 deep-copy isolation).
func TestDiscover_ExtraRoundTrip(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	content := "---\nname: e\ndescription: d\nopendbx_databases:\n  - postgres\n  - mysql\n---\nbody\n"
	_ = os.WriteFile(filepath.Join(dir, "e.md"), []byte(content), 0o644)
	res := Discover(DiscoverOptions{Roots: []SkillRoot{root(dir)}})
	if len(res.Active) != 1 {
		t.Fatalf("want 1 active: %+v", res.Active)
	}
	dbs, ok := res.Active[0].Schema.Extra["opendbx_databases"].([]any)
	if !ok || len(dbs) != 2 {
		t.Errorf("Extra round-trip broken: %+v", res.Active[0].Schema.Extra)
	}
}

// TestDiscover_WarningsAndFatalSameFile — a file that both warns (non-kebab +
// unknown field) AND fatally fails validation (missing description) contributes
// to BOTH Warnings and Errors, and is not Active (review RC-4 / codex).
func TestDiscover_WarningsAndFatalSameFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	content := "---\nname: Not_Kebab\nopendbx_extra: x\n---\nbody\n" // no description → fatal
	_ = os.WriteFile(filepath.Join(dir, "both.md"), []byte(content), 0o644)
	res := Discover(DiscoverOptions{Roots: []SkillRoot{root(dir)}})
	if len(res.Warnings) < 2 {
		t.Errorf("warnings must survive a fatal validate error, got %+v", res.Warnings)
	}
	if len(res.Errors) != 1 || ClassifyError(res.Errors[0]) != ErrValidationFailed.Code() {
		t.Errorf("want 1 VALIDATION_FAILED, got %+v", res.Errors)
	}
	if len(res.Active) != 0 {
		t.Errorf("fatally invalid skill must not be active: %+v", res.Active)
	}
}

// TestLoadSkill_RejectsSymlink — loadSkill refuses a symlinked file even if the
// scan's d_type missed it (DT_UNKNOWN backstop / TOCTOU).
func TestLoadSkill_RejectsSymlink(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("symlink semantics differ on windows")
	}
	target := filepath.Join(t.TempDir(), "target.md")
	writeSkill(t, target, "tgt")
	link := filepath.Join(t.TempDir(), "link.md")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	_, err := loadSkill(link, root(filepath.Dir(link)))
	assertCode(t, err, ErrFileUnreadable.Code())
}

// TestLoadSkill_MissingFile — a file removed between scan and load is isolated
// as FILE_UNREADABLE (TOCTOU error path).
func TestLoadSkill_MissingFile(t *testing.T) {
	t.Parallel()
	_, err := loadSkill(filepath.Join(t.TempDir(), "gone.md"), root(t.TempDir()))
	assertCode(t, err, ErrFileUnreadable.Code())
}

func TestSummarizeDiscovery_Renders(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeSkill(t, filepath.Join(dir, "a.md"), "alpha")
	res := Discover(DiscoverOptions{Roots: []SkillRoot{root(dir)}})
	out := SummarizeDiscovery(res)
	if !strings.Contains(out, "active (1)") || !strings.Contains(out, "alpha") {
		t.Errorf("summary missing active skill: %q", out)
	}
}
