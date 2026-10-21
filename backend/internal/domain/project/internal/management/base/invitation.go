package base

import (
	"sort"
	"time"

	"grimoire/lib/contracts/core/auth/getuser"
	"grimoire/lib/contracts/core/auth/invitation"
)

// InvitationDTO adalah baris daftar undangan belum diterima.
type InvitationDTO struct {
	UserID         string     `json:"user_id"`
	Email          string     `json:"email"`
	Name           string     `json:"name"`
	Role           string     `json:"role"`
	InvitedByLabel *string    `json:"invited_by_label"`
	AddedAt        time.Time  `json:"added_at"`
	SentAt         *time.Time `json:"sent_at"`
	ExpiresAt      time.Time  `json:"expires_at"`
	Expired        bool       `json:"expired"`
	DeliveryStatus string     `json:"delivery_status"`
	EmailFailed    bool       `json:"email_failed"`
	ResendCount    int        `json:"resend_count"`
}

// EmailFailed: delivery_status failed atau not_configured (email belum sampai ke penerima).
func EmailFailed(deliveryStatus string) bool {
	return deliveryStatus == invitation.EmailStatusFailed || deliveryStatus == invitation.EmailStatusNotConfigured
}

// ToInvitationDTO memetakan anggota + hasil get-user ke baris undangan.
// ok=false bila u.Found=false atau u.Invitation==nil (bukan undangan menunggu).
func (m Member) ToInvitationDTO(u getuser.Response) (InvitationDTO, bool) {
	if !u.Found || u.Invitation == nil {
		return InvitationDTO{}, false
	}
	inv := u.Invitation
	d := InvitationDTO{
		UserID: m.UserID, Email: u.Email, Name: u.Name, Role: m.Role,
		InvitedByLabel: m.AddedByLabel, AddedAt: m.AddedAt.UTC(),
		ExpiresAt: inv.ExpiresAt.UTC(), Expired: inv.Expired,
		DeliveryStatus: inv.DeliveryStatus, EmailFailed: EmailFailed(inv.DeliveryStatus),
		ResendCount: inv.ResendCount,
	}
	if inv.SentAt != nil {
		t := inv.SentAt.UTC()
		d.SentAt = &t
	}
	return d, true
}

// SortInvitations mengurutkan added_at menurun, tie-break user_id naik (stabil).
func SortInvitations(items []InvitationDTO) {
	sort.SliceStable(items, func(i, j int) bool {
		a, b := items[i], items[j]
		if !a.AddedAt.Equal(b.AddedAt) {
			return a.AddedAt.After(b.AddedAt)
		}
		return a.UserID < b.UserID
	})
}
