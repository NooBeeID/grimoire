// Package store berisi SQL bersama lintas feature module searchsynonym. SQL khusus feature tetap di
// repository.go feature itu. Semua query dibatasi project_id (rule 6.10); id bukan uuid -> tidak ketemu
// tanpa query.
package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"grimoire/internal/domain/project/internal/searchsynonym/base"
	"grimoire/internal/infra/db"
)

// GroupColumns dipakai semua SELECT/RETURNING yang di-scan ke groupRow.
const GroupColumns = "id::text AS id, project_id::text AS project_id, to_jsonb(terms) AS terms, " +
	"created_at, created_by::text AS created_by, created_by_label, " +
	"updated_at, updated_by::text AS updated_by, updated_by_label"

// jsonStrings memindai kolom jsonb array string.
type jsonStrings []string

func (j *jsonStrings) Scan(src any) error {
	var raw []byte
	switch v := src.(type) {
	case []byte:
		raw = v
	case string:
		raw = []byte(v)
	case nil:
		*j = nil
		return nil
	default:
		return fmt.Errorf("store: scan terms: tipe %T", src)
	}
	return json.Unmarshal(raw, j)
}

type groupRow struct {
	ID             string      `db:"id"`
	ProjectID      string      `db:"project_id"`
	Terms          jsonStrings `db:"terms"`
	CreatedAt      time.Time   `db:"created_at"`
	CreatedBy      string      `db:"created_by"`
	CreatedByLabel string      `db:"created_by_label"`
	UpdatedAt      time.Time   `db:"updated_at"`
	UpdatedBy      string      `db:"updated_by"`
	UpdatedByLabel string      `db:"updated_by_label"`
}

func (r groupRow) toEntity() base.Group {
	return base.Group{
		ID: r.ID, ProjectID: r.ProjectID, Terms: append([]string(nil), r.Terms...),
		CreatedAt: r.CreatedAt.UTC(), CreatedBy: r.CreatedBy, CreatedByLabel: r.CreatedByLabel,
		UpdatedAt: r.UpdatedAt.UTC(), UpdatedBy: r.UpdatedBy, UpdatedByLabel: r.UpdatedByLabel,
	}
}

// Store membungkus *db.DB (di luar tx) atau tx (di dalam InTx).
type Store struct {
	db *db.DB // nil di dalam tx
	q  db.Querier
}

func New(d *db.DB) *Store { return &Store{db: d, q: d} }

// InTx menjalankan fn dengan Store yang terikat satu transaksi. Error fn dikembalikan apa adanya (rollback).
func (s *Store) InTx(ctx context.Context, fn func(*Store) error) error {
	if s.db == nil {
		return fn(s)
	}
	return s.db.BeginTx(ctx, func(tx *sqlx.Tx) error {
		return fn(&Store{q: tx})
	})
}

// Q mengembalikan Querier untuk SQL khusus feature (repository.go).
func (s *Store) Q() db.Querier { return s.q }

// ValidUUID memeriksa string uuid.
func ValidUUID(s string) bool {
	_, err := uuid.Parse(s)
	return err == nil
}

