package endpoint

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"grimoire/internal/domain/discovery/internal/mcpgateway/base"
	"grimoire/lib/contracts/core/audit/record"
	"grimoire/lib/contracts/discovery/mcpgateway/toolkit"
)

type fakeAudit struct{ entries []record.Request }

func (f *fakeAudit) Write(_ context.Context, r record.Request) { f.entries = append(f.entries, r) }

type fakeInvoker struct {
	calls int
	name  string
	args  string
	p     base.Principal
	out   base.Outcome
}

func (f *fakeInvoker) Invoke(_ context.Context, p base.Principal, name string, args json.RawMessage) base.Outcome {
	f.calls++
	f.name, f.args, f.p = name, string(args), p
	return f.out
}

var testPrincipal = base.Principal{KeyID: "k1", KeyName: "laptop", KeyPrefix: "a1b2c3d4", ProjectID: "P-A", ProjectName: "Payments"}

func mkTool(name string) toolkit.Tool {
	return toolkit.Tool{
		Name: name, Description: "d", InputSchema: json.RawMessage(`{"type":"object"}`),
		Handler: func(context.Context, toolkit.Call) (toolkit.Result, error) { return toolkit.Result{}, nil },
	}
}

func newUC(t *testing.T, version string, names ...string) (*Usecase, *fakeInvoker, *fakeAudit) {
	t.Helper()
	var tools []toolkit.Tool
	for _, n := range names {
		tools = append(tools, mkTool(n))
	}
	cat, err := base.NewCatalog(tools)
	if err != nil {
		t.Fatal(err)
	}
	inv, au := &fakeInvoker{}, &fakeAudit{}
	return NewUsecase(slog.New(slog.NewTextHandler(io.Discard, nil)), version, cat, inv, au), inv, au
}

func req(id, method, params string) base.Request {
	r := base.Request{JSONRPC: "2.0", ID: json.RawMessage(id), Method: method}
	if params != "" {
		r.Params = json.RawMessage(params)
	}
	return r
}

