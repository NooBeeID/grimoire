//go:build integration

package store

import (
	"context"
	"reflect"
	"testing"

	"github.com/google/uuid"

	"grimoire/internal/domain/project/internal/searchsynonym/base"
	"grimoire/internal/infra/db"
)

var ctx = context.Background()

var actor = base.Actor{ID: uuid.NewString(), Label: "Budi", Role: "developer"}

func setup(t *testing.T) (*db.DB, *Store) {
	t.Helper()
	d := db.TestDB(t)
	if _, err := db.Up(ctx, d, nil); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return d, New(d)
}

func newProject(t *testing.T, d *db.DB, name string) string {
	t.Helper()
	var id string
	if err := d.GetContext(ctx, &id, `INSERT INTO projects (name) VALUES ($1) RETURNING id::text`, name); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestStore_CRUD(t *testing.T) {
	d, s := setup(t)
	p := newProject(t, d, "Alpha")

	g, err := s.Insert(ctx, base.NewGroup(p, []string{"login", "autentikasi"}, actor))
	if err != nil {
		t.Fatal(err)
	}
	if g.ID == "" || g.ProjectID != p || !reflect.DeepEqual(g.Terms, []string{"login", "autentikasi"}) ||
		g.CreatedByLabel != "Budi" || g.CreatedAt.Location().String() != "UTC" {
		t.Fatalf("insert = %+v", g)
	}

	got, ok, err := s.Get(ctx, p, g.ID)
	if err != nil || !ok || !reflect.DeepEqual(got.Terms, g.Terms) {
		t.Fatalf("get = %+v %v %v", got, ok, err)
	}

	other := base.Actor{ID: uuid.NewString(), Label: "Sari"}
	upd, ok, err := s.Update(ctx, g.WithTerms([]string{"login", "masuk", "sign in"}, other))
	if err != nil || !ok || len(upd.Terms) != 3 || upd.UpdatedByLabel != "Sari" || upd.CreatedByLabel != "Budi" {
		t.Fatalf("update = %+v %v %v", upd, ok, err)
	}

	del, ok, err := s.Delete(ctx, p, g.ID)
	if err != nil || !ok || del.ID != g.ID {
		t.Fatalf("delete = %+v %v %v", del, ok, err)
	}
	if _, ok, _ := s.Get(ctx, p, g.ID); ok {
		t.Fatal("harus hilang")
	}
	if _, ok, _ := s.Delete(ctx, p, g.ID); ok {
		t.Fatal("hapus ulang harus not found")
	}
}

func TestStore_InvalidIDsNotFound(t *testing.T) {
	_, s := setup(t)
	if _, ok, err := s.Get(ctx, "x", "y"); ok || err != nil {
		t.Fatalf("%v %v", ok, err)
	}
	if _, ok, err := s.Delete(ctx, uuid.NewString(), "bukan-uuid"); ok || err != nil {
		t.Fatalf("%v %v", ok, err)
	}
	if _, ok, err := s.Update(ctx, base.Group{ID: "bad", ProjectID: uuid.NewString()}); ok || err != nil {
		t.Fatalf("%v %v", ok, err)
	}
	if l, err := s.List(ctx, "bukan-uuid"); err != nil || len(l) != 0 {
		t.Fatalf("%v %v", l, err)
	}
}

func TestStore_ListAndCountIsolated(t *testing.T) {
	d, s := setup(t)
	a, b := newProject(t, d, "Alpha"), newProject(t, d, "Bravo")
	for _, terms := range [][]string{{"a1", "a2"}, {"b1", "b2"}} {
		if _, err := s.Insert(ctx, base.NewGroup(a, terms, actor)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.Insert(ctx, base.NewGroup(b, []string{"x1", "x2"}, actor)); err != nil {
		t.Fatal(err)
	}
	l, err := s.List(ctx, a)
	if err != nil || len(l) != 2 || l[0].Terms[0] != "a1" || l[1].Terms[0] != "b1" {
		t.Fatalf("list = %+v %v", l, err)
	}
	if n, _ := s.Count(ctx, a); n != 2 {
		t.Fatalf("count = %d", n)
	}
	if n, _ := s.Count(ctx, b); n != 1 {
		t.Fatalf("count b = %d", n)
	}
}

func TestStore_TermsInUse(t *testing.T) {
	d, s := setup(t)
	a, b := newProject(t, d, "Alpha"), newProject(t, d, "Bravo")
	g, _ := s.Insert(ctx, base.NewGroup(a, []string{"login", "masuk"}, actor))
	if _, err := s.Insert(ctx, base.NewGroup(b, []string{"deploy", "rilis"}, actor)); err != nil {
		t.Fatal(err)
	}

	got, err := s.TermsInUse(ctx, a, []string{"masuk", "login", "baru"}, "")
	if err != nil || !reflect.DeepEqual(got, []string{"login", "masuk"}) {
		t.Fatalf("in use = %v %v", got, err)
	}
	if got, _ := s.TermsInUse(ctx, a, []string{"login"}, g.ID); len(got) != 0 {
		t.Fatalf("exceptID harus dikecualikan: %v", got)
	}
	if got, _ := s.TermsInUse(ctx, a, []string{"deploy"}, ""); len(got) != 0 {
		t.Fatalf("project lain tidak boleh terlihat: %v", got)
	}
}

func TestStore_CascadeOnProjectDelete(t *testing.T) {
	d, s := setup(t)
	p := newProject(t, d, "Alpha")
	if _, err := s.Insert(ctx, base.NewGroup(p, []string{"a1", "b1"}, actor)); err != nil {
		t.Fatal(err)
	}
	if _, err := d.ExecContext(ctx, `DELETE FROM projects WHERE id = $1`, p); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := d.GetContext(ctx, &n, `SELECT count(*) FROM knowledge_synonym_groups`); err != nil || n != 0 {
		t.Fatalf("cascade: %d %v", n, err)
	}
}

func TestStore_CardinalityCheck(t *testing.T) {
	d, s := setup(t)
	p := newProject(t, d, "Alpha")
	if _, err := s.Insert(ctx, base.NewGroup(p, []string{"sendiri"}, actor)); err == nil {
		t.Fatal("1 term harus ditolak CHECK")
	}
	_ = d
}
