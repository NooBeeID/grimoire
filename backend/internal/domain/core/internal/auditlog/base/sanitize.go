package base

import (
	"encoding/json"
	"sort"
	"strings"
	"unicode/utf8"
)

// Batas dan penanda sanitasi args/result.
const (
	MaxStringBytes  = 2048
	MaxJSONBytes    = 8192
	MaxDepth        = 5
	MaxSummaryKeys  = 50
	Redacted        = "[REDACTED]"
	Omitted         = "[OMITTED]"
	TooDeep         = "[TOO_DEEP]"
	TruncatedSuffix = "…[truncated]"
)

var sensitiveKeys = []string{"password", "token", "authorization", "api_key", "apikey", "secret"}

func normalizeKey(k string) string {
	return strings.ReplaceAll(strings.ToLower(k), "-", "_")
}

func isSensitiveKey(k string) bool {
	n := normalizeKey(k)
	for _, s := range sensitiveKeys {
		if strings.Contains(n, s) {
			return true
		}
	}
	return false
}

// TruncateUTF8 memotong s hingga paling banyak max byte tanpa memecah rune.
func TruncateUTF8(s string, max int) string {
	if max <= 0 {
		return ""
	}
	if len(s) <= max {
		return s
	}
	n := max
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}

// Sanitize menyalin v dan menutup nilai rahasia, menghilangkan isi file,
// memotong string panjang, membatasi kedalaman dan ukuran total JSON.
// Input tidak pernah dimutasi; nil menghasilkan map kosong.
func Sanitize(v map[string]any) map[string]any {
	out := sanitizeMap(v, 1)
	b, err := json.Marshal(out)
	if err != nil || len(b) <= MaxJSONBytes {
		if err != nil {
			return map[string]any{"_truncated": true}
		}
		return out
	}
	keys := make([]string, 0, len(out))
	for k := range out {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if len(keys) > MaxSummaryKeys {
		keys = keys[:MaxSummaryKeys]
	}
	return map[string]any{
		"_truncated":      true,
		"_original_bytes": len(b),
		"_keys":           keys,
	}
}

func sanitizeMap(m map[string]any, depth int) map[string]any {
	out := make(map[string]any, len(m))
	for k, val := range m {
		switch {
		case isSensitiveKey(k):
			out[k] = Redacted
		case k == "content" || k == "body":
			out[k] = Omitted
		default:
			out[k] = sanitizeValue(val, depth+1)
		}
	}
	return out
}

// sanitizeValue memproses nilai pada kedalaman depth (container di atas depth MaxDepth ditolak).
func sanitizeValue(v any, depth int) any {
	switch t := v.(type) {
	case nil, bool, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64, json.Number:
		return t
	case string:
		return truncateString(t)
	case map[string]any:
		if depth > MaxDepth {
			return TooDeep
		}
		return sanitizeMap(t, depth)
	case []any:
		if depth > MaxDepth {
			return TooDeep
		}
		out := make([]any, len(t))
		for i, x := range t {
			out[i] = sanitizeValue(x, depth+1)
		}
		return out
	default:
		b, err := json.Marshal(t)
		if err != nil {
			return Omitted
		}
		var generic any
		if err := json.Unmarshal(b, &generic); err != nil {
			return Omitted
		}
		return sanitizeValue(generic, depth)
	}
}

func truncateString(s string) string {
	if len(s) <= MaxStringBytes {
		return s
	}
	return TruncateUTF8(s, MaxStringBytes) + TruncatedSuffix
}
