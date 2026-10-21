package addmember

import (
	"errors"
	"testing"

	"grimoire/internal/domain/project/internal/management/base"
	"grimoire/lib/common/errs"
)

func TestValidate_EmptyUserID(t *testing.T) {
	for _, v := range []string{"", "   "} {
		r := AddMemberRequest{UserID: v}
		err := r.Validate()
		var e *errs.Error
		if !errors.As(err, &e) || e.Code != errs.CodeInvalid {
			t.Fatalf("%q: %v", v, err)
		}
		m, _ := e.Details.(map[string]any)
		fields, _ := m["fields"].(base.FieldErrors)
		if fields["user_id"] != "wajib diisi" {
			t.Fatalf("%q: %+v", v, e.Details)
		}
	}
}

func TestValidate_TrimsAndIgnoresRole(t *testing.T) {
	r := AddMemberRequest{UserID: " abc ", Role: "bogus"}
	if err := r.Validate(); err != nil || r.UserID != "abc" {
		t.Fatalf("%v %q", err, r.UserID)
	}
}
