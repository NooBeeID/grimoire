package uploadfiles

import (
	"context"

	"grimoire/internal/domain/project/internal/knowledge/base"
	"grimoire/internal/domain/project/internal/knowledge/base/ingest"
	"grimoire/internal/domain/project/internal/knowledge/base/store"
)

// Repo membungkus *store.Store. SQL tulis batch ada di base/store (dipakai juga oleh sync Git).
type Repo struct{ s *store.Store }

var (
	_ repository = (*Repo)(nil)
	_ ingest.Tx  = (*Repo)(nil)
)

func NewRepo(s *store.Store) *Repo { return &Repo{s: s} }

func (r *Repo) InTx(ctx context.Context, fn func(repository) error) error {
	return r.s.InTx(ctx, func(ts *store.Store) error { return fn(&Repo{s: ts}) })
}

func (r *Repo) LockProject(ctx context.Context, projectID string) error {
	return r.s.LockProject(ctx, projectID)
}

func (r *Repo) CountFiles(ctx context.Context, projectID string) (int, error) {
	return r.s.CountFiles(ctx, projectID)
}

func (r *Repo) ExistingHashes(ctx context.Context, projectID string, paths []string) (map[string][]byte, error) {
	return r.s.ExistingHashes(ctx, projectID, paths)
}

func (r *Repo) InsertFiles(ctx context.Context, files []base.File) error {
	return r.s.InsertFiles(ctx, files)
}

func (r *Repo) UpdateFiles(ctx context.Context, files []base.File) error {
	return r.s.UpdateFiles(ctx, files)
}
