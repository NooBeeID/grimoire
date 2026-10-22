// Package audit mengirim catatan audit module api-spec lewat bus (core.audit.record). Fail-open.
package audit

import (
	"context"
	"log/slog"

	"grimoire/internal/domain/project/internal/apispec/base"
	"grimoire/internal/infra/bus"
	"grimoire/lib/common/reqctx"
	"grimoire/lib/constants"
	"grimoire/lib/contracts/core/audit/record"
)

// TargetType adalah jenis target audit.
const TargetType = "api_spec"

// Recorder mencatat satu event audit. Tidak mengembalikan error (fail-open).
type Recorder interface {
	Record(ctx context.Context, ev Event)
}

type Event struct {
	Action    string // record.ActionAPISpec*
	ProjectID string
	Target    string         // nama spec (setelah aksi; rename: nama baru)
	Args      map[string]any // TANPA isi spec
	Meta      base.RequestMeta
	// Actor (opsional) menggantikan identitas dari ctx; dipakai handler bus yang tidak membawa sesi pemanggil.
	Actor *base.Actor
}

type BusRecorder struct {
	b   *bus.Registry
	log *slog.Logger
	now base.Clock
}

var _ Recorder = (*BusRecorder)(nil)

func New(b *bus.Registry, log *slog.Logger, now base.Clock) *BusRecorder {
	if log == nil {
		log = slog.Default()
	}
	if now == nil {
		now = base.SystemClock
	}
	return &BusRecorder{b: b, log: log, now: now}
}

// Record mengirim audit; semua kegagalan hanya di-log (Args tidak pernah di-log).
func (r *BusRecorder) Record(ctx context.Context, ev Event) {
	var actor base.Actor
	ok := true
	if ev.Actor != nil && ev.Actor.ID != "" {
		actor = *ev.Actor
	} else {
		actor, ok = base.ActorFrom(reqctx.IdentityFrom(ctx))
	}
	if !ok {
		r.log.ErrorContext(ctx, "audit record dilewati: identitas tidak ada", "action", ev.Action, "project_id", ev.ProjectID)
		return
	}
	req := record.Request{
		OccurredAt: r.now(),
		ActorType:  record.ActorUser, ActorID: actor.ID, ActorLabel: actor.Label,
		ProjectID: ev.ProjectID, ProjectLabel: "",
		Action: ev.Action, TargetType: TargetType, Target: ev.Target,
		Args: ev.Args, Status: record.StatusOK,
		IP: ev.Meta.IP, UserAgent: ev.Meta.UserAgent, TraceID: reqctx.TraceID(ctx),
	}
	if _, err := bus.CallTyped[record.Response](ctx, r.b, constants.BUS_CORE_AUDIT_RECORD, req); err != nil {
		r.log.ErrorContext(ctx, "audit record gagal", "action", ev.Action, "project_id", ev.ProjectID, "error", err)
	}
}
