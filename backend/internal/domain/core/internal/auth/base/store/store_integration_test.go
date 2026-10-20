//go:build integration

package store

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	"grimoire/internal/domain/core/internal/auth/base"
	"grimoire/internal/infra/db"
	"grimoire/migrations"
)

func setup(t *testing.T) (*db.DB, *Store) {
	t.Helper()
	d := db.TestDB(t)
	if _, err := db.Migrate(context.Background(), d, migrations.FS, nil); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return d, New(d)
}

func newUser(email, role string) base.User {
	return base.User{Email: email, Name: "N", PasswordHash: "h", Role: role, IsActive: true, MustChangePassword: true}
}

func TestStore_InsertUser_EmailCaseInsensitiveUnique(t *testing.T) {
	_, s := setup(t)
	ctx := context.Background()
	u, err := s.InsertUser(ctx, newUser("A@x.id", base.RoleDeveloper))
	if err != nil || u.ID == "" || u.CreatedAt.IsZero() || u.Email != "A@x.id" {
		t.Fatalf("%+v %v", u, err)
	}
	if _, err := s.InsertUser(ctx, newUser("a@x.id", base.RoleDeveloper)); !errors.Is(err, base.ErrEmailTaken) {
		t.Fatalf("want ErrEmailTaken, got %v", err)
	}
	got, ok, err := s.FindUserByEmail(ctx, "a@X.ID")
	if err != nil || !ok || got.ID != u.ID {
		t.Fatalf("FindUserByEmail: %+v %v %v", got, ok, err)
	}
}

func TestStore_InsertUser_CreatedByAndTimes(t *testing.T) {
	_, s := setup(t)
	ctx := context.Background()
	admin, _ := s.InsertUser(ctx, newUser("adm@x.id", base.RoleSuperAdmin))
	in := newUser("d@x.id", base.RoleDeveloper)
	lbl := "adm@x.id"
	in.CreatedBy, in.CreatedByLabel = &admin.ID, &lbl
	in.CreatedAt = time.Date(2026, 10, 21, 9, 0, 0, 0, time.UTC)
	in.UpdatedAt = in.CreatedAt
	u, err := s.InsertUser(ctx, in)
	if err != nil || u.CreatedBy == nil || *u.CreatedBy != admin.ID || !u.CreatedAt.Equal(in.CreatedAt) || u.LastLoginAt != nil {
		t.Fatalf("%+v %v", u, err)
	}
}

func TestStore_FindUserByID(t *testing.T) {
	_, s := setup(t)
	ctx := context.Background()
	u, _ := s.InsertUser(ctx, newUser("a@x.id", base.RoleDeveloper))
	if got, ok, err := s.FindUserByID(ctx, u.ID); err != nil || !ok || got.Email != "a@x.id" || got.PasswordHash != "h" {
		t.Fatalf("%+v %v %v", got, ok, err)
	}
	for _, id := range []string{"bukan-uuid", "", "00000000-0000-0000-0000-000000000000"} {
		if _, ok, err := s.FindUserByID(ctx, id); ok || err != nil {
			t.Errorf("%q: ok=%v err=%v", id, ok, err)
		}
	}
}

