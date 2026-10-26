//go:build integration

package creategroup

import (
	"context"
	"errors"
	"sync"
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

func newProject(t *testing.T, d *db.DB) string {
	t.Helper()
	var id string
	if err := d.GetContext(context.Background(), &id, `INSERT INTO projects (name) VALUES ('Proyek') RETURNING id::text`); err != nil {
		t.Fatal(err)
	}
	return id
}

func setupDB(t *testing.T) (*db.DB, *Usecase) {
	t.Helper()
	d := db.TestDB(t)
	if _, err := db.Up(context.Background(), d, nil); err != nil {
		t.Fatal(err)
	}
	actor := base.Actor{ID: uuid.NewString(), Label: "Budi", Role: "developer"}
	return d, NewUsecase(okAccess{actor}, NewRepository(store.New(d)), nopAudit{})
}

// Review Focus 4: dua POST bersamaan dengan term yang sama -> tepat satu sukses, satu term_in_use.
func TestCreateGroup_ConcurrentSameTerm(t *testing.T) {
	for round := 0; round < 5; round++ {
		d, uc := setupDB(t)
		p := newProject(t, d)
		inputs := []Input{{Terms: []string{"login", "masuk"}}, {Terms: []string{"login", "signin"}}}

		errsOut := make([]error, len(inputs))
		start := make(chan struct{})
		var wg sync.WaitGroup
		for i := range inputs {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				_, errsOut[i] = uc.Execute(context.Background(), p, inputs[i], base.RequestMeta{})
			}()
		}
		close(start)
		wg.Wait()

		ok, inUse := 0, 0
		for _, err := range errsOut {
			var ee *errs.Error
			switch {
			case err == nil:
				ok++
			case errors.As(err, &ee) && ee.Code == errs.CodeConflict:
				if m, _ := ee.Details.(map[string]any); m["reason"] == base.ReasonTermInUse {
					inUse++
				}
			default:
				t.Fatalf("error tak terduga: %v", err)
			}
		}
		if ok != 1 || inUse != 1 {
			t.Fatalf("round %d: ok=%d inUse=%d errs=%v", round, ok, inUse, errsOut)
		}
		n, err := store.New(d).Count(context.Background(), p)
		if err != nil || n != 1 {
			t.Fatalf("count = %d %v", n, err)
		}
	}
}

func TestCreateGroup_GroupLimit(t *testing.T) {
	d, uc := setupDB(t)
	p := newProject(t, d)
	actor := base.Actor{ID: uuid.NewString(), Label: "Budi"}
	if _, err := d.ExecContext(context.Background(), `
INSERT INTO knowledge_synonym_groups (project_id, terms, created_by, created_by_label, updated_by, updated_by_label)
SELECT $1::uuid, ARRAY['t' || g, 'u' || g], $2::uuid, 'Budi', $2::uuid, 'Budi' FROM generate_series(1, $3) g`,
		p, actor.ID, base.MaxGroupsPerProject); err != nil {
		t.Fatal(err)
	}
	_, err := uc.Execute(context.Background(), p, Input{Terms: []string{"baru", "segar"}}, base.RequestMeta{})
	var ee *errs.Error
	if !errors.As(err, &ee) || ee.Code != errs.CodeConflict {
		t.Fatalf("err = %v", err)
	}
	if m, _ := ee.Details.(map[string]any); m["reason"] != base.ReasonGroupLimit {
		t.Fatalf("details = %v", ee.Details)
	}
}
