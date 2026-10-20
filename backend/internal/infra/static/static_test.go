package static

import (
	"bufio"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"testing/fstest"

	"grimoire/internal/infra/config"
)

func fixture() fstest.MapFS {
	return fstest.MapFS{
		"index.html":            {Data: []byte("<html>INDEX</html>")},
		"assets/app-abc123.js":  {Data: []byte("console.log(1)")},
		"assets/app-abc123.css": {Data: []byte("a{}")},
		"favicon.svg":           {Data: []byte("<svg/>")},
	}
}

func newH(t *testing.T, cfg config.Config) http.Handler {
	t.Helper()
	cfg.AppEnv = firstNonEmpty(cfg.AppEnv, "production")
	h, err := New(cfg, fixture(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func do(h http.Handler, method, target string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, nil)
	rec := httptest.NewRecorder()
	rec.Header().Set("X-Request-Id", "trace-1")
	h.ServeHTTP(rec, req)
	return rec
}

func TestStatic_RootServesIndexNoCache(t *testing.T) {
	rec := do(newH(t, config.Config{}), "GET", "/")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "INDEX") {
		t.Fatalf("got %d %q", rec.Code, rec.Body.String())
	}
	if !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/html") || rec.Header().Get("Cache-Control") != "no-cache" {
		t.Fatalf("headers: %v", rec.Header())
	}
}

func TestStatic_IndexHTMLDirect(t *testing.T) {
	rec := do(newH(t, config.Config{}), "GET", "/index.html")
	if rec.Code != 200 || rec.Header().Get("Location") != "" {
		t.Fatalf("got %d", rec.Code)
	}
}

func TestStatic_SPAFallback(t *testing.T) {
	h := newH(t, config.Config{})
	for _, p := range []string{"/projects/abc", "/admin/users/1/", "/apix"} {
		rec := do(h, "GET", p)
		if rec.Code != 200 || !strings.Contains(rec.Body.String(), "INDEX") || rec.Header().Get("Cache-Control") != "no-cache" {
			t.Fatalf("%s: %d %v", p, rec.Code, rec.Header())
		}
	}
}

func TestStatic_AssetsImmutable(t *testing.T) {
	h := newH(t, config.Config{})
	for _, c := range []struct{ p, ct string }{{"/assets/app-abc123.js", "javascript"}, {"/assets/app-abc123.css", "css"}} {
		rec := do(h, "GET", c.p)
		if rec.Code != 200 || rec.Header().Get("Cache-Control") != "public, max-age=31536000, immutable" ||
			!strings.Contains(rec.Header().Get("Content-Type"), c.ct) {
			t.Fatalf("%s: %d %v", c.p, rec.Code, rec.Header())
		}
	}
}

func TestStatic_NonAssetFileNoCache(t *testing.T) {
	rec := do(newH(t, config.Config{}), "GET", "/favicon.svg")
	if rec.Code != 200 || rec.Header().Get("Cache-Control") != "no-cache" {
		t.Fatalf("got %d %v", rec.Code, rec.Header())
	}
}

func TestStatic_MissingAssetWithExtension404(t *testing.T) {
	h := newH(t, config.Config{})
	for _, p := range []string{"/assets/missing.js", "/foo.png", "/assets/noext", "/assets/", "/assets"} {
		rec := do(h, "GET", p)
		if rec.Code != 404 || strings.Contains(rec.Body.String(), "INDEX") {
			t.Fatalf("%s: %d %q", p, rec.Code, rec.Body.String())
		}
	}
}

func TestStatic_NeverHandlesAPIOrMCP(t *testing.T) {
	h := newH(t, config.Config{})
	for _, m := range []string{"GET", "POST"} {
		for _, p := range []string{"/api", "/api/x", "/api/x/y.js", "/mcp", "/mcp/x", "/api/../api/x"} {
			rec := do(h, m, p)
			b := rec.Body.String()
			if rec.Code != 404 || !strings.Contains(rec.Header().Get("Content-Type"), "application/json") ||
				!strings.Contains(b, `"code":"NOT_FOUND"`) || !strings.Contains(b, "trace_id") || strings.Contains(b, "INDEX") {
				t.Fatalf("%s %s: %d %q", m, p, rec.Code, b)
			}
		}
	}
}

