//go:build integration

package updateuser

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"grimoire/internal/domain/core/internal/auth/base"
	"grimoire/internal/domain/core/internal/auth/base/store"
	"grimoire/internal/infra/db"
	"grimoire/migrations"
)

func setupRepo(t *testing.T) (*db.DB, Repository, *store.Store) {
	t.Helper()
	d := db.TestDB(t)
	if _, err := db.Migrate(context.Background(), d, migrations.FS, nil); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return d, NewRepository(d), store.New(d)
}

func seed(t *testing.T, s *store.Store, email, role string) base.User {
	t.Helper()
	u, err := s.InsertUser(context.Background(), base.User{Email: email, Name: "N", PasswordHash: "h", Role: role, IsActive: true})
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func update(r Repository, u base.User) error {
	return r.InTx(context.Background(), func(ctx context.Context, tx TxRepository) error {
		return tx.UpdateUserFields(ctx, u)
	})
}

func TestRepo_UpdateUserFields_Persists(t *testing.T) {
	_, r, s := setupRepo(t)
	u := seed(t, s, "a@x.id", base.RoleDeveloper)
	u.Name, u.Email, u.Role = "Baru", "b@x.id", base.RoleSuperAdmin
	u.UpdatedAt = time.Now().UTC().Add(time.Hour).Truncate(time.Microsecond)
	if err := update(r, u); err != nil {
		t.Fatal(err)
	}
	got, _, _ := s.FindUserByID(context.Background(), u.ID)
	if got.Name != "Baru" || got.Email != "b@x.id" || got.Role != base.RoleSuperAdmin ||
		!got.UpdatedAt.Equal(u.UpdatedAt) || !got.CreatedAt.Equal(u.CreatedAt) {
		t.Fatalf("%+v", got)
	}
}

func TestRepo_UpdateUserFields_EmailUniqueCaseInsensitive(t *testing.T) {
	_, r, s := setupRepo(t)
	seed(t, s, "a@x.id", base.RoleDeveloper)
	b := seed(t, s, "b@x.id", base.RoleDeveloper)
	b.Email = "A@X.id"
	b.UpdatedAt = time.Now().UTC()
	if err := update(r, b); !errors.Is(err, base.ErrEmailTaken) {
		t.Fatalf("got %v", err)
	}
}

func TestRepo_UpdateUserFields_RoleCheckConstraint(t *testing.T) {
	_, r, s := setupRepo(t)
	u := seed(t, s, "a@x.id", base.RoleDeveloper)
	u.Role, u.UpdatedAt = "owner", time.Now().UTC()
	if err := update(r, u); err == nil || errors.Is(err, base.ErrEmailTaken) {
		t.Fatalf("got %v", err)
	}
}

func TestRepo_InTx_RollbackOnError(t *testing.T) {
	_, r, s := setupRepo(t)
	u := seed(t, s, "a@x.id", base.RoleDeveloper)
	boom := errors.New("boom")
	err := r.InTx(context.Background(), func(ctx context.Context, tx TxRepository) error {
		u.Name, u.UpdatedAt = "Berubah", time.Now().UTC()
		if err := tx.UpdateUserFields(ctx, u); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatal(err)
	}
	got, _, _ := s.FindUserByID(context.Background(), u.ID)
	if got.Name != "N" {
		t.Fatalf("tidak rollback: %q", got.Name)
	}
}

func TestRepo_LockOrder_NoDeadlock(t *testing.T) {
	_, r, s := setupRepo(t)
	a := seed(t, s, "a@x.id", base.RoleSuperAdmin)
	b := seed(t, s, "b@x.id", base.RoleSuperAdmin)
	lockBoth := func(target string) error {
		return r.InTx(context.Background(), func(ctx context.Context, tx TxRepository) error {
			if _, err := tx.LockActiveSuperAdminIDs(ctx); err != nil {
				return err
			}
			if _, _, err := tx.LockUserByID(ctx, target); err != nil {
				return err
			}
			time.Sleep(5 * time.Millisecond)
			return nil
		})
	}
	for i := 0; i < 50; i++ {
		var wg sync.WaitGroup
		errs := make([]error, 2)
		wg.Add(2)
		go func() { defer wg.Done(); errs[0] = lockBoth(b.ID) }()
		go func() { defer wg.Done(); errs[1] = lockBoth(a.ID) }()
		wg.Wait()
		if errs[0] != nil || errs[1] != nil {
			t.Fatalf("iterasi %d: %v %v", i, errs[0], errs[1])
		}
	}
}
