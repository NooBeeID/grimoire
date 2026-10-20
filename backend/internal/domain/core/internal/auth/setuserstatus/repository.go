package setuserstatus

import (
	"context"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"

	"grimoire/internal/domain/core/internal/auth/base"
	"grimoire/internal/domain/core/internal/auth/base/store"
	"grimoire/internal/infra/db"
)

// Repo adalah akses data usecase setuserstatus.
type Repo struct {
	db    *db.DB
	store *store.Store
}

// NewRepo membuat Repo.
func NewRepo(d *db.DB) *Repo { return &Repo{db: d, store: store.New(d)} }

// txOps adalah operasi di dalam transaksi; hanya valid selama callback InTx berjalan.
type txOps struct {
	store *store.Store
	tx    *sqlx.Tx
}

// InTx membuka transaksi, menjalankan fn, commit bila nil, rollback bila error/panic.
func (r *Repo) InTx(ctx context.Context, fn func(Tx) error) error {
	return r.db.BeginTx(ctx, func(tx *sqlx.Tx) error {
		return fn(&txOps{store: r.store.WithTx(tx), tx: tx})
	})
}

// LockActiveSuperAdminIDs mengunci semua super admin aktif, urut id.
func (t *txOps) LockActiveSuperAdminIDs(ctx context.Context) ([]string, error) {
	return t.store.LockActiveSuperAdminIDs(ctx)
}

// LockUser mengunci baris target (FOR UPDATE).
func (t *txOps) LockUser(ctx context.Context, id string) (base.User, bool, error) {
	return t.store.LockUserByID(ctx, id)
}

// SetActive mengubah is_active; baris sudah terkunci sehingga harus tepat 1 baris.
func (t *txOps) SetActive(ctx context.Context, id string, active bool, now time.Time) error {
	res, err := t.tx.ExecContext(ctx,
		`UPDATE users SET is_active = $2, updated_at = $3 WHERE id = $1::uuid`, id, active, now)
	if err != nil {
		return fmt.Errorf("update status user: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if n != 1 {
		return fmt.Errorf("update status user: %d baris terpengaruh", n)
	}
	return nil
}

// DeleteSessions menghapus semua sesi user dan mengembalikan jumlahnya.
func (t *txOps) DeleteSessions(ctx context.Context, userID string) (int64, error) {
	return t.store.DeleteSessionsByUser(ctx, userID, nil)
}
