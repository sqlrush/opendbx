// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package logger

import (
	"reflect"
	"testing"
)

func TestParseDebugFilter(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		input     string
		wantNil   bool
		wantIncl  []string
		wantExcl  []string
		wantExcMd bool
	}{
		{name: "empty", input: "", wantNil: true},
		{name: "whitespace-only", input: "   ", wantNil: true},
		{name: "single inclusive", input: "api", wantIncl: []string{"api"}, wantExcMd: false},
		{name: "multiple inclusive", input: "api,hooks", wantIncl: []string{"api", "hooks"}, wantExcMd: false},
		{name: "with surrounding spaces", input: " api , hooks ", wantIncl: []string{"api", "hooks"}, wantExcMd: false},
		{name: "single exclusive", input: "!1p", wantExcl: []string{"1p"}, wantExcMd: true},
		{name: "multiple exclusive", input: "!1p,!file", wantExcl: []string{"1p", "file"}, wantExcMd: true},
		{name: "mixed inc+exc → nil", input: "api,!1p", wantNil: true},
		{name: "mixed exc+inc → nil", input: "!1p,api", wantNil: true},
		{name: "blank entries skipped", input: "api,,,hooks", wantIncl: []string{"api", "hooks"}, wantExcMd: false},
		{name: "case normalisation", input: "API,Hooks", wantIncl: []string{"api", "hooks"}, wantExcMd: false},
		{name: "trailing comma", input: "api,", wantIncl: []string{"api"}, wantExcMd: false},
		{name: "exc with trailing comma", input: "!1p,", wantExcl: []string{"1p"}, wantExcMd: true},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := parseDebugFilter(tc.input)
			if tc.wantNil {
				if got != nil {
					t.Errorf("parseDebugFilter(%q) = %+v, want nil", tc.input, got)
				}
				return
			}
			if got == nil {
				t.Fatalf("parseDebugFilter(%q) = nil, want non-nil", tc.input)
			}
			if got.exclusive != tc.wantExcMd {
				t.Errorf("parseDebugFilter(%q).exclusive = %v, want %v", tc.input, got.exclusive, tc.wantExcMd)
			}
			if !reflect.DeepEqual(got.include, tc.wantIncl) {
				t.Errorf("parseDebugFilter(%q).include = %v, want %v", tc.input, got.include, tc.wantIncl)
			}
			if !reflect.DeepEqual(got.exclude, tc.wantExcl) {
				t.Errorf("parseDebugFilter(%q).exclude = %v, want %v", tc.input, got.exclude, tc.wantExcl)
			}
		})
	}
}

func TestExtractDebugCategories(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		message string
		want    []string
	}{
		// Pattern 1: simple prefix.
		{name: "p1 simple prefix", message: "api: making request", want: []string{"api"}},
		{name: "p1 lowercase output", message: "API: x", want: []string{"api"}},
		{name: "p1 with spaces trimmed", message: "  api : x", want: []string{"api"}},
		// Pattern 2: [CATEGORY] brackets.
		{name: "p2 brackets only", message: "[ANT-ONLY] event triggered", want: []string{"ant-only"}},
		{name: "p2 brackets + p1 combined", message: "[ANT-ONLY] 1p event: foo", want: []string{"ant-only", "1p"}},
		// Pattern 3: MCP server (must be checked BEFORE pattern 1).
		{name: "p3 mcp server double-quote", message: `MCP server "filesystem": init`, want: []string{"mcp", "filesystem"}},
		{name: "p3 mcp server single-quote", message: `MCP server 'github': call`, want: []string{"mcp", "github"}},
		{name: "p3 mcp NOT mis-classified as p1", message: `MCP server "fs": ready`, want: []string{"mcp", "fs"}}, // ensure "MCP server" is NOT the p1 category
		// Pattern 4: "1p event:" literal anywhere.
		{name: "p4 1p event in body", message: "[ANT] 1p event: tengu_timer", want: []string{"ant", "1p"}},
		// "[log] 1p event: foo": pattern 2 captures "log" (bracket); pattern 4
		// pushes "1p"; pattern 1 is REJECTED because the message starts with "["
		// (its regex `^([^:[]+):` excludes "[" chars). CC parity.
		{name: "p4 1p event with bracket prefix", message: "[log] 1p event: foo", want: []string{"log", "1p"}},
		// Pattern 5: secondary `:something:` mid-string, reasonable.
		{name: "p5 secondary reasonable", message: "Wrapper: Installation type: development", want: []string{"wrapper", "installation"}},
		{name: "p5 secondary too long ignored", message: "a: thisstringismorethanthirtycharactersaaaa: x", want: []string{"a"}},
		{name: "p5 secondary with space ignored", message: "a: two words: x", want: []string{"a"}},
		// Edge cases.
		{name: "no categories", message: "plain text with no marker", want: nil},
		{name: "empty message", message: "", want: nil},
		{name: "dedupe duplicate", message: "[api] api: x", want: []string{"api"}},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := extractDebugCategories(tc.message)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("extractDebugCategories(%q)\n  got:  %v\n  want: %v", tc.message, got, tc.want)
			}
		})
	}
}

