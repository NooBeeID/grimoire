//go:build integration

package jobs

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"
)

func testDB(t *testing.T) *sqlx.DB {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		if os.Getenv("CI") == "true" {
			t.Fatal("TEST_DATABASE_URL is required in CI")
		}
		t.Skip("TEST_DATABASE_URL not set")
	}
	d, err := sqlx.Open("pgx", url)
	if err != nil {
		t.Fatal(err)
	}
	d.SetMaxOpenConns(20)
	t.Cleanup(func() { d.Close() })
	if err := d.Ping(); err != nil {
		t.Fatal(err)
	}
	return d
}

func uniqueName() string { return "it-" + uuid.NewString()[:8] }

// otherSessionTryLock mencoba lock dari koneksi lain lalu melepasnya bila berhasil.
func otherSessionTryLock(t *testing.T, d *sqlx.DB, name string) bool {
	t.Helper()
	ctx := context.Background()
	c, err := d.Connx(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	var got bool
	if err := c.GetContext(ctx, &got, "SELECT pg_try_advisory_lock(hashtext('grimoire.job.'||$1))", name); err != nil {
		t.Fatal(err)
	}
	if got {
		var rel bool
		_ = c.GetContext(ctx, &rel, "SELECT pg_advisory_unlock(hashtext('grimoire.job.'||$1))", name)
	}
	return got
}

func TestTwoSchedulers_OnlyOneRunsPerTick(t *testing.T) {
	d := testDB(t)
	name := uniqueName()
	const every = 500 * time.Millisecond
	var mu sync.Mutex
	var stamps []time.Time
	run := func(ctx context.Context) error {
		mu.Lock()
		stamps = append(stamps, time.Now())
		mu.Unlock()
		time.Sleep(200 * time.Millisecond)
		return nil
	}
	log, buf := newLogger()
	s1, s2 := New(d, log), New(d, log)
	s1.Register(Job{Name: name, Every: every, Run: run})
	s2.Register(Job{Name: name, Every: every, Run: run})
	s1.Start(context.Background())
	s2.Start(context.Background())
	time.Sleep(2300 * time.Millisecond)
	stopOK(t, s1)
	stopOK(t, s2)

	mu.Lock()
	defer mu.Unlock()
	if len(stamps) < 3 {
		t.Fatalf("runs=%d", len(stamps))
	}
	buckets := map[int64]int{}
	for _, ts := range stamps {
		buckets[ts.UnixNano()/int64(every)]++
	}
	for b, n := range buckets {
		if n != 1 {
			t.Fatalf("bucket %d ran %d times", b, n)
		}
	}
	if !strings.Contains(buf.String(), "status=skipped") {
		t.Fatalf("expected skipped in log:\n%s", buf)
	}
}

func TestLockHeldWhileRunning_ReleasedAfter(t *testing.T) {
	d := testDB(t)
	name := uniqueName()
	log, _ := newLogger()
	s := New(d, log)
	started, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	s.Register(Job{Name: name, Every: 50 * time.Millisecond, Run: func(context.Context) error {
		first := false
		once.Do(func() { first = true })
		if first {
			close(started)
			<-release
		}
		return nil
	}})
	s.Start(context.Background())
	<-started
	if otherSessionTryLock(t, d, name) {
		t.Fatal("lock must be held while job runs")
	}
	close(release)
	stopOK(t, s)
	if !otherSessionTryLock(t, d, name) {
		t.Fatal("lock leaked after Stop")
	}
}

func TestLockReleasedAfterPanic(t *testing.T) {
	d := testDB(t)
	name := uniqueName()
	log, _ := newLogger()
	s := New(d, log)
	ran := make(chan struct{}, 1)
	s.Register(Job{Name: name, Every: 30 * time.Millisecond, Run: func(context.Context) error {
		select {
		case ran <- struct{}{}:
		default:
		}
		panic("boom")
	}})
	s.Start(context.Background())
	<-ran
	stopOK(t, s)
	if !otherSessionTryLock(t, d, name) {
		t.Fatal("lock leaked after panic")
	}
}

func TestConnectionReturnedToPool(t *testing.T) {
	d := testDB(t)
	name := uniqueName()
	log, _ := newLogger()
	s := New(d, log)
	var mu sync.Mutex
	n := 0
	s.Register(Job{Name: name, Every: 20 * time.Millisecond, Run: func(context.Context) error {
		mu.Lock()
		n++
		mu.Unlock()
		return nil
	}})
	s.Start(context.Background())
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		c := n
		mu.Unlock()
		if c >= 5 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	stopOK(t, s)
	st := d.Stats()
	if st.InUse != 0 {
		t.Fatalf("InUse=%d", st.InUse)
	}
	if st.OpenConnections > 3 {
		t.Fatalf("OpenConnections=%d", st.OpenConnections)
	}
}

func TestPgLocker_UnlockFailureDiscardsConn(t *testing.T) {
	d := testDB(t)
	name := uniqueName()
	l := pgLocker{db: d}
	unlock, ok, err := l.TryLock(context.Background(), name)
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := unlock(ctx); err == nil {
		t.Fatal("expected unlock error")
	}
	// koneksi dibuang: lock sesi hilang saat koneksi fisik ditutup
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if otherSessionTryLock(t, d, name) {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal(fmt.Sprint("lock still held after discard"))
}
