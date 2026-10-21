package inviteuser

import (
	"errors"
	"strings"
	"testing"

	"grimoire/internal/domain/project/internal/management/base"
	"grimoire/lib/common/errs"
)

func fieldsOf(t *testing.T, err error) base.FieldErrors {
	t.Helper()
	var e *errs.Error
	if !errors.As(err, &e) {
		t.Fatalf("bukan errs.Error: %v", err)
	}
	d, _ := e.Details.(map[string]any)
	f, _ := d["fields"].(base.FieldErrors)
	return f
}

func TestValidate_EmailRules(t *testing.T) {
	for _, em := range []string{"", "   ", strings.Repeat("a", 250) + "@x.id", "a b@x.id"} {
		r := InviteRequest{Email: em}
		err := r.Validate()
		if err == nil {
			t.Fatalf("%q harus ditolak", em)
		}
		if _, ok := fieldsOf(t, err)["email"]; !ok {
			t.Fatalf("%q: fields.email tidak ada", em)
		}
	}
	r := InviteRequest{Email: ""}
	if got := fieldsOf(t, r.Validate())["email"]; got != "wajib diisi" {
		t.Fatalf("%v", got)
	}
}

func TestValidate_Normalizes(t *testing.T) {
	r := InviteRequest{Email: " Baru@Contoh.ID ", Role: "admin", Name: "  Budi "}
	if err := r.Validate(); err != nil {
		t.Fatal(err)
	}
	if r.Email != "baru@contoh.id" || r.Name != "Budi" || r.Role != "admin" {
		t.Fatalf("%+v", r)
	}
}
