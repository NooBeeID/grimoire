package base

import (
	"path"
	"strings"
	"unicode/utf8"
)

const (
	NameMaxLen       = 80
	SourcePathMaxLen = 512
)

func nameByte(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '.' || c == '_' || c == '-'
}

// ValidateName: ^[a-z0-9._-]{1,80}$ dan bukan "." / "..".
func ValidateName(s string) error {
	if len(s) < 1 || len(s) > NameMaxLen || s == "." || s == ".." {
		return ErrInvalidName
	}
	for i := 0; i < len(s); i++ {
		if !nameByte(s[i]) {
			return ErrInvalidName
		}
	}
	return nil
}

// DeriveName menurunkan nama spec dari nama file: basename, tanpa ekstensi terakhir, lowercase,
// karakter di luar [a-z0-9._-] (satu run) menjadi "-", trim "-" dan ".", potong 80 byte lalu trim lagi.
func DeriveName(fileName string) (string, error) {
	f := strings.ReplaceAll(fileName, "\\", "/")
	f = path.Base(f)
	if ext := path.Ext(f); ext != "" {
		f = strings.TrimSuffix(f, ext)
	}
	f = strings.ToLower(f)
	var b strings.Builder
	prevDash := false
	for i := 0; i < len(f); i++ {
		if nameByte(f[i]) {
			b.WriteByte(f[i])
			prevDash = false
			continue
		}
		if !prevDash {
			b.WriteByte('-')
			prevDash = true
		}
	}
	n := strings.Trim(b.String(), "-.")
	if len(n) > NameMaxLen {
		n = strings.Trim(n[:NameMaxLen], "-.")
	}
	if err := ValidateName(n); err != nil {
		return "", err
	}
	return n, nil
}

// NormalizeSourcePath: "\" -> "/", buang "./" di depan, gabungkan "/" ganda. Tolak (ErrInvalidPath):
// kosong, diawali "/", huruf drive, segmen "..", karakter kontrol/NUL, > 512 byte, bukan UTF-8 valid.
func NormalizeSourcePath(raw string) (string, error) {
	if raw == "" || len(raw) > SourcePathMaxLen || !utf8.ValidString(raw) {
		return "", ErrInvalidPath
	}
	for _, r := range raw {
		if r < 0x20 || r == 0x7f {
			return "", ErrInvalidPath
		}
	}
	p := strings.ReplaceAll(raw, "\\", "/")
	for strings.HasPrefix(p, "./") {
		p = p[2:]
	}
	for strings.Contains(p, "//") {
		p = strings.ReplaceAll(p, "//", "/")
	}
	if p == "" || strings.HasPrefix(p, "/") {
		return "", ErrInvalidPath
	}
	if len(p) >= 2 && p[1] == ':' && ((p[0] >= 'a' && p[0] <= 'z') || (p[0] >= 'A' && p[0] <= 'Z')) {
		return "", ErrInvalidPath
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == ".." {
			return "", ErrInvalidPath
		}
	}
	return p, nil
}

// FormatFromExt (case-insensitive): .yaml/.yml -> yaml; .json -> json; lainnya ok=false.
func FormatFromExt(fileName string) (format string, ok bool) {
	switch strings.ToLower(path.Ext(strings.ReplaceAll(fileName, "\\", "/"))) {
	case ".yaml", ".yml":
		return FormatYAML, true
	case ".json":
		return FormatJSON, true
	}
	return "", false
}
