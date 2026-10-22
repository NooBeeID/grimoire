package ingest

import (
	"context"
	"errors"
	"testing"

	"grimoire/internal/domain/project/internal/knowledge/base"
)

type fakeTx struct {
	calls    []string
	count    int
	existing map[string][]byte
	inserted []base.File
	updated  []base.File
	failAt   string
}

func (f *fakeTx) step(n string) error {
	f.calls = append(f.calls, n)
	if f.failAt == n {
		return errors.New("boom")
	}
	return nil
}
func (f *fakeTx) LockProject(context.Context, string) error { return f.step("lock") }
func (f *fakeTx) CountFiles(context.Context, string) (int, error) {
	return f.count, f.step("count")
}
func (f *fakeTx) ExistingHashes(context.Context, string, []string) (map[string][]byte, error) {
	return f.existing, f.step("existing")
}
func (f *fakeTx) InsertFiles(_ context.Context, fs []base.File) error {
	f.inserted = fs
	return f.step("insert")
}
func (f *fakeTx) UpdateFiles(_ context.Context, fs []base.File) error {
	f.updated = fs
	return f.step("update")
}

func inc(i int, p, c string) base.Incoming {
	return base.Incoming{Index: i, Path: p, Content: c, Hash: base.Hash(c)}
}

func TestApply_Plans(t *testing.T) {
	tx := &fakeTx{count: 1, existing: map[string][]byte{"same.md": base.Hash("a"), "old.md": base.Hash("x")}}
	actor := base.Actor{ID: "u1", Role: "developer", Label: "Rina"}
	ds, n, err := Apply(context.Background(), tx, "p1", actor,
		[]base.Incoming{inc(0, "new.md", "# n"), inc(1, "same.md", "a"), inc(2, "old.md", "yy"), inc(3, "new.md", "dup"), inc(4, "over.md", "o")}, 2)
	if err != nil {
		t.Fatal(err)
	}
	got := map[int]base.Status{}
	for _, d := range ds {
		got[d.Index] = d.Status
	}
	want := map[int]base.Status{0: base.StatusCreated, 1: base.StatusUnchanged, 2: base.StatusUpdated, 3: base.StatusRejected, 4: base.StatusRejected}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("index %d: %s want %s", k, got[k], v)
		}
	}
	if n != len("# n")+len("yy") || len(tx.inserted) != 1 || len(tx.updated) != 1 || tx.inserted[0].CreatedBy != "u1" {
		t.Fatalf("n=%d ins=%v upd=%v", n, tx.inserted, tx.updated)
	}
	if tx.calls[0] != "lock" || tx.calls[1] != "count" || tx.calls[2] != "existing" {
		t.Fatalf("urutan: %v", tx.calls)
	}
}

func TestApply_EmptyDoesNothing(t *testing.T) {
	tx := &fakeTx{}
	ds, n, err := Apply(context.Background(), tx, "p", base.Actor{}, nil, 10)
	if err != nil || ds != nil || n != 0 || len(tx.calls) != 0 {
		t.Fatal(ds, n, err, tx.calls)
	}
}

func TestApply_ErrorsWrapped(t *testing.T) {
	for _, step := range []string{"lock", "count", "existing", "insert", "update"} {
		tx := &fakeTx{existing: map[string][]byte{"old.md": base.Hash("x")}, failAt: step}
		_, _, err := Apply(context.Background(), tx, "p", base.Actor{}, []base.Incoming{inc(0, "new.md", "a"), inc(1, "old.md", "b")}, 10)
		if err == nil {
			t.Errorf("%s: want error", step)
		}
	}
}
