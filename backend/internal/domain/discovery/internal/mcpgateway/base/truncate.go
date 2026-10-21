package base

import (
	"fmt"
	"unicode/utf8"
)

// TruncationMarker penanda yang ditambahkan pada teks yang dipotong.
func TruncationMarker(limit, original int) string {
	return fmt.Sprintf("\n\n[truncated: respons dipotong pada %d byte dari %d byte. Persempit permintaan dengan argumen offset/limit.]", limit, original)
}

// Truncate memotong text agar len(out) <= max pada batas rune UTF-8, ditambah penanda.
func Truncate(text string, max int) (out string, truncated bool, original int) {
	original = len(text)
	if original <= max {
		return text, false, original
	}
	// Panjang penanda bergantung pada angka N; iterasi sampai stabil.
	keep := max - len(TruncationMarker(max, original))
	for i := 0; i < 4; i++ {
		if keep < 0 {
			keep = 0
		}
		need := len(TruncationMarker(keep, original))
		if keep+need <= max {
			break
		}
		keep = max - need
	}
	if keep < 0 {
		keep = 0
	}
	for keep > 0 && !utf8.RuneStart(text[keep]) {
		keep--
	}
	marker := TruncationMarker(keep, original)
	if keep+len(marker) > max {
		// max terlalu kecil untuk penanda: potong penanda itu sendiri secara aman.
		m := marker
		if max < 0 {
			max = 0
		}
		if len(m) > max {
			m = m[:max]
			for len(m) > 0 && !utf8.ValidString(m) {
				m = m[:len(m)-1]
			}
		}
		return m, true, original
	}
	return text[:keep] + marker, true, original
}
