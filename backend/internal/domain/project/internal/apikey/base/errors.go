package base

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"grimoire/lib/common/errs"
)

const (
	ReasonNameTaken       = "NAME_TAKEN"
	ReasonKeyLimitReached = "KEY_LIMIT_REACHED"
	ReasonAlreadyRevoked  = "ALREADY_REVOKED"
	ReasonKeyNotFound     = "KEY_NOT_FOUND"
)

// Sentinel dipakai bersama; usecase tidak boleh memodifikasinya.
var (
	ErrProjectNotFound = errs.New(errs.CodeNotFound, "project tidak ditemukan")
	ErrKeyNotFound     = errs.WithDetails(errs.CodeNotFound, "API key tidak ditemukan",
		map[string]any{"reason": ReasonKeyNotFound})
	ErrNameTaken = errs.WithDetails(errs.CodeConflict, "nama key sudah dipakai key aktif Anda yang lain di project ini",
		map[string]any{"reason": ReasonNameTaken})
	ErrAlreadyRevoked = errs.WithDetails(errs.CodeConflict, "API key sudah dicabut",
		map[string]any{"reason": ReasonAlreadyRevoked})

	// ErrPrefixCollision adalah sentinel internal (bukan *errs.Error): repository -> usecase createkey untuk retry.
	ErrPrefixCollision = errors.New("apikey: prefix collision")
)

// ErrKeyLimitReached adalah 409 dengan details {reason, limit}. Instance baru tiap panggilan.
func ErrKeyLimitReached(limit int) *errs.Error {
	return errs.WithDetails(errs.CodeConflict,
		fmt.Sprintf("batas %d key aktif per user per project tercapai; cabut key yang tidak dipakai", limit),
		map[string]any{"reason": ReasonKeyLimitReached, "limit": limit})
}

// FieldErrors memetakan nama field ke pesan kesalahan.
type FieldErrors map[string]string

func (fe FieldErrors) Error() string {
	keys := make([]string, 0, len(fe))
	for k := range fe {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = k + ": " + fe[k]
	}
	return "validasi gagal: " + strings.Join(parts, "; ")
}

// Merge menggabungkan other ke fe (key yang sudah ada dipertahankan). Aman pada fe nil.
func (fe FieldErrors) Merge(other FieldErrors) FieldErrors {
	if len(other) == 0 {
		return fe
	}
	if fe == nil {
		fe = FieldErrors{}
	}
	for k, v := range other {
		if _, ok := fe[k]; !ok {
			fe[k] = v
		}
	}
	return fe
}

// InvalidFields mengubah FieldErrors menjadi 400 INVALID_ARGUMENT dengan details.fields.
func InvalidFields(fe FieldErrors) *errs.Error {
	return errs.WithDetails(errs.CodeInvalid, "permintaan tidak valid", map[string]any{"fields": fe})
}
