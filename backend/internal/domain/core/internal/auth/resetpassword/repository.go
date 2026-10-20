package resetpassword

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"

	"grimoire/internal/domain/core/internal/auth/base"
	"grimoire/internal/domain/core/internal/auth/base/store"
	"grimoire/internal/infra/db"
)

// ErrTargetGone menandakan user target tidak ada saat UPDATE.
var ErrTargetGone = errors.New("resetpassword: user tidak ada")

// ResetInput adalah masukan Repo.Reset.
type ResetInput struct {
	UserID string
	Hash   string
	Now    time.Time
}

// Repo adalah akses data usecase resetpassword.
type Repo struct {
	db    *db.DB
	store *store.Store
}

// NewRepo membuat Repo.
func NewRepo(d *db.DB) *Repo { return &Repo{db: d, store: store.New(d)} }

// FindUserByID membungkus store.FindUserByID (id bukan uuid -> found=false).
func (r *Repo) FindUserByID(ctx context.Context, id string) (base.User, bool, error) {
	return r.store.FindUserByID(ctx, id)
}

// Reset mengganti hash, menyalakan must_change_password, dan menghapus semua sesi target
// dalam satu transaksi. Mengembalikan user terbaru dan jumlah sesi dicabut.
func (r *Repo) Reset(ctx context.Context, in ResetInput) (base.User, int64, error) {
	var (
		updated base.User
		revoked int64
	)
	err := r.db.BeginTx(ctx, func(tx *sqlx.Tx) error {
		s := r.store.WithTx(tx)
		// UPDATE di sini: store.UpdatePassword tidak mengembalikan baris.
		if err := s.UpdatePassword(ctx, in.UserID, in.Hash, true, in.Now); err != nil {
			if errors.Is(err, base.ErrUserNotFound) {
				return ErrTargetGone
			}
			return err
		}
		u, found, err := s.FindUserByID(ctx, in.UserID)
		if err != nil {
			return err
		}
		if !found {
			return ErrTargetGone
		}
		n, err := s.DeleteSessionsByUser(ctx, in.UserID, nil)
		if err != nil {
			return err
		}
		updated, revoked = u, n
		return nil
	})
	if err != nil {
		if errors.Is(err, ErrTargetGone) {
			return base.User{}, 0, ErrTargetGone
		}
		return base.User{}, 0, fmt.Errorf("reset tx: %w", err)
	}
	return updated, revoked, nil
}
