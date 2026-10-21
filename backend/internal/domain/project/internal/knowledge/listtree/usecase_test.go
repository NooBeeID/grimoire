package listtree

import (
	"context"
	"errors"
	"testing"
	"time"

	"grimoire/internal/domain/project/internal/knowledge/base"
	"grimoire/lib/common/errs"
)

type fakeAccess struct{ err error }

func (f fakeAccess) Require(context.Context, string) (base.Actor, error) {
	return base.Actor{ID: "u1"}, f.err
}

type fakeRepo struct {
	calls int
	like  string
	metas []base.Meta
	err   error
}

func (f *fakeRepo) ListMeta(_ context.Context, _, like string) ([]base.Meta, error) {
	f.calls++
	f.like = like
	return f.metas, f.err
}

func TestExecute_AccessErrorsSkipRepo(t *testing.T) {
	infra := errors.New("bus down")
	for name, e := range map[string]error{"notfound": base.ErrProjectNotFound, "infra": infra} {
		r := &fakeRepo{}
		_, err := NewUsecase(fakeAccess{e}, r).Execute(context.Background(), Input{ProjectID: "p", Prefix: "../x"})
		if !errors.Is(err, e) || r.calls != 0 {
			t.Fatalf("%s: err=%v calls=%d", name, err, r.calls)
		}
	}
}

func TestExecute_InvalidPrefix(t *testing.T) {
	cases := map[string]base.Reason{
		"../x":     base.ReasonPathTraversal,
		"/docs":    base.ReasonPathAbsolute,
		"docs//a":  base.ReasonPathEmptySegment,
		"do\x00cs": base.ReasonPathControlChar,
	}
	for p, want := range cases {
		r := &fakeRepo{}
		_, err := NewUsecase(fakeAccess{}, r).Execute(context.Background(), Input{ProjectID: "p", Prefix: p})
		var e *errs.Error
		if !errors.As(err, &e) || e.Code != errs.CodeInvalid || r.calls != 0 {
			t.Fatalf("%q: err=%v calls=%d", p, err, r.calls)
		}
		d, _ := e.Details.(map[string]any)
		if d["reason"] != want {
			t.Fatalf("%q: details=%v want %s", p, e.Details, want)
		}
	}
}

func TestExecute_LikePrefix(t *testing.T) {
	for p, want := range map[string]string{"": "%", "docs": "docs/%", "docs/": "docs/%"} {
		r := &fakeRepo{}
		if _, err := NewUsecase(fakeAccess{}, r).Execute(context.Background(), Input{ProjectID: "p", Prefix: p}); err != nil {
			t.Fatal(err)
		}
		if r.like != want {
			t.Fatalf("%q: like=%q want %q", p, r.like, want)
		}
	}
}

func TestExecute_RepoError(t *testing.T) {
	r := &fakeRepo{err: errors.New("boom")}
	_, err := NewUsecase(fakeAccess{}, r).Execute(context.Background(), Input{ProjectID: "p"})
	if err == nil || err.Error() != "listtree: list meta: boom" {
		t.Fatalf("err=%v", err)
	}
}

func TestExecute_NilMetasNonNilSlices(t *testing.T) {
	lvl, err := NewUsecase(fakeAccess{}, &fakeRepo{}).Execute(context.Background(), Input{ProjectID: "p", Prefix: "tidak/ada"})
	if err != nil || lvl.Folders == nil || lvl.Files == nil || lvl.TotalFiles != 0 {
		t.Fatalf("%+v %v", lvl, err)
	}
}

func TestExecute_EndToEnd(t *testing.T) {
	ts := time.Date(2026, 10, 24, 9, 0, 0, 0, time.UTC)
	r := &fakeRepo{metas: []base.Meta{
		{Path: "docs/a.md", SizeBytes: 120, UpdatedAt: ts},
		{Path: "docs/auth/login.md", SizeBytes: 5, UpdatedAt: ts},
		{Path: "docs/auth/flow/x.md", SizeBytes: 6, UpdatedAt: ts},
	}}
	lvl, err := NewUsecase(fakeAccess{}, r).Execute(context.Background(), Input{ProjectID: "p", Prefix: "docs"})
	if err != nil {
		t.Fatal(err)
	}
	if lvl.TotalFiles != 3 || len(lvl.Folders) != 1 || lvl.Folders[0].Name != "auth" || lvl.Folders[0].FileCount != 2 ||
		len(lvl.Files) != 1 || lvl.Files[0].Path != "docs/a.md" {
		t.Fatalf("%+v", lvl)
	}
}
