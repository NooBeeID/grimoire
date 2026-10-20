package createuser

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"grimoire/internal/domain/core/internal/auth/base"
	"grimoire/internal/domain/core/internal/auth/base/audit"
	"grimoire/lib/common/errs"
	"grimoire/lib/common/reqctx"
)

type fakeRepo struct {
	calls int
	got   base.User
	err   error
}

func (f *fakeRepo) Insert(_ context.Context, u base.User) (base.User, error) {
	f.calls++
	f.got = u
	if f.err != nil {
		return base.User{}, f.err
	}
	u.ID = "new-id"
	return u, nil
}

type fakeHasher struct {
	in    []string
	err   error
	calls int
}

func (f *fakeHasher) Hash(pw string) (string, error) {
	f.calls++
	f.in = append(f.in, pw)
	if f.err != nil {
		return "", f.err
	}
	return "$argon2id$HASHVALUE", nil
}
func (f *fakeHasher) Verify(string, string) (bool, error) { return true, nil }

type fakeRec struct{ evs []audit.Event }

func (f *fakeRec) Record(_ context.Context, ev audit.Event) { f.evs = append(f.evs, ev) }

type failReader struct{}

func (failReader) Read([]byte) (int, error) { return 0, errors.New("rand down") }

var now = time.Date(2026, 10, 21, 9, 0, 0, 0, time.UTC)

type env struct {
	uc *Usecase
	r  *fakeRepo
	h  *fakeHasher
	a  *fakeRec
}

func setup() env {
	e := env{r: &fakeRepo{}, h: &fakeHasher{}, a: &fakeRec{}}
	rnd := bytes.NewReader(bytes.Repeat([]byte{1, 2, 3, 4, 5, 6, 7, 8}, 64))
	e.uc = NewUsecase(e.r, e.h, e.a, func() time.Time { return now }, rnd, base.PasswordPolicy{MinLength: 10})
	return e
}

func adminCtx() context.Context {
	return reqctx.WithIdentity(context.Background(), reqctx.Identity{UserID: "admin-1", Email: "admin@tim.id", Role: base.RoleSuperAdmin})
}

func valid() Input {
	return Input{Email: "sari@tim.id", Name: "Sari", Role: base.RoleDeveloper, Meta: base.RequestMeta{IP: "10.0.0.5", UserAgent: "ua"}}
}

func fieldsOf(t *testing.T, err error) map[string]string {
	t.Helper()
	var e *errs.Error
	if !errors.As(err, &e) || e.Code != errs.CodeInvalid {
		t.Fatalf("bukan INVALID_ARGUMENT: %v", err)
	}
	d := e.Details.(map[string]any)
	return d["fields"].(base.FieldErrors)
}

func noEffects(t *testing.T, e env) {
	t.Helper()
	if e.h.calls != 0 || e.r.calls != 0 || len(e.a.evs) != 0 {
		t.Fatalf("efek samping: hash=%d repo=%d audit=%d", e.h.calls, e.r.calls, len(e.a.evs))
	}
}

func TestExecute_GeneratesPasswordWhenEmpty(t *testing.T) {
	e := setup()
	res, err := e.uc.Execute(adminCtx(), valid())
	if err != nil {
		t.Fatal(err)
	}
	if res.TemporaryPassword == nil || len(*res.TemporaryPassword) != 16 {
		t.Fatalf("temp=%v", res.TemporaryPassword)
	}
	if len(e.h.in) != 1 || e.h.in[0] != *res.TemporaryPassword {
		t.Fatal("hasher harus menerima password yang sama")
	}
	if e.a.evs[0].Args["generated_password"] != true {
		t.Fatal("generated_password")
	}
}

func TestExecute_UsesProvidedPassword(t *testing.T) {
	e := setup()
	in := valid()
	in.Password = "awal-yang-panjang-1"
	res, err := e.uc.Execute(adminCtx(), in)
	if err != nil {
		t.Fatal(err)
	}
	if res.TemporaryPassword != nil || e.h.in[0] != "awal-yang-panjang-1" || e.a.evs[0].Args["generated_password"] != false {
		t.Fatalf("res=%+v in=%v", res, e.h.in)
	}
}

