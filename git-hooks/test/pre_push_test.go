// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// pre-push hook test harness. Each test builds a throwaway git repo in
// t.TempDir(), constructs the target tag scenario (annotated / lightweight
// / wrong commit / missing canonical fields), then pipes pre-push protocol
// lines to the hook and asserts exit code + stderr substring.
//
// spec-0.7 § 2.5 D-5 / T-8.

package prepushtest

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const zeroSHA = "0000000000000000000000000000000000000000"

// canonicalMsg returns a tag message matching spec-0.7 § 2.7 schema with
// a Commit field matching the actual git HEAD short hash. After T-12 H3
// strict validation, the hook compares the Commit field to the actual tag
// target — so fixtures must compute the real hash.
func canonicalMsg(t *testing.T, dir string) string {
	t.Helper()
	commit := mustGit(t, dir, "rev-parse", "--short=12", "HEAD")
	return strings.Join([]string{
		"Spec: spec-0.7-version-numbering",
		"Repo: opendbx",
		"Commit: " + commit,
		"Peer-Repo: opendbrb",
		"Peer-Commit: 789abc012def",
	}, "\n") + "\n"
}

func hookPath(t *testing.T) string {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	abs, err := filepath.Abs(filepath.Join(cwd, "..", "pre-push"))
	if err != nil {
		t.Fatalf("resolve hook path: %v", err)
	}
	if _, err := os.Stat(abs); err != nil {
		t.Fatalf("hook not found at %s: %v", abs, err)
	}
	return abs
}

// fakeRepo builds a minimal git repo with one commit on main, returns its
// path. Caller can then create tags and exec the hook against it.
func fakeRepo(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("pre-push hook requires POSIX shell; skipping on Windows")
	}
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not available")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	dir := t.TempDir()
	mustGit(t, dir, "init", "-b", "main")
	mustGit(t, dir, "config", "user.email", "test@example.com")
	mustGit(t, dir, "config", "user.name", "Test User")
	mustGit(t, dir, "config", "commit.gpgsign", "false")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("test\n"), 0o600); err != nil {
		t.Fatalf("write README: %v", err)
	}
	mustGit(t, dir, "add", "README.md")
	mustGit(t, dir, "commit", "-m", "initial")
	return dir
}

// cleanGitEnv returns the parent process env with GIT_DIR / GIT_WORK_TREE
// FILTERED OUT (not set to ""; empty value triggers "empty string is not
// a valid path" from git). Adds the test-isolation pins for global +
// system config. go-reviewer T-12 M2 fix.
func cleanGitEnv() []string {
	out := make([]string, 0, len(os.Environ())+2)
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "GIT_DIR=") || strings.HasPrefix(kv, "GIT_WORK_TREE=") {
			continue
		}
		out = append(out, kv)
	}
	out = append(out,
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_CONFIG_SYSTEM=/dev/null",
	)
	return out
}

func mustGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = cleanGitEnv()
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s in %s failed: %v\n%s", strings.Join(args, " "), dir, err, out)
	}
	return strings.TrimSpace(string(out))
}

// gitRevHEAD returns the current HEAD sha.
func gitRevHEAD(t *testing.T, dir string) string {
	t.Helper()
	return mustGit(t, dir, "rev-parse", "HEAD")
}

// runHook exec-s the pre-push hook in dir, piping the given stdin (pre-push
// protocol lines). Returns combined stdout+stderr and the exit error.
func runHook(t *testing.T, dir, stdin string, env ...string) (string, error) {
	t.Helper()
	cmd := exec.Command("bash", hookPath(t)) //nolint:gosec // test-only exec of in-repo hook
	cmd.Dir = dir
	cmd.Env = append(cleanGitEnv(), env...)
	cmd.Stdin = strings.NewReader(stdin)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	return buf.String(), err
}

// pushLine constructs the pre-push stdin format:
//
//	<local_ref> <local_sha> <remote_ref> <remote_sha>
//
// Each push direction (create/update/delete) is just the zero-sha
// substituted at the appropriate position.
func pushLine(localRef, localSHA, remoteRef, remoteSHA string) string {
	return fmt.Sprintf("%s %s %s %s\n", localRef, localSHA, remoteRef, remoteSHA)
}

// --- Path 1: valid annotated tag at HEAD with canonical msg → ACCEPT --

