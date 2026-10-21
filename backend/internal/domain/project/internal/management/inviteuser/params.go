package inviteuser

import (
	"strings"
	"time"
	"unicode/utf8"

	"grimoire/internal/domain/project/internal/management/base"
)

const maxEmailLen = 254

// InviteRequest adalah body POST. Role dan Name opsional.
type InviteRequest struct {
	Email string `json:"email"`
	Role  string `json:"role"` // "" -> member
	Name  string `json:"name"` // default dari auth (bagian lokal email)
}

// Validate hanya memeriksa bentuk. Format email penuh dan panjang nama divalidasi auth.
func (r *InviteRequest) Validate() error {
	r.Email = strings.ToLower(strings.TrimSpace(r.Email))
	r.Name = strings.TrimSpace(r.Name)
	fe := base.FieldErrors{}
	switch {
	case r.Email == "":
		fe["email"] = "wajib diisi"
	case utf8.RuneCountInString(r.Email) > maxEmailLen:
		fe["email"] = "maksimal 254 karakter"
	case strings.ContainsAny(r.Email, " \t\r\n"):
		fe["email"] = "format email tidak valid"
	}
	if len(fe) > 0 {
		return base.InvalidFields(fe)
	}
	return nil
}

// CredentialsDTO adalah kredensial fallback sekali tampil (string biasa, bukan base.Secret).
type CredentialsDTO struct {
	Email             string `json:"email"`
	TemporaryPassword string `json:"temporary_password"`
	LoginURL          string `json:"login_url"`
}

type InviteResponse struct {
	Member      base.MemberDTO  `json:"member"`
	Created     bool            `json:"created"`
	EmailSent   bool            `json:"email_sent"`
	EmailStatus string          `json:"email_status"`
	ExpiresAt   time.Time       `json:"expires_at"`
	Credentials *CredentialsDTO `json:"credentials,omitempty"`
}
