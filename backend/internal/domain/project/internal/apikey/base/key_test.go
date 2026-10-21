package base

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"grimoire/lib/common/errs"
)

func TestValidateName(t *testing.T) {
	cases := []struct {
		name, in, want, msg string
	}{
		{"short", "a", "a", "nama key wajib 2-60 karakter"},
		{"min", "ab", "ab", ""},
		{"max60", strings.Repeat("é", 60), strings.Repeat("é", 60), ""},
		{"over60", strings.Repeat("é", 61), strings.Repeat("é", 61), "nama key wajib 2-60 karakter"},
		{"trim", "  claude  ", "claude", ""},
		{"control", "a\tb", "a\tb", "nama key tidak boleh mengandung karakter kontrol"},
		{"blank", "     ", "", "nama key wajib 2-60 karakter"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, fe := ValidateName(tc.in)
			if tc.msg == "" {
				if fe != nil || got != tc.want {
					t.Fatalf("got %q %v", got, fe)
				}
				return
			}
			if fe["name"] != tc.msg {
				t.Fatalf("fe=%v", fe)
			}
		})
	}
}

func rawFixture() RawKey {
	return RawKey{Full: "gr_abcdefgh_" + strings.Repeat("S", 43), Prefix: "abcdefgh", Secret: strings.Repeat("S", 43)}
}

func TestNewKey(t *testing.T) {
	now := time.Date(2026, 10, 23, 9, 0, 0, 0, time.FixedZone("x", 7*3600))
	raw := rawFixture()
	k, err := NewKey("p1", " ci ", raw, Actor{ID: "u1", Label: "u@x"}, now)
	if err != nil {
		t.Fatal(err)
	}
	if k.Name != "ci" || k.Prefix != "abcdefgh" || k.ProjectID != "p1" || !reflect.DeepEqual(k.SecretHash, HashSecret(raw.Secret)) {
		t.Fatalf("%+v", k)
	}
	if *k.CreatedBy != "u1" || *k.CreatedByLabel != "u@x" || k.CreatedAt.Location() != time.UTC || !k.CreatedAt.Equal(now) {
		t.Fatalf("%+v", k)
	}
	// Tidak ada field string yang memuat secret/raw key.
	v := reflect.ValueOf(k)
	for i := 0; i < v.NumField(); i++ {
		if v.Field(i).Kind() == reflect.String && (v.Field(i).String() == raw.Secret || v.Field(i).String() == raw.Full) {
			t.Fatalf("field %s leaks secret", v.Type().Field(i).Name)
		}
	}
	k2, _ := NewKey("p1", "ci", raw, Actor{ID: "u1"}, now)
	if k2.CreatedByLabel != nil {
		t.Fatal("label must be nil")
	}
}

func TestNewKey_InvalidName(t *testing.T) {
	_, err := NewKey("p1", "x", rawFixture(), Actor{ID: "u"}, time.Now())
	var e *errs.Error
	if !errors.As(err, &e) || e.Code != errs.CodeInvalid {
		t.Fatalf("err=%v", err)
	}
	d := e.Details.(map[string]any)["fields"].(FieldErrors)
	if d["name"] == "" {
		t.Fatalf("%v", d)
	}
}

func TestCheckActiveLimit(t *testing.T) {
	if err := CheckActiveLimit(19, 20); err != nil {
		t.Fatal(err)
	}
	for _, n := range []int{20, 25} {
		err := CheckActiveLimit(n, 20)
		var e *errs.Error
		if !errors.As(err, &e) || e.Code != errs.CodeConflict {
			t.Fatalf("err=%v", err)
		}
		det := e.Details.(map[string]any)
		if det["reason"] != ReasonKeyLimitReached || det["limit"] != 20 {
			t.Fatalf("%v", det)
		}
	}
}

func TestKeyRevoke(t *testing.T) {
	k := Key{ID: "k"}
	if !k.Active() {
		t.Fatal("active")
	}
	now := time.Date(2026, 10, 23, 9, 0, 0, 0, time.FixedZone("x", 3600))
	if err := k.Revoke(Actor{ID: "u1", Label: "u@x"}, now); err != nil {
		t.Fatal(err)
	}
	if k.Active() || k.RevokedAt.Location() != time.UTC || *k.RevokedBy != "u1" || *k.RevokedByLabel != "u@x" {
		t.Fatalf("%+v", k)
	}
	before := k
	err := k.Revoke(Actor{ID: "u2", Label: "o"}, now.Add(time.Hour))
	if !errors.Is(err, ErrAlreadyRevoked) {
		t.Fatalf("err=%v", err)
	}
	if !reflect.DeepEqual(before, k) {
		t.Fatal("struct changed")
	}
}
