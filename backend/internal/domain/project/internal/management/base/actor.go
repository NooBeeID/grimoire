package base

import (
	"grimoire/lib/common/reqctx"
	"grimoire/lib/contracts/core/auth/resolvesession"
)

// Actor adalah pelaku request. Label = email aktor (snapshot audit dan *_by_label).
type Actor struct{ ID, Role, Label string }

// ActorFrom membangun Actor dari identitas; ok=false atau UserID kosong -> false.
func ActorFrom(id reqctx.Identity, ok bool) (Actor, bool) {
	if !ok || id.UserID == "" {
		return Actor{}, false
	}
	return Actor{ID: id.UserID, Role: id.Role, Label: id.Email}, true
}

func (a Actor) IsSuperAdmin() bool { return a.Role == resolvesession.RoleSuperAdmin }
