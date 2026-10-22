package base

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

const maxPrefixRunes = 512

// NormalizeTargetPrefix: trim, buang "/" depan/belakang dan segmen kosong; tolak segmen ".", "..",
// NUL/kontrol, UTF-8 tidak valid, dan > 512 rune. Pemeriksaan otoritatif tetap di provider knowledge.
func NormalizeTargetPrefix(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if !utf8.ValidString(s) || utf8.RuneCountInString(s) > maxPrefixRunes {
		return "", ErrInvalidTargetPrefix
	}
	for _, r := range s {
		if r == 0 || unicode.IsControl(r) {
			return "", ErrInvalidTargetPrefix
		}
	}
	var segs []string
	for _, seg := range strings.Split(s, "/") {
		switch seg {
		case "":
			continue
		case ".", "..":
			return "", ErrInvalidTargetPrefix
		}
		segs = append(segs, seg)
	}
	return strings.Join(segs, "/"), nil
}
