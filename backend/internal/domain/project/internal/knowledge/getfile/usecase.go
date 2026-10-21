// Package getfile membaca satu file knowledge beserta version (GET /api/projects/{projectID}/knowledge/files?path=).
package getfile

import (
	"context"
	"fmt"

	"grimoire/internal/domain/project/internal/knowledge/base"
	"grimoire/internal/domain/project/internal/knowledge/base/access"
)

// FileFinder dipenuhi *store.Store (base/store). Didefinisikan di sisi consumer.
type FileFinder interface {
	FindByPath(ctx context.Context, projectID, path string) (base.File, bool, error)
}

type Usecase struct {
	access access.Checker
	files  FileFinder
}

func NewUsecase(a access.Checker, f FileFinder) *Usecase { return &Usecase{access: a, files: f} }

// Execute: otorisasi dulu, lalu validasi path, lalu cari file. Tanpa audit.
func (u *Usecase) Execute(ctx context.Context, projectID, rawPath string) (base.FileDTO, error) {
	if _, err := u.access.Require(ctx, projectID); err != nil {
		return base.FileDTO{}, err
	}
	p, reason := base.NormalizePath(rawPath)
	if reason != "" {
		return base.FileDTO{}, base.Invalid(reason)
	}
	f, found, err := u.files.FindByPath(ctx, projectID, p)
	if err != nil {
		return base.FileDTO{}, fmt.Errorf("getfile: find: %w", err)
	}
	if !found {
		return base.FileDTO{}, base.ErrFileNotFound
	}
	return f.ToDTO(), nil
}
