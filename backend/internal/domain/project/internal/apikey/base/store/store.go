// Package store berisi scan baris dan helper SQL bersama lintas feature module api-key.
// SQL khusus feature tetap di repository.go feature itu.
package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"grimoire/internal/domain/project/internal/apikey/base"
	"grimoire/internal/infra/db"
)

// KeyColumns dipakai semua SELECT/RETURNING yang men-scan base.Key.
const KeyColumns = "id::text AS id, project_id::text AS project_id, name, prefix, secret_hash, " +
	"created_by::text AS created_by, created_by_label, created_at, " +
	"last_used_at, revoked_at, revoked_by::text AS revoked_by, revoked_by_label"

const (
	ConstraintPrefixUnique = "api_keys_prefix_key"
	ConstraintActiveName   = "api_keys_project_active_name_key"
)

type keyRow struct {
	ID             string     `db:"id"`
	ProjectID      string     `db:"project_id"`
	Name           string     `db:"name"`
	Prefix         string     `db:"prefix"`
	SecretHash     []byte     `db:"secret_hash"`
	CreatedBy      *string    `db:"created_by"`
	CreatedByLabel *string    `db:"created_by_label"`
	CreatedAt      time.Time  `db:"created_at"`
	LastUsedAt     *time.Time `db:"last_used_at"`
	RevokedAt      *time.Time `db:"revoked_at"`
	RevokedBy      *string    `db:"revoked_by"`
	RevokedByLabel *string    `db:"revoked_by_label"`
}

func utc(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	u := t.UTC()
	return &u
}

func (r keyRow) toEntity() base.Key {
	return base.Key{
		ID: r.ID, ProjectID: r.ProjectID, Name: r.Name, Prefix: r.Prefix,
		SecretHash: append([]byte(nil), r.SecretHash...),
		CreatedBy:  r.CreatedBy, CreatedByLabel: r.CreatedByLabel, CreatedAt: r.CreatedAt.UTC(),
		LastUsedAt: utc(r.LastUsedAt), RevokedAt: utc(r.RevokedAt),
		RevokedBy: r.RevokedBy, RevokedByLabel: r.RevokedByLabel,
	}
}

// Store membungkus Querier (DB atau Tx).
type Store struct{ q db.Querier }

func New(q db.Querier) *Store { return &Store{q: q} }

// WithTx mengembalikan Store yang berjalan di dalam tx.
func (s *Store) WithTx(tx *sqlx.Tx) *Store { return &Store{q: tx} }

// Querier untuk Exec/QueryRow sederhana di repository feature.
func (s *Store) Querier() db.Querier { return s.q }

// GetKey menjalankan query yang mengembalikan KeyColumns satu baris. Tidak ada baris -> (zero, false, nil).
func (s *Store) GetKey(ctx context.Context, query string, args ...any) (base.Key, bool, error) {
	var r keyRow
	if err := s.q.GetContext(ctx, &r, query, args...); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return base.Key{}, false, nil
		}
		return base.Key{}, false, err
	}
	return r.toEntity(), true, nil
}

// SelectKeys untuk banyak baris; hasil kosong -> slice kosong non-nil.
func (s *Store) SelectKeys(ctx context.Context, query string, args ...any) ([]base.Key, error) {
	var rows []keyRow
	if err := s.q.SelectContext(ctx, &rows, query, args...); err != nil {
		return nil, err
	}
	out := make([]base.Key, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.toEntity())
	}
	return out, nil
}

// ValidUUID memeriksa string uuid; repository memakainya untuk return "tidak ketemu" tanpa query.
func ValidUUID(s string) bool {
	_, err := uuid.Parse(s)
	return err == nil
}
