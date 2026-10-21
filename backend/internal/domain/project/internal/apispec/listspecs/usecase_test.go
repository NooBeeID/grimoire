package listspecs

import (
	"context"
	"errors"
	"testing"

	"grimoire/internal/domain/project/internal/apispec/base"
	"grimoire/lib/common/errs"
	"grimoire/lib/common/reqctx"
)

type fakeGuard struct {
	err     error
	calls   int
	project string
}

func (g *fakeGuard) RequireMember(_ context.Context, p string) error {
	g.calls++
	g.project = p
	return g.err
}

type fakeLister struct {
	items   []base.SpecSummary
	err     error
	calls   int
	project string
}

func (l *fakeLister) ListSpecs(_ context.Context, p string) ([]base.SpecSummary, error) {
	l.calls++
	l.project = p
	return l.items, l.err
}

func authCtx() context.Context {
	return reqctx.WithIdentity(context.Background(), reqctx.Identity{UserID: "u1", Role: "developer", Email: "a@tim.id"})
}

func TestExecute_ReturnsItemsInStoreOrder(t *testing.T) {
	l := &fakeLister{items: []base.SpecSummary{{Name: "auth"}, {Name: "notification"}, {Name: "payment"}}}
	res, err := NewUsecase(&fakeGuard{}, l, 100).Execute(authCtx(), "p1")
	if err != nil || len(res.Items) != 3 || res.Items[0].Name != "auth" || res.Items[2].Name != "payment" || res.Limit != 100 {
		t.Fatalf("res=%+v err=%v", res, err)
	}
}

func TestExecute_EmptyProjectReturnsEmptySliceNotNil(t *testing.T) {
	res, err := NewUsecase(&fakeGuard{}, &fakeLister{}, 5).Execute(authCtx(), "p1")
	if err != nil || res.Items == nil || len(res.Items) != 0 {
		t.Fatalf("res=%+v err=%v", res, err)
	}
}

func TestExecute_GuardCalledBeforeStore(t *testing.T) {
	l := &fakeLister{}
	_, err := NewUsecase(&fakeGuard{err: base.ErrProjectNotFound}, l, 5).Execute(authCtx(), "p1")
	if !errors.Is(err, base.ErrProjectNotFound) || l.calls != 0 {
		t.Fatalf("err=%v calls=%d", err, l.calls)
	}
}

func TestT06_AC7_Execute_NonMemberGets404(t *testing.T) {
	_, err := NewUsecase(&fakeGuard{err: base.ErrProjectNotFound}, &fakeLister{}, 5).Execute(authCtx(), "p1")
	var e *errs.Error
	if !errors.As(err, &e) || e.Code != errs.CodeNotFound || e.Message != "project tidak ditemukan" {
		t.Fatalf("err=%v", err)
	}
}

func TestExecute_GuardInfraErrorPropagates(t *testing.T) {
	boom := errors.New("bus down")
	l := &fakeLister{}
	_, err := NewUsecase(&fakeGuard{err: boom}, l, 5).Execute(authCtx(), "p1")
	if !errors.Is(err, boom) || l.calls != 0 {
		t.Fatalf("err=%v calls=%d", err, l.calls)
	}
}

func TestExecute_NoIdentity(t *testing.T) {
	g, l := &fakeGuard{}, &fakeLister{}
	_, err := NewUsecase(g, l, 5).Execute(context.Background(), "p1")
	var e *errs.Error
	if !errors.As(err, &e) || e.Code != errs.CodeUnauthorized || g.calls != 0 || l.calls != 0 {
		t.Fatalf("err=%v", err)
	}
}

func TestExecute_StoreErrorWrapped(t *testing.T) {
	boom := errors.New("db down")
	_, err := NewUsecase(&fakeGuard{}, &fakeLister{err: boom}, 5).Execute(authCtx(), "p1")
	if !errors.Is(err, boom) || err.Error() != "listspecs: list specs: db down" {
		t.Fatalf("err=%v", err)
	}
}

func TestExecute_PassesProjectIDVerbatim(t *testing.T) {
	g, l := &fakeGuard{}, &fakeLister{}
	if _, err := NewUsecase(g, l, 5).Execute(authCtx(), " bukan-uuid "); err != nil {
		t.Fatal(err)
	}
	if g.project != " bukan-uuid " || l.project != " bukan-uuid " {
		t.Fatalf("guard=%q store=%q", g.project, l.project)
	}
}
