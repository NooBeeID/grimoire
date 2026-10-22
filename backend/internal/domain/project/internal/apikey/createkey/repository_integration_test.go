//go:build integration

package createkey

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

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

const (
	ownerA = "0192f5a0-aaaa-7aaa-8bbb-00000000000a"
	ownerB = "0192f5a0-aaaa-7aaa-8bbb-00000000000b"
	ownerS = "0192f5a0-aaaa-7aaa-8bbb-00000000005a"
)

// mkKeyFor membuat key milik ownerID (created_by tidak di-nol-kan). created_by tanpa FK ke users.
func mkKeyFor(t *testing.T, pid, name, ownerID, label string) (base.Key, base.RawKey) {
	t.Helper()
	raw, err := base.GenerateRawKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	k, err := base.NewKey(pid, name, raw, base.Actor{ID: ownerID, Label: label}, fixedNow)
	if err != nil {
		t.Fatal(err)
	}
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
	k, raw := mkKeyFor(t, pid, "Claude", ownerA, "b@tim.id")
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

	dup, _ := mkKeyFor(t, pid, "Claude", ownerA, "b@tim.id")
	if _, err := insertOne(t, r, dup); !errors.Is(err, base.ErrNameTaken) {
		t.Fatalf("name taken: %v", err)
	}
	if _, err := insertOne(t, r, func() base.Key { x, _ := mkKeyFor(t, pid, "claude", ownerA, "b@tim.id"); return x }()); err != nil {
		t.Fatalf("beda huruf harus boleh: %v", err)
	}
	if _, err := d.ExecContext(context.Background(), `UPDATE api_keys SET revoked_at = now() WHERE name = 'Claude'`); err != nil {
		t.Fatal(err)
	}
	if _, err := insertOne(t, r, dup); err != nil {
		t.Fatalf("nama setelah revoke harus boleh: %v", err)
	}

	same, _ := mkKeyFor(t, pid, "other", ownerA, "b@tim.id")
	same.Prefix = got.Prefix
	if _, err := insertOne(t, r, same); !errors.Is(err, base.ErrPrefixCollision) {
		t.Fatalf("prefix: %v", err)
	}
	ghost, _ := mkKeyFor(t, "00000000-0000-4000-8000-000000000000", "ci", ownerA, "b@tim.id")
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
	return executeAs(uc, pid, ownerA, "developer", name)
}

func executeAs(uc *Usecase, pid, userID, role, name string) error {
	ctx := reqctx.WithIdentity(context.Background(), reqctx.Identity{UserID: userID, Email: userID + "@tim.id", Role: role})
	_, err := uc.Execute(ctx, pid, CreateRequest{Name: name}, base.RequestMeta{})
	return err
}

func isLimit(err error) bool {
	var e *errs.Error
	if !errors.As(err, &e) {
		return false
	}
	d, _ := e.Details.(map[string]any)
	return d["reason"] == base.ReasonKeyLimitReached
}

func newUC(r *Repo, limit int) *Usecase {
	return NewUsecase(r, allowAll{}, nopAuditRecorder{}, base.SystemClock, rand.Reader, limit, "", slog.Default())
}

func activeCount(t *testing.T, d *db.DB, pid, owner string) int {
	t.Helper()
	var n int
	if err := d.GetContext(context.Background(), &n, `SELECT count(*) FROM api_keys WHERE project_id = $1 AND created_by = $2::uuid AND revoked_at IS NULL`, pid, owner); err != nil {
		t.Fatal(err)
	}
	return n
}

