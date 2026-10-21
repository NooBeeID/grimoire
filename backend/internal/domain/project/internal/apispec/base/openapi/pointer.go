package openapi

import (
	"strconv"
	"strings"
)

// EscapePointerToken: "~" -> "~0", "/" -> "~1".
func EscapePointerToken(s string) string {
	s = strings.ReplaceAll(s, "~", "~0")
	return strings.ReplaceAll(s, "/", "~1")
}

func unescapePointerToken(s string) string {
	s = strings.ReplaceAll(s, "~1", "/")
	return strings.ReplaceAll(s, "~0", "~")
}

// percentDecode membuang %XX; urutan tidak valid dibiarkan apa adanya.
func percentDecode(s string) string {
	if !strings.Contains(s, "%") {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '%' && i+2 < len(s) {
			if n, err := strconv.ParseUint(s[i+1:i+3], 16, 8); err == nil {
				b.WriteByte(byte(n))
				i += 2
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// ResolvePointer menelusuri ref "#/a/b" dari root; "#" adalah root. Token di-unescape (~0, ~1, %XX);
// index array numerik tanpa tanda.
func ResolvePointer(root any, ref string) (any, bool) {
	if !strings.HasPrefix(ref, "#") {
		return nil, false
	}
	rest := ref[1:]
	if rest == "" {
		return root, true
	}
	if !strings.HasPrefix(rest, "/") {
		return nil, false
	}
	cur := root
	for _, tok := range strings.Split(rest[1:], "/") {
		tok = unescapePointerToken(percentDecode(tok))
		switch c := cur.(type) {
		case *Map:
			v, ok := c.Get(tok)
			if !ok {
				return nil, false
			}
			cur = v
		case []any:
			if tok == "" || len(tok) > 9 {
				return nil, false
			}
			for i := 0; i < len(tok); i++ {
				if tok[i] < '0' || tok[i] > '9' {
					return nil, false
				}
			}
			idx, _ := strconv.Atoi(tok)
			if idx >= len(c) {
				return nil, false
			}
			cur = c[idx]
		default:
			return nil, false
		}
	}
	return cur, true
}

func joinPtr(parent, token string) string { return parent + "/" + EscapePointerToken(token) }
