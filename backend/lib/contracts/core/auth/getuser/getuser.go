// Package getuser adalah kontrak bus core.auth.get-user.
// UserID bukan uuid atau tidak ada: Found=false tanpa error. User nonaktif:
// Found=true, IsActive=false (konsumen yang memutuskan).
//
// UserID diprioritaskan; Email dipakai hanya bila UserID kosong (setelah trim) dan
// dinormalisasi lowercase+trim oleh provider. Email tidak valid atau tidak ada: Found=false
// tanpa error. Invitation nil bila akun tidak punya undangan menunggu (T12).
package getuser

import "time"

type Request struct {
	UserID string
	Email  string // dipakai bila UserID kosong
}

type Response struct {
	Found    bool
	ID       string
	Name     string
	Email    string
	Role     string
	IsActive bool
	// Invitation nil bila tidak ada undangan menunggu (belum diterima).
	Invitation *Invitation
}

// Invitation adalah penanda undangan menunggu. Undangan kedaluwarsa tetap dilaporkan (Expired=true).
type Invitation struct {
	InvitedAt, IssuedAt, ExpiresAt time.Time
	SentAt                         *time.Time
	Expired                        bool
	DeliveryStatus                 string
	ResendCount                    int
}
