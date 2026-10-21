package resendinvitation

import (
	"time"

	"grimoire/internal/domain/core/internal/auth/base"
)

const (
	ViaUsers   = "users"   // HTTP POST /api/users/{id}/resend-invitation
	ViaProject = "project" // bus dari management
)

// Input adalah input usecase (tanpa *http.Request, tanpa tipe kontrak bus).
type Input struct {
	UserID         string
	InvitedBy      string // id pengundang; aktor audit bila ctx tanpa identitas
	InvitedByLabel string
	ProjectName    string
	Initial        bool   // true: penerbitan pertama (tanpa kuota dan tanpa audit resent)
	Via            string // ViaUsers | ViaProject
	Meta           base.RequestMeta
}

// Credentials adalah kredensial sementara; hanya dikembalikan bila email tidak terkirim.
type Credentials struct {
	Email             string `json:"email"`
	TemporaryPassword string `json:"temporary_password"` // string biasa: base.Secret ter-marshal "***"
	LoginURL          string `json:"login_url"`
}

// Result keluar lewat WriteJSON (HTTP) atau Response bus. TIDAK dilog, TIDAK masuk audit.Event.
type Result struct {
	EmailSent   bool         `json:"email_sent"`
	EmailStatus string       `json:"email_status"` // sent | failed | not_configured
	ExpiresAt   time.Time    `json:"expires_at"`
	Credentials *Credentials `json:"credentials,omitempty"`
}
