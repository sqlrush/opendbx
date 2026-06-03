// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package skills

import (
	"reflect"
	"testing"
)

func TestAllowedToolsList(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		csv  string
		want []string
	}{
		{"empty inherits", "", nil},
		{"blank inherits", "   ", nil},
		{"spaced", "Read, Grep, Glob, Bash", []string{"Read", "Grep", "Glob", "Bash"}},
		{"no spaces", "Read,Grep", []string{"Read", "Grep"}},
		{"consecutive commas filtered", "Read,,Grep", []string{"Read", "Grep"}},
		{"trailing comma filtered", "Read,", []string{"Read"}},
		{"only commas", ",,,", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := Schema{AllowedTools: tc.csv}.AllowedToolsList()
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("AllowedToolsList(%q) = %v; want %v", tc.csv, got, tc.want)
			}
		})
	}
}
