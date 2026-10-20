package resetpassword

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
	user      base.User
	found     bool
	findErr   error
	resetErr  error
	revoked   int64
	resets    []ResetInput
	findCalls int
}

func (f *fakeRepo) FindUserByID(_ context.Context, _ string) (base.User, bool, error) {
	f.findCalls++
	return f.user, f.found, f.findErr
}

func (f *fakeRepo) Reset(_ context.Context, in ResetInput) (base.User, int64, error) {
	f.resets = append(f.resets, in)
	if f.resetErr != nil {
		return base.User{}, 0, f.resetErr
	}
	return f.user.WithResetPassword(in.Hash, in.Now), f.revoked, nil
}

type fakeRec struct{ evs []audit.Event }

func (f *fakeRec) Record(_ context.Context, ev audit.Event) { f.evs = append(f.evs, ev) }

type spyHasher struct {
	inner base.Hasher
	got   []string
	err   error
}

func (s *spyHasher) Hash(pw string) (string, error) {
	s.got = append(s.got, pw)
	if s.err != nil {
		return "", s.err
	}
	return s.inner.Hash(pw)
}
func (s *spyHasher) Verify(e, pw string) (bool, error) { return s.inner.Verify(e, pw) }

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("rand rusak") }

var now = time.Date(2026, 10, 21, 10, 0, 0, 0, time.UTC)

type env struct {
	uc  *Usecase
	rp  *fakeRepo
	rec *fakeRec
	h   *spyHasher
	ctx context.Context
}

func setup() env {
	rp := &fakeRepo{
		user:    base.User{ID: "target-1", Email: "sari@tim.id", Name: "Sari", Role: base.RoleDeveloper, IsActive: true, PasswordHash: "old"},
		found:   true,
		revoked: 2,
	}
	rec := &fakeRec{}
	h := &spyHasher{inner: base.Argon2Hasher{Params: base.Argon2Params{MemoryKiB: 64, Iterations: 1, Parallelism: 1}}}
	uc := NewUsecase(rp, h, rec, func() time.Time { return now }, bytes.NewReader(bytes.Repeat([]byte{1, 2, 3, 4, 5, 6, 7, 8}, 64)), base.PasswordPolicy{MinLength: 10})
	ctx := reqctx.WithIdentity(context.Background(), reqctx.Identity{UserID: "admin-1", Email: "admin@tim.id", Role: base.RoleSuperAdmin})
	return env{uc, rp, rec, h, ctx}
}

func fields(t *testing.T, err error) map[string]string {
	t.Helper()
	var e *errs.Error
	if !errors.As(err, &e) || e.Code != errs.CodeInvalid {
		t.Fatalf("bukan invalid: %v", err)
	}
	m, _ := e.Details.(map[string]any)
	fe, _ := m["fields"].(base.FieldErrors)
	return fe
}

func assertCode(t *testing.T, err error, code errs.Code) *errs.Error {
	t.Helper()
	var e *errs.Error
	if !errors.As(err, &e) || e.Code != code {
		t.Fatalf("want %s, got %v", code, err)
	}
	return e
}

func noEffects(t *testing.T, e env) {
	t.Helper()
	if len(e.rp.resets) != 0 || len(e.rec.evs) != 0 {
		t.Fatalf("efek samping: resets=%d audit=%d", len(e.rp.resets), len(e.rec.evs))
	}
}

func TestExecute_GeneratesPasswordWhenEmpty(t *testing.T) {
	e := setup()
	res, err := e.uc.Execute(e.ctx, Input{TargetID: "target-1"})
	if err != nil {
		t.Fatal(err)
	}
	if res.TemporaryPassword == nil || len(*res.TemporaryPassword) != base.GeneratedPasswordLen {
		t.Fatalf("temp=%v", res.TemporaryPassword)
	}
	if len(e.h.got) != 1 || e.h.got[0] != *res.TemporaryPassword {
		t.Fatal("hasher tidak menerima password yang sama")
	}
	if e.rec.evs[0].Args["generated"] != true {
		t.Fatal("generated harus true")
	}
}

