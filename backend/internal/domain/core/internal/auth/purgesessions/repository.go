package purgesessions

import (
	"context"
	"fmt"
	"time"

	"grimoire/internal/infra/db"
)

// Repository adalah satu-satunya bagian feature yang menyentuh SQL.
type Repository struct{ q db.Querier }

func NewRepository(q db.Querier) *Repository { return &Repository{q: q} }

const deleteSessionsSQL = `DELETE FROM sessions
WHERE expires_at < $1 OR last_seen_at < $2`

const deleteThrottlesSQL = `DELETE FROM login_throttles
WHERE window_started_at < $2
  AND (locked_until IS NULL OR locked_until < $1)`

// DeleteExpiredSessions: $1 = now, $2 = idleCutoff. Mengembalikan RowsAffected.
func (r *Repository) DeleteExpiredSessions(ctx context.Context, now, idleCutoff time.Time) (int64, error) {
	res, err := r.q.ExecContext(ctx, deleteSessionsSQL, now, idleCutoff)
	if err != nil {
		return 0, fmt.Errorf("purgesessions: hapus sessions: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("purgesessions: hapus sessions: rows affected: %w", err)
	}
	return n, nil
}

// DeleteExpiredThrottles: $1 = now, $2 = windowCutoff. Mengembalikan RowsAffected.
func (r *Repository) DeleteExpiredThrottles(ctx context.Context, now, windowCutoff time.Time) (int64, error) {
	res, err := r.q.ExecContext(ctx, deleteThrottlesSQL, now, windowCutoff)
	if err != nil {
		return 0, fmt.Errorf("purgesessions: hapus login_throttles: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("purgesessions: hapus login_throttles: rows affected: %w", err)
	}
	return n, nil
}
