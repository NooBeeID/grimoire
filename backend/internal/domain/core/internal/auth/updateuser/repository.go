package updateuser

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jmoiron/sqlx"

	"grimoire/internal/domain/core/internal/auth/base"
	"grimoire/internal/domain/core/internal/auth/base/store"
	"grimoire/internal/infra/db"
)

const updateUserFieldsSQL = `
UPDATE users
   SET name = $2, email = $3, role = $4, updated_at = $5
 WHERE id = $1::uuid`

type repository struct {
	db    *db.DB
	store *store.Store
}

// NewRepository membuat Repository.
func NewRepository(d *db.DB) Repository { return &repository{db: d, store: store.New(d)} }

func (r *repository) InTx(ctx context.Context, fn func(context.Context, TxRepository) error) error {
	return r.db.BeginTx(ctx, func(tx *sqlx.Tx) error {
		return fn(ctx, &txRepository{store: r.store.WithTx(tx), tx: tx})
	})
}

type txRepository struct {
	store *store.Store
	tx    *sqlx.Tx
}

func (t *txRepository) LockActiveSuperAdminIDs(ctx context.Context) ([]string, error) {
	return t.store.LockActiveSuperAdminIDs(ctx)
}

func (t *txRepository) LockUserByID(ctx context.Context, id string) (base.User, bool, error) {
	return t.store.LockUserByID(ctx, id)
}

func (t *txRepository) UpdateUserFields(ctx context.Context, u base.User) error {
	res, err := t.tx.ExecContext(ctx, updateUserFieldsSQL, u.ID, u.Name, u.Email, u.Role, u.UpdatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "users_email_key" {
			return base.ErrEmailTaken
		}
		return fmt.Errorf("updateuser: update users: %w", err)
	}
	if n, err := res.RowsAffected(); err != nil || n != 1 {
		return fmt.Errorf("updateuser: update users: rows=%d err=%v", n, err)
	}
	return nil
}
