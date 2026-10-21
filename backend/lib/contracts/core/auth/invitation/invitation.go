// Package invitation memuat tipe bersama kontrak undangan core/auth
// (core.auth.invite-user dan core.auth.resend-invitation).
package invitation

// Credentials adalah kredensial sementara fallback. JANGAN di-log, diaudit, atau
// diterbitkan lewat Publish; hanya diteruskan ke respons HTTP pengundang
// (dengan Cache-Control: no-store).
type Credentials struct{ Email, TemporaryPassword, LoginURL string }

// Nilai Response.EmailStatus.
const (
	EmailStatusSent          = "sent"
	EmailStatusFailed        = "failed"
	EmailStatusNotConfigured = "not_configured"
	EmailStatusPending       = "pending" // hanya bila Deliver=false
)
