package searchquality

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"grimoire/lib/common/errs"
	"grimoire/lib/contracts/core/audit/record"
	"grimoire/lib/contracts/core/audit/scanentries"
	"grimoire/lib/contracts/discovery/knowledgetools/searchquality"
)

type fakeScanner struct {
	pages []scanentries.Response
	err   error
	reqs  []scanentries.Request
}

func (f *fakeScanner) Scan(_ context.Context, req scanentries.Request) (scanentries.Response, error) {
	f.reqs = append(f.reqs, req)
	if f.err != nil {
		return scanentries.Response{}, f.err
	}
	i := len(f.reqs) - 1
	if i >= len(f.pages) {
		return scanentries.Response{Entries: []scanentries.Entry{}}, nil
	}
	return f.pages[i], nil
}

func newUC(f *fakeScanner) *Usecase {
	u := NewUsecase(f)
	u.now = func() time.Time { return tNow }
	return u
}

func search(id string, at time.Time, actor, q string, paths ...string) scanentries.Entry {
	ps := make([]any, len(paths))
	for i, p := range paths {
		ps[i] = p
	}
	return scanentries.Entry{
		ID: id, OccurredAt: at, ActorID: actor, Action: record.ActionMCPSearchKnowledge, Status: record.StatusOK,
		Args:   map[string]any{"query": q},
		Result: map[string]any{"total": float64(len(paths)), "result_paths": ps},
	}
}

func read(id string, at time.Time, actor, path string) scanentries.Entry {
	return scanentries.Entry{ID: id, OccurredAt: at, ActorID: actor, Action: record.ActionMCPReadFile,
		Target: path, Status: record.StatusOK, Args: map[string]any{}, Result: map[string]any{}}
}

func filler(n int, at time.Time) []scanentries.Entry {
	out := make([]scanentries.Entry, n)
	for i := range out {
		out[i] = read(fmt.Sprintf("f%d", i), at, "other", "x.md")
	}
	return out
}

func cur(id string) *scanentries.Cursor { return &scanentries.Cursor{OccurredAt: tSince, ID: id} }

func TestUsecase_PagingAndRequest(t *testing.T) {
	f := &fakeScanner{pages: []scanentries.Response{
		{Entries: filler(1000, tSince), Next: cur("a")},
		{Entries: filler(1000, tSince), Next: cur("b")},
		{Entries: filler(5, tSince)},
	}}
	_, err := newUC(f).Report(context.Background(), okReq())
	if err != nil {
		t.Fatal(err)
	}
	if len(f.reqs) != 3 {
		t.Fatalf("panggilan = %d", len(f.reqs))
	}
	if f.reqs[0].After != nil || f.reqs[1].After.ID != "a" || f.reqs[2].After.ID != "b" {
		t.Fatalf("cursor salah: %+v", f.reqs)
	}
	for _, r := range f.reqs {
		if len(r.Actions) != 2 || r.Actions[0] != record.ActionMCPSearchKnowledge || r.Actions[1] != record.ActionMCPReadFile ||
			!r.From.Equal(tSince) || !r.To.Equal(tNow.Add(120*time.Second)) || r.Limit != 1000 || r.ProjectID != pid {
			t.Fatalf("request salah: %+v", r)
		}
	}
}

func TestUsecase_ReportMetrics(t *testing.T) {
	t0 := tNow.Add(-time.Hour)
	f := &fakeScanner{pages: []scanentries.Response{{Entries: []scanentries.Entry{
		search("1", t0, "k1", "login", "a.md", "b.md", "c.md"),
		read("2", t0.Add(30*time.Second), "k1", "b.md"),
		search("3", t0.Add(60*time.Second), "k1", "oauth login", "z.md"),
	}}}}
	res, err := newUC(f).Report(context.Background(), okReq())
	if err != nil {
		t.Fatal(err)
	}
	r := res.Report
	if r.ProjectID != pid || r.Searches != 2 || r.FollowUp.Count != 1 || r.FollowUp.Denominator != 2 ||
		r.Reformulation.Count != 1 || r.RankOpened[1].Count != 1 || r.WindowSeconds != 120 || len(r.RankOpened) != 11 {
		t.Fatalf("report salah: %+v", r)
	}
}

