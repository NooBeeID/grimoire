package base

import (
	"errors"
	"testing"
	"time"

	"grimoire/lib/common/errs"
)

func TestFieldErrors_MergeKeepsFirst(t *testing.T) {
	var fe FieldErrors
	fe = fe.Merge(FieldErrors{"a": "1"})
	fe = fe.Merge(FieldErrors{"a": "2", "b": "3"})
	if fe["a"] != "1" || fe["b"] != "3" || len(fe) != 2 {
		t.Fatalf("%v", fe)
	}
	if got := FieldErrors(nil).Merge(nil); got != nil {
		t.Fatal("nil merge nil harus nil")
	}
}

func TestFieldErrors_ErrorSorted(t *testing.T) {
	if got := (FieldErrors{"b": "2", "a": "1"}).Error(); got != "validasi gagal: a: 1; b: 2" {
		t.Fatal(got)
	}
}

func TestAsFieldErrors(t *testing.T) {
	if AsFieldErrors(nil) != nil || AsFieldErrors(errors.New("x")) != nil {
		t.Fatal("bukan FieldErrors harus nil")
	}
	if AsFieldErrors(FieldErrors{"a": "1"})["a"] != "1" {
		t.Fatal("harus terekstrak")
	}
}

func TestInvalidFields(t *testing.T) {
	e := InvalidFields(FieldErrors{"email": "wajib diisi"})
	d, ok := e.Details.(map[string]any)
	if e.Code != errs.CodeInvalid || !ok || d["fields"].(FieldErrors)["email"] != "wajib diisi" {
		t.Fatalf("%+v", e)
	}
}

func TestAccountLocked(t *testing.T) {
	e := AccountLocked(1500 * time.Millisecond)
	if e.Code != CodeAccountLocked || e.Details.(map[string]any)["retry_after_seconds"] != 2 {
		t.Fatalf("%+v", e)
	}
	if AccountLocked(0).Details.(map[string]any)["retry_after_seconds"] != 1 {
		t.Fatal("minimal 1")
	}
}

func TestSentinels(t *testing.T) {
	if ErrSelfAction.Details.(map[string]any)["reason"] != ReasonSelfAction || ErrSelfAction.Code != errs.CodeForbidden {
		t.Fatal("self")
	}
	if ErrLastSuperAdmin.Details.(map[string]any)["reason"] != ReasonLastSuperAdmin || ErrLastSuperAdmin.Code != errs.CodeConflict {
		t.Fatal("last")
	}
	if ErrUserNotFound.Code != errs.CodeNotFound || ErrInvalidCreds.Code != CodeInvalidCredentials || ErrInvalidCreds.Message != "email atau password salah" {
		t.Fatal("sentinel")
	}
	if EmailConflict().Code != errs.CodeConflict || EmailConflict().Message != "email sudah terdaftar" {
		t.Fatal("conflict")
	}
}
