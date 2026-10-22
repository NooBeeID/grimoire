package respond

import (
	"fmt"
	"testing"
	"time"

	"grimoire/lib/common/errs"
)

func TestStatusFor_AllCodes(t *testing.T) {
	cases := map[errs.Code]int{
		errs.CodeNotFound: 404, errs.CodeForbidden: 403, errs.CodeConflict: 409, errs.CodeInvalid: 400,
		errs.CodeUnauthorized: 401, errs.CodeCSRFRequired: 403, errs.CodeRateLimited: 429,
		errs.CodePayloadTooLarge: 413, errs.CodeMethodNotAllowed: 405, errs.CodeTimeout: 504,
		errs.CodeUnavailable: 503, errs.CodeInternal: 500, errs.CodePasswordChangeRequired: 403, errs.CodeAlreadyRegistered: 409,
		errs.CodeHostNotAllowed: 422, errs.CodeGitUpstream: 502, errs.CodeGitFeatureDisabled: 503, "TIDAK_DIKENAL": 500,
	}
	for c, want := range cases {
		if got := StatusFor(c); got != want {
			t.Errorf("%s: got %d want %d", c, got, want)
		}
	}
}

func TestRegisterCodeStatus(t *testing.T) {
	code := errs.Code(fmt.Sprintf("TEST_TEAPOT_%d", time.Now().UnixNano()))
	RegisterCodeStatus(code, 418)
	if StatusFor(code) != 418 {
		t.Fatal("kode baru tidak terdaftar")
	}
	mustPanic(t, func() { RegisterCodeStatus(code, 418) })
	mustPanic(t, func() { RegisterCodeStatus(errs.CodeNotFound, 404) })
	mustPanic(t, func() { RegisterCodeStatus("TEST_BAD_STATUS", 200) })
	mustPanic(t, func() { RegisterCodeStatus("TEST_BAD_STATUS", 600) })
}

func mustPanic(t *testing.T, f func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Fatal("harus panic")
		}
	}()
	f()
}
