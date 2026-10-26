package backfillsections

import (
	"context"
	"fmt"

	"grimoire/internal/domain/project/internal/knowledge/base/store"
)

const nullSectionsSQL = `SELECT id::text AS id, content FROM knowledge_files WHERE sections IS NULL ORDER BY id LIMIT $1`

// Hanya mengisi baris yang masih NULL: writer yang lebih baru tidak ditimpa.
const setSectionsSQL = `UPDATE knowledge_files SET sections = $2::jsonb WHERE id = $1::uuid AND sections IS NULL`

// Pending adalah satu baris yang belum punya sections.
type Pending struct {
	ID      string `db:"id"`
	Content string `db:"content"`
}

type Repo struct{ s *store.Store }

var _ repository = (*Repo)(nil)

func NewRepository(s *store.Store) *Repo { return &Repo{s: s} }

// NullSections mengambil sampai limit baris dengan sections NULL.
func (r *Repo) NullSections(ctx context.Context, limit int) ([]Pending, error) {
	var rows []Pending
	if err := r.s.Q().SelectContext(ctx, &rows, nullSectionsSQL, limit); err != nil {
		return nil, fmt.Errorf("backfillsections: select: %w", err)
	}
	return rows, nil
}

// SetSections mengisi sections hanya bila masih NULL.
func (r *Repo) SetSections(ctx context.Context, id string, sections []byte) error {
	if _, err := r.s.Q().ExecContext(ctx, setSectionsSQL, id, string(sections)); err != nil {
		return fmt.Errorf("backfillsections: set: %w", err)
	}
	return nil
}
