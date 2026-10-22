// Package busupsertspecs: handler bus project.apispec.upsert-specs (sync spec dari Git).
package busupsertspecs

import (
	"context"
	"fmt"
	"path"
	"strings"

	"github.com/google/uuid"

	"grimoire/internal/domain/project/internal/apispec/base"
	"grimoire/internal/domain/project/internal/apispec/base/audit"
	"grimoire/internal/domain/project/internal/apispec/base/ingest"
	"grimoire/internal/domain/project/internal/apispec/base/upload"
	"grimoire/lib/common/errs"
	"grimoire/lib/contracts/project/apispec/upsertspecs"
)

const maxSourceRunes = 32

type Repository = ingest.Repo

type Usecase struct {
	repo         Repository
	audit        audit.Recorder
	clock        base.Clock
	maxSpecs     int
	maxFileBytes int64
}

func NewUsecase(repo Repository, rec audit.Recorder, clock base.Clock, maxSpecs int, maxFileBytes int64) *Usecase {
	return &Usecase{repo: repo, audit: rec, clock: clock, maxSpecs: maxSpecs, maxFileBytes: maxFileBytes}
}

func invalid(field, msg string) error {
	return errs.WithDetails(errs.CodeInvalid, "permintaan tidak valid",
		map[string]any{"fields": map[string]string{field: msg}})
}

// Execute memproses item berurutan, satu transaksi per file. Error infrastruktur menghentikan sisa batch.
func (u *Usecase) Execute(ctx context.Context, in upsertspecs.Request) (upsertspecs.Response, error) {
	pid, err := uuid.Parse(in.ProjectID)
	if err != nil {
		return upsertspecs.Response{}, invalid("project_id", "project_id harus uuid")
	}
	if strings.TrimSpace(in.Actor.UserID) == "" {
		return upsertspecs.Response{}, invalid("actor", "aktor wajib diisi")
	}
	source := strings.TrimSpace(in.Source)
	if source == "" {
		source = upsertspecs.SourceGitSync
	}
	if r := []rune(source); len(r) > maxSourceRunes {
		source = string(r[:maxSourceRunes])
	}
	deps := ingest.Deps{Repo: u.repo, Audit: u.audit, Clock: u.clock, MaxSpecs: u.maxSpecs, MaxFileBytes: u.maxFileBytes}
	req := ingest.Request{
		ProjectID: pid.String(),
		Actor:     base.Actor{ID: in.Actor.UserID, Label: in.Actor.Label},
		Meta:      base.NewRequestMeta(in.Meta.IP, in.Meta.UserAgent),
		Source:    source,
	}
	results := make([]base.FileResult, 0, len(in.Items))
	items := make([]upsertspecs.ItemResult, 0, len(in.Items))
	reserved := map[string]struct{}{}
	for _, it := range in.Items {
		if err := ctx.Err(); err != nil {
			return upsertspecs.Response{}, fmt.Errorf("busupsertspecs: dibatalkan: %w", err)
		}
		f := upload.File{
			FileName: path.Base(strings.ReplaceAll(it.SourcePath, "\\", "/")),
			RawPath:  it.SourcePath,
			Content:  it.Content,
			TooLarge: it.TooLarge,
		}
		fr, err := ingest.ProcessFile(ctx, deps, req, f, "", reserved)
		if err != nil {
			return upsertspecs.Response{}, err
		}
		results = append(results, fr)
		items = append(items, toItemResult(it.Index, it.SourcePath, fr))
	}
	s := base.Summarize(results)
	return upsertspecs.Response{
		Items:   items,
		Summary: upsertspecs.Summary{Created: s.Created, Updated: s.Updated, Unchanged: s.Unchanged, Skipped: s.Skipped, Rejected: s.Rejected},
	}, nil
}

func toItemResult(index int, rawPath string, fr base.FileResult) upsertspecs.ItemResult {
	sp := rawPath
	if fr.SourcePath != nil {
		sp = *fr.SourcePath
	}
	issues := make([]upsertspecs.Issue, 0, len(fr.Issues))
	for _, is := range fr.Issues {
		issues = append(issues, upsertspecs.Issue{Code: is.Code, Message: is.Message, Pointer: is.Pointer, Line: is.Line})
	}
	return upsertspecs.ItemResult{
		Index: index, SourcePath: sp, Name: fr.Name, Status: string(fr.Status),
		ReasonCode: fr.ReasonCode, Reason: fr.Reason, Issues: issues,
		OpenAPIVersion: fr.OpenAPIVersion, Title: fr.Title, EndpointCount: fr.EndpointCount,
	}
}
