package base

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"grimoire/lib/common/reqctx"
)

func TestNormalizeEmail(t *testing.T) {
	got, err := NormalizeEmail(" Budi@Tim.ID ")
	if err != nil || got != "budi@tim.id" {
		t.Fatalf("got %q %v", got, err)
	}
	for _, in := range []string{"a", "a@", "Budi <b@x.id>", strings.Repeat("a", 250) + "@x.id", "a b@x.id"} {
		_, err := NormalizeEmail(in)
		if fe := AsFieldErrors(err); fe["email"] != "format email tidak valid" {
			t.Errorf("%q: %v", in, err)
		}
	}
	if _, err := NormalizeEmail("  "); AsFieldErrors(err)["email"] != "wajib diisi" {
		t.Fatalf("kosong: %v", err)
	}
}

func TestValidateName(t *testing.T) {
	if got, err := ValidateName("  Sari "); err != nil || got != "Sari" {
		t.Fatalf("got %q %v", got, err)
	}
	if _, err := ValidateName(strings.Repeat("é", 100)); err != nil {
		t.Fatalf("100 rune multibyte harus lolos: %v", err)
	}
	cases := map[string]string{
		"": "wajib diisi", "   ": "wajib diisi", strings.Repeat("a", 101): "maksimal 100 karakter",
		"a\x00b": "tidak boleh berisi karakter kontrol", "a\nb": "tidak boleh berisi karakter kontrol",
	}
	for in, msg := range cases {
		if _, err := ValidateName(in); AsFieldErrors(err)["name"] != msg {
			t.Errorf("%q: %v", in, err)
		}
	}
}

func TestValidRole(t *testing.T) {
	if !ValidRole("super_admin") || !ValidRole("developer") || ValidRole("") || ValidRole("admin") {
		t.Fatal("ValidRole salah")
	}
}

func TestUser_ApplyUpdate(t *testing.T) {
	base := User{Name: "Sari", Email: "sari@tim.id", Role: RoleDeveloper}
	s := func(v string) *string { return &v }

	u := base
	changed, err := u.ApplyUpdate(UserPatch{Name: s(" Sari W. "), Email: s("SARI@tim.id "), Role: s(RoleSuperAdmin)})
	if err != nil || !reflect.DeepEqual(changed, []string{"name", "role"}) {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	if u.Name != "Sari W." || u.Email != "sari@tim.id" || u.Role != RoleSuperAdmin {
		t.Fatalf("%+v", u)
	}

	u = base
	changed, err = u.ApplyUpdate(UserPatch{Name: s("Sari"), Email: s("Sari@Tim.ID"), Role: s(RoleDeveloper)})
	if err != nil || len(changed) != 0 || u != base {
		t.Fatalf("no-op: %v %v", changed, err)
	}

	u = base
	changed, err = u.ApplyUpdate(UserPatch{Name: s(""), Email: s("x"), Role: s("root")})
	fe := AsFieldErrors(err)
	if len(fe) != 3 || changed != nil || u != base {
		t.Fatalf("gabungan: %v %v %+v", fe, changed, u)
	}

	u = base
	if changed, err = u.ApplyUpdate(UserPatch{Email: s("baru@tim.id")}); err != nil || !reflect.DeepEqual(changed, []string{"email"}) || u.Email != "baru@tim.id" {
		t.Fatalf("%v %v", changed, err)
	}
}

func TestUser_ApplyStatus(t *testing.T) {
	t0 := time.Unix(100, 0)
	u := User{IsActive: true, UpdatedAt: t0}
	now := t0.Add(time.Hour)
	if !u.ApplyStatus(false, now) || u.IsActive || !u.UpdatedAt.Equal(now) {
		t.Fatalf("%+v", u)
	}
	if !u.ApplyStatus(true, now.Add(time.Hour)) || !u.IsActive {
		t.Fatal("aktifkan")
	}
	before := u.UpdatedAt
	if u.ApplyStatus(true, now.Add(2*time.Hour)) || !u.UpdatedAt.Equal(before) {
		t.Fatal("status sama harus no-op")
	}
}

func TestUser_WithResetPassword(t *testing.T) {
	u := User{ID: "1", PasswordHash: "old", MustChangePassword: false, Name: "A"}
	now := time.Unix(5, 0)
	v := u.WithResetPassword("new", now)
	if v.PasswordHash != "new" || !v.MustChangePassword || !v.UpdatedAt.Equal(now) || v.ID != "1" || v.Name != "A" {
		t.Fatalf("%+v", v)
	}
	if u.PasswordHash != "old" || u.MustChangePassword {
		t.Fatal("asal termutasi")
	}
}

func TestUser_ToResponseHasNoHash(t *testing.T) {
	u := User{ID: "1", Email: "a@b.id", PasswordHash: "$argon2id$secret", Role: RoleDeveloper}
	b, _ := json.Marshal(u.ToResponse())
	if strings.Contains(string(b), "password_hash") || strings.Contains(string(b), "argon2id") {
		t.Fatalf("bocor: %s", b)
	}
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	for _, k := range []string{"id", "email", "name", "role", "is_active", "must_change_password", "created_at", "created_by", "created_by_label", "updated_at", "last_login_at"} {
		if _, ok := m[k]; !ok {
			t.Errorf("kunci %s hilang", k)
		}
	}
}

func TestUser_ToMeAndMeFromIdentity(t *testing.T) {
	want := MeDTO{ID: "1", Email: "a@b.id", Name: "A", Role: RoleDeveloper, MustChangePassword: true}
	if got := (User{ID: "1", Email: "a@b.id", Name: "A", Role: RoleDeveloper, MustChangePassword: true, PasswordHash: "x"}).ToMe(); got != want {
		t.Fatalf("ToMe %+v", got)
	}
	if got := MeFromIdentity(reqctx.Identity{UserID: "1", Email: "a@b.id", Name: "A", Role: RoleDeveloper, MustChangePassword: true}); got != want {
		t.Fatalf("MeFromIdentity %+v", got)
	}
	b, _ := json.Marshal(want)
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	if len(m) != 5 {
		t.Fatalf("kunci: %s", b)
	}
}

func TestUser_CanLogin(t *testing.T) {
	if !(User{IsActive: true}).CanLogin() || (User{}).CanLogin() {
		t.Fatal("CanLogin")
	}
}

func TestNewUser(t *testing.T) {
	now := time.Unix(10, 0)
	u, fe := NewUser(NewUserInput{Email: " Sari@Tim.ID", Name: " Sari ", Role: RoleDeveloper, CreatedBy: "adm", CreatedByLabel: "admin@tim.id", Now: now})
	if fe != nil || u.Email != "sari@tim.id" || u.Name != "Sari" || !u.IsActive || !u.MustChangePassword ||
		u.CreatedBy == nil || *u.CreatedBy != "adm" || *u.CreatedByLabel != "admin@tim.id" ||
		!u.CreatedAt.Equal(now) || !u.UpdatedAt.Equal(now) || u.PasswordHash != "" || u.ID != "" {
		t.Fatalf("%v %+v", fe, u)
	}
	_, fe = NewUser(NewUserInput{Email: "", Name: "", Role: ""})
	if len(fe) != 3 || fe["email"] != "wajib diisi" || fe["name"] != "wajib diisi" || fe["role"] == "" {
		t.Fatalf("%v", fe)
	}
	u, _ = NewUser(NewUserInput{Email: "a@b.id", Name: "A", Role: RoleDeveloper})
	if u.CreatedBy != nil || u.CreatedByLabel != nil {
		t.Fatal("actor kosong harus nil")
	}
}
