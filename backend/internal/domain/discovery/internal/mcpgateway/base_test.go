package mcpgateway

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"grimoire/internal/infra/app"
	"grimoire/internal/infra/bus"
	"grimoire/internal/infra/config"
	"grimoire/lib/constants"
	"grimoire/lib/contracts/core/audit/record"
	"grimoire/lib/contracts/discovery/mcpgateway/toolkit"
	"grimoire/lib/contracts/project/apikey/verify"
	"grimoire/lib/contracts/project/management/getproject"
)

const (
	projectA = "00000000-0000-7000-8000-00000000000a"
	projectB = "00000000-0000-7000-8000-00000000000b"
	goodKey  = "gr_aaaaaaaa_secretsecretsecret"
	secret   = "SECRET-CONTENT-123"
)

type fixture struct {
	router   chi.Router
	verifies int
	mu       sync.Mutex
	audit    []record.Request
	calls    []toolkit.Call
	readText string
}

func (f *fixture) actions(action string) []record.Request {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []record.Request
	for _, r := range f.audit {
		if r.Action == action {
			out = append(out, r)
		}
	}
	return out
}

func newFixture(t *testing.T, withTool bool) *fixture {
	t.Helper()
	f := &fixture{readText: secret}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	b := bus.NewRegistry(log)
	b.Register(constants.BUS_PROJECT_APIKEY_VERIFY, func(_ context.Context, req any) (any, error) {
		f.mu.Lock()
		f.verifies++
		f.mu.Unlock()
		if req.(verify.Request).RawKey == goodKey {
			return verify.Response{Found: true, KeyID: "key-1", KeyName: "laptop", Prefix: "aaaaaaaa", ProjectID: projectA}, nil
		}
		return verify.Response{Reason: verify.ReasonUnknown}, nil
	})
	b.Register(constants.BUS_PROJECT_MANAGEMENT_GET_PROJECT, func(context.Context, any) (any, error) {
		return getproject.Response{Found: true, ID: projectA, Name: "Payments"}, nil
	})
	b.Register(constants.BUS_PROJECT_APIKEY_TOUCH, func(context.Context, any) (any, error) { return nil, nil })
	b.Register(constants.BUS_CORE_AUDIT_RECORD, func(_ context.Context, req any) (any, error) {
		f.mu.Lock()
		f.audit = append(f.audit, req.(record.Request))
		f.mu.Unlock()
		return record.Response{Recorded: true}, nil
	})

	r := chi.NewRouter()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	d := app.Deps{
		Ctx: ctx, Log: log, Bus: b, MCP: r, Version: "1.4.0",
		Config: config.Config{
			MCPRatePerMinute: 1000, MCPAuthFailPerMinute: 1000, MCPMaxRequestBytes: 65536,
			MCPMaxResponseBytes: 102400, MCPToolTimeout: 2 * time.Second,
			MCPAllowedOrigins: []string{"https://app.example"},
		},
	}
	var tools []toolkit.Tool
	if withTool {
		tools = []toolkit.Tool{{
			Name: "read_file", Description: "baca", InputSchema: json.RawMessage(`{"type":"object"}`),
			Handler: func(_ context.Context, c toolkit.Call) (toolkit.Result, error) {
				f.mu.Lock()
				f.calls = append(f.calls, c)
				f.mu.Unlock()
				return toolkit.Result{Text: f.readText, TargetType: "file", Target: "a.md"}, nil
			},
		}}
	}
	Register(d, tools)
	f.router = r
	return f
}

