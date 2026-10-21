//go:build integration

package createkey

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"testing"

	"grimoire/internal/domain/project/internal/apikey/base"
	"grimoire/internal/domain/project/internal/apikey/base/audit"
	"grimoire/internal/domain/project/internal/apikey/base/store"
	"grimoire/internal/infra/db"
	"grimoire/lib/common/errs"
	"grimoire/lib/common/reqctx"
)

type allowAll struct{}

func (allowAll) Require(context.Context, string, base.Actor) error { return nil }

type nopAuditRecorder struct{}

func (nopAuditRecorder) Record(context.Context, audit.Event) {}

func setup(t *testing.T) (*db.DB, *Repo) {
	t.Helper()
	d := db.TestDB(t)
	if _, err := db.Up(context.Background(), d, nil); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return d, newRepo(d, store.New(d))
}

func newProject(t *testing.T, d *db.DB, name string) string {
	t.Helper()
	var id string
	if err := d.GetContext(context.Background(), &id, `INSERT INTO projects (name) VALUES ($1) RETURNING id::text`, name); err != nil {
		t.Fatal(err)
	}
	return id
}

func mkKey(t *testing.T, pid, name string) (base.Key, base.RawKey) {
	t.Helper()
	raw, err := base.GenerateRawKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	k, err := base.NewKey(pid, name, raw, base.Actor{ID: "", Label: "b@tim.id"}, fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	k.CreatedBy = nil // user tidak ada di tabel users fixture
	return k, raw
}

func insertOne(t *testing.T, r *Repo, k base.Key) (base.Key, error) {
	t.Helper()
	var out base.Key
	err := r.InTx(context.Background(), func(tx Tx) error {
		var e error
		out, e = tx.Insert(context.Background(), k)
		return e
	})
	return out, err
}

func TestRepo_InsertAndConstraints(t *testing.T) {
	d, r := setup(t)
	pid := newProject(t, d, "Alpha")
	k, raw := mkKey(t, pid, "Claude")
	got, err := insertOne(t, r, k)
	if err != nil || got.ID == "" || got.CreatedAt.Location().String() != "UTC" || len(got.SecretHash) != 32 ||
		got.RevokedAt != nil || got.LastUsedAt != nil || got.CreatedByLabel == nil || *got.CreatedByLabel != "b@tim.id" {
		t.Fatalf("%+v %v", got, err)
	}
	var dump string
	if err := d.GetContext(context.Background(), &dump, `SELECT string_agg(row_to_json(k)::text, '') FROM api_keys k`); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains([]byte(dump), []byte(raw.Secret)) || bytes.Contains([]byte(dump), []byte(raw.Full)) {
		t.Fatal("secret tersimpan di tabel")
	}

	dup, _ := mkKey(t, pid, "Claude")
	if _, err := insertOne(t, r, dup); !errors.Is(err, base.ErrNameTaken) {
		t.Fatalf("name taken: %v", err)
	}
	if _, err := insertOne(t, r, func() base.Key { x, _ := mkKey(t, pid, "claude"); return x }()); err != nil {
		t.Fatalf("beda huruf harus boleh: %v", err)
	}
	if _, err := d.ExecContext(context.Background(), `UPDATE api_keys SET revoked_at = now() WHERE name = 'Claude'`); err != nil {
		t.Fatal(err)
	}
	if _, err := insertOne(t, r, dup); err != nil {
		t.Fatalf("nama setelah revoke harus boleh: %v", err)
	}

	same, _ := mkKey(t, pid, "other")
	same.Prefix = got.Prefix
	if _, err := insertOne(t, r, same); !errors.Is(err, base.ErrPrefixCollision) {
		t.Fatalf("prefix: %v", err)
	}
	ghost, _ := mkKey(t, "00000000-0000-4000-8000-000000000000", "ci")
	if _, err := insertOne(t, r, ghost); !errors.Is(err, base.ErrProjectNotFound) {
		t.Fatalf("fk: %v", err)
	}
}

func TestRepo_RollbackOnError(t *testing.T) {
	d, r := setup(t)
	pid := newProject(t, d, "Alpha")
	k, _ := mkKey(t, pid, "ci")
	boom := errors.New("boom")
	err := r.InTx(context.Background(), func(tx Tx) error {
		if _, e := tx.Insert(context.Background(), k); e != nil {
			return e
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatal(err)
	}
	var n int
	_ = d.GetContext(context.Background(), &n, `SELECT count(*) FROM api_keys`)
	if n != 0 {
		t.Fatalf("rollback gagal: %d", n)
	}
}

func execute(uc *Usecase, pid, name string) error {
	ctx := reqctx.WithIdentity(context.Background(), reqctx.Identity{UserID: "00000000-0000-4000-8000-0000000000aa", Email: "a@tim.id", Role: "developer"})
	_, err := uc.Execute(ctx, pid, CreateRequest{Name: name}, base.RequestMeta{})
	return err
}

func TestUsecase_ConcurrentLimit(t *testing.T) {
	d, r := setup(t)
	// created_by diisi aktor non-user; lepas FK bila ada agar fixture sederhana.
	_, _ = d.ExecContext(context.Background(), `ALTER TABLE api_keys DROP CONSTRAINT IF EXISTS api_keys_created_by_fkey`)
	for round := 0; round < 5; round++ {
		pid := newProject(t, d, fmt.Sprintf("P%d", round))
		uc := NewUsecase(r, allowAll{}, nopAuditRecorder{}, base.SystemClock, rand.Reader, 1, "", slog.Default())
		var wg sync.WaitGroup
		var mu sync.Mutex
		ok, limited := 0, 0
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				err := execute(uc, pid, fmt.Sprintf("key-%d", i))
				mu.Lock()
				defer mu.Unlock()
				var e *errs.Error
				switch {
				case err == nil:
					ok++
				case errors.As(err, &e) && e.Details.(map[string]any)["reason"] == base.ReasonKeyLimitReached:
					limited++
				default:
					t.Errorf("err tak terduga: %v", err)
				}
			}(i)
		}
		wg.Wait()
		var n int
		_ = d.GetContext(context.Background(), &n, `SELECT count(*) FROM api_keys WHERE project_id = $1 AND revoked_at IS NULL`, pid)
		if ok != 1 || limited != 7 || n != 1 {
			t.Fatalf("round %d: ok=%d limited=%d aktif=%d", round, ok, limited, n)
		}
	}
}
