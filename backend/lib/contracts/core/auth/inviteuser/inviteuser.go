// Package inviteuser adalah kontrak bus core.auth.invite-user (provider core/auth).
//
// Membuat akun developer (atau memakai ulang akun yang masih menunggu undangan) beserta
// baris undangan. Deliver=false hanya menyiapkan akun; penerbitan password dan pengiriman
// email dilakukan lewat core.auth.resend-invitation (Initial=true).
//
// Pola consumer (project/management):
//
//	res, err := bus.CallTyped[inviteuser.Response](ctx, b, constants.BUS_CORE_AUTH_INVITE_USER,
//		inviteuser.Request{Email: email, InvitedBy: id.UserID, InvitedByLabel: id.Name, Deliver: false})
//	if err != nil { return err } // *errs.Error (ALREADY_REGISTERED, RATE_LIMITED, INVALID_ARGUMENT) diteruskan apa adanya
//
// Request dikirim sebagai nilai (bukan pointer).
//
// Error: INVALID_ARGUMENT (details.fields), ALREADY_REGISTERED (details.reason),
// RATE_LIMITED (details.retry_after_seconds, details.scope).
package inviteuser

import (
	"time"

	"grimoire/lib/contracts/core/auth/invitation"
)

type Request struct {
	Email, Name               string
	InvitedBy, InvitedByLabel string
	ProjectName               string
	Deliver                   bool // false: hanya siapkan (dipakai management)
}

type Response struct {
	UserID      string
	Created     bool
	ExpiresAt   time.Time
	EmailSent   bool
	EmailStatus string                  // invitation.EmailStatus*; "pending" bila Deliver=false
	Credentials *invitation.Credentials // hanya bila Deliver && !EmailSent
}
