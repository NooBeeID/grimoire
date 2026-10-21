// Package auditwriter menulis audit gateway MCP lewat bus (core.audit.record). Fail-open.
package auditwriter

import (
	"context"
	"log/slog"

	"grimoire/internal/infra/bus"
	"grimoire/lib/constants"
	"grimoire/lib/contracts/core/audit/record"
)

// Writer menulis satu entri audit. Tidak mengembalikan error (fail-open).
type Writer interface {
	Write(ctx context.Context, req record.Request)
}

type busWriter struct {
	b   *bus.Registry
	log *slog.Logger
}

// New membuat Writer berbasis bus.
func New(b *bus.Registry, log *slog.Logger) Writer {
	if log == nil {
		log = slog.Default()
	}
	return &busWriter{b: b, log: log}
}

// Write memanggil core.audit.record dengan ctx tanpa cancel. Args/Result tidak pernah di-log.
func (w *busWriter) Write(ctx context.Context, req record.Request) {
	ctx = context.WithoutCancel(ctx)
	if _, err := bus.CallTyped[record.Response](ctx, w.b, constants.BUS_CORE_AUDIT_RECORD, req); err != nil {
		w.log.ErrorContext(ctx, "audit write failed", "action", req.Action, "err", err)
	}
}
