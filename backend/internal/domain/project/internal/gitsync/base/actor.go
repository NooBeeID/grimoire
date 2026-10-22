package base

import (
	"strings"
	"time"
	"unicode/utf8"

	"grimoire/internal/infra/gitprovider"
	"grimoire/lib/common/reqctx"
)

const maxUserAgentBytes = 512

// Actor adalah pelaku request. Label = Name (trim) bila ada, selain itu Email.
type Actor struct{ ID, Role, Label string }

// ActorFrom membangun Actor; ok=false atau UserID kosong -> false.
func ActorFrom(id reqctx.Identity, ok bool) (Actor, bool) {
	if !ok || id.UserID == "" {
		return Actor{}, false
	}
	label := strings.TrimSpace(id.Name)
	if label == "" {
		label = id.Email
	}
	return Actor{ID: id.UserID, Role: id.Role, Label: label}, true
}

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

// Clock memberi waktu sekarang (seam tes).
type Clock func() time.Time

// SystemClock memakai waktu sistem.
var SystemClock Clock = time.Now

// Shared dibangun sekali di gitsync/base.go dan diteruskan ke Init tiap feature.
type Shared struct {
	Git   *gitprovider.Client // tree + blob; Policy() untuk ParseRepoURL
	Clock Clock
}
