// Package secret menyediakan tipe string tersamar agar rahasia (mis. PAT) tidak
// bocor lewat fmt, slog, atau JSON. Satu-satunya jalan ke plaintext adalah Reveal.
package secret

import (
	"fmt"
	"log/slog"
	"unicode/utf8"
)

const redacted = "[REDACTED]"

// String menyimpan rahasia; field tidak diekspor.
type String struct{ v string }

// New membungkus s.
func New(s string) String { return String{v: s} }

// Reveal mengembalikan plaintext. Pakai hanya saat benar-benar perlu (header, enkripsi).
func (s String) Reveal() string { return s.v }

// IsZero true bila kosong.
func (s String) IsZero() bool { return s.v == "" }

// Last4 mengembalikan sampai 4 karakter (rune) terakhir.
func (s String) Last4() string {
	n := utf8.RuneCountInString(s.v)
	if n <= 4 {
		return s.v
	}
	skip := n - 4
	for i := range s.v {
		if skip == 0 {
			return s.v[i:]
		}
		skip--
	}
	return ""
}

// String implements fmt.Stringer.
func (s String) String() string { return redacted }

// GoString implements fmt.GoStringer.
func (s String) GoString() string { return redacted }

// LogValue implements slog.LogValuer.
func (s String) LogValue() slog.Value { return slog.StringValue(redacted) }

// MarshalJSON selalu menulis penanda tersamar.
func (s String) MarshalJSON() ([]byte, error) { return []byte(`"` + redacted + `"`), nil }

// Format menulis penanda tersamar untuk semua verb (termasuk %q, %x, %#v).
func (s String) Format(f fmt.State, verb rune) { _, _ = f.Write([]byte(redacted)) }
