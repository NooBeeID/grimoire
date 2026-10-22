package base

import "grimoire/lib/common/errs"

// CodeTokenInvalid: provider menolak token (422). Didaftarkan module di registerCodes.
const CodeTokenInvalid errs.Code = "TOKEN_INVALID"

// Sentinel dipakai bersama; usecase TIDAK boleh memodifikasinya. Pesan tidak memuat token.
var (
	ErrInvalidProvider = errs.WithDetails(errs.CodeInvalid, "provider tidak didukung",
		map[string]any{"fields": map[string]string{"provider": "provider harus github atau gitlab"}})
	ErrInvalidHost = errs.WithDetails(errs.CodeInvalid, "host tidak valid",
		map[string]any{"fields": map[string]string{"host": "host harus berbentuk nama[:port] tanpa path"}})
	ErrTokenFormat = errs.WithDetails(errs.CodeInvalid, "permintaan tidak valid",
		map[string]any{"fields": map[string]string{"token": "token wajib 8-512 karakter ASCII tanpa spasi"}})
	ErrHostNotAllowed = errs.New(errs.CodeHostNotAllowed,
		"host Git tidak diizinkan; minta admin menambahkannya ke GIT_SYNC_ALLOWED_HOSTS")
	ErrTokenInvalid       = errs.New(CodeTokenInvalid, "token tidak valid atau tidak punya akses")
	ErrFeatureDisabled    = errs.New(errs.CodeGitFeatureDisabled, "sync Git belum dikonfigurasi oleh admin")
	ErrUpstream           = errs.New(errs.CodeGitUpstream, "provider Git tidak dapat dihubungi, coba lagi")
	ErrRateLimited        = errs.New(errs.CodeRateLimited, "provider Git membatasi permintaan; coba lagi nanti")
	ErrUnauthenticated    = errs.New(errs.CodeUnauthorized, "perlu login")
	ErrCredentialNotFound = errs.New(errs.CodeNotFound, "token tidak ditemukan")
)

// HostNotAllowed menyertakan host di details (host bukan rahasia).
func HostNotAllowed(host string) *errs.Error {
	return errs.WithDetails(errs.CodeHostNotAllowed, ErrHostNotAllowed.Message, map[string]any{"host": host})
}

// TokenInvalid -> 422 TOKEN_INVALID dengan details {provider, host}.
func TokenInvalid(p Provider, host string) *errs.Error {
	return errs.WithDetails(CodeTokenInvalid, ErrTokenInvalid.Message,
		map[string]any{"provider": string(p), "host": host})
}

// RateLimited -> 429 RATE_LIMITED; retryAfterSeconds > 0 disertakan di details.
func RateLimited(retryAfterSeconds int) *errs.Error {
	if retryAfterSeconds <= 0 {
		return errs.New(errs.CodeRateLimited, ErrRateLimited.Message)
	}
	return errs.WithDetails(errs.CodeRateLimited, ErrRateLimited.Message,
		map[string]any{"retry_after_seconds": retryAfterSeconds})
}
