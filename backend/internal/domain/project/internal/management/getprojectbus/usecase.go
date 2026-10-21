// Package getprojectbus menyediakan handler bus project.management.get-project.
package getprojectbus

import (
	"context"
	"fmt"

	"grimoire/internal/domain/project/internal/management/base"
	"grimoire/lib/contracts/project/management/getproject"
)

type projectFinder interface {
	FindProject(ctx context.Context, id string) (base.Project, bool, error)
}

// Usecase mengorkestrasi pencarian identitas ringkas project.
type Usecase struct{ repo projectFinder }

func NewUsecase(repo projectFinder) *Usecase { return &Usecase{repo: repo} }

// Execute mengembalikan Found=false tanpa error bila project tidak ada atau id bukan uuid.
func (u *Usecase) Execute(ctx context.Context, projectID string) (getproject.Response, error) {
	p, found, err := u.repo.FindProject(ctx, projectID)
	if err != nil {
		return getproject.Response{}, fmt.Errorf("getprojectbus: find: %w", err)
	}
	if !found {
		return getproject.Response{Found: false}, nil
	}
	return getproject.Response{Found: true, ID: p.ID, Name: p.Name}, nil
}
