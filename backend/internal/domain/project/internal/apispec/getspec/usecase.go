// Package getspec melayani GET /api/projects/{projectID}/api-specs/{name}.
package getspec

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"grimoire/internal/domain/project/internal/apispec/base"
	"grimoire/internal/domain/project/internal/apispec/base/access"
	"grimoire/internal/domain/project/internal/apispec/base/openapi"
	"grimoire/lib/common/errs"
	"grimoire/lib/common/reqctx"
)

// repository dipenuhi *store.Store.
type repository interface {
	FindSpec(ctx context.Context, projectID, name string) (base.Spec, bool, error)
	ListSpecEndpoints(ctx context.Context, specID string) ([]base.Endpoint, error)
}

// Result adalah detail spec: metadata, indeks endpoint, dan dokumen terparsir (urutan kunci asli).
type Result struct {
	Spec      base.SpecSummary
	Endpoints []base.Endpoint
	Document  json.RawMessage
}

type Usecase struct {
	repo  repository
	guard access.Checker
	log   *slog.Logger
}

func NewUsecase(repo repository, guard access.Checker, log *slog.Logger) *Usecase {
	if log == nil {
		log = slog.Default()
	}
	return &Usecase{repo: repo, guard: guard, log: log}
}

func (u *Usecase) Execute(ctx context.Context, projectID, name string) (Result, error) {
	if _, ok := base.ActorFrom(reqctx.IdentityFrom(ctx)); !ok {
		return Result{}, errs.New(errs.CodeUnauthorized, "perlu login")
	}
	if err := u.guard.RequireMember(ctx, projectID); err != nil {
		return Result{}, err
	}
	sp, found, err := u.repo.FindSpec(ctx, projectID, name)
	if err != nil {
		return Result{}, fmt.Errorf("getspec: find spec: %w", err)
	}
	if !found {
		return Result{}, base.ErrSpecNotFound
	}
	eps, err := u.repo.ListSpecEndpoints(ctx, sp.ID)
	if err != nil {
		return Result{}, fmt.Errorf("getspec: list endpoints: %w", err)
	}

	doc, issues := openapi.Parse([]byte(sp.Content), openapi.Format(sp.Format), openapi.DefaultLimits)
	if len(issues) > 0 {
		u.log.ErrorContext(ctx, "getspec: parse ulang spec tersimpan gagal",
			"feature", "getspec", "project_id", projectID, "spec", name, "issue_code", issues[0].Code)
		return Result{}, fmt.Errorf("getspec: reparse stored spec %s: %s", sp.ID, issues[0].Code)
	}
	raw, err := openapi.ToJSON(doc)
	if err != nil {
		return Result{}, fmt.Errorf("getspec: marshal document: %w", err)
	}
	return Result{Spec: summaryOf(sp, len(eps)), Endpoints: eps, Document: raw}, nil
}

func summaryOf(sp base.Spec, n int) base.SpecSummary {
	return base.SpecSummary{
		ID: sp.ID, ProjectID: sp.ProjectID, Name: sp.Name, SourcePath: sp.SourcePath,
		Title: sp.Title, APIVersion: sp.APIVersion, OpenAPIVersion: sp.OpenAPIVersion, Format: sp.Format,
		SizeBytes: sp.SizeBytes, EndpointCount: n,
		CreatedAt: sp.CreatedAt, CreatedByLabel: sp.CreatedByLabel,
		UpdatedAt: sp.UpdatedAt, UpdatedByLabel: sp.UpdatedByLabel,
	}
}
