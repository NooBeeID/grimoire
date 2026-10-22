package busupsertspecs

import (
	"context"
	"fmt"

	"github.com/jmoiron/sqlx"

	"grimoire/internal/domain/project/internal/apispec/base"
	"grimoire/internal/domain/project/internal/apispec/base/ingest"
	"grimoire/internal/domain/project/internal/apispec/base/store"
	"grimoire/internal/infra/db"
)

type Repo struct {
	db *db.DB
	s  *store.Store
}

func newRepo(d *db.DB, s *store.Store) *Repo { return &Repo{db: d, s: s} }

var _ ingest.Repo = (*Repo)(nil)

func (r *Repo) InTx(ctx context.Context, fn func(ingest.Tx) error) error {
	return r.db.BeginTx(ctx, func(tx *sqlx.Tx) error {
		return fn(&txRepo{s: r.s.WithTx(tx), q: tx})
	})
}

type txRepo struct {
	s *store.Store
	q db.Querier
}

var _ ingest.Tx = (*txRepo)(nil)

func (t *txRepo) LockProjectSpecs(ctx context.Context, projectID string) error {
	return t.s.LockProjectSpecs(ctx, projectID)
}

func (t *txRepo) LockSpec(ctx context.Context, projectID, name string) (base.Spec, bool, error) {
	return t.s.LockSpec(ctx, projectID, name)
}

func (t *txRepo) CountSpecs(ctx context.Context, projectID string) (int, error) {
	return t.s.CountSpecs(ctx, projectID)
}

func (t *txRepo) ReplaceEndpoints(ctx context.Context, specID string, es []base.Endpoint) error {
	return t.s.ReplaceEndpoints(ctx, specID, es)
}

func (t *txRepo) InsertSpec(ctx context.Context, s *base.Spec) error {
	const q = `INSERT INTO api_specs (project_id, name, source_path, title, api_version, openapi_version,
    content, content_format, content_hash, size_bytes,
    created_at, created_by, created_by_label, updated_at, updated_by, updated_by_label)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$11,$12,$13)
RETURNING id::text`
	var id string
	err := t.q.GetContext(ctx, &id, q, s.ProjectID, s.Name, s.SourcePath, s.Title, s.APIVersion, s.OpenAPIVersion,
		s.Content, s.Format, s.Hash[:], s.SizeBytes, s.CreatedAt.UTC(), s.CreatedBy, s.CreatedByLabel)
	if err != nil {
		if mapped := store.MapNameTaken(err); mapped == store.ErrNameTaken {
			return mapped
		}
		return fmt.Errorf("busupsertspecs: insert spec: %w", err)
	}
	s.ID = id
	return nil
}

func (t *txRepo) UpdateSpecContent(ctx context.Context, s base.Spec) error {
	const q = `UPDATE api_specs SET source_path=$3, title=$4, api_version=$5, openapi_version=$6,
    content=$7, content_format=$8, content_hash=$9, size_bytes=$10,
    updated_at=$11, updated_by=$12, updated_by_label=$13
WHERE project_id=$1 AND id=$2`
	_, err := t.q.ExecContext(ctx, q, s.ProjectID, s.ID, s.SourcePath, s.Title, s.APIVersion, s.OpenAPIVersion,
		s.Content, s.Format, s.Hash[:], s.SizeBytes, s.UpdatedAt.UTC(), s.UpdatedBy, s.UpdatedByLabel)
	if err != nil {
		return fmt.Errorf("busupsertspecs: update spec: %w", err)
	}
	return nil
}
