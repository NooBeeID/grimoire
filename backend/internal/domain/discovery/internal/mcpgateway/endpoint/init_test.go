package endpoint

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"grimoire/internal/domain/discovery/internal/mcpgateway/base"
	"grimoire/internal/infra/app"
	"grimoire/internal/infra/config"
)

type initFixture struct {
	router   chi.Router
	admitted int
	reject   bool
	inv      *fakeInvoker
}

func newInitFixture(t *testing.T, cfg config.Config) *initFixture {
	t.Helper()
	f := &initFixture{inv: &fakeInvoker{}}
	r := chi.NewRouter()
	cat, err := base.NewCatalog(nil)
	if err != nil {
		t.Fatal(err)
	}
	admit := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			f.admitted++
			if f.reject {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, req.WithContext(base.WithPrincipal(req.Context(), testPrincipal)))
		})
	}
	Init(app.Deps{Config: cfg, Log: slog.New(slog.NewTextHandler(io.Discard, nil)), MCP: r, Version: "1.4.0"}, cat, f.inv, admit, &fakeAudit{})
	f.router = r
	return f
}

func (f *initFixture) do(method, body string, hdr map[string]string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "/", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	for k, v := range hdr {
		r.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	f.router.ServeHTTP(w, r)
	return w
}

var initCfg = config.Config{MCPMaxRequestBytes: 1024, MCPAllowedOrigins: []string{"https://app.example"}}

func TestInit_OriginGuardBeforeAdmit(t *testing.T) {
	f := newInitFixture(t, initCfg)
	w := f.do(http.MethodPost, ping, map[string]string{"Origin": "https://evil.example"})
	if w.Code != 403 || f.admitted != 0 {
		t.Fatalf("%d admitted=%d", w.Code, f.admitted)
	}
	for _, h := range []map[string]string{{"Origin": "https://app.example"}, nil} {
		if w := f.do(http.MethodPost, ping, h); w.Code != 200 {
			t.Fatalf("%v: %d", h, w.Code)
		}
	}
	r := httptest.NewRequest(http.MethodPost, "http://same.host/", strings.NewReader(ping))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Origin", "http://same.host")
	w = httptest.NewRecorder()
	f.router.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("origin = host: %d", w.Code)
	}
}

type spyBody struct{ read bool }

func (s *spyBody) Read([]byte) (int, error) { s.read = true; return 0, io.EOF }

func TestInit_AdmitRunsBeforeBodyRead(t *testing.T) {
	f := newInitFixture(t, initCfg)
	f.reject = true
	spy := &spyBody{}
	r := httptest.NewRequest(http.MethodPost, "/", spy)
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	f.router.ServeHTTP(w, r)
	if w.Code != 401 || spy.read {
		t.Fatalf("%d read=%v", w.Code, spy.read)
	}
}

func TestInit_GetDeleteWithoutAuth(t *testing.T) {
	f := newInitFixture(t, initCfg)
	for _, m := range []string{http.MethodGet, http.MethodDelete} {
		w := f.do(m, "", nil)
		if w.Code != 405 || w.Header().Get("Allow") != "POST" {
			t.Fatalf("%s: %d", m, w.Code)
		}
	}
	if f.admitted != 0 {
		t.Fatal("admit dipanggil")
	}
	f.do(http.MethodPost, ping, nil)
	if f.admitted != 1 {
		t.Fatalf("admitted=%d", f.admitted)
	}
}

func TestInit_BodyLimitFromConfig(t *testing.T) {
	f := newInitFixture(t, initCfg)
	big := `{"jsonrpc":"2.0","id":1,"method":"ping","params":{"x":"` + strings.Repeat("a", 2048) + `"}}`
	w := f.do(http.MethodPost, big, nil)
	if w.Code != 413 {
		t.Fatalf("%d", w.Code)
	}
	var m struct{ Error struct{ Data map[string]any } }
	_ = json.Unmarshal(w.Body.Bytes(), &m)
	if m.Error.Data["limit_bytes"] != float64(1024) {
		t.Fatalf("%v", m.Error.Data)
	}
}
