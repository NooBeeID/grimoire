package listfiles

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"grimoire/lib/common/errs"
	"grimoire/lib/contracts/project/knowledge/listfiles"
)

type fakeLister struct {
	got  []listfiles.Request
	resp listfiles.Response
	err  error
}

func (f *fakeLister) List(_ context.Context, req listfiles.Request) (listfiles.Response, error) {
	f.got = append(f.got, req)
	return f.resp, f.err
}

func TestExecute_ForwardsRequest(t *testing.T) {
	f := &fakeLister{}
	_, err := newUsecase(f).Execute(context.Background(), "proj-1", Params{Prefix: "docs", Recursive: true, Limit: 2, Offset: 4})
	if err != nil {
		t.Fatal(err)
	}
	want := listfiles.Request{ProjectID: "proj-1", Prefix: "docs", Recursive: true, Limit: 2, Offset: 4}
	if len(f.got) != 1 || f.got[0] != want {
		t.Fatalf("got %+v", f.got)
	}
}

func TestExecute_DefaultParamsSendExplicitLimit200(t *testing.T) {
	f := &fakeLister{}
	_, _ = newUsecase(f).Execute(context.Background(), "p", Params{Limit: 200})
	if f.got[0].Limit != 200 {
		t.Fatalf("limit %d", f.got[0].Limit)
	}
}

func TestExecute_BuildsListPage(t *testing.T) {
	f := &fakeLister{resp: listfiles.Response{
		Items:   []listfiles.Item{{Path: "docs/a.md", SizeBytes: 10, UpdatedAt: time.Now()}},
		Folders: []string{"docs/guides"},
		Total:   7,
	}}
	page, err := newUsecase(f).Execute(context.Background(), "p", Params{Prefix: "docs", Limit: 5})
	if err != nil {
		t.Fatal(err)
	}
	if page.Prefix != "docs" || page.Recursive || page.Offset != 0 || page.Total != 7 || len(page.Folders) != 1 {
		t.Fatalf("page %+v", page)
	}
	page, _ = newUsecase(f).Execute(context.Background(), "p", Params{Prefix: "docs", Limit: 5, Offset: 3})
	if page.Folders != nil {
		t.Fatalf("folder harus tersembunyi pada offset > 0: %v", page.Folders)
	}
}

func TestExecute_PropagatesError(t *testing.T) {
	for _, base := range []error{errs.ErrInvalid, errors.New("db down"), context.DeadlineExceeded} {
		f := &fakeLister{err: fmt.Errorf("wrap: %w", base)}
		page, err := newUsecase(f).Execute(context.Background(), "p", Params{Limit: 1})
		if !errors.Is(err, base) {
			t.Fatalf("errors.Is gagal untuk %v: %v", base, err)
		}
		if page.Total != 0 || page.Items != nil {
			t.Fatalf("page harus nol: %+v", page)
		}
	}
}
