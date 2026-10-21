//go:build integration

package touchlastused

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"

	"grimoire/internal/domain/project/internal/apikey/base/store"
	"grimoire/internal/infra/bus"
	"grimoire/internal/infra/db"
	"grimoire/lib/constants"
	"grimoire/lib/contracts/project/apikey/touch"
)

func setup(t *testing.T) (*db.DB, *Repo, string) {
	t.Helper()
	d := db.TestDB(t)
	if _, err := db.Up(context.Background(), d, nil); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	var pid string
	if err := d.GetContext(context.Background(), &pid, `INSERT INTO projects (name) VALUES ('P') RETURNING id::text`); err != nil {
		t.Fatal(err)
	}
	return d, newRepo(store.New(d)), pid
}

func newKey(t *testing.T, d *db.DB, pid, name, prefix string) string {
	t.Helper()
	var id string
	err := d.GetContext(context.Background(), &id,
		`INSERT INTO api_keys (project_id, name, prefix, secret_hash) VALUES ($1,$2,$3, decode(repeat('00',32),'hex')) RETURNING id::text`, pid, name, prefix)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func lastUsed(t *testing.T, d *db.DB, id string) *time.Time {
	t.Helper()
	var ts *time.Time
	if err := d.GetContext(context.Background(), &ts, `SELECT last_used_at FROM api_keys WHERE id = $1`, id); err != nil {
		t.Fatal(err)
	}
	return ts
}

func touchAt(t *testing.T, r *Repo, id string, at time.Time) int64 {
	t.Helper()
	n, err := r.Touch(context.Background(), id, at)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func TestT04_AC7_TouchOncePerMinute(t *testing.T) {
	d, r, pid := setup(t)
	id := newKey(t, d, pid, "k1", "abcd1234")
	t0 := time.Date(2026, 10, 23, 11, 0, 0, 0, time.UTC)
	if lastUsed(t, d, id) != nil {
		t.Fatal("want NULL")
	}
	steps := []struct {
		at   time.Duration
		rows int64
		want time.Duration
	}{
		{0, 1, 0},
		{10 * time.Second, 0, 0},
		{30 * time.Second, 0, 0},
		{59 * time.Second, 0, 0},
		{59900 * time.Millisecond, 0, 0},
		{60 * time.Second, 1, 60 * time.Second},
		{61 * time.Second, 0, 60 * time.Second},
		{121 * time.Second, 1, 121 * time.Second},
	}
	for _, s := range steps {
		if n := touchAt(t, r, id, t0.Add(s.at)); n != s.rows {
			t.Fatalf("at %v: rows=%d want %d", s.at, n, s.rows)
		}
		if got := lastUsed(t, d, id); got == nil || !got.Equal(t0.Add(s.want)) {
			t.Fatalf("at %v: last_used_at=%v want %v", s.at, got, t0.Add(s.want))
		}
	}
}

func TestTouch_ConcurrentSingleWinner(t *testing.T) {
	d, r, pid := setup(t)
	id := newKey(t, d, pid, "k1", "abcd1234")
	now := time.Date(2026, 10, 23, 11, 0, 0, 0, time.UTC)
	var total atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			n, err := r.Touch(context.Background(), id, now)
			if err != nil {
				t.Error(err)
				return
			}
			total.Add(n)
		}()
	}
	wg.Wait()
	if total.Load() != 1 {
		t.Fatalf("total=%d", total.Load())
	}
}

func TestTouch_ClockBackwardsNeverRegresses(t *testing.T) {
	d, r, pid := setup(t)
	id := newKey(t, d, pid, "k1", "abcd1234")
	t0 := time.Date(2026, 10, 23, 11, 0, 0, 0, time.UTC)
	touchAt(t, r, id, t0)
	if n := touchAt(t, r, id, t0.Add(-5*time.Minute)); n != 0 {
		t.Fatalf("rows=%d", n)
	}
	if got := lastUsed(t, d, id); !got.Equal(t0) {
		t.Fatalf("got %v", got)
	}
}

