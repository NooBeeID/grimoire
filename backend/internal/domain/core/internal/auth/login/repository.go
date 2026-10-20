package login

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/jmoiron/sqlx"

	"grimoire/internal/domain/core/internal/auth/base"
	"grimoire/internal/domain/core/internal/auth/base/store"
	"grimoire/internal/infra/db"
)

// Repo adalah satu-satunya yang menyentuh SQL untuk usecase login.
type Repo struct {
	db    *db.DB
	store *store.Store
}

// NewRepository membuat Repository.
func NewRepository(d *db.DB) *Repo {
	return &Repo{db: d, store: store.New(d)}
}

// FindUserByEmail mendelegasikan ke store bersama.
func (r *Repo) FindUserByEmail(ctx context.Context, email string) (base.User, bool, error) {
	return r.store.FindUserByEmail(ctx, email)
}

type throttleRow struct {
	Email           string       `db:"email"`
	FailedCount     int          `db:"failed_count"`
	WindowStartedAt time.Time    `db:"window_started_at"`
	LockedUntil     sql.NullTime `db:"locked_until"`
}

func (t throttleRow) entity() base.LoginThrottle {
	e := base.LoginThrottle{Email: t.Email, FailedCount: t.FailedCount, WindowStartedAt: t.WindowStartedAt}
	if t.LockedUntil.Valid {
		l := t.LockedUntil.Time
		e.LockedUntil = &l
	}
	return e
}

const throttleCols = `email::text AS email, failed_count, window_started_at, locked_until`

// GetThrottle membaca throttle email; tanpa baris -> zero value dengan email terisi.
func (r *Repo) GetThrottle(ctx context.Context, email string) (base.LoginThrottle, error) {
	var row throttleRow
	err := r.db.GetContext(ctx, &row, `SELECT `+throttleCols+` FROM login_throttles WHERE email = $1`, email)
	if errors.Is(err, sql.ErrNoRows) {
		return base.LoginThrottle{Email: email}, nil
	}
	if err != nil {
		return base.LoginThrottle{}, fmt.Errorf("login: baca throttle: %w", err)
	}
	return row.entity(), nil
}

// UpdateThrottle menerapkan mutate (rule entity) pada baris throttle terkunci FOR UPDATE.
func (r *Repo) UpdateThrottle(ctx context.Context, email string, now time.Time,
	mutate func(*base.LoginThrottle)) (base.LoginThrottle, error) {
	var out base.LoginThrottle
	err := r.db.BeginTx(ctx, func(tx *sqlx.Tx) error {
		if _, err := tx.ExecContext(ctx, `INSERT INTO login_throttles (email, failed_count, window_started_at)
			VALUES ($1, 0, $2) ON CONFLICT (email) DO NOTHING`, email, now); err != nil {
			return fmt.Errorf("login: buat throttle: %w", err)
		}
		var row throttleRow
		if err := tx.GetContext(ctx, &row, `SELECT `+throttleCols+` FROM login_throttles WHERE email = $1 FOR UPDATE`, email); err != nil {
			return fmt.Errorf("login: kunci throttle: %w", err)
		}
		t := row.entity()
		mutate(&t)
		var locked sql.NullTime
		if t.LockedUntil != nil {
			locked = sql.NullTime{Time: *t.LockedUntil, Valid: true}
		}
		if _, err := tx.ExecContext(ctx, `UPDATE login_throttles SET failed_count=$2, window_started_at=$3, locked_until=$4, updated_at=$5 WHERE email=$1`,
			email, t.FailedCount, t.WindowStartedAt, locked, now); err != nil {
			return fmt.Errorf("login: simpan throttle: %w", err)
		}
		out = t
		return nil
	})
	if err != nil {
		return base.LoginThrottle{}, err
	}
	return out, nil
}

// CompleteLogin menghapus throttle, menyisipkan sesi, dan mengisi last_login_at dalam satu transaksi.
func (r *Repo) CompleteLogin(ctx context.Context, email, userID string, s base.Session, now time.Time) error {
	return r.db.BeginTx(ctx, func(tx *sqlx.Tx) error {
		if _, err := tx.ExecContext(ctx, `DELETE FROM login_throttles WHERE email = $1`, email); err != nil {
			return fmt.Errorf("login: hapus throttle: %w", err)
		}
		var ip, ua any
		if s.IP != nil && net.ParseIP(*s.IP) != nil {
			ip = *s.IP
		}
		if s.UserAgent != nil && *s.UserAgent != "" {
			ua = *s.UserAgent
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO sessions (id, user_id, token_hash, created_at, last_seen_at, expires_at, ip, user_agent)
			VALUES (COALESCE(NULLIF($1,'')::uuid, gen_random_uuid()), $2, $3, $4, $5, $6, $7::inet, $8)`,
			s.ID, userID, s.TokenHash, s.CreatedAt, s.LastSeenAt, s.ExpiresAt, ip, ua); err != nil {
			return fmt.Errorf("login: buat sesi: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `UPDATE users SET last_login_at = $2 WHERE id = $1`, userID, now); err != nil {
			return fmt.Errorf("login: update last_login_at: %w", err)
		}
		return nil
	})
}
