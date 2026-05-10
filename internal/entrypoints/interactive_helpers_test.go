// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package entrypoints

import (
	"context"
	"errors"
	"testing"
)

func TestRenderAndRun_NotImplemented(t *testing.T) {
	err := RenderAndRun(context.Background())
	if !errors.Is(err, ErrInteractiveHelperNotImplemented) {
		t.Errorf("expected ErrInteractiveHelperNotImplemented, got %v", err)
	}
}

func TestShowSetupDialog_NotImplemented(t *testing.T) {
	v, err := ShowSetupDialog(context.Background(), nil)
	if !errors.Is(err, ErrInteractiveHelperNotImplemented) {
		t.Errorf("expected ErrInteractiveHelperNotImplemented, got %v", err)
	}
	if v != nil {
		t.Errorf("expected nil value, got %v", v)
	}
}
