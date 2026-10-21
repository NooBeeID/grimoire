package purgeinvitations

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"strings"
	"testing"
	"time"

	"grimoire/internal/infra/bus"
	"grimoire/lib/common/errs"
	"grimoire/lib/contracts/project/management/countmemberships"
)

var t0 = time.Date(2026, 10, 20, 3, 30, 0, 0, time.UTC)

type listCall struct {
	cutoff time.Time
	after  *Candidate
	limit  int
}

type fakeRepo struct {
	batches   [][]Candidate // skrip; habis -> kosong
	always    []Candidate   // bila non-nil: selalu dikembalikan
	listCalls []listCall
	delCalls  [][]string
	delCutoff []time.Time
	listErr   error
	delErr    error
}

func (f *fakeRepo) ListCandidates(_ context.Context, cutoff time.Time, after *Candidate, limit int) ([]Candidate, error) {
	f.listCalls = append(f.listCalls, listCall{cutoff, after, limit})
	if f.listErr != nil {
		return nil, f.listErr
	}
	if f.always != nil {
		return f.always, nil
	}
	i := len(f.listCalls) - 1
	if i >= len(f.batches) {
		return nil, nil
	}
	return f.batches[i], nil
}

func (f *fakeRepo) DeleteUsers(_ context.Context, ids []string, cutoff time.Time) (int64, error) {
	f.delCalls = append(f.delCalls, ids)
	f.delCutoff = append(f.delCutoff, cutoff)
	if f.delErr != nil {
		return 0, f.delErr
	}
	return int64(len(ids)), nil
}

type fakeCounter struct {
	counts map[string]int
	errs   []error // per panggilan (indeks); nil = sukses
	calls  [][]string
}

func (f *fakeCounter) Count(_ context.Context, ids []string) (map[string]int, error) {
	i := len(f.calls)
	f.calls = append(f.calls, ids)
	if i < len(f.errs) && f.errs[i] != nil {
		return nil, f.errs[i]
	}
	return f.counts, nil
}

func cand(ids ...string) []Candidate {
	out := make([]Candidate, len(ids))
	for i, id := range ids {
		out[i] = Candidate{UserID: id, ExpiresAt: t0.Add(-time.Duration(i+1) * time.Hour)}
	}
	return out
}

func manyCands(prefix string, n int) []Candidate {
	out := make([]Candidate, n)
	for i := range out {
		out[i] = Candidate{UserID: fmt.Sprintf("%s-%04d", prefix, i), ExpiresAt: t0.Add(-time.Duration(n-i) * time.Minute)}
	}
	return out
}

func newUC(r *fakeRepo, c *fakeCounter, clock func() time.Time) (*Usecase, *bytes.Buffer) {
	buf := &bytes.Buffer{}
	return New(r, c, clock, 720*time.Hour, slog.New(slog.NewTextHandler(buf, nil))), buf
}

func fixed() time.Time { return t0 }

func TestRun_RetainsMembers(t *testing.T) {
	r := &fakeRepo{batches: [][]Candidate{cand("a", "b", "c")}}
	c := &fakeCounter{counts: map[string]int{"a": 2, "c": 0}}
	uc, buf := newUC(r, c, fixed)
	if err := uc.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(r.delCalls) != 1 || !reflect.DeepEqual(r.delCalls[0], []string{"b", "c"}) {
		t.Fatalf("delete = %v", r.delCalls)
	}
	if !strings.Contains(buf.String(), "retained=1") || !strings.Contains(buf.String(), "deleted=2") {
		t.Fatalf("log: %s", buf)
	}
}

func TestRun_AllHaveMembers(t *testing.T) {
	r := &fakeRepo{batches: [][]Candidate{cand("a", "b")}}
	c := &fakeCounter{counts: map[string]int{"a": 1, "b": 3}}
	uc, _ := newUC(r, c, fixed)
	if err := uc.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(r.delCalls) != 0 {
		t.Fatalf("DeleteUsers dipanggil: %v", r.delCalls)
	}
}

