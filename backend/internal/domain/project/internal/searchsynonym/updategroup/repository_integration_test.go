//go:build integration

package updategroup

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/google/uuid"

	"grimoire/internal/domain/project/internal/searchsynonym/base"
	"grimoire/internal/domain/project/internal/searchsynonym/base/audit"
	"grimoire/internal/domain/project/internal/searchsynonym/base/store"
	"grimoire/internal/infra/db"
	"grimoire/lib/common/errs"
)

type okAccess struct{ actor base.Actor }

func (o okAccess) Require(context.Context, string) (base.Actor, error) { return o.actor, nil }

type nopAudit struct{}

func (nopAudit) Record(context.Context, audit.Event) {}

func TestUpdateGroup_Integration(t *testing.T) {
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
	creator := base.Actor{ID: uuid.NewString(), Label: "Budi"}
	editor := base.Actor{ID: uuid.NewString(), Label: "Sari", Role: "developer"}
	g1, err := s.Insert(ctx, base.NewGroup(pa, []string{"login", "masuk"}, creator))
	if err != nil {
		t.Fatal(err)
	}
	g2, err := s.Insert(ctx, base.NewGroup(pa, []string{"token", "kunci"}, creator))
	if err != nil {
		t.Fatal(err)
	}
	other, err := s.Insert(ctx, base.NewGroup(pb, []string{"login", "masuk"}, creator))
	if err != nil {
		t.Fatal(err)
	}
	uc := NewUsecase(okAccess{editor}, NewRepository(s), nopAudit{})

	// Term miliknya sendiri tidak 409; term baru yang sama dengan project lain juga aman.
	dto, err := uc.Execute(ctx, pa, g1.ID, Input{Terms: []string{"login", "masuk", "signin"}}, base.RequestMeta{})
	if err != nil || !reflect.DeepEqual(dto.Terms, []string{"login", "masuk", "signin"}) ||
		dto.UpdatedByLabel != "Sari" || dto.CreatedByLabel != "Budi" {
		t.Fatalf("%+v %v", dto, err)
	}

	// Term milik grup lain di project yang sama -> 409.
	_, err = uc.Execute(ctx, pa, g1.ID, Input{Terms: []string{"login", "token"}}, base.RequestMeta{})
	var ee *errs.Error
	if !errors.As(err, &ee) || ee.Code != errs.CodeConflict {
		t.Fatalf("want conflict, got %v", err)
	}
	if m, _ := ee.Details.(map[string]any); !reflect.DeepEqual(m["terms"], []string{"token"}) {
		t.Fatalf("details = %v", ee.Details)
	}
	_ = g2

	// Grup project lain -> 404.
	if _, err := uc.Execute(ctx, pa, other.ID, Input{Terms: []string{"x1", "y1"}}, base.RequestMeta{}); !errors.Is(err, base.ErrGroupNotFound) {
		t.Fatalf("want not found, got %v", err)
	}
	// Grup yang tak ada -> 404.
	if _, err := uc.Execute(ctx, pa, uuid.NewString(), Input{Terms: []string{"x1", "y1"}}, base.RequestMeta{}); !errors.Is(err, base.ErrGroupNotFound) {
		t.Fatalf("want not found, got %v", err)
	}
}
