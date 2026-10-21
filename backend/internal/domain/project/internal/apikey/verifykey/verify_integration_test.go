//go:build integration

package verifykey

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"grimoire/internal/domain/project/internal/apikey/base"
	"grimoire/internal/domain/project/internal/apikey/base/store"
	"grimoire/internal/infra/app"
	"grimoire/internal/infra/bus"
	"grimoire/internal/infra/db"
	"grimoire/lib/constants"
	"grimoire/lib/contracts/project/apikey/verify"
)

// countingQ membungkus Querier dan mencatat SQL yang dijalankan.
type countingQ struct {
	db.Querier
	mu   sync.Mutex
	sqls []string
	fail error
}

func (c *countingQ) record(q string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sqls = append(c.sqls, q)
	return c.fail
}

func (c *countingQ) GetContext(ctx context.Context, dest any, q string, args ...any) error {
	if err := c.record(q); err != nil {
		return err
	}
	return c.Querier.GetContext(ctx, dest, q, args...)
}

func (c *countingQ) SelectContext(ctx context.Context, dest any, q string, args ...any) error {
	if err := c.record(q); err != nil {
		return err
	}
	return c.Querier.SelectContext(ctx, dest, q, args...)
}

func (c *countingQ) QueryxContext(ctx context.Context, q string, args ...any) (*sqlx.Rows, error) {
	if err := c.record(q); err != nil {
		return nil, err
	}
	return c.Querier.QueryxContext(ctx, q, args...)
}

func (c *countingQ) QueryRowxContext(ctx context.Context, q string, args ...any) *sqlx.Row {
	_ = c.record(q)
	return c.Querier.QueryRowxContext(ctx, q, args...)
}

func (c *countingQ) count() int { c.mu.Lock(); defer c.mu.Unlock(); return len(c.sqls) }

func callVerify(t testing.TB, r *bus.Registry, raw string) (verify.Response, error) {
	t.Helper()
	return bus.CallTyped[verify.Response](context.Background(), r, constants.BUS_PROJECT_APIKEY_VERIFY, verify.Request{RawKey: raw})
}

func realBus(t testing.TB, d *db.DB, log *slog.Logger) *bus.Registry {
	t.Helper()
	r := bus.NewRegistry(log)
	Init(app.Deps{Bus: r, DB: d})
	return r
}

