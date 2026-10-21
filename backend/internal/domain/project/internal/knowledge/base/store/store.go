// Package store berisi SQL bersama lintas feature module knowledge: lock kuota, hitung, cari path,
// daftar metadata, dan pencarian full-text. SQL khusus feature tetap di repository.go feature itu.
// Semua query dibatasi project_id (rule 6.10); projectID bukan uuid -> hasil kosong tanpa query.
package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jmoiron/sqlx"

	"grimoire/internal/domain/project/internal/knowledge/base"
	"grimoire/internal/infra/db"
)

const (
	// ConstraintPathKey adalah constraint unik (project_id, path).
	ConstraintPathKey = "knowledge_files_project_id_path_key"

	// FileColumns dipakai semua SELECT/RETURNING yang di-scan ke base.File lewat GetFile.
	FileColumns = "id::text AS id, project_id::text AS project_id, path, title, content, content_hash, size_bytes, " +
		"created_at, created_by::text AS created_by, created_by_label, " +
		"updated_at, updated_by::text AS updated_by, updated_by_label"

	// HeadlineOptions untuk ts_headline: marker \x01..\x02 (diurai base.ParseHeadline).
	HeadlineOptions = "StartSel=\x01, StopSel=\x02, MaxFragments=2, MaxWords=30, MinWords=10, FragmentDelimiter=\" … \""

	codeUniqueViolation = "23505"
)

const searchSQL = `
WITH q AS (SELECT websearch_to_tsquery('simple', $2) AS q),
hits AS (
  SELECT f.id, f.path, f.title,
         ts_rank_cd(f.search, q.q, 1)::float8 AS score,
         count(*) OVER () AS total
  FROM knowledge_files f, q
  WHERE f.project_id = $1 AND f.search @@ q.q AND f.path LIKE $3 ESCAPE '\'
  ORDER BY score DESC, f.path COLLATE "C"
  LIMIT $4
)
SELECT h.path, h.title, h.score, h.total,
       ts_headline('simple',
         replace(replace(left(f.content, 200000), E'\x01', ''), E'\x02', ''),
         q.q, $5) AS headline
FROM hits h JOIN knowledge_files f ON f.id = h.id, q
ORDER BY h.score DESC, h.path COLLATE "C"`

type fileRow struct {
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
}

func (r fileRow) toEntity() base.File {
	return base.File{
		ID: r.ID, ProjectID: r.ProjectID, Path: r.Path, Title: r.Title, Content: r.Content,
		Hash: append([]byte(nil), r.Hash...), SizeBytes: r.SizeBytes,
		CreatedAt: r.CreatedAt.UTC(), CreatedBy: r.CreatedBy, CreatedByLabel: r.CreatedByLabel,
		UpdatedAt: r.UpdatedAt.UTC(), UpdatedBy: r.UpdatedBy, UpdatedByLabel: r.UpdatedByLabel,
	}
}

type metaRow struct {
	Path      string    `db:"path"`
	SizeBytes int       `db:"size_bytes"`
	UpdatedAt time.Time `db:"updated_at"`
}

type hitRow struct {
	Path     string  `db:"path"`
	Title    string  `db:"title"`
	Score    float64 `db:"score"`
	Total    int     `db:"total"`
	Headline string  `db:"headline"`
}

// Store membungkus *db.DB (di luar tx) atau tx (di dalam InTx).
type Store struct {
	db *db.DB // nil di dalam tx
	q  db.Querier
}

func New(d *db.DB) *Store { return &Store{db: d, q: d} }

// InTx menjalankan fn dengan Store yang terikat satu transaksi. Error fn dikembalikan apa adanya
// (rollback). Dipanggil di dalam tx yang sudah ada, fn berjalan di tx itu juga.
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

