// Package resolvesession menangani bus core.auth.resolve-session: token cookie menjadi identitas.
package resolvesession

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"grimoire/internal/domain/core/internal/auth/base"
	"grimoire/internal/infra/db"
)

// Repository dipakai usecase lewat interface; implementasi konkret Repo.
type Repository interface {
	// found=false bila tidak ada baris.
	FindSession(ctx context.Context, tokenHash []byte) (base.Session, base.User, bool, error)
	// TouchSession mengembalikan true bila baris berubah (false bila sudah disentuh request lain).
	TouchSession(ctx context.Context, sessionID string, now time.Time) (bool, error)
}

// Repo adalah implementasi Repository di atas db.Querier.
type Repo struct{ q db.Querier }

// NewRepo membuat Repo.
func NewRepo(q db.Querier) *Repo { return &Repo{q: q} }

type sessionUserRow struct {
	ID                 string    `db:"id"`
	UserID             string    `db:"user_id"`
	TokenHash          []byte    `db:"token_hash"`
	CreatedAt          time.Time `db:"created_at"`
	LastSeenAt         time.Time `db:"last_seen_at"`
	ExpiresAt          time.Time `db:"expires_at"`
	Email              string    `db:"email"`
	Name               string    `db:"name"`
	Role               string    `db:"role"`
	IsActive           bool      `db:"is_active"`
	MustChangePassword bool      `db:"must_change_password"`
}

const findSessionSQL = `
SELECT s.id::text AS id, s.user_id::text AS user_id, s.token_hash, s.created_at, s.last_seen_at, s.expires_at,
       u.email::text AS email, u.name, u.role, u.is_active, u.must_change_password
FROM sessions s
JOIN users u ON u.id = s.user_id
WHERE s.token_hash = $1`

// FindSession mengambil sesi beserta user saat ini dalam satu query.
func (r *Repo) FindSession(ctx context.Context, tokenHash []byte) (base.Session, base.User, bool, error) {
	var row sessionUserRow
	if err := r.q.GetContext(ctx, &row, findSessionSQL, tokenHash); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return base.Session{}, base.User{}, false, nil
		}
		return base.Session{}, base.User{}, false, fmt.Errorf("resolvesession: query sesi: %w", err)
	}
	s := base.Session{
		ID: row.ID, UserID: row.UserID, TokenHash: row.TokenHash,
		CreatedAt: row.CreatedAt, LastSeenAt: row.LastSeenAt, ExpiresAt: row.ExpiresAt,
	}
	u := base.User{
		ID: row.UserID, Email: row.Email, Name: row.Name, Role: row.Role,
		IsActive: row.IsActive, MustChangePassword: row.MustChangePassword,
	}
	return s, u, true, nil
}

// touchSessionSQL: interval 1 menit mencerminkan base.TouchInterval.
const touchSessionSQL = `
UPDATE sessions SET last_seen_at = $2::timestamptz
WHERE id = $1 AND last_seen_at < $2::timestamptz - interval '1 minute'`

// TouchSession memajukan last_seen_at secara idempoten (paling sering sekali per menit).
func (r *Repo) TouchSession(ctx context.Context, sessionID string, now time.Time) (bool, error) {
	res, err := r.q.ExecContext(ctx, touchSessionSQL, sessionID, now)
	if err != nil {
		return false, fmt.Errorf("resolvesession: touch sesi: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("resolvesession: rows affected: %w", err)
	}
	return n > 0, nil
}
