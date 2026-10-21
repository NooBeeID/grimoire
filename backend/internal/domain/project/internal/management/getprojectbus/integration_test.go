//go:build integration

package getprojectbus_test

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"

	"grimoire/internal/domain/project/internal/management/getprojectbus"
	"grimoire/internal/infra/app"
	"grimoire/internal/infra/bus"
	"grimoire/internal/infra/db"
	"grimoire/lib/constants"
	"grimoire/lib/contracts/project/management/getproject"
)

func setup(t *testing.T) (*db.DB, *bus.Registry) {
	t.Helper()
	d := db.TestDB(t)
	if _, err := db.Up(context.Background(), d, nil); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	r := bus.NewRegistry(nil)
	getprojectbus.Init(app.Deps{DB: d, Bus: r})
	return d, r
}

func call(t *testing.T, r *bus.Registry, id string) getproject.Response {
	t.Helper()
	res, err := bus.CallTyped[getproject.Response](context.Background(), r, constants.BUS_PROJECT_MANAGEMENT_GET_PROJECT, getproject.Request{ProjectID: id})
	if err != nil {
		t.Fatalf("call %q: %v", id, err)
	}
	return res
}

func insert(t *testing.T, d *db.DB, name string) string {
	t.Helper()
	var id string
	if err := d.GetContext(context.Background(), &id, `INSERT INTO projects (name) VALUES ($1) RETURNING id::text`, name); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestGetProjectBus_Integration_Found(t *testing.T) {
	d, r := setup(t)
	id := insert(t, d, "Payments")
	if res := call(t, r, id); res != (getproject.Response{Found: true, ID: id, Name: "Payments"}) {
		t.Fatalf("got %+v", res)
	}
}

func TestGetProjectBus_Integration_UnknownUUID(t *testing.T) {
	_, r := setup(t)
	if res := call(t, r, uuid.NewString()); res.Found {
		t.Fatalf("got %+v", res)
	}
}

func TestGetProjectBus_Integration_InvalidUUID(t *testing.T) {
	d, r := setup(t)
	insert(t, d, "Keep")
	for _, id := range []string{"abc", "123", "'; DROP TABLE projects;--", "", strings.Repeat("a", 1000)} {
		if res := call(t, r, id); res.Found {
			t.Fatalf("%q: %+v", id, res)
		}
	}
	var n int
	if err := d.GetContext(context.Background(), &n, `SELECT count(*) FROM projects`); err != nil || n != 1 {
		t.Fatalf("projects n=%d err=%v", n, err)
	}
}

func TestGetProjectBus_Integration_NoActorRequired(t *testing.T) {
	d, r := setup(t)
	id := insert(t, d, "Proj")
	if res := call(t, r, id); !res.Found {
		t.Fatalf("got %+v", res)
	}
}

func TestGetProjectBus_Integration_ReflectsRename(t *testing.T) {
	d, r := setup(t)
	id := insert(t, d, "Old")
	call(t, r, id)
	if _, err := d.ExecContext(context.Background(), `UPDATE projects SET name='New' WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	if res := call(t, r, id); res.Name != "New" {
		t.Fatalf("got %+v", res)
	}
}

func TestGetProjectBus_Integration_AfterDelete(t *testing.T) {
	d, r := setup(t)
	id := insert(t, d, "Gone")
	if _, err := d.ExecContext(context.Background(), `DELETE FROM projects WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	if res := call(t, r, id); res.Found {
		t.Fatalf("got %+v", res)
	}
}

func TestGetProjectBus_Integration_Concurrent(t *testing.T) {
	d, r := setup(t)
	id := insert(t, d, "Par")
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res, err := bus.CallTyped[getproject.Response](context.Background(), r, constants.BUS_PROJECT_MANAGEMENT_GET_PROJECT, getproject.Request{ProjectID: id})
			if err != nil || res != (getproject.Response{Found: true, ID: id, Name: "Par"}) {
				t.Errorf("got %+v %v", res, err)
			}
		}()
	}
	wg.Wait()
}

func TestGetProjectBus_Integration_NoAudit(t *testing.T) {
	d, r := setup(t)
	var mu sync.Mutex
	calls := 0
	r.Register(constants.BUS_CORE_AUDIT_RECORD, func(context.Context, any) (any, error) {
		mu.Lock()
		calls++
		mu.Unlock()
		return nil, nil
	})
	call(t, r, insert(t, d, "Proj"))
	mu.Lock()
	defer mu.Unlock()
	if calls != 0 {
		t.Fatalf("audit calls=%d", calls)
	}
}