func TestRun_CounterFailsClosed(t *testing.T) {
	forbidden := errs.New(errs.CodeInvalid, "bad")
	cases := []struct {
		name string
		err  error
		is   error
	}{
		{"generik", errors.New("boom"), nil},
		{"not registered", fmt.Errorf("x: %w: ep", bus.ErrNotRegistered), bus.ErrNotRegistered},
		{"errs.Error", forbidden, forbidden},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := &fakeRepo{batches: [][]Candidate{cand("a")}}
			c := &fakeCounter{errs: []error{tc.err}}
			uc, buf := newUC(r, c, fixed)
			err := uc.Run(context.Background())
			if err == nil {
				t.Fatal("harus error")
			}
			if !errors.Is(err, tc.err) {
				t.Fatalf("err tidak membungkus penyebab: %v", err)
			}
			if tc.is != nil && !errors.Is(err, tc.is) {
				t.Fatalf("errors.Is gagal: %v", err)
			}
			if len(r.delCalls) != 0 {
				t.Fatal("tidak boleh ada DELETE")
			}
			if !strings.Contains(buf.String(), "count-memberships gagal") {
				t.Fatalf("log: %s", buf)
			}
		})
	}
}

func TestRun_CutoffAndClockOnce(t *testing.T) {
	calls := 0
	clock := func() time.Time { calls++; return t0 }
	r := &fakeRepo{batches: [][]Candidate{manyCands("a", batchSize), cand("z")}}
	c := &fakeCounter{counts: map[string]int{}}
	uc, _ := newUC(r, c, clock)
	if err := uc.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("clock %d kali", calls)
	}
	want := t0.Add(-720 * time.Hour)
	for _, lc := range r.listCalls {
		if !lc.cutoff.Equal(want) {
			t.Fatalf("cutoff = %v", lc.cutoff)
		}
	}
	for _, dc := range r.delCutoff {
		if !dc.Equal(want) {
			t.Fatalf("delete cutoff = %v", dc)
		}
	}
}

func TestRun_NoCandidates(t *testing.T) {
	r := &fakeRepo{}
	c := &fakeCounter{}
	uc, buf := newUC(r, c, fixed)
	if err := uc.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(c.calls) != 0 {
		t.Fatal("counter dipanggil")
	}
	if !strings.Contains(buf.String(), "candidates=0") || !strings.Contains(buf.String(), "deleted=0") {
		t.Fatalf("log: %s", buf)
	}
}

func TestRun_Pagination(t *testing.T) {
	b1, b2, b3 := manyCands("a", batchSize), manyCands("b", batchSize), manyCands("c", 3)
	r := &fakeRepo{batches: [][]Candidate{b1, b2, b3}}
	c := &fakeCounter{counts: map[string]int{}}
	uc, _ := newUC(r, c, fixed)
	if err := uc.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(r.listCalls) != 3 {
		t.Fatalf("list calls = %d", len(r.listCalls))
	}
	if r.listCalls[0].after != nil {
		t.Fatal("kursor awal harus nil")
	}
	if *r.listCalls[1].after != b1[len(b1)-1] || *r.listCalls[2].after != b2[len(b2)-1] {
		t.Fatal("kursor salah")
	}
	if len(c.calls) != 3 {
		t.Fatalf("counter calls = %d", len(c.calls))
	}
	for _, ids := range c.calls {
		if len(ids) > batchSize {
			t.Fatalf("batch %d > %d", len(ids), batchSize)
		}
	}
	for _, lc := range r.listCalls {
		if lc.limit != batchSize {
			t.Fatalf("limit = %d", lc.limit)
		}
	}
}

