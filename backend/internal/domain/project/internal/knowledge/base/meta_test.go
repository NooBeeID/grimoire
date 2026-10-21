package base

import (
	"strings"
	"testing"

	"grimoire/lib/common/reqctx"
)

func TestActorFrom(t *testing.T) {
	if _, ok := ActorFrom(reqctx.Identity{UserID: "u"}, false); ok {
		t.Fatal("ok=false must fail")
	}
	if _, ok := ActorFrom(reqctx.Identity{}, true); ok {
		t.Fatal("empty user must fail")
	}
	a, ok := ActorFrom(reqctx.Identity{UserID: "u", Name: " Budi ", Email: "b@x", Role: "developer"}, true)
	if !ok || a.Label != "Budi" || a.Role != "developer" || a.ID != "u" {
		t.Fatalf("%+v", a)
	}
	a, _ = ActorFrom(reqctx.Identity{UserID: "u", Name: "  ", Email: "b@x"}, true)
	if a.Label != "b@x" {
		t.Fatalf("fallback: %+v", a)
	}
}

func TestNewRequestMeta(t *testing.T) {
	m := NewRequestMeta("1.2.3.4", strings.Repeat("é", 400))
	if len(m.UserAgent) > 512 || len(m.UserAgent)%2 != 0 || m.IP != "1.2.3.4" {
		t.Fatalf("len=%d", len(m.UserAgent))
	}
}
