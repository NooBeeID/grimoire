// Package audit mengirim catatan audit module auth lewat bus (core.audit.record).
// Fail-open: kegagalan hanya di-log, tidak pernah menggagalkan request.
package audit

import (
	"context"
	"log/slog"
	"strings"

	"grimoire/internal/domain/core/internal/auth/base"
	"grimoire/internal/infra/bus"
	"grimoire/lib/common/reqctx"
	"grimoire/lib/constants"
	"grimoire/lib/contracts/core/audit/record"
)

// Recorder mencatat satu event audit. Tidak mengembalikan error (fail-open).
type Recorder interface {
	Record(ctx context.Context, ev Event)
}

// Actor adalah pelaku eksplisit (mis. login gagal untuk email dikenal, atau system "cli").
type Actor struct{ Type, ID, Label string }

// Event adalah satu aksi yang dicatat.
type Event struct {
	Action     string
	TargetType string         // "user" | "session"
	Target     string         // id user target
	Args       map[string]any // TIDAK pernah berisi password/token/hash
	Status     string         // ok | denied | error (kosong -> ok)
	Result     map[string]any
	Meta       base.RequestMeta
	Actor      *Actor // nil -> dari reqctx.IdentityFrom(ctx); tanpa identitas -> anonymous
}

// BusRecorder mengirim event ke core.audit.record dengan bus.Call.
type BusRecorder struct {
	bus *bus.Registry
	log *slog.Logger
}

var _ Recorder = (*BusRecorder)(nil)

// New membuat BusRecorder.
func New(b *bus.Registry, log *slog.Logger) *BusRecorder {
	if log == nil {
		log = slog.Default()
	}
	return &BusRecorder{bus: b, log: log}
}

// Record membangun record.Request lalu memanggil bus. Error hanya di-log.
func (r *BusRecorder) Record(ctx context.Context, ev Event) {
	req := buildRequest(ctx, ev)
	if _, err := bus.CallTyped[record.Response](ctx, r.bus, constants.BUS_CORE_AUDIT_RECORD, req); err != nil {
		r.log.ErrorContext(ctx, "audit record gagal", "action", ev.Action, "trace_id", req.TraceID, "error", err)
	}
}

func buildRequest(ctx context.Context, ev Event) record.Request {
	req := record.Request{
		ActorType:  record.ActorAnonymous,
		Action:     ev.Action,
		TargetType: ev.TargetType,
		Target:     ev.Target,
		Args:       sanitize(ev.Args),
		Result:     sanitize(ev.Result),
		Status:     ev.Status,
		IP:         ev.Meta.IP,
		UserAgent:  ev.Meta.UserAgent,
		TraceID:    reqctx.TraceID(ctx),
	}
	if req.Status == "" {
		req.Status = record.StatusOK
	}
	switch {
	case ev.Actor != nil:
		req.ActorType, req.ActorID, req.ActorLabel = ev.Actor.Type, ev.Actor.ID, ev.Actor.Label
	default:
		if id, ok := reqctx.IdentityFrom(ctx); ok {
			req.ActorType, req.ActorID, req.ActorLabel = record.ActorUser, id.UserID, id.Email
		}
	}
	return req
}

var sensitiveKeys = []string{"password", "token", "secret", "hash"}

func sensitive(key string) bool {
	k := strings.ToLower(key)
	for _, s := range sensitiveKeys {
		if strings.Contains(k, s) {
			return true
		}
	}
	return false
}

// sanitize menyalin m tanpa kunci sensitif (rekursif pada map bersarang).
func sanitize(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		if sensitive(k) {
			continue
		}
		if nested, ok := v.(map[string]any); ok {
			v = sanitize(nested)
		}
		out[k] = v
	}
	return out
}
