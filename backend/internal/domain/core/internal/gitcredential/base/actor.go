package base

import (
	"time"
	"unicode/utf8"

	"grimoire/lib/common/reqctx"
)

// Actor adalah pelaku request. Label = Name; fallback Email.
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

// Clock memberi waktu sekarang (seam tes).
type Clock func() time.Time

// SystemClock memakai waktu sistem.
var SystemClock Clock = time.Now

const maxUserAgentBytes = 512

// RequestMeta adalah metadata request yang diteruskan handler ke usecase.
type RequestMeta struct{ IP, UserAgent string }

// NewRequestMeta memotong UA maksimal 512 byte tanpa memotong rune.
func NewRequestMeta(ip, ua string) RequestMeta {
	if len(ua) > maxUserAgentBytes {
		cut := maxUserAgentBytes
		for cut > 0 && !utf8.RuneStart(ua[cut]) {
			cut--
		}
		ua = ua[:cut]
	}
	return RequestMeta{IP: ip, UserAgent: ua}
}
