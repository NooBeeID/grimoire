// Package store berisi SQL bersama lintas feature module api-spec.
// SQL khusus satu feature tetap di repository.go feature itu.
package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jmoiron/sqlx"

	"grimoire/internal/domain/project/internal/apispec/base"
	"grimoire/internal/infra/db"
)

// ConstraintProjectName adalah nama constraint unik (project_id, name).
const ConstraintProjectName = "api_specs_project_name_key"

// ErrNameTaken dikembalikan Violation-aware helper bila 23505 pada ConstraintProjectName.
var ErrNameTaken = errors.New("store: spec name taken")

// MapNameTaken mengubah error 23505 pada constraint nama menjadi ErrNameTaken; selain itu err apa adanya.
func MapNameTaken(err error) error {
	var pe *pgconn.PgError
	if errors.As(err, &pe) && pe.Code == "23505" && pe.ConstraintName == ConstraintProjectName {
		return ErrNameTaken
	}
	return err
}

// Daftar kolom yang men-scan baris spec (alias tabel `s`).
const (
	metaColumns = "s.id::text AS id, s.project_id::text AS project_id, s.name, s.source_path, s.title, " +
		"s.api_version, s.openapi_version, s.content_format AS format, s.size_bytes, " +
		"s.created_at, s.created_by_label, s.updated_at, s.updated_by_label"
	specColumns = "s.id::text AS id, s.project_id::text AS project_id, s.name, s.source_path, s.title, " +
		"s.api_version, s.openapi_version, s.content, s.content_format AS format, s.content_hash, s.size_bytes, " +
		"s.created_at, s.created_by::text AS created_by, s.created_by_label, " +
		"s.updated_at, s.updated_by::text AS updated_by, s.updated_by_label"
)

type specRow struct {
	ID             string    `db:"id"`
	ProjectID      string    `db:"project_id"`
	Name           string    `db:"name"`
	SourcePath     *string   `db:"source_path"`
	Title          string    `db:"title"`
	APIVersion     string    `db:"api_version"`
	OpenAPIVersion string    `db:"openapi_version"`
	Content        string    `db:"content"`
	Format         string    `db:"format"`
	ContentHash    []byte    `db:"content_hash"`
	SizeBytes      int       `db:"size_bytes"`
	CreatedAt      time.Time `db:"created_at"`
	CreatedBy      string    `db:"created_by"`
	CreatedByLabel string    `db:"created_by_label"`
	UpdatedAt      time.Time `db:"updated_at"`
	UpdatedBy      string    `db:"updated_by"`
	UpdatedByLabel string    `db:"updated_by_label"`
}

func (r specRow) entity() base.Spec {
	var h [32]byte
	copy(h[:], r.ContentHash)
	return base.Spec{
		ID: r.ID, ProjectID: r.ProjectID, Name: r.Name, SourcePath: r.SourcePath, Title: r.Title,
		APIVersion: r.APIVersion, OpenAPIVersion: r.OpenAPIVersion, Content: r.Content, Format: r.Format,
		Hash: h, SizeBytes: r.SizeBytes, CreatedAt: r.CreatedAt.UTC(), CreatedBy: r.CreatedBy,
		CreatedByLabel: r.CreatedByLabel, UpdatedAt: r.UpdatedAt.UTC(), UpdatedBy: r.UpdatedBy,
		UpdatedByLabel: r.UpdatedByLabel,
	}
}

type summaryRow struct {
	ID             string    `db:"id"`
	ProjectID      string    `db:"project_id"`
	Name           string    `db:"name"`
	SourcePath     *string   `db:"source_path"`
	Title          string    `db:"title"`
	APIVersion     string    `db:"api_version"`
	OpenAPIVersion string    `db:"openapi_version"`
	Format         string    `db:"format"`
	SizeBytes      int       `db:"size_bytes"`
	EndpointCount  int       `db:"endpoint_count"`
	CreatedAt      time.Time `db:"created_at"`
	CreatedByLabel string    `db:"created_by_label"`
	UpdatedAt      time.Time `db:"updated_at"`
	UpdatedByLabel string    `db:"updated_by_label"`
}

