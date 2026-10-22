// Package uploadspecs menerima satu atau banyak file spec OpenAPI, memvalidasi, dan menyimpannya per project.
package uploadspecs

import (
	"context"
	"fmt"
	"strings"

	"grimoire/internal/domain/project/internal/apispec/base"
	"grimoire/internal/domain/project/internal/apispec/base/access"
	"grimoire/internal/domain/project/internal/apispec/base/audit"
	"grimoire/internal/domain/project/internal/apispec/base/ingest"
	"grimoire/internal/domain/project/internal/apispec/base/upload"
	"grimoire/lib/common/errs"
	"grimoire/lib/common/reqctx"
)

// Input diisi handler. Files berasal dari upload.Read.
type Input struct {
	ProjectID string
	Files     []upload.File
	Name      *string
	Meta      base.RequestMeta
}

type Output struct {
	Results []base.FileResult
	Summary base.UploadSummary
}

// Tx adalah operasi DB di dalam satu transaksi (didefinisikan di base/ingest).
type Tx = ingest.Tx

// Repository membuka transaksi.
type Repository = ingest.Repo

type Usecase struct {
	repo         Repository
	guard        access.Checker
	audit        audit.Recorder
	clock        base.Clock
	maxSpecs     int
	maxFileBytes int64
}

func NewUsecase(repo Repository, guard access.Checker, rec audit.Recorder, clock base.Clock, maxSpecs int, maxFileBytes int64) *Usecase {
	return &Usecase{repo: repo, guard: guard, audit: rec, clock: clock, maxSpecs: maxSpecs, maxFileBytes: maxFileBytes}
}

// Execute memproses seluruh file berurutan. Error infrastruktur membatalkan sisa request.
func (u *Usecase) Execute(ctx context.Context, in Input) (Output, error) {
	actor, ok := base.ActorFrom(reqctx.IdentityFrom(ctx))
	if !ok {
		return Output{}, errs.New(errs.CodeUnauthorized, "perlu login")
	}
	if err := u.guard.RequireMember(ctx, in.ProjectID); err != nil {
		return Output{}, err
	}
	var explicit string
	if in.Name != nil {
		if n := strings.TrimSpace(*in.Name); n != "" {
			if err := base.ValidateName(n); err != nil {
				return Output{}, base.ErrInvalidName
			}
			explicit = n
		}
	}
	deps := ingest.Deps{Repo: u.repo, Audit: u.audit, Clock: u.clock, MaxSpecs: u.maxSpecs, MaxFileBytes: u.maxFileBytes}
	req := ingest.Request{ProjectID: in.ProjectID, Actor: actor, Meta: in.Meta}
	results := make([]base.FileResult, 0, len(in.Files))
	reserved := map[string]struct{}{}
	for _, f := range in.Files {
		if err := ctx.Err(); err != nil {
			return Output{}, fmt.Errorf("uploadspecs: dibatalkan: %w", err)
		}
		fr, err := ingest.ProcessFile(ctx, deps, req, f, explicit, reserved)
		if err != nil {
			return Output{}, err
		}
		results = append(results, fr)
	}
	return Output{Results: results, Summary: base.Summarize(results)}, nil
}