func (f *fixture) post(body string, hdr map[string]string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Authorization", "Bearer "+goodKey)
	for k, v := range hdr {
		r.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	f.router.ServeHTTP(w, r)
	return w
}

type rpcResp struct {
	Result json.RawMessage
	Error  *struct {
		Code    int
		Message string
	}
}

func decode(t *testing.T, w *httptest.ResponseRecorder) rpcResp {
	t.Helper()
	var r rpcResp
	if err := json.Unmarshal(w.Body.Bytes(), &r); err != nil {
		t.Fatalf("body: %q", w.Body)
	}
	return r
}

func TestT07_AC1_JSONRPCFlow(t *testing.T) {
	f := newFixture(t, true)
	w := f.post(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18"}}`, nil)
	if w.Code != 200 {
		t.Fatal(w.Code)
	}
	var init struct {
		ProtocolVersion string
		ServerInfo      struct{ Name string }
	}
	_ = json.Unmarshal(decode(t, w).Result, &init)
	if init.ProtocolVersion != "2025-06-18" || init.ServerInfo.Name != "grimoire" {
		t.Fatalf("%+v", init)
	}
	if w := f.post(`{"jsonrpc":"2.0","method":"notifications/initialized"}`, nil); w.Code != 202 || w.Body.Len() != 0 {
		t.Fatalf("%d %q", w.Code, w.Body)
	}
	w = f.post(`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`, nil)
	if !strings.Contains(w.Body.String(), `"name":"read_file"`) || !strings.Contains(w.Body.String(), `"readOnlyHint":true`) {
		t.Fatal(w.Body.String())
	}
	w = f.post(`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"read_file","arguments":{"path":"a.md"}}}`, nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), secret) {
		t.Fatal(w.Body.String())
	}
	for _, a := range []string{"mcp.initialize", "mcp.tools_list", "mcp.read_file"} {
		if n := len(f.actions(a)); n != 1 {
			t.Fatalf("%s: %d entri", a, n)
		}
	}
}

func TestT07_AC3_ProjectIDFromKeyOnly(t *testing.T) {
	f := newFixture(t, true)
	f.post(`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"read_file","arguments":{"path":"x","project_id":"`+projectB+`"}}}`, nil)
	if len(f.calls) != 1 || f.calls[0].ProjectID != projectA || !strings.Contains(string(f.calls[0].Arguments), projectB) {
		t.Fatalf("%+v", f.calls)
	}
	a := f.actions("mcp.read_file")
	if len(a) != 1 || a[0].ProjectID != projectA {
		t.Fatalf("%+v", a)
	}
}

func TestT07_AC4_ExactlyOneAuditNoContent(t *testing.T) {
	f := newFixture(t, true)
	f.post(`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"read_file","arguments":{}}}`, nil)
	if len(f.actions("mcp.read_file")) != 1 {
		t.Fatal("harus tepat 1 entri")
	}
	for _, e := range f.audit {
		b, _ := json.Marshal(e)
		if strings.Contains(string(b), secret) {
			t.Fatalf("isi bocor di audit: %s", e.Action)
		}
	}
}

func TestT07_AC9_ForeignOrigin(t *testing.T) {
	f := newFixture(t, true)
	w := f.post(`{"jsonrpc":"2.0","id":1,"method":"ping"}`, map[string]string{"Origin": "https://evil.example"})
	if w.Code != 403 || f.verifies != 0 {
		t.Fatalf("%d verifies=%d", w.Code, f.verifies)
	}
	if w := f.post(`{"jsonrpc":"2.0","id":1,"method":"ping"}`, map[string]string{"Origin": "https://app.example"}); w.Code != 200 {
		t.Fatal(w.Code)
	}
	if w := f.post(`{"jsonrpc":"2.0","id":1,"method":"ping"}`, nil); w.Code != 200 {
		t.Fatal(w.Code)
	}
}

func TestT07_GetDeleteMethodNotAllowed(t *testing.T) {
	f := newFixture(t, true)
	for _, m := range []string{http.MethodGet, http.MethodDelete} {
		w := httptest.NewRecorder()
		f.router.ServeHTTP(w, httptest.NewRequest(m, "/", nil))
		if w.Code != 405 || w.Header().Get("Allow") != "POST" {
			t.Fatalf("%s: %d", m, w.Code)
		}
	}
	if f.verifies != 0 {
		t.Fatal("verify dipanggil")
	}
}

func TestT07_UnknownTool(t *testing.T) {
	f := newFixture(t, true)
	r := decode(t, f.post(`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"write_file"}}`, nil))
	if r.Error == nil || r.Error.Code != -32602 || r.Error.Message != "Unknown tool: write_file" {
		t.Fatalf("%+v", r.Error)
	}
	if len(f.actions("mcp.unknown_tool")) != 1 {
		t.Fatal("audit unknown_tool")
	}
}

func TestT07_InvalidParamsNoToolAudit(t *testing.T) {
	f := newFixture(t, true)
	r := decode(t, f.post(`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":""}}`, nil))
	if r.Error == nil || r.Error.Code != -32602 {
		t.Fatalf("%+v", r.Error)
	}
	if len(f.audit) != 0 || len(f.calls) != 0 {
		t.Fatalf("audit=%d calls=%d", len(f.audit), len(f.calls))
	}
}

func TestT07_EmptyCatalog(t *testing.T) {
	f := newFixture(t, false)
	w := f.post(`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`, nil)
	if !strings.Contains(w.Body.String(), `"tools":[]`) {
		t.Fatal(w.Body.String())
	}
}

func TestT07_AC7_TruncatedThroughFullPath(t *testing.T) {
	f := newFixture(t, true)
	f.readText = strings.Repeat("x", 500*1024)
	w := f.post(`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"read_file"}}`, nil)
	var res struct {
		Content []struct{ Text string }
		Meta    struct{ Truncated bool } `json:"_meta"`
	}
	_ = json.Unmarshal(decode(t, w).Result, &res)
	if !res.Meta.Truncated || len(res.Content) != 1 || len(res.Content[0].Text) > 102400 {
		t.Fatalf("truncated=%v len=%d", res.Meta.Truncated, len(res.Content[0].Text))
	}
}

func TestT07_AC8_RegisterPanicsOnDuplicate(t *testing.T) {
	tool := toolkit.Tool{
		Name: "read_file", InputSchema: json.RawMessage(`{"type":"object"}`),
		Handler: func(context.Context, toolkit.Call) (toolkit.Result, error) { return toolkit.Result{}, nil },
	}
	defer func() {
		msg, _ := recover().(string)
		if !strings.HasPrefix(msg, "mcpgateway:") || !strings.Contains(msg, `"read_file"`) {
			t.Fatalf("panic = %q", msg)
		}
	}()
	Register(app.Deps{}, []toolkit.Tool{tool, tool})
	t.Fatal("harus panic")
}
