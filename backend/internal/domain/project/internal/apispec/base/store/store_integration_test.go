//go:build integration

package store

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	"grimoire/internal/domain/project/internal/apispec/base"
	"grimoire/internal/infra/db"
)

const actorID = "0b6c0000-0000-4000-8000-000000000001"

func setup(t *testing.T) (*db.DB, *Store, string) {
	t.Helper()
	d := db.TestDB(t)
	if _, err := db.Up(context.Background(), d, nil); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return d, New(d), newProject(t, d, "Alpha")
}

func newProject(t *testing.T, d *db.DB, name string) string {
	t.Helper()
	var pid string
	if err := d.GetContext(context.Background(), &pid, `INSERT INTO projects (name) VALUES ($1) RETURNING id::text`, name); err != nil {
		t.Fatal(err)
	}
	return pid
}

func insertSpec(t *testing.T, q db.Querier, pid, name string) base.Spec {
	t.Helper()
	sp := base.NewSpec(pid, name, nil, base.DocMeta{Title: "T " + name, APIVersion: "1", OpenAPIVersion: "3.0.3"},
		"openapi: 3.0.3 # "+name, base.FormatYAML, base.Actor{ID: actorID, Label: "Budi"}, time.Now())
	var id string
	err := q.GetContext(context.Background(), &id, `INSERT INTO api_specs (project_id, name, source_path, title, api_version, openapi_version,
content, content_format, content_hash, size_bytes, created_at, created_by, created_by_label, updated_at, updated_by, updated_by_label)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$11,$12,$13) RETURNING id::text`,
		pid, name, "dir/"+name+".yaml", sp.Title, sp.APIVersion, sp.OpenAPIVersion, sp.Content, sp.Format, sp.Hash[:], sp.SizeBytes,
		sp.CreatedAt, actorID, "Budi")
	if err != nil {
		t.Fatal(err)
	}
	sp.ID = id
	return sp
}

func str(s string) *string { return &s }

func TestStore_CRUDBasics(t *testing.T) {
	_, s, pid := setup(t)
	ctx := context.Background()
	want := insertSpec(t, s.Querier(), pid, "payment")

	got, ok, err := s.FindSpec(ctx, pid, "payment")
	if err != nil || !ok {
		t.Fatalf("find: %v %v", ok, err)
	}
	if got.Content != want.Content || got.Hash != want.Hash || got.Format != "yaml" || got.CreatedAt.Location().String() != "UTC" ||
		got.CreatedBy != actorID || got.SourcePath == nil || *got.SourcePath != "dir/payment.yaml" || got.ID != want.ID {
		t.Fatalf("%+v", got)
	}
	m, ok, err := s.FindSpecMeta(ctx, pid, "payment")
	if err != nil || !ok || m.Title != "T payment" || m.EndpointCount != 0 {
		t.Fatalf("meta: %+v %v %v", m, ok, err)
	}
	if _, ok, err := s.FindSpec(ctx, pid, "nope"); ok || err != nil {
		t.Fatalf("missing: %v %v", ok, err)
	}
	if n, err := s.CountSpecs(ctx, pid); err != nil || n != 1 {
		t.Fatalf("count: %d %v", n, err)
	}
}

func TestStore_NonUUIDReturnsEmptyWithoutQuery(t *testing.T) {
	_, s, _ := setup(t)
	ctx := context.Background()
	if _, ok, err := s.FindSpec(ctx, "bukan-uuid", "x"); ok || err != nil {
		t.Fatalf("%v %v", ok, err)
	}
	if _, ok, err := s.FindSpecMeta(ctx, "bukan-uuid", "x"); ok || err != nil {
		t.Fatalf("%v %v", ok, err)
	}
	if l, err := s.ListSpecs(ctx, "x"); err != nil || l == nil || len(l) != 0 {
		t.Fatalf("%v %v", l, err)
	}
	if l, err := s.ListSpecNames(ctx, "x"); err != nil || l == nil || len(l) != 0 {
		t.Fatalf("%v %v", l, err)
	}
	if n, err := s.CountSpecs(ctx, "x"); err != nil || n != 0 {
		t.Fatalf("%d %v", n, err)
	}
	if es, err := s.ListSpecEndpoints(ctx, "x"); err != nil || es == nil || len(es) != 0 {
		t.Fatalf("%v %v", es, err)
	}
}

func TestStore_ListSpecsOrderAndEndpointCount(t *testing.T) {
	_, s, pid := setup(t)
	ctx := context.Background()
	b := insertSpec(t, s.Querier(), pid, "b-spec")
	insertSpec(t, s.Querier(), pid, "a-spec")
	if err := s.ReplaceEndpoints(ctx, b.ID, []base.Endpoint{{Method: "GET", Path: "/x", Tags: []string{}}, {Method: "POST", Path: "/x", Tags: []string{"t"}}}); err != nil {
		t.Fatal(err)
	}
	l, err := s.ListSpecs(ctx, pid)
	if err != nil || len(l) != 2 || l[0].Name != "a-spec" || l[1].Name != "b-spec" || l[0].EndpointCount != 0 || l[1].EndpointCount != 2 {
		t.Fatalf("%+v %v", l, err)
	}
	names, err := s.ListSpecNames(ctx, pid)
	if err != nil || len(names) != 2 || names[0] != "a-spec" {
		t.Fatalf("%v %v", names, err)
	}
}

