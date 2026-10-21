package callaudit

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
	"time"

	"grimoire/internal/domain/discovery/internal/mcpgateway/base"
	"grimoire/internal/domain/discovery/internal/mcpgateway/base/auditwriter"
	"grimoire/lib/common/reqctx"
	"grimoire/lib/contracts/discovery/mcpgateway/toolkit"
)

const (
	defaultTimeout  = 10 * time.Second
	defaultMaxBytes = 102400
)

// Options mengatur Usecase.
type Options struct {
	Timeout  time.Duration
	MaxBytes int
	Now      func() time.Time
}

// Usecase mengimplementasikan base.ToolInvoker.
type Usecase struct {
	cat   base.Catalog
	audit auditwriter.Writer
	log   *slog.Logger
	opt   Options
}

var _ base.ToolInvoker = (*Usecase)(nil)

// NewUsecase membuat Usecase dengan default defensif untuk opsi nol.
func NewUsecase(cat base.Catalog, audit auditwriter.Writer, log *slog.Logger, opt Options) *Usecase {
	if opt.Timeout <= 0 {
		opt.Timeout = defaultTimeout
	}
	if opt.MaxBytes <= 0 {
		opt.MaxBytes = defaultMaxBytes
	}
	if opt.Now == nil {
		opt.Now = time.Now
	}
	if log == nil {
		log = slog.Default()
	}
	return &Usecase{cat: cat, audit: audit, log: log, opt: opt}
}

type runResult struct {
	res  toolkit.Result
	code string
}

// Invoke menjalankan tool dan menulis tepat satu entri audit. Tidak pernah panic.
func (u *Usecase) Invoke(ctx context.Context, p base.Principal, name string, args json.RawMessage) base.Outcome {
	start := u.opt.Now()
	tool, ok := u.cat.Lookup(name)
	if !ok {
		u.audit.Write(ctx, base.UnknownToolAudit(p, name, u.opt.Now().Sub(start)))
		return base.Outcome{UnknownTool: true}
	}
	run := u.run(ctx, p, tool, args)
	dur := u.opt.Now().Sub(start)
	text, truncated, orig := base.Truncate(run.res.Text, u.opt.MaxBytes)
	run.res.Text = text
	out := base.Outcome{Result: run.res, Truncated: truncated, OriginalBytes: orig, ErrorCode: run.code}
	u.audit.Write(ctx, base.ToolCallAudit(p, tool.Name, base.DecodeArgs(args), out, dur))
	return out
}

type done struct {
	res      toolkit.Result
	err      error
	panicked bool
}

func (u *Usecase) run(ctx context.Context, p base.Principal, tool toolkit.Tool, args json.RawMessage) runResult {
	call := toolkit.Call{ProjectID: p.ProjectID, Arguments: normalizeArgs(args)}
	tctx, cancel := context.WithTimeout(ctx, u.opt.Timeout)
	defer cancel()

	ch := make(chan done, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				u.log.ErrorContext(ctx, "mcp tool panic", "tool", tool.Name, "panic", fmt.Sprint(r), "stack", string(debug.Stack()))
				ch <- done{panicked: true}
			}
		}()
		res, err := tool.Handler(tctx, call)
		ch <- done{res: res, err: err}
	}()

	select {
	case d := <-ch:
		switch {
		case d.panicked:
			return u.failure(ctx, tool.Name, "panic")
		case d.err != nil && isDeadline(tctx, d.err):
			u.log.WarnContext(ctx, "mcp tool timeout", "tool", tool.Name)
			return u.failure(ctx, tool.Name, "timeout")
		case d.err != nil:
			u.log.ErrorContext(ctx, "mcp tool error", "tool", tool.Name, "err", d.err)
			return u.failure(ctx, tool.Name, "internal")
		default:
			return runResult{res: d.res}
		}
	case <-tctx.Done():
		if errors.Is(tctx.Err(), context.DeadlineExceeded) {
			u.log.WarnContext(ctx, "mcp tool timeout", "tool", tool.Name)
			return u.failure(ctx, tool.Name, "timeout")
		}
		u.log.WarnContext(ctx, "mcp tool canceled", "tool", tool.Name)
		return u.failure(ctx, tool.Name, "internal")
	}
}

func isDeadline(tctx context.Context, err error) bool {
	return errors.Is(err, context.DeadlineExceeded) && errors.Is(tctx.Err(), context.DeadlineExceeded)
}

func (u *Usecase) failure(ctx context.Context, name, code string) runResult {
	var msg string
	if code == "timeout" {
		msg = fmt.Sprintf("Tool %s melebihi batas waktu %s. Coba lagi dengan permintaan yang lebih sempit.", name, u.opt.Timeout.String())
	} else {
		tid := reqctx.TraceID(ctx)
		if tid == "" {
			tid = "-"
		}
		msg = fmt.Sprintf("Terjadi kesalahan internal saat menjalankan tool %s. trace_id=%s", name, tid)
	}
	return runResult{res: toolkit.Result{Text: msg, IsError: true}, code: code}
}

func normalizeArgs(raw json.RawMessage) json.RawMessage {
	t := bytes.TrimSpace(raw)
	if len(t) == 0 || t[0] != '{' || !json.Valid(t) {
		return json.RawMessage("{}")
	}
	return raw
}
