package searchknowledge

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"grimoire/lib/common/errs"
	"grimoire/lib/contracts/project/knowledge/search"
)

type ctxKey struct{}

type fakeSearcher struct {
	reqs []search.Request
	ctxs []context.Context
	resp search.Response
	err  error
}

func (f *fakeSearcher) Search(ctx context.Context, req search.Request) (search.Response, error) {
	f.reqs = append(f.reqs, req)
	f.ctxs = append(f.ctxs, ctx)
	return f.resp, f.err
}

func TestExecute_ForwardsRequest(t *testing.T) {
	f := &fakeSearcher{}
	_, err := newUsecase(f).Execute(context.Background(), "proj-1", Params{Query: "rate limit", Prefix: "docs", Limit: 7})
	want := search.Request{ProjectID: "proj-1", Query: "rate limit", Prefix: "docs", Limit: 7}
	if err != nil || len(f.reqs) != 1 || f.reqs[0] != want {
		t.Fatalf("%+v %v", f.reqs, err)
	}
}

func TestExecute_BuildsHits(t *testing.T) {
	f := &fakeSearcher{resp: search.Response{Items: []search.Item{{Path: "a"}, {Path: "b"}}, Total: 5}}
	h, err := newUsecase(f).Execute(context.Background(), "p", Params{Query: "q", Prefix: "docs", Limit: 2})
	if err != nil || h.Query != "q" || h.Prefix != "docs" || len(h.Items) != 2 || h.Total != 5 {
		t.Fatalf("%+v %v", h, err)
	}
}

func TestExecute_PropagatesError(t *testing.T) {
	f := &fakeSearcher{err: fmt.Errorf("x: %w", errs.ErrInvalid)}
	h, err := newUsecase(f).Execute(context.Background(), "p", Params{Query: "q", Limit: 1})
	if !errors.Is(err, errs.ErrInvalid) || h.Query != "" || h.Items != nil {
		t.Fatalf("%+v %v", h, err)
	}
}

func TestExecute_PropagatesCtx(t *testing.T) {
	f := &fakeSearcher{}
	ctx := context.WithValue(context.Background(), ctxKey{}, "v")
	_, _ = newUsecase(f).Execute(ctx, "p", Params{Query: "q", Limit: 1})
	if f.ctxs[0].Value(ctxKey{}) != "v" {
		t.Fatal("ctx tidak diteruskan")
	}
}

func TestExecute_EmptyResponse(t *testing.T) {
	h, err := newUsecase(&fakeSearcher{}).Execute(context.Background(), "p", Params{Query: "q", Limit: 1})
	if err != nil || len(h.Items) != 0 || h.Total != 0 {
		t.Fatalf("%+v %v", h, err)
	}
}
