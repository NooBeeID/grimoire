package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"grimoire/internal/infra/http/respond"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

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
	respond.WriteJSON(w, 200, map[string]string{"ok": "1"})
}

type stack struct {
	bus  *bus.Registry
	logs *syncBuf
	log  *slog.Logger
	cfg  config.Config
}

func newStack(t *testing.T, env map[string]string, _ any) *stack {
	t.Helper()
	log, buf := useLogger(t)
	cfg := testConfig(t, env)
	return &stack{bus: bus.NewRegistry(log), logs: buf, log: log, cfg: cfg}
}

func mustPanic(t *testing.T, f func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Fatal("harus panic")
		}
	}()
	f()
}
