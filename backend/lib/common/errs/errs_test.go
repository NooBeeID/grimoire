package errs

import (
	"errors"
	"strings"
	"testing"
)

func TestErrs_WrapUnwrap(t *testing.T) {
	err := error(Wrap(CodeNotFound, "x", ErrNotFound))
	if !errors.Is(err, ErrNotFound) {
		t.Fatal("errors.Is should match sentinel")
	}
	var e *Error
	if !errors.As(err, &e) || e.Code != CodeNotFound {
		t.Fatalf("errors.As failed: %v", e)
	}
}

func TestErrs_ErrorStringHidesCause(t *testing.T) {
	e := Wrap(CodeInternal, "gagal", errors.New("secret internal"))
	if strings.Contains(e.Error(), "secret internal") {
		t.Fatal("cause leaked")
	}
	if e.Error() != "INTERNAL: gagal" {
		t.Fatalf("got %q", e.Error())
	}
}

func TestErrs_WithDetails(t *testing.T) {
	e := WithDetails(CodeInvalid, "bad", map[string]string{"name": "wajib"})
	if e.Details == nil || e.Err != nil {
		t.Fatalf("unexpected %+v", e)
	}
}