func TestExecute_UsesProvidedPassword(t *testing.T) {
	e := setup()
	res, err := e.uc.Execute(e.ctx, Input{TargetID: "target-1", Password: "sementara-baru-2026"})
	if err != nil {
		t.Fatal(err)
	}
	if res.TemporaryPassword != nil || e.h.got[0] != "sementara-baru-2026" || e.rec.evs[0].Args["generated"] != false {
		t.Fatalf("res=%+v hashed=%v args=%v", res, e.h.got, e.rec.evs[0].Args)
	}
	if !res.User.MustChangePassword {
		t.Fatal("must_change_password harus true")
	}
}

func TestExecute_ResetInputShape(t *testing.T) {
	e := setup()
	if _, err := e.uc.Execute(e.ctx, Input{TargetID: "target-1", Password: "sementara-baru-2026"}); err != nil {
		t.Fatal(err)
	}
	if len(e.rp.resets) != 1 {
		t.Fatalf("resets=%d", len(e.rp.resets))
	}
	in := e.rp.resets[0]
	ok, err := e.h.Verify(in.Hash, "sementara-baru-2026")
	if in.UserID != "target-1" || !in.Now.Equal(now) || err != nil || !ok || in.Hash == "sementara-baru-2026" {
		t.Fatalf("%+v ok=%v err=%v", in, ok, err)
	}
}

func TestExecute_PasswordValidation(t *testing.T) {
	cases := []struct{ name, pw, want string }{
		{"TooShort", "123456789", "minimal 10 karakter"},
		{"EqualsTargetEmail", "SARI@tim.id", "password tidak boleh sama dengan email"},
		{"TooLong", strings.Repeat("a", 257), "maksimal 256 byte"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := setup()
			_, err := e.uc.Execute(e.ctx, Input{TargetID: "target-1", Password: base.Secret(c.pw)})
			if got := fields(t, err)["password"]; got != c.want {
				t.Fatalf("got %q", got)
			}
			if len(e.h.got) != 0 {
				t.Fatal("hasher tidak boleh dipanggil")
			}
			noEffects(t, e)
		})
	}
}

func TestExecute_TargetNotFound(t *testing.T) {
	e := setup()
	e.rp.found = false
	_, err := e.uc.Execute(e.ctx, Input{TargetID: "x"})
	assertCode(t, err, errs.CodeNotFound)
	if len(e.h.got) != 0 {
		t.Fatal("hashing terjadi")
	}
	noEffects(t, e)
}

func TestExecute_SelfResetForbidden(t *testing.T) {
	e := setup()
	e.rp.user.ID = "admin-1"
	_, err := e.uc.Execute(e.ctx, Input{TargetID: "admin-1", Password: "short"})
	ce := assertCode(t, err, errs.CodeForbidden)
	if ce.Details.(map[string]any)["reason"] != base.ReasonSelfAction {
		t.Fatalf("details=%v", ce.Details)
	}
	if len(e.h.got) != 0 {
		t.Fatal("hashing terjadi")
	}
	noEffects(t, e)
}

func TestExecute_InactiveTargetAllowed(t *testing.T) {
	e := setup()
	e.rp.user.IsActive = false
	res, err := e.uc.Execute(e.ctx, Input{TargetID: "target-1"})
	if err != nil || res.User.IsActive {
		t.Fatalf("err=%v active=%v", err, res.User.IsActive)
	}
}

func TestExecute_TargetGoneRace(t *testing.T) {
	e := setup()
	e.rp.resetErr = ErrTargetGone
	_, err := e.uc.Execute(e.ctx, Input{TargetID: "target-1"})
	assertCode(t, err, errs.CodeNotFound)
	if len(e.rec.evs) != 0 {
		t.Fatal("audit tidak boleh")
	}
}

