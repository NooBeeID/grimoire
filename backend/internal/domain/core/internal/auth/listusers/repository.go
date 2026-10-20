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
	List(ctx context.Context, f base.UserFilter) (users []base.User, total int, err error)
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
}

const selectCols = `SELECT id::text AS id, email::text AS email, name, role, is_active, must_change_password,
       created_at, created_by::text AS created_by, created_by_label, updated_at, last_login_at,
       COUNT(*) OVER() AS total
FROM users`

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
		add(" AND role = $n", f.Role)
	}
	if f.IsActive != nil {
		add(" AND is_active = $n", *f.IsActive)
	}
	if p := f.LikePattern(); p != "" {
		add(` AND (email ILIKE $n ESCAPE '\' OR name ILIKE $n ESCAPE '\')`, p)
	}
	return sb.String(), args
}

// List mengimplementasikan Repository.
func (r *Repo) List(ctx context.Context, f base.UserFilter) ([]base.User, int, error) {
	clause, args := where(f)
	query := selectCols + clause + " ORDER BY name ASC, id ASC LIMIT $" +
		strconv.Itoa(len(args)+1) + " OFFSET $" + strconv.Itoa(len(args)+2)
	var rows []userRow
	if err := r.q.SelectContext(ctx, &rows, query, append(args, f.Limit, f.Offset)...); err != nil {
		return nil, 0, fmt.Errorf("listusers: query users: %w", err)
	}
	users := make([]base.User, 0, len(rows))
	for _, x := range rows {
		users = append(users, base.User{
			ID: x.ID, Email: x.Email, Name: x.Name, Role: x.Role, IsActive: x.IsActive,
			MustChangePassword: x.MustChangePassword, CreatedAt: x.CreatedAt, CreatedBy: x.CreatedBy,
			CreatedByLabel: x.CreatedByLabel, UpdatedAt: x.UpdatedAt, LastLoginAt: x.LastLoginAt,
		})
	}
	if len(rows) > 0 {
		return users, rows[0].Total, nil
	}
	if f.Offset == 0 {
		return users, 0, nil
	}
	var total int
	if err := r.q.GetContext(ctx, &total, "SELECT count(*) FROM users"+clause, args...); err != nil {
		return nil, 0, fmt.Errorf("listusers: count users: %w", err)
	}
	return users, total, nil
}
