package bustoken

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"grimoire/internal/domain/core/internal/gitcredential/base"
	"grimoire/internal/infra/db"
)

// Repository adalah satu-satunya yang menyentuh SQL feature ini (read-only).
type Repository struct{ q db.Querier }

func NewRepository(q db.Querier) *Repository { return &Repository{q: q} }

const getSQL = `SELECT token_enc FROM git_credentials
                WHERE user_id = $1 AND provider = $2 AND host = $3`

// Get mengambil blob terenkripsi milik tepat satu (user, provider, host).
func (r *Repository) Get(ctx context.Context, userID string, p base.Provider, host string) ([]byte, bool, error) {
	var blob []byte
	if err := r.q.GetContext(ctx, &blob, getSQL, userID, string(p), host); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("bustoken: get: %w", err)
	}
	return blob, true, nil
}
