// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package buffer

import (
	"strings"
	"testing"
)

func TestErrInvalidDimension_Code(t *testing.T) {
	t.Parallel()
	if ErrInvalidDimension.Code() != "RENDER.INVALID_DIMENSION" {
		t.Errorf("ErrInvalidDimension.Code() = %q, want RENDER.INVALID_DIMENSION", ErrInvalidDimension.Code())
	}
	if !strings.Contains(ErrInvalidDimension.Hint(), "cols") {
		t.Errorf("ErrInvalidDimension.Hint() should mention cols; got %q", ErrInvalidDimension.Hint())
	}
}