func TestShouldShowDebugCategories(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		categories []string
		filter     *debugFilter
		want       bool
	}{
		{name: "nil filter → show", categories: []string{"x"}, filter: nil, want: true},
		{name: "nil filter empty cats → show", categories: nil, filter: nil, want: true},
		{name: "filter set empty cats → hide", categories: nil, filter: &debugFilter{include: []string{"api"}}, want: false},
		{name: "inclusive match", categories: []string{"api"}, filter: &debugFilter{include: []string{"api", "hooks"}}, want: true},
		{name: "inclusive no match", categories: []string{"other"}, filter: &debugFilter{include: []string{"api"}}, want: false},
		{name: "inclusive multi-cat one matches", categories: []string{"x", "api"}, filter: &debugFilter{include: []string{"api"}}, want: true},
		{name: "exclusive miss → show", categories: []string{"api"}, filter: &debugFilter{exclude: []string{"file"}, exclusive: true}, want: true},
		{name: "exclusive hit → hide", categories: []string{"file"}, filter: &debugFilter{exclude: []string{"file"}, exclusive: true}, want: false},
		{name: "exclusive multi-cat one hits → hide", categories: []string{"x", "file"}, filter: &debugFilter{exclude: []string{"file"}, exclusive: true}, want: false},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := shouldShowDebugCategories(tc.categories, tc.filter)
			if got != tc.want {
				t.Errorf("shouldShowDebugCategories(%v, %+v) = %v, want %v",
					tc.categories, tc.filter, got, tc.want)
			}
		})
	}
}

func TestShouldShowDebugMessage(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		message string
		filter  *debugFilter
		want    bool
	}{
		{name: "nil filter fast path", message: "anything goes", filter: nil, want: true},
		{name: "inclusive matches", message: "api: x", filter: parseDebugFilter("api"), want: true},
		{name: "inclusive misses", message: "other: x", filter: parseDebugFilter("api"), want: false},
		{name: "exclusive blocks", message: "1p event: x", filter: parseDebugFilter("!1p"), want: false},
		{name: "exclusive passes", message: "api: x", filter: parseDebugFilter("!1p"), want: true},
		{name: "mixed pattern → nil → all show", message: "api: x", filter: parseDebugFilter("api,!1p"), want: true},
		{name: "mcp pattern + include mcp", message: `MCP server "fs": go`, filter: parseDebugFilter("mcp"), want: true},
		{name: "mcp pattern + include named server", message: `MCP server "fs": go`, filter: parseDebugFilter("fs"), want: true},
		{name: "uncategorised under filter → hide", message: "plain text", filter: parseDebugFilter("api"), want: false},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := shouldShowDebugMessage(tc.message, tc.filter)
			if got != tc.want {
				t.Errorf("shouldShowDebugMessage(%q, %+v) = %v, want %v",
					tc.message, tc.filter, got, tc.want)
			}
		})
	}
}

func TestContainsString(t *testing.T) {
	t.Parallel()
	if !containsString([]string{"a", "b", "c"}, "b") {
		t.Error("containsString miss for present element")
	}
	if containsString([]string{"a", "b", "c"}, "z") {
		t.Error("containsString hit for absent element")
	}
	if containsString(nil, "x") {
		t.Error("containsString hit for nil slice")
	}
}
