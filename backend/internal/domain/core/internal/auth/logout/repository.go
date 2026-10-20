package logout

import (
	"context"
	"fmt"

	"grimoire/internal/infra/db"
)

// Repository adalah akses data usecase Logout.
type Repository interface {
	DeleteSessionByHash(ctx context.Context, tokenHash []byte) (deleted int64, err error)
}

type repository struct{ db db.Querier }

// NewRepository membuat Repository di atas db.Querier.
func NewRepository(q db.Querier) Repository { return &repository{db: q} }

const deleteSessionSQL = `DELETE FROM sessions WHERE token_hash = $1`

func (r *repository) DeleteSessionByHash(ctx context.Context, tokenHash []byte) (int64, error) {
	res, err := r.db.ExecContext(ctx, deleteSessionSQL, tokenHash)
	if err != nil {
		return 0, fmt.Errorf("delete session: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("delete session: rows affected: %w", err)
	}
	return n, nil
}
