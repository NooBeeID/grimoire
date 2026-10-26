package listgroups

import (
	"context"

	"grimoire/internal/domain/project/internal/searchsynonym/base"
	"grimoire/internal/domain/project/internal/searchsynonym/base/store"
)

// Repository adalah satu-satunya lapisan yang menyentuh SQL (lewat store bersama).
type Repository interface {
	List(ctx context.Context, projectID string) ([]base.Group, error)
}

type repo struct{ s *store.Store }

var _ Repository = (*repo)(nil)

func newRepo(s *store.Store) *repo { return &repo{s: s} }

func (r *repo) List(ctx context.Context, projectID string) ([]base.Group, error) {
	return r.s.List(ctx, projectID)
}
