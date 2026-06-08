// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package dbquery

import (
	"strings"
	"testing"
)

// TestErrQueryInputInvalid_Registered — the new code carries the Rule 7
// triple with the SKILL-less DB. prefix and Chinese text (spec-2.3a Q11).
func TestErrQueryInputInvalid_Registered(t *testing.T) {
	t.Parallel()
	if ErrQueryInputInvalid.Code() != "DB.QUERY_INPUT_INVALID" {
		t.Errorf("code = %q", ErrQueryInputInvalid.Code())
	}
	if ErrQueryInputInvalid.Message() == "" || ErrQueryInputInvalid.Hint() == "" {
		t.Error("missing message/hint (Rule 7 triple)")
	}
}

// TestInputInvalidContent — template carries code, message, the detail
// variant, and the call-shape hint (spec-2.3a D-5).
func TestInputInvalidContent(t *testing.T) {
	t.Parallel()
	got := inputInvalidContent("缺少或非字符串 sql 字段")
	for _, want := range []string{
		"DB.QUERY_INPUT_INVALID",
		"缺少或非字符串 sql 字段",
		`{"sql":`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("inputInvalidContent missing %q in %q", want, got)
		}
	}
}