// LockProject mengambil advisory xact lock per project (kunci teks sendiri, bukan baris projects).
// WAJIB dipanggil di dalam InTx. Menyerialkan penulisan yang menambah file sehingga kuota tidak terlampaui.
func (s *Store) LockProject(ctx context.Context, projectID string) error {
	if !ValidUUID(projectID) {
		return nil
	}
	_, err := s.q.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('knowledge:' || $1, 0))`, projectID)
	return err
}

func (s *Store) CountFiles(ctx context.Context, projectID string) (int, error) {
	if !ValidUUID(projectID) {
		return 0, nil
	}
	var n int
	err := s.q.GetContext(ctx, &n, `SELECT count(*) FROM knowledge_files WHERE project_id = $1`, projectID)
	return n, err
}

func (s *Store) FindByPath(ctx context.Context, projectID, path string) (base.File, bool, error) {
	if !ValidUUID(projectID) {
		return base.File{}, false, nil
	}
	return s.GetFile(ctx, `SELECT `+FileColumns+` FROM knowledge_files WHERE project_id = $1 AND path = $2`, projectID, path)
}

// GetFile menjalankan query yang mengembalikan FileColumns satu baris. Tidak ada baris -> (zero, false, nil).
func (s *Store) GetFile(ctx context.Context, query string, args ...any) (base.File, bool, error) {
	var r fileRow
	if err := s.q.GetContext(ctx, &r, query, args...); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return base.File{}, false, nil
		}
		return base.File{}, false, err
	}
	return r.toEntity(), true, nil
}

// ListMeta mengembalikan metadata (tanpa isi) semua file yang cocok dengan likePrefix
// (dari base.LikePrefix), urut path byte order.
func (s *Store) ListMeta(ctx context.Context, projectID, likePrefix string) ([]base.Meta, error) {
	if !ValidUUID(projectID) {
		return []base.Meta{}, nil
	}
	var rows []metaRow
	err := s.q.SelectContext(ctx, &rows,
		`SELECT path, size_bytes, updated_at FROM knowledge_files
		 WHERE project_id = $1 AND path LIKE $2 ESCAPE '\' ORDER BY path COLLATE "C"`, projectID, likePrefix)
	if err != nil {
		return nil, err
	}
	out := make([]base.Meta, 0, len(rows))
	for _, r := range rows {
		out = append(out, base.Meta{Path: r.Path, SizeBytes: r.SizeBytes, UpdatedAt: r.UpdatedAt.UTC()})
	}
	return out, nil
}

// ListPaths mengembalikan semua path project (untuk saran path mirip), urut byte order.
func (s *Store) ListPaths(ctx context.Context, projectID string) ([]string, error) {
	if !ValidUUID(projectID) {
		return []string{}, nil
	}
	paths := []string{}
	err := s.q.SelectContext(ctx, &paths,
		`SELECT path FROM knowledge_files WHERE project_id = $1 ORDER BY path COLLATE "C"`, projectID)
	return paths, err
}

// Search menjalankan pencarian full-text. tsq berasal dari base.ParseQuery (sudah aman), likePrefix dari
// base.LikePrefix. Mengembalikan hit teratas (maks limit) dan total dokumen cocok (0 bila tidak ada).
func (s *Store) Search(ctx context.Context, projectID, tsq, likePrefix string, limit int) ([]base.SearchHit, int, error) {
	if !ValidUUID(projectID) || tsq == "" {
		return []base.SearchHit{}, 0, nil
	}
	var rows []hitRow
	if err := s.q.SelectContext(ctx, &rows, searchSQL, projectID, tsq, likePrefix, limit, HeadlineOptions); err != nil {
		return nil, 0, err
	}
	hits := make([]base.SearchHit, 0, len(rows))
	total := 0
	for _, r := range rows {
		total = r.Total
		hits = append(hits, base.SearchHit{Path: r.Path, Title: r.Title, Score: r.Score, Headline: r.Headline})
	}
	return hits, total, nil
}

// IsUniqueViolation: err (atau yang dibungkusnya) adalah pelanggaran unik Postgres (23505).
func IsUniqueViolation(err error) bool {
	var pe *pgconn.PgError
	return errors.As(err, &pe) && pe.Code == codeUniqueViolation
}