func TestTouch_UnknownKeyIDNoOp(t *testing.T) {
	_, r, _ := setup(t)
	if n := touchAt(t, r, uuid.NewString(), time.Now()); n != 0 {
		t.Fatalf("rows=%d", n)
	}
}

func TestTouch_RevokedKeyStillTouchable(t *testing.T) {
	d, r, pid := setup(t)
	id := newKey(t, d, pid, "k1", "abcd1234")
	if _, err := d.ExecContext(context.Background(), `UPDATE api_keys SET revoked_at = now(), revoked_by = gen_random_uuid() WHERE id = $1`, id); err != nil {
		t.Fatal(err)
	}
	var before struct {
		At *time.Time `db:"revoked_at"`
		By *string    `db:"revoked_by"`
	}
	q := `SELECT revoked_at, revoked_by::text AS revoked_by FROM api_keys WHERE id = $1`
	if err := d.GetContext(context.Background(), &before, q, id); err != nil {
		t.Fatal(err)
	}
	if n := touchAt(t, r, id, time.Now()); n != 1 {
		t.Fatalf("rows=%d", n)
	}
	after := before
	if err := d.GetContext(context.Background(), &after, q, id); err != nil {
		t.Fatal(err)
	}
	if !after.At.Equal(*before.At) || *after.By != *before.By {
		t.Fatalf("revoked fields changed: %+v %+v", before, after)
	}
}

func TestTouch_OnlyTargetRowChanged(t *testing.T) {
	d, r, pid := setup(t)
	a := newKey(t, d, pid, "ka", "abcd1234")
	b := newKey(t, d, pid, "kb", "efgh5678")
	touchAt(t, r, a, time.Now())
	if lastUsed(t, d, b) != nil {
		t.Fatal("key B changed")
	}
}

func busFor(t *testing.T, d *db.DB, buf *bytes.Buffer) *bus.Registry {
	log := slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	b := bus.NewRegistry(log)
	uc := NewUsecase(newRepo(store.New(d)), func() time.Time { return time.Now().UTC() }, log)
	b.Register(constants.BUS_PROJECT_APIKEY_TOUCH, newHandler(uc).Handle)
	return b
}

func TestTouch_NonUUIDNoQuery(t *testing.T) {
	d, _, pid := setup(t)
	id := newKey(t, d, pid, "k1", "abcd1234")
	var buf bytes.Buffer
	b := busFor(t, d, &buf)
	if err := b.Publish(context.Background(), constants.BUS_PROJECT_APIKEY_TOUCH, touch.Request{KeyID: "bukan-uuid"}); err != nil {
		t.Fatal(err)
	}
	_ = b.Shutdown(context.Background())
	if lastUsed(t, d, id) != nil || strings.Contains(buf.String(), "level=WARN") {
		t.Fatal(buf.String())
	}
}

func TestTouch_DBErrorSwallowed(t *testing.T) {
	d, _, _ := setup(t)
	var buf bytes.Buffer
	b := busFor(t, d, &buf)
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}
	if err := b.Publish(context.Background(), constants.BUS_PROJECT_APIKEY_TOUCH, touch.Request{KeyID: uuid.NewString()}); err != nil {
		t.Fatal(err)
	}
	_ = b.Shutdown(context.Background())
	if !strings.Contains(buf.String(), "level=WARN") {
		t.Fatal(buf.String())
	}
}

func TestTouch_NoSecretInLogs(t *testing.T) {
	d, _, pid := setup(t)
	id := newKey(t, d, pid, "k1", "abcd1234")
	var buf bytes.Buffer
	b := busFor(t, d, &buf)
	_ = b.Publish(context.Background(), constants.BUS_PROJECT_APIKEY_TOUCH, touch.Request{KeyID: id})
	_ = b.Publish(context.Background(), constants.BUS_PROJECT_APIKEY_TOUCH, touch.Request{KeyID: "x"})
	_ = b.Shutdown(context.Background())
	if strings.Contains(buf.String(), "gr_") {
		t.Fatal(buf.String())
	}
}
