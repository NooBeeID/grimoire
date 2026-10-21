package uploadfiles

import (
	"context"
	"fmt"

	"grimoire/internal/domain/project/internal/knowledge/base"
	"grimoire/internal/domain/project/internal/knowledge/base/access"
	"grimoire/internal/domain/project/internal/knowledge/base/audit"
	"grimoire/lib/contracts/core/audit/record"
)

const maxAuditPaths = 20

// repository adalah akses data uploadfiles.
type repository interface {
	InTx(ctx context.Context, fn func(repository) error) error
	LockProject(ctx context.Context, projectID string) error
	CountFiles(ctx context.Context, projectID string) (int, error)
	ExistingHashes(ctx context.Context, projectID string, paths []string) (map[string][]byte, error)
	InsertFiles(ctx context.Context, files []base.File) error
	UpdateFiles(ctx context.Context, files []base.File) error
}

type Usecase struct {
	repo   repository
	access access.Checker
	audit  audit.Recorder
	limits base.Limits
}

func NewUsecase(repo repository, ac access.Checker, au audit.Recorder, limits base.Limits) *Usecase {
	return &Usecase{repo: repo, access: ac, audit: au, limits: limits}
}

type Input struct {
	ProjectID string
	Parts     []RawPart
	Meta      base.RequestMeta
}

// Authorize dipanggil handler SEBELUM membaca body (non-anggota -> 404 tanpa membaca stream).
func (u *Usecase) Authorize(ctx context.Context, projectID string) (base.Actor, error) {
	return u.access.Require(ctx, projectID)
}

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n])
	}
	return s
}

func (u *Usecase) Execute(ctx context.Context, actor base.Actor, in Input) (base.UploadResultDTO, error) {
	if len(in.Parts) == 0 {
		return base.UploadResultDTO{}, base.Invalid(base.ReasonNoFiles)
	}
	decisions := make([]base.Decision, 0, len(in.Parts))
	incoming := make([]base.Incoming, 0, len(in.Parts))
	for _, p := range in.Parts {
		if p.OverCount {
			decisions = append(decisions, base.Decision{Index: p.Index, Path: truncateRunes(p.Name, 1024),
				Status: base.StatusRejected, Reason: base.ReasonTooManyFiles})
			continue
		}
		inc, dec, ok := base.ClassifyPart(p.Index, p.Name, p.Data, p.TooLarge, u.limits.MaxFileBytes)
		if !ok {
			decisions = append(decisions, dec)
			continue
		}
		incoming = append(incoming, inc)
	}

	all := decisions
	var bytesWritten int
	if len(incoming) > 0 {
		var planned []base.Decision
		err := u.repo.InTx(ctx, func(tx repository) error {
			if err := tx.LockProject(ctx, in.ProjectID); err != nil {
				return fmt.Errorf("lock project: %w", err)
			}
			count, err := tx.CountFiles(ctx, in.ProjectID)
			if err != nil {
				return fmt.Errorf("count files: %w", err)
			}
			paths := make([]string, 0, len(incoming))
			for _, f := range incoming {
				paths = append(paths, f.Path)
			}
			existing, err := tx.ExistingHashes(ctx, in.ProjectID, paths)
			if err != nil {
				return fmt.Errorf("existing hashes: %w", err)
			}
			planned = base.PlanUpload(incoming, existing, count, u.limits.MaxProjectFiles)

			byIndex := make(map[int]base.Incoming, len(incoming))
			for _, f := range incoming {
				byIndex[f.Index] = f
			}
			var created, updated []base.File
			bytesWritten = 0
			for _, d := range planned {
				f := byIndex[d.Index]
				switch d.Status {
				case base.StatusCreated:
					nf := base.NewFile(in.ProjectID, f.Path, f.Content, actor)
					created = append(created, nf)
					bytesWritten += nf.SizeBytes
				case base.StatusUpdated:
					nf := base.NewFile(in.ProjectID, f.Path, f.Content, actor)
					updated = append(updated, nf)
					bytesWritten += nf.SizeBytes
				}
			}
			if len(created) > 0 {
				if err := tx.InsertFiles(ctx, created); err != nil {
					return fmt.Errorf("insert files: %w", err)
				}
			}
			if len(updated) > 0 {
				if err := tx.UpdateFiles(ctx, updated); err != nil {
					return fmt.Errorf("update files: %w", err)
				}
			}
			return nil
		})
		if err != nil {
			return base.UploadResultDTO{}, fmt.Errorf("uploadfiles: batch: %w", err)
		}
		all = append(append([]base.Decision(nil), decisions...), planned...)
	}
	base.SortByIndex(all)

	sum := base.Summarize(all)
	paths, trunc := base.ChangedPaths(all, maxAuditPaths)
	u.audit.Record(ctx, audit.Event{
		Action: record.ActionKnowledgeUploadCompleted, ProjectID: in.ProjectID, Actor: actor,
		TargetType: audit.TargetKnowledge, Target: "",
		Args: map[string]any{"files": len(in.Parts)},
		Result: map[string]any{
			"created": sum.Created, "updated": sum.Updated, "unchanged": sum.Unchanged,
			"skipped": sum.Skipped, "rejected": sum.Rejected,
			"bytes": bytesWritten, "paths": paths, "paths_truncated": trunc,
		},
		Meta: in.Meta,
	})
	return base.NewUploadResult(all), nil
}
