package base

import (
	"sort"
	"strings"

	"grimoire/lib/common/errs"
)

const (
	ReasonAlreadyMember        = "ALREADY_MEMBER"
	ReasonConfirmationMismatch = "CONFIRMATION_MISMATCH"
	ReasonUserNotFound         = "USER_NOT_FOUND"
	ReasonUserNotDeveloper     = "USER_NOT_DEVELOPER"
	ReasonUserInactive         = "USER_INACTIVE"
	ReasonLastOwner            = "LAST_OWNER"
	ReasonInvitationNotPending = "INVITATION_NOT_PENDING"
)

var (
	ErrProjectNotFound = errs.New(errs.CodeNotFound, "project tidak ditemukan")
	ErrMemberNotFound  = errs.New(errs.CodeNotFound, "anggota tidak ditemukan")
	ErrForbidden       = errs.New(errs.CodeForbidden, "akses ditolak")
	ErrLastOwner       = errs.WithDetails(errs.CodeConflict,
		"owner terakhir tidak bisa keluar atau diturunkan; angkat owner lain lebih dulu",
		map[string]any{"reason": ReasonLastOwner})
	ErrAlreadyMember = errs.WithDetails(errs.CodeConflict, "user sudah menjadi anggota",
		map[string]any{"reason": ReasonAlreadyMember})
	ErrInvitationNotPending = errs.WithDetails(errs.CodeConflict, "undangan sudah diterima atau tidak ada",
		map[string]any{"reason": ReasonInvitationNotPending})
	ErrConfirmationMismatch = errs.WithDetails(errs.CodeInvalid, "nama konfirmasi tidak cocok",
		map[string]any{"reason": ReasonConfirmationMismatch})
)

// UserNotEligible adalah 400 INVALID_ARGUMENT dengan details.reason.
func UserNotEligible(reason string) *errs.Error {
	var msg string
	switch reason {
	case ReasonUserNotFound:
		msg = "user tidak ditemukan"
	case ReasonUserNotDeveloper:
		msg = "hanya developer yang bisa ditambahkan sebagai anggota; Super Admin tidak perlu menjadi anggota"
	case ReasonUserInactive:
		msg = "user nonaktif tidak bisa ditambahkan sebagai anggota"
	default:
		msg = "user tidak memenuhi syarat"
	}
	return errs.WithDetails(errs.CodeInvalid, msg, map[string]any{"reason": reason})
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
