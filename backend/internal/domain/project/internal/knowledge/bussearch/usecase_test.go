package bussearch

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/google/uuid"

	"grimoire/internal/domain/project/internal/knowledge/base"
	"grimoire/internal/domain/project/internal/knowledge/base/searchrun"
	"grimoire/lib/common/errs"
	"grimoire/lib/contracts/project/knowledge/search"
)

type fakeStore struct {
	hits     []base.SearchHit
	total    int
	expanded bool
	err      error

	calls int
	terms []base.Term
	like  string
	limit int
}

func (f *fakeStore) Run(_ context.Context, _ string, terms []base.Term, like string, limit int) (searchrun.Outcome, error) {
	f.calls++
	f.terms, f.like, f.limit = terms, like, limit
	return searchrun.Outcome{Hits: f.hits, Total: f.total, Expanded: f.expanded}, f.err
}

func pid() string { return uuid.NewString() }

func reasonOf(t *testing.T, err error) base.Reason {
	t.Helper()
	if !errors.Is(err, errs.ErrInvalid) {
		t.Fatalf("bukan ErrInvalid: %v", err)
	}
	var e *errs.Error
	if !errors.As(err, &e) {
		t.Fatalf("bukan *errs.Error: %v", err)
	}
	m, _ := e.Details.(map[string]any)
	r, _ := m["reason"].(base.Reason)
	return r
}

func TestMapsHits(t *testing.T) {
	f := &fakeStore{
		hits:  []base.SearchHit{{Path: "a.md", Score: 0.5, Headline: "Alur \x01login\x02 memakai..."}, {Path: "b.md", Headline: "x"}},
		total: 7,
	}
	res, err := NewUsecase(f).Execute(context.Background(), search.Request{ProjectID: pid(), Query: "login"})
	if err != nil {
		t.Fatal(err)
	}
	if len(f.terms) != 1 || f.terms[0].Words[0] != "login" || f.like != "%" || f.limit != 20 {
		t.Fatalf("args: %+v %q %d", f.terms, f.like, f.limit)
	}
	if res.Total != 7 || len(res.Items) != 2 || res.Items[0].Snippet != "Alur **login** memakai..." || res.Items[0].Path != "a.md" || res.Items[0].Score != 0.5 {
		t.Fatalf("%+v", res)
	}
}

func TestEmptyQuery(t *testing.T) {
	for _, q := range []string{"", "   ", "&&&", `"""`} {
		f := &fakeStore{}
		res, err := NewUsecase(f).Execute(context.Background(), search.Request{ProjectID: pid(), Query: q})
		if err != nil || res.Total != 0 || len(res.Items) != 0 || f.calls != 0 {
			t.Fatalf("%q: %+v %v calls=%d", q, res, err, f.calls)
		}
	}
}

func TestInvalidProjectID(t *testing.T) {
	for _, id := range []string{"", "bukan-uuid"} {
		for _, q := range []string{"login", ""} {
			f := &fakeStore{}
			_, err := NewUsecase(f).Execute(context.Background(), search.Request{ProjectID: id, Query: q})
			if reasonOf(t, err) != base.ReasonInvalidProjectID || f.calls != 0 {
				t.Fatalf("id=%q q=%q err=%v", id, q, err)
			}
		}
	}
}

func TestPrefix(t *testing.T) {
	ok := map[string]string{"docs": "docs/%", "/docs": "docs/%", "docs/": "docs/%", "": "%", "a_b": `a\_b/%`}
	for in, want := range ok {
		f := &fakeStore{}
		if _, err := NewUsecase(f).Execute(context.Background(), search.Request{ProjectID: pid(), Query: "x", Prefix: in}); err != nil {
			t.Fatalf("%q: %v", in, err)
		}
		if f.like != want {
			t.Fatalf("%q: like=%q want %q", in, f.like, want)
		}
	}
	for _, in := range []string{"//docs", "../x", "a//b"} {
		f := &fakeStore{}
		_, err := NewUsecase(f).Execute(context.Background(), search.Request{ProjectID: pid(), Query: "x", Prefix: in})
		if !errors.Is(err, errs.ErrInvalid) || f.calls != 0 {
			t.Fatalf("%q: %v calls=%d", in, err, f.calls)
		}
	}
}

func TestLimitClamp(t *testing.T) {
	cases := map[int]int{0: 20, -5: 20, 1: 1, 50: 50, 51: 50, 1000: 50}
	for in, want := range cases {
		f := &fakeStore{}
		if _, err := NewUsecase(f).Execute(context.Background(), search.Request{ProjectID: pid(), Query: "x", Limit: in}); err != nil {
			t.Fatal(err)
		}
		if f.limit != want {
			t.Fatalf("limit %d: got %d want %d", in, f.limit, want)
		}
	}
}

func TestWeirdQueriesNoError(t *testing.T) {
	for _, q := range []string{`"unclosed phrase`, `a & b : c or -d !e`, `'; DROP TABLE x; --`} {
		f := &fakeStore{}
		if _, err := NewUsecase(f).Execute(context.Background(), search.Request{ProjectID: pid(), Query: q}); err != nil {
			t.Fatalf("%q: %v", q, err)
		}
		want, _ := base.ParseTerms(q)
		if !reflect.DeepEqual(f.terms, want) {
			t.Fatalf("%q: terms=%+v want %+v", q, f.terms, want)
		}
	}
}

func TestStoreError(t *testing.T) {
	boom := errors.New("boom")
	_, err := NewUsecase(&fakeStore{err: boom}).Execute(context.Background(), search.Request{ProjectID: pid(), Query: "x"})
	var e *errs.Error
	if !errors.Is(err, boom) || errors.As(err, &e) {
		t.Fatalf("%v", err)
	}
}

func TestNoHits(t *testing.T) {
	res, err := NewUsecase(&fakeStore{}).Execute(context.Background(), search.Request{ProjectID: pid(), Query: "x"})
	if err != nil || len(res.Items) != 0 || res.Total != 0 {
		t.Fatalf("%+v %v", res, err)
	}
}

func TestSectionAndExpandedForwarded(t *testing.T) {
	f := &fakeStore{
		hits:     []base.SearchHit{{Path: "a.md", Headline: "x", Section: "Alur login"}, {Path: "b.md", Headline: "y"}},
		total:    2,
		expanded: true,
	}
	res, err := NewUsecase(f).Execute(context.Background(), search.Request{ProjectID: pid(), Query: "login"})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Expanded || res.Items[0].Section != "Alur login" || res.Items[1].Section != "" {
		t.Fatalf("%+v", res)
	}
}

func TestNotExpandedByDefault(t *testing.T) {
	res, err := NewUsecase(&fakeStore{}).Execute(context.Background(), search.Request{ProjectID: pid(), Query: "x"})
	if err != nil || res.Expanded {
		t.Fatalf("%+v %v", res, err)
	}
}
