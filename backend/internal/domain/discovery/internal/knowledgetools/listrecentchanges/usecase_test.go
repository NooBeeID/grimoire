package listrecentchanges

import (
	"context"
	"errors"
	"testing"
	"time"

	"grimoire/lib/contracts/project/apispec/listspecs"
	"grimoire/lib/contracts/project/knowledge/listchanges"
)

type fakeFiles struct {
	reqs []listchanges.Request
	resp listchanges.Response
	err  error
}

func (f *fakeFiles) ListChanges(_ context.Context, r listchanges.Request) (listchanges.Response, error) {
	f.reqs = append(f.reqs, r)
	return f.resp, f.err
}

type fakeSpecs struct {
	reqs []listspecs.Request
	resp listspecs.Response
	err  error
}

func (f *fakeSpecs) ListSpecs(_ context.Context, r listspecs.Request) (listspecs.Response, error) {
	f.reqs = append(f.reqs, r)
	return f.resp, f.err
}

func TestExecute_KindRouting(t *testing.T) {
	for _, c := range []struct {
		kind         string
		wantF, wantS int
	}{{KindFiles, 1, 0}, {KindSpecs, 0, 1}, {KindAll, 1, 1}} {
		f, s := &fakeFiles{}, &fakeSpecs{}
		_, err := newUsecase(f, s).Execute(context.Background(), "proj-1", Params{Since: now, Kind: c.kind, Limit: 20})
		if err != nil {
			t.Fatal(err)
		}
		if len(f.reqs) != c.wantF || len(s.reqs) != c.wantS {
			t.Errorf("%s: files=%d specs=%d", c.kind, len(f.reqs), len(s.reqs))
		}
		for _, r := range f.reqs {
			if r.ProjectID != "proj-1" {
				t.Errorf("project files = %q", r.ProjectID)
			}
		}
		for _, r := range s.reqs {
			if r.ProjectID != "proj-1" {
				t.Errorf("project specs = %q", r.ProjectID)
			}
		}
	}
}

func TestExecute_ForwardsSincePrefixLimit(t *testing.T) {
	f := &fakeFiles{}
	since := now.Add(-time.Hour)
	_, err := newUsecase(f, &fakeSpecs{}).Execute(context.Background(), "p", Params{Since: since, Kind: KindFiles, Prefix: "docs", Limit: 7})
	if err != nil {
		t.Fatal(err)
	}
	got := f.reqs[0]
	if !got.Since.Equal(since) || got.Prefix != "docs" || got.Limit != 7 {
		t.Fatalf("req = %+v", got)
	}
}

func TestExecute_MergesAndFiltersSpecs(t *testing.T) {
	f := &fakeFiles{resp: listchanges.Response{Items: []listchanges.Item{{Path: "a.md", CreatedAt: now, UpdatedAt: now}}, Total: 1}}
	s := &fakeSpecs{resp: listspecs.Response{Items: []listspecs.Spec{
		{Name: "old", UpdatedAt: now.Add(-48 * time.Hour)}, {Name: "new", UpdatedAt: now},
	}}}
	page, err := newUsecase(f, s).Execute(context.Background(), "p", Params{Since: now.Add(-time.Hour), Kind: KindAll, Limit: 20})
	if err != nil || page.Total != 2 || len(page.Items) != 2 {
		t.Fatalf("%+v err=%v", page, err)
	}
}

func TestExecute_PropagatesErrors(t *testing.T) {
	boom := errors.New("boom")
	if _, err := newUsecase(&fakeFiles{err: boom}, &fakeSpecs{}).Execute(context.Background(), "p", Params{Since: now, Kind: KindAll}); !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
	if _, err := newUsecase(&fakeFiles{}, &fakeSpecs{err: boom}).Execute(context.Background(), "p", Params{Since: now, Kind: KindSpecs}); !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
}