func body(t *testing.T, r base.Response) string {
	t.Helper()
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestInitialize_SupportedVersions(t *testing.T) {
	for _, v := range base.SupportedVersions {
		u, _, au := newUC(t, "1.4.0", "read_file")
		resp := u.Handle(context.Background(), testPrincipal,
			req("1", "initialize", `{"protocolVersion":"`+v+`","clientInfo":{"name":"cc","version":"2.1"}}`))
		var res initializeResult
		if err := json.Unmarshal(resp.Result, &res); err != nil {
			t.Fatal(err)
		}
		if res.ProtocolVersion != v || res.ServerInfo.Name != "grimoire" || res.ServerInfo.Version != "1.4.0" || res.Capabilities.Tools.ListChanged {
			t.Fatalf("%+v", res)
		}
		if !strings.Contains(res.Instructions, "Payments") || !strings.Contains(res.Instructions, "read_file") {
			t.Fatalf("instructions: %s", res.Instructions)
		}
		if len(au.entries) != 1 || au.entries[0].Action != "mcp.initialize" || au.entries[0].Status != "ok" {
			t.Fatalf("audit: %+v", au.entries)
		}
		if au.entries[0].Args["client_name"] != "cc" || au.entries[0].Args["protocol_version"] != v {
			t.Fatalf("args: %+v", au.entries[0].Args)
		}
	}
}

func TestInitialize_VersionDefaultsToDev(t *testing.T) {
	u, _, _ := newUC(t, "")
	resp := u.Handle(context.Background(), testPrincipal, req("1", "initialize", `{"protocolVersion":"2025-06-18"}`))
	if !strings.Contains(string(resp.Result), `"version":"dev"`) || !strings.Contains(string(resp.Result), "Belum ada tool yang terdaftar.") {
		t.Fatalf("%s", resp.Result)
	}
}

func TestInitialize_UnsupportedVersion(t *testing.T) {
	u, _, au := newUC(t, "1")
	resp := u.Handle(context.Background(), testPrincipal, req("1", "initialize", `{"protocolVersion":"2024-11-05"}`))
	want := `{"jsonrpc":"2.0","id":1,"error":{"code":-32602,"message":"Unsupported protocol version","data":{"requested":"2024-11-05","supported":["2025-11-25","2025-06-18","2025-03-26"]}}}`
	if got := body(t, resp); got != want {
		t.Fatalf("%s", got)
	}
	if len(au.entries) != 1 || au.entries[0].Status != "error" || au.entries[0].Result["error_code"] != "unsupported_version" {
		t.Fatalf("audit: %+v", au.entries)
	}
}

func TestInitialize_BadParamsNoAudit(t *testing.T) {
	u, _, au := newUC(t, "1")
	resp := u.Handle(context.Background(), testPrincipal, req("1", "initialize", `{}`))
	want := `{"jsonrpc":"2.0","id":1,"error":{"code":-32602,"message":"Invalid params","data":{"reason":"protocolVersion must be a string"}}}`
	if body(t, resp) != want || len(au.entries) != 0 {
		t.Fatalf("%s %d", body(t, resp), len(au.entries))
	}
}

func TestPing(t *testing.T) {
	u, inv, au := newUC(t, "1")
	for _, id := range []string{`1`, `"p-1"`} {
		resp := u.Handle(context.Background(), testPrincipal, req(id, "ping", ""))
		if body(t, resp) != `{"jsonrpc":"2.0","id":`+id+`,"result":{}}` {
			t.Fatal(body(t, resp))
		}
	}
	if len(au.entries) != 0 || inv.calls != 0 {
		t.Fatal("ping tidak boleh audit/invoke")
	}
}

func TestToolsList(t *testing.T) {
	u, _, au := newUC(t, "1", "search_knowledge", "list_files", "read_file")
	resp := u.Handle(context.Background(), testPrincipal, req("2", "tools/list", ""))
	var res struct {
		Tools []struct {
			Name        string
			Annotations struct{ ReadOnlyHint bool }
		}
	}
	if err := json.Unmarshal(resp.Result, &res); err != nil {
		t.Fatal(err)
	}
	if len(res.Tools) != 3 || res.Tools[0].Name != "list_files" || res.Tools[2].Name != "search_knowledge" {
		t.Fatalf("%+v", res.Tools)
	}
	for _, tl := range res.Tools {
		if !tl.Annotations.ReadOnlyHint {
			t.Fatal("readOnlyHint")
		}
	}
	if len(au.entries) != 1 || au.entries[0].Action != "mcp.tools_list" || au.entries[0].Result["count"] != 3 {
		t.Fatalf("%+v", au.entries)
	}
}

func TestToolsList_Empty(t *testing.T) {
	u, _, au := newUC(t, "1")
	resp := u.Handle(context.Background(), testPrincipal, req("2", "tools/list", ""))
	if string(resp.Result) != `{"tools":[]}` || au.entries[0].Result["count"] != 0 {
		t.Fatalf("%s %+v", resp.Result, au.entries)
	}
}

func TestDurationFromClock(t *testing.T) {
	u, _, au := newUC(t, "1")
	t0 := time.Unix(0, 0)
	calls := 0
	u.now = func() time.Time {
		calls++
		if calls%2 == 0 {
			return t0.Add(25 * time.Millisecond)
		}
		return t0
	}
	u.Handle(context.Background(), testPrincipal, req("1", "tools/list", ""))
	u.Handle(context.Background(), testPrincipal, req("2", "initialize", `{"protocolVersion":"2025-06-18"}`))
	for _, e := range au.entries {
		if e.Result["duration_ms"] != int64(25) {
			t.Fatalf("%s: %#v", e.Action, e.Result["duration_ms"])
		}
	}
}

func TestToolsCall(t *testing.T) {
	ctx := context.Background()
	t.Run("valid, args utuh, tanpa audit endpoint", func(t *testing.T) {
		u, inv, au := newUC(t, "1", "read_file")
		inv.out.Result.Text = "isi"
		args := `{"path":"x","project_id":"B"}`
		resp := u.Handle(ctx, testPrincipal, req("3", "tools/call", `{"name":"read_file","arguments":`+args+`}`))
		if inv.calls != 1 || inv.name != "read_file" || inv.args != args || inv.p.ProjectID != "P-A" {
			t.Fatalf("%+v", inv)
		}
		if body(t, resp) != `{"jsonrpc":"2.0","id":3,"result":{"content":[{"type":"text","text":"isi"}],"isError":false}}` {
			t.Fatal(body(t, resp))
		}
		if len(au.entries) != 0 {
			t.Fatal("audit dari endpoint")
		}
	})
	t.Run("unknown tool", func(t *testing.T) {
		u, inv, _ := newUC(t, "1")
		inv.out.UnknownTool = true
		resp := u.Handle(ctx, testPrincipal, req("3", "tools/call", `{"name":"write_file"}`))
		if body(t, resp) != `{"jsonrpc":"2.0","id":3,"error":{"code":-32602,"message":"Unknown tool: write_file"}}` {
			t.Fatal(body(t, resp))
		}
		long := strings.Repeat("n", 128)
		resp = u.Handle(ctx, testPrincipal, req("3", "tools/call", `{"name":"`+long+`"}`))
		if resp.Error.Message != "Unknown tool: "+long {
			t.Fatal(resp.Error.Message)
		}
	})
	t.Run("isError dan truncated", func(t *testing.T) {
		u, inv, _ := newUC(t, "1")
		inv.out = base.Outcome{ErrorCode: "timeout", Truncated: true, OriginalBytes: 9}
		inv.out.Result.IsError = true
		resp := u.Handle(ctx, testPrincipal, req("3", "tools/call", `{"name":"x"}`))
		s := string(resp.Result)
		if !strings.Contains(s, `"isError":true`) || !strings.Contains(s, `"original_bytes":9`) {
			t.Fatal(s)
		}
	})
	t.Run("params salah", func(t *testing.T) {
		for _, p := range []string{`{"name":1}`, `{"name":""}`, `{"name":"` + strings.Repeat("a", 129) + `"}`,
			`{"name":"x","arguments":[]}`, `{"name":"x","arguments":null}`, ``} {
			u, inv, au := newUC(t, "1")
			resp := u.Handle(ctx, testPrincipal, req("3", "tools/call", p))
			if resp.Error == nil || resp.Error.Code != base.CodeInvalidParams || resp.Error.Message != "Invalid params" {
				t.Fatalf("%s: %+v", p, resp.Error)
			}
			if inv.calls != 0 || len(au.entries) != 0 {
				t.Fatalf("%s: invoker/audit dipanggil", p)
			}
		}
	})
}

func TestMethodNotFound(t *testing.T) {
	for _, m := range []string{"resources/list", "prompts/list", "completion/complete"} {
		u, inv, au := newUC(t, "1")
		resp := u.Handle(context.Background(), testPrincipal, req("4", m, ""))
		want := `{"jsonrpc":"2.0","id":4,"error":{"code":-32601,"message":"Method not found","data":{"method":"` + m + `"}}}`
		if body(t, resp) != want || inv.calls != 0 || len(au.entries) != 0 {
			t.Fatalf("%s", body(t, resp))
		}
	}
}
