package resolvesession

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"grimoire/internal/domain/core/internal/auth/base"
	"grimoire/lib/contracts/core/auth/resolvesession"
)

var (
	testSecret = []byte("secret-test")
	testNow    = time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	testToken  = strings.Repeat("a", 43)
	idle       = 30 * time.Minute
)

type fakeRepo struct {
	sess     base.Session
	user     base.User
	found    bool
	findErr  error
	touchErr error
	finds    int
	touches  []time.Time
	gotHash  []byte
	onFind   func(*fakeRepo)
}

func (f *fakeRepo) FindSession(_ context.Context, h []byte) (base.Session, base.User, bool, error) {
	f.finds++
	f.gotHash = h
	if f.onFind != nil {
		f.onFind(f)
	}
	return f.sess, f.user, f.found, f.findErr
}

func (f *fakeRepo) TouchSession(_ context.Context, _ string, now time.Time) (bool, error) {
	f.touches = append(f.touches, now)
	return f.touchErr == nil, f.touchErr
}

func validRepo() *fakeRepo {
	return &fakeRepo{
		found: true,
		sess: base.Session{ID: "s1", UserID: "u1", LastSeenAt: testNow.Add(-10 * time.Second),
			ExpiresAt: testNow.Add(time.Hour)},
		user: base.User{ID: "u1", Email: "budi@tim.id", Name: "Budi", Role: base.RoleDeveloper,
			IsActive: true, MustChangePassword: true},
	}
}

func newUC(r *fakeRepo, log *slog.Logger) *Usecase {
	if log == nil {
		log = slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	}
	return NewUsecase(r, func() time.Time { return testNow },
		base.SessionConfig{Secret: testSecret, IdleTimeout: idle}, log)
}

func TestResolveSession_ValidSession(t *testing.T) {
	r := validRepo()
	got, err := newUC(r, nil).Execute(context.Background(), resolvesession.Request{Token: testToken})
	want := resolvesession.Response{Found: true, UserID: "u1", Email: "budi@tim.id", Name: "Budi",
		Role: base.RoleDeveloper, MustChangePassword: true}
	if err != nil || got != want {
		t.Fatalf("got %+v err %v", got, err)
	}
}

func TestResolveSession_BadTokenLength(t *testing.T) {
	for name, tok := range map[string]string{"empty": "", "42": strings.Repeat("a", 42), "44": strings.Repeat("a", 44)} {
		t.Run(name, func(t *testing.T) {
			r := validRepo()
			got, err := newUC(r, nil).Execute(context.Background(), resolvesession.Request{Token: tok})
			if err != nil || got.Found || r.finds != 0 {
				t.Fatalf("got %+v err %v finds %d", got, err, r.finds)
			}
		})
	}
}

func TestResolveSession_NotFound(t *testing.T) {
	r := validRepo()
	r.found = false
	got, err := newUC(r, nil).Execute(context.Background(), resolvesession.Request{Token: testToken})
	if err != nil || got.Found {
		t.Fatalf("got %+v err %v", got, err)
	}
}

func TestResolveSession_Expired(t *testing.T) {
	for name, exp := range map[string]time.Time{"equal": testNow, "after": testNow.Add(-time.Second)} {
		t.Run(name, func(t *testing.T) {
			r := validRepo()
			r.sess.ExpiresAt = exp
			got, err := newUC(r, nil).Execute(context.Background(), resolvesession.Request{Token: testToken})
			if err != nil || got.Found || len(r.touches) != 0 {
				t.Fatalf("got %+v err %v touches %d", got, err, len(r.touches))
			}
		})
	}
}

func TestResolveSession_IdleTimeout(t *testing.T) {
	cases := map[string]struct {
		last time.Time
		want bool
	}{
		"at idle boundary": {testNow.Add(-idle), false},
		"just inside":      {testNow.Add(-idle + time.Second), true},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			r := validRepo()
			r.sess.LastSeenAt = c.last
			got, err := newUC(r, nil).Execute(context.Background(), resolvesession.Request{Token: testToken})
			if err != nil || got.Found != c.want {
				t.Fatalf("got %+v err %v", got, err)
			}
		})
	}
}

