package httpx

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/go-chi/chi/v5"
	"grimoire/internal/infra/bus"
	"grimoire/internal/infra/config"
	"grimoire/internal/infra/logger"
	"grimoire/lib/constants"
	"grimoire/lib/contracts/core/auth/resolvesession"
)

type syncBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuf) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuf) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

// logRecords mengurai log JSON per baris.
func (s *syncBuf) records(t *testing.T) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(s.String()), "\n") {
		if line == "" {
			continue
		}
		m := map[string]any{}
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("baris log bukan JSON: %q", line)
		}
		out = append(out, m)
	}
	return out
}

func (s *syncBuf) find(t *testing.T, msg string) map[string]any {
	t.Helper()
	for _, m := range s.records(t) {
		if m["msg"] == msg {
			return m
		}
	}
	return nil
}

// useLogger membuat logger debug ke buffer dan menjadikannya slog default.
func useLogger(t *testing.T) (*slog.Logger, *syncBuf) {
	t.Helper()
	buf := &syncBuf{}
	log := logger.NewWithWriter(config.Config{LogLevel: "debug", LogFormat: "json"}, buf)
	old := slog.Default()
	slog.SetDefault(log)
	t.Cleanup(func() { slog.SetDefault(old) })
	return log, buf
}

func testConfig(t *testing.T, env map[string]string) config.Config {
	t.Helper()
	base := map[string]string{
		"DATABASE_URL":   "postgres://u:p@localhost:5432/db",
		"SESSION_SECRET": strings.Repeat("s", 32),
		"COOKIE_SECURE":  "false",
	}
	for k, v := range env {
		base[k] = v
	}
	cfg, err := config.Load(func(k string) string { return base[k] })
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

type stack struct {
	root, api, mcp chi.Router
	bus            *bus.Registry
	logs           *syncBuf
	log            *slog.Logger
	cfg            config.Config
}

type fakePinger func(ctx context.Context) error

func (f fakePinger) Ping(ctx context.Context) error { return f(ctx) }

func newStack(t *testing.T, env map[string]string, p Pinger) *stack {
	t.Helper()
	log, buf := useLogger(t)
	cfg := testConfig(t, env)
	reg := bus.NewRegistry(log)
	root, api, mcp := NewRouter(cfg, log, reg, p)
	// chi baru memasang middleware grup setelah ada minimal satu route; module nyata
	// selalu mendaftarkan route, jadi route dummy ini meniru kondisi produksi.
	api.Get("/_test/ping", okHandler)
	return &stack{root: root, api: api, mcp: mcp, bus: reg, logs: buf, log: log, cfg: cfg}
}

// registerSession mendaftarkan provider resolve-session palsu dan mengembalikan penghitung panggilan.
func registerSession(reg *bus.Registry, h func(token string) (resolvesession.Response, error)) *int {
	n := new(int)
	var mu sync.Mutex
	reg.Register(constants.BUS_CORE_AUTH_RESOLVE_SESSION, func(ctx context.Context, req any) (any, error) {
		mu.Lock()
		*n++
		mu.Unlock()
		r, err := bus.Expect[resolvesession.Request](req)
		if err != nil {
			return nil, err
		}
		return h(r.Token)
	})
	return n
}

func do(h http.Handler, method, target string, hdr map[string]string, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, target, strings.NewReader(body))
	r.RemoteAddr = "10.0.0.5:4321"
	for k, v := range hdr {
		r.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

type errResp struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Details any    `json:"details"`
	} `json:"error"`
	TraceID string `json:"trace_id"`
}

func decodeErr(t *testing.T, w *httptest.ResponseRecorder) errResp {
	t.Helper()
	var e errResp
	if err := json.Unmarshal(w.Body.Bytes(), &e); err != nil {
		t.Fatalf("body bukan error JSON: %q (%v)", w.Body.String(), err)
	}
	return e
}

func okHandler(w http.ResponseWriter, r *http.Request) {
	WriteJSON(w, 200, map[string]string{"ok": "1"})
}

func readBody(w http.ResponseWriter, r *http.Request) {
	var v map[string]any
	if err := DecodeJSON(r, &v); err != nil {
		WriteError(w, r, err)
		return
	}
	WriteJSON(w, 200, v)
}