func TestUsecase_Empty(t *testing.T) {
	f := &fakeScanner{pages: []scanentries.Response{{Entries: []scanentries.Entry{}}}}
	res, err := newUC(f).Report(context.Background(), okReq())
	if err != nil || len(f.reqs) != 1 {
		t.Fatalf("err=%v calls=%d", err, len(f.reqs))
	}
	r := res.Report
	if r.Searches != 0 || len(r.RankOpened) != 11 || r.FollowUp.Rate != 0 || r.ZeroResult.Rate != 0 {
		t.Fatalf("%+v", r)
	}
}

func TestUsecase_ScanError(t *testing.T) {
	sentinel := errors.New("boom")
	_, err := newUC(&fakeScanner{err: sentinel}).Report(context.Background(), okReq())
	if !errors.Is(err, sentinel) || !strings.HasPrefix(err.Error(), "searchquality: scan audit:") {
		t.Fatalf("err=%v", err)
	}
	inv := errs.Wrap(errs.CodeInvalid, "x", errs.ErrInvalid)
	_, err = newUC(&fakeScanner{err: inv}).Report(context.Background(), okReq())
	var e *errs.Error
	if !errors.As(err, &e) || e.Code != errs.CodeInvalid {
		t.Fatalf("err=%v", err)
	}
}

func TestUsecase_MaxEntries(t *testing.T) {
	pages := func() []scanentries.Response {
		return []scanentries.Response{
			{Entries: filler(1000, tSince), Next: cur("a")},
			{Entries: filler(1000, tSince), Next: cur("b")},
			{Entries: filler(500, tSince), Next: cur("c")},
			{Entries: filler(1, tSince)},
		}
	}
	f := &fakeScanner{pages: pages()}
	u := newUC(f)
	u.maxEntries = 2500
	pg := pages()
	pg[2].Entries = filler(501, tSince)
	f.pages = pg
	_, err := u.Report(context.Background(), okReq())
	if !errors.Is(err, errs.ErrInvalid) || !strings.Contains(err.Error(), "rentang terlalu besar") {
		t.Fatalf("err=%v", err)
	}
	if len(f.reqs) != 3 {
		t.Fatalf("panggilan = %d, mau 3", len(f.reqs))
	}

	// tepat sama dengan batas: sukses
	f2 := &fakeScanner{pages: []scanentries.Response{
		{Entries: filler(1000, tSince), Next: cur("a")},
		{Entries: filler(1000, tSince), Next: cur("b")},
		{Entries: filler(500, tSince)},
	}}
	u2 := newUC(f2)
	u2.maxEntries = 2500
	if _, err := u2.Report(context.Background(), okReq()); err != nil {
		t.Fatal(err)
	}
}

func TestUsecase_CursorGuard(t *testing.T) {
	same := &fakeScanner{pages: []scanentries.Response{
		{Entries: filler(1, tSince), Next: cur("a")},
		{Entries: filler(1, tSince), Next: cur("a")},
	}}
	if _, err := newUC(same).Report(context.Background(), okReq()); err == nil || !strings.Contains(err.Error(), "kursor tidak maju") {
		t.Fatalf("err=%v", err)
	}
	empty := &fakeScanner{pages: []scanentries.Response{{Entries: []scanentries.Entry{}, Next: cur("a")}}}
	if _, err := newUC(empty).Report(context.Background(), okReq()); err == nil || !strings.Contains(err.Error(), "kursor tidak maju") {
		t.Fatalf("err=%v", err)
	}
}

func TestUsecase_CanceledAndInvalid(t *testing.T) {
	f := &fakeScanner{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := newUC(f).Report(ctx, okReq()); !errors.Is(err, context.Canceled) || len(f.reqs) != 0 {
		t.Fatalf("err=%v calls=%d", err, len(f.reqs))
	}
	_, err := newUC(f).Report(context.Background(), searchquality.Request{ProjectID: "x"})
	if !errors.Is(err, errs.ErrInvalid) || len(f.reqs) != 0 {
		t.Fatalf("err=%v calls=%d", err, len(f.reqs))
	}
}

func TestUsecase_IgnoresNonOKAndAnonymous(t *testing.T) {
	t0 := tNow.Add(-time.Hour)
	bad := search("1", t0, "k1", "q", "a.md")
	bad.Status = record.StatusError
	anon := search("2", t0, "", "q", "a.md")
	f := &fakeScanner{pages: []scanentries.Response{{Entries: []scanentries.Entry{bad, anon}}}}
	res, err := newUC(f).Report(context.Background(), okReq())
	if err != nil || res.Report.Searches != 0 {
		t.Fatalf("err=%v %+v", err, res.Report)
	}
}
