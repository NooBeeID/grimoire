package buslistgroups

import (
	"context"

	"grimoire/internal/domain/project/internal/searchsynonym/base/store"
)

type Repo struct{ s *store.Store }

func NewRepo(s *store.Store) *Repo { return &Repo{s: s} }

// ListGroups memetakan kelompok project (urut created_at, id) ke daftar term.
func (r *Repo) ListGroups(ctx context.Context, projectID string) ([][]string, error) {
	gs, err := r.s.List(ctx, projectID)
	if err != nil {
		return nil, err
	}
	out := make([][]string, 0, len(gs))
	for _, g := range gs {
		out = append(out, g.Terms)
	}
	return out, nil
}