func TestT04_AC3_NewKeyValidThenRevoked(t *testing.T) {
	d, pid := setupDB(t)
	raw := newRaw(t)
	id := insertKey(t, d, pid, "claude-code", raw, nil, "")
	r := realBus(t, d, nil)
	res, err := callVerify(t, r, raw.Full)
	want := verify.Response{Found: true, KeyID: id, KeyName: "claude-code", Prefix: raw.Prefix, ProjectID: pid}
	if err != nil || res != want {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	if _, err := d.Exec(`UPDATE api_keys SET revoked_at = now() WHERE id = $1`, id); err != nil {
		t.Fatal(err)
	}
	res, err = callVerify(t, r, raw.Full)
	if err != nil || res.Found || res.Reason != verify.ReasonRevoked || res.Prefix != raw.Prefix {
		t.Fatalf("res=%+v err=%v", res, err)
	}
}

func TestT04_AC6_CreatorRemovedOrDeactivatedKeyStillValid(t *testing.T) {
	d, pid := setupDB(t)
	ghost := uuid.NewString()
	a, b := newRaw(t), newRaw(t)
	insertKey(t, d, pid, "with-ghost", a, &ghost, "budi@tim.id")
	insertKey(t, d, pid, "no-creator", b, nil, "")
	cq := &countingQ{Querier: d}
	r := bus.NewRegistry(nil)
	r.Register(constants.BUS_PROJECT_APIKEY_VERIFY, newHandler(NewUsecase(newRepo(store.New(cq)))).Handle)
	for _, raw := range []base.RawKey{a, b} {
		res, err := callVerify(t, r, raw.Full)
		if err != nil || !res.Found {
			t.Fatalf("res=%+v err=%v", res, err)
		}
	}
	for _, q := range cq.sqls {
		if strings.Contains(strings.ToUpper(q), "JOIN") || strings.Contains(q, "users") || strings.Contains(q, "project_members") {
			t.Fatalf("query menyentuh tabel lain: %s", q)
		}
	}
}

func TestVerify_ExactlyOneQuery(t *testing.T) {
	d, pid := setupDB(t)
	raw, other := newRaw(t), newRaw(t)
	insertKey(t, d, pid, "ci", raw, nil, "")
	revoked := newRaw(t)
	insertKey(t, d, pid, "old", revoked, nil, "")
	if _, err := d.Exec(`UPDATE api_keys SET revoked_at = now() WHERE prefix = $1`, revoked.Prefix); err != nil {
		t.Fatal(err)
	}
	cq := &countingQ{Querier: d}
	r := bus.NewRegistry(nil)
	r.Register(constants.BUS_PROJECT_APIKEY_VERIFY, newHandler(NewUsecase(newRepo(store.New(cq)))).Handle)
	inputs := []string{raw.Full, "gr_zzzzzzzz_" + other.Secret, "gr_" + raw.Prefix + "_" + other.Secret, revoked.Full}
	for _, in := range inputs {
		before := cq.count()
		if _, err := callVerify(t, r, in); err != nil {
			t.Fatal(err)
		}
		if got := cq.count() - before; got != 1 {
			t.Fatalf("queries=%d for %q", got, in)
		}
	}
	for _, q := range cq.sqls {
		if strings.Contains(strings.ToUpper(q), "JOIN") || strings.Count(q, "FROM") != 1 || !strings.Contains(q, "FROM api_keys WHERE prefix = $1") {
			t.Fatalf("sql: %s", q)
		}
	}
	before := cq.count()
	if res, _ := callVerify(t, r, "junk"); res.Reason != verify.ReasonMalformed || cq.count() != before {
		t.Fatalf("malformed hit DB: %+v", res)
	}
}

func TestVerify_TwoKeysSamePrefixImpossible(t *testing.T) {
	d, pid := setupDB(t)
	raw := newRaw(t)
	insertKey(t, d, pid, "a1", raw, nil, "")
	_, err := d.Exec(`INSERT INTO api_keys (project_id, name, prefix, secret_hash) VALUES ($1,'b2',$2,$3)`, pid, raw.Prefix, base.HashSecret("x"))
	code, c, ok := store.Violation(err)
	if !ok || code != store.CodeUniqueViolation || c != store.ConstraintPrefixUnique {
		t.Fatalf("code=%q c=%q ok=%v err=%v", code, c, ok, err)
	}
}

func TestVerify_PrefixKnownSecretOfOtherKey(t *testing.T) {
	d, pid := setupDB(t)
	a, b := newRaw(t), newRaw(t)
	insertKey(t, d, pid, "aa", a, nil, "")
	insertKey(t, d, pid, "bb", b, nil, "")
	res, err := callVerify(t, realBus(t, d, nil), "gr_"+a.Prefix+"_"+b.Secret)
	if err != nil || res.Found || res.Reason != verify.ReasonMismatch {
		t.Fatalf("res=%+v err=%v", res, err)
	}
}

func TestVerify_CascadeOnProjectDelete(t *testing.T) {
	d, pid := setupDB(t)
	raw := newRaw(t)
	insertKey(t, d, pid, "ci", raw, nil, "")
	r := realBus(t, d, nil)
	if res, _ := callVerify(t, r, raw.Full); !res.Found {
		t.Fatalf("res=%+v", res)
	}
	if _, err := d.Exec(`DELETE FROM projects WHERE id = $1`, pid); err != nil {
		t.Fatal(err)
	}
	res, err := callVerify(t, r, raw.Full)
	if err != nil || res.Found || res.Reason != verify.ReasonUnknown {
		t.Fatalf("res=%+v err=%v", res, err)
	}
}

func TestVerify_FailClosedOnDBError(t *testing.T) {
	d, pid := setupDB(t)
	raw := newRaw(t)
	insertKey(t, d, pid, "ci", raw, nil, "")
	cq := &countingQ{Querier: d, fail: context.DeadlineExceeded}
	r := bus.NewRegistry(nil)
	r.Register(constants.BUS_PROJECT_APIKEY_VERIFY, newHandler(NewUsecase(newRepo(store.New(cq)))).Handle)
	res, err := callVerify(t, r, raw.Full)
	if err == nil || res.Found {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	_ = d.Close()
	res, err = callVerify(t, realBus(t, d, nil), raw.Full)
	if err == nil || res.Found {
		t.Fatalf("closed pool: res=%+v err=%v", res, err)
	}
}

func TestVerify_NoSecretInLogs(t *testing.T) {
	d, pid := setupDB(t)
	raw, other := newRaw(t), newRaw(t)
	insertKey(t, d, pid, "ci", raw, nil, "")
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	r := realBus(t, d, log)
	for _, in := range []string{raw.Full, "gr_" + raw.Prefix + "_" + other.Secret, "gr_zzzzzzzz_" + other.Secret, "junk"} {
		_, _ = callVerify(t, r, in)
	}
	_ = d.Close()
	_, _ = callVerify(t, r, raw.Full)
	out := buf.String()
	if strings.Contains(out, raw.Full) || strings.Contains(out, raw.Secret) || strings.Contains(out, other.Secret) {
		t.Fatalf("log leaks secret: %s", out)
	}
}

// TestT04_AC8_DBPathUnknownVsMismatchComparable: jaminan utama adalah struktural
// (TestT04_AC8_StructuralHashBeforeStatusCheck + tes VerifyOutcome di base/); tes ini
// hanya sabuk pengaman terhadap kebocoran besar.
func TestT04_AC8_DBPathUnknownVsMismatchComparable(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode")
	}
	d, pid := setupDB(t)
	raw, other := newRaw(t), newRaw(t)
	insertKey(t, d, pid, "ci", raw, nil, "")
	r := realBus(t, d, nil)
	a := "gr_zzzzzzzz_" + other.Secret
	b := "gr_" + raw.Prefix + "_" + other.Secret
	ctx := context.Background()

	attempt := func() (time.Duration, time.Duration, bool) {
		const N, warm = 600, 50
		var da, db2 []time.Duration
		for i := 0; i < N; i++ {
			t0 := time.Now()
			_, _ = r.Call(ctx, constants.BUS_PROJECT_APIKEY_VERIFY, verify.Request{RawKey: a})
			x := time.Since(t0)
			t0 = time.Now()
			_, _ = r.Call(ctx, constants.BUS_PROJECT_APIKEY_VERIFY, verify.Request{RawKey: b})
			y := time.Since(t0)
			if i >= warm {
				da, db2 = append(da, x), append(db2, y)
			}
		}
		ma, mb := median(da), median(db2)
		ratio := float64(ma) / float64(mb)
		diff := ma - mb
		if diff < 0 {
			diff = -diff
		}
		t.Logf("median unknown=%v mismatch=%v ratio=%.2f", ma, mb, ratio)
		return ma, mb, (ratio >= 0.6 && ratio <= 1.7) || diff < 300*time.Microsecond
	}
	if _, _, ok := attempt(); ok {
		return
	}
	ma, mb, ok := attempt()
	if !ok {
		t.Fatalf("timing berbeda setelah 2 percobaan: unknown=%v mismatch=%v", ma, mb)
	}
}

// TestVerify_MalformedFastPathDocumented: jalur malformed memang lebih cepat dari jalur DB.
// Format key publik (K14) sehingga ini tidak membocorkan rahasia. Tanpa assert waktu.
func TestVerify_MalformedFastPathDocumented(t *testing.T) {
	d, _ := setupDB(t)
	res, err := callVerify(t, realBus(t, d, nil), "junk")
	if err != nil || res.Reason != verify.ReasonMalformed {
		t.Fatalf("res=%+v err=%v", res, err)
	}
}
