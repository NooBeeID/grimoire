package getconfig

import (
	"context"
	"fmt"

	"grimoire/internal/domain/project/internal/gitsync/base"
	"grimoire/internal/domain/project/internal/gitsync/base/store"
)

// Repo membaca config lewat store bersama (satu-satunya akses data feature ini).
type Repo struct{ s *store.Store }

func newRepo(s *store.Store) *Repo { return &Repo{s: s} }

// Get mengembalikan config (project, kind) atau (nil, nil) bila belum ada.
func (r *Repo) Get(ctx context.Context, projectID string, kind base.Kind) (*base.Config, error) {
	c, ok, err := r.s.SelectConfig(ctx, projectID, kind)
	if err != nil {
		return nil, fmt.Errorf("getconfig: get: %w", err)
	}
	if !ok {
		return nil, nil
	}
	return &c, nil
}

// InProgress true bila sync (project, kind) sedang berjalan (advisory lock dipegang).
// Delegasi ke store.IsLocked; tidak mengambil lock.
func (r *Repo) InProgress(ctx context.Context, projectID string, kind base.Kind) (bool, error) {
	ok, err := r.s.IsLocked(ctx, projectID, kind)
	if err != nil {
		return false, fmt.Errorf("getconfig: in progress: %w", err)
	}
	return ok, nil
}