func TestPrePush_AcceptValidTag(t *testing.T) {
	dir := fakeRepo(t)
	mustGit(t, dir, "tag", "-a", "v0.7.0-stage0.7", "-m", canonicalMsg(t, dir))
	head := gitRevHEAD(t, dir)

	stdin := pushLine("refs/tags/v0.7.0-stage0.7", head, "refs/tags/v0.7.0-stage0.7", zeroSHA)
	out, err := runHook(t, dir, stdin)
	if err != nil {
		t.Fatalf("hook should accept valid tag; got error: %v\n%s", err, out)
	}
}

// --- Path 2: invalid VersionPattern → REJECT --------------------------

func TestPrePush_RejectInvalidName(t *testing.T) {
	dir := fakeRepo(t)
	mustGit(t, dir, "tag", "-a", "v1.0", "-m", canonicalMsg(t, dir)) // missing -stage<S>.<N>
	head := gitRevHEAD(t, dir)

	stdin := pushLine("refs/tags/v1.0", head, "refs/tags/v1.0", zeroSHA)
	out, err := runHook(t, dir, stdin)
	if err == nil {
		t.Fatalf("hook should reject invalid name; got success:\n%s", out)
	}
	if !strings.Contains(out, "violates VersionPattern") {
		t.Errorf("error message should mention VersionPattern:\n%s", out)
	}
}

// --- Path 3: lightweight tag (not annotated) → REJECT -----------------

func TestPrePush_RejectLightweightTag(t *testing.T) {
	dir := fakeRepo(t)
	// `git tag <name>` without -a creates lightweight.
	mustGit(t, dir, "tag", "v0.7.0-stage0.7")
	head := gitRevHEAD(t, dir)

	stdin := pushLine("refs/tags/v0.7.0-stage0.7", head, "refs/tags/v0.7.0-stage0.7", zeroSHA)
	out, err := runHook(t, dir, stdin)
	if err == nil {
		t.Fatalf("hook should reject lightweight tag; got success:\n%s", out)
	}
	if !strings.Contains(out, "must be annotated") {
		t.Errorf("error should say 'must be annotated':\n%s", out)
	}
}

// --- Path 4: tag points to non-main commit → REJECT -------------------

func TestPrePush_RejectNonMainTarget(t *testing.T) {
	dir := fakeRepo(t)
	firstHead := gitRevHEAD(t, dir)
	// Add a second commit so main HEAD moves forward.
	if err := os.WriteFile(filepath.Join(dir, "second.txt"), []byte("more\n"), 0o600); err != nil {
		t.Fatalf("write second: %v", err)
	}
	mustGit(t, dir, "add", "second.txt")
	mustGit(t, dir, "commit", "-m", "second commit")
	// Tag the OLD commit (now != main HEAD).
	mustGit(t, dir, "tag", "-a", "v0.7.0-stage0.7", firstHead, "-m", canonicalMsg(t, dir))

	stdin := pushLine("refs/tags/v0.7.0-stage0.7", firstHead, "refs/tags/v0.7.0-stage0.7", zeroSHA)
	out, err := runHook(t, dir, stdin)
	if err == nil {
		t.Fatalf("hook should reject non-main target; got success:\n%s", out)
	}
	if !strings.Contains(out, "not main HEAD") {
		t.Errorf("error should mention 'not main HEAD':\n%s", out)
	}
}

// --- Path 5: canonical message missing field → REJECT (codex T-12 H3:
//             now caught by 5-line count check, not field-presence).

func TestPrePush_RejectMissingCanonicalField(t *testing.T) {
	dir := fakeRepo(t)
	// Missing "Peer-Commit:" line — only 4 non-empty lines total.
	incompleteMsg := strings.Join([]string{
		"Spec: spec-0.7-version-numbering",
		"Repo: opendbx",
		"Commit: abc123def456",
		"Peer-Repo: opendbrb",
		// Peer-Commit intentionally omitted.
	}, "\n") + "\n"
	mustGit(t, dir, "tag", "-a", "v0.7.0-stage0.7", "-m", incompleteMsg)
	head := gitRevHEAD(t, dir)

	stdin := pushLine("refs/tags/v0.7.0-stage0.7", head, "refs/tags/v0.7.0-stage0.7", zeroSHA)
	out, err := runHook(t, dir, stdin)
	if err == nil {
		t.Fatalf("hook should reject missing canonical field; got success:\n%s", out)
	}
	if !strings.Contains(out, "5 non-empty lines") {
		t.Errorf("error should mention line count check:\n%s", out)
	}
}

