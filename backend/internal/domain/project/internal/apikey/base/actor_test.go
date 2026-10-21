package base

import (
	"strings"
	"testing"
	"unicode/utf8"

	"grimoire/lib/common/reqctx"
)

func TestActorFrom(t *testing.T) {
	a, ok := ActorFrom(reqctx.Identity{UserID: "u", Role: "developer", Email: "u@x"}, true)
	if !ok || a != (Actor{ID: "u", Role: "developer", Label: "u@x"}) {
		t.Fatalf("%+v %v", a, ok)
	}
	if _, ok := ActorFrom(reqctx.Identity{UserID: "u"}, false); ok {
		t.Fatal("ok=false")
	}
	if _, ok := ActorFrom(reqctx.Identity{}, true); ok {
		t.Fatal("empty user")
	}
}

func TestNewRequestMeta(t *testing.T) {
	m := NewRequestMeta("1.2.3.4", "ua")
	if m.IP != "1.2.3.4" || m.UserAgent != "ua" {
		t.Fatalf("%+v", m)
	}
	m = NewRequestMeta("", strings.Repeat("é", 300)) // 600 byte
	if len(m.UserAgent) > 512 || !utf8.ValidString(m.UserAgent) || len(m.UserAgent) < 510 {
		t.Fatalf("len=%d", len(m.UserAgent))
	}
}

func TestSystemClock(t *testing.T) {
	if SystemClock().Location().String() != "UTC" {
		t.Fatal("utc")
	}
}
