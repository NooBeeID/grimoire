package base

import (
	"strings"
	"unicode/utf8"
)

const MaxPathBytes = 1024

// NormalizePath untuk read_file: TrimSpace, buang satu "/" di depan. Kosong atau folder ditolak.
func NormalizePath(field, raw string) (string, *ArgError) {
	p := strings.TrimSpace(raw)
	p = strings.TrimPrefix(p, "/")
	if p == "" {
		return "", &ArgError{Field: field, Reason: "wajib diisi"}
	}
	if strings.HasSuffix(p, "/") {
		return "", &ArgError{Field: field, Reason: "menunjuk folder", Got: Brief(raw),
			Hint: "Gunakan list_files dengan path=<tanpa slash akhir>."}
	}
	if err := checkPath(field, p); err != nil {
		return "", err
	}
	return p, nil
}

// NormalizePrefix untuk list_files/search_knowledge: buang satu "/" di depan dan satu di belakang. Kosong = akar.
func NormalizePrefix(field, raw string) (string, *ArgError) {
	p := strings.TrimSpace(raw)
	p = strings.TrimPrefix(p, "/")
	p = strings.TrimSuffix(p, "/")
	if p == "" {
		return "", nil
	}
	if err := checkPath(field, p); err != nil {
		return "", err
	}
	return p, nil
}

func checkPath(field, p string) *ArgError {
	if len(p) > MaxPathBytes {
		return &ArgError{Field: field, Reason: "maksimal 1024 byte"}
	}
	if !utf8.ValidString(p) {
		return &ArgError{Field: field, Reason: "bukan UTF-8 valid"}
	}
	for _, r := range p {
		if r < 0x20 || r == 0x7f {
			return &ArgError{Field: field, Reason: "memuat karakter kontrol", Got: Brief(p)}
		}
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == ".." {
			return &ArgError{Field: field, Reason: "tidak boleh memuat segmen ..", Got: Brief(p),
				Hint: "Gunakan path relatif dari akar knowledge, mis. docs/guides/a.md."}
		}
	}
	return nil
}
