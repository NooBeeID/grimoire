package resendinvitation

import (
	"context"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"

	"grimoire/internal/domain/core/internal/auth/base"
	"grimoire/internal/domain/core/internal/auth/base/store"
	"grimoire/internal/infra/db"
)

// Mutator menerapkan rule entity di dalam tx. inv nil bila baris undangan tidak ada.
// Error dikembalikan apa adanya oleh Repo.Issue.
type Mutator func(u base.User, inv *base.Invitation) error

// IssueInput adalah masukan Repo.Issue.
type IssueInput struct {
	UserID string
	Hash   string
	Now    time.Time
	Apply  Mutator
}

// Issued adalah hasil Repo.Issue.
type Issued struct {
	User       base.User
	Invitation base.Invitation // setelah mutasi
	Revoked    int64           // jumlah sesi dicabut
}

// Repo adalah akses data usecase resendinvitation (tanpa SQL sendiri).
type Repo struct {
	db *db.DB
	s  *store.Store
}

// NewRepo membuat Repo.
func NewRepo(d *db.DB) *Repo { return &Repo{db: d, s: store.New(d)} }

// Issue menerbitkan ulang undangan dalam satu tx. Urutan lock: undangan lalu users
// (sama dengan changepassword) agar tidak deadlock.
func (r *Repo) Issue(ctx context.Context, in IssueInput) (Issued, error) {
	var out Issued
	err := r.db.BeginTx(ctx, func(tx *sqlx.Tx) error {
		s := r.s.WithTx(tx)
		user, found, err := s.FindUserByID(ctx, in.UserID)
		if err != nil {
			return fmt.Errorf("cari user: %w", err)
		}
		if !found {
			return base.ErrUserNotFound
		}
		inv, ok, err := s.LockInvitation(ctx, in.UserID)
		if err != nil {
			return fmt.Errorf("kunci undangan: %w", err)
		}
		var invPtr *base.Invitation
		if ok {
			invPtr = &inv
		}
		if err := in.Apply(user, invPtr); err != nil {
			return err
		}
		if invPtr == nil {
			return base.ErrInvitationNotPending
		}
		if err := s.UpdatePassword(ctx, user.ID, in.Hash, true, in.Now); err != nil {
			return fmt.Errorf("ganti password: %w", err)
		}
		n, err := s.DeleteSessionsByUser(ctx, user.ID, nil)
		if err != nil {
			return fmt.Errorf("cabut sesi: %w", err)
		}
		if err := s.UpdateInvitation(ctx, inv); err != nil {
			return fmt.Errorf("ubah undangan: %w", err)
		}
		user.PasswordHash, user.MustChangePassword, user.UpdatedAt = in.Hash, true, in.Now
		out = Issued{User: user, Invitation: inv, Revoked: n}
		return nil
	})
	if err != nil {
		return Issued{}, err // *errs.Error tetap terdeteksi; pembungkusan error infra di usecase
	}
	return out, nil
}

// MarkDelivery mencatat hasil pengiriman. Baris hilang atau sudah diterima: no-op.
func (r *Repo) MarkDelivery(ctx context.Context, userID, status string, now time.Time) error {
	return r.db.BeginTx(ctx, func(tx *sqlx.Tx) error {
		s := r.s.WithTx(tx)
		inv, ok, err := s.LockInvitation(ctx, userID)
		if err != nil {
			return err
		}
		if !ok || !inv.IsPending() {
			return nil
		}
		inv.MarkDelivery(status, now)
		return s.UpdateInvitation(ctx, inv)
	})
}
