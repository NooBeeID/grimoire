package logout

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"grimoire/internal/domain/core/internal/auth/base"
	"grimoire/internal/domain/core/internal/auth/base/audit"
	"grimoire/lib/common/reqctx"
)

type fakeRepo struct {
	n     int64
	err   error
	calls int
	got   []byte
}

func (f *fakeRepo) DeleteSessionByHash(_ context.Context, h []byte) (int64, error) {
	f.calls++
	f.got = h
	return f.n, f.err
}

type fakeRec struct{ evs []audit.Event }

func (f *fakeRec) Record(_ context.Context, ev audit.Event) { f.evs = append(f.evs, ev) }

var secret = []byte("secret")

func setup(n int64, err error) (*Usecase, *fakeRepo, *fakeRec, context.Context) {
	r := &fakeRepo{n: n, err: err}
	rec := &fakeRec{}
	ctx := reqctx.WithIdentity(context.Background(), reqctx.Identity{UserID: "u1", Email: "a@x.id"})
	return NewUsecase(r, rec, base.SessionConfig{Secret: secret}), r, rec, ctx
}

func TestLogout_HashesTokenAndDeletes(t *testing.T) {
	uc, r, _, ctx := setup(1, nil)
	if err := uc.Logout(ctx, Params{Token: "tok"}); err != nil {
		t.Fatal(err)
	}
	if string(r.got) != string(base.HashToken(secret, "tok")) || string(r.got) == "tok" {
		t.Fatalf("hash salah: %x", r.got)
	}
}

func TestLogout_DeletedRecordsAuditOnce(t *testing.T) {
	uc, _, rec, ctx := setup(1, nil)
	_ = uc.Logout(ctx, Params{Token: "tok"})
	if len(rec.evs) != 1 {
		t.Fatalf("n=%d", len(rec.evs))
	}
	e := rec.evs[0]
	if e.Action != "auth.logout" || e.Status != "ok" || e.Target != "u1" || e.TargetType != "user" || len(e.Args) != 0 {
		t.Fatalf("%+v", e)
	}
}

func TestLogout_NoRowsIsIdempotent(t *testing.T) {
	uc, _, rec, ctx := setup(0, nil)
	if err := uc.Logout(ctx, Params{Token: "tok"}); err != nil || len(rec.evs) != 0 {
		t.Fatalf("err=%v audit=%d", err, len(rec.evs))
	}
}

func TestLogout_EmptyTokenSkipsDB(t *testing.T) {
	uc, r, rec, ctx := setup(1, nil)
	if err := uc.Logout(ctx, Params{}); err != nil || r.calls != 0 || len(rec.evs) != 0 {
		t.Fatalf("err=%v calls=%d audit=%d", err, r.calls, len(rec.evs))
	}
}

func TestLogout_RepoErrorWrapped(t *testing.T) {
	src := errors.New("db down")
	uc, _, rec, ctx := setup(0, src)
	err := uc.Logout(ctx, Params{Token: "tok"})
	if !errors.Is(err, src) || len(rec.evs) != 0 {
		t.Fatalf("err=%v audit=%d", err, len(rec.evs))
	}
}

func TestLogout_AuditEventHasNoToken(t *testing.T) {
	uc, _, rec, ctx := setup(1, nil)
	_ = uc.Logout(ctx, Params{Token: "tok-rahasia"})
	b, _ := json.Marshal(rec.evs[0])
	if strings.Contains(string(b), "tok-rahasia") || strings.Contains(string(b), string(base.HashToken(secret, "tok-rahasia"))) {
		t.Fatalf("bocor: %s", b)
	}
}
