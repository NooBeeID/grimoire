package login

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"grimoire/internal/domain/core/internal/auth/base"
	"grimoire/internal/domain/core/internal/auth/base/audit"
	"grimoire/lib/common/errs"
)

var (
	testNow    = time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	testSecret = []byte("secret-test-secret-test-secret-1")
	absTimeout = 168 * time.Hour
)

type countingHasher struct {
	inner  base.Argon2Hasher
	hashes []string
	err    error
}

func (c *countingHasher) Hash(pw string) (string, error) { return c.inner.Hash(pw) }
func (c *countingHasher) Verify(enc, pw string) (bool, error) {
	c.hashes = append(c.hashes, enc)
	if c.err != nil {
		return false, c.err
	}
	return c.inner.Verify(enc, pw)
}

type fakeRepo struct {
	user      base.User
	found     bool
	throttle  base.LoginThrottle
	policy    base.ThrottlePolicy
	errGet    error
	errFind   error
	errUpdate error
	errDone   error

	finds, updates int
	completed      []base.Session
	completedFor   []string
	foundEmails    []string
}

func (f *fakeRepo) FindUserByEmail(_ context.Context, email string) (base.User, bool, error) {
	f.finds++
	f.foundEmails = append(f.foundEmails, email)
	return f.user, f.found, f.errFind
}
func (f *fakeRepo) GetThrottle(_ context.Context, email string) (base.LoginThrottle, error) {
	t := f.throttle
	t.Email = email
	return t, f.errGet
}
func (f *fakeRepo) UpdateThrottle(_ context.Context, _ string, now time.Time, m func(*base.LoginThrottle)) (base.LoginThrottle, error) {
	f.updates++
	if f.errUpdate != nil {
		return base.LoginThrottle{}, f.errUpdate
	}
	m(&f.throttle)
	return f.throttle, nil
}
func (f *fakeRepo) CompleteLogin(_ context.Context, email, userID string, s base.Session, _ time.Time) error {
	if f.errDone != nil {
		return f.errDone
	}
	f.completed = append(f.completed, s)
	f.completedFor = append(f.completedFor, email)
	f.throttle = base.LoginThrottle{}
	return nil
}

type fakeRec struct{ events []audit.Event }

func (f *fakeRec) Record(_ context.Context, ev audit.Event) { f.events = append(f.events, ev) }

type fixture struct {
	uc     *Usecase
	repo   *fakeRepo
	hasher *countingHasher
	rec    *fakeRec
	logBuf *bytes.Buffer
	dummy  string
	now    *time.Time
}

const goodPW = "rahasia-panjang-1"

func newFixture(t *testing.T) *fixture {
	t.Helper()
	h := &countingHasher{inner: base.Argon2Hasher{Params: base.Argon2Params{MemoryKiB: 64, Iterations: 1, Parallelism: 1}}}
	userHash, err := h.Hash(goodPW)
	if err != nil {
		t.Fatal(err)
	}
	dummy, err := h.Hash("dummy-password-x")
	if err != nil {
		t.Fatal(err)
	}
	pol := base.ThrottlePolicy{MaxFailures: 5, Window: 15 * time.Minute, LockDuration: 15 * time.Minute}
	repo := &fakeRepo{
		found:  true,
		policy: pol,
		user: base.User{ID: "u-1", Email: "budi@tim.id", Name: "Budi", PasswordHash: userHash,
			Role: base.RoleDeveloper, IsActive: true, MustChangePassword: true},
	}
	now := testNow
	rec := &fakeRec{}
	buf := &bytes.Buffer{}
	shared := base.Shared{
		Hasher: h, DummyHash: dummy, Clock: func() time.Time { return now }, Rand: rand.Reader,
		Session:  base.SessionConfig{Secret: testSecret, IdleTimeout: 12 * time.Hour, AbsoluteTimeout: absTimeout},
		Throttle: pol,
	}
	uc := NewUsecase(repo, shared, rec, slog.New(slog.NewJSONHandler(buf, nil)))
	return &fixture{uc: uc, repo: repo, hasher: h, rec: rec, logBuf: buf, dummy: dummy, now: &now}
}

func (f *fixture) run(email, pw string) (Result, error) {
	return f.uc.Execute(context.Background(), Input{Email: email, Password: base.Secret(pw),
		Meta: base.RequestMeta{IP: "10.0.0.5", UserAgent: "UA"}})
}

func asErr(t *testing.T, err error) *errs.Error {
	t.Helper()
	var e *errs.Error
	if !errors.As(err, &e) {
		t.Fatalf("bukan *errs.Error: %v", err)
	}
	return e
}

