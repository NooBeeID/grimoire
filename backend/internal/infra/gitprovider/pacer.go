package gitprovider

import (
	"context"
	"net/http"
	"strconv"
	"sync"
	"time"

	"grimoire/lib/common/secret"
)

const (
	maxRateLimitRetries = 3
	maxHeaderWait       = 24 * time.Hour
	epochThreshold      = 1_000_000_000
)

// WaitReason menjelaskan mengapa Session menunggu.
type WaitReason string

const (
	// WaitRateLimitExhausted: proaktif, jatah provider habis (remaining=0).
	WaitRateLimitExhausted WaitReason = "rate_limit_exhausted"
	// WaitRateLimited: provider membalas 429 / 403 rate limit.
	WaitRateLimited WaitReason = "rate_limited"
)

// PaceOptions mengatur pacing request dalam satu Session.
type PaceOptions struct {
	// Interval: jarak minimal antar awal request (0 = tanpa jeda).
	Interval time.Duration
	// MaxWait: batas satu kali tunggu rate limit.
	MaxWait time.Duration
	// MaxTotalWait: batas total tunggu rate limit per Session.
	MaxTotalWait time.Duration
	// MaxRetries: retry maksimal setelah 429/403-rl; 0 = maxRateLimitRetries.
	MaxRetries int
}

func (o PaceOptions) withDefaults() PaceOptions {
	if o.MaxRetries <= 0 {
		o.MaxRetries = maxRateLimitRetries
	}
	return o
}

// Wait dilaporkan ke onWait saat Session menunggu rate limit.
type Wait struct {
	Duration time.Duration
	Reason   WaitReason
}

// Stats merangkum tunggu rate limit sebuah Session.
type Stats struct {
	Waits     int
	TotalWait time.Duration
}

// Session adalah jalur request ber-pacing untuk satu sync. Aman dipakai bersamaan.
type Session struct {
	c      *Client
	onWait func(Wait)

	mu           sync.Mutex
	nextSlot     time.Time
	blockedUntil time.Time
	accounted    time.Time // batas blockedUntil yang sudah dihitung ke totalWait
	reason       WaitReason
	waits        int
	totalWait    time.Duration
}

// Session membuat Session baru; onWait boleh nil.
func (c *Client) Session(onWait func(Wait)) *Session {
	return &Session{c: c, onWait: onWait}
}

// ListTree seperti Client.ListTree, dengan pacing.
func (s *Session) ListTree(ctx context.Context, repo Repo, folder string, token secret.String) ([]TreeEntry, error) {
	return s.c.listTree(ctx, s.get, repo, folder, token)
}

// FetchBlob seperti Client.FetchBlob, dengan pacing.
func (s *Session) FetchBlob(ctx context.Context, repo Repo, sha string, maxBytes int64, token secret.String) ([]byte, bool, error) {
	return s.c.fetchBlob(ctx, s.get, repo, sha, maxBytes, token)
}

// Stats mengembalikan ringkasan tunggu sejauh ini.
func (s *Session) Stats() Stats {
	s.mu.Lock()
	defer s.mu.Unlock()
	return Stats{Waits: s.waits, TotalWait: s.totalWait}
}

func (s *Session) get(ctx context.Context, p Provider, u string, token secret.String, accept string) (*http.Response, error) {
	pc := s.c.pace
	maxRetries := pc.MaxRetries
	if maxRetries <= 0 {
		maxRetries = maxRateLimitRetries
	}
	for retry := 0; ; retry++ {
		wait, rl, reason, err := s.reserve()
		if err != nil {
			return nil, err
		}
		if rl > 0 && s.onWait != nil {
			s.onWait(Wait{Duration: rl, Reason: reason})
		}
		if wait > 0 {
			if err := s.c.sleep(ctx, wait); err != nil {
				return nil, err
			}
		} else if err := ctx.Err(); err != nil {
			return nil, err
		}
		resp, err := s.c.send(ctx, p, u, token, accept)
		if err != nil {
			return nil, err
		}
		remaining, hasRem, reset, _ := parseRateHeaders(resp.Header, s.c.now())
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			s.noteExhausted(remaining, hasRem, reset)
			return resp, nil
		}
		mapped := mapStatus(resp)
		drain(resp)
		rle, isRL := mapped.(*RateLimitedError)
		if !isRL {
			s.noteExhausted(remaining, hasRem, reset)
			return nil, mapped
		}
		if retry >= maxRetries {
			return nil, rle
		}
		d := rle.RetryAfter
		if d <= 0 {
			d = time.Second << retry // 1s, 2s, 4s
		}
		s.block(s.c.now().Add(d), WaitRateLimited)
	}
}

