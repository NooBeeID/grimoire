package renamespec

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"grimoire/lib/common/errs"
)

func parse(body string) (string, error) {
	return parseRename(httptest.NewRequest(http.MethodPatch, "/x", strings.NewReader(body)))
}

func TestParseRename_OK(t *testing.T) {
	for body, want := range map[string]string{`{"name":"payment-v2"}`: "payment-v2", `{"name":""}`: ""} {
		got, err := parse(body)
		if err != nil || got != want {
			t.Fatalf("%s: got %q err %v", body, got, err)
		}
	}
}

func TestParseRename_Invalid(t *testing.T) {
	for _, body := range []string{`{}`, `{"name":null}`, `{"name":"x","extra":1}`, `{bad`, ``, `{"name":5}`} {
		_, err := parse(body)
		var e *errs.Error
		if err == nil || !errors.As(err, &e) || e.Code != errs.CodeInvalid {
			t.Fatalf("%q: err=%v", body, err)
		}
	}
	_, err := parse(`{}`)
	var e *errs.Error
	if !errors.As(err, &e) || !strings.Contains(e.Message, "tidak valid") {
		t.Fatalf("err=%v", err)
	}
}
