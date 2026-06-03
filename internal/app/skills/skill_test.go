// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package skills

import "testing"

func TestSkillKey(t *testing.T) {
	t.Parallel()
	s := Skill{Schema: Schema{Name: "top-sql"}}
	if s.Key() != "top-sql" {
		t.Errorf("Key() = %q; want top-sql", s.Key())
	}
}

func TestSourceKindString(t *testing.T) {
	t.Parallel()
	cases := map[SourceKind]string{
		SourceUnknown:     "unknown",
		SourceBuiltin:     "builtin",
		SourcePluginCache: "plugin-cache",
		SourceUserGlobal:  "user-global",
		SourceProject:     "project",
	}
	for k, want := range cases {
		if got := k.String(); got != want {
			t.Errorf("SourceKind(%d).String() = %q; want %q", k, got, want)
		}
	}
}
