package resendinvitation

import (
	"context"

	"grimoire/internal/domain/project/internal/management/base"
	"grimoire/internal/domain/project/internal/management/base/store"
)

// Repo hanya mendelegasikan ke store bersama (tanpa SQL baru).
type Repo struct{ s *store.Store }

var _ Repository = (*Repo)(nil)

func newRepo(s *store.Store) *Repo { return &Repo{s: s} }

func (r *Repo) FindAccess(ctx context.Context, projectID, userID string) (base.Project, string, bool, error) {
	return r.s.FindAccess(ctx, projectID, userID)
}

func (r *Repo) ListMembers(ctx context.Context, projectID string) ([]base.Member, error) {
	return r.s.ListMembers(ctx, projectID)
}
