package purgeexpired

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"grimoire/internal/domain/core/internal/auditlog/base"
)

type call struct {
	cutoff time.Time
	limit  int
}

type fakeRepo struct {
	script []int64
	err    map[int]error // indeks panggilan -> error
	calls  []call
	hook   func(i int)
}

func (f *fakeRepo) DeleteBatch(_ context.Context, cutoff time.Time, limit int) (int64, error) {
	i := len(f.calls)
	f.calls = append(f.calls, call{cutoff, limit})
	if f.hook != nil {
		f.hook(i)
	}
	if e := f.err[i]; e != nil {
		return 0, e
	}
	if i < len(f.script) {
		return f.script[i], nil
	}
	return f.script[len(f.script)-1], nil
}

var t0 = time.Date(2026, 10, 20, 3, 15, 0, 0, time.UTC)

func newUC(repo repository, opt Options, buf *bytes.Buffer) *Usecase {
	log := slog.New(slog.NewTextHandler(buf, nil))
	return New(repo, opt, func() time.Time { return t0 }, log)
}

func TestRun_StopsOnPartialBatch(t *testing.T) {
	var buf bytes.Buffer
	r := &fakeRepo{script: []int64{3, 3, 2}}
	if err := newUC(r, Options{Retention: 90, BatchSize: 3}, &buf).Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(r.calls) != 3 {
		t.Fatalf("calls=%d", len(r.calls))
	}
	if !strings.Contains(buf.String(), "total_deleted=8") || !strings.Contains(buf.String(), "batches=3") {
		t.Fatal(buf.String())
	}
}

func TestRun_StopsOnEmptyFirstBatch(t *testing.T) {
	var buf bytes.Buffer
	r := &fakeRepo{script: []int64{0}}
	if err := newUC(r, Options{Retention: 90, BatchSize: 3}, &buf).Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(r.calls) != 1 || !strings.Contains(buf.String(), "total_deleted=0") {
		t.Fatal(len(r.calls), buf.String())
	}
}

func TestRun_FullBatchThenZero(t *testing.T) {
	r := &fakeRepo{script: []int64{3, 0}}
	if err := newUC(r, Options{Retention: 90, BatchSize: 3}, &bytes.Buffer{}).Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(r.calls) != 2 {
		t.Fatalf("calls=%d", len(r.calls))
	}
}

func TestRun_CutoffComputedOnce(t *testing.T) {
	r := &fakeRepo{script: []int64{2, 2, 2, 0}}
	n := 0
	clock := func() time.Time { n++; return t0.Add(time.Duration(n) * time.Hour) }
	uc := New(r, Options{Retention: 90, BatchSize: 2}, clock, slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)))
	if err := uc.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("clock calls=%d", n)
	}
	want := base.PurgeCutoff(t0.Add(time.Hour), 90)
	if len(r.calls) != 4 {
		t.Fatalf("calls=%d", len(r.calls))
	}
	for _, c := range r.calls {
		if !c.cutoff.Equal(want) {
			t.Fatalf("cutoff %v want %v", c.cutoff, want)
		}
	}
}

func TestRun_CutoffValue(t *testing.T) {
	for days, want := range map[int]time.Time{
		90: time.Date(2026, 7, 22, 3, 15, 0, 0, time.UTC),
		30: time.Date(2026, 9, 20, 3, 15, 0, 0, time.UTC),
	} {
		r := &fakeRepo{script: []int64{0}}
		if err := newUC(r, Options{Retention: days}, &bytes.Buffer{}).Run(context.Background()); err != nil {
			t.Fatal(err)
		}
		if !r.calls[0].cutoff.Equal(want) {
			t.Fatalf("days=%d cutoff=%v want %v", days, r.calls[0].cutoff, want)
		}
	}
}

func TestRun_PassesBatchSize(t *testing.T) {
	r := &fakeRepo{script: []int64{0}}
	_ = newUC(r, Options{Retention: 1, BatchSize: 7}, &bytes.Buffer{}).Run(context.Background())
	if r.calls[0].limit != 7 {
		t.Fatal(r.calls[0].limit)
	}
	r = &fakeRepo{script: []int64{0}}
	_ = newUC(r, Options{Retention: 1}, &bytes.Buffer{}).Run(context.Background())
	if r.calls[0].limit != 5000 {
		t.Fatal(r.calls[0].limit)
	}
}

