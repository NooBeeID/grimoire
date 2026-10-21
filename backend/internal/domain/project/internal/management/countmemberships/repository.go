package countmemberships

import (
	"context"

	"grimoire/internal/infra/db"
)

// Repo adalah akses SQL untuk feature countmemberships.
type Repo struct{ q db.Querier }

func newRepo(q db.Querier) *Repo { return &Repo{q: q} }

type countRow struct {
	UserID string `db:"user_id"`
	N      int    `db:"n"`
}

const qCountByUsers = `
SELECT user_id::text AS user_id, count(*) AS n
FROM project_members
WHERE user_id = ANY($1::text[]::uuid[])
GROUP BY user_id`

// CountByUsers: ids sudah kanonik, unik, 1..500 (dijamin usecase). Lintas semua project.
func (r *Repo) CountByUsers(ctx context.Context, ids []string) (map[string]int, error) {
	var rows []countRow
	if err := r.q.SelectContext(ctx, &rows, qCountByUsers, ids); err != nil {
		return nil, err
	}
	out := make(map[string]int, len(rows))
	for _, row := range rows {
		out[row.UserID] = row.N
	}
	return out, nil
}
