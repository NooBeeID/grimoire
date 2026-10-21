package createfile

import (
	"context"

	"grimoire/internal/domain/project/internal/knowledge/base"
	"grimoire/internal/domain/project/internal/knowledge/base/store"
)

const insertSQL = `INSERT INTO knowledge_files
  (project_id, path, title, content, content_hash, size_bytes,
   created_by, created_by_label, updated_by, updated_by_label)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$7,$8)
RETURNING ` + store.FileColumns

type Repo struct{ s *store.Store }

var _ Repository = (*Repo)(nil)

func NewRepository(s *store.Store) *Repo { return &Repo{s: s} }

func (r *Repo) InTx(ctx context.Context, fn func(Repository) error) error {
	return r.s.InTx(ctx, func(ts *store.Store) error { return fn(&Repo{s: ts}) })
}

func (r *Repo) LockProject(ctx context.Context, id string) error { return r.s.LockProject(ctx, id) }

func (r *Repo) CountFiles(ctx context.Context, id string) (int, error) {
	return r.s.CountFiles(ctx, id)
}

func (r *Repo) Insert(ctx context.Context, f base.File) (base.File, error) {
	saved, ok, err := r.s.GetFile(ctx, insertSQL, f.ProjectID, f.Path, f.Title, f.Content, f.Hash, f.SizeBytes,
		f.CreatedBy, f.CreatedByLabel)
	if err != nil {
		if store.IsUniqueViolation(err) {
			return base.File{}, base.ErrPathExists
		}
		return base.File{}, err
	}
	if !ok {
		return base.File{}, base.ErrFileNotFound
	}
	return saved, nil
}
