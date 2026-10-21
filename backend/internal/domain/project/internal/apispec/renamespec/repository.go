package renamespec

import (
	"context"
	"errors"
	"fmt"

	"github.com/jmoiron/sqlx"

	"grimoire/internal/domain/project/internal/apispec/base"
	"grimoire/internal/domain/project/internal/apispec/base/store"
	"grimoire/internal/infra/db"
)

type Repo struct {
	db *db.DB
	s  *store.Store
}

func newRepo(d *db.DB, s *store.Store) *Repo { return &Repo{db: d, s: s} }

var _ Repository = (*Repo)(nil)

// InTx menjalankan fn dalam satu transaksi; Tx terikat ke transaksi itu.
func (r *Repo) InTx(ctx context.Context, fn func(Tx) error) error {
	return r.db.BeginTx(ctx, func(tx *sqlx.Tx) error {
		return fn(&txRepo{s: r.s.WithTx(tx), q: tx})
	})
}

type txRepo struct {
	s *store.Store
	q db.Querier
}

var _ Tx = (*txRepo)(nil)

func (t *txRepo) LockSpec(ctx context.Context, projectID, name string) (base.Spec, bool, error) {
	return t.s.LockSpec(ctx, projectID, name)
}

func (t *txRepo) FindSpecMeta(ctx context.Context, projectID, name string) (base.SpecSummary, bool, error) {
	return t.s.FindSpecMeta(ctx, projectID, name)
}

// RenameSpec menulis nama dan metadata update; unique violation -> store.ErrNameTaken.
func (t *txRepo) RenameSpec(ctx context.Context, spec base.Spec) error {
	const q = `UPDATE api_specs SET name=$2, updated_at=$3, updated_by=$4, updated_by_label=$5 WHERE id=$1`
	res, err := t.q.ExecContext(ctx, q, spec.ID, spec.Name, spec.UpdatedAt.UTC(), spec.UpdatedBy, spec.UpdatedByLabel)
	if err != nil {
		if mapped := store.MapNameTaken(err); errors.Is(mapped, store.ErrNameTaken) {
			return store.ErrNameTaken
		}
		return fmt.Errorf("renamespec: rename: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("renamespec: rename: %w", err)
	}
	if n == 0 {
		return errors.New("renamespec: rename: baris hilang")
	}
	return nil
}
