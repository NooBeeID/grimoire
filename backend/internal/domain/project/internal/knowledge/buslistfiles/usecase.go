// Package buslistfiles menyediakan handler bus project.knowledge.list-files (consumer T08).
package buslistfiles

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"grimoire/internal/domain/project/internal/knowledge/base"
	"grimoire/lib/contracts/project/knowledge/listfiles"
)

type Repository interface {
	ListMeta(ctx context.Context, projectID, likePrefix string) ([]base.Meta, error)
	ListPage(ctx context.Context, projectID, likePrefix string, limit, offset int) ([]base.Meta, error)
	CountByPrefix(ctx context.Context, projectID, likePrefix string) (int, error)
}

type Usecase struct{ repo Repository }

func New(repo Repository) *Usecase { return &Usecase{repo: repo} }

func (u *Usecase) Execute(ctx context.Context, in listfiles.Request) (listfiles.Response, error) {
	pid, err := uuid.Parse(in.ProjectID)
	if err != nil {
		return listfiles.Response{}, base.Invalid(base.ReasonInvalidProjectID)
	}
	projectID := pid.String()

	prefix, reason := base.NormalizePrefix(base.StripLeadingSlash(in.Prefix))
	if reason != "" {
		return listfiles.Response{}, base.InvalidMsg(reason,
			fmt.Sprintf("prefix %q tidak valid: %s", in.Prefix, reason.Message()))
	}

	limit := in.Limit
	if limit <= 0 {
		limit = listfiles.DefaultLimit
	}
	if limit > listfiles.MaxLimit {
		limit = listfiles.MaxLimit
	}
	if in.Offset < 0 {
		return listfiles.Response{}, base.InvalidMsg(base.ReasonInvalidOffset, "offset tidak boleh negatif")
	}
	offset := in.Offset
	like := base.LikePrefix(prefix)

	var res listfiles.Response
	if in.Recursive {
		metas, err := u.repo.ListPage(ctx, projectID, like, limit, offset)
		if err != nil {
			return listfiles.Response{}, fmt.Errorf("buslistfiles: list page: %w", err)
		}
		total, err := u.repo.CountByPrefix(ctx, projectID, like)
		if err != nil {
			return listfiles.Response{}, fmt.Errorf("buslistfiles: count: %w", err)
		}
		res.Items = toItems(metas)
		res.Total = total
	} else {
		metas, err := u.repo.ListMeta(ctx, projectID, like)
		if err != nil {
			return listfiles.Response{}, fmt.Errorf("buslistfiles: list meta: %w", err)
		}
		level := base.BuildLevel(prefix, metas)
		total := len(level.Files)
		files := []base.Meta{}
		if offset < total {
			for _, f := range level.Files[offset:min(offset+limit, total)] {
				files = append(files, base.Meta{Path: f.Path, SizeBytes: f.SizeBytes, UpdatedAt: f.UpdatedAt})
			}
		}
		res.Items = toItems(files)
		res.Folders = make([]string, 0, len(level.Folders))
		for _, f := range level.Folders {
			res.Folders = append(res.Folders, f.Path)
		}
		res.Total = total
	}
	res.Truncated = offset+len(res.Items) < res.Total
	return res, nil
}

func toItems(metas []base.Meta) []listfiles.Item {
	out := make([]listfiles.Item, 0, len(metas))
	for _, m := range metas {
		out = append(out, listfiles.Item{Path: m.Path, SizeBytes: m.SizeBytes, UpdatedAt: m.UpdatedAt})
	}
	return out
}
