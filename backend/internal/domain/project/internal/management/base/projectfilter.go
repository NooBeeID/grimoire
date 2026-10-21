package base

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	DefaultProjectLimit  = 20
	MaxProjectLimit      = 100
	MaxProjectQueryRunes = 100
	MaxProjectOffset     = 1_000_000
)

// ProjectFilter adalah filter daftar project yang sudah tervalidasi dan dinormalisasi.
type ProjectFilter struct {
	Query  string
	Limit  int
	Offset int
}

// ProjectFilterInput adalah nilai mentah dari transport. Limit/Offset nil = tidak dikirim.
type ProjectFilterInput struct {
	Query         string
	Limit, Offset *int
}

// NewProjectFilter memvalidasi semua field sekaligus; error bertipe FieldErrors.
func NewProjectFilter(in ProjectFilterInput) (ProjectFilter, error) {
	fe := FieldErrors{}
	f := ProjectFilter{Query: strings.TrimSpace(in.Query), Limit: DefaultProjectLimit}
	if utf8.RuneCountInString(f.Query) > MaxProjectQueryRunes {
		fe["q"] = "maksimal 100 karakter"
	} else {
		for _, r := range f.Query {
			if unicode.IsControl(r) {
				fe["q"] = "mengandung karakter tidak valid"
				break
			}
		}
	}
	if in.Limit != nil {
		if *in.Limit < 1 || *in.Limit > MaxProjectLimit {
			fe["limit"] = "limit harus 1-100"
		} else {
			f.Limit = *in.Limit
		}
	}
	if in.Offset != nil {
		switch {
		case *in.Offset < 0:
			fe["offset"] = "offset minimal 0"
		case *in.Offset > MaxProjectOffset:
			fe["offset"] = "offset terlalu besar"
		default:
			f.Offset = *in.Offset
		}
	}
	if len(fe) > 0 {
		return ProjectFilter{}, fe
	}
	return f, nil
}

// LikePattern: "" bila Query kosong; selain itu pola ILIKE ter-escape.
func (f ProjectFilter) LikePattern() string {
	if f.Query == "" {
		return ""
	}
	return LikePattern(f.Query)
}
