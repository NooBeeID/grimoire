package base

import (
	"strconv"
	"testing"
	"time"
)

func TestAuditThrottle(t *testing.T) {
	now := time.Unix(1000, 0)
	th := NewAuditThrottle(time.Minute, func() time.Time { return now })
	if !th.Allow("key:a") || th.Allow("key:a") {
		t.Fatal("entri kedua dalam window harus ditolak")
	}
	if !th.Allow("key:b") {
		t.Fatal("subjek berbeda independen")
	}
	now = now.Add(59 * time.Second)
	if th.Allow("key:a") {
		t.Fatal("masih dalam window")
	}
	now = now.Add(2 * time.Second)
	if !th.Allow("key:a") {
		t.Fatal("window lewat")
	}
}

func TestAuditThrottle_Sweep(t *testing.T) {
	now := time.Unix(1000, 0)
	th := NewAuditThrottle(time.Minute, func() time.Time { return now })
	for i := 0; i < 1100; i++ {
		th.Allow("k" + strconv.Itoa(i))
	}
	now = now.Add(2 * time.Minute)
	th.Allow("baru")
	if len(th.last) != 1 {
		t.Fatalf("entri kedaluwarsa harus dibersihkan, sisa %d", len(th.last))
	}
}
