package resendinvitation

import "time"

// ResendResponse adalah 200 untuk kirim ulang undangan.
type ResendResponse struct {
	EmailSent   bool                 `json:"email_sent"`
	EmailStatus string               `json:"email_status"`
	ExpiresAt   time.Time            `json:"expires_at"`
	Credentials *CredentialsResponse `json:"credentials,omitempty"`
}

// CredentialsResponse memakai string biasa (BUKAN base.Secret) agar ter-marshal apa adanya.
type CredentialsResponse struct {
	Email             string `json:"email"`
	TemporaryPassword string `json:"temporary_password"`
	LoginURL          string `json:"login_url"`
}
