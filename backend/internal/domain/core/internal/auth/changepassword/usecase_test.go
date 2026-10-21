package changepassword

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"grimoire/internal/domain/core/internal/auth/base"
	"grimoire/internal/domain/core/internal/auth/base/audit"
	"grimoire/lib/common/errs"
	"grimoire/lib/common/reqctx"
)

const (
	oldPW = "password-lama-1"
	newPW = "password-baru-22"
)

type countHasher struct {
	base.Hasher
	verify, hash atomic.Int32
}

func (c *countHasher) Verify(e, p string) (bool, error) {
	c.verify.Add(1)
	return c.Hasher.Verify(e, p)
}
func (c *countHasher) Hash(p string) (string, error) { c.hash.Add(1); return c.Hasher.Hash(p) }

type fakeRepo struct {
	user    base.User
	found   bool
	findErr error
	revoked int64
	err     error
	calls   int
	got     ChangeInput

	inv      base.Invitation
	hasInv   bool
	errInv   error
	accepted bool
	invFinds int
}

func (f *fakeRepo) FindUserByID(context.Context, string) (base.User, bool, error) {
	return f.user, f.found, f.findErr
}
func (f *fakeRepo) FindInvitation(context.Context, string) (base.Invitation, bool, error) {
	f.invFinds++
	return f.inv, f.hasInv, f.errInv
}
func (f *fakeRepo) Change(_ context.Context, in ChangeInput) (int64, bool, error) {
	f.calls++
	f.got = in
	return f.revoked, f.accepted, f.err
}

type fakeRec struct{ evs []audit.Event }

func (f *fakeRec) Record(_ context.Context, ev audit.Event) { f.evs = append(f.evs, ev) }

type env struct {
	uc   *Usecase
	repo *fakeRepo
	rec  *fakeRec
	h    *countHasher
	ctx  context.Context
}

func setup(t *testing.T) *env {
	t.Helper()
	h := &countHasher{Hasher: base.Argon2Hasher{Params: base.Argon2Params{MemoryKiB: 64, Iterations: 1, Parallelism: 1}}}
	hash, err := h.Hasher.Hash(oldPW)
	if err != nil {
		t.Fatal(err)
	}
	repo := &fakeRepo{found: true, revoked: 2, user: base.User{ID: "u1", Email: "budi@tim.id", PasswordHash: hash, IsActive: true, MustChangePassword: true}}
	rec := &fakeRec{}
	now := time.Now()
	uc := NewUsecase(repo, h, rec, base.PasswordPolicy{MinLength: 10}, func() time.Time { return now })
	ctx := reqctx.WithIdentity(context.Background(), reqctx.Identity{UserID: "u1", Email: "budi@tim.id"})
	return &env{uc: uc, repo: repo, rec: rec, h: h, ctx: ctx}
}

func in(cur, next string) Input {
	return Input{CurrentPassword: base.Secret(cur), NewPassword: base.Secret(next), CurrentTokenHash: []byte("tok")}
}

func fieldsOf(err error) base.FieldErrors {
	var e *errs.Error
	if !errors.As(err, &e) || e.Code != errs.CodeInvalid {
		return nil
	}
	m, _ := e.Details.(map[string]any)
	fe, _ := m["fields"].(base.FieldErrors)
	return fe
}

func TestChangePassword_Success(t *testing.T) {
	e := setup(t)
	if err := e.uc.Execute(e.ctx, in(oldPW, newPW)); err != nil {
		t.Fatal(err)
	}
	if e.repo.calls != 1 || e.repo.got.OldHash != e.repo.user.PasswordHash || string(e.repo.got.KeepTokenHash) != "tok" || e.repo.got.UserID != "u1" {
		t.Fatalf("%+v", e.repo.got)
	}
	if ok, _ := e.h.Hasher.Verify(e.repo.got.NewHash, newPW); !ok {
		t.Fatal("NewHash tidak memverifikasi password baru")
	}
	if len(e.rec.evs) != 1 || e.rec.evs[0].Action != "auth.password.changed" || e.rec.evs[0].Args["revoked_sessions"] != int64(2) {
		t.Fatalf("%+v", e.rec.evs)
	}
	if e.h.verify.Load() != 1 || e.h.hash.Load() != 1 {
		t.Fatalf("verify=%d hash=%d", e.h.verify.Load(), e.h.hash.Load())
	}
}

