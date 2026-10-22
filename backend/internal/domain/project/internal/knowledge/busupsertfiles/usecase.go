package busupsertfiles

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"grimoire/internal/domain/project/internal/knowledge/base"
	"grimoire/internal/domain/project/internal/knowledge/base/audit"
	"grimoire/internal/domain/project/internal/knowledge/base/ingest"
	"grimoire/lib/contracts/core/audit/record"
	"grimoire/lib/contracts/project/knowledge/upsertfiles"
)

const maxAuditPaths = 20

// Reason lokal feature ini (tidak mengubah base/reasons.go).
const (
	reasonInvalidActor  base.Reason = "invalid_actor"
	reasonInvalidSource base.Reason = "invalid_source"
)

type Usecase struct {
	repo   repository
	audit  audit.Recorder
	limits base.Limits
}

func NewUsecase(repo repository, au audit.Recorder, limits base.Limits) *Usecase {
	return &Usecase{repo: repo, audit: au, limits: limits}
}

func (u *Usecase) Execute(ctx context.Context, in upsertfiles.Request) (upsertfiles.Response, error) {
	pid, err := uuid.Parse(in.ProjectID)
	if err != nil {
		return upsertfiles.Response{}, base.Invalid(base.ReasonInvalidProjectID)
	}
	if _, err := uuid.Parse(in.Actor.UserID); err != nil {
		return upsertfiles.Response{}, base.InvalidMsg(reasonInvalidActor, "aktor tidak valid")
	}
	if in.Source != upsertfiles.SourceGitSync {
		return upsertfiles.Response{}, base.InvalidMsg(reasonInvalidSource, "source tidak didukung")
	}
	projectID := pid.String()

	prefix, reason := base.NormalizePrefix(in.TargetPrefix)
	if reason != "" {
		return upsertfiles.Response{}, base.InvalidMsg(reason,
			fmt.Sprintf("prefix tujuan %q tidak valid: %s", in.TargetPrefix, reason.Message()))
	}
	if len(in.Items) == 0 {
		return upsertfiles.Response{Items: []upsertfiles.ItemResult{}}, nil
	}

	actor := base.Actor{ID: in.Actor.UserID, Role: in.Actor.Role, Label: in.Actor.Label}
	decisions := make([]base.Decision, 0, len(in.Items))
	incoming := make([]base.Incoming, 0, len(in.Items))
	for _, it := range in.Items {
		raw := it.Path
		if prefix != "" {
			raw = prefix + "/" + it.Path
		}
		inc, dec, ok := base.ClassifyPart(it.Index, raw, it.Content, it.TooLarge, u.limits.MaxFileBytes)
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
		err := u.repo.InTx(ctx, func(tx ingest.Tx) error {
			var err error
			planned, bytesWritten, err = ingest.Apply(ctx, tx, projectID, actor, incoming, u.limits.MaxProjectFiles)
			return err
		})
		if err != nil {
			return upsertfiles.Response{}, fmt.Errorf("busupsertfiles: batch: %w", err)
		}
		all = append(append([]base.Decision(nil), decisions...), planned...)
	}
	base.SortByIndex(all)

	sum := base.Summarize(all)
	paths, trunc := base.ChangedPaths(all, maxAuditPaths)
	u.audit.Record(ctx, audit.Event{
		Action: record.ActionKnowledgeUploadCompleted, ProjectID: projectID, Actor: actor,
		TargetType: audit.TargetKnowledge, Target: "",
		Args: map[string]any{"files": len(in.Items), "source": in.Source},
		Result: map[string]any{
			"created": sum.Created, "updated": sum.Updated, "unchanged": sum.Unchanged,
			"skipped": sum.Skipped, "rejected": sum.Rejected,
			"bytes": bytesWritten, "paths": paths, "paths_truncated": trunc,
		},
		Meta: base.RequestMeta{IP: in.Meta.IP, UserAgent: in.Meta.UserAgent},
	})

	res := upsertfiles.Response{
		Items: make([]upsertfiles.ItemResult, 0, len(all)),
		Summary: upsertfiles.Summary{Created: sum.Created, Updated: sum.Updated,
			Unchanged: sum.Unchanged, Skipped: sum.Skipped, Rejected: sum.Rejected},
	}
	for _, d := range all {
		res.Items = append(res.Items, upsertfiles.ItemResult{
			Index: d.Index, Path: d.Path, Status: string(d.Status), Reason: string(d.Reason)})
	}
	return res, nil
}
