package inviteuser

import (
	"time"

	"grimoire/internal/domain/core/internal/auth/base"
	"grimoire/lib/contracts/core/auth/invitation"
)

const (
	ViaUsers   = "users"   // HTTP POST /api/users/invite
	ViaProject = "project" // bus dari management
)

// InviteRequest adalah body POST /api/users/invite.
type InviteRequest struct {
	Email string `json:"email"`
	Name  string `json:"name"` // opsional
}

// Input adalah input usecase (tanpa *http.Request, tanpa tipe kontrak bus).
type Input struct {
	Email, Name               string
	InvitedBy, InvitedByLabel string // id dan label pengundang (label: nama, fallback email)
	ProjectName               string // hanya untuk isi email dan audit
	Deliver                   bool   // false: hanya siapkan (dipakai management)
	Via                       string // ViaUsers | ViaProject
	Meta                      base.RequestMeta
}

// Result adalah hasil usecase. TIDAK dilog dan tidak masuk audit.Event.
type Result struct {
	User        base.User
	Invitation  base.Invitation
	Created     bool
	Now         time.Time               // waktu yang dipakai usecase (untuk Expired di respons)
	EmailStatus string                  // invitation.EmailStatus*; "pending" bila !Deliver
	EmailSent   bool                    // EmailStatus == sent
	Credentials *invitation.Credentials // hanya Deliver && !EmailSent
}

// InvitationView adalah penanda undangan pada respons HTTP.
type InvitationView struct {
	Pending        bool       `json:"pending"`
	InvitedAt      time.Time  `json:"invited_at"` // = created_at baris undangan
	SentAt         *time.Time `json:"sent_at"`
	ExpiresAt      time.Time  `json:"expires_at"`
	Expired        bool       `json:"expired"`
	DeliveryStatus string     `json:"delivery_status"`
	ResendCount    int        `json:"resend_count"`
}

// UserView menyematkan base.UserDTO; field Invitation luar menang atas yang tersemat
// (shadowing encoding/json).
type UserView struct {
	base.UserDTO
	Invitation InvitationView `json:"invitation"`
}

// CredentialsDTO memakai string biasa (base.Secret ter-marshal "***").
type CredentialsDTO struct {
	Email             string `json:"email"`
	TemporaryPassword string `json:"temporary_password"`
	LoginURL          string `json:"login_url"`
}

// InviteResponse adalah body 201.
type InviteResponse struct {
	User        UserView        `json:"user"`
	Created     bool            `json:"created"`
	EmailSent   bool            `json:"email_sent"`
	EmailStatus string          `json:"email_status"`
	ExpiresAt   time.Time       `json:"expires_at"`
	Credentials *CredentialsDTO `json:"credentials,omitempty"`
}

// toResponse memetakan Result ke JSON HTTP; now dipakai untuk Expired.
func toResponse(r Result, now time.Time) InviteResponse {
	inv := r.Invitation
	out := InviteResponse{
		User: UserView{
			UserDTO: r.User.ToResponse(),
			Invitation: InvitationView{
				Pending: inv.IsPending(), InvitedAt: inv.CreatedAt, SentAt: inv.SentAt, ExpiresAt: inv.ExpiresAt,
				Expired: inv.IsExpired(now), DeliveryStatus: inv.DeliveryStatus, ResendCount: inv.ResendCount,
			},
		},
		Created:     r.Created,
		EmailSent:   r.EmailSent,
		EmailStatus: r.EmailStatus,
		ExpiresAt:   inv.ExpiresAt,
	}
	if c := r.Credentials; c != nil {
		out.Credentials = &CredentialsDTO{Email: c.Email, TemporaryPassword: c.TemporaryPassword, LoginURL: c.LoginURL}
	}
	return out
}
