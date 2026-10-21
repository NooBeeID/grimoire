package endpoint

import (
	"context"
	"log/slog"
	"time"

	"grimoire/internal/domain/discovery/internal/mcpgateway/base"
	"grimoire/internal/domain/discovery/internal/mcpgateway/base/auditwriter"
)

// Usecase men-dispatch satu request JSON-RPC per metode.
type Usecase struct {
	log     *slog.Logger
	version string
	cat     base.Catalog
	inv     base.ToolInvoker
	audit   auditwriter.Writer
	now     func() time.Time
}

func NewUsecase(log *slog.Logger, version string, cat base.Catalog, inv base.ToolInvoker, audit auditwriter.Writer) *Usecase {
	if log == nil {
		log = slog.Default()
	}
	if version == "" {
		version = "dev"
	}
	return &Usecase{log: log, version: version, cat: cat, inv: inv, audit: audit, now: time.Now}
}

// Handle memproses satu request (Kind == KindRequest) dan selalu mengembalikan Response.
func (u *Usecase) Handle(ctx context.Context, p base.Principal, req base.Request) base.Response {
	switch req.Method {
	case "initialize":
		return u.initialize(ctx, p, req)
	case "ping":
		return u.success(ctx, req, struct{}{})
	case "tools/list":
		return u.toolsList(ctx, p, req)
	case "tools/call":
		return u.toolsCall(ctx, p, req)
	default:
		return base.Failure(req.ID, &base.Error{
			Code: base.CodeMethodNotFound, Message: "Method not found",
			Data: map[string]string{"method": req.Method},
		})
	}
}

func (u *Usecase) success(ctx context.Context, req base.Request, result any) base.Response {
	resp, err := base.Success(req.ID, result)
	if err != nil {
		u.log.ErrorContext(ctx, "mcp encode result failed", "method", req.Method, "err", err)
		return base.Failure(req.ID, &base.Error{Code: base.CodeInternal, Message: "Internal error"})
	}
	return resp
}

func invalidParams(req base.Request, pe *paramsError) base.Response {
	return base.Failure(req.ID, &base.Error{
		Code: base.CodeInvalidParams, Message: "Invalid params",
		Data: map[string]string{"reason": pe.Reason},
	})
}

func (u *Usecase) initialize(ctx context.Context, p base.Principal, req base.Request) base.Response {
	start := u.now()
	params, pe := parseInitializeParams(req.Params)
	if pe != nil {
		return invalidParams(req, pe)
	}
	negotiated, ok := base.Negotiate(params.ProtocolVersion)
	if !ok {
		u.audit.Write(ctx, base.InitializeAudit(p, params.ProtocolVersion, "", params.ClientName, params.ClientVersion, false, u.now().Sub(start)))
		return base.Failure(req.ID, &base.Error{
			Code: base.CodeInvalidParams, Message: "Unsupported protocol version",
			Data: map[string]any{"requested": params.ProtocolVersion, "supported": base.SupportedVersions},
		})
	}
	res := initializeResult{
		ProtocolVersion: negotiated,
		ServerInfo:      serverInfo{Name: base.ServerName, Version: u.version},
		Instructions:    base.Instructions(p.ProjectName, u.cat.Names()),
	}
	u.audit.Write(ctx, base.InitializeAudit(p, params.ProtocolVersion, negotiated, params.ClientName, params.ClientVersion, true, u.now().Sub(start)))
	return u.success(ctx, req, res)
}

func (u *Usecase) toolsList(ctx context.Context, p base.Principal, req base.Request) base.Response {
	start := u.now()
	entries := toToolEntries(u.cat.List())
	u.audit.Write(ctx, base.ToolsListAudit(p, len(entries), u.now().Sub(start)))
	return u.success(ctx, req, toolsListResult{Tools: entries})
}

func (u *Usecase) toolsCall(ctx context.Context, p base.Principal, req base.Request) base.Response {
	params, pe := parseToolsCallParams(req.Params)
	if pe != nil {
		return invalidParams(req, pe)
	}
	out := u.inv.Invoke(ctx, p, params.Name, params.Arguments)
	if out.UnknownTool {
		return base.Failure(req.ID, &base.Error{
			Code: base.CodeInvalidParams, Message: "Unknown tool: " + base.TruncateName(params.Name, maxNameBytes),
		})
	}
	return u.success(ctx, req, toCallToolResult(out))
}
