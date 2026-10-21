package base

import (
	"errors"
	"testing"

	"grimoire/lib/common/errs"
)

func TestErrors_CodeAndReason(t *testing.T) {
	cases := []struct {
		err    *errs.Error
		code   errs.Code
		reason string
	}{
		{ErrProjectNotFound, errs.CodeNotFound, ""},
		{ErrSpecNotFound, errs.CodeNotFound, ""},
		{ErrInvalidName, errs.CodeInvalid, ReasonInvalidName},
		{ErrNameTaken, errs.CodeConflict, ReasonNameTaken},
		{ErrNoFiles, errs.CodeInvalid, ReasonNoFiles},
		{ErrNameWithMultipleFiles, errs.CodeInvalid, ReasonNameWithMultipleFiles},
		{ErrNotMultipart, errs.CodeInvalid, ReasonNotMultipart},
		{ErrPayloadTooLarge, errs.CodePayloadTooLarge, ""},
	}
	for _, c := range cases {
		var e *errs.Error
		if !errors.As(c.err, &e) || e.Code != c.code {
			t.Fatalf("%v: code", c.err)
		}
		if c.reason != "" {
			m, ok := e.Details.(map[string]any)
			if !ok || m["reason"] != c.reason {
				t.Fatalf("%v: details %v", c.err, e.Details)
			}
		}
	}
}
