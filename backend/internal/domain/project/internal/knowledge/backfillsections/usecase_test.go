package backfillsections

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"grimoire/internal/domain/project/internal/knowledge/base"
)

type fakeRepo struct {
	pending  []Pending
	set      map[string][]byte
	batches  int
	selErr   error
	setErr   error
	onSelect func()
}

func (f *fakeRepo) NullSections(_ context.Context, limit int) ([]Pending, error) {
	f.batches++
	if f.onSelect != nil {
		f.onSelect()
	}
	if f.selErr != nil {
		return nil, f.selErr
	}
	n := min(limit, len(f.pending))
	out := f.pending[:n]
	f.pending = f.pending[n:]
	return out, nil
}

func (f *fakeRepo) SetSections(_ context.Context, id string, s []byte) error {
	if f.setErr != nil {
		return f.setErr
	}
	f.set[id] = s
	return nil
}

func TestRun_FillsAndStopsOnEmptyBatch(t *testing.T) {
	r := &fakeRepo{set: map[string][]byte{}, pending: []Pending{{"a", "# A\nx"}, {"b", ""}}}
	if err := NewUsecase(r).Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if r.batches != 2 || len(r.set) != 2 {
		t.Fatalf("batches=%d set=%d", r.batches, len(r.set))
	}
	var secs []base.Section
	if err := json.Unmarshal(r.set["a"], &secs); err != nil || len(secs) != 1 || secs[0].Heading != "A" {
		t.Fatalf("%s %v", r.set["a"], err)
	}
	if string(r.set["b"]) != "[]" {
		t.Fatalf("kosong -> %s", r.set["b"])
	}
}

func TestRun_MaxBatches(t *testing.T) {
	r := &fakeRepo{set: map[string][]byte{}}
	for i := range maxBatches*batchSize + 50 {
		r.pending = append(r.pending, Pending{ID: time.Duration(i).String(), Content: "x"})
	}
	if err := NewUsecase(r).Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if r.batches != maxBatches || len(r.set) != maxBatches*batchSize {
		t.Fatalf("batches=%d set=%d", r.batches, len(r.set))
	}
}

func TestRun_Errors(t *testing.T) {
	boom := errors.New("boom")
	if err := NewUsecase(&fakeRepo{selErr: boom}).Run(context.Background()); !errors.Is(err, boom) {
		t.Fatalf("select err: %v", err)
	}
	r := &fakeRepo{setErr: boom, pending: []Pending{{"a", "x"}}}
	if err := NewUsecase(r).Run(context.Background()); !errors.Is(err, boom) {
		t.Fatalf("set err: %v", err)
	}
}

func TestRun_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r := &fakeRepo{set: map[string][]byte{}, pending: []Pending{{"a", "x"}}}
	if err := NewUsecase(r).Run(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("want canceled, got %v", err)
	}
	if r.batches != 0 {
		t.Fatalf("tidak boleh query, batches=%d", r.batches)
	}
}

func TestNewJob(t *testing.T) {
	j := newJob(func(context.Context) error { return nil })
	if j.Name != "knowledge.backfill-sections" || j.Every != time.Minute || j.DailyAt != "" {
		t.Fatalf("%+v", j)
	}
}
