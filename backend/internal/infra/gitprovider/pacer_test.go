package gitprovider

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeClock: jam palsu; sleep memajukan jam tanpa tidur nyata.
type fakeClock struct {
	mu     sync.Mutex
	t      time.Time
	sleeps []time.Duration
	starts []time.Time // jam saat tiap sleep selesai (awal request)
}

func newFakeClock() *fakeClock { return &fakeClock{t: time.Unix(1_800_000_000, 0)} }

func (f *fakeClock) now() time.Time { f.mu.Lock(); defer f.mu.Unlock(); return f.t }

func (f *fakeClock) sleep(ctx context.Context, d time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sleeps = append(f.sleeps, d)
	f.t = f.t.Add(d)
	return nil
}

func (f *fakeClock) positive() (n int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, d := range f.sleeps {
		if d > 0 {
			n++
		}
	}
	return
}

func paced(t *testing.T, o PaceOptions, h http.HandlerFunc, gitlab bool) (*Client, *fakeClock, *rec, string) {
	t.Helper()
	c, _, r, host := newClient(t, h, gitlab)
	fc := newFakeClock()
	c.pace = o.withDefaults()
	c.now, c.sleep = fc.now, fc.sleep
	return c, fc, r, host
}

var ghRepo = Repo{Provider: GitHub, Host: "github.com", Path: "o/r", Branch: "main"}

func okBlob(w http.ResponseWriter) { w.WriteHeader(200); fmt.Fprint(w, "x") }

func TestSession_NoWaitWhenRoomy(t *testing.T) {
	c, fc, _, _ := paced(t, PaceOptions{MaxWait: 2 * time.Minute, MaxTotalWait: 5 * time.Minute}, func(w http.ResponseWriter, q *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "4999")
		okBlob(w)
	}, false)
	var waits int
	s := c.Session(func(Wait) { waits++ })
	for i := 0; i < 3; i++ {
		if _, _, err := s.FetchBlob(context.Background(), ghRepo, "abc", 100, tok); err != nil {
			t.Fatal(err)
		}
	}
	if fc.positive() != 0 || waits != 0 || s.Stats() != (Stats{}) {
		t.Fatalf("sleeps=%v waits=%d stats=%+v", fc.sleeps, waits, s.Stats())
	}
}

func TestSession_IntervalConcurrent(t *testing.T) {
	var mu sync.Mutex
	var starts []time.Time
	var fc *fakeClock
	c, fc, _, _ := paced(t, PaceOptions{Interval: 500 * time.Millisecond, MaxWait: time.Minute, MaxTotalWait: time.Minute}, func(w http.ResponseWriter, q *http.Request) {
		okBlob(w)
	}, false)
	s := c.Session(func(Wait) { t.Error("onWait tidak boleh dipanggil untuk interval") })
	// Awal request = now() sesudah sleep; catat lewat pembungkus sleep.
	orig := c.sleep
	c.sleep = func(ctx context.Context, d time.Duration) error {
		err := orig(ctx, d)
		mu.Lock()
		starts = append(starts, fc.now())
		mu.Unlock()
		return err
	}
	var wg sync.WaitGroup
	for g := 0; g < 4; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 5; i++ {
				if _, _, err := s.FetchBlob(context.Background(), ghRepo, "abc", 100, tok); err != nil {
					t.Error(err)
				}
			}
		}()
	}
	wg.Wait()
	// Slot bertambah 500ms tiap request: total sleep >= 19*500ms dalam jam palsu.
	if got := fc.now().Sub(time.Unix(1_800_000_000, 0)); got < 19*500*time.Millisecond {
		t.Fatalf("jam maju %v, want >= 9.5s", got)
	}
	if s.Stats() != (Stats{}) {
		t.Fatal(s.Stats())
	}
}

func TestSession_ProactiveExhausted(t *testing.T) {
	var n int
	var c *Client
	c, fc, _, _ := paced(t, PaceOptions{MaxWait: 2 * time.Minute, MaxTotalWait: 5 * time.Minute}, func(w http.ResponseWriter, q *http.Request) {
		n++
		if n == 1 {
			w.Header().Set("X-RateLimit-Remaining", "0")
			w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(c.now().Add(30*time.Second).Unix(), 10))
		}
		okBlob(w)
	}, false)
	var got []Wait
	s := c.Session(func(w Wait) { got = append(got, w) })
	for i := 0; i < 2; i++ {
		if _, _, err := s.FetchBlob(context.Background(), ghRepo, "abc", 100, tok); err != nil {
			t.Fatal(err)
		}
	}
	if len(got) != 1 || got[0].Reason != WaitRateLimitExhausted || got[0].Duration != 30*time.Second {
		t.Fatalf("waits: %+v", got)
	}
	if st := s.Stats(); st.Waits != 1 || st.TotalWait != 30*time.Second {
		t.Fatalf("stats %+v", st)
	}
	if fc.positive() != 1 {
		t.Fatal(fc.sleeps)
	}
}

