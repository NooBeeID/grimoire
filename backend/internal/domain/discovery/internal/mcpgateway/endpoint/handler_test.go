package endpoint

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"grimoire/internal/domain/discovery/internal/mcpgateway/base"
	"grimoire/internal/infra/middleware"
)

type fakeDispatcher struct {
	calls int
	last  base.Request
	resp  *base.Response
}

func (f *fakeDispatcher) Handle(_ context.Context, _ base.Principal, req base.Request) base.Response {
	f.calls++
	f.last = req
	if f.resp != nil {
		return *f.resp
	}
	r, _ := base.Success(req.ID, struct{}{})
	return r
}

func rawHandler(d dispatcher) *Handler {
	return &Handler{log: slog.New(slog.NewTextHandler(io.Discard, nil)), uc: d, maxBytes: 65536}
}

func newHandler(d dispatcher) http.Handler { return http.HandlerFunc(rawHandler(d).Post) }

type opt func(*http.Request)

func withHeader(k, v string) opt { return func(r *http.Request) { r.Header.Set(k, v) } }
func noContentType() opt         { return func(r *http.Request) { r.Header.Del("Content-Type") } }
func noPrincipal() opt {
	return func(r *http.Request) { *r = *r.WithContext(context.Background()) }
}

func doPost(h http.Handler, body string, opts ...opt) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	*r = *r.WithContext(base.WithPrincipal(r.Context(), testPrincipal))
	for _, o := range opts {
		o(r)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func rpcError(t *testing.T, w *httptest.ResponseRecorder) (code int, data map[string]any) {
	t.Helper()
	var m struct {
		JSONRPC string
		ID      json.RawMessage
		Error   *struct {
			Code int
			Data map[string]any
		}
	}
	if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
		t.Fatalf("body bukan JSON: %q", w.Body.String())
	}
	if m.JSONRPC != "2.0" || string(m.ID) != "null" || m.Error == nil {
		t.Fatalf("bentuk salah: %s", w.Body.String())
	}
	return m.Error.Code, m.Error.Data
}

const ping = `{"jsonrpc":"2.0","id":1,"method":"ping"}`

func TestPost_Rejections(t *testing.T) {
	tests := []struct {
		name   string
		body   string
		opts   []opt
		status int
		code   int
		reason string
	}{
		{"CT kosong", ping, []opt{noContentType()}, 415, -32600, "Content-Type must be application/json"},
		{"CT text/plain", ping, []opt{withHeader("Content-Type", "text/plain")}, 415, -32600, ""},
		{"body kosong", "", nil, 400, -32700, ""},
		{"JSON rusak", `{"jsonrpc":`, nil, 400, -32700, ""},
		{"batch", `[` + ping + `]`, nil, 400, -32600, "batch not supported"},
		{"angka", `123`, nil, 400, -32600, ""},
		{"string", `"x"`, nil, 400, -32600, ""},
		{"null", `null`, nil, 400, -32600, ""},
		{"jsonrpc salah", `{"jsonrpc":"1.0","id":1,"method":"ping"}`, nil, 400, -32600, ""},
		{"id null", `{"jsonrpc":"2.0","id":null,"method":"ping"}`, nil, 400, -32600, ""},
		{"id bool", `{"jsonrpc":"2.0","id":true,"method":"ping"}`, nil, 400, -32600, ""},
		{"id object", `{"jsonrpc":"2.0","id":{},"method":"ping"}`, nil, 400, -32600, ""},
		{"params array", `{"jsonrpc":"2.0","id":1,"method":"ping","params":[]}`, nil, 400, -32600, ""},
		{"header versi tak dikenal", ping, []opt{withHeader("MCP-Protocol-Version", "2024-11-05")}, 400, -32600, "unsupported MCP-Protocol-Version"},
		{"header versi tak dikenal + notification", `{"jsonrpc":"2.0","method":"notifications/initialized"}`, []opt{withHeader("MCP-Protocol-Version", "1999-01-01")}, 400, -32600, ""},
		{"principal hilang", ping, []opt{noPrincipal()}, 500, -32603, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := &fakeDispatcher{}
			w := doPost(newHandler(d), tc.body, tc.opts...)
			if w.Code != tc.status || d.calls != 0 {
				t.Fatalf("status=%d calls=%d body=%s", w.Code, d.calls, w.Body)
			}
			code, data := rpcError(t, w)
			if code != tc.code {
				t.Fatalf("code=%d", code)
			}
			if tc.reason != "" && data["reason"] != tc.reason {
				t.Fatalf("data=%v", data)
			}
			if w.Header().Get("Content-Type") != "application/json" {
				t.Fatal("content-type")
			}
		})
	}
}

