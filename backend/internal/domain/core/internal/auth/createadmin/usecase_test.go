package createadmin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"grimoire/internal/domain/core/internal/auth/base"
	"grimoire/internal/domain/core/internal/auth/base/audit"
	"grimoire/lib/common/errs"
	"grimoire/lib/contracts/core/auth/createadmin"
)

type fakeRepo struct {
	calls int
	got   base.User
	err   error
	log   *[]string
}

func (f *fakeRepo) InsertUser(_ context.Context, u base.User) (base.User, error) {
	f.calls++
	f.got = u
	if f.log != nil {
		*f.log = append(*f.log, "insert")
	}
	if f.err != nil {
		return base.User{}, f.err
	}
	u.ID = "new-id"
	return u, nil
}

type fakeHasher struct {
	calls int
	err   error
}

func (f *fakeHasher) Hash(string) (string, error) {
	f.calls++
	if f.err != nil {
		return "", f.err
	}
	return "$argon2id$HASHVALUE", nil
}
func (f *fakeHasher) Verify(string, string) (bool, error) { return true, nil }

type fakeRec struct {
	evs []audit.Event
	log *[]string
}

func (f *fakeRec) Record(_ context.Context, ev audit.Event) {
	f.evs = append(f.evs, ev)
	if f.log != nil {
		*f.log = append(*f.log, "audit")
	}
}

type env struct {
	uc *Usecase
	r  *fakeRepo
	h  *fakeHasher
	a  *fakeRec
}

func setup() env {
	e := env{r: &fakeRepo{}, h: &fakeHasher{}, a: &fakeRec{}}
	e.uc = NewUsecase(e.r, e.h, e.a, base.PasswordPolicy{MinLength: 10})
	return e
}

func validReq() createadmin.Request {
	return createadmin.Request{Email: " Admin@Tim.ID ", Name: "  Budi  ", Password: "kata-sandi-panjang"}
}

func TestCreate_Success(t *testing.T) {
	e := setup()
	res, err := e.uc.Create(context.Background(), validReq())
	if err != nil {
		t.Fatal(err)
	}
	if res.UserID != "new-id" {
		t.Errorf("UserID = %q", res.UserID)
	}
	g := e.r.got
	if g.Role != base.RoleSuperAdmin || !g.IsActive || g.MustChangePassword || g.CreatedBy != nil {
		t.Errorf("entity = %+v", g)
	}
	if g.CreatedByLabel == nil || *g.CreatedByLabel != "cli" {
		t.Errorf("CreatedByLabel = %v", g.CreatedByLabel)
	}
	if g.Email != "admin@tim.id" || g.Name != "Budi" {
		t.Errorf("email/name = %q/%q", g.Email, g.Name)
	}
	if !strings.HasPrefix(g.PasswordHash, "$argon2id$") || g.PasswordHash == "kata-sandi-panjang" {
		t.Errorf("hash = %q", g.PasswordHash)
	}
}

func TestCreate_EmailTaken_ReturnsConflict(t *testing.T) {
	e := setup()
	e.r.err = base.ErrEmailTaken
	_, err := e.uc.Create(context.Background(), validReq())
	var ee *errs.Error
	if !errors.As(err, &ee) || ee.Code != errs.CodeConflict || ee.Message != "email sudah terdaftar" {
		t.Fatalf("err = %v", err)
	}
	if len(e.a.evs) != 0 {
		t.Error("audit tidak boleh dipanggil")
	}
}

