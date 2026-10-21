package base

import (
	"sync"
	"time"
)

const throttleSweepThreshold = 1024

// AuditThrottle membatasi entri audit: maks 1 per subjek per window.
type AuditThrottle struct {
	mu     sync.Mutex
	last   map[string]time.Time
	window time.Duration
	now    func() time.Time
}

// NewAuditThrottle membuat throttle; now nil memakai time.Now.
func NewAuditThrottle(window time.Duration, now func() time.Time) *AuditThrottle {
	if now == nil {
		now = time.Now
	}
	return &AuditThrottle{last: map[string]time.Time{}, window: window, now: now}
}

// Allow true bila belum ada entri untuk subject dalam window (dan mencatatnya).
func (t *AuditThrottle) Allow(subject string) bool {
	now := t.now()
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(t.last) > throttleSweepThreshold {
		for k, v := range t.last {
			if now.Sub(v) >= t.window {
				delete(t.last, k)
			}
		}
	}
	if v, ok := t.last[subject]; ok && now.Sub(v) < t.window {
		return false
	}
	t.last[subject] = now
	return true
}
