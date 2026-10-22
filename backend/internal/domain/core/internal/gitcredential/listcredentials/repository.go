package listcredentials

import (
	"context"
	"fmt"
	"time"

	"grimoire/internal/domain/core/internal/gitcredential/base"
	"grimoire/internal/domain/core/internal/gitcredential/base/store"
	"grimoire/internal/infra/db"
)

// Kolom token_enc SENGAJA tidak ada di sini.
// DILARANG menambah token_enc ke SELECT ini; jalur dekripsi hanya di bustoken.
const listByUserSQL = `SELECT provider, host, last4, created_at, updated_at
FROM git_credentials
WHERE user_id = $1::uuid
ORDER BY provider, host`

// Repo adalah akses SQL untuk feature listcredentials.
type Repo struct{ db *db.DB }

func newRepo(d *db.DB) *Repo { return &Repo{db: d} }

type row struct {
	Provider  string    `db:"provider"`
	Host      string    `db:"host"`
	Last4     string    `db:"last4"`
	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`
}

// ListByUser mengembalikan kredensial milik userID (TokenEnc selalu nil). Fail-closed bila bukan uuid.
func (r *Repo) ListByUser(ctx context.Context, userID string) ([]base.Credential, error) {
	if !store.ValidUUID(userID) {
		return []base.Credential{}, nil
	}
	var rows []row
	if err := r.db.SelectContext(ctx, &rows, listByUserSQL, userID); err != nil {
		return nil, fmt.Errorf("listcredentials: select: %w", err)
	}
	out := make([]base.Credential, 0, len(rows))
	for _, x := range rows {
		out = append(out, base.Credential{
			UserID: userID, Provider: base.Provider(x.Provider), Host: x.Host, Last4: x.Last4,
			CreatedAt: x.CreatedAt.UTC(), UpdatedAt: x.UpdatedAt.UTC(),
		})
	}
	return out, nil
}