func TestRun_SecondBatchCounterFails(t *testing.T) {
	r := &fakeRepo{batches: [][]Candidate{manyCands("a", batchSize), manyCands("b", 5)}}
	c := &fakeCounter{counts: map[string]int{}, errs: []error{nil, errors.New("boom")}}
	uc, buf := newUC(r, c, fixed)
	if err := uc.Run(context.Background()); err == nil {
		t.Fatal("harus error")
	}
	if len(r.delCalls) != 1 {
		t.Fatalf("delete calls = %d", len(r.delCalls))
	}
	if !strings.Contains(buf.String(), fmt.Sprintf("deleted=%d", batchSize)) {
		t.Fatalf("log: %s", buf)
	}
}

func TestRun_MaxBatches(t *testing.T) {
	r := &fakeRepo{always: manyCands("a", batchSize)}
	c := &fakeCounter{counts: map[string]int{}}
	uc, _ := newUC(r, c, fixed)
	if err := uc.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(r.listCalls) != maxBatches {
		t.Fatalf("batches = %d", len(r.listCalls))
	}
}

func TestRun_ListError(t *testing.T) {
	boom := errors.New("db")
	r := &fakeRepo{listErr: boom}
	c := &fakeCounter{}
	uc, _ := newUC(r, c, fixed)
	err := uc.Run(context.Background())
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
	if len(c.calls) != 0 {
		t.Fatal("counter dipanggil")
	}
}

func TestRun_DeleteError(t *testing.T) {
	boom := errors.New("db")
	r := &fakeRepo{batches: [][]Candidate{cand("a")}, delErr: boom}
	c := &fakeCounter{counts: map[string]int{}}
	uc, _ := newUC(r, c, fixed)
	if err := uc.Run(context.Background()); !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
}

func TestRun_CancelledContext(t *testing.T) {
	r := &fakeRepo{batches: [][]Candidate{cand("a")}}
	c := &fakeCounter{}
	uc, _ := newUC(r, c, fixed)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := uc.Run(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
	if len(r.listCalls) != 0 {
		t.Fatal("repo dipanggil")
	}
}

func TestNew_PanicsOnNonPositive(t *testing.T) {
	for _, d := range []time.Duration{0, -time.Hour} {
		func() {
			defer func() {
				if recover() == nil {
					t.Fatalf("harus panic untuk %v", d)
				}
			}()
			New(&fakeRepo{}, &fakeCounter{}, fixed, d, slog.Default())
		}()
	}
}

func TestBatchSizeWithinContract(t *testing.T) {
	if batchSize > countmemberships.MaxUserIDs {
		t.Fatalf("batchSize %d > MaxUserIDs %d", batchSize, countmemberships.MaxUserIDs)
	}
}

func TestRun_LogHasNoPII(t *testing.T) {
	r := &fakeRepo{batches: [][]Candidate{cand("user-id-secret-1", "user-id-secret-2")}}
	c := &fakeCounter{counts: map[string]int{"user-id-secret-1": 1}}
	uc, buf := newUC(r, c, fixed)
	_ = uc.Run(context.Background())
	c2 := &fakeCounter{errs: []error{errors.New("boom")}}
	r2 := &fakeRepo{batches: [][]Candidate{cand("user-id-secret-1")}}
	uc2, buf2 := newUC(r2, c2, fixed)
	_ = uc2.Run(context.Background())
	for _, s := range []string{buf.String(), buf2.String()} {
		if strings.Contains(s, "@") || strings.Contains(s, "user-id-secret") {
			t.Fatalf("log memuat PII: %s", s)
		}
	}
}

func TestRun_Idempotent(t *testing.T) {
	r := &fakeRepo{batches: [][]Candidate{cand("a")}}
	c := &fakeCounter{counts: map[string]int{}}
	uc, _ := newUC(r, c, fixed)
	if err := uc.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := uc.Run(context.Background()); err != nil { // batches habis -> kosong
		t.Fatal(err)
	}
	if len(r.delCalls) != 1 {
		t.Fatalf("delete calls = %d", len(r.delCalls))
	}
}