func TestSession_429RetryThenOK(t *testing.T) {
	var n int
	c, _, r, _ := paced(t, PaceOptions{MaxWait: 2 * time.Minute, MaxTotalWait: 5 * time.Minute}, func(w http.ResponseWriter, q *http.Request) {
		n++
		if n == 1 {
			w.Header().Set("Retry-After", "2")
			w.WriteHeader(429)
			return
		}
		okBlob(w)
	}, false)
	var got []Wait
	s := c.Session(func(w Wait) { got = append(got, w) })
	if _, _, err := s.FetchBlob(context.Background(), ghRepo, "abc", 100, tok); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != (Wait{2 * time.Second, WaitRateLimited}) || len(r.reqs) != 2 {
		t.Fatalf("waits %+v reqs %d", got, len(r.reqs))
	}
}

func TestSession_403RateNoResetBackoff(t *testing.T) {
	var n int
	c, fc, _, _ := paced(t, PaceOptions{MaxWait: 2 * time.Minute, MaxTotalWait: 5 * time.Minute}, func(w http.ResponseWriter, q *http.Request) {
		n++
		if n == 1 {
			w.Header().Set("X-RateLimit-Remaining", "0")
			w.WriteHeader(403)
			return
		}
		okBlob(w)
	}, false)
	s := c.Session(nil)
	if _, _, err := s.FetchBlob(context.Background(), ghRepo, "abc", 100, tok); err != nil {
		t.Fatal(err)
	}
	if len(fc.sleeps) == 0 || fc.sleeps[len(fc.sleeps)-1] != time.Second {
		t.Fatal(fc.sleeps)
	}
}

func TestSession_RetriesExhausted(t *testing.T) {
	c, _, r, _ := paced(t, PaceOptions{MaxWait: 2 * time.Minute, MaxTotalWait: time.Hour}, func(w http.ResponseWriter, q *http.Request) {
		w.WriteHeader(429)
	}, false)
	_, _, err := c.Session(nil).FetchBlob(context.Background(), ghRepo, "abc", 100, tok)
	if !errors.Is(err, ErrRateLimited) || len(r.reqs) != 4 {
		t.Fatalf("err=%v reqs=%d", err, len(r.reqs))
	}
}

func TestSession_MaxWaitExceeded(t *testing.T) {
	var c *Client
	c, fc, r, _ := paced(t, PaceOptions{MaxWait: 120 * time.Second, MaxTotalWait: 5 * time.Minute}, func(w http.ResponseWriter, q *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(c.now().Add(300*time.Second).Unix(), 10))
		okBlob(w)
	}, false)
	called := false
	s := c.Session(func(Wait) { called = true })
	if _, _, err := s.FetchBlob(context.Background(), ghRepo, "abc", 100, tok); err != nil {
		t.Fatal(err)
	}
	n := len(fc.sleeps)
	_, _, err := s.FetchBlob(context.Background(), ghRepo, "abc", 100, tok)
	var rl *RateLimitedError
	if !errors.As(err, &rl) || rl.RetryAfter != 300*time.Second {
		t.Fatalf("err %v", err)
	}
	if len(fc.sleeps) != n || called || len(r.reqs) != 1 {
		t.Fatalf("sleep/onWait/req tidak boleh terjadi: %v %v %d", fc.sleeps, called, len(r.reqs))
	}
}

func TestSession_TotalWaitExceeded(t *testing.T) {
	var c *Client
	c, fc, _, _ := paced(t, PaceOptions{MaxWait: 2 * time.Minute, MaxTotalWait: 250 * time.Second}, func(w http.ResponseWriter, q *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(c.now().Add(100*time.Second).Unix(), 10))
		okBlob(w)
	}, false)
	s := c.Session(nil)
	var err error
	for i := 0; i < 4 && err == nil; i++ {
		_, _, err = s.FetchBlob(context.Background(), ghRepo, "abc", 100, tok)
		if i == 3 && err == nil {
			t.Fatal("tunggu ketiga harus gagal")
		}
	}
	if !errors.Is(err, ErrRateLimited) {
		t.Fatal(err)
	}
	if st := s.Stats(); st.Waits != 2 || st.TotalWait != 200*time.Second {
		t.Fatalf("stats %+v sleeps %v", st, fc.sleeps)
	}
}

func TestSession_CtxCancelledDuringSleep(t *testing.T) {
	c, _, r, _ := paced(t, PaceOptions{Interval: time.Second, MaxWait: time.Minute, MaxTotalWait: time.Minute}, func(w http.ResponseWriter, q *http.Request) { okBlob(w) }, false)
	ctx, cancel := context.WithCancel(context.Background())
	c.sleep = func(ctx context.Context, d time.Duration) error { cancel(); return ctx.Err() }
	s := c.Session(nil)
	_, _, _ = s.FetchBlob(context.Background(), ghRepo, "abc", 100, tok) // slot pertama, tanpa sleep
	_, _, err := s.FetchBlob(ctx, ghRepo, "abc", 100, tok)
	if !errors.Is(err, context.Canceled) || len(r.reqs) != 1 {
		t.Fatalf("err=%v reqs=%d", err, len(r.reqs))
	}
}