// reserve mengambil slot request. Bila tunggu rate limit melewati batas, kembalikan
// RateLimitedError tanpa tidur.
func (s *Session) reserve() (wait, rl time.Duration, reason WaitReason, err error) {
	pc := s.c.pace
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.c.now()
	if d := s.blockedUntil.Sub(now); d > 0 {
		rl = d
	}
	if rl > 0 {
		if (pc.MaxWait > 0 && rl > pc.MaxWait) || (pc.MaxTotalWait > 0 && s.totalWait+s.addedWait(now) > pc.MaxTotalWait) {
			return 0, 0, "", &RateLimitedError{RetryAfter: rl}
		}
		if add := s.addedWait(now); add > 0 {
			s.totalWait += add
			s.waits++
			s.accounted = s.blockedUntil
		}
		reason = s.reason
	}
	start := now
	if s.nextSlot.After(start) {
		start = s.nextSlot
	}
	if s.blockedUntil.After(start) {
		start = s.blockedUntil
	}
	s.nextSlot = start.Add(pc.Interval)
	return start.Sub(now), rl, reason, nil
}

// addedWait: tunggu baru yang belum dihitung (goroutine lain yang menunggu blok yang sama tidak dihitung ganda).
func (s *Session) addedWait(now time.Time) time.Duration {
	from := now
	if s.accounted.After(from) {
		from = s.accounted
	}
	if d := s.blockedUntil.Sub(from); d > 0 {
		return d
	}
	return 0
}

func (s *Session) block(until time.Time, reason WaitReason) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if until.After(s.blockedUntil) {
		s.blockedUntil = until
		s.reason = reason
	}
}

func (s *Session) noteExhausted(remaining int, hasRem bool, reset time.Time) {
	if hasRem && remaining == 0 && !reset.IsZero() {
		s.block(reset, WaitRateLimitExhausted)
	}
}

// parseRateHeaders membaca header rate limit provider. Semua durasi di-clamp [0, 24h].
// reset zero bila tidak diketahui.
func parseRateHeaders(h http.Header, now time.Time) (remaining int, hasRemaining bool, reset time.Time, retryAfter time.Duration) {
	for _, k := range []string{"X-RateLimit-Remaining", "RateLimit-Remaining"} {
		if v := h.Get(k); v != "" {
			if n, err := strconv.Atoi(v); err == nil {
				remaining, hasRemaining = n, true
				break
			}
		}
	}
	for _, k := range []string{"X-RateLimit-Reset", "RateLimit-Reset"} {
		if v := h.Get(k); v != "" {
			if n, err := strconv.ParseInt(v, 10, 64); err == nil {
				var t time.Time
				if n > epochThreshold {
					t = time.Unix(n, 0)
				} else {
					t = now.Add(clampWait(time.Duration(n) * time.Second))
				}
				reset = now.Add(clampWait(t.Sub(now)))
				break
			}
		}
	}
	if v := h.Get("Retry-After"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			if n > int64(maxHeaderWait/time.Second) {
				n = int64(maxHeaderWait / time.Second)
			}
			retryAfter = clampWait(time.Duration(n) * time.Second)
		}
	}
	return
}

func clampWait(d time.Duration) time.Duration {
	if d < 0 {
		return 0
	}
	if d > maxHeaderWait {
		return maxHeaderWait
	}
	return d
}

// rateRetryAfter: Retry-After, lalu selisih reset; 0 = tidak diketahui.
func rateRetryAfter(h http.Header, now time.Time) time.Duration {
	_, _, reset, ra := parseRateHeaders(h, now)
	if ra > 0 {
		return ra
	}
	if !reset.IsZero() {
		return reset.Sub(now)
	}
	return 0
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
