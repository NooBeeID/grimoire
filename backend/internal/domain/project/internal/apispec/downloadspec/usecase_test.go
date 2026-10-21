package downloadspec

import (
	"context"
	"errors"
	"testing"

	"grimoire/internal/domain/project/internal/apispec/base"
	"grimoire/lib/common/errs"
	"grimoire/lib/common/reqctx"
)

type fakeGuard struct {
	err   error
	calls int
}

func (g *fakeGuard) RequireMember(context.Context, string) error { g.calls++; return g.err }

type fakeFinder struct {
	spec       base.Spec
	found      bool
	err        error
	calls      int
	gotP, gotN string
}

func (f *fakeFinder) FindSpec(_ context.Context, p, n string) (base.Spec, bool, error) {
	f.calls++
	f.gotP, f.gotN = p, n
	return f.spec, f.found, f.err
}

func authCtx() context.Context {
	return reqctx.WithIdentity(context.Background(), reqctx.Identity{UserID: "u1", Role: "developer", Email: "a@tim.id"})
}

func spec(content, format string) base.Spec {
	return base.Spec{Name: "payment", Format: format, Content: content}
}

func TestExecute_OKByteExact(t *testing.T) {
	content := "a: 1\r\n\tb: \"héllo ✓\"\r\nlast"
	f := &fakeFinder{found: true, spec: spec(content, "yaml")}
	got, err := NewUsecase(f, &fakeGuard{}).Execute(authCtx(), Params{ProjectID: "p1", Name: "payment"})
	if err != nil || got.Content != content || got.FileName != "payment.yaml" || got.MediaType != "application/yaml; charset=utf-8" {
		t.Fatalf("got=%+v err=%v", got, err)
	}
	if f.gotP != "p1" || f.gotN != "payment" {
		t.Fatalf("args %q %q", f.gotP, f.gotN)
	}
}

func TestExecute_NoIdentity(t *testing.T) {
	g, f := &fakeGuard{}, &fakeFinder{found: true}
	_, err := NewUsecase(f, g).Execute(context.Background(), Params{})
	var e *errs.Error
	if !errors.As(err, &e) || e.Code != errs.CodeUnauthorized || g.calls != 0 || f.calls != 0 {
		t.Fatalf("err=%v", err)
	}
}

func TestExecute_GuardErrors(t *testing.T) {
	infra := errors.New("bus down")
	for _, want := range []error{base.ErrProjectNotFound, infra} {
		g, f := &fakeGuard{err: want}, &fakeFinder{found: true}
		_, err := NewUsecase(f, g).Execute(authCtx(), Params{ProjectID: "p1", Name: "x"})
		if !errors.Is(err, want) || f.calls != 0 {
			t.Fatalf("err=%v finder=%d", err, f.calls)
		}
	}
}

func TestExecute_NotFound(t *testing.T) {
	_, err := NewUsecase(&fakeFinder{}, &fakeGuard{}).Execute(authCtx(), Params{})
	if !errors.Is(err, base.ErrSpecNotFound) {
		t.Fatalf("err=%v", err)
	}
}

func TestExecute_FinderError(t *testing.T) {
	_, err := NewUsecase(&fakeFinder{err: errors.New("db")}, &fakeGuard{}).Execute(authCtx(), Params{})
	var e *errs.Error
	if err == nil || errors.As(err, &e) || err.Error()[:len("downloadspec: find spec:")] != "downloadspec: find spec:" {
		t.Fatalf("err=%v", err)
	}
}