func TestExecute_PasswordErrors(t *testing.T) {
	cases := map[string]struct {
		pw   string
		want string
	}{
		"short":    {"123456789", "minimal 10 karakter"},
		"equal":    {"SARI@tim.id", "password tidak boleh sama dengan email"},
		"toolong":  {strings.Repeat("a", 257), "maksimal 256 byte"},
		"multibyt": {"ééééééééé", "minimal 10 karakter"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			e := setup()
			in := valid()
			in.Password = base.Secret(c.pw)
			_, err := e.uc.Execute(adminCtx(), in)
			if got := fieldsOf(t, err)["password"]; got != c.want {
				t.Fatalf("got %q want %q", got, c.want)
			}
			noEffects(t, e)
		})
	}
}

func TestExecute_PasswordEqualsEmailEvenWhenOtherFieldInvalid(t *testing.T) {
	e := setup()
	in := valid()
	in.Name = ""
	in.Password = "sari@tim.id"
	_, err := e.uc.Execute(adminCtx(), in)
	f := fieldsOf(t, err)
	if f["name"] == "" || f["password"] != "password tidak boleh sama dengan email" {
		t.Fatalf("%v", f)
	}
}

func TestExecute_CollectsAllFieldErrors(t *testing.T) {
	e := setup()
	_, err := e.uc.Execute(adminCtx(), Input{Email: "x", Name: "", Role: "admin", Password: "short"})
	f := fieldsOf(t, err)
	for _, k := range []string{"email", "name", "role", "password"} {
		if f[k] == "" {
			t.Errorf("field %s kosong: %v", k, f)
		}
	}
	noEffects(t, e)
}

func TestExecute_InvalidEmailFormats(t *testing.T) {
	for _, em := range []string{"", "a", "a@", "Budi <b@x.id>", strings.Repeat("a", 250) + "@x.id"} {
		e := setup()
		in := valid()
		in.Email = em
		_, err := e.uc.Execute(adminCtx(), in)
		if fieldsOf(t, err)["email"] == "" {
			t.Errorf("email %q harus ditolak", em)
		}
		noEffects(t, e)
	}
}

func TestExecute_InvalidName(t *testing.T) {
	for _, n := range []string{"", "   ", strings.Repeat("a", 101), "bu\x00di"} {
		e := setup()
		in := valid()
		in.Name = n
		_, err := e.uc.Execute(adminCtx(), in)
		if fieldsOf(t, err)["name"] == "" {
			t.Errorf("name %q harus ditolak", n)
		}
	}
	e := setup()
	in := valid()
	in.Name = strings.Repeat("é", 100)
	if _, err := e.uc.Execute(adminCtx(), in); err != nil {
		t.Fatalf("100 rune multibyte harus diterima: %v", err)
	}
}

func TestExecute_InvalidRole(t *testing.T) {
	for _, r := range []string{"", "admin", "SUPER_ADMIN"} {
		e := setup()
		in := valid()
		in.Role = r
		_, err := e.uc.Execute(adminCtx(), in)
		if fieldsOf(t, err)["role"] != "role harus super_admin atau developer" {
			t.Errorf("role %q", r)
		}
	}
}

func TestExecute_NormalizesEmail(t *testing.T) {
	e := setup()
	in := valid()
	in.Email = " Budi@Tim.ID "
	in.Name = "  Budi  "
	if _, err := e.uc.Execute(adminCtx(), in); err != nil {
		t.Fatal(err)
	}
	if e.r.got.Email != "budi@tim.id" || e.r.got.Name != "Budi" {
		t.Fatalf("%+v", e.r.got)
	}
}

func TestExecute_SetsFlagsAndCreator(t *testing.T) {
	e := setup()
	in := valid()
	in.Role = base.RoleSuperAdmin
	if _, err := e.uc.Execute(adminCtx(), in); err != nil {
		t.Fatal(err)
	}
	u := e.r.got
	if !u.IsActive || !u.MustChangePassword || u.Role != base.RoleSuperAdmin ||
		u.CreatedBy == nil || *u.CreatedBy != "admin-1" || u.CreatedByLabel == nil || *u.CreatedByLabel != "admin@tim.id" ||
		u.PasswordHash != "$argon2id$HASHVALUE" || !u.CreatedAt.Equal(now) {
		t.Fatalf("%+v", u)
	}
}

