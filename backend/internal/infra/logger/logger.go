// Package logger membuat *slog.Logger dengan trace_id/user_id/api_key_id dari reqctx.
// Wajib memakai InfoContext(ctx, ...) dan sejenisnya agar ctx terbaca.
package logger

import (
	"context"
	"io"
	"log/slog"
	"os"
	"strings"

	"grimoire/internal/infra/config"
	"grimoire/lib/common/reqctx"
)

// New membuat logger ke stdout.
func New(cfg config.Config) *slog.Logger { return NewWithWriter(cfg, os.Stdout) }

// NewWithWriter membuat logger ke w.
func NewWithWriter(cfg config.Config, w io.Writer) *slog.Logger {
	var lvl slog.Level
	if err := lvl.UnmarshalText([]byte(cfg.LogLevel)); err != nil {
		lvl = slog.LevelInfo
	}
	opts := &slog.HandlerOptions{Level: lvl, ReplaceAttr: redact}
	var h slog.Handler
	if cfg.LogFormat == "text" {
		h = slog.NewTextHandler(w, opts)
	} else {
		h = slog.NewJSONHandler(w, opts)
	}
	return slog.New(ctxHandler{h})
}

var secretKeys = map[string]bool{
	"password": true, "secret": true, "session_secret": true, "token": true, "database_url": true,
}

func redact(groups []string, a slog.Attr) slog.Attr {
	if secretKeys[strings.ToLower(a.Key)] {
		return slog.String(a.Key, "***")
	}
	return a
}

type ctxHandler struct{ slog.Handler }

func (h ctxHandler) Handle(ctx context.Context, r slog.Record) error {
	if v := reqctx.TraceID(ctx); v != "" {
		r.AddAttrs(slog.String("trace_id", v))
	}
	if v := reqctx.UserID(ctx); v != "" {
		r.AddAttrs(slog.String("user_id", v))
	}
	if v := reqctx.APIKeyID(ctx); v != "" {
		r.AddAttrs(slog.String("api_key_id", v))
	}
	return h.Handler.Handle(ctx, r)
}

func (h ctxHandler) WithAttrs(a []slog.Attr) slog.Handler { return ctxHandler{h.Handler.WithAttrs(a)} }
func (h ctxHandler) WithGroup(n string) slog.Handler      { return ctxHandler{h.Handler.WithGroup(n)} }
