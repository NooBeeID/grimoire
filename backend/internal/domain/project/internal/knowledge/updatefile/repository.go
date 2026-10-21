package updatefile

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"grimoire/internal/domain/project/internal/knowledge/base"
	"grimoire/internal/domain/project/internal/knowledge/base/store"
)

const updateSQL = `
WITH old AS (
    SELECT id, size_bytes
    FROM knowledge_files
    WHERE project_id = $1 AND path = $2 AND content_hash = $3
    FOR UPDATE
)
UPDATE knowledge_files f
SET content = $4, title = $5, content_hash = $6, size_bytes = $7,
    updated_at = now(), updated_by = $8, updated_by_label = $9
FROM old
WHERE f.id = old.id
RETURNING f.id::text AS id, f.project_id::text AS project_id, f.path, f.title, f.content, f.content_hash, f.size_bytes,
          f.created_at, f.created_by::text AS created_by, f.created_by_label,
          f.updated_at, f.updated_by::text AS updated_by, f.updated_by_label,
          old.size_bytes AS bytes_before`

type updatedRow struct {
	ID             string    `db:"id"`
	ProjectID      string    `db:"project_id"`
	Path           string    `db:"path"`
	Title          string    `db:"title"`
	Content        string    `db:"content"`
	Hash           []byte    `db:"content_hash"`
	SizeBytes      int       `db:"size_bytes"`
	CreatedAt      time.Time `db:"created_at"`
	CreatedBy      string    `db:"created_by"`
	CreatedByLabel string    `db:"created_by_label"`
	UpdatedAt      time.Time `db:"updated_at"`
	UpdatedBy      string    `db:"updated_by"`
	UpdatedByLabel string    `db:"updated_by_label"`
	BytesBefore    int       `db:"bytes_before"`
}

func (r updatedRow) entity() base.File {
	return base.File{
		ID: r.ID, ProjectID: r.ProjectID, Path: r.Path, Title: r.Title, Content: r.Content,
		Hash: append([]byte(nil), r.Hash...), SizeBytes: r.SizeBytes,
		CreatedAt: r.CreatedAt.UTC(), CreatedBy: r.CreatedBy, CreatedByLabel: r.CreatedByLabel,
		UpdatedAt: r.UpdatedAt.UTC(), UpdatedBy: r.UpdatedBy, UpdatedByLabel: r.UpdatedByLabel,
	}
}

// Repo membungkus store bersama.
type Repo struct{ s *store.Store }

var _ Repository = (*Repo)(nil)

func NewRepo(s *store.Store) *Repo { return &Repo{s: s} }

// FindByPath mendelegasikan ke store.FindByPath.
func (r *Repo) FindByPath(ctx context.Context, projectID, path string) (base.File, bool, error) {
	return r.s.FindByPath(ctx, projectID, path)
}

// UpdateIfVersion menjalankan UPDATE atomik. found=false bila 0 baris (path tidak ada ATAU hash berbeda).
func (r *Repo) UpdateIfVersion(ctx context.Context, projectID, path string, expectedHash []byte, f base.File) (base.File, int, bool, error) {
	if !store.ValidUUID(projectID) {
		return base.File{}, 0, false, nil
	}
	var row updatedRow
	err := r.s.Q().GetContext(ctx, &row, updateSQL, projectID, path, expectedHash,
		f.Content, f.Title, f.Hash, f.SizeBytes, f.UpdatedBy, f.UpdatedByLabel)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return base.File{}, 0, false, nil
		}
		return base.File{}, 0, false, fmt.Errorf("updatefile: update: %w", err)
	}
	return row.entity(), row.BytesBefore, true, nil
}
