package base

import "time"

// ThrottlePolicy adalah kebijakan lockout login per email.
type ThrottlePolicy struct {
	MaxFailures  int
	Window       time.Duration
	LockDuration time.Duration
}

// LoginThrottle adalah status percobaan gagal per email (terdaftar maupun tidak).
type LoginThrottle struct {
	Email           string
	FailedCount     int
	WindowStartedAt time.Time
	LockedUntil     *time.Time
}

// IsLocked: true selama now < LockedUntil.
func (t LoginThrottle) IsLocked(now time.Time) bool {
	return t.LockedUntil != nil && now.Before(*t.LockedUntil)
}

// RetryAfter adalah sisa waktu kunci dibulatkan ke atas ke detik penuh; 0 bila tidak terkunci.
func (t LoginThrottle) RetryAfter(now time.Time) time.Duration {
	if !t.IsLocked(now) {
		return 0
	}
	d := t.LockedUntil.Sub(now)
	return (d + time.Second - 1).Truncate(time.Second)
}

// RegisterFailure mencatat satu kegagalan; window yang sudah lewat di-reset lebih dulu.
func (t *LoginThrottle) RegisterFailure(now time.Time, p ThrottlePolicy) {
	if t.WindowStartedAt.IsZero() || !now.Before(t.WindowStartedAt.Add(p.Window)) {
		t.FailedCount = 0
		t.WindowStartedAt = now
		t.LockedUntil = nil
	}
	t.FailedCount++
	if t.FailedCount >= p.MaxFailures {
		until := now.Add(p.LockDuration)
		t.LockedUntil = &until
	}
}

// Expired mencerminkan predikat purge SQL: tidak sedang terkunci dan window sudah lewat.
func (t LoginThrottle) Expired(now time.Time, p ThrottlePolicy) bool {
	unlocked := t.LockedUntil == nil || t.LockedUntil.Before(now)
	return unlocked && t.WindowStartedAt.Before(now.Add(-p.Window))
}
