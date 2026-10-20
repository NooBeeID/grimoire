package logger

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"grimoire/internal/infra/config"
	"grimoire/lib/common/reqctx"
)

func cfg(level, format string) config.Config {
	return config.Config{LogLevel: level, LogFormat: format}
}

func decode(t *testing.T, b *bytes.Buffer) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(b.Bytes(), &m); err != nil {
		t.Fatalf("%v: %s", err, b.String())
	}
	return m
}

func TestLogger_IncludesTraceAndUser(t *testing.T) {
	var b bytes.Buffer
	ctx := reqctx.WithAPIKeyID(reqctx.WithUserID(reqctx.WithTraceID(context.Background(), "tr"), "us"), "ak")
	NewWithWriter(cfg("info", "json"), &b).InfoContext(ctx, "x")
	m := decode(t, &b)
	if m["trace_id"] != "tr" || m["user_id"] != "us" || m["api_key_id"] != "ak" {
		t.Fatalf("%v", m)
	}
}

func TestLogger_OmitsEmptyContextFields(t *testing.T) {
	var b bytes.Buffer
	NewWithWriter(cfg("info", "json"), &b).InfoContext(context.Background(), "x")
	m := decode(t, &b)
	for _, k := range []string{"trace_id", "user_id", "api_key_id"} {
		if _, ok := m[k]; ok {
			t.Fatalf("unexpected %s", k)
		}
	}
}

func TestLogger_WithAttrsKeepsContextHandler(t *testing.T) {
	var b bytes.Buffer
	ctx := reqctx.WithTraceID(context.Background(), "tr")
	l := NewWithWriter(cfg("info", "json"), &b)
	l.With("a", "b").InfoContext(ctx, "x")
	if decode(t, &b)["trace_id"] != "tr" {
		t.Fatal("With lost trace_id")
	}
	b.Reset()
	l.WithGroup("g").InfoContext(ctx, "x")
	if !strings.Contains(b.String(), "tr") {
		t.Fatal("WithGroup lost trace_id")
	}
}

func TestLogger_DoesNotPrintSecrets(t *testing.T) {
	var b bytes.Buffer
	c, err := config.Load(func(k string) string {
		return map[string]string{"DATABASE_URL": "postgres://u:dbpw@h/d", "SESSION_SECRET": "0123456789abcdef0123456789abcdef"}[k]
	})
	if err != nil {
		t.Fatal(err)
	}
	NewWithWriter(cfg("info", "json"), &b).Info("x", "password", "pw1", "Session_Secret", "ss1", "token", "tk1", "database_url", "du1", "config", c)
	out := b.String()
	for _, s := range []string{"pw1", "ss1", "tk1", "du1", "dbpw", "0123456789abcdef"} {
		if strings.Contains(out, s) {
			t.Fatalf("leak %q: %s", s, out)
		}
	}
	if !strings.Contains(out, "***") {
		t.Fatal("no mask")
	}
}

func TestLogger_LevelAndFormat(t *testing.T) {
	var b bytes.Buffer
	NewWithWriter(cfg("warn", "json"), &b).Info("hidden")
	if b.Len() != 0 {
		t.Fatal("info should be hidden")
	}
	NewWithWriter(cfg("info", "text"), &b).Info("shown")
	if strings.HasPrefix(b.String(), "{") || !strings.Contains(b.String(), "msg=shown") {
		t.Fatalf("not text: %s", b.String())
	}
}