func TestLogin_Success(t *testing.T) {
	f := newFixture(t)
	res, err := f.run("budi@tim.id", goodPW)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Token) != 43 || res.MaxAge != int(absTimeout.Seconds()) || !res.User.MustChangePassword || res.User.ID != "u-1" {
		t.Fatalf("%+v", res)
	}
	if len(f.repo.completed) != 1 {
		t.Fatalf("CompleteLogin x%d", len(f.repo.completed))
	}
	s := f.repo.completed[0]
	if !bytes.Equal(s.TokenHash, base.HashToken(testSecret, res.Token)) || bytes.Contains(s.TokenHash, []byte(res.Token)) {
		t.Fatal("token hash salah / plaintext bocor")
	}
	if len(f.rec.events) != 1 || f.rec.events[0].Action != "auth.login.success" ||
		f.rec.events[0].Actor.Type != "user" || f.rec.events[0].Actor.ID != "u-1" {
		t.Fatalf("%+v", f.rec.events)
	}
	if len(f.hasher.hashes) != 1 {
		t.Fatalf("verify x%d", len(f.hasher.hashes))
	}
}

func TestLogin_NormalizesEmail(t *testing.T) {
	f := newFixture(t)
	if _, err := f.run("  Budi@Tim.ID ", goodPW); err != nil {
		t.Fatal(err)
	}
	if f.repo.foundEmails[0] != "budi@tim.id" || f.repo.completedFor[0] != "budi@tim.id" {
		t.Fatalf("%v %v", f.repo.foundEmails, f.repo.completedFor)
	}
}

func TestLogin_InputValidation(t *testing.T) {
	cases := []struct{ name, email, pw, field string }{
		{"email kosong", "", goodPW, "email"},
		{"email spasi", "   ", goodPW, "email"},
		{"email 255 byte", strings.Repeat("a", 255), goodPW, "email"},
		{"password kosong", "budi@tim.id", "", "password"},
		{"password 257 byte", "budi@tim.id", strings.Repeat("p", 257), "password"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newFixture(t)
			_, err := f.run(c.email, c.pw)
			e := asErr(t, err)
			if e.Code != errs.CodeInvalid {
				t.Fatalf("code %s", e.Code)
			}
			fields := e.Details.(map[string]any)["fields"].(base.FieldErrors)
			if _, ok := fields[c.field]; !ok {
				t.Fatalf("fields %v", fields)
			}
			if f.repo.finds != 0 || len(f.hasher.hashes) != 0 || len(f.rec.events) != 0 || f.repo.updates != 0 {
				t.Fatal("tidak boleh ada I/O pada validasi gagal")
			}
		})
	}
	f := newFixture(t)
	f.repo.found = false
	if _, err := f.run("bukan-email", goodPW); err != base.ErrInvalidCreds {
		t.Fatalf("format aneh harus lanjut ke 401: %v", err)
	}
}

func TestLogin_UnknownEmail_UsesDummyHash(t *testing.T) {
	f := newFixture(t)
	f.repo.found = false
	f.repo.user = base.User{}
	_, err := f.run("tidakada@tim.id", goodPW)
	if err != base.ErrInvalidCreds {
		t.Fatalf("%v", err)
	}
	if len(f.hasher.hashes) != 1 || f.hasher.hashes[0] != f.dummy {
		t.Fatalf("verify: %v", f.hasher.hashes)
	}
	if f.repo.updates != 1 {
		t.Fatal("throttle harus naik untuk email tak terdaftar")
	}
	ev := f.rec.events[0]
	if len(f.rec.events) != 1 || ev.Action != "auth.login.failed" || ev.Actor.Type != "anonymous" ||
		ev.Actor.Label != "tidakada@tim.id" || ev.Args["reason"] != "invalid_credentials" || ev.Target != "" {
		t.Fatalf("%+v", ev)
	}
}

func TestLogin_WrongPassword(t *testing.T) {
	f := newFixture(t)
	_, err := f.run("budi@tim.id", "salah-salah-1")
	if err != base.ErrInvalidCreds {
		t.Fatal(err)
	}
	if len(f.hasher.hashes) != 1 || f.hasher.hashes[0] != f.repo.user.PasswordHash {
		t.Fatal("verify harus 1x dengan hash user")
	}
	ev := f.rec.events[0]
	if ev.Actor.Type != "user" || ev.Target != "u-1" || ev.Status != "denied" {
		t.Fatalf("%+v", ev)
	}
}

func TestLogin_InactiveUser_SameResponseAsWrongPassword(t *testing.T) {
	f := newFixture(t)
	f.repo.user.IsActive = false
	_, err := f.run("budi@tim.id", goodPW)
	if err != base.ErrInvalidCreds {
		t.Fatalf("%v", err)
	}
	if f.repo.updates != 1 || len(f.repo.completed) != 0 || f.rec.events[0].Args["reason"] != "inactive" {
		t.Fatalf("updates=%d events=%+v", f.repo.updates, f.rec.events)
	}
}

func TestLogin_Locked(t *testing.T) {
	f := newFixture(t)
	until := testNow.Add(10 * time.Minute)
	f.repo.throttle.LockedUntil = &until
	_, err := f.run("budi@tim.id", goodPW)
	e := asErr(t, err)
	if e.Code != base.CodeAccountLocked || e.Details.(map[string]any)["retry_after_seconds"] != 600 {
		t.Fatalf("%+v", e)
	}
	if len(f.hasher.hashes) != 0 || f.repo.finds != 0 || f.repo.updates != 0 {
		t.Fatal("jalur terkunci tidak boleh hash/cari/naikkan counter")
	}
	if len(f.rec.events) != 1 || f.rec.events[0].Args["reason"] != "locked" || f.rec.events[0].Actor.Type != "anonymous" {
		t.Fatalf("%+v", f.rec.events)
	}
}