// --- Path 6: tag delete → REJECT (unless OPENDBX_TAG_REPAIR=1)
//
// codex T-12 H1: real git pre-push delete protocol uses local_ref="(delete)"
// + remote_ref="refs/tags/<name>". Test BOTH the unrealistic fixture (kept
// for compatibility) and the realistic protocol shape.

func TestPrePush_RejectTagDelete_RealProtocol(t *testing.T) {
	dir := fakeRepo(t)
	head := gitRevHEAD(t, dir)
	// Real git protocol: local_ref is the literal "(delete)" string.
	stdin := pushLine("(delete)", zeroSHA, "refs/tags/v0.7.0-stage0.7", head)
	out, err := runHook(t, dir, stdin)
	if err == nil {
		t.Fatalf("hook should reject real-protocol tag delete; got success:\n%s", out)
	}
	if !strings.Contains(out, "tag delete blocked") {
		t.Errorf("error should mention 'tag delete blocked':\n%s", out)
	}
}

func TestPrePush_RejectTagDelete(t *testing.T) {
	dir := fakeRepo(t)
	head := gitRevHEAD(t, dir)
	// Legacy fixture: local_ref still refs/tags/* but local_sha=zero. Some
	// git client versions emit this shape historically; the hook handles
	// both. We assert the same reject behavior.
	stdin := pushLine("refs/tags/v0.7.0-stage0.7", zeroSHA, "refs/tags/v0.7.0-stage0.7", head)
	out, err := runHook(t, dir, stdin)
	if err == nil {
		t.Fatalf("hook should reject tag delete; got success:\n%s", out)
	}
	if !strings.Contains(out, "tag delete blocked") {
		t.Errorf("error should mention 'tag delete blocked':\n%s", out)
	}
}

// --- Path 7: tag update → REJECT (unless OPENDBX_TAG_REPAIR=1) --------

func TestPrePush_RejectTagUpdate(t *testing.T) {
	dir := fakeRepo(t)
	mustGit(t, dir, "tag", "-a", "v0.7.0-stage0.7", "-m", canonicalMsg(t, dir))
	head := gitRevHEAD(t, dir)
	// Update: both shas non-zero AND differ.
	stdin := pushLine("refs/tags/v0.7.0-stage0.7", head, "refs/tags/v0.7.0-stage0.7", "1111111111111111111111111111111111111111")
	out, err := runHook(t, dir, stdin)
	if err == nil {
		t.Fatalf("hook should reject tag update; got success:\n%s", out)
	}
	if !strings.Contains(out, "tag update blocked") {
		t.Errorf("error should mention 'tag update blocked':\n%s", out)
	}
}

// --- Path 8: OPENDBX_TAG_REPAIR=1 bypasses delete check ---------------

func TestPrePush_RepairEnvBypassesDelete(t *testing.T) {
	dir := fakeRepo(t)
	head := gitRevHEAD(t, dir)
	stdin := pushLine("refs/tags/v0.7.0-stage0.7", zeroSHA, "refs/tags/v0.7.0-stage0.7", head)
	// With OPENDBX_TAG_REPAIR=1 the delete is not blocked. The rest of the
	// hook's checks (VersionPattern + annotated + main-HEAD + canonical)
	// still run on the deleted-local-sha which is zero — but a delete is
	// special: local_sha=zero means "no tag to validate", so the hook's
	// downstream checks would still try to peel a non-existent tag. The
	// repair env only suppresses the up-front "delete blocked" gate; after
	// that, the hook will still fail because tag isn't annotated, etc.
	// Spec § 2.5 documents repair env as "explicit opt-in" — actual tag
	// content checks still run. So we expect FAILURE here, but with a
	// different error than "tag delete blocked".
	out, err := runHook(t, dir, stdin, "OPENDBX_TAG_REPAIR=1")
	if err == nil {
		t.Fatalf("hook should still validate content after repair bypass; got success:\n%s", out)
	}
	if strings.Contains(out, "tag delete blocked") {
		t.Errorf("OPENDBX_TAG_REPAIR=1 should bypass delete-blocked error:\n%s", out)
	}
}

// --- Path 9: non-tag refs are ignored ---------------------------------

