package usage

import (
	"context"
	"fmt"

	"grimoire/internal/domain/core/internal/auditlog/base"
	"grimoire/internal/infra/bus"
	"grimoire/lib/constants"
	"grimoire/lib/contracts/project/knowledge/listfiles"
	"grimoire/lib/contracts/project/management/getproject"
)

// ProjectFinder adalah port ke provider project.management.get-project.
type ProjectFinder interface {
	Exists(ctx context.Context, projectID string) (bool, error)
}

// FileLister adalah port ke provider project.knowledge.list-files.
type FileLister interface {
	// ListAll memindai file project maks base.MaxScanFiles. capped=true bila masih ada file yang tidak terpindai.
	ListAll(ctx context.Context, projectID string) (files []base.KnownFile, capped bool, err error)
}

type busProjectFinder struct{ b *bus.Registry }

// NewProjectFinder membuat adapter bus. Tidak mendereferensi b saat dibuat.
func NewProjectFinder(b *bus.Registry) ProjectFinder { return &busProjectFinder{b: b} }

func (f *busProjectFinder) Exists(ctx context.Context, projectID string) (bool, error) {
	res, err := bus.CallTyped[getproject.Response](ctx, f.b, constants.BUS_PROJECT_MANAGEMENT_GET_PROJECT, getproject.Request{ProjectID: projectID})
	if err != nil {
		return false, err
	}
	return res.Found, nil
}

type busFileLister struct{ b *bus.Registry }

// NewFileLister membuat adapter bus. Tidak mendereferensi b saat dibuat.
func NewFileLister(b *bus.Registry) FileLister { return &busFileLister{b: b} }

func (l *busFileLister) ListAll(ctx context.Context, projectID string) ([]base.KnownFile, bool, error) {
	files := []base.KnownFile{}
	for {
		res, err := bus.CallTyped[listfiles.Response](ctx, l.b, constants.BUS_PROJECT_KNOWLEDGE_LIST_FILES, listfiles.Request{
			ProjectID: projectID,
			Recursive: true,
			Limit:     listfiles.MaxLimit,
			Offset:    len(files),
		})
		if err != nil {
			return nil, false, fmt.Errorf("list files: %w", err)
		}
		for _, it := range res.Items {
			files = append(files, base.KnownFile{Path: it.Path, UpdatedAt: it.UpdatedAt.UTC()})
		}
		if !res.Truncated || len(res.Items) == 0 {
			return files, false, nil
		}
		if len(files) >= base.MaxScanFiles {
			return files, true, nil
		}
	}
}
