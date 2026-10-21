package listapispecs

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"grimoire/internal/infra/bus"
	"grimoire/lib/contracts/project/apispec/listspecs"
)

type fakeLister struct {
	got   listspecs.Request
	calls int
	resp  listspecs.Response
	err   error
	ctx   context.Context
}

func (f *fakeLister) ListSpecs(ctx context.Context, req listspecs.Request) (listspecs.Response, error) {
	f.got, f.ctx = req, ctx
	f.calls++
	return f.resp, f.err
}

const pid = "11111111-1111-4111-8111-111111111111"

func TestExecute_PassesProjectID(t *testing.T) {
	f := &fakeLister{}
	if _, err := newUsecase(f, 100).Execute(context.Background(), pid, Params{}); err != nil {
		t.Fatal(err)
	}
	if f.got != (listspecs.Request{ProjectID: pid}) || f.calls != 1 {
		t.Fatalf("%+v %d", f.got, f.calls)
	}
}

func TestExecute_BuildsCatalogWithBudget(t *testing.T) {
	f := &fakeLister{resp: listspecs.Response{Items: []listspecs.Spec{{Name: "a"}, {Name: "b"}}}}
	cat, err := newUsecase(f, 4321).Execute(context.Background(), pid, Params{})
	if err != nil || cat.Budget != 4321 || len(cat.Items) != 2 || cat.Items[0].Name != "a" {
		t.Fatalf("%+v %v", cat, err)
	}
}

func TestExecute_EmptyResponse(t *testing.T) {
	f := &fakeLister{resp: listspecs.Response{Items: []listspecs.Spec{}}}
	cat, err := newUsecase(f, 100).Execute(context.Background(), pid, Params{})
	if err != nil || len(cat.Items) != 0 {
		t.Fatalf("%+v %v", cat, err)
	}
}

func TestExecute_PortErrorPropagated(t *testing.T) {
	f := &fakeLister{err: fmt.Errorf("x: %w", bus.ErrNotRegistered)}
	cat, err := newUsecase(f, 100).Execute(context.Background(), pid, Params{})
	if !errors.Is(err, bus.ErrNotRegistered) || len(cat.Items) != 0 || cat.Budget != 0 {
		t.Fatalf("%+v %v", cat, err)
	}
}

type ctxKey struct{}

func TestExecute_ContextPassedThrough(t *testing.T) {
	f := &fakeLister{}
	ctx := context.WithValue(context.Background(), ctxKey{}, "v")
	_, _ = newUsecase(f, 100).Execute(ctx, pid, Params{})
	if f.ctx.Value(ctxKey{}) != "v" {
		t.Fatal("ctx hilang")
	}
}
