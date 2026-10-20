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
	ID    string `db:"id"`
	Name  string `db:"name"`
	Email string `db:"email"`
}

// FindActiveDevelopers mengembalikan developer aktif yang email/namanya cocok dengan pattern.
// pattern WAJIB hasil base.LikePattern. Selalu slice non-nil.
func (r *Repository) FindActiveDevelopers(ctx context.Context, pattern string, limit int) ([]contract.Developer, error) {
	const q = `
SELECT id::text AS id, name, email::text AS email
FROM users
WHERE role = 'developer'
  AND is_active
  AND (email ILIKE $1 ESCAPE '\' OR name ILIKE $1 ESCAPE '\')
ORDER BY name ASC, id ASC
LIMIT $2`
	var rows []developerRow
	if err := r.q.SelectContext(ctx, &rows, q, pattern, limit); err != nil {
		return nil, err
	}
	out := make([]contract.Developer, 0, len(rows))
	for _, row := range rows {
		out = append(out, contract.Developer{ID: row.ID, Name: row.Name, Email: row.Email})
	}
	return out, nil
}