func TestStatic_PostReturns405(t *testing.T) {
	h := newH(t, config.Config{})
	for _, c := range []struct{ m, p string }{{"POST", "/"}, {"DELETE", "/projects/abc"}} {
		rec := do(h, c.m, c.p)
		if rec.Code != 405 || rec.Header().Get("Allow") != "GET, HEAD" || !strings.Contains(rec.Body.String(), `"code":"METHOD_NOT_ALLOWED"`) {
			t.Fatalf("%s %s: %d %v %q", c.m, c.p, rec.Code, rec.Header(), rec.Body.String())
		}
	}
}

func TestStatic_HeadRequest(t *testing.T) {
	h := newH(t, config.Config{})
	get, head := do(h, "GET", "/"), do(h, "HEAD", "/")
	if head.Code != 200 || head.Body.Len() != 0 {
		t.Fatalf("head: %d %d", head.Code, head.Body.Len())
	}
	for _, k := range []string{"Content-Type", "Cache-Control"} {
		if get.Header().Get(k) != head.Header().Get(k) {
			t.Fatalf("header %s differs", k)
		}
	}
}

func TestStatic_PathTraversal(t *testing.T) {
	h := newH(t, config.Config{})
	for _, p := range []string{"/../etc/passwd", "/%2e%2e/x", "/assets/../index.html", "/%00", "/..%2f..%2fetc/passwd"} {
		req := httptest.NewRequest("GET", "http://x"+p, nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		b := rec.Body.String()
		if rec.Code >= 500 || strings.Contains(b, "root:") {
			t.Fatalf("%s: %d %q", p, rec.Code, b)
		}
		if rec.Code == 200 && !strings.Contains(b, "INDEX") {
			t.Fatalf("%s: unexpected 200 body %q", p, b)
		}
	}
}

func TestStatic_ConditionalGet(t *testing.T) {
	h := newH(t, config.Config{})
	req := httptest.NewRequest("GET", "/assets/app-abc123.js", nil)
	req.Header.Set("If-Modified-Since", "Mon, 02 Jan 2006 15:04:05 GMT")
	req.Header.Set("If-None-Match", `"x"`)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code >= 500 {
		t.Fatalf("got %d", rec.Code)
	}
	req = httptest.NewRequest("GET", "/assets/app-abc123.js", nil)
	req.Header.Set("Range", "bytes=0-6")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 206 || rec.Body.String() != "console" {
		t.Fatalf("range: %d %q", rec.Code, rec.Body.String())
	}
}

func TestNew_ErrorWithoutIndex(t *testing.T) {
	if _, err := New(config.Config{}, fstest.MapFS{"x.txt": {}}, nil); err == nil {
		t.Fatal("want error")
	}
}

func TestNew_ErrorBadViteURL(t *testing.T) {
	if _, err := New(config.Config{AppEnv: "development", DevViteURL: "::bad"}, fixture(), nil); err == nil {
		t.Fatal("want error")
	}
}

func TestStatic_DevProxy(t *testing.T) {
	var apiHits atomic.Int32
	var gotHost, gotUpgrade atomic.Value
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api") {
			apiHits.Add(1)
		}
		gotHost.Store(r.Host)
		if strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
			gotUpgrade.Store(r.Header.Get("Upgrade"))
			hj := w.(http.Hijacker)
			conn, buf, _ := hj.Hijack()
			defer conn.Close()
			_, _ = buf.WriteString("HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n\r\n")
			_ = buf.Flush()
			line, _ := buf.ReadString('\n')
			_, _ = buf.WriteString("echo:" + line)
			_ = buf.Flush()
			return
		}
		_, _ = w.Write([]byte("vite:" + r.URL.Path))
	}))
	defer fake.Close()

	h := newH(t, config.Config{AppEnv: "development", DevViteURL: fake.URL})
	for _, p := range []string{"/", "/src/main.ts", "/@vite/client"} {
		rec := do(h, "GET", p)
		want := "vite:" + p
		if rec.Code != 200 || rec.Body.String() != want {
			t.Fatalf("%s: %d %q", p, rec.Code, rec.Body.String())
		}
	}
	if host := gotHost.Load().(string); host != strings.TrimPrefix(fake.URL, "http://") {
		t.Fatalf("host = %s", host)
	}
	rec := do(h, "GET", "/api/x")
	if rec.Code != 404 || !strings.Contains(rec.Body.String(), `"code":"NOT_FOUND"`) || apiHits.Load() != 0 {
		t.Fatalf("api: %d %q hits=%d", rec.Code, rec.Body.String(), apiHits.Load())
	}

	// WebSocket upgrade diteruskan lewat proxy.
	front := httptest.NewServer(h)
	defer front.Close()
	conn, err := net.Dial("tcp", strings.TrimPrefix(front.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_, _ = conn.Write([]byte("GET /ws HTTP/1.1\r\nHost: x\r\nConnection: Upgrade\r\nUpgrade: websocket\r\n\r\n"))
	br := bufio.NewReader(conn)
	status, _ := br.ReadString('\n')
	if !strings.Contains(status, "101") || gotUpgrade.Load() == nil {
		t.Fatalf("upgrade status %q", status)
	}
}

func TestStatic_DevProxyUnreachable(t *testing.T) {
	closed := httptest.NewServer(http.NotFoundHandler())
	url := closed.URL
	closed.Close()
	h := newH(t, config.Config{AppEnv: "development", DevViteURL: url})
	rec := do(h, "GET", "/")
	if rec.Code != 502 || !strings.Contains(rec.Body.String(), "vite dev server tidak berjalan") {
		t.Fatalf("got %d %q", rec.Code, rec.Body.String())
	}
}

func TestStatic_ProductionIgnoresViteURL(t *testing.T) {
	var hits atomic.Int32
	fake := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hits.Add(1) }))
	defer fake.Close()
	h := newH(t, config.Config{AppEnv: "production", DevViteURL: fake.URL})
	rec := do(h, "GET", "/")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "INDEX") || hits.Load() != 0 {
		t.Fatalf("got %d hits=%d", rec.Code, hits.Load())
	}
}

func TestEmbedded_HasIndex(t *testing.T) {
	h, err := New(config.Config{AppEnv: "production"}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if rec := do(h, "GET", "/"); rec.Code != 200 {
		t.Fatalf("got %d", rec.Code)
	}
}

func TestT00_AC2_EmbeddedFSServes(t *testing.T) {
	h, err := Register(config.Config{AppEnv: "production"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	mux.Handle("/", h)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	get := func(p string) (int, string, string) {
		resp, err := http.Get(srv.URL + p)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, resp.Header.Get("Content-Type"), string(b)
	}
	for _, p := range []string{"/", "/some/route"} {
		if c, ct, _ := get(p); c != 200 || !strings.HasPrefix(ct, "text/html") {
			t.Fatalf("%s: %d %s", p, c, ct)
		}
	}
	if c, ct, _ := get("/healthz"); c != 200 || !strings.Contains(ct, "json") {
		t.Fatalf("healthz: %d %s", c, ct)
	}
	if c, ct, _ := get("/api/nope"); c != 404 || !strings.Contains(ct, "json") {
		t.Fatalf("api: %d %s", c, ct)
	}
	if c, _, _ := get("/assets/nonexistent.js"); c != 404 {
		t.Fatalf("asset: %d", c)
	}
}
