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

// invCols memilih kolom user_invitations dengan cast uuid ke text.
const invCols = `user_id::text AS user_id, invited_by::text AS invited_by, invited_by_label, issued_at, expires_at,
	sent_at, delivery_status, accepted_at, resend_count, resend_window_started_at, resend_window_count, created_at`

// invitationRow adalah baris user_invitations; base.Invitation sengaja tanpa tag db.
type invitationRow struct {
	UserID                string       `db:"user_id"`
	InvitedBy             string       `db:"invited_by"`
	InvitedByLabel        *string      `db:"invited_by_label"`
	IssuedAt              time.Time    `db:"issued_at"`
	ExpiresAt             time.Time    `db:"expires_at"`
	SentAt                sql.NullTime `db:"sent_at"`
	DeliveryStatus        string       `db:"delivery_status"`
	AcceptedAt            sql.NullTime `db:"accepted_at"`
	ResendCount           int          `db:"resend_count"`
	ResendWindowStartedAt sql.NullTime `db:"resend_window_started_at"`
	ResendWindowCount     int          `db:"resend_window_count"`
	CreatedAt             time.Time    `db:"created_at"`
}

func nullTimePtr(n sql.NullTime) *time.Time {
	if !n.Valid {
		return nil
	}
	t := n.Time
	return &t
}

func (r invitationRow) toInvitation() base.Invitation {
	return base.Invitation{
		UserID: r.UserID, InvitedBy: r.InvitedBy, InvitedByLabel: r.InvitedByLabel,
		IssuedAt: r.IssuedAt, ExpiresAt: r.ExpiresAt, CreatedAt: r.CreatedAt,
		SentAt: nullTimePtr(r.SentAt), AcceptedAt: nullTimePtr(r.AcceptedAt),
		ResendWindowStartedAt: nullTimePtr(r.ResendWindowStartedAt),
		DeliveryStatus:        r.DeliveryStatus, ResendCount: r.ResendCount, ResendWindowCount: r.ResendWindowCount,
	}
}

func (s *Store) getInvitation(ctx context.Context, userID, suffix string) (base.Invitation, bool, error) {
	cid, ok := canonicalUUID(userID)
	if !ok {
		return base.Invitation{}, false, nil
	}
	var r invitationRow
	if err := s.q.GetContext(ctx, &r, `SELECT `+invCols+` FROM user_invitations WHERE user_id = $1::uuid`+suffix, cid); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return base.Invitation{}, false, nil
		}
		return base.Invitation{}, false, fmt.Errorf("store: ambil undangan: %w", err)
	}
	return r.toInvitation(), true, nil
}

// FindInvitation mengembalikan undangan user. id bukan uuid atau tanpa baris -> found=false.
func (s *Store) FindInvitation(ctx context.Context, userID string) (base.Invitation, bool, error) {
	return s.getInvitation(ctx, userID, "")
}

// LockInvitation seperti FindInvitation dengan FOR UPDATE. Wajib dalam transaksi.
func (s *Store) LockInvitation(ctx context.Context, userID string) (base.Invitation, bool, error) {
	return s.getInvitation(ctx, userID, " FOR UPDATE")
}

// InsertInvitation menyimpan undangan baru. user_id wajib uuid milik users.
func (s *Store) InsertInvitation(ctx context.Context, inv base.Invitation) error {
	cid, ok := canonicalUUID(inv.UserID)
	if !ok {
		return base.ErrUserNotFound
	}
	_, err := s.q.ExecContext(ctx, `
INSERT INTO user_invitations (user_id, invited_by, invited_by_label, issued_at, expires_at, sent_at,
	delivery_status, accepted_at, resend_count, resend_window_started_at, resend_window_count, created_at)
VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
		cid, inv.InvitedBy, inv.InvitedByLabel, inv.IssuedAt, inv.ExpiresAt, inv.SentAt,
		inv.DeliveryStatus, inv.AcceptedAt, inv.ResendCount, inv.ResendWindowStartedAt, inv.ResendWindowCount, inv.CreatedAt)
	if err != nil {
		return fmt.Errorf("store: simpan undangan: %w", err)
	}
	return nil
}

// UpdateInvitation menyimpan semua kolom mutable. Baris tidak ada -> base.ErrInvitationNotPending.
func (s *Store) UpdateInvitation(ctx context.Context, inv base.Invitation) error {
	cid, ok := canonicalUUID(inv.UserID)
	if !ok {
		return base.ErrInvitationNotPending
	}
	res, err := s.q.ExecContext(ctx, `
UPDATE user_invitations SET invited_by_label = $2, issued_at = $3, expires_at = $4, sent_at = $5,
	delivery_status = $6, accepted_at = $7, resend_count = $8, resend_window_started_at = $9, resend_window_count = $10
WHERE user_id = $1::uuid`,
		cid, inv.InvitedByLabel, inv.IssuedAt, inv.ExpiresAt, inv.SentAt, inv.DeliveryStatus, inv.AcceptedAt,
		inv.ResendCount, inv.ResendWindowStartedAt, inv.ResendWindowCount)
	if err != nil {
		return fmt.Errorf("store: ubah undangan: %w", err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return base.ErrInvitationNotPending
	}
	return nil
}

// CountInvitationsCreatedBy menghitung undangan yang dibuat inviterID sejak since (created_at >= since)
// dan mengembalikan created_at tertua dalam jendela (zero bila count 0).
func (s *Store) CountInvitationsCreatedBy(ctx context.Context, inviterID string, since time.Time) (int, time.Time, error) {
	cid, ok := canonicalUUID(inviterID)
	if !ok {
		return 0, time.Time{}, nil
	}
	var row struct {
		N      int          `db:"n"`
		Oldest sql.NullTime `db:"oldest"`
	}
	err := s.q.GetContext(ctx, &row,
		`SELECT count(*) AS n, min(created_at) AS oldest FROM user_invitations WHERE invited_by = $1::uuid AND created_at >= $2`,
		cid, since)
	if err != nil {
		return 0, time.Time{}, fmt.Errorf("store: hitung undangan: %w", err)
	}
	return row.N, row.Oldest.Time, nil
}

// AcceptInvitation menandai undangan diterima. true hanya bila baris menunggu ditemukan.
func (s *Store) AcceptInvitation(ctx context.Context, userID string, now time.Time) (bool, error) {
	cid, ok := canonicalUUID(userID)
	if !ok {
		return false, nil
	}
	res, err := s.q.ExecContext(ctx,
		`UPDATE user_invitations SET accepted_at = $2 WHERE user_id = $1::uuid AND accepted_at IS NULL`, cid, now)
	if err != nil {
		return false, fmt.Errorf("store: terima undangan: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("store: hitung undangan diterima: %w", err)
	}
	return n > 0, nil
}