func (r summaryRow) entity() base.SpecSummary {
	return base.SpecSummary{
		ID: r.ID, ProjectID: r.ProjectID, Name: r.Name, SourcePath: r.SourcePath, Title: r.Title,
		APIVersion: r.APIVersion, OpenAPIVersion: r.OpenAPIVersion, Format: r.Format, SizeBytes: r.SizeBytes,
		EndpointCount: r.EndpointCount, CreatedAt: r.CreatedAt.UTC(), CreatedByLabel: r.CreatedByLabel,
		UpdatedAt: r.UpdatedAt.UTC(), UpdatedByLabel: r.UpdatedByLabel,
	}
}

type endpointRow struct {
	Method      string  `db:"method"`
	Path        string  `db:"path"`
	OperationID *string `db:"operation_id"`
	Summary     string  `db:"summary"`
	Tags        string  `db:"tags"` // JSON array via to_json
	Deprecated  bool    `db:"deprecated"`
}

// Store membungkus Querier (DB atau Tx).
type Store struct{ q db.Querier }

func New(q db.Querier) *Store { return &Store{q: q} }

// WithTx mengembalikan Store yang berjalan di dalam tx.
func (s *Store) WithTx(tx *sqlx.Tx) *Store { return &Store{q: tx} }

// Querier untuk Exec/Query sederhana di repository feature.
func (s *Store) Querier() db.Querier { return s.q }

// MetaColumns/SpecColumns diekspos agar repository feature memakai daftar kolom yang sama.
func MetaColumns() string { return metaColumns }
func SpecColumns() string { return specColumns }

func validUUID(ids ...string) bool {
	for _, id := range ids {
		if _, err := uuid.Parse(id); err != nil {
			return false
		}
	}
	return true
}

// GetSpecQuery menjalankan query yang mengembalikan SpecColumns satu baris (juga INSERT/UPDATE ... RETURNING).
// Tidak ada baris -> (zero, false, nil).
func (s *Store) GetSpecQuery(ctx context.Context, query string, args ...any) (base.Spec, bool, error) {
	var r specRow
	if err := s.q.GetContext(ctx, &r, query, args...); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return base.Spec{}, false, nil
		}
		return base.Spec{}, false, err
	}
	return r.entity(), true, nil
}

// FindSpec memuat spec beserta content. projectID bukan uuid -> (zero, false, nil) tanpa query.
func (s *Store) FindSpec(ctx context.Context, projectID, name string) (base.Spec, bool, error) {
	if !validUUID(projectID) {
		return base.Spec{}, false, nil
	}
	return s.GetSpecQuery(ctx, `SELECT `+specColumns+` FROM api_specs s WHERE s.project_id = $1 AND s.name = $2`, projectID, name)
}

// LockSpec seperti FindSpec tetapi FOR UPDATE; wajib di dalam tx.
func (s *Store) LockSpec(ctx context.Context, projectID, name string) (base.Spec, bool, error) {
	if !validUUID(projectID) {
		return base.Spec{}, false, nil
	}
	return s.GetSpecQuery(ctx, `SELECT `+specColumns+` FROM api_specs s WHERE s.project_id = $1 AND s.name = $2 FOR UPDATE`, projectID, name)
}

const summarySelect = `SELECT ` + metaColumns + `, (SELECT count(*) FROM api_spec_endpoints e WHERE e.spec_id = s.id)::int AS endpoint_count FROM api_specs s `

// FindSpecMeta memuat metadata tanpa content, beserta jumlah endpoint.
func (s *Store) FindSpecMeta(ctx context.Context, projectID, name string) (base.SpecSummary, bool, error) {
	if !validUUID(projectID) {
		return base.SpecSummary{}, false, nil
	}
	var r summaryRow
	if err := s.q.GetContext(ctx, &r, summarySelect+`WHERE s.project_id = $1 AND s.name = $2`, projectID, name); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return base.SpecSummary{}, false, nil
		}
		return base.SpecSummary{}, false, err
	}
	return r.entity(), true, nil
}

// ListSpecs mengembalikan semua spec project urut nama; tidak pernah nil.
func (s *Store) ListSpecs(ctx context.Context, projectID string) ([]base.SpecSummary, error) {
	out := []base.SpecSummary{}
	if !validUUID(projectID) {
		return out, nil
	}
	var rows []summaryRow
	if err := s.q.SelectContext(ctx, &rows, summarySelect+`WHERE s.project_id = $1 ORDER BY s.name`, projectID); err != nil {
		return nil, err
	}
	for _, r := range rows {
		out = append(out, r.entity())
	}
	return out, nil
}

