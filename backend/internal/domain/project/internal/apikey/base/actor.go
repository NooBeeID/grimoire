package base

import "grimoire/lib/common/reqctx"

// Actor adalah pelaku request. Label = email aktor (K2).
type Actor struct{ ID, Role, Label string }

// ActorFrom membangun Actor; ok=false atau UserID kosong -> false.
func ActorFrom(id reqctx.Identity, ok bool) (Actor, bool) {
	if !ok || id.UserID == "" {
		return Actor{}, false
	}
	return Actor{ID: id.UserID, Role: id.Role, Label: id.Email}, true
}
