package middleware

import (
	"context"
	"grimoire/internal/infra/http/respond"
	"math"
	"net/http"
	"strconv"
	"sync"
	"time"

	"golang.org/x/time/rate"
	"grimoire/lib/common/errs"
	"grimoire/lib/common/reqctx"
)

// Rule mendefinisikan satu aturan rate limit.
type Rule struct {
	Name    string
	Rate    rate.Limit // token per detik
	Burst   int
	KeyFunc func(r *http.Request) string // "" = tidak dibatasi
}

type entry struct {
	lim      *rate.Limiter
	lastSeen time.Time
}

// Limiter adalah rate limiter per key dengan janitor entri idle.
type Limiter struct {
	rule    Rule
	mu      sync.Mutex
	entries map[string]*entry
	now     func() time.Time
	idleTTL time.Duration
	done    chan struct{} // ditutup saat janitor berhenti
}

// NewLimiter membuat Limiter; janitor berhenti saat ctx selesai.
func NewLimiter(ctx context.Context, rule Rule) *Limiter {
	return newLimiter(ctx, rule, time.Now, time.Minute, 10*time.Minute)
}

func newLimiter(ctx context.Context, rule Rule, now func() time.Time, every, ttl time.Duration) *Limiter {
	if rule.Rate <= 0 || rule.Burst < 1 || rule.KeyFunc == nil {
		panic("httpx: Rule rate limit tidak valid (Rate>0, Burst>=1, KeyFunc wajib)")
	}
	l := &Limiter{rule: rule, entries: map[string]*entry{}, now: now, idleTTL: ttl, done: make(chan struct{})}
	go func() {
		defer close(l.done)
		t := time.NewTicker(every)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				l.sweep()
			}
		}
	}()
	return l
}

func (l *Limiter) sweep() {
	cutoff := l.now().Add(-l.idleTTL)
	l.mu.Lock()
	defer l.mu.Unlock()
	for k, e := range l.entries {
		if e.lastSeen.Before(cutoff) {
			delete(l.entries, k)
		}
	}
}

func (l *Limiter) size() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.entries)
}

// Allow mengambil satu token untuk key; bila habis mengembalikan lama tunggu.
func (l *Limiter) Allow(key string) (bool, time.Duration) {
	now := l.now()
	l.mu.Lock()
	e, ok := l.entries[key]
	if !ok {
		e = &entry{lim: rate.NewLimiter(l.rule.Rate, l.rule.Burst)}
		l.entries[key] = e
	}
	e.lastSeen = now
	res := e.lim.ReserveN(now, 1)
	l.mu.Unlock()
	if !res.OK() {
		return false, time.Duration(float64(time.Second) / float64(l.rule.Rate))
	}
	if d := res.DelayFrom(now); d > 0 {
		res.CancelAt(now)
		return false, d
	}
	return true, 0
}

// Peek memeriksa apakah key masih punya token tanpa mengonsumsi dan tanpa membuat entri.
// Key belum dikenal -> (true, 0).
func (l *Limiter) Peek(key string) (bool, time.Duration) {
	now := l.now()
	l.mu.Lock()
	defer l.mu.Unlock()
	e, ok := l.entries[key]
	if !ok {
		return true, 0
	}
	tokens := e.lim.TokensAt(now)
	if tokens >= 1 {
		return true, 0
	}
	wait := time.Duration((1 - tokens) / float64(l.rule.Rate) * float64(time.Second))
	return false, wait
}

// RateLimit menolak dengan 429 + Retry-After bila token habis.
func RateLimit(l *Limiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := l.rule.KeyFunc(r)
			if key == "" {
				next.ServeHTTP(w, r)
				return
			}
			ok, wait := l.Allow(key)
			if !ok {
				secs := int(math.Ceil(wait.Seconds()))
				if secs < 1 {
					secs = 1
				}
				w.Header().Set("Retry-After", strconv.Itoa(secs))
				respond.WriteError(w, r, errs.WithDetails(errs.CodeRateLimited,
					"terlalu banyak permintaan, coba lagi nanti", map[string]int{"retry_after_seconds": secs}))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// KeyByIP memakai host dari RemoteAddr.
func KeyByIP(r *http.Request) string { return remoteIP(r) }

// KeyByUser memakai user id sesi; fallback ke IP.
func KeyByUser(r *http.Request) string {
	if id := reqctx.UserID(r.Context()); id != "" {
		return id
	}
	return KeyByIP(r)
}
