// Package createadmin adalah kontrak bus core.auth.create-admin (consumer: cmd/cli).
// Kegagalan dikembalikan provider sebagai *errs.Error: CONFLICT (email sudah ada),
// INVALID_ARGUMENT dengan details.fields (validasi), atau error terbungkus (infra).
package createadmin

type Request struct {
	Email, Name, Password string
}

type Response struct {
	UserID string
}