// ListSpecNames mengembalikan nama spec project urut nama; tidak pernah nil.
func (s *Store) ListSpecNames(ctx context.Context, projectID string) ([]string, error) {
	out := []string{}
	if !validUUID(projectID) {
		return out, nil
	}
	if err := s.q.SelectContext(ctx, &out, `SELECT name FROM api_specs WHERE project_id = $1 ORDER BY name`, projectID); err != nil {
		return nil, err
	}
	if out == nil {
		out = []string{}
	}
	return out, nil
}

// CountSpecs menghitung spec project.
func (s *Store) CountSpecs(ctx context.Context, projectID string) (int, error) {
	if !validUUID(projectID) {
		return 0, nil
	}
	var n int
	err := s.q.GetContext(ctx, &n, `SELECT count(*) FROM api_specs WHERE project_id = $1`, projectID)
	return n, err
}

// LockProjectSpecs mengambil advisory lock transaksi per project; wajib di dalam tx.
func (s *Store) LockProjectSpecs(ctx context.Context, projectID string) error {
	_, err := s.q.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('apispec:' || $1::text, 0))`, projectID)
	return err
}

// ListSpecEndpoints mengembalikan endpoint spec dalam urutan kanonik; tidak pernah nil.
func (s *Store) ListSpecEndpoints(ctx context.Context, specID string) ([]base.Endpoint, error) {
	out := []base.Endpoint{}
	if !validUUID(specID) {
		return out, nil
	}
	var rows []endpointRow
	err := s.q.SelectContext(ctx, &rows, `SELECT method, path, operation_id, summary, to_json(tags)::text AS tags, deprecated
FROM api_spec_endpoints e WHERE spec_id = $1
ORDER BY path, array_position(ARRAY['GET','PUT','POST','DELETE','OPTIONS','HEAD','PATCH','TRACE'], e.method)`, specID)
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		tags := []string{}
		if err := json.Unmarshal([]byte(r.Tags), &tags); err != nil {
			return nil, err
		}
		if tags == nil {
			tags = []string{}
		}
		out = append(out, base.Endpoint{Method: r.Method, Path: r.Path, OperationID: r.OperationID,
			Summary: r.Summary, Tags: tags, Deprecated: r.Deprecated})
	}
	return out, nil
}

// ReplaceEndpoints mengganti seluruh endpoint spec; wajib di dalam tx.
func (s *Store) ReplaceEndpoints(ctx context.Context, specID string, es []base.Endpoint) error {
	if _, err := s.q.ExecContext(ctx, `DELETE FROM api_spec_endpoints WHERE spec_id = $1`, specID); err != nil {
		return err
	}
	if len(es) == 0 {
		return nil
	}
	methods := make([]string, len(es))
	paths := make([]string, len(es))
	opIDs := make([]string, len(es)) // "" -> NULL
	sums := make([]string, len(es))
	tags := make([]string, len(es))
	deps := make([]bool, len(es))
	for i, e := range es {
		methods[i], paths[i], sums[i], deps[i] = e.Method, e.Path, e.Summary, e.Deprecated
		if e.OperationID != nil {
			opIDs[i] = *e.OperationID
		}
		t := e.Tags
		if t == nil {
			t = []string{}
		}
		b, err := json.Marshal(t)
		if err != nil {
			return err
		}
		tags[i] = string(b)
	}
	_, err := s.q.ExecContext(ctx, `INSERT INTO api_spec_endpoints (spec_id, method, path, operation_id, summary, tags, deprecated)
SELECT $1::uuid, u.method, u.path, NULLIF(u.operation_id, ''), u.summary,
       ARRAY(SELECT jsonb_array_elements_text(u.tags::jsonb)), u.deprecated
FROM unnest($2::text[], $3::text[], $4::text[], $5::text[], $6::text[], $7::bool[])
     AS u(method, path, operation_id, summary, tags, deprecated)`,
		specID, methods, paths, opIDs, sums, tags, deps)
	return err
}

// CountEndpoints menghitung endpoint spec.
func (s *Store) CountEndpoints(ctx context.Context, specID string) (int, error) {
	if !validUUID(specID) {
		return 0, nil
	}
	var n int
	err := s.q.GetContext(ctx, &n, `SELECT count(*) FROM api_spec_endpoints WHERE spec_id = $1`, specID)
	return n, err
}
