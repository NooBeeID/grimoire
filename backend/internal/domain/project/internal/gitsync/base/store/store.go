// Package store berisi SQL bersama module gitsync: kolom config, scan, dan pembacaan config.
// Semua query dibatasi project_id; projectID bukan uuid -> hasil kosong tanpa query.
// Penulisan (upsert config, SaveResult, lock) ada di repository runsync.
package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"grimoire/internal/domain/project/internal/gitsync/base"
	"grimoire/internal/infra/db"
)

// ConfigColumns dipakai semua SELECT/RETURNING yang di-scan ke Row. Kolom uuid di-cast ::text,
// last_result jsonb di-cast ::text.
const ConfigColumns = "id::text AS id, project_id::text AS project_id, kind, provider, host, repo_url, repo_path, " +
	"branch, folder, target_prefix, last_status, last_synced_at, last_synced_by::text AS last_synced_by, " +
	"last_synced_by_label, last_result::text AS last_result, created_at, updated_at"

// Row adalah satu baris git_sync_configs sesuai ConfigColumns.
type Row struct {
	ID                string     `db:"id"`
	ProjectID         string     `db:"project_id"`
	Kind              string     `db:"kind"`
	Provider          string     `db:"provider"`
	Host              string     `db:"host"`
	RepoURL           string     `db:"repo_url"`
	RepoPath          string     `db:"repo_path"`
	Branch            string     `db:"branch"`
	Folder            string     `db:"folder"`
	TargetPrefix      string     `db:"target_prefix"`
	LastStatus        *string    `db:"last_status"`
	LastSyncedAt      *time.Time `db:"last_synced_at"`
	LastSyncedBy      *string    `db:"last_synced_by"`
	LastSyncedByLabel *string    `db:"last_synced_by_label"`
	LastResult        *string    `db:"last_result"`
	CreatedAt         time.Time  `db:"created_at"`
	UpdatedAt         time.Time  `db:"updated_at"`
}

// ScanConfig memetakan Row ke entity. last_result rusak -> LastResult nil + warn tanpa isi
// (data historis tidak boleh mematikan dialog).
func ScanConfig(r Row) base.Config {
	c := base.Config{
		ID: r.ID, ProjectID: r.ProjectID, Kind: base.Kind(r.Kind), Provider: r.Provider, Host: r.Host,
		RepoURL: r.RepoURL, RepoPath: r.RepoPath, Branch: r.Branch, Folder: r.Folder, TargetPrefix: r.TargetPrefix,
		LastStatus: r.LastStatus, LastSyncedAt: r.LastSyncedAt, LastSyncedBy: r.LastSyncedBy,
		LastSyncedByLabel: r.LastSyncedByLabel, CreatedAt: r.CreatedAt.UTC(), UpdatedAt: r.UpdatedAt.UTC(),
	}
	if c.LastSyncedAt != nil {
		t := c.LastSyncedAt.UTC()
		c.LastSyncedAt = &t
	}
	if r.LastResult != nil {
		var d base.ReportDTO
		if err := json.Unmarshal([]byte(*r.LastResult), &d); err != nil {
			slog.Warn("gitsync: last_result tidak dapat dibaca", "project_id", r.ProjectID, "kind", r.Kind)
		} else {
			rep := d.ToReport()
			c.LastResult = &rep
		}
	}
	return c
}

// MarshalResult menyerialkan laporan untuk kolom last_result (dipotong MaxStoredReportItems).
func MarshalResult(r base.Report) ([]byte, error) {
	return json.Marshal(r.CapForStorage(base.MaxStoredReportItems).ToDTO())
}

// ValidUUID true bila semua id adalah uuid valid.
func ValidUUID(ids ...string) bool {
	for _, id := range ids {
		if _, err := uuid.Parse(id); err != nil {
			return false
		}
	}
	return true
}

// Store membungkus Querier (DB atau Tx).
type Store struct{ q db.Querier }

func New(q db.Querier) *Store { return &Store{q: q} }

// Querier untuk query sederhana di repository feature.
func (s *Store) Querier() db.Querier { return s.q }

// SelectConfig membaca config (project, kind). Tidak ada -> (zero, false, nil).
func (s *Store) SelectConfig(ctx context.Context, projectID string, kind base.Kind) (base.Config, bool, error) {
	if !ValidUUID(projectID) {
		return base.Config{}, false, nil
	}
	var r Row
	err := s.q.GetContext(ctx, &r, "SELECT "+ConfigColumns+" FROM git_sync_configs WHERE project_id = $1 AND kind = $2", projectID, string(kind))
	if errors.Is(err, sql.ErrNoRows) {
		return base.Config{}, false, nil
	}
	if err != nil {
		return base.Config{}, false, err
	}
	return ScanConfig(r), true, nil
}

// LockKeySQL adalah ekspresi kunci advisory lock (project, kind); $1 = project_id, $2 = kind.
// Paket ini satu-satunya pemilik teksnya: TryLock/Unlock (runsync) dan IsLocked memakai ini.
const LockKeySQL = "hashtext('grimoire.gitsync.' || $1::text || '.' || $2::text)"

// isLockedSQL membaca pg_locks tanpa mengambil lock. hashtext (int4) di-cast bigint agar
// sign-extension sama dengan pg_try_advisory_lock(bigint): hash negatif memberi classid 4294967295.
const isLockedSQL = "WITH k AS (SELECT (" + LockKeySQL + ")::bigint AS v) " +
	"SELECT EXISTS (SELECT 1 FROM pg_locks, k " +
	"WHERE locktype = 'advisory' AND granted AND objsubid = 1 " +
	"AND database = (SELECT oid FROM pg_database WHERE datname = current_database()) " +
	"AND classid = ((k.v >> 32) & 4294967295)::oid " +
	"AND objid = (k.v & 4294967295)::oid)"

// IsLocked true bila advisory lock (project, kind) sedang dipegang sesi mana pun. Tidak mengambil
// lock (jangan pakai pg_try_advisory_lock + unlock: bisa membuat run lain 409 palsu).
// projectID bukan uuid -> false tanpa query.
func (s *Store) IsLocked(ctx context.Context, projectID string, kind base.Kind) (bool, error) {
	if !ValidUUID(projectID) {
		return false, nil
	}
	var locked bool
	if err := s.q.GetContext(ctx, &locked, isLockedSQL, projectID, string(kind)); err != nil {
		return false, err
	}
	return locked, nil
}
