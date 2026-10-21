package movefile

import (
	"context"
	"fmt"

	"grimoire/internal/domain/project/internal/knowledge/base"
	"grimoire/internal/domain/project/internal/knowledge/base/store"
)

const existsSQL = `SELECT EXISTS(SELECT 1 FROM knowledge_files WHERE project_id = $1 AND path = $2)`

const renameSQL = `UPDATE knowledge_files
   SET path = $3, updated_at = now(), updated_by = $4, updated_by_label = $5
 WHERE project_id = $1 AND path = $2`

// Repo membungkus store bersama.
type Repo struct{ s *store.Store }

var _ Repository = (*Repo)(nil)

func NewRepo(s *store.Store) *Repo { return &Repo{s: s} }

// InTx menjalankan fn dengan Repo terikat tx.
func (r *Repo) InTx(ctx context.Context, fn func(Repository) error) error {
	return r.s.InTx(ctx, func(ts *store.Store) error { return fn(&Repo{s: ts}) })
}

func (r *Repo) LockProject(ctx context.Context, projectID string) error {
	return r.s.LockProject(ctx, projectID)
}

func (r *Repo) FindByPath(ctx context.Context, projectID, path string) (base.File, bool, error) {
	return r.s.FindByPath(ctx, projectID, path)
}

// PathExists: projectID bukan uuid -> false tanpa query.
func (r *Repo) PathExists(ctx context.Context, projectID, path string) (bool, error) {
	if !store.ValidUUID(projectID) {
		return false, nil
	}
	var ok bool
	if err := r.s.Q().GetContext(ctx, &ok, existsSQL, projectID, path); err != nil {
		return false, err
	}
	return ok, nil
}

// Rename hanya mengubah path dan jejak perubah. Mengembalikan RowsAffected.
func (r *Repo) Rename(ctx context.Context, projectID, from, to string, actor base.Actor) (int64, error) {
	if !store.ValidUUID(projectID) {
		return 0, nil
	}
	res, err := r.s.Q().ExecContext(ctx, renameSQL, projectID, from, to, actor.ID, actor.Label)
	if err != nil {
		if store.IsUniqueViolation(err) {
			return 0, base.ErrPathExists
		}
		return 0, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("rows affected: %w", err)
	}
	return n, nil
}