func TestParseRateHeaders(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	h := func(kv ...string) http.Header {
		hh := http.Header{}
		for i := 0; i < len(kv); i += 2 {
			hh.Set(kv[i], kv[i+1])
		}
		return hh
	}
	_, _, r1, _ := parseRateHeaders(h("RateLimit-Reset", "30"), now)
	_, _, r2, _ := parseRateHeaders(h("X-RateLimit-Reset", strconv.FormatInt(now.Add(30*time.Second).Unix(), 10)), now)
	if !r1.Equal(r2) || r1.Sub(now) != 30*time.Second {
		t.Fatalf("%v %v", r1, r2)
	}
	if _, _, _, ra := parseRateHeaders(h("Retry-After", "-5"), now); ra != 0 {
		t.Fatal(ra)
	}
	if _, _, _, ra := parseRateHeaders(h("Retry-After", "999999999"), now); ra != 24*time.Hour {
		t.Fatal(ra)
	}
	rem, ok, _, _ := parseRateHeaders(h("RateLimit-Remaining", "7"), now)
	if !ok || rem != 7 {
		t.Fatal(rem, ok)
	}
}

func TestSession_HugeRetryAfterFailsMaxWait(t *testing.T) {
	c, fc, _, _ := paced(t, PaceOptions{MaxWait: 2 * time.Minute, MaxTotalWait: 5 * time.Minute}, func(w http.ResponseWriter, q *http.Request) {
		w.Header().Set("Retry-After", "999999999")
		w.WriteHeader(429)
	}, false)
	_, _, err := c.Session(nil).FetchBlob(context.Background(), ghRepo, "abc", 100, tok)
	var rl *RateLimitedError
	if !errors.As(err, &rl) || rl.RetryAfter != 24*time.Hour || fc.positive() != 0 {
		t.Fatalf("err=%v sleeps=%v", err, fc.sleeps)
	}
}

func TestSession_Forbidden403NoRetry(t *testing.T) {
	c, _, r, _ := paced(t, PaceOptions{MaxWait: time.Minute, MaxTotalWait: time.Minute}, func(w http.ResponseWriter, q *http.Request) { w.WriteHeader(403) }, false)
	_, _, err := c.Session(nil).FetchBlob(context.Background(), ghRepo, "abc", 100, tok)
	if !errors.Is(err, ErrForbidden) || len(r.reqs) != 1 {
		t.Fatalf("err=%v reqs=%d", err, len(r.reqs))
	}
}

func TestSession_GitLabTreePacedPerPage(t *testing.T) {
	var srvURL string
	c, fc, r, host := paced(t, PaceOptions{Interval: 500 * time.Millisecond, MaxWait: time.Minute, MaxTotalWait: time.Minute}, func(w http.ResponseWriter, q *http.Request) {
		page := q.URL.Query().Get("p")
		if page == "" {
			page = "1"
		}
		n, _ := strconv.Atoi(page)
		if n < 3 {
			w.Header().Set("Link", fmt.Sprintf(`<%s%s?p=%d>; rel="next"`, srvURL, q.URL.Path, n+1))
		}
		fmt.Fprintf(w, `[{"id":"s%d","type":"blob","path":"a%d.md"}]`, n, n)
	}, true)
	srvURL = "http://" + host
	repo := Repo{Provider: GitLab, Host: host, Path: "g/p", Branch: "main"}
	out, err := c.Session(nil).ListTree(context.Background(), repo, "", tok)
	if err != nil || len(out) != 3 || len(r.reqs) != 3 {
		t.Fatalf("out=%d reqs=%d err=%v", len(out), len(r.reqs), err)
	}
	if got := fc.now().Sub(time.Unix(1_800_000_000, 0)); got != time.Second {
		t.Fatalf("3 slot = 2 interval antar-awal, got %v", got)
	}
}

func TestSession_TokenNeverInErrors(t *testing.T) {
	for _, st := range []int{401, 403, 404, 429, 500} {
		c, _, _, _ := paced(t, PaceOptions{MaxWait: time.Minute, MaxTotalWait: time.Minute}, func(w http.ResponseWriter, q *http.Request) {
			w.WriteHeader(st)
			fmt.Fprint(w, canary)
		}, false)
		_, _, err := c.Session(nil).FetchBlob(context.Background(), ghRepo, "abc", 100, tok)
		if err == nil || strings.Contains(err.Error(), canary) {
			t.Fatalf("status %d: %v", st, err)
		}
	}
}

func TestClient_LegacyNotPaced(t *testing.T) {
	c, fc, _, _ := paced(t, PaceOptions{Interval: time.Hour, MaxWait: time.Minute, MaxTotalWait: time.Minute}, func(w http.ResponseWriter, q *http.Request) { okBlob(w) }, false)
	for i := 0; i < 3; i++ {
		if _, _, err := c.FetchBlob(context.Background(), ghRepo, "abc", 100, tok); err != nil {
			t.Fatal(err)
		}
	}
	if err := c.VerifyToken(context.Background(), GitHub, "github.com", tok); err != nil {
		t.Fatal(err)
	}
	if len(fc.sleeps) != 0 {
		t.Fatal(fc.sleeps)
	}
}
