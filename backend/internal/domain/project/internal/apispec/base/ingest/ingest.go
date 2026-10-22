// Package ingest adalah satu jalur proses+simpan satu file spec: dipakai upload multipart dan
// sync Git (K7). Rule parse/validasi tetap di base dan base/openapi; paket ini mengorkestrasi
// satu file (satu transaksi per file) dan audit.
package ingest

import (
	"context"
	"errors"
	"fmt"
	"path"
	"time"

	"grimoire/internal/domain/project/internal/apispec/base"
	"grimoire/internal/domain/project/internal/apispec/base/audit"
	"grimoire/internal/domain/project/internal/apispec/base/openapi"
	"grimoire/internal/domain/project/internal/apispec/base/store"
	"grimoire/internal/domain/project/internal/apispec/base/upload"
	"grimoire/lib/contracts/core/audit/record"
)

// Tx adalah operasi DB di dalam satu transaksi.
type Tx interface {
	LockProjectSpecs(ctx context.Context, projectID string) error
	LockSpec(ctx context.Context, projectID, name string) (base.Spec, bool, error)
	CountSpecs(ctx context.Context, projectID string) (int, error)
	InsertSpec(ctx context.Context, s *base.Spec) error
	UpdateSpecContent(ctx context.Context, s base.Spec) error
	ReplaceEndpoints(ctx context.Context, specID string, es []base.Endpoint) error
}

// Repo membuka transaksi.
type Repo interface {
	InTx(ctx context.Context, fn func(Tx) error) error
}

// Deps adalah dependensi bersama.
type Deps struct {
	Repo         Repo
	Audit        audit.Recorder
	Clock        base.Clock
	MaxSpecs     int
	MaxFileBytes int64
}

// Request adalah konteks satu batch. Source "" = upload multipart; selain itu (mis. "git_sync")
// ditambahkan ke Args audit. Actor dikirim eksplisit ke audit (tidak bergantung ctx).
type Request struct {
	ProjectID string
	Actor     base.Actor
	Meta      base.RequestMeta
	Source    string
}

// Reject menandai hasil file sebagai ditolak dengan kode reason dan pesan baku.
func Reject(fr base.FileResult, code string, args ...any) base.FileResult {
	fr.Status = base.StatusRejected
	fr.ReasonCode = code
	fr.Reason = base.ReasonMessage(code, args...)
	return fr
}

// ProcessFile memproses satu file: normalisasi path, deteksi format, nama, parse, simpan (satu tx),
// lalu audit setelah commit. Kegagalan validasi menjadi hasil rejected/skipped (bukan error);
// error hanya untuk kegagalan infrastruktur. reserved melacak nama yang dipakai batch ini.
func ProcessFile(ctx context.Context, d Deps, req Request, f upload.File, explicitName string, reserved map[string]struct{}) (base.FileResult, error) {
	fr := base.FileResult{File: f.FileName, Issues: []base.Issue{}}
	if f.Rejected == base.ReasonTooManyFiles {
		return Reject(fr, base.ReasonTooManyFiles), nil
	}
	sourcePath, err := base.NormalizeSourcePath(f.RawPath)
	if err != nil {
		return Reject(fr, base.ReasonInvalidPath), nil
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
		return Reject(fr, base.ReasonFileTooLarge, d.MaxFileBytes), nil
	}
	name := explicitName
	if name == "" {
		derived, derr := base.DeriveName(path.Base(sourcePath))
		if derr != nil {
			return Reject(fr, base.ReasonInvalidName), nil
		}
		name = derived
	}
	if err := base.ValidateName(name); err != nil {
		return Reject(fr, base.ReasonInvalidName), nil
	}
	fr.Name = name
	if _, dup := reserved[name]; dup {
		return Reject(fr, base.ReasonDuplicateNameInUpload), nil
	}

	content := openapi.StripBOM(f.Content)
	doc, issues := openapi.Parse(content, openapi.Format(format), openapi.DefaultLimits)
	if len(issues) > 0 {
		fr = Reject(fr, base.ReasonInvalidSpec)
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

	decision, err := persist(ctx, d, req.ProjectID, name, sourcePath, format, string(content), meta, eps, req.Actor)
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
		fr = Reject(fr, base.ReasonSpecLimitReached, d.MaxSpecs)
		fr.OpenAPIVersion, fr.Title, fr.APIVersion, fr.EndpointCount = "", "", "", 0
		return fr, nil
	}
	reserved[name] = struct{}{}
	if decision == base.DecisionCreate || decision == base.DecisionUpdate {
		args := map[string]any{
			"name": name, "source_path": sourcePath, "openapi_version": doc.OpenAPIVersion,
			"endpoint_count": len(eps), "size_bytes": len(content), "overwritten": decision == base.DecisionUpdate,
		}
		if req.Source != "" {
			args["source"] = req.Source
		}
		actor := req.Actor
		d.Audit.Record(context.WithoutCancel(ctx), audit.Event{
			Action:    record.ActionAPISpecUploaded,
			ProjectID: req.ProjectID,
			Target:    name,
			Args:      args,
			Actor:     &actor,
			Meta:      req.Meta,
		})
	}
	return fr, nil
}

// persist menyimpan satu file dalam satu tx; diulang sekali bila store.ErrNameTaken (U6).
func persist(ctx context.Context, d Deps, projectID, name, sourcePath, format, content string,
	meta base.DocMeta, eps []base.Endpoint, actor base.Actor) (base.UploadDecision, error) {
	hash := base.HashContent(content)
	clock := d.Clock
	if clock == nil {
		clock = func() time.Time { return time.Now() }
	}
	var decision base.UploadDecision
	var err error
	for attempt := 0; attempt < 2; attempt++ {
		err = d.Repo.InTx(ctx, func(tx Tx) error {
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
			decision = base.DecideUpload(ex, hash, count, d.MaxSpecs)
			var saved base.Spec
			switch decision {
			case base.DecisionUnchanged, base.DecisionRejectLimit:
				return nil
			case base.DecisionCreate:
				saved = base.NewSpec(projectID, name, &sourcePath, meta, content, format, actor, clock())
				if err := tx.InsertSpec(ctx, &saved); err != nil {
					return err
				}
			case base.DecisionUpdate:
				saved = existing
				saved.ApplyContent(&sourcePath, meta, content, format, actor, clock())
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
	return 0, fmt.Errorf("ingest: simpan %q: %w", name, err)
}