func TestPrePush_IgnoresNonTagRefs(t *testing.T) {
	dir := fakeRepo(t)
	head := gitRevHEAD(t, dir)
	// Push a branch (refs/heads/*) — hook should skip without error.
	stdin := pushLine("refs/heads/main", head, "refs/heads/main", zeroSHA)
	out, err := runHook(t, dir, stdin)
	if err != nil {
		t.Fatalf("hook should accept non-tag ref; got error: %v\n%s", err, out)
	}
}

// --- Path 10: empty stdin → ACCEPT (no refs to validate) ---------------

func TestPrePush_EmptyStdinAccepted(t *testing.T) {
	dir := fakeRepo(t)
	out, err := runHook(t, dir, "")
	if err != nil {
		t.Fatalf("hook should accept empty stdin; got error: %v\n%s", err, out)
	}
}

// --- Path 11: canonical schema strict — wrong Repo value → REJECT ----
// codex T-12 H3: hook must enforce Repo=opendbx (in opendbx repo) literal.

func TestPrePush_RejectWrongRepoField(t *testing.T) {
	dir := fakeRepo(t)
	commit := mustGit(t, dir, "rev-parse", "--short=12", "HEAD")
	wrongMsg := strings.Join([]string{
		"Spec: spec-0.7-version-numbering",
		"Repo: opendbrb", // wrong — must be opendbx
		"Commit: " + commit,
		"Peer-Repo: opendbrb",
		"Peer-Commit: 789abc012def",
	}, "\n") + "\n"
	mustGit(t, dir, "tag", "-a", "v0.7.0-stage0.7", "-m", wrongMsg)
	head := gitRevHEAD(t, dir)
	stdin := pushLine("refs/tags/v0.7.0-stage0.7", head, "refs/tags/v0.7.0-stage0.7", zeroSHA)
	out, err := runHook(t, dir, stdin)
	if err == nil {
		t.Fatalf("hook should reject wrong Repo field; got success:\n%s", out)
	}
	if !strings.Contains(out, "must be 'Repo: opendbx'") {
		t.Errorf("error should mention 'must be Repo: opendbx':\n%s", out)
	}
}

// --- Path 12: canonical schema — Commit value mismatch → REJECT ------
// codex T-12 H3: hook must enforce Commit == actual git rev-parse short=12 of tag^{}.

func TestPrePush_RejectMismatchedCommit(t *testing.T) {
	dir := fakeRepo(t)
	mismatchMsg := strings.Join([]string{
		"Spec: spec-0.7-version-numbering",
		"Repo: opendbx",
		"Commit: deadbeef0000", // wrong — does not match actual HEAD
		"Peer-Repo: opendbrb",
		"Peer-Commit: 789abc012def",
	}, "\n") + "\n"
	mustGit(t, dir, "tag", "-a", "v0.7.0-stage0.7", "-m", mismatchMsg)
	head := gitRevHEAD(t, dir)
	stdin := pushLine("refs/tags/v0.7.0-stage0.7", head, "refs/tags/v0.7.0-stage0.7", zeroSHA)
	out, err := runHook(t, dir, stdin)
	if err == nil {
		t.Fatalf("hook should reject mismatched Commit; got success:\n%s", out)
	}
	if !strings.Contains(out, "Commit field") {
		t.Errorf("error should mention 'Commit field' mismatch:\n%s", out)
	}
}

// --- Path 13: canonical schema — extra line beyond 5 → REJECT --------

func TestPrePush_RejectExtraLinesInMessage(t *testing.T) {
	dir := fakeRepo(t)
	commit := mustGit(t, dir, "rev-parse", "--short=12", "HEAD")
	extraMsg := strings.Join([]string{
		"Spec: spec-0.7-version-numbering",
		"Repo: opendbx",
		"Commit: " + commit,
		"Peer-Repo: opendbrb",
		"Peer-Commit: 789abc012def",
		"Extra: bogus", // 6th line — must be rejected
	}, "\n") + "\n"
	mustGit(t, dir, "tag", "-a", "v0.7.0-stage0.7", "-m", extraMsg)
	head := gitRevHEAD(t, dir)
	stdin := pushLine("refs/tags/v0.7.0-stage0.7", head, "refs/tags/v0.7.0-stage0.7", zeroSHA)
	out, err := runHook(t, dir, stdin)
	if err == nil {
		t.Fatalf("hook should reject extra lines; got success:\n%s", out)
	}
	if !strings.Contains(out, "5 non-empty lines") {
		t.Errorf("error should mention line-count enforcement:\n%s", out)
	}
}
