package base

import (
	"errors"
	"net/http"
	"testing"

	"grimoire/internal/infra/http/respond"
	"grimoire/lib/common/errs"
)

func asErr(t *testing.T, err error) *errs.Error {
	t.Helper()
	var e *errs.Error
	if !errors.As(err, &e) {
		t.Fatalf("not *errs.Error: %v", err)
	}
	return e
}

func reasonOf(t *testing.T, e *errs.Error) Reason {
	t.Helper()
	d, ok := e.Details.(map[string]any)
	if !ok {
		t.Fatalf("details = %#v", e.Details)
	}
	r, _ := d["reason"].(Reason)
	return r
}

func TestInvalid(t *testing.T) {
	e := Invalid(ReasonPathTraversal)
	if e.Code != errs.CodeInvalid || reasonOf(t, e) != ReasonPathTraversal || e.Message != ReasonPathTraversal.Message() ||
		!errors.Is(e, errs.ErrInvalid) || respond.StatusFor(e.Code) != http.StatusBadRequest {
		t.Fatalf("%+v", e)
	}
	m := InvalidMsg(ReasonHidden, `path ".git" tidak valid`)
	if m.Message != `path ".git" tidak valid` || reasonOf(t, m) != ReasonHidden || !errors.Is(m, errs.ErrInvalid) {
		t.Fatalf("%+v", m)
	}
}

func TestTooLarge(t *testing.T) {
	e := TooLarge(ReasonTooLarge, 1048576)
	d := e.Details.(map[string]any)
	if e.Code != errs.CodePayloadTooLarge || respond.StatusFor(e.Code) != http.StatusRequestEntityTooLarge ||
		d["max_bytes"] != int64(1048576) || e.Message != "ukuran file melebihi batas 1048576 byte" {
		t.Fatalf("%+v", e)
	}
	if u := TooLarge(ReasonUploadTooLarge, 5); u.Message != ReasonUploadTooLarge.Message() {
		t.Fatal(u.Message)
	}
}

func TestVersionConflict(t *testing.T) {
	cur := ConflictCurrent{Content: "x", Version: "v", UpdatedByLabel: "Sari"}
	e := VersionConflict(cur)
	d := e.Details.(map[string]any)
	if e.Code != errs.CodeConflict || respond.StatusFor(e.Code) != http.StatusConflict ||
		reasonOf(t, e) != ReasonVersionConflict || d["current"].(ConflictCurrent) != cur {
		t.Fatalf("%+v", e)
	}
}

func TestSentinels(t *testing.T) {
	if respond.StatusFor(ErrProjectNotFound.Code) != 404 || respond.StatusFor(ErrFileNotFound.Code) != 404 ||
		respond.StatusFor(ErrFolderNotFound.Code) != 404 || respond.StatusFor(ErrUnauthenticated.Code) != 401 ||
		respond.StatusFor(ErrPathExists.Code) != 409 || reasonOf(t, ErrPathExists) != ReasonPathExists {
		t.Fatal("sentinel status")
	}
}

func TestReasonMessageCovered(t *testing.T) {
	for r := range reasonMessages {
		if r.Message() == "" || r.Message() == string(r) {
			t.Errorf("reason %q has no message", r)
		}
	}
	if Reason("zzz").Message() != "zzz" {
		t.Fatal("unknown reason fallback")
	}
}