func TestResolveSession_InactiveUser(t *testing.T) {
	r := validRepo()
	r.user.IsActive = false
	r.sess.LastSeenAt = testNow.Add(-5 * time.Minute) // akan butuh touch bila valid
	got, err := newUC(r, nil).Execute(context.Background(), resolvesession.Request{Token: testToken})
	if err != nil || got.Found || len(r.touches) != 0 {
		t.Fatalf("got %+v err %v touches %d", got, err, len(r.touches))
	}
}

func TestResolveSession_HashesToken(t *testing.T) {
	r := validRepo()
	if _, err := newUC(r, nil).Execute(context.Background(), resolvesession.Request{Token: testToken}); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(r.gotHash, base.HashToken(testSecret, testToken)) || bytes.Equal(r.gotHash, []byte(testToken)) {
		t.Fatal("repo harus menerima HMAC token")
	}
	if bytes.Equal(base.HashToken([]byte("lain"), testToken), r.gotHash) {
		t.Fatal("secret berbeda harus menghasilkan hash berbeda")
	}
}

func TestResolveSession_TouchThrottle(t *testing.T) {
	r := validRepo()
	r.sess.LastSeenAt = testNow.Add(-59 * time.Second)
	_, _ = newUC(r, nil).Execute(context.Background(), resolvesession.Request{Token: testToken})
	if len(r.touches) != 0 {
		t.Fatalf("59s: tidak boleh touch, got %d", len(r.touches))
	}
	r = validRepo()
	r.sess.LastSeenAt = testNow.Add(-60 * time.Second)
	_, _ = newUC(r, nil).Execute(context.Background(), resolvesession.Request{Token: testToken})
	if len(r.touches) != 1 || !r.touches[0].Equal(testNow) {
		t.Fatalf("60s: want 1 touch at now, got %v", r.touches)
	}
}

func TestResolveSession_TouchErrorIsNonFatal(t *testing.T) {
	r := validRepo()
	r.sess.LastSeenAt = testNow.Add(-5 * time.Minute)
	r.touchErr = errors.New("db down")
	var buf bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&buf, nil))
	got, err := newUC(r, log).Execute(context.Background(), resolvesession.Request{Token: testToken})
	if err != nil || !got.Found {
		t.Fatalf("got %+v err %v", got, err)
	}
	out := buf.String()
	if !strings.Contains(out, "touch session gagal") || !strings.Contains(out, `"level":"WARN"`) {
		t.Fatalf("log: %s", out)
	}
	if strings.Contains(out, testToken) {
		t.Fatalf("log memuat token: %s", out)
	}
}

func TestResolveSession_FindErrorPropagates(t *testing.T) {
	r := validRepo()
	base0 := errors.New("boom")
	r.findErr = base0
	got, err := newUC(r, nil).Execute(context.Background(), resolvesession.Request{Token: testToken})
	if got.Found || !errors.Is(err, base0) || strings.Contains(err.Error(), testToken) {
		t.Fatalf("got %+v err %v", got, err)
	}
}

func TestResolveSession_FreshRoleAndFlag(t *testing.T) {
	r := validRepo()
	uc := newUC(r, nil)
	first, _ := uc.Execute(context.Background(), resolvesession.Request{Token: testToken})
	r.user.Role = base.RoleSuperAdmin
	r.user.MustChangePassword = false
	second, _ := uc.Execute(context.Background(), resolvesession.Request{Token: testToken})
	if first.Role != base.RoleDeveloper || !first.MustChangePassword ||
		second.Role != base.RoleSuperAdmin || second.MustChangePassword {
		t.Fatalf("first %+v second %+v", first, second)
	}
}