func TestLogin_LockExpired_AllowsLogin(t *testing.T) {
	for _, off := range []time.Duration{0, -time.Second} {
		f := newFixture(t)
		until := testNow.Add(off)
		f.repo.throttle.LockedUntil = &until
		if _, err := f.run("budi@tim.id", goodPW); err != nil {
			t.Fatalf("off=%v: %v", off, err)
		}
	}
}

func TestLogin_FailureNeverRevealsReason(t *testing.T) {
	var got []*errs.Error
	for i := 0; i < 3; i++ {
		f := newFixture(t)
		switch i {
		case 0:
			f.repo.found = false
		case 1:
		case 2:
			f.repo.user.IsActive = false
		}
		pw := goodPW
		if i == 1 {
			pw = "salah-salah-1"
		}
		_, err := f.run("budi@tim.id", pw)
		got = append(got, asErr(t, err))
	}
	for _, e := range got {
		if e != got[0] || e.Details != nil || strings.Contains(e.Message, "budi") || strings.Contains(e.Message, "inactive") {
			t.Fatalf("%+v", e)
		}
	}
}

func TestLogin_VerifyError_TreatedAsFailure(t *testing.T) {
	f := newFixture(t)
	f.hasher.err = errors.New("format hash rusak")
	_, err := f.run("budi@tim.id", goodPW)
	if err != base.ErrInvalidCreds || f.repo.updates != 1 {
		t.Fatalf("%v updates=%d", err, f.repo.updates)
	}
	logs := f.logBuf.String()
	if !strings.Contains(logs, "login: verifikasi hash gagal") || strings.Contains(logs, "$argon2id$") || strings.Contains(logs, goodPW) {
		t.Fatalf("log: %s", logs)
	}
}

func TestLogin_InfraErrors(t *testing.T) {
	boom := errors.New("db down")
	cases := map[string]func(*fakeRepo){
		"get":      func(r *fakeRepo) { r.errGet = boom },
		"find":     func(r *fakeRepo) { r.errFind = boom },
		"update":   func(r *fakeRepo) { r.errUpdate = boom; r.found = false },
		"complete": func(r *fakeRepo) { r.errDone = boom },
	}
	for name, mod := range cases {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			mod(f.repo)
			_, err := f.run("budi@tim.id", goodPW)
			var e *errs.Error
			if !errors.Is(err, boom) || errors.As(err, &e) {
				t.Fatalf("harus error terbungkus non-klien: %v", err)
			}
			if len(f.rec.events) != 0 {
				t.Fatalf("tidak boleh ada audit: %+v", f.rec.events)
			}
		})
	}
}

func TestLogin_FifthFailureStill401ThenLocked(t *testing.T) {
	f := newFixture(t)
	for i := 1; i <= 5; i++ {
		if _, err := f.run("budi@tim.id", "salah-salah-1"); err != base.ErrInvalidCreds {
			t.Fatalf("percobaan %d: %v", i, err)
		}
	}
	_, err := f.run("budi@tim.id", goodPW)
	if e := asErr(t, err); e.Code != base.CodeAccountLocked {
		t.Fatalf("%v", err)
	}
}

func TestLogin_AuditNeverContainsSecrets(t *testing.T) {
	f := newFixture(t)
	until := testNow.Add(time.Minute)
	_, _ = f.run("budi@tim.id", goodPW) // sukses
	res := f.repo.completed[0]
	_ = res
	_, _ = f.run("budi@tim.id", "salah-salah-1")
	f.repo.found = false
	_, _ = f.run("x@tim.id", "salah-salah-2")
	f.repo.throttle.LockedUntil = &until
	_, _ = f.run("budi@tim.id", goodPW)
	b, _ := json.Marshal(f.rec.events)
	s := string(b)
	for _, bad := range []string{goodPW, "salah-salah", "$argon2id$"} {
		if strings.Contains(s, bad) {
			t.Fatalf("audit memuat %q: %s", bad, s)
		}
	}
	if len(f.rec.events) != 4 {
		t.Fatalf("events=%d", len(f.rec.events))
	}
}

func TestLogin_AuditFailureDoesNotFailLogin(t *testing.T) {
	f := newFixture(t)
	f.uc.audit = silentRec{}
	if _, err := f.run("budi@tim.id", goodPW); err != nil {
		t.Fatal(err)
	}
}

type silentRec struct{}

func (silentRec) Record(context.Context, audit.Event) {}

func TestLogin_PasswordNeverLoggedByFmt(t *testing.T) {
	in := Input{Email: "a@b.c", Password: base.Secret(goodPW)}
	if s := fmt.Sprintf("%v %+v", in, in); strings.Contains(s, goodPW) {
		t.Fatal(s)
	}
}