func TestExecute_EmailTakenMapsToConflict(t *testing.T) {
	e := setup()
	e.r.err = base.ErrEmailTaken
	_, err := e.uc.Execute(adminCtx(), valid())
	var ee *errs.Error
	if !errors.As(err, &ee) || ee.Code != errs.CodeConflict || ee.Message != "email sudah terdaftar" {
		t.Fatalf("%v", err)
	}
	if len(e.a.evs) != 0 {
		t.Fatal("audit tidak boleh dipanggil")
	}
}

func TestExecute_RepoErrorIsWrapped(t *testing.T) {
	e := setup()
	e.r.err = errors.New("db down")
	in := valid()
	in.Password = "awal-yang-panjang-1"
	_, err := e.uc.Execute(adminCtx(), in)
	var ee *errs.Error
	if err == nil || errors.As(err, &ee) || !strings.HasPrefix(err.Error(), "createuser: insert:") ||
		strings.Contains(err.Error(), "awal-yang-panjang-1") {
		t.Fatalf("%v", err)
	}
	if len(e.a.evs) != 0 {
		t.Fatal("audit tidak boleh dipanggil")
	}
}

func TestExecute_HasherErrorNoLeak(t *testing.T) {
	e := setup()
	e.h.err = errors.New("boom")
	in := valid()
	in.Password = "awal-yang-panjang-1"
	_, err := e.uc.Execute(adminCtx(), in)
	if err == nil || strings.Contains(err.Error(), "awal-yang-panjang-1") || e.r.calls != 0 {
		t.Fatalf("err=%v repo=%d", err, e.r.calls)
	}
}

func TestExecute_RandError(t *testing.T) {
	e := setup()
	e.uc.rand = failReader{}
	_, err := e.uc.Execute(adminCtx(), valid())
	if err == nil || !strings.Contains(err.Error(), "createuser: generate password") || e.r.calls != 0 || e.h.calls != 0 {
		t.Fatalf("err=%v", err)
	}
}

func TestExecute_NoIdentity(t *testing.T) {
	e := setup()
	_, err := e.uc.Execute(context.Background(), valid())
	var ee *errs.Error
	if !errors.As(err, &ee) || ee.Code != errs.CodeUnauthorized {
		t.Fatalf("%v", err)
	}
	noEffects(t, e)
}

func TestExecute_AuditEventShape(t *testing.T) {
	e := setup()
	if _, err := e.uc.Execute(adminCtx(), valid()); err != nil {
		t.Fatal(err)
	}
	if len(e.a.evs) != 1 {
		t.Fatalf("n=%d", len(e.a.evs))
	}
	ev := e.a.evs[0]
	if ev.Action != "user.created" || ev.TargetType != "user" || ev.Target != "new-id" || ev.Status != "ok" ||
		ev.Actor != nil || ev.Meta.IP != "10.0.0.5" || ev.Meta.UserAgent != "ua" || len(ev.Args) != 4 ||
		ev.Args["email"] != "sari@tim.id" || ev.Args["name"] != "Sari" || ev.Args["role"] != "developer" {
		t.Fatalf("%+v", ev)
	}
}

func TestExecute_AuditNeverContainsSecrets(t *testing.T) {
	for _, pw := range []string{"", "awal-yang-panjang-1"} {
		e := setup()
		in := valid()
		in.Password = base.Secret(pw)
		res, err := e.uc.Execute(adminCtx(), in)
		if err != nil {
			t.Fatal(err)
		}
		plain := pw
		if res.TemporaryPassword != nil {
			plain = *res.TemporaryPassword
		}
		b, _ := json.Marshal(e.a.evs[0])
		if strings.Contains(string(b), plain) || strings.Contains(string(b), "HASHVALUE") || strings.Contains(string(b), "argon2id") {
			t.Fatalf("bocor: %s", b)
		}
	}
}

func TestExecute_ResultHasNoHash(t *testing.T) {
	e := setup()
	res, err := e.uc.Execute(adminCtx(), valid())
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(res)
	if strings.Contains(string(b), "password_hash") || strings.Contains(string(b), "HASHVALUE") {
		t.Fatalf("bocor hash: %s", b)
	}
}