func TestStore_ReplaceEndpointsReplacesAll(t *testing.T) {
	_, s, pid := setup(t)
	ctx := context.Background()
	sp := insertSpec(t, s.Querier(), pid, "pay")
	first := []base.Endpoint{
		{Method: "DELETE", Path: "/b", Summary: "x", Tags: []string{"a", "b"}, Deprecated: true},
		{Method: "POST", Path: "/a", OperationID: str("createA"), Tags: []string{}},
		{Method: "GET", Path: "/a", OperationID: str("listA"), Tags: []string{"z"}},
		{Method: "GET", Path: "/old", Tags: nil},
	}
	if err := s.ReplaceEndpoints(ctx, sp.ID, first); err != nil {
		t.Fatal(err)
	}
	got, err := s.ListSpecEndpoints(ctx, sp.ID)
	if err != nil || len(got) != 4 {
		t.Fatalf("%+v %v", got, err)
	}
	order := []string{"GET /a", "POST /a", "DELETE /b", "GET /old"}
	for i, e := range got {
		if e.Method+" "+e.Path != order[i] {
			t.Fatalf("urutan: %+v", got)
		}
		if e.Tags == nil {
			t.Fatal("tags nil")
		}
	}
	if got[0].OperationID == nil || *got[0].OperationID != "listA" || got[3].OperationID != nil || !got[2].Deprecated ||
		len(got[2].Tags) != 2 || got[2].Tags[1] != "b" {
		t.Fatalf("%+v", got)
	}
	if err := s.ReplaceEndpoints(ctx, sp.ID, []base.Endpoint{{Method: "PATCH", Path: "/n", Tags: []string{}}}); err != nil {
		t.Fatal(err)
	}
	got, _ = s.ListSpecEndpoints(ctx, sp.ID)
	if len(got) != 1 || got[0].Path != "/n" {
		t.Fatalf("endpoint lama harus hilang: %+v", got)
	}
	if err := s.ReplaceEndpoints(ctx, sp.ID, nil); err != nil {
		t.Fatal(err)
	}
	if n, _ := s.CountEndpoints(ctx, sp.ID); n != 0 {
		t.Fatalf("n=%d", n)
	}
}

func TestStore_NameTakenMapping(t *testing.T) {
	d, s, pid := setup(t)
	insertSpec(t, s.Querier(), pid, "dup")
	sp := base.NewSpec(pid, "dup", nil, base.DocMeta{}, "x", base.FormatYAML, base.Actor{ID: actorID, Label: "B"}, time.Now())
	_, err := d.ExecContext(context.Background(), `INSERT INTO api_specs (project_id, name, title, api_version, openapi_version, content,
content_format, content_hash, size_bytes, created_by, created_by_label, updated_by, updated_by_label)
VALUES ($1,'dup','t','1','3.0.3','x','yaml',$2,1,$3,'B',$3,'B')`, pid, sp.Hash[:], actorID)
	if err == nil || !errors.Is(MapNameTaken(err), ErrNameTaken) {
		t.Fatalf("err=%v mapped=%v", err, MapNameTaken(err))
	}
	if other := errors.New("lain"); MapNameTaken(other) != other {
		t.Fatal("error lain harus apa adanya")
	}
}

func TestStore_LockProjectSpecsSerializes(t *testing.T) {
	d, _, pid := setup(t)
	ctx := context.Background()
	var mu sync.Mutex
	var events []string
	log := func(s string) { mu.Lock(); events = append(events, s); mu.Unlock() }
	acquired := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_ = d.BeginTx(ctx, func(tx *sqlx.Tx) error {
			if err := New(d).WithTx(tx).LockProjectSpecs(ctx, pid); err != nil {
				return err
			}
			log("a-lock")
			close(acquired)
			time.Sleep(300 * time.Millisecond)
			log("a-done")
			return nil
		})
	}()
	go func() {
		defer wg.Done()
		<-acquired
		_ = d.BeginTx(ctx, func(tx *sqlx.Tx) error {
			if err := New(d).WithTx(tx).LockProjectSpecs(ctx, pid); err != nil {
				return err
			}
			log("b-lock")
			return nil
		})
	}()
	wg.Wait()
	if len(events) != 3 || events[1] != "a-done" || events[2] != "b-lock" {
		t.Fatalf("tx kedua tidak menunggu: %v", events)
	}
}

func TestStore_LockSpecInTx(t *testing.T) {
	d, s, pid := setup(t)
	ctx := context.Background()
	insertSpec(t, s.Querier(), pid, "lk")
	err := d.BeginTx(ctx, func(tx *sqlx.Tx) error {
		sp, ok, err := s.WithTx(tx).LockSpec(ctx, pid, "lk")
		if err != nil || !ok || sp.Name != "lk" {
			t.Fatalf("%+v %v %v", sp, ok, err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestStore_ProjectIsolationAndCascade(t *testing.T) {
	d, s, pidA := setup(t)
	ctx := context.Background()
	pidB := newProject(t, d, "Beta")
	a := insertSpec(t, s.Querier(), pidA, "shared")
	if err := s.ReplaceEndpoints(ctx, a.ID, []base.Endpoint{{Method: "GET", Path: "/x", Tags: []string{}}}); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := s.FindSpec(ctx, pidB, "shared"); ok {
		t.Fatal("spec project A terlihat dari B")
	}
	if l, _ := s.ListSpecs(ctx, pidB); len(l) != 0 {
		t.Fatal("list B bocor")
	}
	if _, err := d.ExecContext(ctx, `DELETE FROM projects WHERE id = $1`, pidA); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := d.GetContext(ctx, &n, `SELECT (SELECT count(*) FROM api_specs) + (SELECT count(*) FROM api_spec_endpoints)`); err != nil || n != 0 {
		t.Fatalf("cascade gagal: %d %v", n, err)
	}
}