func TestExecute_RepoErrorWrapped(t *testing.T) {
	e := setup()
	e.rp.resetErr = errors.New("db down")
	_, err := e.uc.Execute(e.ctx, Input{TargetID: "target-1", Password: "sementara-baru-2026"})
	var ce *errs.Error
	if err == nil || errors.As(err, &ce) || !strings.HasPrefix(err.Error(), "resetpassword: simpan: ") ||
		strings.Contains(err.Error(), "sementara-baru-2026") || strings.Contains(err.Error(), e.rp.resets[0].Hash) {
		t.Fatalf("err=%v", err)
	}
	if len(e.rec.evs) != 0 {
		t.Fatal("audit tidak boleh")
	}
}

func TestExecute_FindErrorWrapped(t *testing.T) {
	e := setup()
	e.rp.findErr = errors.New("db down")
	_, err := e.uc.Execute(e.ctx, Input{TargetID: "target-1"})
	if err == nil || !strings.HasPrefix(err.Error(), "resetpassword: cari user: ") {
		t.Fatalf("err=%v", err)
	}
	noEffects(t, e)
}

func TestExecute_HasherErrorNoLeak(t *testing.T) {
	e := setup()
	e.h.err = errors.New("hash gagal")
	_, err := e.uc.Execute(e.ctx, Input{TargetID: "target-1", Password: "sementara-baru-2026"})
	if err == nil || !strings.HasPrefix(err.Error(), "resetpassword: hash: ") || strings.Contains(err.Error(), "sementara-baru-2026") {
		t.Fatalf("err=%v", err)
	}
	noEffects(t, e)
}

func TestExecute_RandError(t *testing.T) {
	e := setup()
	e.uc.rand = errReader{}
	_, err := e.uc.Execute(e.ctx, Input{TargetID: "target-1"})
	if err == nil || !strings.HasPrefix(err.Error(), "resetpassword: generate password: ") {
		t.Fatalf("err=%v", err)
	}
	noEffects(t, e)
}

func TestExecute_NoIdentity(t *testing.T) {
	e := setup()
	_, err := e.uc.Execute(context.Background(), Input{TargetID: "target-1"})
	assertCode(t, err, errs.CodeUnauthorized)
	if e.rp.findCalls != 0 {
		t.Fatal("repo tidak boleh dipanggil")
	}
	noEffects(t, e)
}

func TestExecute_AuditEventShape(t *testing.T) {
	e := setup()
	meta := base.RequestMeta{IP: "10.0.0.5", UserAgent: "ua"}
	if _, err := e.uc.Execute(e.ctx, Input{TargetID: "target-1", Meta: meta}); err != nil {
		t.Fatal(err)
	}
	if len(e.rec.evs) != 1 {
		t.Fatalf("audit=%d", len(e.rec.evs))
	}
	ev := e.rec.evs[0]
	if ev.Action != "auth.password.reset" || ev.TargetType != "user" || ev.Target != "target-1" ||
		ev.Status != "ok" || ev.Meta != meta || ev.Actor != nil || len(ev.Args) != 2 ||
		ev.Args["generated"] != true || ev.Args["revoked_sessions"] != int64(2) {
		t.Fatalf("%+v", ev)
	}
}

func TestExecute_AuditNeverContainsSecrets(t *testing.T) {
	for _, pw := range []base.Secret{"", "sementara-baru-2026"} {
		e := setup()
		res, err := e.uc.Execute(e.ctx, Input{TargetID: "target-1", Password: pw})
		if err != nil {
			t.Fatal(err)
		}
		b, _ := json.Marshal(e.rec.evs[0])
		plain := string(pw)
		if res.TemporaryPassword != nil {
			plain = *res.TemporaryPassword
		}
		if strings.Contains(string(b), plain) || strings.Contains(string(b), e.rp.resets[0].Hash) {
			t.Fatalf("audit bocor: %s", b)
		}
	}
}

func TestExecute_ResultHasNoHash(t *testing.T) {
	e := setup()
	res, err := e.uc.Execute(e.ctx, Input{TargetID: "target-1"})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(res)
	if strings.Contains(string(b), "password_hash") || strings.Contains(string(b), e.rp.resets[0].Hash) ||
		!strings.Contains(string(b), `"must_change_password":true`) {
		t.Fatalf("%s", b)
	}
}
