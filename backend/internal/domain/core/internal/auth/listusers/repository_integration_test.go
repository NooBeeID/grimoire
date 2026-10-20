//go:build integration

package listusers

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"grimoire/internal/domain/core/internal/auth/base"
	"grimoire/internal/infra/db"
	"grimoire/migrations"
)

func newRepo(t *testing.T) (*Repo, *db.DB) {
	t.Helper()
	d := db.TestDB(t)
	if _, err := db.Migrate(context.Background(), d, migrations.FS, nil); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return NewRepo(d), d
}

type seedUser struct {
	email, name, role string
	active            bool
	login             *time.Time
}

func seed(t *testing.T, d *db.DB, us []seedUser) {
	t.Helper()
	for _, u := range us {
		if _, err := d.ExecContext(context.Background(), `INSERT INTO users (email, name, password_hash, role, is_active, last_login_at)
			VALUES ($1,$2,'$argon2id$secret',$3,$4,$5)`, u.email, u.name, u.role, u.active, u.login); err != nil {
			t.Fatal(err)
		}
	}
}

func std(t *testing.T, d *db.DB) {
	login := time.Date(2026, 10, 21, 9, 10, 0, 0, time.UTC)
	seed(t, d, []seedUser{
		{"admin@tim.id", "Admin", "super_admin", true, &login},
		{"old@tim.id", "budi", "super_admin", false, nil},
		{"sari1@tim.id", "Sari", "developer", true, nil},
		{"x@sari.id", "Zed", "developer", true, nil},
		{"sari2@tim.id", "Sari", "developer", false, nil},
		{"off@tim.id", "50%_off", "developer", true, nil},
		{"back@tim.id", `back\slash`, "developer", true, nil},
	})
}

