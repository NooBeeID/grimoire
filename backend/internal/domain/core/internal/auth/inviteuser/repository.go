package inviteuser

import (
	"context"
	"time"

	"github.com/jmoiron/sqlx"

	"grimoire/internal/domain/core/internal/auth/base"
	"grimoire/internal/domain/core/internal/auth/base/store"
	"grimoire/internal/infra/db"
)

// Repo menyusun method base/store; tidak ada SQL sendiri.
type Repo struct {
	db *db.DB
	s  *store.Store
}

// NewRepo membuat Repo.
func NewRepo(d *db.DB) *Repo { return &Repo{db: d, s: store.New(d)} }

// Target adalah keadaan email: User nil bila belum ada akun, Invitation nil bila tanpa undangan.
type Target struct {
	User       *base.User
	Invitation *base.Invitation
}

// FindTarget membaca user dan undangannya (undangan hanya dicari bila user ada).
func (r *Repo) FindTarget(ctx context.Context, email string) (Target, error) {
	u, found, err := r.s.FindUserByEmail(ctx, email)
	if err != nil || !found {
		return Target{}, err
	}
	t := Target{User: &u}
	inv, ok, err := r.s.FindInvitation(ctx, u.ID)
	if err != nil {
		return Target{}, err
	}
	if ok {
		t.Invitation = &inv
	}
	return t, nil
}

// CountInvitedSince menghitung undangan baru milik pengundang sejak since.
func (r *Repo) CountInvitedSince(ctx context.Context, inviterID string, since time.Time) (int, time.Time, error) {
	return r.s.CountInvitationsCreatedBy(ctx, inviterID, since)
}

// InTx menjalankan fn dalam satu transaksi. Error dikembalikan apa adanya.
func (r *Repo) InTx(ctx context.Context, fn func(Tx) error) error {
	return r.db.BeginTx(ctx, func(tx *sqlx.Tx) error {
		return fn(&txRepo{s: r.s.WithTx(tx)})
	})
}

type txRepo struct{ s *store.Store }

func (t *txRepo) InsertUser(ctx context.Context, u base.User) (base.User, error) {
	return t.s.InsertUser(ctx, u)
}

func (t *txRepo) InsertInvitation(ctx context.Context, inv base.Invitation) error {
	return t.s.InsertInvitation(ctx, inv)
}

func (t *txRepo) LockUser(ctx context.Context, id string) (base.User, bool, error) {
	return t.s.LockUserByID(ctx, id)
}

func (t *txRepo) LockInvitation(ctx context.Context, userID string) (base.Invitation, bool, error) {
	return t.s.LockInvitation(ctx, userID)
}

func (t *txRepo) UpdateInvitation(ctx context.Context, inv base.Invitation) error {
	return t.s.UpdateInvitation(ctx, inv)
}

func (t *txRepo) UpdatePassword(ctx context.Context, id, hash string, mustChange bool, now time.Time) error {
	return t.s.UpdatePassword(ctx, id, hash, mustChange, now)
}

func (t *txRepo) DeleteSessions(ctx context.Context, userID string) (int64, error) {
	return t.s.DeleteSessionsByUser(ctx, userID, nil)
}
