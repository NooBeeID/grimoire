package changepassword

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

// ErrStale menandakan hash lama sudah berubah (atau user nonaktif/hilang) saat UPDATE.
var ErrStale = errors.New("changepassword: password berubah oleh proses lain")

// ChangeInput adalah masukan Repo.Change.
type ChangeInput struct {
	UserID        string
	OldHash       string // guard optimistik
	NewHash       string
	KeepTokenHash []byte // sesi saat ini
	Now           time.Time
}

// Repo adalah akses data usecase changepassword.
type Repo struct {
	db    *db.DB
	store *store.Store
}

// NewRepo membuat Repo.
func NewRepo(d *db.DB) *Repo { return &Repo{db: d, store: store.New(d)} }

// FindUserByID membungkus store.FindUserByID.
func (r *Repo) FindUserByID(ctx context.Context, id string) (base.User, bool, error) {
	return r.store.FindUserByID(ctx, id)
}

const updatePasswordSQL = `UPDATE users SET password_hash = $2, must_change_password = false, updated_at = $4
	WHERE id = $1::uuid AND password_hash = $3 AND is_active`

// Change mengganti hash dan mencabut sesi lain dalam satu transaksi.
// Mengembalikan jumlah sesi yang dicabut.
func (r *Repo) Change(ctx context.Context, in ChangeInput) (int64, error) {
	var revoked int64
	err := r.db.BeginTx(ctx, func(tx *sqlx.Tx) error {
		res, err := tx.ExecContext(ctx, updatePasswordSQL, in.UserID, in.NewHash, in.OldHash, in.Now)
		if err != nil {
			return fmt.Errorf("update password: %w", err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("update password: rows affected: %w", err)
		}
		if n == 0 {
			return ErrStale
		}
		keep := in.KeepTokenHash
		if keep == nil {
			keep = []byte{}
		}
		revoked, err = r.store.WithTx(tx).DeleteSessionsByUser(ctx, in.UserID, keep)
		return err
	})
	if err != nil {
		if errors.Is(err, ErrStale) {
			return 0, ErrStale
		}
		return 0, fmt.Errorf("change password tx: %w", err)
	}
	return revoked, nil
}
