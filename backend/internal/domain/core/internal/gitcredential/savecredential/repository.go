package savecredential

import (
	"context"
	"fmt"

	"grimoire/internal/domain/core/internal/gitcredential/base"
	"grimoire/internal/domain/core/internal/gitcredential/base/store"
	"grimoire/internal/infra/db"
	"grimoire/lib/common/errs"
)

type Repo struct{ db *db.DB }

func newRepo(d *db.DB) *Repo { return &Repo{db: d} }

var _ Repository = (*Repo)(nil)

const qUpsert = `
INSERT INTO git_credentials (user_id, provider, host, token_enc, last4)
VALUES ($1::uuid, $2, $3, $4, $5)
ON CONFLICT (user_id, provider, host) DO UPDATE
   SET token_enc = EXCLUDED.token_enc,
       last4     = EXCLUDED.last4,
       updated_at = now()
RETURNING created_at, updated_at, (xmax <> 0) AS replaced`

// Upsert satu statement atomik. replaced = true bila baris lama ditimpa.
func (r *Repo) Upsert(ctx context.Context, c base.Credential) (base.Saved, error) {
	if !store.ValidUUID(c.UserID) {
		return base.Saved{}, base.ErrUnauthenticated
	}
	var s base.Saved
	err := r.db.QueryRowxContext(ctx, qUpsert, c.UserID, string(c.Provider), c.Host, c.TokenEnc, c.Last4).
		Scan(&s.CreatedAt, &s.UpdatedAt, &s.Replaced)
	if err != nil {
		if _, constraint, ok := store.Violation(err); ok {
			switch constraint {
			case store.ConstraintGitHubHost, store.ConstraintHostCheck, store.ConstraintProviderCheck:
				return base.Saved{}, base.HostNotAllowed(truncate(c.Host, 253))
			case store.ConstraintUserFK:
				return base.Saved{}, errs.New(errs.CodeUnauthorized, "perlu login")
			}
		}
		return base.Saved{}, fmt.Errorf("savecredential: upsert: %w", err)
	}
	return s, nil
}
