// Package store berisi query SQL yang identik lintas feature module auth (rule 5.3).
// Query yang dipakai satu feature saja tetap di repository.go feature itu.
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jmoiron/sqlx"

	"grimoire/internal/domain/core/internal/auth/base"
	"grimoire/internal/infra/db"
)

const (
	pgUniqueViolation = "23505"
	emailUniqueName   = "users_email_key"
)

// userCols memilih kolom user dengan cast ke text agar uuid/citext terbaca sebagai string.
const userCols = `id::text AS id, email::text AS email, name, password_hash, role, is_active, must_change_password,
	created_at, created_by::text AS created_by, created_by_label, updated_at, last_login_at`

// userRow adalah baris tabel users; base.User sengaja tanpa tag db.
type userRow struct {
	ID                 string       `db:"id"`
	Email              string       `db:"email"`
	Name               string       `db:"name"`
	PasswordHash       string       `db:"password_hash"`
	Role               string       `db:"role"`
	IsActive           bool         `db:"is_active"`
	MustChangePassword bool         `db:"must_change_password"`
	CreatedAt          time.Time    `db:"created_at"`
	CreatedBy          *string      `db:"created_by"`
	CreatedByLabel     *string      `db:"created_by_label"`
	UpdatedAt          time.Time    `db:"updated_at"`
	LastLoginAt        sql.NullTime `db:"last_login_at"`
}

func (r userRow) toUser() base.User {
	u := base.User{
		ID: r.ID, Email: r.Email, Name: r.Name, PasswordHash: r.PasswordHash, Role: r.Role,
		IsActive: r.IsActive, MustChangePassword: r.MustChangePassword, CreatedAt: r.CreatedAt,
		CreatedBy: r.CreatedBy, CreatedByLabel: r.CreatedByLabel, UpdatedAt: r.UpdatedAt,
	}
	if r.LastLoginAt.Valid {
		t := r.LastLoginAt.Time
		u.LastLoginAt = &t
	}
	return u
}

// Store membungkus db.Querier (DB atau transaksi).
type Store struct{ q db.Querier }

// New membuat Store di atas q.
func New(q db.Querier) *Store { return &Store{q: q} }

// WithTx mengembalikan Store yang berjalan di dalam tx.
func (s *Store) WithTx(tx *sqlx.Tx) *Store { return &Store{q: tx} }

// canonicalUUID mengembalikan bentuk kanonik id, atau ok=false bila bukan uuid.
func canonicalUUID(id string) (string, bool) {
	u, err := uuid.Parse(id)
	if err != nil || len(id) != 36 {
		return "", false
	}
	return u.String(), true
}

func (s *Store) getUser(ctx context.Context, query string, arg any) (base.User, bool, error) {
	var r userRow
	if err := s.q.GetContext(ctx, &r, query, arg); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return base.User{}, false, nil
		}
		return base.User{}, false, fmt.Errorf("store: ambil user: %w", err)
	}
	return r.toUser(), true, nil
}

// FindUserByID mengembalikan user. id yang bukan uuid -> (zero, false, nil) tanpa query.
func (s *Store) FindUserByID(ctx context.Context, id string) (base.User, bool, error) {
	cid, ok := canonicalUUID(id)
	if !ok {
		return base.User{}, false, nil
	}
	return s.getUser(ctx, `SELECT `+userCols+` FROM users WHERE id = $1::uuid`, cid)
}

// FindUserByEmail mencari user (email citext: tidak peka huruf besar/kecil).
func (s *Store) FindUserByEmail(ctx context.Context, email string) (base.User, bool, error) {
	return s.getUser(ctx, `SELECT `+userCols+` FROM users WHERE email = $1`, email)
}

// LockUserByID seperti FindUserByID dengan FOR UPDATE. Wajib dalam transaksi.
func (s *Store) LockUserByID(ctx context.Context, id string) (base.User, bool, error) {
	cid, ok := canonicalUUID(id)
	if !ok {
		return base.User{}, false, nil
	}
	return s.getUser(ctx, `SELECT `+userCols+` FROM users WHERE id = $1::uuid FOR UPDATE`, cid)
}

// LockActiveSuperAdminIDs mengunci semua super admin aktif, urut id (urutan kunci konsisten
// mencegah deadlock). Urutan kunci pemanggil: set ini dulu, baru baris target. Wajib dalam transaksi.
func (s *Store) LockActiveSuperAdminIDs(ctx context.Context) ([]string, error) {
	ids := []string{}
	err := s.q.SelectContext(ctx, &ids,
		`SELECT id::text FROM users WHERE role = 'super_admin' AND is_active ORDER BY id FOR UPDATE`)
	if err != nil {
		return nil, fmt.Errorf("store: kunci super admin aktif: %w", err)
	}
	return ids, nil
}

// InsertUser menyimpan user baru dan mengembalikan baris lengkap. Email bentrok -> base.ErrEmailTaken.
// CreatedAt/UpdatedAt nol -> now() DB.
func (s *Store) InsertUser(ctx context.Context, u base.User) (base.User, error) {
	var created, updated any
	if !u.CreatedAt.IsZero() {
		created = u.CreatedAt
	}
	if !u.UpdatedAt.IsZero() {
		updated = u.UpdatedAt
	}
	var r userRow
	err := s.q.GetContext(ctx, &r, `
INSERT INTO users (email, name, password_hash, role, is_active, must_change_password,
	created_at, created_by, created_by_label, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, COALESCE($7::timestamptz, now()), $8::uuid, $9, COALESCE($10::timestamptz, now()))
RETURNING `+userCols,
		u.Email, u.Name, u.PasswordHash, u.Role, u.IsActive, u.MustChangePassword,
		created, u.CreatedBy, u.CreatedByLabel, updated)
	if err != nil {
		var pg *pgconn.PgError
		if errors.As(err, &pg) && pg.Code == pgUniqueViolation && pg.ConstraintName == emailUniqueName {
			return base.User{}, base.ErrEmailTaken
		}
		return base.User{}, fmt.Errorf("store: simpan user: %w", err)
	}
	return r.toUser(), nil
}

// UpdatePassword mengganti hash dan flag wajib-ganti. User tidak ada -> base.ErrUserNotFound.
func (s *Store) UpdatePassword(ctx context.Context, id, hash string, mustChange bool, now time.Time) error {
	cid, ok := canonicalUUID(id)
	if !ok {
		return base.ErrUserNotFound
	}
	res, err := s.q.ExecContext(ctx,
		`UPDATE users SET password_hash = $2, must_change_password = $3, updated_at = $4 WHERE id = $1::uuid`,
		cid, hash, mustChange, now)
	if err != nil {
		return fmt.Errorf("store: ganti password: %w", err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return base.ErrUserNotFound
	}
	return nil
}

// DeleteSessionsByUser menghapus sesi user. exceptTokenHash != nil dikecualikan (sesi saat ini).
// Mengembalikan jumlah baris terhapus.
func (s *Store) DeleteSessionsByUser(ctx context.Context, userID string, exceptTokenHash []byte) (int64, error) {
	cid, ok := canonicalUUID(userID)
	if !ok {
		return 0, nil
	}
	var (
		res sql.Result
		err error
	)
	if exceptTokenHash == nil {
		res, err = s.q.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = $1::uuid`, cid)
	} else {
		res, err = s.q.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = $1::uuid AND token_hash <> $2`, cid, exceptTokenHash)
	}
	if err != nil {
		return 0, fmt.Errorf("store: hapus sesi: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("store: hitung sesi terhapus: %w", err)
	}
	return n, nil
}
