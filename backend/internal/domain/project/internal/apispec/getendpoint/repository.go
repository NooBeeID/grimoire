package getendpoint

import (
	"context"

	"grimoire/internal/domain/project/internal/apispec/base"
	"grimoire/internal/domain/project/internal/apispec/base/store"
)

// Repo mendelegasikan semua query ke store (tanpa SQL sendiri).
type Repo struct{ s *store.Store }

func newRepo(s *store.Store) *Repo { return &Repo{s: s} }

func (r *Repo) ListSpecNames(ctx context.Context, projectID string) ([]string, error) {
	return r.s.ListSpecNames(ctx, projectID)
}

func (r *Repo) FindSpec(ctx context.Context, projectID, name string) (base.Spec, bool, error) {
	return r.s.FindSpec(ctx, projectID, name)
}

func (r *Repo) ListSpecEndpoints(ctx context.Context, specID string) ([]base.Endpoint, error) {
	return r.s.ListSpecEndpoints(ctx, specID)
}
