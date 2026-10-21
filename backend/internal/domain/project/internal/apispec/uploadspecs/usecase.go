// Package uploadspecs menerima satu atau banyak file spec OpenAPI, memvalidasi, dan menyimpannya per project.
package uploadspecs

import (
	"context"
	"errors"
	"fmt"
	"path"
	"strings"

	"grimoire/internal/domain/project/internal/apispec/base"
	"grimoire/internal/domain/project/internal/apispec/base/access"
	"grimoire/internal/domain/project/internal/apispec/base/audit"
	"grimoire/internal/domain/project/internal/apispec/base/openapi"
	"grimoire/internal/domain/project/internal/apispec/base/store"
	"grimoire/internal/domain/project/internal/apispec/base/upload"
	"grimoire/lib/common/errs"
	"grimoire/lib/common/reqctx"
	"grimoire/lib/contracts/core/audit/record"
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

// Tx adalah operasi DB di dalam satu transaksi.
type Tx interface {
	LockProjectSpecs(ctx context.Context, projectID string) error
	LockSpec(ctx context.Context, projectID, name string) (base.Spec, bool, error)
	CountSpecs(ctx context.Context, projectID string) (int, error)
	InsertSpec(ctx context.Context, s *base.Spec) error
	UpdateSpecContent(ctx context.Context, s base.Spec) error
	ReplaceEndpoints(ctx context.Context, specID string, es []base.Endpoint) error
}

type Repository interface {
	InTx(ctx context.Context, fn func(Tx) error) error
}

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
	results := make([]base.FileResult, 0, len(in.Files))
	reserved := map[string]struct{}{}
	for _, f := range in.Files {
		if err := ctx.Err(); err != nil {
			return Output{}, fmt.Errorf("uploadspecs: dibatalkan: %w", err)
		}
		fr, err := u.processFile(ctx, in, actor, f, explicit, reserved)
		if err != nil {
			return Output{}, err
		}
		results = append(results, fr)
	}
	return Output{Results: results, Summary: base.Summarize(results)}, nil
}

func reject(fr base.FileResult, code string, args ...any) base.FileResult {
	fr.Status = base.StatusRejected
	fr.ReasonCode = code
	fr.Reason = base.ReasonMessage(code, args...)
	return fr
}

