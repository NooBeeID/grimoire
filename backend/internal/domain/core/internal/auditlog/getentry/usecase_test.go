package getentry

import (
	"context"
	"errors"
	"testing"

	"grimoire/internal/domain/core/internal/auditlog/base"
	"grimoire/lib/common/errs"
)

type ctxKey struct{}

type fakeRepo struct {
	entry  base.Entry
	found  bool
	err    error
	calls  int
	gotID  string
	gotCtx context.Context
}

func (f *fakeRepo) Get(ctx context.Context, id string) (base.Entry, bool, error) {
	f.calls++
	f.gotID, f.gotCtx = id, ctx
	return f.entry, f.found, f.err
}

func TestUsecase_Found(t *testing.T) {
	want := base.Entry{ID: validID, Action: "mcp.read_file"}
	uc := NewUsecase(&fakeRepo{entry: want, found: true})
	got, err := uc.Get(context.Background(), Params{ID: validID})
	if err != nil || got.ID != want.ID || got.Action != want.Action {
		t.Fatalf("got %+v err %v", got, err)
	}
}

func TestUsecase_NotFound(t *testing.T) {
	uc := NewUsecase(&fakeRepo{})
	_, err := uc.Get(context.Background(), Params{ID: validID})
	if !errors.Is(err, errs.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestUsecase_RepoErrorWrapped(t *testing.T) {
	sentinel := errors.New("boom")
	uc := NewUsecase(&fakeRepo{err: sentinel})
	_, err := uc.Get(context.Background(), Params{ID: validID})
	if !errors.Is(err, sentinel) || errors.Is(err, errs.ErrNotFound) {
		t.Fatalf("got %v", err)
	}
}

func TestUsecase_PassesCtxAndID(t *testing.T) {
	f := &fakeRepo{found: true}
	ctx := context.WithValue(context.Background(), ctxKey{}, "v")
	_, _ = NewUsecase(f).Get(ctx, Params{ID: validID})
	if f.gotID != validID || f.gotCtx.Value(ctxKey{}) != "v" {
		t.Fatalf("argumen tidak diteruskan: %q", f.gotID)
	}
}
