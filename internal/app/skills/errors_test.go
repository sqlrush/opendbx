// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package skills

import (
	"strings"
	"testing"

	"github.com/sqlrush/opendbx/internal/platform/errcode"
)

// allSkillSentinels lists every SKILL.* sentinel for table-driven assertions.
func allSkillSentinels() []errcode.Sentinel {
	return []errcode.Sentinel{
		ErrNoFrontmatter, ErrUnterminatedFrontmatter, ErrParseError,
		ErrTooLarge, ErrTooDeep, ErrMissingName, ErrInvalidName,
		ErrMissingDescription, ErrNamespaceConflict, ErrValidationFailed,
		// spec-2.2 discovery
		ErrRootUnreadable, ErrFileUnreadable, ErrRootTooManyFiles,
	}
}

// TestErrorsRegistered — every SKILL.* sentinel is registered with the
// three-piece contract (Code/Message/Hint all non-empty), code prefix SKILL.
func TestErrorsRegistered(t *testing.T) {
	t.Parallel()
	seen := map[string]bool{}
	for _, s := range allSkillSentinels() {
		if !strings.HasPrefix(s.Code(), "SKILL.") {
			t.Errorf("code %q lacks SKILL. prefix", s.Code())
		}
		if s.Message() == "" || s.Hint() == "" {
			t.Errorf("%s missing message/hint (规则 7 三件套)", s.Code())
		}
		if seen[s.Code()] {
			t.Errorf("duplicate code %s", s.Code())
		}
		seen[s.Code()] = true
	}
	if len(seen) != 13 {
		t.Errorf("expected 13 SKILL.* codes, got %d", len(seen))
	}
}