func TestCreate_ValidationErrors(t *testing.T) {
	long := strings.Repeat("a", 101)
	cases := []struct {
		name string
		req  createadmin.Request
		keys []string
	}{
		{"email", createadmin.Request{Email: "a@", Name: "N", Password: "kata-sandi-panjang"}, []string{"email"}},
		{"name kosong", createadmin.Request{Email: "a@b.co", Name: "", Password: "kata-sandi-panjang"}, []string{"name"}},
		{"name 101", createadmin.Request{Email: "a@b.co", Name: long, Password: "kata-sandi-panjang"}, []string{"name"}},
		{"pw 9", createadmin.Request{Email: "a@b.co", Name: "N", Password: "123456789"}, []string{"password"}},
		{"pw kosong", createadmin.Request{Email: "a@b.co", Name: "N", Password: ""}, []string{"password"}},
		{"pw = email", createadmin.Request{Email: "admin@tim.id", Name: "N", Password: "ADMIN@tim.id"}, []string{"password"}},
		{"pw 257", createadmin.Request{Email: "a@b.co", Name: "N", Password: strings.Repeat("x", 257)}, []string{"password"}},
		{"gabungan", createadmin.Request{Email: "a@", Name: "N", Password: "pendek"}, []string{"email", "password"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := setup()
			_, err := e.uc.Create(context.Background(), c.req)
			var ee *errs.Error
			if !errors.As(err, &ee) || ee.Code != errs.CodeInvalid {
				t.Fatalf("err = %v", err)
			}
			d := ee.Details.(map[string]any)["fields"].(base.FieldErrors)
			for _, k := range c.keys {
				if d[k] == "" {
					t.Errorf("field %q tidak ada: %v", k, d)
				}
			}
			if e.r.calls != 0 || e.h.calls != 0 || len(e.a.evs) != 0 {
				t.Error("repo/hasher/audit tidak boleh dipanggil")
			}
		})
	}
}

func TestCreate_InfraErrorWrapped(t *testing.T) {
	cause := errors.New("db down")
	e := setup()
	e.r.err = cause
	_, err := e.uc.Create(context.Background(), validReq())
	var ee *errs.Error
	if err == nil || errors.As(err, &ee) || !errors.Is(err, cause) || !strings.Contains(err.Error(), "createadmin: insert user:") {
		t.Fatalf("err = %v", err)
	}
	if len(e.a.evs) != 0 {
		t.Error("audit tidak boleh dipanggil")
	}

	e = setup()
	e.h.err = cause
	_, err = e.uc.Create(context.Background(), validReq())
	if err == nil || !errors.Is(err, cause) || !strings.Contains(err.Error(), "createadmin: hash password:") {
		t.Fatalf("err = %v", err)
	}
	if e.r.calls != 0 {
		t.Error("repo tidak boleh dipanggil")
	}
}

func TestCreate_AuditEvent(t *testing.T) {
	e := setup()
	if _, err := e.uc.Create(context.Background(), validReq()); err != nil {
		t.Fatal(err)
	}
	if len(e.a.evs) != 1 {
		t.Fatalf("evs = %d", len(e.a.evs))
	}
	ev := e.a.evs[0]
	if ev.Action != "user.created" || ev.TargetType != "user" || ev.Target != "new-id" || ev.Status != "ok" {
		t.Errorf("ev = %+v", ev)
	}
	if ev.Actor == nil || *ev.Actor != (audit.Actor{Type: "system", ID: "", Label: "cli"}) {
		t.Errorf("actor = %+v", ev.Actor)
	}
	for _, k := range []string{"email", "name", "role", "generated_password"} {
		if _, ok := ev.Args[k]; !ok {
			t.Errorf("args tanpa %q", k)
		}
	}
	if ev.Args["generated_password"] != false {
		t.Error("generated_password harus false")
	}
}

func TestCreate_AuditNeverContainsSecrets(t *testing.T) {
	e := setup()
	if _, err := e.uc.Create(context.Background(), validReq()); err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(e.a.evs)
	s := string(b) + fmt.Sprintf("%+v", e.a.evs)
	if strings.Contains(s, "kata-sandi-panjang") || strings.Contains(s, "$argon2id$") {
		t.Errorf("rahasia bocor: %s", s)
	}
}

func TestCreate_AuditAfterInsertOnly(t *testing.T) {
	var order []string
	e := setup()
	e.r.log, e.a.log = &order, &order
	if _, err := e.uc.Create(context.Background(), validReq()); err != nil {
		t.Fatal(err)
	}
	if strings.Join(order, ",") != "insert,audit" {
		t.Errorf("order = %v", order)
	}
}
