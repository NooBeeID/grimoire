package bustoken

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"grimoire/internal/domain/core/internal/gitcredential/base"
	"grimoire/internal/infra/app"
	"grimoire/internal/infra/bus"
	"grimoire/internal/infra/crypto/secretbox"
	"grimoire/lib/common/errs"
	"grimoire/lib/constants"
	"grimoire/lib/contracts/core/gitcredential/gettoken"
)

const (
	canary = "ghp_CANARY0123456789abcdefCANARY"
	uid    = "0192f5a0-aaaa-7aaa-8bbb-00000000000a"
	uid2   = "0192f5a0-aaaa-7aaa-8bbb-00000000000b"
)

type fakeRepo struct {
	blob  []byte
	found bool
	err   error
	calls int
	u     string
	p     base.Provider
	h     string
}

func (f *fakeRepo) Get(_ context.Context, userID string, p base.Provider, host string) ([]byte, bool, error) {
	f.calls++
	f.u, f.p, f.h = userID, p, host
	return f.blob, f.found, f.err
}

func key(b byte) []byte { return bytes.Repeat([]byte{b}, 32) }

func newBox(t *testing.T, b byte) *secretbox.Box {
	t.Helper()
	box, err := secretbox.New(key(b))
	if err != nil {
		t.Fatal(err)
	}
	return box
}

func seal(t *testing.T, box *secretbox.Box, user string, host string) []byte {
	t.Helper()
	blob, err := box.Seal([]byte(canary), base.AAD(user, base.ProviderGitHub, host))
	if err != nil {
		t.Fatal(err)
	}
	return blob
}

// newBus membangun registry lewat Init sungguhan; box nil diuji sebagai jebakan nil-interface.
func newBus(repo tokenReader, box *secretbox.Box, logBuf *bytes.Buffer) *bus.Registry {
	lg := slog.New(slog.NewJSONHandler(logBuf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	r := bus.NewRegistry(lg)
	var o opener
	if box != nil {
		o = box
	}
	r.Register(constants.BUS_CORE_GITCREDENTIAL_GET_TOKEN, NewHandler(NewUsecase(repo, o, lg)).Handle)
	return r
}

func call(r *bus.Registry, req gettoken.Request) (gettoken.Response, error) {
	return bus.CallTyped[gettoken.Response](context.Background(), r, constants.BUS_CORE_GITCREDENTIAL_GET_TOKEN, req)
}

var okReq = gettoken.Request{UserID: uid, Provider: "github", Host: "github.com"}

func noLeak(t *testing.T, logs *bytes.Buffer, blob []byte, res any) {
	t.Helper()
	s := logs.String()
	if strings.Contains(s, canary) || (len(blob) > 0 && (strings.Contains(s, hex.EncodeToString(blob)) || strings.Contains(s, base64.StdEncoding.EncodeToString(blob)))) {
		t.Fatalf("log bocor: %s", s)
	}
	if res != nil {
		if out := fmt.Sprintf("%v %+v %#v", res, res, res); strings.Contains(out, canary) {
			t.Fatalf("format bocor: %s", out)
		}
	}
}

func TestBadRequestType(t *testing.T) {
	repo := &fakeRepo{}
	r := newBus(repo, newBox(t, 1), &bytes.Buffer{})
	for _, req := range []any{"string", &gettoken.Request{}, struct{}{}} {
		_, err := r.Call(context.Background(), constants.BUS_CORE_GITCREDENTIAL_GET_TOKEN, req)
		if !errors.Is(err, bus.ErrBadRequestType) || !strings.Contains(err.Error(), "gettoken.Request") {
			t.Fatalf("%T: %v", req, err)
		}
	}
	if repo.calls != 0 {
		t.Fatal("repo dipanggil")
	}
}

func TestSuccess(t *testing.T) {
	box := newBox(t, 1)
	blob := seal(t, box, uid, "github.com")
	repo := &fakeRepo{blob: blob, found: true}
	var logs bytes.Buffer
	res, err := call(newBus(repo, box, &logs), okReq)
	if err != nil || !res.Found || res.Unreadable || res.Disabled || res.Token.Reveal() != canary {
		t.Fatalf("%+v %v", res, err)
	}
	if repo.u != uid || repo.p != base.ProviderGitHub || repo.h != "github.com" {
		t.Fatalf("%+v", repo)
	}
	noLeak(t, &logs, blob, res)
}

func TestNotFound(t *testing.T) {
	repo := &fakeRepo{}
	res, err := call(newBus(repo, newBox(t, 1), &bytes.Buffer{}), okReq)
	if err != nil || res.Found || !res.Token.IsZero() || res.Unreadable || res.Disabled {
		t.Fatalf("%+v %v", res, err)
	}
}

func TestUnreadable(t *testing.T) {
	boxA, boxB := newBox(t, 1), newBox(t, 2)
	cases := map[string][]byte{
		"kunci beda": seal(t, boxA, uid, "github.com"),
		"user lain":  seal(t, boxB, uid2, "github.com"),
		"host lain":  seal(t, boxB, uid, "other.com"),
		"rusak":      {1, 2, 3},
	}
	for name, blob := range cases {
		t.Run(name, func(t *testing.T) {
			var logs bytes.Buffer
			res, err := call(newBus(&fakeRepo{blob: blob, found: true}, boxB, &logs), okReq)
			if err != nil || !res.Found || !res.Unreadable || !res.Token.IsZero() || res.Disabled {
				t.Fatalf("%+v %v", res, err)
			}
			s := logs.String()
			for _, w := range []string{`"level":"WARN"`, uid, "github", "github.com"} {
				if !strings.Contains(s, w) {
					t.Fatalf("log tanpa %q: %s", w, s)
				}
			}
			noLeak(t, &logs, blob, res)
		})
	}
}

func TestDisabled(t *testing.T) {
	repo := &fakeRepo{found: true, blob: []byte("x")}
	res, err := call(newBus(repo, nil, &bytes.Buffer{}), okReq)
	if err != nil || !res.Disabled || res.Found || repo.calls != 0 {
		t.Fatalf("%+v %v %d", res, err, repo.calls)
	}
}

// Init dengan Shared{Box: nil} harus nonaktif (bukan nil-pointer di dalam interface).
func TestInitNilBoxIsDisabled(t *testing.T) {
	r := bus.NewRegistry(slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil)))
	Init(app.Deps{Bus: r, Log: slog.Default()}, base.Shared{Box: nil})
	res, err := call(r, okReq)
	if err != nil || !res.Disabled {
		t.Fatalf("%+v %v", res, err)
	}
}

