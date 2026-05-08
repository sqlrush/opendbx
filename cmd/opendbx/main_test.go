// Copyright 2026 opendbx contributors. See LICENSE.
// Author: sqlrush
package main

import "testing"

// TestVersionDefault ensures the version variable has a sensible default.
// Real CLI behavior tests will be added in spec-0.3-cmd-entrypoints.
func TestVersionDefault(t *testing.T) {
	if version == "" {
		t.Fatal("version must not be empty")
	}
}
