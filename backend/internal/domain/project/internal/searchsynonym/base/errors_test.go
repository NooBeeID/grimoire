package base

import (
	"errors"
	"testing"

	"grimoire/lib/common/errs"
)

func TestInvalid(t *testing.T) {
	e := Invalid(ReasonTooFewTerms)
	if e.Code != errs.CodeInvalid || !errors.Is(e, errs.ErrInvalid) {
		t.Fatalf("e = %+v", e)
	}
	if d := e.Details.(map[string]any); d["reason"] != ReasonTooFewTerms {
		t.Fatalf("details = %v", d)
	}
}

func TestTermInUse(t *testing.T) {
	e := TermInUse([]string{"login"})
	if e.Code != errs.CodeConflict {
		t.Fatalf("code = %s", e.Code)
	}
	d := e.Details.(map[string]any)
	if d["reason"] != ReasonTermInUse || d["terms"].([]string)[0] != "login" {
		t.Fatalf("details = %v", d)
	}
}

func TestGroupLimit(t *testing.T) {
	e := GroupLimit()
	if e.Code != errs.CodeConflict || e.Details.(map[string]any)["reason"] != ReasonGroupLimit {
		t.Fatalf("e = %+v", e)
	}
}

func TestSentinels(t *testing.T) {
	if ErrProjectNotFound.Code != errs.CodeNotFound || ErrGroupNotFound.Code != errs.CodeNotFound ||
		ErrUnauthenticated.Code != errs.CodeUnauthorized {
		t.Fatal("kode sentinel salah")
	}
}

func TestReasonMessage(t *testing.T) {
	if ReasonTermInUse.Message() == string(ReasonTermInUse) {
		t.Fatal("pesan harus diterjemahkan")
	}
	if Reason("x").Message() != "x" {
		t.Fatal("fallback ke kode")
	}
}