func countFor(t *testing.T, r *Repo, pid, owner string) int {
	t.Helper()
	var n int
	err := r.InTx(context.Background(), func(tx Tx) error {
		var e error
		n, e = tx.CountActive(context.Background(), pid, owner)
		return e
	})
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func TestRepo_CountActive_PerOwner(t *testing.T) {
	d, r := setup(t)
	pid := newProject(t, d, "Alpha")
	for i, o := range []string{ownerA, ownerA, ownerA, ownerB} {
		k, _ := mkKeyFor(t, pid, fmt.Sprintf("k%d", i), o, "x@tim.id")
		if _, err := insertOne(t, r, k); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := d.ExecContext(context.Background(), `UPDATE api_keys SET revoked_at = now() WHERE name = 'k2'`); err != nil {
		t.Fatal(err)
	}
	legacy, _ := mkKey(t, pid, "legacy")
	if _, err := insertOne(t, r, legacy); err != nil {
		t.Fatal(err)
	}
	if a, b, s := countFor(t, r, pid, ownerA), countFor(t, r, pid, ownerB), countFor(t, r, pid, ownerS); a != 2 || b != 1 || s != 0 {
		t.Fatalf("a=%d b=%d s=%d", a, b, s)
	}
}

func TestRepo_CountActive_ScopedToProject(t *testing.T) {
	d, r := setup(t)
	p1, p2 := newProject(t, d, "P1"), newProject(t, d, "P2")
	for _, c := range []struct{ pid, name string }{{p1, "aa"}, {p1, "bb"}, {p2, "aa"}} {
		k, _ := mkKeyFor(t, c.pid, c.name, ownerA, "a@tim.id")
		if _, err := insertOne(t, r, k); err != nil {
			t.Fatal(err)
		}
	}
	if countFor(t, r, p1, ownerA) != 2 || countFor(t, r, p2, ownerA) != 1 {
		t.Fatal("hitungan per project harus terpisah")
	}
}

func TestRepo_InsertSameNameDifferentOwners(t *testing.T) {
	d, r := setup(t)
	pid := newProject(t, d, "Alpha")
	for _, o := range []string{ownerA, ownerB} {
		k, _ := mkKeyFor(t, pid, "ci", o, "x@tim.id")
		if _, err := insertOne(t, r, k); err != nil {
			t.Fatalf("%s: %v", o, err)
		}
	}
}

func TestRepo_InsertSameNameSameOwner_NameTaken(t *testing.T) {
	d, r := setup(t)
	pid := newProject(t, d, "Alpha")
	k, _ := mkKeyFor(t, pid, "ci", ownerA, "x@tim.id")
	if _, err := insertOne(t, r, k); err != nil {
		t.Fatal(err)
	}
	k2, _ := mkKeyFor(t, pid, "ci", ownerA, "x@tim.id")
	if _, err := insertOne(t, r, k2); !errors.Is(err, base.ErrNameTaken) {
		t.Fatalf("err=%v", err)
	}
}

func TestRepo_NameReusableAfterRevoke(t *testing.T) {
	d, r := setup(t)
	pid := newProject(t, d, "Alpha")
	k, _ := mkKeyFor(t, pid, "ci", ownerA, "x@tim.id")
	if _, err := insertOne(t, r, k); err != nil {
		t.Fatal(err)
	}
	if _, err := d.ExecContext(context.Background(), `UPDATE api_keys SET revoked_at = now() WHERE name = 'ci'`); err != nil {
		t.Fatal(err)
	}
	k2, _ := mkKeyFor(t, pid, "ci", ownerA, "x@tim.id")
	if _, err := insertOne(t, r, k2); err != nil {
		t.Fatal(err)
	}
}

func TestRepo_LockOwnerKeys_Independent(t *testing.T) {
	d, r := setup(t)
	pid := newProject(t, d, "Alpha")
	ctx := context.Background()

	held := make(chan struct{})
	release := make(chan struct{})
	tx1Done := make(chan error, 1)
	go func() {
		tx1Done <- r.InTx(ctx, func(tx Tx) error {
			if err := tx.LockOwnerKeys(ctx, pid, ownerA); err != nil {
				return err
			}
			close(held)
			<-release
			return nil
		})
	}()
	<-held

	// B tidak menunggu A.
	bDone := make(chan error, 1)
	go func() {
		bDone <- r.InTx(ctx, func(tx Tx) error { return tx.LockOwnerKeys(ctx, pid, ownerB) })
	}()
	select {
	case err := <-bDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("lock B menunggu lock A")
	}

	// A kedua menunggu sampai tx1 selesai.
	a2Done := make(chan error, 1)
	go func() {
		a2Done <- r.InTx(ctx, func(tx Tx) error { return tx.LockOwnerKeys(ctx, pid, ownerA) })
	}()
	select {
	case <-a2Done:
		t.Fatal("lock A kedua tidak boleh lolos selama tx1 menahan")
	case <-time.After(300 * time.Millisecond):
	}
	close(release)
	if err := <-tx1Done; err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-a2Done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("lock A kedua tidak lepas setelah tx1 commit")
	}
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

func TestUsecase_ConcurrentLimit_SameOwnerTwice(t *testing.T) {
	d, r := setup(t)
	uc := newUC(r, 1)
	for round := 0; round < 10; round++ {
		pid := newProject(t, d, fmt.Sprintf("P%d", round))
		errsC := make(chan error, 2)
		var wg sync.WaitGroup
		for i := 0; i < 2; i++ {
			wg.Add(1)
			go func(i int) { defer wg.Done(); errsC <- execute(uc, pid, fmt.Sprintf("k%d", i)) }(i)
		}
		wg.Wait()
		close(errsC)
		ok, limited := 0, 0
		for err := range errsC {
			switch {
			case err == nil:
				ok++
			case isLimit(err):
				limited++
			default:
				t.Fatalf("err tak terduga: %v", err)
			}
		}
		if ok != 1 || limited != 1 || activeCount(t, d, pid, ownerA) != 1 {
			t.Fatalf("round %d: ok=%d limited=%d", round, ok, limited)
		}
	}
}

func TestUsecase_ConcurrentDifferentOwners_BothSucceed(t *testing.T) {
	d, r := setup(t)
	uc := newUC(r, 1)
	for round := 0; round < 10; round++ {
		pid := newProject(t, d, fmt.Sprintf("P%d", round))
		nameB := "nb"
		if round%2 == 0 {
			nameB = "same" // nama sama antar user harus tetap boleh
		}
		nameA := "na"
		if round%2 == 0 {
			nameA = "same"
		}
		results := make(chan error, 2)
		var wg sync.WaitGroup
		wg.Add(2)
		go func() { defer wg.Done(); results <- executeAs(uc, pid, ownerA, "developer", nameA) }()
		go func() { defer wg.Done(); results <- executeAs(uc, pid, ownerB, "developer", nameB) }()
		wg.Wait()
		close(results)
		for err := range results {
			if err != nil {
				t.Fatalf("round %d: %v", round, err)
			}
		}
		if activeCount(t, d, pid, ownerA) != 1 || activeCount(t, d, pid, ownerB) != 1 {
			t.Fatalf("round %d: aktif tidak 1+1", round)
		}
	}
}

func TestUsecase_LimitPerOwner(t *testing.T) {
	d, r := setup(t)
	pid := newProject(t, d, "Alpha")
	uc := newUC(r, 1)
	if err := executeAs(uc, pid, ownerA, "developer", "a1"); err != nil {
		t.Fatal(err)
	}
	err := executeAs(uc, pid, ownerA, "developer", "a2")
	var e *errs.Error
	if !errors.As(err, &e) || !isLimit(err) || e.Details.(map[string]any)["limit"] != 1 {
		t.Fatalf("err=%v", err)
	}
	if err := executeAs(uc, pid, ownerB, "developer", "b1"); err != nil {
		t.Fatalf("B tidak boleh terpengaruh kuota A: %v", err)
	}
}

func TestUsecase_SuperAdminQuotaSeparate(t *testing.T) {
	d, r := setup(t)
	pid := newProject(t, d, "Alpha")
	uc := newUC(r, 1)
	if err := executeAs(uc, pid, ownerA, "developer", "a1"); err != nil {
		t.Fatal(err)
	}
	if err := executeAs(uc, pid, ownerS, "super_admin", "s1"); err != nil {
		t.Fatalf("super admin pertama harus sukses: %v", err)
	}
	if err := executeAs(uc, pid, ownerA, "developer", "a2"); !isLimit(err) {
		t.Fatalf("A kedua harus limit: %v", err)
	}
	if err := executeAs(uc, pid, ownerS, "super_admin", "s2"); !isLimit(err) {
		t.Fatalf("super admin kedua harus limit: %v", err)
	}
	var by string
	if err := d.GetContext(context.Background(), &by, `SELECT created_by::text FROM api_keys WHERE name = 's1'`); err != nil || by != ownerS {
		t.Fatalf("created_by=%q err=%v", by, err)
	}
	// A mencabut lalu bisa buat lagi; kuota S tidak berubah.
	if _, err := d.ExecContext(context.Background(), `UPDATE api_keys SET revoked_at = now() WHERE name = 'a1'`); err != nil {
		t.Fatal(err)
	}
	if err := executeAs(uc, pid, ownerA, "developer", "a2"); err != nil {
		t.Fatalf("A setelah revoke: %v", err)
	}
	if activeCount(t, d, pid, ownerS) != 1 {
		t.Fatal("kuota S berubah")
	}
}

// TestMigration_OwnerScope_OnT04Data membuktikan migrasi owner-scope aman pada data T04.
func TestMigration_OwnerScope_OnT04Data(t *testing.T) {
	d, r := setup(t)
	ctx := context.Background()
	// Pulihkan keadaan T04.
	for _, q := range []string{
		`DROP INDEX IF EXISTS api_keys_project_active_name_key`,
		`CREATE UNIQUE INDEX api_keys_project_active_name_key ON api_keys (project_id, name) WHERE revoked_at IS NULL`,
		`DROP INDEX IF EXISTS api_keys_owner_idx`,
	} {
		if _, err := d.ExecContext(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	pid := newProject(t, d, "Alpha")
	rows := []struct {
		name  string
		owner string
		rev   bool
	}{{"ka", ownerA, false}, {"kb", ownerB, false}, {"legacy", "", false}, {"ka", ownerA, true}}
	for _, row := range rows {
		var k base.Key
		if row.owner == "" {
			k, _ = mkKey(t, pid, row.name)
		} else {
			k, _ = mkKeyFor(t, pid, row.name, row.owner, "x@tim.id")
		}
		if row.rev {
			// key dicabut bernama sama dengan key aktif: disisipkan langsung agar lolos index T04.
			if _, err := d.ExecContext(ctx, `INSERT INTO api_keys (project_id, name, prefix, secret_hash, created_by, created_by_label, created_at, revoked_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, now())`, k.ProjectID, k.Name, k.Prefix, k.SecretHash, k.CreatedBy, k.CreatedByLabel, k.CreatedAt); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if _, err := insertOne(t, r, k); err != nil {
			t.Fatal(err)
		}
	}
	files, err := filepath.Glob("../../../../../../migrations/project/*_api_keys_owner_scope.sql")
	if err != nil || len(files) != 1 {
		t.Fatalf("glob migrasi: %v %v", files, err)
	}
	sqlBytes, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ { // sekali + idempoten
		if _, err := d.ExecContext(ctx, string(sqlBytes)); err != nil {
			t.Fatalf("jalan ke-%d: %v", i+1, err)
		}
		var n int
		if err := d.GetContext(ctx, &n, `SELECT count(*) FROM api_keys`); err != nil || n != 4 {
			t.Fatalf("jumlah baris=%d err=%v", n, err)
		}
	}
	var def string
	if err := d.GetContext(ctx, &def, `SELECT indexdef FROM pg_indexes WHERE schemaname = current_schema() AND indexname = 'api_keys_project_active_name_key'`); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(def, "(project_id, created_by, name)") || !strings.Contains(def, "revoked_at IS NULL") {
		t.Fatalf("indexdef: %s", def)
	}
	var cnt int
	if err := d.GetContext(ctx, &cnt, `SELECT count(*) FROM pg_indexes WHERE schemaname = current_schema() AND indexname = 'api_keys_owner_idx'`); err != nil || cnt != 1 {
		t.Fatalf("api_keys_owner_idx hilang: %d %v", cnt, err)
	}
	// Setelah migrasi: nama sama beda user OK, user sama -> NAME_TAKEN.
	p2 := newProject(t, d, "Beta")
	for _, o := range []string{ownerA, ownerB} {
		k, _ := mkKeyFor(t, p2, "shared", o, "x@tim.id")
		if _, err := insertOne(t, r, k); err != nil {
			t.Fatalf("%s: %v", o, err)
		}
	}
	dup, _ := mkKeyFor(t, p2, "shared", ownerA, "x@tim.id")
	err = r.InTx(ctx, func(tx Tx) error {
		_, e := tx.(*Repo).s.Querier().ExecContext(ctx, `INSERT INTO api_keys (project_id, name, prefix, secret_hash, created_by, created_by_label, created_at) VALUES ($1,$2,$3,$4,$5,$6,$7)`,
			dup.ProjectID, dup.Name, dup.Prefix, dup.SecretHash, dup.CreatedBy, dup.CreatedByLabel, dup.CreatedAt)
		return e
	})
	if code, constraint, ok := store.Violation(err); !ok || code != store.CodeUniqueViolation || constraint != store.ConstraintActiveName {
		t.Fatalf("violation: %v", err)
	}
	if _, err := insertOne(t, r, dup); !errors.Is(err, base.ErrNameTaken) {
		t.Fatalf("insert: %v", err)
	}
}