func TestChangePassword_ValidationFailures(t *testing.T) {
	cases := []struct {
		name, cur, next, field, msg string
	}{
		{"WrongCurrent", "salah-salah-1", newPW, "current_password", "password lama salah"},
		{"NewTooShort", oldPW, "pendek123", "new_password", "minimal 10 karakter"},
		{"NewEqualsEmail", oldPW, "BUDI@tim.id", "new_password", "password tidak boleh sama dengan email"},
		{"NewEqualsCurrent", oldPW, oldPW, "new_password", "password baru harus berbeda dari password lama"},
		{"CurrentEmpty", "", newPW, "current_password", "wajib diisi"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := setup(t)
			err := e.uc.Execute(e.ctx, in(c.cur, c.next))
			fe := fieldsOf(err)
			if fe[c.field] != c.msg {
				t.Fatalf("fields=%v err=%v", fe, err)
			}
			if e.repo.calls != 0 || len(e.rec.evs) != 0 || e.h.hash.Load() != 0 {
				t.Fatal("tidak boleh ada efek samping")
			}
		})
	}
}

func TestChangePassword_CollectsAllFieldErrors(t *testing.T) {
	e := setup(t)
	err := e.uc.Execute(e.ctx, in("salah-salah-1", "pendek"))
	fe := fieldsOf(err)
	if fe["current_password"] != "password lama salah" || fe["new_password"] != "minimal 10 karakter" || len(fe) != 2 {
		t.Fatalf("%v", fe)
	}
	// current salah + new sama dengan current: pesan "beda dari lama" tidak muncul.
	err = e.uc.Execute(e.ctx, in("salah-salah-1", "salah-salah-1"))
	fe = fieldsOf(err)
	if _, ok := fe["new_password"]; ok {
		t.Fatalf("%v", fe)
	}
}

func TestChangePassword_StaleMapsToConflict(t *testing.T) {
	e := setup(t)
	e.repo.err = ErrStale
	err := e.uc.Execute(e.ctx, in(oldPW, newPW))
	var ee *errs.Error
	if !errors.As(err, &ee) || ee.Code != errs.CodeConflict || len(e.rec.evs) != 0 {
		t.Fatalf("%v", err)
	}
}

func TestChangePassword_RepoErrorWrapped(t *testing.T) {
	e := setup(t)
	e.repo.err = errors.New("boom")
	err := e.uc.Execute(e.ctx, in(oldPW, newPW))
	if err == nil || !strings.Contains(err.Error(), "boom") || strings.Contains(err.Error(), oldPW) || strings.Contains(err.Error(), newPW) || len(e.rec.evs) != 0 {
		t.Fatalf("%v", err)
	}
	e.repo.findErr = errors.New("db down")
	if err := e.uc.Execute(e.ctx, in(oldPW, newPW)); err == nil || !strings.Contains(err.Error(), "db down") {
		t.Fatalf("%v", err)
	}
}

func TestChangePassword_UserInactiveOrMissing(t *testing.T) {
	isUnauth := func(err error) bool {
		var ee *errs.Error
		return errors.As(err, &ee) && ee.Code == errs.CodeUnauthorized
	}
	e := setup(t)
	e.repo.user.IsActive = false
	if err := e.uc.Execute(e.ctx, in(oldPW, newPW)); !isUnauth(err) {
		t.Fatalf("inactive: %v", err)
	}
	e = setup(t)
	e.repo.found = false
	if err := e.uc.Execute(e.ctx, in(oldPW, newPW)); !isUnauth(err) {
		t.Fatalf("missing: %v", err)
	}
	e = setup(t)
	if err := e.uc.Execute(context.Background(), in(oldPW, newPW)); !isUnauth(err) {
		t.Fatalf("no identity: %v", err)
	}
	i := in(oldPW, newPW)
	i.CurrentTokenHash = nil
	if err := e.uc.Execute(e.ctx, i); !isUnauth(err) {
		t.Fatalf("no token: %v", err)
	}
}

func TestChangePassword_AuditDoesNotLeakSecrets(t *testing.T) {
	e := setup(t)
	if err := e.uc.Execute(e.ctx, in(oldPW, newPW)); err != nil {
		t.Fatal(err)
	}
	s := strings.Join([]string{e.rec.evs[0].Action, e.rec.evs[0].Target}, "")
	for k, v := range e.rec.evs[0].Args {
		s += k + strings.TrimSpace(strings.ReplaceAll(strings.ToLower(toStr(v)), " ", ""))
	}
	if strings.Contains(s, oldPW) || strings.Contains(s, newPW) || strings.Contains(s, "argon2") {
		t.Fatal(s)
	}
}

func toStr(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}
