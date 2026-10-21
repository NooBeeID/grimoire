package base

import (
	"bytes"
	"encoding/json"
	"unicode/utf8"
)

// DecodeArgs: kosong/null -> {}; object -> map; selain itu {"_invalid": true}.
func DecodeArgs(raw json.RawMessage) map[string]any {
	t := bytes.TrimSpace(raw)
	if len(t) == 0 || bytes.Equal(t, []byte("null")) {
		return map[string]any{}
	}
	var m map[string]any
	if t[0] != '{' || json.Unmarshal(t, &m) != nil || m == nil {
		return map[string]any{"_invalid": true}
	}
	return m
}

// TruncateName memotong name maksimal max byte pada batas rune.
func TruncateName(name string, max int) string {
	if len(name) <= max {
		return name
	}
	n := max
	for n > 0 && !utf8.RuneStart(name[n]) {
		n--
	}
	return name[:n]
}
