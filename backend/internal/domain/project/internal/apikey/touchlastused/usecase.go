package touchlastused

import (
	"context"
	"log/slog"
	"time"

	"grimoire/internal/domain/project/internal/apikey/base"
	"grimoire/internal/domain/project/internal/apikey/base/store"
	"grimoire/lib/contracts/project/apikey/touch"
)

// Repository adalah akses data yang dibutuhkan usecase.
type Repository interface {
	Touch(ctx context.Context, id string, now time.Time) (int64, error)
}

// Usecase mengorkestrasi validasi uuid, clock, repository, dan log.
type Usecase struct {
	repo  Repository
	clock base.Clock
	log   *slog.Logger
}

func NewUsecase(repo Repository, clock base.Clock, log *slog.Logger) *Usecase {
	return &Usecase{repo: repo, clock: clock, log: log}
}

// Touch tidak pernah mengembalikan error: kegagalan DB hanya di-log (warn).
func (uc *Usecase) Touch(ctx context.Context, req touch.Request) {
	if !store.ValidUUID(req.KeyID) {
		uc.log.DebugContext(ctx, "apikey: touch diabaikan, key_id bukan uuid", "feature", "touchlastused")
		return
	}
	n, err := uc.repo.Touch(ctx, req.KeyID, uc.clock())
	if err != nil {
		uc.log.WarnContext(ctx, "apikey: touch last_used_at gagal", "feature", "touchlastused", "key_id", req.KeyID, "error", err)
		return
	}
	uc.log.DebugContext(ctx, "apikey: touch", "feature", "touchlastused", "key_id", req.KeyID, "written", n == 1)
}
