// Package busreadfile menyediakan handler bus project.knowledge.read-file (consumer T08).
package busreadfile

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"grimoire/internal/domain/project/internal/knowledge/base"
	"grimoire/lib/contracts/project/knowledge/readfile"
)

type files interface {
	FindByPath(ctx context.Context, projectID, path string) (base.File, bool, error)
	ListPaths(ctx context.Context, projectID string) ([]string, error)
}

const similarMax = 5

type Usecase struct {
	files  files
	limits base.Limits
}

func NewUsecase(f files, l base.Limits) *Usecase { return &Usecase{files: f, limits: l} }

func effectiveLimit(requested, maxRead int) int {
	if requested <= 0 {
		requested = readfile.DefaultLimit
	}
	if maxRead > 0 && requested > maxRead {
		requested = maxRead
	}
	return requested
}

func (u *Usecase) Execute(ctx context.Context, req readfile.Request) (readfile.Response, error) {
	if _, err := uuid.Parse(req.ProjectID); err != nil {
		return readfile.Response{}, base.Invalid(base.ReasonInvalidProjectID)
	}
	path, reason := base.NormalizePath(base.StripLeadingSlash(req.Path))
	if reason != "" {
		return readfile.Response{}, base.InvalidMsg(reason,
			fmt.Sprintf("path %q tidak valid: %s", req.Path, reason.Message()))
	}

	f, found, err := u.files.FindByPath(ctx, req.ProjectID, path)
	if err != nil {
		return readfile.Response{}, fmt.Errorf("busreadfile: find file: %w", err)
	}
	if !found {
		all, err := u.files.ListPaths(ctx, req.ProjectID)
		if err != nil {
			return readfile.Response{}, fmt.Errorf("busreadfile: list paths: %w", err)
		}
		return readfile.Response{Found: false, Similar: base.SimilarPaths(path, all, similarMax)}, nil
	}

	if req.Whole {
		if req.Offset != 0 {
			return readfile.Response{}, base.InvalidMsg(base.ReasonInvalidOffset, "whole tidak bisa digabung dengan offset")
		}
		total := len(f.Content)
		return readfile.Response{
			Found: true, Path: path, Content: f.Content,
			SizeBytes: total, TotalBytes: total, NextOffset: total, Truncated: false,
			UpdatedAt: f.UpdatedAt.UTC(),
		}, nil
	}

	limit := effectiveLimit(req.Limit, u.limits.MaxReadBytes)
	total := len(f.Content)
	part, _, end, truncated, reason := base.SliceUTF8(f.Content, req.Offset, limit)
	if reason == base.ReasonInvalidOffset {
		return readfile.Response{}, base.InvalidMsg(reason,
			fmt.Sprintf("offset %d di luar ukuran file (%d byte)", req.Offset, total))
	}
	return readfile.Response{
		Found: true, Path: path, Content: part,
		SizeBytes: len(part), TotalBytes: total, NextOffset: end, Truncated: truncated,
		UpdatedAt: f.UpdatedAt.UTC(),
	}, nil
}
