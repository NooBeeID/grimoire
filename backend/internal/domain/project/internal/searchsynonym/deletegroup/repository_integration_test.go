//go:build integration

package deletegroup

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"grimoire/internal/domain/project/internal/searchsynonym/base"
	"grimoire/internal/domain/project/internal/searchsynonym/base/audit"
	"grimoire/internal/domain/project/internal/searchsynonym/base/store"
	"grimoire/internal/infra/db"
)

type okAccess struct{ actor base.Actor }

func (o okAccess) Require(context.Context, string) (base.Actor, error) { return o.actor, nil }

type nopAudit struct{}

func (nopAudit) Record(context.Context, audit.Event) {}

func TestDeleteGroup_Integration(t *testing.T) {
	ctx := context.Background()
	d := db.TestDB(t)
	if _, err := db.Up(ctx, d, nil); err != nil {
		t.Fatal(err)
	}
	mk := func(name string) string {
		var id string
		if err := d.GetContext(ctx, &id, `INSERT INTO projects (name) VALUES ($1) RETURNING id::text`, name); err != nil {
			t.Fatal(err)
		}
		return id
	}
	pa, pb := mk("Proyek A"), mk("Proyek B")
	s := store.New(d)
	actor := base.Actor{ID: uuid.NewString(), Label: "Budi", Role: "developer"}
	g, err := s.Insert(ctx, base.NewGroup(pa, []string{"login", "masuk"}, actor))
	if err != nil {
		t.Fatal(err)
	}
	uc := NewUsecase(okAccess{actor}, NewRepository(s), nopAudit{})

	// Dari project lain -> 404, grup tetap ada.
	if err := uc.Execute(ctx, pb, g.ID, base.RequestMeta{}); !errors.Is(err, base.ErrGroupNotFound) {
		t.Fatalf("want not found, got %v", err)
	}
	if _, ok, _ := s.Get(ctx, pa, g.ID); !ok {
		t.Fatal("grup project A ikut terhapus")
	}
	if err := uc.Execute(ctx, pa, g.ID, base.RequestMeta{}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{g.ID, "bukan-uuid"} {
		if err := uc.Execute(ctx, pa, id, base.RequestMeta{}); !errors.Is(err, base.ErrGroupNotFound) {
			t.Fatalf("%s: want not found, got %v", id, err)
		}
	}
}