func TestRun_RepoErrorReturned(t *testing.T) {
	var buf bytes.Buffer
	sentinel := errors.New("boom")
	r := &fakeRepo{script: []int64{3}, err: map[int]error{1: sentinel}}
	err := newUC(r, Options{Retention: 90, BatchSize: 3}, &buf).Run(context.Background())
	if !errors.Is(err, sentinel) {
		t.Fatal(err)
	}
	if len(r.calls) != 2 || !strings.Contains(buf.String(), "total_deleted=3") || !strings.Contains(buf.String(), "level=ERROR") {
		t.Fatal(len(r.calls), buf.String())
	}
}

func TestRun_ContextCanceledBeforeStart(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r := &fakeRepo{script: []int64{3}}
	err := newUC(r, Options{Retention: 90, BatchSize: 3}, &bytes.Buffer{}).Run(ctx)
	if !errors.Is(err, context.Canceled) || len(r.calls) != 0 {
		t.Fatal(err, len(r.calls))
	}
}

func TestRun_ContextCanceledDuringPause(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r := &fakeRepo{script: []int64{3}, hook: func(int) { cancel() }}
	done := make(chan error, 1)
	go func() {
		done <- newUC(r, Options{Retention: 90, BatchSize: 3, Pause: time.Hour}, &bytes.Buffer{}).Run(ctx)
	}()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) || len(r.calls) != 1 {
			t.Fatal(err, len(r.calls))
		}
	case <-time.After(time.Second):
		t.Fatal("Run did not return on cancel")
	}
}

func TestRun_ContextCanceledBetweenBatches_NoPause(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r := &fakeRepo{script: []int64{3}, hook: func(i int) {
		if i == 1 {
			cancel()
		}
	}}
	err := newUC(r, Options{Retention: 90, BatchSize: 3}, &bytes.Buffer{}).Run(ctx)
	if !errors.Is(err, context.Canceled) || len(r.calls) != 2 {
		t.Fatal(err, len(r.calls))
	}
}

func TestRun_LogsSummaryOnSuccess(t *testing.T) {
	var buf bytes.Buffer
	r := &fakeRepo{script: []int64{1}}
	if err := newUC(r, Options{Retention: 90, BatchSize: 3}, &buf).Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	s := buf.String()
	for _, k := range []string{`msg="audit purge done"`, "total_deleted=", "batches=", "duration_ms="} {
		if !strings.Contains(s, k) {
			t.Fatalf("missing %s in %s", k, s)
		}
	}
	if strings.Contains(s, "level=ERROR") {
		t.Fatal(s)
	}
}

func TestNew_RejectsNonPositiveRetention(t *testing.T) {
	for _, d := range []int{0, -1} {
		func() {
			defer func() {
				if recover() == nil {
					t.Fatalf("no panic for %d", d)
				}
			}()
			New(&fakeRepo{}, Options{Retention: d}, nil, nil)
		}()
	}
}

type fakeRunner struct {
	ctx context.Context
	err error
}

func (f *fakeRunner) Run(ctx context.Context) error { f.ctx = ctx; return f.err }

type ctxKey struct{}

func TestHandler_DelegatesToRun(t *testing.T) {
	sentinel := errors.New("x")
	f := &fakeRunner{err: sentinel}
	ctx := context.WithValue(context.Background(), ctxKey{}, 1)
	if err := Handler(f)(ctx); !errors.Is(err, sentinel) || f.ctx != ctx {
		t.Fatal(err)
	}
}

func TestNewJob(t *testing.T) {
	j := newJob(func(context.Context) error { return nil })
	if j.Name != "core.auditlog.purge-expired" || j.DailyAt != "03:15" || j.Every != 0 || j.Run == nil {
		t.Fatalf("%+v", j)
	}
}
