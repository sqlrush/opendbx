// paint-pattern-lint:include — production fixture: opt this _test.go
// file into the linter so the suite can prove the include directive
// actually flips the default skip.

package includedfile

import "example.com/paint-pattern-lint-fixtures/buffer"

func ViolationInIncludedTestFile(dst *buffer.Grid, src buffer.Buffer) {
	dst.SetCell(0, 0, src.Cell(0, 0)) // lint MUST report this
}
