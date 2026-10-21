package purgeinvitations

import (
	"context"
	"fmt"
	"time"

	"grimoire/internal/infra/db"
)

// Repository adalah satu-satunya bagian feature yang menyentuh SQL.
type Repository struct{ q db.Querier }

func NewRepository(q db.Querier) *Repository { return &Repository{q: q} }

// Candidate adalah akun undangan kedaluwarsa; juga dipakai sebagai kursor keyset.
type Candidate struct {
	UserID    string    `db:"user_id"`
	ExpiresAt time.Time `db:"expires_at"`
}

// $1 cutoff, $2 after.expires_at (NULL = awal), $3 after.user_id (NULL = awal), $4 limit.
const listCandidatesSQL = `SELECT i.user_id::text AS user_id, i.expires_at
FROM user_invitations i
WHERE i.accepted_at IS NULL
  AND i.expires_at < $1
  AND ($2::timestamptz IS NULL OR (i.expires_at, i.user_id) > ($2::timestamptz, $3::uuid))
ORDER BY i.expires_at, i.user_id
LIMIT $4`

// $1 ids, $2 cutoff. Kondisi dicek ulang agar akun yang diterima/diundang ulang tidak terhapus.
const deleteUsersSQL = `DELETE FROM users u
WHERE u.id = ANY($1::text[]::uuid[])
  AND u.role = 'developer'
  AND EXISTS (SELECT 1 FROM user_invitations i
              WHERE i.user_id = u.id
                AND i.accepted_at IS NULL
                AND i.expires_at < $2)`

// ListCandidates mengambil satu batch kandidat setelah kursor (nil = dari awal).
func (r *Repository) ListCandidates(ctx context.Context, cutoff time.Time, after *Candidate, limit int) ([]Candidate, error) {
	var afterAt *time.Time
	var afterID *string
	if after != nil {
		afterAt, afterID = &after.ExpiresAt, &after.UserID
	}
	var out []Candidate
	if err := r.q.SelectContext(ctx, &out, listCandidatesSQL, cutoff, afterAt, afterID, limit); err != nil {
		return nil, fmt.Errorf("purgeinvitations: list kandidat: %w", err)
	}
	return out, nil
}

// DeleteUsers menghapus user (cascade ke sessions dan user_invitations). Mengembalikan RowsAffected.
func (r *Repository) DeleteUsers(ctx context.Context, ids []string, cutoff time.Time) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	res, err := r.q.ExecContext(ctx, deleteUsersSQL, ids, cutoff)
	if err != nil {
		return 0, fmt.Errorf("purgeinvitations: hapus users: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("purgeinvitations: hapus users: rows affected: %w", err)
	}
	return n, nil
}
