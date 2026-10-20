package base

import "unicode/utf8"

// MaxUserAgentBytes adalah batas panjang User-Agent yang disimpan.
const MaxUserAgentBytes = 512

// RequestMeta adalah metadata request yang diteruskan handler ke usecase.
type RequestMeta struct{ IP, UserAgent string }

// TruncateUA memotong User-Agent ke MaxUserAgentBytes tanpa memecah rune UTF-8.
func TruncateUA(s string) string {
	if len(s) <= MaxUserAgentBytes {
		return s
	}
	s = s[:MaxUserAgentBytes]
	for len(s) > 0 && !utf8.ValidString(s) {
		s = s[:len(s)-1]
	}
	return s
}