func TestPost_HeaderVersionSupported(t *testing.T) {
	for _, v := range base.SupportedVersions {
		w := doPost(newHandler(&fakeDispatcher{}), ping, withHeader("MCP-Protocol-Version", v))
		if w.Code != 200 {
			t.Fatalf("%s: %d", v, w.Code)
		}
	}
	// tanpa header dianggap versi default
	if w := doPost(newHandler(&fakeDispatcher{}), `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`); w.Code != 200 {
		t.Fatal(w.Code)
	}
	// initialize melewati pemeriksaan header
	w := doPost(newHandler(&fakeDispatcher{}), `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`, withHeader("MCP-Protocol-Version", "2024-11-05"))
	if w.Code != 200 {
		t.Fatal(w.Code)
	}
}

func TestPost_ContentTypeVariants(t *testing.T) {
	for _, ct := range []string{"application/json; charset=utf-8", "APPLICATION/JSON"} {
		if w := doPost(newHandler(&fakeDispatcher{}), ping, withHeader("Content-Type", ct)); w.Code != 200 {
			t.Fatalf("%s: %d", ct, w.Code)
		}
	}
}

func TestPost_Accepted(t *testing.T) {
	for _, b := range []string{
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","method":"notifications/cancelled","params":{}}`,
		`{"jsonrpc":"2.0","id":1,"result":{}}`,
	} {
		d := &fakeDispatcher{}
		w := doPost(newHandler(d), b)
		if w.Code != 202 || w.Body.Len() != 0 || w.Header().Get("Content-Type") != "" || d.calls != 0 {
			t.Fatalf("%s: %d %q", b, w.Code, w.Body)
		}
	}
}

func TestPost_Success(t *testing.T) {
	d := &fakeDispatcher{}
	w := doPost(newHandler(d), ping)
	if w.Code != 200 || w.Header().Get("Content-Type") != "application/json" || w.Header().Get("Mcp-Session-Id") != "" {
		t.Fatalf("%d %v", w.Code, w.Header())
	}
	if w.Body.String() != `{"jsonrpc":"2.0","id":1,"result":{}}` {
		t.Fatal(w.Body.String())
	}
	w = doPost(newHandler(d), `{"jsonrpc":"2.0","id":"p-1","method":"ping"}`)
	if w.Body.String() != `{"jsonrpc":"2.0","id":"p-1","result":{}}` {
		t.Fatal(w.Body.String())
	}
}

func TestPost_DispatcherErrorStays200(t *testing.T) {
	r := base.Failure(json.RawMessage("1"), &base.Error{Code: base.CodeMethodNotFound, Message: "Method not found"})
	w := doPost(newHandler(&fakeDispatcher{resp: &r}), ping)
	if w.Code != 200 {
		t.Fatal(w.Code)
	}
}

func TestPost_BodyLimit(t *testing.T) {
	pad := func(n int) string {
		prefix := `{"jsonrpc":"2.0","id":1,"method":"ping","params":{"x":"`
		suffix := `"}}`
		return prefix + strings.Repeat("a", n-len(prefix)-len(suffix)) + suffix
	}
	h := middleware.BodyLimit(65536)(newHandler(&fakeDispatcher{}))
	w := doPost(h, pad(70*1024))
	if w.Code != 413 {
		t.Fatalf("status=%d", w.Code)
	}
	code, data := rpcError(t, w)
	if code != -32600 || data["limit_bytes"] != float64(65536) {
		t.Fatalf("%d %v", code, data)
	}
	if w := doPost(h, pad(65536)); w.Code != 200 {
		t.Fatalf("tepat di batas: %d", w.Code)
	}
}

func TestPost_NoInternalLeak(t *testing.T) {
	w := doPost(newHandler(&fakeDispatcher{}), ping, noPrincipal())
	b := w.Body.String()
	for _, s := range []string{"principal", "goroutine", ".go", "Reason"} {
		if strings.Contains(b, s) {
			t.Fatalf("bocor %q: %s", s, b)
		}
	}
}

func TestMethodNotAllowed(t *testing.T) {
	h := rawHandler(&fakeDispatcher{})
	for _, m := range []string{http.MethodGet, http.MethodDelete} {
		w := httptest.NewRecorder()
		h.MethodNotAllowed(w, httptest.NewRequest(m, "/mcp", nil))
		if w.Code != 405 || w.Header().Get("Allow") != "POST" {
			t.Fatalf("%s: %d %v", m, w.Code, w.Header())
		}
		code, data := rpcError(t, w)
		if code != -32600 || data["reason"] != "method not allowed; use POST" {
			t.Fatalf("%d %v", code, data)
		}
	}
}
