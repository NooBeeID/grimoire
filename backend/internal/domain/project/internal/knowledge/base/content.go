package base

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"unicode/utf8"
)

const (
	titleScanBytes = 4096
	maxTitleRunes  = 255
)

var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// PrepareContent membuang BOM UTF-8, lalu memvalidasi: UTF-8 valid tanpa NUL
// (invalid_encoding) dan len <= maxBytes setelah BOM dibuang (too_large). Isi kosong valid.
func PrepareContent(raw []byte, maxBytes int64) (string, Reason) {
	raw = bytes.TrimPrefix(raw, utf8BOM)
	if !utf8.Valid(raw) || bytes.IndexByte(raw, 0) >= 0 {
		return "", ReasonInvalidEncoding
	}
	if int64(len(raw)) > maxBytes {
		return "", ReasonTooLarge
	}
	return string(raw), ""
}

// Hash mengembalikan sha256 isi (32 byte).
func Hash(content string) []byte {
	h := sha256.Sum256([]byte(content))
	return h[:]
}

// Version mengembalikan hex huruf kecil (64 karakter) dari hash.
func Version(hash []byte) string { return hex.EncodeToString(hash) }

// ValidVersion: tepat 64 karakter [0-9a-f].
func ValidVersion(v string) bool {
	if len(v) != 64 {
		return false
	}
	for i := 0; i < len(v); i++ {
		c := v[i]
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

// Title mengambil judul dari heading ATX "# judul" pertama di 4 KiB pertama.
// Baris di dalam blok kode ``` diabaikan. Tidak ada -> "".
func Title(content string) string {
	if len(content) > titleScanBytes {
		cut := titleScanBytes
		for cut > 0 && !utf8.RuneStart(content[cut]) {
			cut--
		}
		content = content[:cut]
	}
	inFence := false
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSuffix(line, "\r")
		if strings.HasPrefix(strings.TrimLeft(line, " \t"), "```") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		if !strings.HasPrefix(line, "# ") && !strings.HasPrefix(line, "#\t") {
			continue
		}
		return cleanTitle(line[1:])
	}
	return ""
}

func cleanTitle(s string) string {
	s = strings.TrimSpace(s)
	// "#" penutup opsional hanya bila didahului spasi (atau seluruh isi adalah "#").
	if t := strings.TrimRight(s, "#"); t != s && (t == "" || strings.HasSuffix(t, " ") || strings.HasSuffix(t, "\t")) {
		s = strings.TrimSpace(t)
	}
	if utf8.RuneCountInString(s) > maxTitleRunes {
		s = string([]rune(s)[:maxTitleRunes])
	}
	return s
}
