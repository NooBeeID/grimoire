package base

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	MaxPathRunes    = 512
	MaxSegmentRunes = 255
)

// NormalizePath memvalidasi path file. Path dipakai apa adanya (tidak di-trim, tidak di-lowercase);
// yang tidak lolos ditolak dengan alasan, bukan diperbaiki diam-diam.
func NormalizePath(raw string) (string, Reason) {
	if raw == "" {
		return "", ReasonPathEmpty
	}
	if !utf8.ValidString(raw) {
		return "", ReasonInvalidEncoding
	}
	for _, r := range raw {
		if r == 0 || unicode.IsControl(r) {
			return "", ReasonPathControlChar
		}
	}
	if strings.HasPrefix(raw, "/") {
		return "", ReasonPathAbsolute
	}
	if utf8.RuneCountInString(raw) > MaxPathRunes {
		return "", ReasonPathTooLong
	}
	for _, seg := range strings.Split(raw, "/") {
		switch {
		case seg == "":
			return "", ReasonPathEmptySegment
		case seg == "." || seg == "..":
			return "", ReasonPathTraversal
		case utf8.RuneCountInString(seg) > MaxSegmentRunes:
			return "", ReasonSegmentTooLong
		}
	}
	return raw, ""
}

// NormalizePrefix untuk folder: "" adalah root. Satu "/" di akhir dibuang ("docs/" -> "docs").
// Sisanya divalidasi seperti NormalizePath.
func NormalizePrefix(raw string) (string, Reason) {
	if raw == "" {
		return "", ""
	}
	trimmed := strings.TrimSuffix(raw, "/")
	if trimmed == "" {
		return "", ReasonPathAbsolute // raw == "/"
	}
	return NormalizePath(trimmed)
}

// StripLeadingSlash membuang tepat SATU "/" di depan (dipakai handler bus untuk agent).
func StripLeadingSlash(s string) string { return strings.TrimPrefix(s, "/") }

// IsHidden: true bila ada segmen yang diawali ".".
func IsHidden(p string) bool {
	for _, seg := range strings.Split(p, "/") {
		if strings.HasPrefix(seg, ".") {
			return true
		}
	}
	return false
}

// Ext mengembalikan ekstensi segmen terakhir dalam huruf kecil; tanpa titik -> "".
func Ext(p string) string {
	b := Base(p)
	i := strings.LastIndex(b, ".")
	if i < 0 {
		return ""
	}
	return strings.ToLower(b[i:])
}

// IsSupported: ekstensi .md, .markdown, atau .txt.
func IsSupported(p string) bool {
	switch Ext(p) {
	case ".md", ".markdown", ".txt":
		return true
	}
	return false
}

// Base mengembalikan segmen terakhir.
func Base(p string) string {
	if i := strings.LastIndex(p, "/"); i >= 0 {
		return p[i+1:]
	}
	return p
}

// Dir mengembalikan semua kecuali segmen terakhir; "" untuk root.
func Dir(p string) string {
	if i := strings.LastIndex(p, "/"); i >= 0 {
		return p[:i]
	}
	return ""
}

var likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

// LikePrefix menghasilkan pola untuk LIKE ... ESCAPE '\'. "" -> "%" (seluruh project);
// "docs" -> "docs/%" sehingga "docs2/x.md" dan "docs.md" tidak cocok.
func LikePrefix(prefix string) string {
	if prefix == "" {
		return "%"
	}
	return likeEscaper.Replace(prefix) + "/%"
}
