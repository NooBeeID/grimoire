//go:build integration

package purgeexpired

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"grimoire/internal/infra/db"
)

func setupDB(t *testing.T) *db.DB {
	t.Helper()
	d := db.TestDB(t)
	if _, err := db.Up(context.Background(), d, slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))); err != nil {
		t.Fatal(err)
	}
	return d
}

func seed(t *testing.T, d *db.DB, at time.Time, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		id, err := uuid.NewV7()
		if err != nil {
			t.Fatal(err)
		}
		_, err = d.Exec(`INSERT INTO audit_log (id, occurred_at, actor_type, actor_label, action, status)
			VALUES ($1, $2, 'system', 'system', 'x.y', 'ok')`, id, at.Add(time.Duration(i)*time.Millisecond))
		if err != nil {
			t.Fatal(err)
		}
	}
}

func count(t *testing.T, d *db.DB, where string) int {
	t.Helper()
	var n int
	if err := d.Get(&n, "SELECT count(*) FROM audit_log "+where); err != nil {
		t.Fatal(err)
	}
	return n
}

func day(n int) time.Time { return time.Now().Add(-time.Duration(n) * 24 * time.Hour) }

func TestT02_AC6_PurgeRemovesOldKeepsNew(t *testing.T) {
	d := setupDB(t)
	seed(t, d, day(91), 10)
	seed(t, d, day(89), 3)
	seed(t, d, time.Now(), 2)
	var buf bytes.Buffer
	uc := New(NewRepository(d), Options{Retention: 90, BatchSize: 3}, time.Now, slog.New(slog.NewTextHandler(&buf, nil)))
	if err := uc.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if n := count(t, d, ""); n != 5 {
		t.Fatalf("remaining=%d", n)
	}
	if n := count(t, d, "WHERE occurred_at < now() - interval '90 days'"); n != 0 {
		t.Fatalf("old=%d", n)
	}
	if !strings.Contains(buf.String(), "batches=4") || !strings.Contains(buf.String(), "total_deleted=10") {
		t.Fatal(buf.String())
	}
}

func TestT02_AC6_ExactMultipleOfBatch(t *testing.T) {
	d := setupDB(t)
	seed(t, d, day(91), 6)
	var buf bytes.Buffer
	uc := New(NewRepository(d), Options{Retention: 90, BatchSize: 3}, time.Now, slog.New(slog.NewTextHandler(&buf, nil)))
	if err := uc.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if count(t, d, "") != 0 || !strings.Contains(buf.String(), "batches=3") {
		t.Fatal(buf.String())
	}
}

func TestT02_AC6_BoundaryExclusive(t *testing.T) {
	d := setupDB(t)
	now := time.Date(2026, 10, 20, 3, 15, 0, 0, time.UTC)
	cutoff := now.Add(-90 * 24 * time.Hour)
	seed(t, d, cutoff.Add(-time.Second), 1)
	seed(t, d, cutoff, 1)
	seed(t, d, cutoff.Add(time.Second), 1)
	uc := New(NewRepository(d), Options{Retention: 90, BatchSize: 10}, func() time.Time { return now }, slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)))
	if err := uc.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if n := count(t, d, ""); n != 2 {
		t.Fatalf("remaining=%d", n)
	}
	if n := count(t, d, "WHERE occurred_at < '"+cutoff.Format(time.RFC3339)+"'"); n != 0 {
		t.Fatalf("before cutoff=%d", n)
	}
}

func TestDeleteBatch_RespectsLimitAndOrder(t *testing.T) {
	d := setupDB(t)
	base := day(100)
	for i := 0; i < 5; i++ {
		seed(t, d, base.Add(time.Duration(i)*time.Hour), 1)
	}
	n, err := NewRepository(d).DeleteBatch(context.Background(), day(90), 2)
	if err != nil || n != 2 {
		t.Fatal(n, err)
	}
	var oldest time.Time
	if err := d.Get(&oldest, "SELECT min(occurred_at) FROM audit_log"); err != nil {
		t.Fatal(err)
	}
	if want := base.Add(2 * time.Hour); oldest.Sub(want).Abs() > time.Millisecond {
		t.Fatalf("oldest=%v want %v", oldest, want)
	}
}

func TestDeleteBatch_NothingToDelete(t *testing.T) {
	d := setupDB(t)
	r := NewRepository(d)
	if n, err := r.DeleteBatch(context.Background(), day(90), 10); err != nil || n != 0 {
		t.Fatal(n, err)
	}
	seed(t, d, time.Now(), 2)
	if n, err := r.DeleteBatch(context.Background(), day(90), 10); err != nil || n != 0 {
		t.Fatal(n, err)
	}
}

func TestDeleteBatch_NotBlockedByAppendOnlyTrigger(t *testing.T) {
	d := setupDB(t)
	seed(t, d, day(200), 1)
	if _, err := d.Exec("UPDATE " + "audit_log SET action = 'z'"); err == nil {
		t.Fatal("update should be blocked")
	}
	if n, err := NewRepository(d).DeleteBatch(context.Background(), day(90), 10); err != nil || n != 1 {
		t.Fatal(n, err)
	}
}

func TestRun_DBError(t *testing.T) {
	d := setupDB(t)
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	uc := New(NewRepository(d), Options{Retention: 90}, time.Now, slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)))
	if err := uc.Run(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	_ = d.Close()
	if err := uc.Run(context.Background()); err == nil {
		t.Fatal("expected error on closed db")
	}
}
