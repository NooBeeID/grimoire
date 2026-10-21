package validatespec

import (
	"context"
	"fmt"

	"grimoire/internal/domain/project/internal/apispec/base"
	"grimoire/internal/domain/project/internal/apispec/base/access"
	"grimoire/internal/domain/project/internal/apispec/base/openapi"
	"grimoire/internal/domain/project/internal/apispec/base/upload"
)

// Usecase memvalidasi file spec tanpa efek samping.
type Usecase struct {
	guard  access.Checker
	limits base.UploadLimits // hanya MaxFileBytes dan MaxFiles dipakai
}

func NewUsecase(guard access.Checker, limits base.UploadLimits) *Usecase {
	return &Usecase{guard: guard, limits: limits}
}

// Result adalah hasil validasi seluruh file.
type Result struct {
	Results []base.FileResult
	Summary base.ValidateSummaryDTO
}

// Execute: RequireMember, validasi nama eksplisit, lalu per file berurutan.
func (u *Usecase) Execute(ctx context.Context, p Params) (Result, error) {
	if err := u.guard.RequireMember(ctx, p.ProjectID); err != nil {
		return Result{}, err
	}
	if p.Name != nil {
		if err := base.ValidateName(*p.Name); err != nil {
			return Result{}, err
		}
	}
	seen := map[string]struct{}{}
	res := Result{Results: make([]base.FileResult, 0, len(p.Files))}
	for _, f := range p.Files {
		if err := ctx.Err(); err != nil {
			return Result{}, fmt.Errorf("validatespec: %w", err)
		}
		r := u.checkFile(f, p.Name, seen)
		switch r.Status {
		case base.StatusValid:
			res.Summary.Valid++
		case base.StatusSkipped:
			res.Summary.Skipped++
		default:
			res.Summary.Rejected++
		}
		res.Results = append(res.Results, r)
	}
	return res, nil
}

func reject(r base.FileResult, status base.FileStatus, code string, args ...any) base.FileResult {
	r.Status = status
	r.ReasonCode = code
	r.Reason = base.ReasonMessage(code, args...)
	return r
}

// checkFile tanpa I/O dan deterministik; berhenti pada langkah pertama yang menolak/melewati.
func (u *Usecase) checkFile(f upload.File, explicit *string, seen map[string]struct{}) base.FileResult {
	r := base.FileResult{File: f.FileName, Issues: []base.Issue{}}
	sp, pathErr := base.NormalizeSourcePath(f.RawPath)
	if pathErr == nil {
		r.SourcePath = &sp
	}
	format, ok := base.FormatFromExt(f.FileName)
	if !ok {
		return reject(r, base.StatusSkipped, base.ReasonNotSpecFile)
	}
	if f.Rejected == base.ReasonTooManyFiles {
		return reject(r, base.StatusRejected, base.ReasonTooManyFiles)
	}
	if f.TooLarge {
		return reject(r, base.StatusRejected, base.ReasonFileTooLarge, u.limits.MaxFileBytes)
	}
	if pathErr != nil {
		return reject(r, base.StatusRejected, base.ReasonInvalidPath)
	}
	var name string
	if explicit != nil {
		name = *explicit
	} else {
		n, err := base.DeriveName(f.FileName)
		if err != nil {
			return reject(r, base.StatusRejected, base.ReasonInvalidName)
		}
		name = n
	}
	r.Name = name
	if _, dup := seen[name]; dup {
		return reject(r, base.StatusRejected, base.ReasonDuplicateNameInUpload)
	}
	seen[name] = struct{}{}

	content := openapi.StripBOM(f.Content)
	doc, issues := openapi.Parse(content, openapi.Format(format), openapi.DefaultLimits)
	if len(issues) > 0 {
		r = reject(r, base.StatusRejected, base.ReasonInvalidSpec)
		r.Reason = issues[0].Message
		n := len(issues)
		if n > openapi.MaxIssues {
			n = openapi.MaxIssues
		}
		r.Issues = make([]base.Issue, n)
		for i := 0; i < n; i++ {
			is := issues[i]
			r.Issues[i] = base.Issue{Code: is.Code, Message: is.Message, Pointer: is.Pointer, Line: is.Line}
		}
		return r
	}
	eps := openapi.ExtractEndpoints(doc)
	r.Status = base.StatusValid
	r.OpenAPIVersion = doc.OpenAPIVersion
	r.Title = doc.Title
	r.APIVersion = doc.APIVersion
	r.EndpointCount = len(eps)
	return r
}
