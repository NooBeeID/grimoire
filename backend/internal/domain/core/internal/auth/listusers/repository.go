package listusers

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"grimoire/internal/domain/core/internal/auth/base"
	"grimoire/internal/infra/db"
)

// Repository adalah akses data daftar user.
type Repository interface {
	// List mengembalikan satu halaman (tanpa password_hash) urut name ASC, id ASC dan
	// total baris yang cocok filter.
	List(ctx context.Context, f base.UserFilter) (users []base.UserWithInvitation, total int, err error)
}

// Repo mengimplementasikan Repository di atas SQL.
type Repo struct{ q db.Querier }

// NewRepo membuat Repo.
func NewRepo(q db.Querier) *Repo { return &Repo{q: q} }

type userRow struct {
	ID                 string     `db:"id"`
	Email              string     `db:"email"`
	Name               string     `db:"name"`
	Role               string     `db:"role"`
	IsActive           bool       `db:"is_active"`
	MustChangePassword bool       `db:"must_change_password"`
	CreatedAt          time.Time  `db:"created_at"`
	CreatedBy          *string    `db:"created_by"`
	CreatedByLabel     *string    `db:"created_by_label"`
	UpdatedAt          time.Time  `db:"updated_at"`
	LastLoginAt        *time.Time `db:"last_login_at"`
	Total              int        `db:"total"`

	InvPresent        bool       `db:"inv_present"`
	InvCreatedAt      *time.Time `db:"inv_created_at"`
	InvIssuedAt       *time.Time `db:"inv_issued_at"`
	InvExpiresAt      *time.Time `db:"inv_expires_at"`
	InvSentAt         *time.Time `db:"inv_sent_at"`
	InvDeliveryStatus *string    `db:"inv_delivery_status"`
	InvResendCount    *int       `db:"inv_resend_count"`
}

// invitation merekonstruksi undangan menunggu dari kolom join; nil bila tidak ada.
func (x userRow) invitation() *base.Invitation {
	if !x.InvPresent || x.InvCreatedAt == nil || x.InvIssuedAt == nil || x.InvExpiresAt == nil ||
		x.InvDeliveryStatus == nil || x.InvResendCount == nil {
		return nil
	}
	return &base.Invitation{
		UserID: x.ID, CreatedAt: *x.InvCreatedAt, IssuedAt: *x.InvIssuedAt, ExpiresAt: *x.InvExpiresAt,
		SentAt: x.InvSentAt, DeliveryStatus: *x.InvDeliveryStatus, ResendCount: *x.InvResendCount,
	}
}

const selectCols = `SELECT u.id::text AS id, u.email::text AS email, u.name, u.role, u.is_active, u.must_change_password,
       u.created_at, u.created_by::text AS created_by, u.created_by_label, u.updated_at, u.last_login_at,
       i.user_id IS NOT NULL AS inv_present,
       i.created_at AS inv_created_at, i.issued_at AS inv_issued_at, i.expires_at AS inv_expires_at,
       i.sent_at AS inv_sent_at, i.delivery_status AS inv_delivery_status,
       i.resend_count AS inv_resend_count,
       COUNT(*) OVER() AS total
FROM users u
LEFT JOIN user_invitations i ON i.user_id = u.id AND i.accepted_at IS NULL`

// where membangun klausa WHERE dari fragmen konstan; nilai hanya lewat argumen posisional.
func where(f base.UserFilter) (string, []any) {
	var sb strings.Builder
	sb.WriteString(" WHERE 1=1")
	var args []any
	add := func(frag string, v any) {
		args = append(args, v)
		sb.WriteString(strings.ReplaceAll(frag, "$n", "$"+strconv.Itoa(len(args))))
	}
	if f.Role != "" {
		add(" AND u.role = $n", f.Role)
	}
	if f.IsActive != nil {
		add(" AND u.is_active = $n", *f.IsActive)
	}
	if p := f.LikePattern(); p != "" {
		add(` AND (u.email ILIKE $n ESCAPE '\' OR u.name ILIKE $n ESCAPE '\')`, p)
	}
	return sb.String(), args
}

// List mengimplementasikan Repository.
func (r *Repo) List(ctx context.Context, f base.UserFilter) ([]base.UserWithInvitation, int, error) {
	clause, args := where(f)
	query := selectCols + clause + " ORDER BY u.name ASC, u.id ASC LIMIT $" +
		strconv.Itoa(len(args)+1) + " OFFSET $" + strconv.Itoa(len(args)+2)
	var rows []userRow
	if err := r.q.SelectContext(ctx, &rows, query, append(args, f.Limit, f.Offset)...); err != nil {
		return nil, 0, fmt.Errorf("listusers: query users: %w", err)
	}
	users := make([]base.UserWithInvitation, 0, len(rows))
	for _, x := range rows {
		users = append(users, base.UserWithInvitation{
			User: base.User{
				ID: x.ID, Email: x.Email, Name: x.Name, Role: x.Role, IsActive: x.IsActive,
				MustChangePassword: x.MustChangePassword, CreatedAt: x.CreatedAt, CreatedBy: x.CreatedBy,
				CreatedByLabel: x.CreatedByLabel, UpdatedAt: x.UpdatedAt, LastLoginAt: x.LastLoginAt,
			},
			Invitation: x.invitation(),
		})
	}
	if len(rows) > 0 {
		return users, rows[0].Total, nil
	}
	if f.Offset == 0 {
		return users, 0, nil
	}
	var total int
	if err := r.q.GetContext(ctx, &total, "SELECT count(*) FROM users u"+clause, args...); err != nil {
		return nil, 0, fmt.Errorf("listusers: count users: %w", err)
	}
	return users, total, nil
}
