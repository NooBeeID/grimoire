package base

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"reflect"
	"strconv"
	"strings"
)

// ArgError adalah kegagalan validasi argumen yang dikembalikan ke LLM sebagai isError.
type ArgError struct {
	Field  string // nama argumen; "arguments" untuk masalah bentuk object
	Reason string // mis. "harus 1-500", "wajib diisi"
	Got    string // nilai ringkas; "" berarti bagian (diterima: ...) dihilangkan
	Hint   string // saran langkah berikut; boleh kosong
	Custom string // bila diisi, Message() mengembalikan ini apa adanya
}

// Message memformat pesan sesuai D6.
func (e *ArgError) Message() string {
	if e.Custom != "" {
		return e.Custom
	}
	var b strings.Builder
	b.WriteString("Argumen tidak valid: ")
	b.WriteString(e.Field)
	b.WriteString(" ")
	b.WriteString(e.Reason)
	if e.Got != "" {
		b.WriteString(" (diterima: ")
		b.WriteString(e.Got)
		b.WriteString(")")
	}
	b.WriteString(".")
	if e.Hint != "" {
		b.WriteString(" ")
		b.WriteString(e.Hint)
	}
	return b.String()
}

// Summary adalah ringkasan audit untuk argumen tidak valid.
func (e *ArgError) Summary() map[string]any {
	return map[string]any{"error_code": "invalid_argument", "field": e.Field}
}

// DecodeStrict mendekode object JSON ke dst dengan menolak field tak dikenal dan token sisa.
func DecodeStrict(raw json.RawMessage, dst any, allowed []string) *ArgError {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		trimmed = []byte("{}")
	}
	if trimmed[0] != '{' {
		if !json.Valid(trimmed) {
			return &ArgError{Field: "arguments", Reason: "bukan JSON valid"}
		}
		return &ArgError{Field: "arguments", Reason: "harus object JSON"}
	}
	dec := json.NewDecoder(bytes.NewReader(trimmed))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return mapDecodeError(err, allowed)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		if err == nil {
			return &ArgError{Field: "arguments", Reason: "harus satu object JSON"}
		}
		return &ArgError{Field: "arguments", Reason: "bukan JSON valid"}
	}
	return nil
}

func mapDecodeError(err error, allowed []string) *ArgError {
	var ute *json.UnmarshalTypeError
	if errors.As(err, &ute) {
		return &ArgError{Field: ute.Field, Reason: "harus " + typeName(ute.Type), Got: ute.Value}
	}
	const prefix = "json: unknown field "
	if msg := err.Error(); strings.HasPrefix(msg, prefix) {
		name, uerr := strconv.Unquote(strings.TrimPrefix(msg, prefix))
		if uerr != nil {
			name = strings.Trim(strings.TrimPrefix(msg, prefix), `"`)
		}
		for _, a := range ProjectArgAliases {
			if a == name {
				return &ArgError{Field: name, Custom: "Argumen " + name + " tidak didukung: project ditentukan oleh API key. Hapus argumen ini lalu panggil ulang."}
			}
		}
		return &ArgError{Field: name, Custom: "Argumen tidak valid: " + name + " tidak dikenal. Argumen yang didukung: " + strings.Join(allowed, ", ") + "."}
	}
	return &ArgError{Field: "arguments", Reason: "bukan JSON valid"}
}

func typeName(t reflect.Type) string {
	if t == nil {
		return "tipe lain"
	}
	switch t.Kind() {
	case reflect.String:
		return "string"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return "integer"
	case reflect.Bool:
		return "boolean"
	}
	return t.String()
}

// IntArg memvalidasi argumen integer opsional. nil -> def.
func IntArg(field string, v *int, def, min, max int, hint string) (int, *ArgError) {
	if v == nil {
		return def, nil
	}
	if *v < min || *v > max {
		reason := fmt.Sprintf("harus %d-%d", min, max)
		if max == math.MaxInt32 {
			reason = fmt.Sprintf("harus >= %d", min)
		}
		return 0, &ArgError{Field: field, Reason: reason, Got: strconv.Itoa(*v), Hint: hint}
	}
	return *v, nil
}

// BoolArg: nil -> def.
func BoolArg(v *bool, def bool) bool {
	if v == nil {
		return def
	}
	return *v
}

// Brief meringkas nilai untuk Got: maks 40 rune, karakter kontrol sebagai \uXXXX, "..." bila terpotong.
func Brief(s string) string {
	const maxRunes = 40
	var b strings.Builder
	n := 0
	for _, r := range s {
		if n == maxRunes {
			b.WriteString("...")
			break
		}
		if r < 0x20 || r == 0x7f {
			fmt.Fprintf(&b, `\u%04x`, r)
		} else {
			b.WriteRune(r)
		}
		n++
	}
	return b.String()
}
