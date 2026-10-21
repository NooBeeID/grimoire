//go:build integration

package updateproject

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"grimoire/internal/domain/project/internal/management/base"
	"grimoire/internal/domain/project/internal/management/base/store"
	"grimoire/internal/infra/db"
)

func migratedDB(t *testing.T) *db.DB {
	t.Helper()
	d := db.TestDB(t)
	if _, err := db.Up(context.Background(), d, nil); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return d
}

func seed(t *testing.T, d *db.DB) string {
	t.Helper()
	var id string
	err := d.GetContext(context.Background(), &id,
		`INSERT INTO projects (name, description, created_at, updated_at) VALUES ('Lama','d',now(),now()) RETURNING id::text`)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestRepo_UpdateProjectCommitAndRollback(t *testing.T) {
	d := migratedDB(t)
	r := newRepo(d, store.New(d))
	ctx := context.Background()
	id := seed(t, d)
	at := time.Now().UTC().Add(time.Minute)

	boom := errors.New("boom")
	err := r.InTx(ctx, func(tx TxRepository) error {
		if _, err := tx.UpdateProject(ctx, base.Project{ID: id, Name: "Rollback", UpdatedAt: at}); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatal(err)
	}
	p, _, _ := store.New(d).FindProject(ctx, id)
	if p.Name != "Lama" {
		t.Fatalf("rollback gagal: %s", p.Name)
	}

	var got time.Time
	err = r.InTx(ctx, func(tx TxRepository) error {
		var err error
		got, err = tx.UpdateProject(ctx, base.Project{ID: id, Name: "Baru", Description: "", UpdatedAt: at})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	p, _, _ = store.New(d).FindProject(ctx, id)
	if p.Name != "Baru" || p.Description != "" || !p.UpdatedAt.Equal(got) {
		t.Fatalf("p: %+v got %v", p, got)
	}
}

func TestRepo_UpdateProjectNotFound(t *testing.T) {
	d := migratedDB(t)
	r := newRepo(d, store.New(d))
	err := r.InTx(context.Background(), func(tx TxRepository) error {
		_, err := tx.UpdateProject(context.Background(), base.Project{ID: uuid.NewString(), Name: "Baru", UpdatedAt: time.Now()})
		return err
	})
	if !errors.Is(err, base.ErrProjectNotFound) {
		t.Fatalf("err: %v", err)
	}
}
