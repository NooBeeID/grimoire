package base

import "unicode/utf8"

const DefaultReadLimit = 65536

// SliceUTF8 memotong content berdasarkan BYTE pada batas rune. offset < 0 atau offset > len(content) ->
// invalid_offset. limit <= 0 dianggap DefaultReadLimit. start maju ke awal rune berikutnya bila offset di
// tengah rune; end mundur ke batas rune, tetapi selalu maju minimal satu rune bila start < len.
// offset == len -> part "", truncated=false.
func SliceUTF8(content string, offset, limit int) (part string, start, end int, truncated bool, reason Reason) {
	n := len(content)
	if offset < 0 || offset > n {
		return "", 0, 0, false, ReasonInvalidOffset
	}
	if limit <= 0 {
		limit = DefaultReadLimit
	}
	start = offset
	for start < n && !utf8.RuneStart(content[start]) {
		start++
	}
	if start >= n {
		return "", n, n, false, ""
	}
	if limit >= n-start {
		end = n
	} else {
		end = start + limit
		for end > start && !utf8.RuneStart(content[end]) {
			end--
		}
		if end == start {
			_, size := utf8.DecodeRuneInString(content[start:])
			end = start + size
		}
	}
	return content[start:end], start, end, end < n, ""
}
