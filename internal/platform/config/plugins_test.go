// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package config

import (
	"strings"
	"testing"
)

func TestValidatePlugins_OK(t *testing.T) {
	t.Parallel()
	cfg := &Config{Plugins: PluginsConfig{
		SkillSearchPaths: []string{"/abs/skills", "rel/skills"},
		DisabledSkills:   []string{"noisy-skill"},
	}}
	var errs ValidationErrors
	validatePlugins(cfg, &errs)
	if len(errs) != 0 {
		t.Errorf("clean plugins config should pass: %v", errs)
	}
}

func TestValidatePlugins_RejectsTraversal(t *testing.T) {
	t.Parallel()
	cfg := &Config{Plugins: PluginsConfig{
		SkillSearchPaths: []string{"../../etc", "ok/skills", "a/../../b"},
	}}
	var errs ValidationErrors
	validatePlugins(cfg, &errs)
	n := 0
	for _, e := range errs {
		if e.Rule == "no-traversal" {
			n++
		}
	}
	if n != 2 {
		t.Errorf("expected 2 traversal rejections, got %d (%v)", n, errs)
	}
}

func TestValidatePlugins_CapsCount(t *testing.T) {
	t.Parallel()
	many := make([]string, maxSkillSearchPaths+1)
	for i := range many {
		many[i] = "p"
	}
	cfg := &Config{Plugins: PluginsConfig{SkillSearchPaths: many}}
	var errs ValidationErrors
	validatePlugins(cfg, &errs)
	found := false
	for _, e := range errs {
		if e.Path == "Plugins.SkillSearchPaths" && e.Rule == "max" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected max-count violation, got %v", errs)
	}
}

func TestHasDotDot(t *testing.T) {
	t.Parallel()
	// Clean resolves non-escaping ".." (a/b/.. → a), so only an ESCAPING ".."
	// component survives — that is exactly the traversal we reject.
	cases := map[string]bool{
		"/abs/ok":      false,
		"rel/ok":       false,
		"../escape":    true,  // leading .. escapes the base
		"a/../../b":    true,  // cleans to ../b — escapes
		"a/b/..":       false, // cleans to a — safe, stays within
		"..hidden/ok":  false, // ".." only as a full component
		"/abs/../back": false, // cleans to /back — absolute, no escaping ..
	}
	for p, want := range cases {
		if got := hasDotDot(p); got != want {
			t.Errorf("hasDotDot(%q) = %v; want %v", p, got, want)
		}
	}
}

func TestValidatePlugins_ErrorMessageFormat(t *testing.T) {
	t.Parallel()
	cfg := &Config{Plugins: PluginsConfig{SkillSearchPaths: []string{"../x"}}}
	var errs ValidationErrors
	validatePlugins(cfg, &errs)
	if len(errs) == 0 || !strings.Contains(errs.Error(), "traversal") {
		t.Errorf("traversal error should mention traversal: %v", errs)
	}
}
