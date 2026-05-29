// Package testfile carries an intentional violation INSIDE a _test.go
// file so the test suite can prove the default lint behavior skips
// test files entirely (and confirm `paint-pattern-lint:include` opt-in
// flips that behavior — exercised by includedfile/).
package testfile

import "example.com/paint-pattern-lint-fixtures/buffer"

func violationInTestFile(dst *buffer.Grid, src buffer.Buffer) {
	dst.SetCell(0, 0, src.Cell(0, 0)) // must be skipped by default
}