func (u *Usecase) processFile(ctx context.Context, in Input, actor base.Actor, f upload.File, explicit string, reserved map[string]struct{}) (base.FileResult, error) {
	fr := base.FileResult{File: f.FileName, Issues: []base.Issue{}}
	if f.Rejected == base.ReasonTooManyFiles {
		return reject(fr, base.ReasonTooManyFiles), nil
	}
	sourcePath, err := base.NormalizeSourcePath(f.RawPath)
	if err != nil {
		return reject(fr, base.ReasonInvalidPath), nil
	}
	fr.SourcePath = &sourcePath
	format, ok := base.FormatFromExt(path.Base(sourcePath))
	if !ok {
		fr.Status = base.StatusSkipped
		fr.ReasonCode = base.ReasonNotSpecFile
		fr.Reason = base.ReasonMessage(base.ReasonNotSpecFile)
		return fr, nil
	}
	if f.TooLarge {
		return reject(fr, base.ReasonFileTooLarge, u.maxFileBytes), nil
	}
	name := explicit
	if name == "" {
		derived, derr := base.DeriveName(path.Base(sourcePath))
		if derr != nil {
			return reject(fr, base.ReasonInvalidName), nil
		}
		name = derived
	}
	if err := base.ValidateName(name); err != nil {
		return reject(fr, base.ReasonInvalidName), nil
	}
	fr.Name = name
	if _, dup := reserved[name]; dup {
		return reject(fr, base.ReasonDuplicateNameInUpload), nil
	}

	content := openapi.StripBOM(f.Content)
	doc, issues := openapi.Parse(content, openapi.Format(format), openapi.DefaultLimits)
	if len(issues) > 0 {
		fr = reject(fr, base.ReasonInvalidSpec)
		fr.Reason = issues[0].Message
		n := min(len(issues), openapi.MaxIssues)
		fr.Issues = make([]base.Issue, 0, n)
		for _, is := range issues[:n] {
			fr.Issues = append(fr.Issues, base.Issue{Code: is.Code, Message: is.Message, Pointer: is.Pointer, Line: is.Line})
		}
		return fr, nil
	}
	eps := openapi.ExtractEndpoints(doc)
	meta := base.DocMeta{Title: doc.Title, APIVersion: doc.APIVersion, OpenAPIVersion: doc.OpenAPIVersion}
	fr.OpenAPIVersion, fr.Title, fr.APIVersion, fr.EndpointCount = doc.OpenAPIVersion, doc.Title, doc.APIVersion, len(eps)

	decision, err := u.persist(ctx, in.ProjectID, name, sourcePath, format, string(content), meta, eps, actor)
	if err != nil {
		return base.FileResult{}, err
	}
	switch decision {
	case base.DecisionCreate:
		fr.Status = base.StatusCreated
	case base.DecisionUpdate:
		fr.Status = base.StatusUpdated
	case base.DecisionUnchanged:
		fr.Status = base.StatusUnchanged
	default:
		fr = reject(fr, base.ReasonSpecLimitReached, u.maxSpecs)
		fr.OpenAPIVersion, fr.Title, fr.APIVersion, fr.EndpointCount = "", "", "", 0
		return fr, nil
	}
	reserved[name] = struct{}{}
	if decision == base.DecisionCreate || decision == base.DecisionUpdate {
		u.audit.Record(context.WithoutCancel(ctx), audit.Event{
			Action:    record.ActionAPISpecUploaded,
			ProjectID: in.ProjectID,
			Target:    name,
			Args: map[string]any{
				"name": name, "source_path": sourcePath, "openapi_version": doc.OpenAPIVersion,
				"endpoint_count": len(eps), "size_bytes": len(content), "overwritten": decision == base.DecisionUpdate,
			},
			Meta: in.Meta,
		})
	}
	return fr, nil
}

// persist menyimpan satu file dalam satu tx; diulang sekali bila store.ErrNameTaken (U6).
func (u *Usecase) persist(ctx context.Context, projectID, name, sourcePath, format, content string,
	meta base.DocMeta, eps []base.Endpoint, actor base.Actor) (base.UploadDecision, error) {
	hash := base.HashContent(content)
	var decision base.UploadDecision
	var err error
	for attempt := 0; attempt < 2; attempt++ {
		err = u.repo.InTx(ctx, func(tx Tx) error {
			if err := tx.LockProjectSpecs(ctx, projectID); err != nil {
				return err
			}
			existing, found, err := tx.LockSpec(ctx, projectID, name)
			if err != nil {
				return err
			}
			count, err := tx.CountSpecs(ctx, projectID)
			if err != nil {
				return err
			}
			var ex *base.Spec
			if found {
				ex = &existing
			}
			decision = base.DecideUpload(ex, hash, count, u.maxSpecs)
			var saved base.Spec
			switch decision {
			case base.DecisionUnchanged, base.DecisionRejectLimit:
				return nil
			case base.DecisionCreate:
				saved = base.NewSpec(projectID, name, &sourcePath, meta, content, format, actor, u.clock())
				if err := tx.InsertSpec(ctx, &saved); err != nil {
					return err
				}
			case base.DecisionUpdate:
				saved = existing
				saved.ApplyContent(&sourcePath, meta, content, format, actor, u.clock())
				if err := tx.UpdateSpecContent(ctx, saved); err != nil {
					return err
				}
			}
			return tx.ReplaceEndpoints(ctx, saved.ID, eps)
		})
		if err == nil {
			return decision, nil
		}
		if !errors.Is(err, store.ErrNameTaken) {
			break
		}
	}
	return 0, fmt.Errorf("uploadspecs: simpan %q: %w", name, err)
}
