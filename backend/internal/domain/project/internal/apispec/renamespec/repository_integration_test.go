//go:build integration

package renamespec

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"grimoire/internal/domain/project/internal/apispec/base"
	"grimoire/internal/domain/project/internal/apispec/base/store"
	"grimoire/internal/infra/db"
)

const actorID = "0b6c0000-0000-4000-8000-000000000001"

func setupDB(t *testing.T) (*db.DB, *Repo) {
	t.Helper()
	d := db.TestDB(t)
	if _, err := db.Up(context.Background(), d, nil); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return d, newRepo(d, store.New(d))
}

func newProject(t *testing.T, d *db.DB, name string) string {
	t.Helper()
	var id string
	if err := d.GetContext(context.Background(), &id, `INSERT INTO projects (name) VALUES ($1) RETURNING id::text`, "Proyek "+name); err != nil {
		t.Fatal(err)
	}
	return id
}

func insertSpec(t *testing.T, d *db.DB, pid, name string) {
	t.Helper()
	sp := base.NewSpec(pid, name, nil, base.DocMeta{Title: "T", APIVersion: "1", OpenAPIVersion: "3.0.3"},
		"openapi: 3.0.3 # "+name, base.FormatYAML, base.Actor{ID: actorID, Label: "Budi"}, time.Now())
	_, err := d.ExecContext(context.Background(), `INSERT INTO api_specs (project_id, name, title, api_version, openapi_version,
content, content_format, content_hash, size_bytes, created_at, created_by, created_by_label, updated_at, updated_by, updated_by_label)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$10,$11,$12)`,
		pid, name, sp.Title, sp.APIVersion, sp.OpenAPIVersion, sp.Content, sp.Format, sp.Hash[:], sp.SizeBytes,
		sp.CreatedAt, actorID, "Budi")
	if err != nil {
		t.Fatal(err)
	}
}

func renameVia(ctx context.Context, r *Repo, pid, from, to string) error {
	return r.InTx(ctx, func(tx Tx) error {
		sp, ok, err := tx.LockSpec(ctx, pid, from)
		if err != nil || !ok {
			return errors.Join(err, base.ErrSpecNotFound)
		}
		sp.Rename(to, base.Actor{ID: actorID, Label: "Sari"}, time.Now())
		return tx.RenameSpec(ctx, sp)
	})
}

func TestRepo_RenameKeepsContent(t *testing.T) {
	d, r := setupDB(t)
	ctx := context.Background()
	pid := newProject(t, d, "A")
	insertSpec(t, d, pid, "payment")
	before, _, _ := r.s.FindSpec(ctx, pid, "payment")
	if err := renameVia(ctx, r, pid, "payment", "payment-v2"); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := r.s.FindSpec(ctx, pid, "payment"); ok {
		t.Fatal("nama lama masih ada")
	}
	after, ok, _ := r.s.FindSpec(ctx, pid, "payment-v2")
	if !ok || after.ID != before.ID || after.Content != before.Content || after.Hash != before.Hash ||
		!after.CreatedAt.Equal(before.CreatedAt) || after.UpdatedByLabel != "Sari" {
		t.Fatalf("after=%+v before=%+v", after, before)
	}
}

func TestRepo_NameTakenRollsBack(t *testing.T) {
	d, r := setupDB(t)
	ctx := context.Background()
	pid := newProject(t, d, "A")
	insertSpec(t, d, pid, "a")
	insertSpec(t, d, pid, "b")
	if err := renameVia(ctx, r, pid, "a", "b"); !errors.Is(err, store.ErrNameTaken) {
		t.Fatalf("err=%v", err)
	}
	if _, ok, _ := r.s.FindSpec(ctx, pid, "a"); !ok {
		t.Fatal("nama asli hilang")
	}
}

func TestRepo_SameNameOtherProjectOK(t *testing.T) {
	d, r := setupDB(t)
	p1, p2 := newProject(t, d, "A"), newProject(t, d, "B")
	insertSpec(t, d, p1, "a")
	insertSpec(t, d, p2, "b")
	if err := renameVia(context.Background(), r, p1, "a", "b"); err != nil {
		t.Fatal(err)
	}
}

func TestRepo_InTxErrorRollsBack(t *testing.T) {
	d, r := setupDB(t)
	ctx := context.Background()
	pid := newProject(t, d, "A")
	insertSpec(t, d, pid, "a")
	boom := errors.New("boom")
	err := r.InTx(ctx, func(tx Tx) error {
		sp, _, _ := tx.LockSpec(ctx, pid, "a")
		sp.Rename("z", base.Actor{ID: actorID, Label: "S"}, time.Now())
		if err := tx.RenameSpec(ctx, sp); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatal(err)
	}
	if _, ok, _ := r.s.FindSpec(ctx, pid, "a"); !ok {
		t.Fatal("rollback gagal")
	}
}

func TestRepo_ConcurrentSameTarget(t *testing.T) {
	d, r := setupDB(t)
	ctx := context.Background()
	pid := newProject(t, d, "A")
	insertSpec(t, d, pid, "a")
	insertSpec(t, d, pid, "b")
	var wg sync.WaitGroup
	errsOut := make([]error, 2)
	for i, from := range []string{"a", "b"} {
		wg.Add(1)
		go func() { defer wg.Done(); errsOut[i] = renameVia(ctx, r, pid, from, "c") }()
	}
	wg.Wait()
	ok, taken := 0, 0
	for _, e := range errsOut {
		switch {
		case e == nil:
			ok++
		case errors.Is(e, store.ErrNameTaken):
			taken++
		}
	}
	if ok != 1 || taken != 1 {
		t.Fatalf("errs=%v", errsOut)
	}
}