func TestStore_FindUserByID_InvalidUUID(t *testing.T) {
	// Pool ditutup: bila query dijalankan akan error; id invalid harus kembali (zero,false,nil) tanpa query.
	d, s := setup(t)
	_ = d.Close()
	if _, ok, err := s.FindUserByID(context.Background(), "bukan-uuid"); ok || err != nil {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if _, ok, err := s.LockUserByID(context.Background(), "x"); ok || err != nil {
		t.Fatalf("lock ok=%v err=%v", ok, err)
	}
}

func TestStore_UpdatePassword(t *testing.T) {
	_, s := setup(t)
	ctx := context.Background()
	u, _ := s.InsertUser(ctx, newUser("a@x.id", base.RoleDeveloper))
	now := time.Now().UTC().Truncate(time.Microsecond)
	if err := s.UpdatePassword(ctx, u.ID, "baru", false, now); err != nil {
		t.Fatal(err)
	}
	got, _, _ := s.FindUserByID(ctx, u.ID)
	if got.PasswordHash != "baru" || got.MustChangePassword || !got.UpdatedAt.Equal(now) {
		t.Fatalf("%+v", got)
	}
	if err := s.UpdatePassword(ctx, "00000000-0000-0000-0000-000000000000", "x", false, now); !errors.Is(err, base.ErrUserNotFound) {
		t.Fatalf("got %v", err)
	}
}

func addSession(t *testing.T, d *db.DB, userID string, hash byte) {
	t.Helper()
	_, err := d.Exec(`INSERT INTO sessions (user_id, token_hash, expires_at) VALUES ($1::uuid, $2, now() + interval '1 day')`, userID, []byte{hash})
	if err != nil {
		t.Fatal(err)
	}
}

func TestStore_DeleteSessionsByUser_Except(t *testing.T) {
	d, s := setup(t)
	ctx := context.Background()
	a, _ := s.InsertUser(ctx, newUser("a@x.id", base.RoleDeveloper))
	b, _ := s.InsertUser(ctx, newUser("b@x.id", base.RoleDeveloper))
	addSession(t, d, a.ID, 1)
	addSession(t, d, a.ID, 2)
	addSession(t, d, a.ID, 3)
	addSession(t, d, b.ID, 4)
	n, err := s.DeleteSessionsByUser(ctx, a.ID, []byte{2})
	if err != nil || n != 2 {
		t.Fatalf("except: n=%d err=%v", n, err)
	}
	n, err = s.DeleteSessionsByUser(ctx, a.ID, nil)
	if err != nil || n != 1 {
		t.Fatalf("all: n=%d err=%v", n, err)
	}
	var left int
	_ = d.Get(&left, `SELECT count(*) FROM sessions`)
	if left != 1 {
		t.Fatalf("sesi user lain harus utuh, sisa %d", left)
	}
	if n, err := s.DeleteSessionsByUser(ctx, "bukan-uuid", nil); n != 0 || err != nil {
		t.Fatal(n, err)
	}
}

func TestStore_LockActiveSuperAdminIDs_OrderedAndFiltered(t *testing.T) {
	d, s := setup(t)
	ctx := context.Background()
	var want []string
	for _, e := range []string{"c@x.id", "a@x.id", "b@x.id"} {
		u, _ := s.InsertUser(ctx, newUser(e, base.RoleSuperAdmin))
		want = append(want, u.ID)
	}
	inactive := newUser("off@x.id", base.RoleSuperAdmin)
	inactive.IsActive = false
	_, _ = s.InsertUser(ctx, inactive)
	_, _ = s.InsertUser(ctx, newUser("dev@x.id", base.RoleDeveloper))

	err := d.BeginTx(ctx, func(tx *sqlx.Tx) error {
		ids, err := s.WithTx(tx).LockActiveSuperAdminIDs(ctx)
		if err != nil {
			return err
		}
		if len(ids) != 3 {
			t.Fatalf("ids=%v", ids)
		}
		for i := 1; i < len(ids); i++ {
			if ids[i-1] >= ids[i] {
				t.Fatalf("tidak terurut: %v", ids)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = want
}

// Dua super admin aktif saling menonaktifkan secara bersamaan: tepat satu berhasil,
// satu mendapat LAST_SUPER_ADMIN, dan tersisa satu super admin aktif.
func TestStore_LockActiveSuperAdminIDs_Concurrency(t *testing.T) {
	d, s := setup(t)
	ctx := context.Background()
	a, _ := s.InsertUser(ctx, newUser("a@x.id", base.RoleSuperAdmin))
	b, _ := s.InsertUser(ctx, newUser("b@x.id", base.RoleSuperAdmin))

	deactivate := func(actor, target string) error {
		return d.BeginTx(ctx, func(tx *sqlx.Tx) error {
			st := s.WithTx(tx)
			ids, err := st.LockActiveSuperAdminIDs(ctx) // set super admin dulu, baru target
			if err != nil {
				return err
			}
			u, ok, err := st.LockUserByID(ctx, target)
			if err != nil || !ok {
				return errors.Join(err, base.ErrUserNotFound)
			}
			if err := base.CheckDeactivate(actor, u, len(ids)); err != nil {
				return err
			}
			time.Sleep(100 * time.Millisecond) // beri peluang tx lain mengantre
			_, err = tx.ExecContext(ctx, `UPDATE users SET is_active = false WHERE id = $1::uuid`, target)
			return err
		})
	}

	var wg sync.WaitGroup
	errsOut := make([]error, 2)
	wg.Add(2)
	go func() { defer wg.Done(); errsOut[0] = deactivate(a.ID, b.ID) }()
	go func() { defer wg.Done(); errsOut[1] = deactivate(b.ID, a.ID) }()
	wg.Wait()

	ok, last := 0, 0
	for _, e := range errsOut {
		switch {
		case e == nil:
			ok++
		case errors.Is(e, base.ErrLastSuperAdmin):
			last++
		default:
			t.Fatalf("error tak terduga: %v", e)
		}
	}
	if ok != 1 || last != 1 {
		t.Fatalf("ok=%d last=%d errs=%v", ok, last, errsOut)
	}
	var active int
	_ = d.Get(&active, `SELECT count(*) FROM users WHERE role='super_admin' AND is_active`)
	if active != 1 {
		t.Fatalf("super admin aktif=%d", active)
	}
}

func TestMigration_TablesAndConstraints(t *testing.T) {
	d, s := setup(t)
	ctx := context.Background()
	if _, err := d.Exec(`INSERT INTO users (email,name,password_hash,role) VALUES ('x@x.id','N','h','root')`); err == nil {
		t.Fatal("role invalid harus ditolak CHECK")
	}
	if _, err := d.Exec(`INSERT INTO users (email,name,password_hash,role) VALUES ('y@x.id','','h','developer')`); err == nil {
		t.Fatal("nama kosong harus ditolak CHECK")
	}
	u, _ := s.InsertUser(ctx, newUser("a@x.id", base.RoleDeveloper))
	addSession(t, d, u.ID, 9)
	if _, err := d.Exec(`DELETE FROM users WHERE id = $1::uuid`, u.ID); err != nil {
		t.Fatal(err)
	}
	var n int
	_ = d.Get(&n, `SELECT count(*) FROM sessions`)
	if n != 0 {
		t.Fatalf("sesi harus ikut terhapus (cascade), sisa %d", n)
	}
	if _, err := d.Exec(`INSERT INTO login_throttles (email, failed_count, window_started_at) VALUES ('Z@x.id', 1, now())`); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Exec(`INSERT INTO login_throttles (email, failed_count, window_started_at) VALUES ('z@x.id', 1, now())`); err == nil {
		t.Fatal("login_throttles.email harus citext unik")
	}
}
