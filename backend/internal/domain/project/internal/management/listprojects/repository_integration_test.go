//go:build integration

package listprojects

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"grimoire/internal/infra/db"
)

type seedData struct {
	d       *db.DB
	A, B, C string
	p       map[string]string // nama -> id
	repo    *Repo
	order   []string // urutan harapan (updated_at DESC, id DESC)
}

func seedProjects(t *testing.T) *seedData {
	t.Helper()
	d := db.TestDB(t)
	if _, err := db.Up(context.Background(), d, nil); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	s := &seedData{d: d, A: uuid.NewString(), B: uuid.NewString(), C: uuid.NewString(), p: map[string]string{}, repo: NewRepo(d)}
	ctx := context.Background()
	base := time.Date(2026, 10, 22, 10, 0, 0, 0, time.UTC)
	specs := []struct {
		name string
		ago  int
	}{{"P1", 1}, {"P2", 2}, {"P3", 3}, {"P4", 4}, {"P5", 5}, {"50%_off", 6}, {"Payments", 7}}
	for _, sp := range specs {
		var id string
		if err := d.GetContext(ctx, &id, `INSERT INTO projects (name, updated_at) VALUES ($1,$2) RETURNING id::text`,
			sp.name, base.Add(-time.Duration(sp.ago)*time.Hour)); err != nil {
			t.Fatal(err)
		}
		s.p[sp.name] = id
		s.order = append(s.order, sp.name)
	}
	mem := func(p, u, role string) {
		if _, err := d.ExecContext(ctx, `INSERT INTO project_members (project_id, user_id, role) VALUES ($1,$2,$3)`, s.p[p], u, role); err != nil {
			t.Fatal(err)
		}
	}
	mem("P1", s.A, "owner")
	mem("P2", s.A, "owner")
	mem("P2", s.B, "member")
	mem("P2", s.C, "member")
	mem("P3", s.A, "member")
	mem("P4", s.B, "owner")
	mem("Payments", s.A, "owner")
	return s
}

func names(s *seedData, items []Item) []string {
	rev := map[string]string{}
	for n, id := range s.p {
		rev[id] = n
	}
	out := []string{}
	for _, it := range items {
		out = append(out, rev[it.Project.ID])
	}
	return out
}

func eq(a, b []string) bool { return strings.Join(a, "|") == strings.Join(b, "|") }

func list(t *testing.T, s *seedData, actor string, restrict bool, f ProjectFilter) ([]Item, int) {
	t.Helper()
	if f.Limit == 0 {
		f.Limit = 100
	}
	items, total, err := s.repo.List(context.Background(), actor, restrict, f)
	if err != nil {
		t.Fatal(err)
	}
	if items == nil {
		t.Fatal("items nil")
	}
	return items, total
}

func TestListProjects_DeveloperOnlyOwnMemberships(t *testing.T) {
	s := seedProjects(t)
	it, total := list(t, s, s.A, true, ProjectFilter{})
	if want := []string{"P1", "P2", "P3", "Payments"}; !eq(names(s, it), want) || total != 4 {
		t.Fatalf("A: %v %d", names(s, it), total)
	}
	if it, _ := list(t, s, s.B, true, ProjectFilter{}); !eq(names(s, it), []string{"P2", "P4"}) {
		t.Fatalf("B: %v", names(s, it))
	}
	if it, total := list(t, s, uuid.NewString(), true, ProjectFilter{}); len(it) != 0 || total != 0 {
		t.Fatal("non-anggota harus kosong")
	}
}

func TestListProjects_SuperAdminSeesAll_LeftJoin(t *testing.T) {
	s := seedProjects(t)
	it, total := list(t, s, uuid.NewString(), false, ProjectFilter{})
	if len(it) != 7 || total != 7 {
		t.Fatalf("%d %d", len(it), total)
	}
	for _, x := range it {
		if x.MyRole != "" {
			t.Fatal("MyRole harus kosong")
		}
	}
	it, _ = list(t, s, s.A, false, ProjectFilter{})
	if len(it) != 7 {
		t.Fatal(len(it))
	}
	roles := map[string]string{}
	for i, n := range names(s, it) {
		roles[n] = it[i].MyRole
	}
	if roles["P1"] != "owner" || roles["P3"] != "member" || roles["P4"] != "" {
		t.Fatalf("%v", roles)
	}
}

