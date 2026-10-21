package base

import "grimoire/lib/common/reqctx"

// Actor adalah pelaku request. Label = Name; fallback Email (D6).
type Actor struct{ ID, Label string }

// ActorFrom membangun Actor; ok=false atau UserID kosong -> false.
func ActorFrom(id reqctx.Identity, ok bool) (Actor, bool) {
	if !ok || id.UserID == "" {
		return Actor{}, false
	}
	label := id.Name
	if label == "" {
		label = id.Email
	}
	return Actor{ID: id.UserID, Label: label}, true
}
