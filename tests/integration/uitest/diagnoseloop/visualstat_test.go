// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

//go:build !windows

package diagnoseloop_test

import (
	"fmt"
	"os"
)

// visualStat is a thin os.Stat wrapper that returns a non-nil error when
// the path is missing OR is not a directory — exactly the two failure
// modes the parked-fixture guard cares about. Kept in its own file so
// the test file stays focused on test logic.
func visualStat(path string) (os.FileInfo, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%s is not a directory", path)
	}
	return info, nil
}
