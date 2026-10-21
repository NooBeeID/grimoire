package base

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strconv"
	"strings"
	"unicode/utf8"

	"grimoire/lib/contracts/discovery/mcpgateway/toolkit"
)

// ArgError adalah kegagalan validasi argumen yang dikembalikan ke LLM sebagai isError.
type ArgError struct {
	Field  string // nama argumen; "arguments" untuk masalah bentuk object
	Reason string // mis. "harus 1-300", "wajib diisi"
	Got    string // nilai ringkas; "" berarti bagian (diterima: ...) dihilangkan
	Hint   string // saran langkah berikut; boleh kosong
	Custom string // bila diisi, Message() mengembalikan ini apa adanya
}

// Message memformat pesan sesuai K7.
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

// ArgResult mengubah ArgError menjadi hasil tool isError (bus tidak dipanggil).
func ArgResult(e *ArgError, targetType string) toolkit.Result {
	return toolkit.Result{Text: e.Message(), IsError: true, TargetType: targetType, Summary: e.Summary()}
}

// DecodeStrict mendekode object JSON ke dst dengan menolak field tak dikenal dan token sisa.
// allowed kosong berarti tool tanpa argumen.
func DecodeStrict(raw json.RawMessage, dst any, allowed []string) *ArgError {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		trimmed = []byte("{}")
	}
	if trimmed[0] != '{' {
		if !json.Valid(trimmed) {
			return &ArgError{Field: "arguments", Reason: "bukan JSON valid"}
		}
		return &ArgError{Field: "arguments", Reason: "harus object JSON", Got: Brief(string(trimmed))}
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
		if len(allowed) == 0 {
			return &ArgError{Field: name, Custom: "Argumen tidak valid: " + name + " tidak dikenal. Tool ini tidak menerima argumen."}
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

// IntArg memvalidasi argumen integer opsional. nil -> def; di luar [min,max] -> ArgError.
func IntArg(field string, v *int, def, min, max int, hint string) (int, *ArgError) {
	if v == nil {
		return def, nil
	}
	if *v < min || *v > max {
		return 0, &ArgError{Field: field, Reason: fmt.Sprintf("harus %d-%d", min, max), Got: strconv.Itoa(*v), Hint: hint}
	}
	return *v, nil
}

// StrArg memvalidasi argumen string opsional: nil -> ""; TrimSpace; tanpa karakter kontrol;
// maksimal maxRunes rune; harus UTF-8 valid.
func StrArg(field string, v *string, maxRunes int, hint string) (string, *ArgError) {
	if v == nil {
		return "", nil
	}
	s := strings.TrimSpace(*v)
	if !utf8.ValidString(s) {
		return "", &ArgError{Field: field, Reason: "bukan UTF-8 valid", Hint: hint}
	}
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return "", &ArgError{Field: field, Reason: "memuat karakter kontrol", Got: Brief(s), Hint: hint}
		}
	}
	if utf8.RuneCountInString(s) > maxRunes {
		return "", &ArgError{Field: field, Reason: fmt.Sprintf("maksimal %d karakter", maxRunes), Got: Brief(s), Hint: hint}
	}
	return s, nil
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
