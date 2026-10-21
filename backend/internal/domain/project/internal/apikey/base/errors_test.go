package base

import (
	"errors"
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
		{ErrKeyNotFound, errs.CodeNotFound, ReasonKeyNotFound},
		{ErrNameTaken, errs.CodeConflict, ReasonNameTaken},
		{ErrAlreadyRevoked, errs.CodeConflict, ReasonAlreadyRevoked},
		{ErrKeyLimitReached(3), errs.CodeConflict, ReasonKeyLimitReached},
	}
	for _, tc := range cases {
		if tc.err.Code != tc.code {
			t.Fatalf("%v code", tc.err)
		}
		if tc.reason == "" {
			continue
		}
		if tc.err.Details.(map[string]any)["reason"] != tc.reason {
			t.Fatalf("%v reason", tc.err)
		}
	}
	var e *errs.Error
	if errors.As(ErrPrefixCollision, &e) {
		t.Fatal("ErrPrefixCollision must be internal")
	}
}

func TestErrKeyLimitReached_FreshInstance(t *testing.T) {
	a, b := ErrKeyLimitReached(1), ErrKeyLimitReached(2)
	if a == b || a.Details.(map[string]any)["limit"] != 1 || b.Details.(map[string]any)["limit"] != 2 {
		t.Fatal("not fresh")
	}
	if a.Message != "batas 1 key aktif per project tercapai; cabut key yang tidak dipakai" {
		t.Fatal(a.Message)
	}
}

func TestFieldErrors(t *testing.T) {
	var fe FieldErrors
	fe = fe.Merge(FieldErrors{"b": "2", "a": "1"})
	fe = fe.Merge(FieldErrors{"a": "x", "c": "3"})
	if fe["a"] != "1" || fe["c"] != "3" {
		t.Fatalf("%v", fe)
	}
	if fe.Error() != "validasi gagal: a: 1; b: 2; c: 3" {
		t.Fatal(fe.Error())
	}
	e := InvalidFields(fe)
	if e.Code != errs.CodeInvalid {
		t.Fatal(e.Code)
	}
}
