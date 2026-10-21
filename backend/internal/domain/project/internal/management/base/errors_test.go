package base

import (
	"testing"

	"grimoire/lib/common/errs"
)

func TestSentinels(t *testing.T) {
	cases := []struct {
		err    *errs.Error
		code   errs.Code
		reason string
	}{
		{ErrProjectNotFound, errs.CodeNotFound, ""},
		{ErrMemberNotFound, errs.CodeNotFound, ""},
		{ErrForbidden, errs.CodeForbidden, ""},
		{ErrLastOwner, errs.CodeConflict, ReasonLastOwner},
		{ErrAlreadyMember, errs.CodeConflict, ReasonAlreadyMember},
		{ErrConfirmationMismatch, errs.CodeInvalid, ReasonConfirmationMismatch},
		{UserNotEligible(ReasonUserInactive), errs.CodeInvalid, ReasonUserInactive},
		{UserNotEligible(ReasonUserNotDeveloper), errs.CodeInvalid, ReasonUserNotDeveloper},
		{UserNotEligible(ReasonUserNotFound), errs.CodeInvalid, ReasonUserNotFound},
	}
	for _, c := range cases {
		if c.err.Code != c.code || reasonOf(c.err) != c.reason {
			t.Errorf("%v: code=%s reason=%q", c.err, c.err.Code, reasonOf(c.err))
		}
	}
}

func TestFieldErrors(t *testing.T) {
	fe := FieldErrors{"b": "2", "a": "1"}
	if fe.Error() != "validasi gagal: a: 1; b: 2" {
		t.Error(fe.Error())
	}
	var nilFE FieldErrors
	if got := nilFE.Merge(FieldErrors{"x": "y"}); got["x"] != "y" {
		t.Error("merge nil")
	}
	first := FieldErrors{"x": "first"}
	if got := first.Merge(FieldErrors{"x": "second"}); got["x"] != "first" {
		t.Error("error pertama menang")
	}
}