// LockProject mengambil advisory xact lock per project. WAJIB dipanggil di dalam InTx. Menyerialkan
// penulisan kamus sehingga keunikan term dan kuota kelompok per project terjaga.
func (s *Store) LockProject(ctx context.Context, projectID string) error {
	if !ValidUUID(projectID) {
		return nil
	}
	_, err := s.q.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('search-synonym:' || $1, 0))`, projectID)
	return err
}

// List mengembalikan semua kelompok project, urut created_at, id.
func (s *Store) List(ctx context.Context, projectID string) ([]base.Group, error) {
	if !ValidUUID(projectID) {
		return []base.Group{}, nil
	}
	var rows []groupRow
	if err := s.q.SelectContext(ctx, &rows,
		`SELECT `+GroupColumns+` FROM knowledge_synonym_groups WHERE project_id = $1 ORDER BY created_at, id`, projectID); err != nil {
		return nil, err
	}
	out := make([]base.Group, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.toEntity())
	}
	return out, nil
}

// Get mengembalikan satu kelompok; id atau projectID bukan uuid -> (zero, false, nil).
func (s *Store) Get(ctx context.Context, projectID, id string) (base.Group, bool, error) {
	if !ValidUUID(projectID) || !ValidUUID(id) {
		return base.Group{}, false, nil
	}
	return s.one(ctx, `SELECT `+GroupColumns+` FROM knowledge_synonym_groups WHERE project_id = $1 AND id = $2`, projectID, id)
}

func (s *Store) one(ctx context.Context, query string, args ...any) (base.Group, bool, error) {
	var r groupRow
	if err := s.q.GetContext(ctx, &r, query, args...); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return base.Group{}, false, nil
		}
		return base.Group{}, false, err
	}
	return r.toEntity(), true, nil
}

// Count menghitung kelompok project.
func (s *Store) Count(ctx context.Context, projectID string) (int, error) {
	if !ValidUUID(projectID) {
		return 0, nil
	}
	var n int
	err := s.q.GetContext(ctx, &n, `SELECT count(*) FROM knowledge_synonym_groups WHERE project_id = $1`, projectID)
	return n, err
}

// TermsInUse mengembalikan (urut alfabet, unik) term di `terms` yang sudah dipakai kelompok lain pada
// project yang sama. exceptID ("" = tidak ada) dikecualikan; project lain tidak dilihat.
func (s *Store) TermsInUse(ctx context.Context, projectID string, terms []string, exceptID string) ([]string, error) {
	if !ValidUUID(projectID) || len(terms) == 0 {
		return []string{}, nil
	}
	var out []string
	err := s.q.SelectContext(ctx, &out, `
SELECT DISTINCT t FROM knowledge_synonym_groups g, unnest(g.terms) AS t
WHERE g.project_id = $1 AND t = ANY($2::text[]) AND ($3::text = '' OR g.id::text <> $3)
ORDER BY t`, projectID, terms, exceptID)
	if err != nil {
		return nil, err
	}
	if out == nil {
		out = []string{}
	}
	return out, nil
}

// Insert menyimpan kelompok baru dan mengembalikan barisnya (id, waktu dari DB).
func (s *Store) Insert(ctx context.Context, g base.Group) (base.Group, error) {
	out, ok, err := s.one(ctx, `INSERT INTO knowledge_synonym_groups
  (project_id, terms, created_by, created_by_label, updated_by, updated_by_label)
VALUES ($1, $2::text[], $3::uuid, $4, $3::uuid, $4) RETURNING `+GroupColumns,
		g.ProjectID, g.Terms, g.CreatedBy, g.CreatedByLabel)
	if err == nil && !ok {
		err = sql.ErrNoRows
	}
	return out, err
}

// Update mengganti term dan jejak pengubah. ok=false bila kelompok tidak ada di project itu.
func (s *Store) Update(ctx context.Context, g base.Group) (base.Group, bool, error) {
	if !ValidUUID(g.ProjectID) || !ValidUUID(g.ID) {
		return base.Group{}, false, nil
	}
	return s.one(ctx, `UPDATE knowledge_synonym_groups
SET terms = $3::text[], updated_at = now(), updated_by = $4::uuid, updated_by_label = $5
WHERE project_id = $1 AND id = $2 RETURNING `+GroupColumns,
		g.ProjectID, g.ID, g.Terms, g.UpdatedBy, g.UpdatedByLabel)
}

// Delete menghapus kelompok dan mengembalikan barisnya. ok=false bila tidak ada.
func (s *Store) Delete(ctx context.Context, projectID, id string) (base.Group, bool, error) {
	if !ValidUUID(projectID) || !ValidUUID(id) {
		return base.Group{}, false, nil
	}
	return s.one(ctx, `DELETE FROM knowledge_synonym_groups WHERE project_id = $1 AND id = $2 RETURNING `+GroupColumns,
		projectID, id)
}
