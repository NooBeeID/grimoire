package base

import (
	"strings"
	"time"
	"unicode/utf8"
)

// Status pengiriman email undangan (kolom user_invitations.delivery_status).
const (
	DeliveryPending       = "pending"
	DeliverySent          = "sent"
	DeliveryFailed        = "failed"
	DeliveryNotConfigured = "not_configured"
)

// resendWindow adalah lebar jendela batas kirim ulang dan batas undangan per pengundang.
const resendWindow = time.Hour

// Invitation adalah entity undangan (satu baris per akun yang diundang). Tanpa tag db.
type Invitation struct {
	UserID                string
	InvitedBy             string
	InvitedByLabel        *string
	IssuedAt, ExpiresAt   time.Time
	CreatedAt             time.Time
	SentAt, AcceptedAt    *time.Time
	ResendWindowStartedAt *time.Time
	DeliveryStatus        string
	ResendCount           int
	ResendWindowCount     int
}

// InvitationPolicy adalah kebijakan undangan dari config.
type InvitationPolicy struct {
	TTL                  time.Duration
	MaxPerInviterPerHour int
	ResendMaxPerHour     int
	PurgeAfter           time.Duration
}

// InviteMode adalah hasil CheckInviteTarget.
type InviteMode int

const (
	// InviteCreateNew: email belum terdaftar, buat akun baru.
	InviteCreateNew InviteMode = iota + 1
	// InviteReissuePending: akun masih menunggu undangan; terbitkan ulang.
	InviteReissuePending
)

// NewInvitation membuat undangan baru: terbit sekarang, kedaluwarsa now+TTL, status pending.
// Label kosong disimpan sebagai nil.
func NewInvitation(userID, invitedBy, label string, now time.Time, p InvitationPolicy) Invitation {
	inv := Invitation{
		UserID: userID, InvitedBy: invitedBy,
		IssuedAt: now, ExpiresAt: now.Add(p.TTL), CreatedAt: now,
		DeliveryStatus: DeliveryPending,
	}
	if l := strings.TrimSpace(label); l != "" {
		inv.InvitedByLabel = &l
	}
	return inv
}

// IsPending: undangan belum diterima (termasuk yang sudah kedaluwarsa).
func (i Invitation) IsPending() bool { return i.AcceptedAt == nil }

// IsExpired: menunggu dan now >= ExpiresAt.
func (i Invitation) IsExpired(now time.Time) bool { return i.IsPending() && !now.Before(i.ExpiresAt) }

// BlocksLogin: undangan kedaluwarsa menolak login/ganti password.
func (i Invitation) BlocksLogin(now time.Time) bool { return i.IsExpired(now) }

// Reissue memperpanjang masa berlaku dari now dan mengembalikan status ke pending.
func (i *Invitation) Reissue(now time.Time, p InvitationPolicy) {
	i.IssuedAt = now
	i.ExpiresAt = now.Add(p.TTL)
	i.DeliveryStatus = DeliveryPending
}

// windowOpen: jendela kirim ulang masih berjalan pada now.
func (i Invitation) windowOpen(now time.Time) bool {
	return i.ResendWindowStartedAt != nil && now.Before(i.ResendWindowStartedAt.Add(resendWindow))
}

// CheckResend memeriksa kuota kirim ulang tanpa mengubah state.
func (i Invitation) CheckResend(now time.Time, p InvitationPolicy) error {
	if i.windowOpen(now) && i.ResendWindowCount >= p.ResendMaxPerHour {
		return ErrInviteRateLimited(i.ResendWindowStartedAt.Add(resendWindow).Sub(now), ScopeResend)
	}
	return nil
}

