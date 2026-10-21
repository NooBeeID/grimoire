package listendpoints

import (
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"grimoire/lib/contracts/project/apispec/listendpoints"
)

// Filter adalah parameter query yang sudah dinormalkan dan aman.
type Filter struct {
	ProjectID string
	Spec      string // "" = semua spec
	Tag       string // "" = tanpa filter
	Pattern   string // "" = tanpa filter; selain itu "%<escaped>%"
	Limit     int    // 1..MaxLimit
	Offset    int    // >= 0
}

// normalize: clamp Limit/Offset; sanitize Spec/Tag/Query; potong Query ke
// MaxQueryRunes rune; bentuk Pattern bila Query tidak kosong.
func normalize(req listendpoints.Request) Filter {
	f := Filter{
		ProjectID: req.ProjectID,
		Spec:      sanitize(req.Spec),
		Tag:       sanitize(req.Tag),
		Limit:     req.Limit,
		Offset:    req.Offset,
	}
	if f.Limit <= 0 {
		f.Limit = listendpoints.DefaultLimit
	}
	if f.Limit > listendpoints.MaxLimit {
		f.Limit = listendpoints.MaxLimit
	}
	if f.Offset < 0 {
		f.Offset = 0
	}
	q := sanitize(req.Query)
	if utf8.RuneCountInString(q) > listendpoints.MaxQueryRunes {
		q = string([]rune(q)[:listendpoints.MaxQueryRunes])
	}
	if q != "" {
		f.Pattern = "%" + escapeLike(q) + "%"
	}
	return f
}

// sanitize membuang UTF-8 tidak valid dan NUL, lalu trim spasi.
func sanitize(s string) string {
	s = strings.ToValidUTF8(s, "")
	s = strings.ReplaceAll(s, "\x00", "")
	return strings.TrimSpace(s)
}

// escapeLike meng-escape karakter khusus LIKE; backslash lebih dulu.
func escapeLike(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	return strings.ReplaceAll(s, `_`, `\_`)
}

// validProjectID hanya menerima bentuk kanonik uuid 36 karakter.
func validProjectID(s string) bool {
	if len(s) != 36 {
		return false
	}
	_, err := uuid.Parse(s)
	return err == nil
}
