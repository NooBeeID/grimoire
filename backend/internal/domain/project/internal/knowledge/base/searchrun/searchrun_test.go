package searchrun

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"

	"grimoire/internal/domain/project/internal/knowledge/base"
	"grimoire/internal/infra/bus"
	"grimoire/lib/constants"
	"grimoire/lib/contracts/project/searchsynonym/listgroups"
)

type fakeStore struct {
	suggestCalls int
	suggestWords []string
	sugg         map[string][]string
	suggestErr   error
	plan         base.SearchPlan
	searchErr    error
	hits         []base.SearchHit
	total        int
}

func (f *fakeStore) SuggestTerms(_ context.Context, _ string, words []string) (map[string][]string, error) {
	f.suggestCalls++
	f.suggestWords = words
	return f.sugg, f.suggestErr
}

func (f *fakeStore) Search(_ context.Context, _ string, plan base.SearchPlan, _ string, _ int) ([]base.SearchHit, int, error) {
	f.plan = plan
	return f.hits, f.total, f.searchErr
}

type fakeSyn struct {
	groups [][]string
	err    error
}

func (f fakeSyn) Groups(context.Context, string) ([][]string, error) { return f.groups, f.err }

func terms(t *testing.T, q string) []base.Term {
	t.Helper()
	ts, ok := base.ParseTerms(q)
	if !ok {
		t.Fatalf("query %q", q)
	}
	return ts
}

func quiet() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func TestRun_SynonymFailureIsSoft(t *testing.T) {
	var buf bytes.Buffer
	st := &fakeStore{hits: []base.SearchHit{{Path: "a.md"}}, total: 1}
	r := New(st, fakeSyn{err: errors.New("bus down")}, slog.New(slog.NewTextHandler(&buf, nil)))
	out, err := r.Run(context.Background(), "p1", terms(t, "login"), "%", 10)
	if err != nil {
		t.Fatal(err)
	}
	if out.Expanded || out.Total != 1 || len(out.Hits) != 1 {
		t.Fatalf("out = %+v", out)
	}
	if !strings.Contains(buf.String(), "level=WARN") || strings.Contains(buf.String(), "login") {
		t.Fatalf("log harus warn tanpa isi query: %s", buf.String())
	}
}

func TestRun_SynonymExpands(t *testing.T) {
	st := &fakeStore{total: 0}
	r := New(st, fakeSyn{groups: [][]string{{"login", "autentikasi"}}}, quiet())
	out, err := r.Run(context.Background(), "p1", terms(t, "login"), "%", 10)
	if err != nil || !out.Expanded {
		t.Fatalf("out = %+v err=%v", out, err)
	}
	if st.suggestCalls != 0 {
		t.Fatal("kata dengan sinonim bukan kandidat typo")
	}
	if len(st.plan.Groups) != 1 || len(st.plan.Groups[0].Alts) < 2 {
		t.Fatalf("plan = %+v", st.plan)
	}
}

func TestRun_NoTypoCandidatesSkipsSuggest(t *testing.T) {
	st := &fakeStore{}
	r := New(st, fakeSyn{}, quiet())
	if _, err := r.Run(context.Background(), "p1", terms(t, "api 2026"), "%", 10); err != nil {
		t.Fatal(err)
	}
	if st.suggestCalls != 0 {
		t.Fatalf("suggest dipanggil %d kali", st.suggestCalls)
	}
}

func TestRun_TypoApplied(t *testing.T) {
	st := &fakeStore{sugg: map[string][]string{"rollbak": {"rollback"}}}
	r := New(st, fakeSyn{}, quiet())
	out, err := r.Run(context.Background(), "p1", terms(t, "rollbak"), "%", 10)
	if err != nil || !out.Expanded {
		t.Fatalf("out = %+v err=%v", out, err)
	}
	if st.suggestCalls != 1 || len(st.suggestWords) != 1 || st.suggestWords[0] != "rollbak" {
		t.Fatalf("suggest = %d %v", st.suggestCalls, st.suggestWords)
	}
}

func TestRun_StoreErrorsWrapped(t *testing.T) {
	boom := errors.New("boom")
	r := New(&fakeStore{suggestErr: boom}, fakeSyn{}, quiet())
	if _, err := r.Run(context.Background(), "p1", terms(t, "rollbak"), "%", 10); !errors.Is(err, boom) {
		t.Fatalf("suggest err = %v", err)
	}
	r = New(&fakeStore{searchErr: boom}, fakeSyn{}, quiet())
	if _, err := r.Run(context.Background(), "p1", terms(t, "api"), "%", 10); !errors.Is(err, boom) {
		t.Fatalf("search err = %v", err)
	}
}

func TestBusSynonyms(t *testing.T) {
	b := bus.NewRegistry(quiet())
	b.Register(constants.BUS_PROJECT_SEARCHSYNONYM_LIST_GROUPS, func(_ context.Context, req any) (any, error) {
		if req.(listgroups.Request).ProjectID != "p1" {
			t.Errorf("req = %v", req)
		}
		return listgroups.Response{Groups: [][]string{{"a1", "b1"}}}, nil
	})
	g, err := NewBusSynonyms(b).Groups(context.Background(), "p1")
	if err != nil || len(g) != 1 || g[0][1] != "b1" {
		t.Fatalf("groups = %v err=%v", g, err)
	}
	if _, err := NewBusSynonyms(bus.NewRegistry(quiet())).Groups(context.Background(), "p1"); err == nil {
		t.Fatal("handler tidak terdaftar harus error")
	}
}
