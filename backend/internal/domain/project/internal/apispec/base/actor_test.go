package base

import (
	"strings"
	"testing"

	"grimoire/lib/common/reqctx"
)

func TestActorFrom(t *testing.T) {
	a, ok := ActorFrom(reqctx.Identity{UserID: "u", Name: "Budi", Email: "b@x"}, true)
	if !ok || a.ID != "u" || a.Label != "Budi" {
		t.Fatalf("%+v %v", a, ok)
	}
	a, ok = ActorFrom(reqctx.Identity{UserID: "u", Email: "b@x"}, true)
	if !ok || a.Label != "b@x" {
		t.Fatalf("fallback email: %+v", a)
	}
	if _, ok := ActorFrom(reqctx.Identity{UserID: "u"}, false); ok {
		t.Fatal("ok=false")
	}
	if _, ok := ActorFrom(reqctx.Identity{}, true); ok {
		t.Fatal("userID kosong")
	}
}

func TestNewRequestMeta_TruncatesOnRuneBoundary(t *testing.T) {
	ua := strings.Repeat("a", 511) + "é" // é = 2 byte, melewati batas 512
	m := NewRequestMeta("1.2.3.4", ua)
	if len(m.UserAgent) != 511 || m.IP != "1.2.3.4" {
		t.Fatalf("len=%d", len(m.UserAgent))
	}
	if short := NewRequestMeta("", "ua"); short.UserAgent != "ua" {
		t.Fatal("short")
	}
}

func TestSystemClockUTC(t *testing.T) {
	if SystemClock().Location().String() != "UTC" {
		t.Fatal("not UTC")
	}
}
