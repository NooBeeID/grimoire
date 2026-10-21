package base

import "unicode/utf8"

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
