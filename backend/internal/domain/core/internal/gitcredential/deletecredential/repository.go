package deletecredential

import (
	"context"
	"database/sql"
	"errors"

	"grimoire/internal/domain/core/internal/gitcredential/base"
	"grimoire/internal/domain/core/internal/gitcredential/base/store"
	"grimoire/internal/infra/db"
)

const deleteSQL = `DELETE FROM git_credentials WHERE user_id = $1::uuid AND provider = $2 AND host = $3 RETURNING 1`

// Repo adalah satu-satunya tempat SQL feature ini.
type Repo struct{ db db.Querier }

func newRepo(q db.Querier) *Repo { return &Repo{db: q} }

// Delete menghapus satu baris milik userID. Satu statement, atomik.
func (r *Repo) Delete(ctx context.Context, userID string, p base.Provider, host string) (bool, error) {
	if !store.ValidUUID(userID) {
		return false, nil
	}
	var one int
	err := r.db.QueryRowxContext(ctx, deleteSQL, userID, string(p), host).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}
