// Package buslistchanges menyediakan handler bus project.knowledge.list-changes (consumer T18 list_recent_changes).
package buslistchanges

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"grimoire/internal/domain/project/internal/knowledge/base"
	"grimoire/lib/contracts/project/knowledge/listchanges"
)

type Repository interface {
	ListChanges(ctx context.Context, projectID string, since time.Time, likePrefix string, limit int) ([]listchanges.Item, int, error)
}

type Usecase struct{ repo Repository }

func New(repo Repository) *Usecase { return &Usecase{repo: repo} }

func (u *Usecase) Execute(ctx context.Context, in listchanges.Request) (listchanges.Response, error) {
	pid, err := uuid.Parse(in.ProjectID)
	if err != nil {
		return listchanges.Response{}, base.Invalid(base.ReasonInvalidProjectID)
	}
	if in.Since.IsZero() {
		return listchanges.Response{}, base.InvalidMsg(base.ReasonInvalidSince, "since wajib diisi")
	}
	prefix, reason := base.NormalizePrefix(base.StripLeadingSlash(in.Prefix))
	if reason != "" {
		return listchanges.Response{}, base.InvalidMsg(reason,
			fmt.Sprintf("prefix %q tidak valid: %s", in.Prefix, reason.Message()))
	}
	limit := in.Limit
	if limit <= 0 {
		limit = listchanges.DefaultLimit
	}
	if limit > listchanges.MaxLimit {
		limit = listchanges.MaxLimit
	}

	items, total, err := u.repo.ListChanges(ctx, pid.String(), in.Since.UTC(), base.LikePrefix(prefix), limit)
	if err != nil {
		return listchanges.Response{}, fmt.Errorf("buslistchanges: list changes: %w", err)
	}
	if items == nil {
		items = []listchanges.Item{}
	}
	return listchanges.Response{Items: items, Total: total}, nil
}
