package getproject

import (
	"context"

	"github.com/jmoiron/sqlx"

	"grimoire/internal/domain/project/internal/management/base"
	"grimoire/internal/domain/project/internal/management/base/store"
	"grimoire/internal/infra/db"
)

const memberStatsSQL = `
SELECT COUNT(*) AS members, COUNT(*) FILTER (WHERE role = 'owner') AS owners
FROM project_members WHERE project_id = $1`

// Repo adalah satu-satunya yang menyentuh SQL untuk feature ini.
type Repo struct {
	s *store.Store
	q db.Querier
}

var _ repository = (*Repo)(nil)

func newRepo(q db.Querier, s *store.Store) *Repo { return &Repo{s: s, q: q} }

func (r *Repo) FindAccess(ctx context.Context, projectID, userID string) (base.Project, string, bool, error) {
	return r.s.FindAccess(ctx, projectID, userID)
}

// MemberStats: jumlah anggota dan owner project. Project tanpa anggota -> (0, 0, nil).
func (r *Repo) MemberStats(ctx context.Context, projectID string) (members, owners int, err error) {
	var row struct {
		Members int `db:"members"`
		Owners  int `db:"owners"`
	}
	if err := sqlx.GetContext(ctx, r.q, &row, memberStatsSQL, projectID); err != nil {
		return 0, 0, err
	}
	return row.Members, row.Owners, nil
}
