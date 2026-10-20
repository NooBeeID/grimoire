package base

import (
	"testing"
	"time"
)

var tp = ThrottlePolicy{MaxFailures: 5, Window: 15 * time.Minute, LockDuration: 15 * time.Minute}

func TestThrottle_RegisterFailure(t *testing.T) {
	now := time.Unix(1000, 0)
	var th LoginThrottle
	for i := 0; i < 4; i++ {
		th.RegisterFailure(now, tp)
		if th.IsLocked(now) {
			t.Fatalf("terkunci terlalu cepat pada percobaan %d", i+1)
		}
	}
	th.RegisterFailure(now, tp)
	if !th.IsLocked(now) || th.FailedCount != 5 || !th.LockedUntil.Equal(now.Add(15*time.Minute)) {
		t.Fatalf("%+v", th)
	}
	if th.IsLocked(now.Add(15 * time.Minute)) {
		t.Fatal("tidak terkunci tepat di LockedUntil")
	}
}

func TestThrottle_WindowResets(t *testing.T) {
	now := time.Unix(1000, 0)
	th := LoginThrottle{FailedCount: 4, WindowStartedAt: now}
	later := now.Add(15 * time.Minute)
	th.RegisterFailure(later, tp)
	if th.FailedCount != 1 || !th.WindowStartedAt.Equal(later) || th.LockedUntil != nil || th.IsLocked(later) {
		t.Fatalf("%+v", th)
	}
}

func TestThrottle_RetryAfterRoundsUp(t *testing.T) {
	now := time.Unix(1000, 0)
	until := now.Add(1500 * time.Millisecond)
	th := LoginThrottle{LockedUntil: &until}
	if got := th.RetryAfter(now); got != 2*time.Second {
		t.Fatalf("got %v", got)
	}
	if th.RetryAfter(until) != 0 || (LoginThrottle{}).RetryAfter(now) != 0 {
		t.Fatal("tidak terkunci harus 0")
	}
}

func TestThrottle_Expired(t *testing.T) {
	now := time.Unix(100000, 0)
	old := now.Add(-16 * time.Minute)
	lockedFuture := now.Add(time.Minute)
	lockedPast := now.Add(-time.Second)
	cases := []struct {
		name string
		th   LoginThrottle
		want bool
	}{
		{"window lewat tanpa kunci", LoginThrottle{WindowStartedAt: old}, true},
		{"window masih berjalan", LoginThrottle{WindowStartedAt: now.Add(-time.Minute)}, false},
		{"masih terkunci", LoginThrottle{WindowStartedAt: old, LockedUntil: &lockedFuture}, false},
		{"kunci sudah lewat", LoginThrottle{WindowStartedAt: old, LockedUntil: &lockedPast}, true},
		{"tepat batas window", LoginThrottle{WindowStartedAt: now.Add(-15 * time.Minute)}, false},
	}
	for _, c := range cases {
		if got := c.th.Expired(now, tp); got != c.want {
			t.Errorf("%s: got %v", c.name, got)
		}
	}
}