func TestListProjects_OrderUpdatedAtDescThenID(t *testing.T) {
	s := seedProjects(t)
	ts := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	var ids []string
	for i := 0; i < 2; i++ {
		var id string
		if err := s.d.GetContext(context.Background(), &id, `INSERT INTO projects (name, updated_at) VALUES ($1,$2) RETURNING id::text`, fmt.Sprintf("Tie%d", i), ts); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	it, _ := list(t, s, s.A, false, ProjectFilter{})
	a, b := it[0].Project.ID, it[1].Project.ID
	if a < b {
		t.Fatalf("tie-break id DESC: %s %s", a, b)
	}
	if a != maxStr(ids[0], ids[1]) {
		t.Fatal("dua teratas harus project tie")
	}
	rest := names(s, it[2:])
	if !eq(rest, s.order) {
		t.Fatalf("%v", rest)
	}
}

func maxStr(a, b string) string {
	if a > b {
		return a
	}
	return b
}

func TestListProjects_MemberCountCorrect(t *testing.T) {
	s := seedProjects(t)
	it, _ := list(t, s, s.A, true, ProjectFilter{})
	got := map[string]int{}
	for i, n := range names(s, it) {
		got[n] = it[i].MemberCount
	}
	if got["P2"] != 3 || got["P1"] != 1 {
		t.Fatalf("%v", got)
	}
	it, _ = list(t, s, s.A, false, ProjectFilter{})
	for i, n := range names(s, it) {
		if n == "P5" && it[i].MemberCount != 0 {
			t.Fatal("P5 harus 0")
		}
	}
}

func TestListProjects_SearchCaseInsensitive(t *testing.T) {
	s := seedProjects(t)
	it, total := list(t, s, s.A, true, ProjectFilter{Query: "PAY"})
	if !eq(names(s, it), []string{"Payments"}) || total != 1 {
		t.Fatalf("%v", names(s, it))
	}
}

func TestListProjects_SearchEscapesWildcards(t *testing.T) {
	s := seedProjects(t)
	for _, q := range []string{"%", "_"} {
		it, _ := list(t, s, uuid.NewString(), false, ProjectFilter{Query: q})
		if !eq(names(s, it), []string{"50%_off"}) {
			t.Fatalf("%q: %v", q, names(s, it))
		}
	}
	if it, _ := list(t, s, uuid.NewString(), false, ProjectFilter{Query: `\`}); len(it) != 0 {
		t.Fatalf("backslash literal: %v", names(s, it))
	}
}

func TestListProjects_SQLInjectionInQuery(t *testing.T) {
	s := seedProjects(t)
	it, _ := list(t, s, s.A, true, ProjectFilter{Query: "'; DROP TABLE projects; --"})
	if len(it) != 0 {
		t.Fatal("harus kosong")
	}
	if it, _ := list(t, s, s.A, true, ProjectFilter{}); len(it) != 4 {
		t.Fatal("tabel rusak")
	}
}

func TestListProjects_SearchDoesNotLeakOtherProjects(t *testing.T) {
	s := seedProjects(t)
	if it, _ := list(t, s, s.B, true, ProjectFilter{Query: "Payments"}); len(it) != 0 {
		t.Fatal("bocor")
	}
}

func TestListProjects_Pagination(t *testing.T) {
	s := seedProjects(t)
	full, _ := list(t, s, s.A, false, ProjectFilter{})
	var got []string
	for off := 0; off < 7; off += 2 {
		it, total := list(t, s, s.A, false, ProjectFilter{Limit: 2, Offset: off})
		if total != 7 {
			t.Fatalf("total %d", total)
		}
		got = append(got, names(s, it)...)
	}
	if !eq(got, names(s, full)) {
		t.Fatalf("%v vs %v", got, names(s, full))
	}
}

func TestListProjects_OffsetBeyondEnd_TotalStillCorrect(t *testing.T) {
	s := seedProjects(t)
	cases := []struct {
		actor    string
		restrict bool
		q        string
		want     int
	}{{s.A, true, "", 4}, {s.A, true, "P", 4}, {s.C, false, "", 7}, {s.C, false, "pay", 1}}
	for _, c := range cases {
		it, total := list(t, s, c.actor, c.restrict, ProjectFilter{Query: c.q, Limit: 20, Offset: 100})
		if len(it) != 0 || total != c.want {
			t.Fatalf("%+v: %d", c, total)
		}
	}
}

func TestListProjects_EmptyDB(t *testing.T) {
	d := db.TestDB(t)
	if _, err := db.Up(context.Background(), d, nil); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	s := &seedData{d: d}
	s.repo = NewRepo(s.d)
	it, total := list(t, s, uuid.NewString(), false, ProjectFilter{})
	if len(it) != 0 || total != 0 {
		t.Fatal("harus kosong")
	}
}

func TestListProjects_NonUUIDActorID(t *testing.T) {
	s := seedProjects(t)
	if it, _ := list(t, s, "bukan-uuid", true, ProjectFilter{}); len(it) != 0 {
		t.Fatal("developer harus kosong")
	}
	if it, _ := list(t, s, "bukan-uuid", false, ProjectFilter{}); len(it) != 7 {
		t.Fatal("super_admin harus semua")
	}
}

func TestListProjects_DescriptionAndLabelsMapped(t *testing.T) {
	s := seedProjects(t)
	var id string
	if err := s.d.GetContext(context.Background(), &id,
		`INSERT INTO projects (name, description, created_by, created_by_label) VALUES ('Zed','desk',$1,'z@x') RETURNING id::text`, s.A); err != nil {
		t.Fatal(err)
	}
	it, _ := list(t, s, s.A, false, ProjectFilter{})
	for _, x := range it {
		if x.Project.ID == id {
			if x.Project.Description != "desk" || x.Project.CreatedBy == nil || *x.Project.CreatedBy != s.A || *x.Project.CreatedByLabel != "z@x" {
				t.Fatalf("%+v", x.Project)
			}
		} else if x.Project.CreatedBy != nil || x.Project.CreatedByLabel != nil {
			t.Fatal("NULL harus nil")
		}
		if x.Project.CreatedAt.Location() != time.UTC {
			t.Fatal("harus UTC")
		}
	}
}
