// Package resendinvitation adalah kontrak bus core.auth.resend-invitation (provider core/auth).
//
// Menerbitkan password sementara baru (hash baru, must_change_password=true, semua sesi dicabut),
// memperpanjang masa berlaku, lalu mengirim email setelah commit. Initial=true: penerbitan
// pertama dari management (tanpa hitungan kuota kirim ulang dan tanpa audit "resent").
//
// Pola consumer (project/management):
//
//	res, err := bus.CallTyped[resendinvitation.Response](ctx, b, constants.BUS_CORE_AUTH_RESEND_INVITATION,
//		resendinvitation.Request{UserID: uid, InvitedBy: id.UserID, InvitedByLabel: id.Name, ProjectName: name})
//	if err != nil { return err }
//
// Request dikirim sebagai nilai (bukan pointer).
//
// Error: NOT_FOUND, CONFLICT (details.reason INVITATION_NOT_PENDING),
// ALREADY_REGISTERED (details.reason inactive), RATE_LIMITED (details.scope resend).
package resendinvitation

import (
	"time"

	"grimoire/lib/contracts/core/auth/invitation"
)

type Request struct {
	UserID                    string
	InvitedBy, InvitedByLabel string
	ProjectName               string
	Initial                   bool // true: penerbitan pertama dari management; tanpa kuota & audit resent
}

type Response struct {
	EmailSent   bool
	EmailStatus string
	ExpiresAt   time.Time
	Credentials *invitation.Credentials // hanya bila !EmailSent dan penerbitan berhasil
}
