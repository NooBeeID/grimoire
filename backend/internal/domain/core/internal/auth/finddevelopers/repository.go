package finddevelopers

import (
	"context"

	"grimoire/internal/infra/db"
	contract "grimoire/lib/contracts/core/auth/finddevelopers"
)

// Repository adalah satu-satunya bagian feature yang menyentuh SQL.
type Repository struct{ q db.Querier }

func NewRepository(q db.Querier) *Repository { return &Repository{q: q} }

// developerRow adalah row privat ber-tag db.
type developerRow struct {
	ID                string `db:"id"`
	Name              string `db:"name"`
	Email             string `db:"email"`
	InvitationPending bool   `db:"invitation_pending"`
}

// FindActiveDevelopers mengembalikan developer aktif yang email/namanya cocok dengan pattern.
// pattern WAJIB hasil base.LikePattern. Selalu slice non-nil.
func (r *Repository) FindActiveDevelopers(ctx context.Context, pattern string, limit int) ([]contract.Developer, error) {
	const q = `
SELECT u.id::text AS id, u.name, u.email::text AS email,
       (i.user_id IS NOT NULL) AS invitation_pending
FROM users u
LEFT JOIN user_invitations i ON i.user_id = u.id AND i.accepted_at IS NULL
WHERE u.role = 'developer'
  AND u.is_active
  AND (u.email ILIKE $1 ESCAPE '\' OR u.name ILIKE $1 ESCAPE '\')
ORDER BY u.name ASC, u.id ASC
LIMIT $2`
	var rows []developerRow
	if err := r.q.SelectContext(ctx, &rows, q, pattern, limit); err != nil {
		return nil, err
	}
	out := make([]contract.Developer, 0, len(rows))
	for _, row := range rows {
		out = append(out, contract.Developer{ID: row.ID, Name: row.Name, Email: row.Email, InvitationPending: row.InvitationPending})
	}
	return out, nil
}
