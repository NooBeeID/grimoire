//go:build integration

package createfile

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"grimoire/internal/domain/project/internal/knowledge/base"
	"grimoire/internal/domain/project/internal/knowledge/base/store"
	"grimoire/internal/infra/db"
)

func TestRepoInsert(t *testing.T) {
	ctx := context.Background()
	d := db.TestDB(t)
	if _, err := db.Up(ctx, d, nil); err != nil {
		t.Fatal(err)
	}
	mk := func(name string) string {
		var id string
		if err := d.GetContext(ctx, &id, `INSERT INTO projects (name) VALUES ($1 || ' project') RETURNING id::text`, name); err != nil {
			t.Fatal(err)
		}
		return id
	}
	a, b := mk("A"), mk("B")
	repo := NewRepository(store.New(d))
	actor := base.Actor{ID: uuid.NewString(), Label: "Budi"}

	saved, err := repo.Insert(ctx, base.NewFile(a, "docs/auth/login.md", "# Login é😀", actor))
	if err != nil || saved.SizeBytes != len("# Login é😀") || len(saved.Hash) != 32 ||
		saved.CreatedBy != saved.UpdatedBy || saved.CreatedAt.Location().String() != "UTC" {
		t.Fatalf("%+v %v", saved, err)
	}
	if _, err := repo.Insert(ctx, base.NewFile(a, "docs/auth/login.md", "x", actor)); !errors.Is(err, base.ErrPathExists) {
		t.Fatalf("want path exists, got %v", err)
	}
	if _, err := repo.Insert(ctx, base.NewFile(b, "docs/auth/login.md", "x", actor)); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"A.md", "a.md"} {
		if _, err := repo.Insert(ctx, base.NewFile(a, p, "x", actor)); err != nil {
			t.Fatal(err)
		}
	}
}