func filter(t *testing.T, in base.UserFilterInput) base.UserFilter {
	t.Helper()
	f, err := base.NewUserFilter(in)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func names(us []base.User) string {
	var s []string
	for _, u := range us {
		s = append(s, u.Name)
	}
	return strings.Join(s, ",")
}

func TestListUsers_OrderNameThenID(t *testing.T) {
	r, d := newRepo(t)
	std(t, d)
	us, total, err := r.List(context.Background(), filter(t, base.UserFilterInput{}))
	if err != nil || total != 7 || len(us) != 7 {
		t.Fatalf("n=%d total=%d err=%v", len(us), total, err)
	}
	// urutan mengikuti collation DB: bandingkan dengan ORDER BY langsung.
	var ids []string
	if err := d.SelectContext(context.Background(), &ids, `SELECT id::text FROM users ORDER BY name ASC, id ASC`); err != nil {
		t.Fatal(err)
	}
	for i := range us {
		if us[i].ID != ids[i] {
			t.Fatalf("urutan salah: %s", names(us))
		}
	}
	// dua "Sari" bersebelahan dan terurut oleh id.
	for i := 1; i < len(us); i++ {
		if us[i-1].Name == us[i].Name && us[i-1].ID >= us[i].ID {
			t.Fatalf("tie-break id salah: %s", names(us))
		}
	}
}

func TestListUsers_Filters(t *testing.T) {
	r, d := newRepo(t)
	std(t, d)
	ctx := context.Background()
	f := func(in base.UserFilterInput) ([]base.User, int) {
		us, n, err := r.List(ctx, filter(t, in))
		if err != nil {
			t.Fatal(err)
		}
		return us, n
	}
	if _, n := f(base.UserFilterInput{Role: "super_admin"}); n != 2 {
		t.Fatalf("role n=%d", n)
	}
	if _, n := f(base.UserFilterInput{Status: "inactive"}); n != 2 {
		t.Fatalf("status n=%d", n)
	}
	us, n := f(base.UserFilterInput{Role: "super_admin", Status: "active"})
	if n != 1 || us[0].Email != "admin@tim.id" {
		t.Fatalf("and n=%d %v", n, names(us))
	}
	// q cocok nama atau email, tanpa memandang huruf besar kecil.
	us, n = f(base.UserFilterInput{Query: "SAR"})
	if n != 3 {
		t.Fatalf("search n=%d %s", n, names(us))
	}
}

func TestListUsers_SearchEscapesWildcards(t *testing.T) {
	r, d := newRepo(t)
	std(t, d)
	for q, want := range map[string]string{"%": "50%_off", "_": "50%_off", `\`: `back\slash`} {
		us, n, err := r.List(context.Background(), filter(t, base.UserFilterInput{Query: q}))
		if err != nil || n != 1 || us[0].Name != want {
			t.Fatalf("q=%q n=%d %s err=%v", q, n, names(us), err)
		}
	}
}

func TestListUsers_SQLInjectionInQuery(t *testing.T) {
	r, d := newRepo(t)
	std(t, d)
	us, n, err := r.List(context.Background(), filter(t, base.UserFilterInput{Query: "'; DROP TABLE users; --"}))
	if err != nil || n != 0 || len(us) != 0 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	var c int
	if err := d.GetContext(context.Background(), &c, `SELECT count(*) FROM users`); err != nil || c != 7 {
		t.Fatalf("users hilang: %d %v", c, err)
	}
}

func TestListUsers_Pagination(t *testing.T) {
	r, d := newRepo(t)
	std(t, d)
	ctx := context.Background()
	all, _, _ := r.List(ctx, filter(t, base.UserFilterInput{}))
	var got []string
	for off := 0; off < 9; off += 3 {
		l, o := 3, off
		us, total, err := r.List(ctx, filter(t, base.UserFilterInput{Limit: &l, Offset: &o}))
		if err != nil || total != 7 {
			t.Fatalf("off=%d total=%d err=%v", off, total, err)
		}
		for _, u := range us {
			got = append(got, u.ID)
		}
	}
	if len(got) != len(all) {
		t.Fatalf("got %d want %d", len(got), len(all))
	}
	for i := range all {
		if got[i] != all[i].ID {
			t.Fatalf("beda di %d", i)
		}
	}
}

func TestListUsers_OffsetBeyondEnd_TotalStillCorrect(t *testing.T) {
	r, d := newRepo(t)
	std(t, d)
	o := 100
	us, total, err := r.List(context.Background(), filter(t, base.UserFilterInput{Offset: &o}))
	if err != nil || len(us) != 0 || us == nil || total != 7 {
		t.Fatalf("n=%d total=%d err=%v", len(us), total, err)
	}
	us, total, err = r.List(context.Background(), filter(t, base.UserFilterInput{Offset: &o, Role: "super_admin"}))
	if err != nil || len(us) != 0 || total != 2 {
		t.Fatalf("filtered total=%d err=%v", total, err)
	}
}

func TestListUsers_EmptyDB(t *testing.T) {
	r, _ := newRepo(t)
	us, total, err := r.List(context.Background(), filter(t, base.UserFilterInput{}))
	if err != nil || us == nil || len(us) != 0 || total != 0 {
		t.Fatalf("us=%v total=%d err=%v", us, total, err)
	}
}

func TestListUsers_NeverSelectsPasswordHash_LastLogin(t *testing.T) {
	r, d := newRepo(t)
	std(t, d)
	us, _, err := r.List(context.Background(), filter(t, base.UserFilterInput{}))
	if err != nil {
		t.Fatal(err)
	}
	var withLogin, without int
	for _, u := range us {
		if u.PasswordHash != "" {
			t.Fatal("password_hash terbaca")
		}
		if u.LastLoginAt != nil {
			withLogin++
			if u.LastLoginAt.Location() != time.UTC && u.LastLoginAt.UTC().Format(time.RFC3339) != "2026-10-21T09:10:00Z" {
				t.Fatalf("login=%v", u.LastLoginAt)
			}
		} else {
			without++
		}
	}
	if withLogin != 1 || without != 6 {
		t.Fatal(fmt.Sprint(withLogin, without))
	}
}
