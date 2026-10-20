package purgeexpired

import (
	"context"
	"fmt"
	"time"

	"grimoire/internal/infra/db"
)

type Repository struct{ db db.Querier }

func NewRepository(q db.Querier) *Repository { return &Repository{db: q} }

const deleteBatchSQL = `DELETE FROM audit_log
WHERE id IN (
    SELECT id FROM audit_log
    WHERE occurred_at < $1
    ORDER BY occurred_at
    LIMIT $2
)`

// DeleteBatch menghapus sampai limit entri dengan occurred_at < cutoff; mengembalikan RowsAffected.
func (r *Repository) DeleteBatch(ctx context.Context, cutoff time.Time, limit int) (int64, error) {
	res, err := r.db.ExecContext(ctx, deleteBatchSQL, cutoff, limit)
	if err != nil {
		return 0, fmt.Errorf("purge audit_log batch: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("purge audit_log rows affected: %w", err)
	}
	return n, nil
}
