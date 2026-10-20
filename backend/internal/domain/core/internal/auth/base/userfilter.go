package base

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	DefaultUserLimit = 20
	MaxUserLimit     = 100
	MaxUserQueryLen  = 100 // rune
	MaxUserOffset    = 1_000_000
	StatusActive     = "active"
	StatusInactive   = "inactive"
)

// UserFilter adalah filter daftar user yang sudah tervalidasi dan dinormalisasi.
type UserFilter struct {
	Role     string // "" = semua
	IsActive *bool  // nil = semua
	Query    string // sudah di-trim; "" = tanpa pencarian
	Limit    int    // 1..100
	Offset   int    // >= 0
}

// UserFilterInput adalah nilai mentah dari transport. Limit/Offset nil = tidak dikirim.
type UserFilterInput struct {
	Role, Status, Query string
	Limit, Offset       *int
}

// NewUserFilter memvalidasi semua field sekaligus (FieldErrors gabungan sebagai error),
// lalu menormalisasi.
func NewUserFilter(in UserFilterInput) (UserFilter, error) {
	fe := FieldErrors{}
	f := UserFilter{Limit: DefaultUserLimit}

	f.Role = strings.TrimSpace(in.Role)
	if f.Role != "" && !ValidRole(f.Role) {
		fe["role"] = msgRole
	}

	switch strings.TrimSpace(in.Status) {
	case "":
	case StatusActive:
		v := true
		f.IsActive = &v
	case StatusInactive:
		v := false
		f.IsActive = &v
	default:
		fe["status"] = "harus active atau inactive"
	}

	f.Query = strings.TrimSpace(in.Query)
	switch {
	case utf8.RuneCountInString(f.Query) > MaxUserQueryLen:
		fe["q"] = "maksimal 100 karakter"
	case strings.IndexFunc(f.Query, unicode.IsControl) >= 0:
		fe["q"] = "mengandung karakter tidak valid"
	}

	if in.Limit != nil {
		switch {
		case *in.Limit < 1:
			fe["limit"] = "minimal 1"
		case *in.Limit > MaxUserLimit:
			fe["limit"] = "maksimal 100"
		default:
			f.Limit = *in.Limit
		}
	}
	if in.Offset != nil {
		switch {
		case *in.Offset < 0:
			fe["offset"] = "minimal 0"
		case *in.Offset > MaxUserOffset:
			fe["offset"] = "terlalu besar"
		default:
			f.Offset = *in.Offset
		}
	}

	if len(fe) > 0 {
		return UserFilter{}, fe
	}
	return f, nil
}

// LikePattern mengembalikan pola ILIKE ter-escape; "" bila Query kosong.
func (f UserFilter) LikePattern() string {
	if f.Query == "" {
		return ""
	}
	return LikePattern(f.Query)
}