func TestValidation(t *testing.T) {
	bad := []gettoken.Request{
		{UserID: "", Provider: "github", Host: "github.com"},
		{UserID: "x", Provider: "github", Host: "github.com"},
		{UserID: "not-a-uuid", Provider: "github", Host: "github.com"},
		{UserID: uid, Provider: "bitbucket", Host: "github.com"},
		{UserID: uid, Provider: "", Host: "github.com"},
		{UserID: uid, Provider: "github", Host: ""},
		{UserID: uid, Provider: "github", Host: "   "},
	}
	for _, withBox := range []bool{true, false} { // validasi mendahului disabled
		for _, req := range bad {
			repo := &fakeRepo{}
			var box *secretbox.Box
			if withBox {
				box = newBox(t, 1)
			}
			_, err := call(newBus(repo, box, &bytes.Buffer{}), req)
			var e *errs.Error
			if !errors.As(err, &e) || e.Code != errs.CodeInvalid || repo.calls != 0 {
				t.Fatalf("%+v: %v calls=%d", req, err, repo.calls)
			}
		}
	}
}

func TestNormalization(t *testing.T) {
	box := newBox(t, 1)
	repo := &fakeRepo{blob: seal(t, box, uid, "github.com"), found: true}
	res, err := call(newBus(repo, box, &bytes.Buffer{}), gettoken.Request{UserID: uid, Provider: "GitHub", Host: " GitHub.com "})
	if err != nil || res.Token.Reveal() != canary || repo.p != base.ProviderGitHub || repo.h != "github.com" {
		t.Fatalf("%+v %v %+v", res, err, repo)
	}
}

func TestInfraError(t *testing.T) {
	boom := errors.New("db down")
	var logs bytes.Buffer
	res, err := call(newBus(&fakeRepo{err: boom}, newBox(t, 1), &logs), okReq)
	if !errors.Is(err, boom) || res.Found || !res.Token.IsZero() {
		t.Fatalf("%+v %v", res, err)
	}
	noLeak(t, &logs, nil, res)
}
