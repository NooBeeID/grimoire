package search

import (
	"context"
	"errors"
	"strings"
	"testing"

	"grimoire/internal/domain/project/internal/knowledge/base"
)

type fakeAccess struct{ err error }

func (f fakeAccess) Require(context.Context, string) (base.Actor, error) {
	return base.Actor{ID: "u1"}, f.err
}

type fakeRepo struct {
	calls int
	tsq   string
	like  string
	limit int
	hits  []base.SearchHit
	total int
	err   error
}

func (f *fakeRepo) Search(_ context.Context, _, tsq, like string, limit int) ([]base.SearchHit, int, error) {
	f.calls++
	f.tsq, f.like, f.limit = tsq, like, limit
	return f.hits, f.total, f.err
}

func run(r *fakeRepo, in Input) (base.SearchResultDTO, error) {
	return NewUsecase(fakeAccess{}, r).Execute(context.Background(), in)
}

func TestExecute_AccessErrorsSkipRepo(t *testing.T) {
	infra := errors.New("bus down")
	for name, e := range map[string]error{"notfound": base.ErrProjectNotFound, "infra": infra} {
		r := &fakeRepo{}
		_, err := NewUsecase(fakeAccess{e}, r).Execute(context.Background(), Input{ProjectID: "p", Query: "x", Prefix: "../x", Limit: "abc"})
		if !errors.Is(err, e) || r.calls != 0 {
			t.Fatalf("%s: err=%v calls=%d", name, err, r.calls)
		}
	}
}

func TestExecute_EmptyQuery(t *testing.T) {
	for _, q := range []string{"", "   ", `"""`, "&&&"} {
		r := &fakeRepo{}
		res, err := run(r, Input{ProjectID: "p", Query: q})
		if err != nil || r.calls != 0 || res.Items == nil || len(res.Items) != 0 || res.Total != 0 {
			t.Fatalf("%q: %+v %v calls=%d", q, res, err, r.calls)
		}
	}
	// q kosong menang atas prefix tidak valid
	r := &fakeRepo{}
	if _, err := run(r, Input{ProjectID: "p", Prefix: "../x"}); err != nil || r.calls != 0 {
		t.Fatalf("%v %d", err, r.calls)
	}
}

func TestExecute_Defaults(t *testing.T) {
	r := &fakeRepo{}
	if _, err := run(r, Input{ProjectID: "p", Query: "login"}); err != nil {
		t.Fatal(err)
	}
	if r.tsq != `"login"` || r.like != "%" || r.limit != 20 || r.calls != 1 {
		t.Fatalf("%+v", r)
	}
}

func TestExecute_LongQueryTruncated(t *testing.T) {
	r := &fakeRepo{}
	if _, err := run(r, Input{ProjectID: "p", Query: strings.Repeat("a", 300)}); err != nil {
		t.Fatal(err)
	}
	if r.tsq != `"`+strings.Repeat("a", 200)+`"` {
		t.Fatalf("len=%d", len(r.tsq))
	}
}

func TestExecute_WeirdQueriesQuoted(t *testing.T) {
	for _, q := range []string{"a & b : c or -d !e", `"unclosed`} {
		r := &fakeRepo{}
		if _, err := run(r, Input{ProjectID: "p", Query: q}); err != nil || r.calls != 1 {
			t.Fatalf("%q: %v", q, err)
		}
		if !strings.HasPrefix(r.tsq, `"`) || !strings.HasSuffix(r.tsq, `"`) {
			t.Fatalf("%q -> %q", q, r.tsq)
		}
	}
}

func TestExecute_Prefix(t *testing.T) {
	for _, p := range []string{"docs", "docs/"} {
		r := &fakeRepo{}
		if _, err := run(r, Input{ProjectID: "p", Query: "x", Prefix: p}); err != nil || r.like != "docs/%" {
			t.Fatalf("%q: %v %q", p, err, r.like)
		}
	}
	cases := map[string]base.Reason{
		"../x":    base.ReasonPathTraversal,
		"/docs":   base.ReasonPathAbsolute,
		"docs//a": base.ReasonPathEmptySegment,
	}
	for p, want := range cases {
		r := &fakeRepo{}
		_, err := run(r, Input{ProjectID: "p", Query: "x", Prefix: p})
		if err == nil || r.calls != 0 {
			t.Fatalf("%q: %v", p, err)
		}
		if want2 := base.Invalid(want); err.Error() != want2.Error() {
			t.Fatalf("%q: %v want %v", p, err, want2)
		}
	}
}

func TestExecute_Limit(t *testing.T) {
	ok := map[string]int{"": 20, "5": 5, "0": 20, "-3": 20, "51": 50, "1000": 50, " 7 ": 7}
	for in, want := range ok {
		r := &fakeRepo{}
		if _, err := run(r, Input{ProjectID: "p", Query: "x", Limit: in}); err != nil || r.limit != want {
			t.Fatalf("%q: %v limit=%d want %d", in, err, r.limit, want)
		}
	}
	for _, in := range []string{"abc", "1.5", "99999999999999999999"} {
		r := &fakeRepo{}
		_, err := run(r, Input{ProjectID: "p", Query: "x", Limit: in})
		if err == nil || r.calls != 0 || err.Error() != base.Invalid(base.ReasonInvalidLimit).Error() {
			t.Fatalf("%q: %v", in, err)
		}
	}
}

func TestExecute_RepoError(t *testing.T) {
	boom := errors.New("boom")
	_, err := run(&fakeRepo{err: boom}, Input{ProjectID: "p", Query: "x"})
	if !errors.Is(err, boom) || !strings.HasPrefix(err.Error(), "search: query: ") {
		t.Fatalf("%v", err)
	}
}

func TestExecute_MapsHits(t *testing.T) {
	r := &fakeRepo{total: 7, hits: []base.SearchHit{
		{Path: "b.md", Title: "B", Score: 0.5, Headline: "a \x01login\x02 b"},
		{Path: "a.md", Title: "A", Score: 0.1, Headline: ""},
	}}
	res, err := run(r, Input{ProjectID: "p", Query: "login"})
	if err != nil || res.Total != 7 || len(res.Items) != 2 {
		t.Fatalf("%+v %v", res, err)
	}
	if res.Items[0].Path != "b.md" || res.Items[1].Path != "a.md" {
		t.Fatalf("urutan: %+v", res.Items)
	}
	s := res.Items[0].Snippet
	if len(s) != 3 || s[0].Highlight || !s[1].Highlight || s[1].Text != "login" || s[2].Highlight {
		t.Fatalf("%+v", s)
	}
	if res.Items[1].Snippet == nil {
		t.Fatal("snippet nil")
	}
}
