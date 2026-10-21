package base

import (
	"errors"
	"sort"
	"strings"
	"time"

	"grimoire/lib/common/errs"
)

// Kode error milik module auth. Pemetaan status HTTP didaftarkan di auth/base.go
// (package base tidak boleh meng-import httpx).
const (
	CodeInvalidCredentials errs.Code = "INVALID_CREDENTIALS" // 401
	CodeAccountLocked      errs.Code = "ACCOUNT_LOCKED"      // 429
)

var (
	// ErrEmailTaken adalah sentinel repository untuk unique violation users_email_key.
	ErrEmailTaken = errors.New("auth: email sudah terdaftar")

	ErrUserNotFound   = errs.New(errs.CodeNotFound, "user tidak ditemukan")
	ErrInvalidCreds   = errs.New(CodeInvalidCredentials, "email atau password salah")
	ErrSelfAction     = errs.WithDetails(errs.CodeForbidden, "tidak boleh menonaktifkan atau menurunkan role akun sendiri", map[string]any{"reason": ReasonSelfAction})
	ErrLastSuperAdmin = errs.WithDetails(errs.CodeConflict, "minimal harus ada satu super admin aktif", map[string]any{"reason": ReasonLastSuperAdmin})
)

// Alasan dan scope error undangan (details.reason / details.scope).
const (
	ReasonInvitationNotPending    = "INVITATION_NOT_PENDING"
	AlreadyRegisteredRegistered   = "registered"
	AlreadyRegisteredInactive     = "inactive"
	AlreadyRegisteredNotDeveloper = "not_developer"

	ScopeInvite = "invite"
	ScopeResend = "resend"
)

// ErrInvitationNotPending: undangan sudah diterima atau tidak ada (409 CONFLICT).
var ErrInvitationNotPending = errs.WithDetails(errs.CodeConflict, "undangan sudah diterima atau tidak ada",
	map[string]any{"reason": ReasonInvitationNotPending})

// ErrAlreadyRegistered adalah 409 ALREADY_REGISTERED dengan details.reason.
func ErrAlreadyRegistered(reason string) *errs.Error {
	return errs.WithDetails(errs.CodeAlreadyRegistered, "email sudah terdaftar", map[string]any{"reason": reason})
}

// ErrInviteRateLimited adalah 429 dengan details.retry_after_seconds (dibulatkan ke atas,
// minimal 1) dan details.scope ("invite" atau "resend").
func ErrInviteRateLimited(retry time.Duration, scope string) *errs.Error {
	secs := int((retry + time.Second - 1) / time.Second)
	if secs < 1 {
		secs = 1
	}
	return errs.WithDetails(errs.CodeRateLimited, "terlalu banyak permintaan undangan",
		map[string]any{"retry_after_seconds": secs, "scope": scope})
}

// FieldErrors memetakan nama field ke pesan kesalahan. Nilai nil/kosong berarti lolos.
// Mengimplementasikan error; hanya dikembalikan sebagai error bila len > 0.
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

// Merge menggabungkan other ke fe dan mengembalikan hasilnya. Key yang sudah ada di fe
// dipertahankan (error pertama menang). Aman dipanggil pada fe nil.
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

// AsFieldErrors mengekstrak FieldErrors dari err (nil bila bukan FieldErrors).
func AsFieldErrors(err error) FieldErrors {
	var fe FieldErrors
	if errors.As(err, &fe) {
		return fe
	}
	return nil
}

// fieldErr membuat error satu field.
func fieldErr(field, msg string) error { return FieldErrors{field: msg} }

// InvalidFields mengubah FieldErrors menjadi 400 INVALID_ARGUMENT dengan details.fields.
func InvalidFields(fe FieldErrors) *errs.Error {
	return errs.WithDetails(errs.CodeInvalid, "permintaan tidak valid", map[string]any{"fields": fe})
}

// EmailConflict adalah 409 untuk email yang sudah terdaftar.
func EmailConflict() *errs.Error {
	return errs.New(errs.CodeConflict, "email sudah terdaftar")
}

// AccountLocked adalah 429 dengan details.retry_after_seconds (dibulatkan ke atas, minimal 1).
func AccountLocked(retry time.Duration) *errs.Error {
	secs := int((retry + time.Second - 1) / time.Second)
	if secs < 1 {
		secs = 1
	}
	return errs.WithDetails(CodeAccountLocked, "terlalu banyak percobaan login, coba lagi nanti",
		map[string]any{"retry_after_seconds": secs})
}