// RegisterResend mencatat satu kirim ulang (window 1 jam). Lolos: ResendCount++ dan
// ResendWindowCount++. Melewati batas: ErrInviteRateLimited dan state tidak berubah.
func (i *Invitation) RegisterResend(now time.Time, p InvitationPolicy) error {
	if !i.windowOpen(now) {
		start := now
		i.ResendWindowStartedAt = &start
		i.ResendWindowCount = 0
	}
	if err := i.CheckResend(now, p); err != nil {
		return err
	}
	i.ResendWindowCount++
	i.ResendCount++
	return nil
}

// MarkDelivery mencatat hasil pengiriman; status sent mengisi SentAt.
func (i *Invitation) MarkDelivery(status string, now time.Time) {
	i.DeliveryStatus = status
	if status == DeliverySent {
		t := now
		i.SentAt = &t
	}
}

// Accept menandai undangan diterima.
func (i *Invitation) Accept(now time.Time) {
	t := now
	i.AcceptedAt = &t
}

// CheckInviteTarget menentukan apa yang boleh dilakukan untuk email target.
// u == nil: buat akun baru. Akun developer aktif dengan undangan menunggu (juga yang
// sudah kedaluwarsa): terbitkan ulang. Selain itu ALREADY_REGISTERED dengan reason
// berurutan inactive, not_developer, registered.
func CheckInviteTarget(u *User, inv *Invitation) (InviteMode, error) {
	if u == nil {
		return InviteCreateNew, nil
	}
	if inv != nil && inv.IsPending() && u.IsActive && u.Role == RoleDeveloper {
		return InviteReissuePending, nil
	}
	switch {
	case !u.IsActive:
		return 0, ErrAlreadyRegistered(AlreadyRegisteredInactive)
	case u.Role != RoleDeveloper:
		return 0, ErrAlreadyRegistered(AlreadyRegisteredNotDeveloper)
	default:
		return 0, ErrAlreadyRegistered(AlreadyRegisteredRegistered)
	}
}

// CheckInviterQuota membatasi jumlah undangan baru per pengundang per jam.
// oldestInWindow adalah created_at undangan tertua dalam jendela (menentukan retry-after).
func CheckInviterQuota(createdLastHour int, oldestInWindow, now time.Time, p InvitationPolicy) error {
	if createdLastHour < p.MaxPerInviterPerHour {
		return nil
	}
	return ErrInviteRateLimited(oldestInWindow.Add(resendWindow).Sub(now), ScopeInvite)
}

// DefaultNameFromEmail membentuk nama awal dari bagian lokal email. Hasil selalu lolos ValidateName.
func DefaultNameFromEmail(email string) string {
	local := email
	if i := strings.Index(email, "@"); i >= 0 {
		local = email[:i]
	}
	local = strings.NewReplacer(".", " ", "_", " ", "-", " ", "+", " ").Replace(local)
	local = strings.Join(strings.Fields(local), " ")
	if utf8.RuneCountInString(local) > MaxNameRunes {
		local = strings.TrimSpace(string([]rune(local)[:MaxNameRunes]))
	}
	if n, err := ValidateName(local); err == nil {
		return n
	}
	return "Developer"
}

// InvitationDTO adalah penanda undangan pada respons user (tanpa rahasia).
type InvitationDTO struct {
	Pending        bool       `json:"pending"` // selalu true bila objek ada
	InvitedAt      time.Time  `json:"invited_at"`
	SentAt         *time.Time `json:"sent_at"`
	ExpiresAt      time.Time  `json:"expires_at"`
	Expired        bool       `json:"expired"`
	DeliveryStatus string     `json:"delivery_status"`
	ResendCount    int        `json:"resend_count"`
}

// ToResponse mengembalikan nil bila undangan sudah diterima (bukan menunggu).
func (i Invitation) ToResponse(now time.Time) *InvitationDTO {
	if !i.IsPending() {
		return nil
	}
	return &InvitationDTO{
		Pending: true, InvitedAt: i.CreatedAt, SentAt: i.SentAt, ExpiresAt: i.ExpiresAt,
		Expired: i.IsExpired(now), DeliveryStatus: i.DeliveryStatus, ResendCount: i.ResendCount,
	}
}
