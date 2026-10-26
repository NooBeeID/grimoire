package getfileoutline

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"grimoire/internal/domain/discovery/internal/knowledgetools/base"
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

func TestExecute_RequestsWhole(t *testing.T) {
	f := &fakeReader{resp: rf.Response{Found: true, Path: "docs/a.md", Content: "# A\n", TotalBytes: 4}}
	out, err := newUsecase(f, 4096).Execute(context.Background(), "p", Params{Path: "docs/a.md"})
	if err != nil {
		t.Fatal(err)
	}
	want := []rf.Request{{ProjectID: "p", Path: "docs/a.md", Whole: true}}
	if !reflect.DeepEqual(f.reqs, want) {
		t.Fatalf("reqs = %+v", f.reqs)
	}
	if o, ok := out.(base.Outline); !ok || len(o.Headings) != 1 {
		t.Fatalf("out = %#v", out)
	}
}

func TestExecute_NotFound(t *testing.T) {
	f := &fakeReader{resp: rf.Response{Found: false, Similar: []string{"docs/readme.md"}}}
	out, err := newUsecase(f, 4096).Execute(context.Background(), "p", Params{Path: "docs/redme.md"})
	if err != nil {
		t.Fatal(err)
	}
	nf, ok := out.(base.NotFound)
	if !ok || nf.Path != "docs/redme.md" || len(nf.Similar) != 1 {
		t.Fatalf("out = %#v", out)
	}
}

func TestExecute_PropagatesError(t *testing.T) {
	boom := errors.New("boom")
	out, err := newUsecase(&fakeReader{err: boom}, 4096).Execute(context.Background(), "p", Params{Path: "a.md"})
	if out != nil || !errors.Is(err, boom) {
		t.Fatalf("out=%v err=%v", out, err)
	}
}
