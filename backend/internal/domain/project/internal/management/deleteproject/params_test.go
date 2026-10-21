package deleteproject

import (
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"grimoire/internal/domain/project/internal/management/base"
	"grimoire/internal/infra/http/respond"
	"grimoire/lib/common/errs"
)

func TestRequest_Validate_MissingField(t *testing.T) {
	err := Request{}.Validate()
	var e *errs.Error
	if !errors.As(err, &e) || e.Code != errs.CodeInvalid {
		t.Fatalf("%v", err)
	}
	d, _ := e.Details.(map[string]any)
	fields, _ := d["fields"].(base.FieldErrors)
	if fields["confirm_name"] != "wajib diisi" {
		t.Fatalf("%+v", e.Details)
	}
}

func TestRequest_Validate_EmptyStringPasses(t *testing.T) {
	s := ""
	if err := (Request{ConfirmName: &s}).Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestRequest_Decode_Errors(t *testing.T) {
	for _, body := range []string{"", "{rusak", `{"name":"x"}`, `{"confirm_name": 5}`} {
		var req Request
		r := httptest.NewRequest("DELETE", "/", strings.NewReader(body))
		err := respond.DecodeJSON(r, &req)
		var e *errs.Error
		if !errors.As(err, &e) || e.Code != errs.CodeInvalid {
			t.Fatalf("%q: %v", body, err)
		}
	}
}
