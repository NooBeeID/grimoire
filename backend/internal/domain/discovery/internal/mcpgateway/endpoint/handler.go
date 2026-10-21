package endpoint

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"

	"grimoire/internal/domain/discovery/internal/mcpgateway/base"
)

type dispatcher interface {
	Handle(ctx context.Context, p base.Principal, req base.Request) base.Response
}

// Handler adalah lapisan transport HTTP untuk /mcp.
type Handler struct {
	log      *slog.Logger
	uc       dispatcher
	maxBytes int64
}

const staticInternalError = `{"jsonrpc":"2.0","id":null,"error":{"code":-32603,"message":"Internal error"}}`

func (h *Handler) Post(w http.ResponseWriter, r *http.Request) {
	p, ok := base.PrincipalFrom(r.Context())
	if !ok {
		h.log.ErrorContext(r.Context(), "mcp principal missing")
		h.writeJSON(w, r, http.StatusInternalServerError, base.HTTPReject(base.CodeInternal, "Internal error", nil))
		return
	}
	mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mt != "application/json" {
		h.reject(w, r, http.StatusUnsupportedMediaType, base.CodeInvalidRequest, "Invalid Request",
			map[string]string{"reason": "Content-Type must be application/json"})
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			h.reject(w, r, http.StatusRequestEntityTooLarge, base.CodeInvalidRequest, "Invalid Request",
				map[string]any{"reason": "request body too large", "limit_bytes": h.maxBytes})
			return
		}
		h.log.DebugContext(r.Context(), "mcp read body failed", "err", err)
		h.reject(w, r, http.StatusBadRequest, base.CodeParseError, "Parse error", nil)
		return
	}
	kind, req, perr := base.ParseMessage(body)
	switch kind {
	case base.KindParseError:
		h.reject(w, r, http.StatusBadRequest, base.CodeParseError, "Parse error", nil)
		return
	case base.KindBatch, base.KindInvalid:
		resp := base.Failure(nil, perr)
		if perr == nil {
			resp = base.HTTPReject(base.CodeInvalidRequest, "Invalid Request", nil)
		}
		h.writeJSON(w, r, http.StatusBadRequest, resp)
		return
	}
	if !(kind == base.KindRequest && req.Method == "initialize") {
		if _, ok := base.KnownHeaderVersion(r.Header.Get(base.HeaderProtocolVersion)); !ok {
			h.reject(w, r, http.StatusBadRequest, base.CodeInvalidRequest, "Invalid Request",
				map[string]any{"reason": "unsupported MCP-Protocol-Version", "supported": base.SupportedVersions})
			return
		}
	}
	if kind != base.KindRequest {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	h.writeJSON(w, r, http.StatusOK, h.uc.Handle(r.Context(), p, req))
}

func (h *Handler) MethodNotAllowed(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Allow", http.MethodPost)
	h.reject(w, r, http.StatusMethodNotAllowed, base.CodeInvalidRequest, "Invalid Request",
		map[string]string{"reason": "method not allowed; use POST"})
}

func (h *Handler) reject(w http.ResponseWriter, r *http.Request, status, code int, msg string, data any) {
	h.writeJSON(w, r, status, base.HTTPReject(code, msg, data))
}

func (h *Handler) writeJSON(w http.ResponseWriter, r *http.Request, status int, resp base.Response) {
	b, err := json.Marshal(resp)
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		h.log.ErrorContext(r.Context(), "mcp encode response failed", "err", err)
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(staticInternalError))
		return
	}
	w.WriteHeader(status)
	_, _ = w.Write(b)
}
