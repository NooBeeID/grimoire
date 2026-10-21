package base

import (
	"testing"

	"grimoire/lib/common/reqctx"
)

func TestActorFrom(t *testing.T) {
	if _, ok := ActorFrom(reqctx.Identity{UserID: "u"}, false); ok {
		t.Error("ok=false")
	}
	if _, ok := ActorFrom(reqctx.Identity{}, true); ok {
		t.Error("UserID kosong")
	}
	a, ok := ActorFrom(reqctx.Identity{UserID: "u", Email: "u@x", Role: "super_admin"}, true)
	if !ok || a.ID != "u" || a.Label != "u@x" || !a.IsSuperAdmin() {
		t.Errorf("%+v", a)
	}
}
