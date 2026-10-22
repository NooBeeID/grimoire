package summary

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"grimoire/internal/domain/core/internal/auditlog/base"
	"grimoire/lib/contracts/discovery/knowledgetools/searchquality"
)

var fixedNow = time.Date(2026, 10, 22, 10, 30, 15, 500, time.UTC)

type fakeRepo struct {
	events *[]string
	w      base.Window
	pid    string
	err    error
}

func (f *fakeRepo) Load(_ context.Context, w base.Window, pid string) (base.Aggregates, error) {
	*f.events = append(*f.events, "load")
	f.w, f.pid = w, pid
	if f.err != nil {
		return base.Aggregates{}, f.err
	}
	return base.Aggregates{Traffic: trafficFor(w)}, nil
}

type fakeQuality struct {
	events *[]string
	calls  int
	req    searchquality.Request
	hasDL  bool
	dl     time.Duration
	rep    searchquality.Report
	err    error
}

func (f *fakeQuality) Report(ctx context.Context, r searchquality.Request) (searchquality.Report, error) {
	*f.events = append(*f.events, "quality")
	f.calls++
	f.req = r
	if d, ok := ctx.Deadline(); ok {
		f.hasDL, f.dl = true, time.Until(d)
	}
	return f.rep, f.err
}

func newUC(repoErr, qErr error) (*Usecase, *fakeRepo, *fakeQuality, *bytes.Buffer, *[]string) {
	ev := &[]string{}
	buf := &bytes.Buffer{}
	r := &fakeRepo{events: ev, err: repoErr}
	q := &fakeQuality{events: ev, err: qErr, rep: searchquality.Report{Searches: 3, WindowSeconds: 120}}
	uc := NewUsecase(r, q, func() time.Time { return fixedNow }, slog.New(slog.NewTextHandler(buf, nil)))
	return uc, r, q, buf, ev
}

func TestSummarize_NoProject(t *testing.T) {
	uc, r, q, _, _ := newUC(nil, nil)
	rep, err := uc.Summarize(context.Background(), base.SummaryQuery{Range: base.Range7d})
	if err != nil {
		t.Fatal(err)
	}
	if q.calls != 0 || rep.SearchQuality != nil || rep.ProjectID != nil {
		t.Fatalf("calls=%d sq=%v", q.calls, rep.SearchQuality)
	}
	if want := fixedNow.UTC().Truncate(time.Second).Add(-7 * 24 * time.Hour); !r.w.From.Equal(want) {
		t.Fatalf("from %v want %v", r.w.From, want)
	}
	if r.pid != "" {
		t.Fatalf("pid %q", r.pid)
	}
}

func TestSummarize_WithProject(t *testing.T) {
	uc, r, q, _, ev := newUC(nil, nil)
	pid := "0192f5a0-1111-7aaa-8bbb-000000000001"
	rep, err := uc.Summarize(context.Background(), base.SummaryQuery{Range: base.Range24h, ProjectID: pid})
	if err != nil {
		t.Fatal(err)
	}
	if q.calls != 1 || q.req.ProjectID != pid || !q.req.Since.Equal(r.w.From) || !q.req.Until.Equal(r.w.To) || q.req.Window != 0 {
		t.Fatalf("req %+v calls %d", q.req, q.calls)
	}
	if !q.hasDL || q.dl > qualityTimeout {
		t.Fatalf("deadline %v %v", q.hasDL, q.dl)
	}
	if rep.SearchQuality == nil || !rep.SearchQuality.Available {
		t.Fatalf("sq %+v", rep.SearchQuality)
	}
	if strings.Join(*ev, ",") != "load,quality" {
		t.Fatalf("urutan %v", *ev)
	}
}

func TestSummarize_QualityFailSoft(t *testing.T) {
	uc, _, _, buf, _ := newUC(nil, errors.New("boom"))
	rep, err := uc.Summarize(context.Background(), base.SummaryQuery{Range: base.Range7d, ProjectID: "0192f5a0-1111-7aaa-8bbb-000000000001"})
	if err != nil {
		t.Fatal(err)
	}
	if rep.SearchQuality == nil || rep.SearchQuality.Available || rep.SearchQuality.ErrorCode != "UNAVAILABLE" {
		t.Fatalf("sq %+v", rep.SearchQuality)
	}
	if !strings.Contains(buf.String(), "level=WARN") || strings.Contains(buf.String(), "args") {
		t.Fatalf("log %s", buf.String())
	}
}

func TestSummarize_RepoError(t *testing.T) {
	uc, _, q, _, _ := newUC(errors.New("db down"), nil)
	_, err := uc.Summarize(context.Background(), base.SummaryQuery{Range: base.Range7d, ProjectID: "0192f5a0-1111-7aaa-8bbb-000000000001"})
	if err == nil || !strings.HasPrefix(err.Error(), "summary:") {
		t.Fatalf("err %v", err)
	}
	if q.calls != 0 {
		t.Fatal("reporter dipanggil")
	}
}
