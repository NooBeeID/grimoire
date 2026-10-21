package readfile

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"grimoire/internal/domain/discovery/internal/knowledgetools/base"
	"grimoire/lib/common/errs"
	rf "grimoire/lib/contracts/project/knowledge/readfile"
)

type fakeReader struct {
	reqs []rf.Request
	resp rf.Response
	err  error
}

func (f *fakeReader) Read(_ context.Context, req rf.Request) (rf.Response, error) {
	f.reqs = append(f.reqs, req)
	return f.resp, f.err
}

func TestExecute_ForwardsRequest(t *testing.T) {
	f := &fakeReader{resp: rf.Response{Found: true}}
	_, err := newUsecase(f).Execute(context.Background(), "p-1", Params{Path: "docs/a.md", Offset: 50, Limit: 1000})
	if err != nil {
		t.Fatal(err)
	}
	want := []rf.Request{{ProjectID: "p-1", Path: "docs/a.md", Offset: 50, Limit: 1000}}
	if !reflect.DeepEqual(f.reqs, want) {
		t.Fatalf("reqs=%+v", f.reqs)
	}
}

func TestExecute_FoundBecomesChunk(t *testing.T) {
	f := &fakeReader{resp: rf.Response{Found: true, Path: "docs/a.md", Content: "abc", SizeBytes: 3, TotalBytes: 10, NextOffset: 53, Truncated: true}}
	out, err := newUsecase(f).Execute(context.Background(), "p", Params{Path: "docs/a.md", Offset: 50, Limit: 3})
	if err != nil {
		t.Fatal(err)
	}
	c, ok := out.(base.ReadChunk)
	if !ok {
		t.Fatalf("tipe %T", out)
	}
	if c.Offset != 50 || c.Content != "abc" || c.TotalBytes != 10 || c.NextOffset != 53 || !c.Truncated {
		t.Fatalf("chunk=%+v", c)
	}
}

func TestExecute_NotFoundBecomesNotFound(t *testing.T) {
	f := &fakeReader{resp: rf.Response{Found: false, Similar: []string{"a", "b"}}}
	out, err := newUsecase(f).Execute(context.Background(), "p", Params{Path: "docs/redme.md"})
	if err != nil {
		t.Fatal(err)
	}
	nf, ok := out.(base.NotFound)
	if !ok || nf.Path != "docs/redme.md" || !reflect.DeepEqual(nf.Similar, []string{"a", "b"}) {
		t.Fatalf("out=%#v", out)
	}
}

func TestExecute_PropagatesError(t *testing.T) {
	for _, want := range []error{fmt.Errorf("x: %w", errs.ErrInvalid), context.DeadlineExceeded} {
		f := &fakeReader{err: want}
		out, err := newUsecase(f).Execute(context.Background(), "p", Params{Path: "a"})
		if out != nil || !errors.Is(err, want) {
			t.Fatalf("out=%v err=%v", out, err)
		}
	}
}
