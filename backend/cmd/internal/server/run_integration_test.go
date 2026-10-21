package server

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"grimoire/internal/infra/app"
	"grimoire/internal/infra/config"
)

const testSecret = "0123456789abcdef0123456789abcdef-secret"

// testDBURL membuat schema unik dan mengembalikan URL dengan search_path ke schema itu.
func testDBURL(t *testing.T) (dbURL, schema string, admin *sql.DB) {
	t.Helper()
	base := os.Getenv("TEST_DATABASE_URL")
	if base == "" {
		if os.Getenv("CI") == "true" {
			t.Fatal("TEST_DATABASE_URL wajib di CI")
		}
		t.Skip("TEST_DATABASE_URL tidak di-set; lewati test integrasi")
	}
	admin, err := sql.Open("pgx", base)
	if err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 4)
	_, _ = rand.Read(buf)
	schema = "t_" + hex.EncodeToString(buf)
	if _, err := admin.Exec("CREATE SCHEMA " + schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = admin.Exec("DROP SCHEMA " + schema + " CASCADE")
		_ = admin.Close()
	})
	u, err := url.Parse(base)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	// public ikut di search_path: ekstensi citext (migrasi auth) dipasang di sana.
	_, _ = admin.Exec("CREATE EXTENSION IF NOT EXISTS citext SCHEMA public")
	q.Set("search_path", schema+",public")
	u.RawQuery = q.Encode()
	return u.String(), schema, admin
}

func testCfg(t *testing.T, dbURL string, extra map[string]string) config.Config {
	t.Helper()
	env := map[string]string{
		"DATABASE_URL":   dbURL,
		"SESSION_SECRET": testSecret,
		"HTTP_ADDR":      "127.0.0.1:0",
		"COOKIE_SECURE":  "false",
	}
	for k, v := range extra {
		env[k] = v
	}
	cfg, err := config.Load(func(k string) string { return env[k] })
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

type running struct {
	base   string
	cancel context.CancelFunc
	done   chan error
}

func startRun(t *testing.T, cfg config.Config, opts ...Option) *running {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	ready := make(chan string, 1)
	done := make(chan error, 1)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	go func() { done <- Run(ctx, cfg, log, append(opts, WithListenReady(ready))...) }()
	select {
	case addr := <-ready:
		r := &running{base: "http://" + addr, cancel: cancel, done: done}
		t.Cleanup(cancel)
		return r
	case err := <-done:
		cancel()
		t.Fatalf("Run berhenti sebelum listen: %v", err)
	case <-time.After(10 * time.Second):
		cancel()
		t.Fatal("timeout menunggu listen")
	}
	return nil
}

func (r *running) wait(t *testing.T, d time.Duration) error {
	t.Helper()
	select {
	case err := <-r.done:
		return err
	case <-time.After(d):
		t.Fatal("Run tidak berhenti")
		return nil
	}
}

func do(t *testing.T, method, u string) (*http.Response, string) {
	t.Helper()
	req, _ := http.NewRequest(method, u, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp, string(b)
}

func TestT00_AC2_ServeWithOnlyRequiredEnv(t *testing.T) {
	dbURL, _, _ := testDBURL(t)
	// chi baru memasang middleware sub-router setelah ada minimal satu route;
	// hook mewakili route domain yang akan ada (T01+).
	r := startRun(t, testCfg(t, dbURL, nil), WithAppHook(func(d app.Deps) {
		d.API.Get("/_test/ping", func(w http.ResponseWriter, _ *http.Request) {})
	}))

	resp, _ := do(t, "GET", r.base+"/")
	if resp.StatusCode != 200 || !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/html") || resp.Header.Get("Cache-Control") != "no-cache" {
		t.Errorf("GET /: %d %v", resp.StatusCode, resp.Header)
	}
	resp, _ = do(t, "GET", r.base+"/projects/abc")
	if resp.StatusCode != 200 || !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/html") {
		t.Errorf("SPA fallback: %d", resp.StatusCode)
	}
	resp, body := do(t, "GET", r.base+"/healthz")
	if resp.StatusCode != 200 || !strings.Contains(body, `"ok"`) {
		t.Errorf("healthz: %d %s", resp.StatusCode, body)
	}
	if resp, _ = do(t, "GET", r.base+"/readyz"); resp.StatusCode != 200 {
		t.Errorf("readyz: %d", resp.StatusCode)
	}
	if resp, _ = do(t, "GET", r.base+"/assets/missing.js"); resp.StatusCode != 404 {
		t.Errorf("missing asset: %d", resp.StatusCode)
	}
	resp, body = do(t, "GET", r.base+"/api/nope")
	if resp.StatusCode != 404 || !strings.Contains(body, "NOT_FOUND") || !strings.Contains(body, "trace_id") {
		t.Errorf("api 404: %d %s", resp.StatusCode, body)
	}
	if resp, _ = do(t, "DELETE", r.base+"/healthz"); resp.StatusCode != 405 {
		t.Errorf("DELETE healthz: %d", resp.StatusCode)
	}
	resp, body = do(t, "POST", r.base+"/api/x")
	if resp.StatusCode != 403 || !strings.Contains(body, "CSRF_REQUIRED") {
		t.Errorf("csrf: %d %s", resp.StatusCode, body)
	}

	r.cancel()
	if err := r.wait(t, 5*time.Second); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestServe_GracefulShutdownWaitsInflight(t *testing.T) {
	dbURL, _, _ := testDBURL(t)
	started := make(chan struct{})
	r := startRun(t, testCfg(t, dbURL, nil), WithAppHook(func(d app.Deps) {
		d.API.Get("/_test/slow", func(w http.ResponseWriter, _ *http.Request) {
			close(started)
			time.Sleep(500 * time.Millisecond)
			_, _ = w.Write([]byte("slow-ok"))
		})
	}))
	type res struct {
		code int
		body string
		at   time.Time
		err  error
	}
	ch := make(chan res, 1)
	go func() {
		resp, err := http.Get(r.base + "/api/_test/slow")
		if err != nil {
			ch <- res{err: err}
			return
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		ch <- res{code: resp.StatusCode, body: string(b), at: time.Now()}
	}()
	<-started
	r.cancel()
	err := r.wait(t, 5*time.Second)
	ranAt := time.Now()
	rs := <-ch
	if rs.err != nil || rs.code != 200 || rs.body != "slow-ok" {
		t.Fatalf("respons slow: %+v", rs)
	}
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if rs.at.After(ranAt) {
		t.Error("respons selesai setelah Run kembali")
	}
	if _, err := http.Get(r.base + "/healthz"); err == nil {
		t.Error("listener masih menerima koneksi")
	}
}

func TestServe_ShutdownTimeoutReturnsError(t *testing.T) {
	dbURL, _, _ := testDBURL(t)
	started := make(chan struct{})
	r := startRun(t, testCfg(t, dbURL, map[string]string{"SHUTDOWN_TIMEOUT": "100ms"}), WithAppHook(func(d app.Deps) {
		d.API.Get("/_test/slow", func(w http.ResponseWriter, _ *http.Request) {
			close(started)
			time.Sleep(2 * time.Second)
		})
	}))
	go func() { _, _ = http.Get(r.base + "/api/_test/slow") }()
	<-started
	begin := time.Now()
	r.cancel()
	err := r.wait(t, 5*time.Second)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v", err)
	}
	if time.Since(begin) > 1500*time.Millisecond {
		t.Errorf("shutdown terlalu lama: %v", time.Since(begin))
	}
}

func TestServe_BusPublishDrainedOnShutdown(t *testing.T) {
	dbURL, _, _ := testDBURL(t)
	var flag atomic.Bool
	r := startRun(t, testCfg(t, dbURL, nil), WithAppHook(func(d app.Deps) {
		d.Bus.Register("test.wiring.slow", func(ctx context.Context, _ any) (any, error) {
			time.Sleep(200 * time.Millisecond)
			flag.Store(true)
			return nil, nil
		})
		d.API.Get("/_test/pub", func(w http.ResponseWriter, req *http.Request) {
			_ = d.Bus.Publish(req.Context(), "test.wiring.slow", nil)
			w.WriteHeader(204)
		})
	}))
	if resp, _ := do(t, "GET", r.base+"/api/_test/pub"); resp.StatusCode != 204 {
		t.Fatalf("status %d", resp.StatusCode)
	}
	r.cancel()
	if err := r.wait(t, 5*time.Second); err != nil {
		t.Fatal(err)
	}
	if !flag.Load() {
		t.Error("handler Publish belum selesai saat Run kembali")
	}
}

func tableExists(t *testing.T, admin *sql.DB, schema, table string) bool {
	t.Helper()
	var n int
	err := admin.QueryRow(`SELECT count(*) FROM information_schema.tables WHERE table_schema=$1 AND table_name=$2`, schema, table).Scan(&n)
	if err != nil {
		t.Fatal(err)
	}
	return n > 0
}

func TestServe_RunMigrationsOnStart(t *testing.T) {
	for _, on := range []bool{true, false} {
		dbURL, schema, admin := testDBURL(t)
		extra := map[string]string{}
		if on {
			extra["RUN_MIGRATIONS_ON_START"] = "true"
		}
		r := startRun(t, testCfg(t, dbURL, extra))
		got := tableExists(t, admin, schema, "schema_migrations")
		r.cancel()
		if err := r.wait(t, 5*time.Second); err != nil {
			t.Fatal(err)
		}
		if got != on {
			t.Errorf("RUN_MIGRATIONS_ON_START=%v: schema_migrations ada = %v", on, got)
		}
	}
}

func TestServe_BindFailureReturnsError(t *testing.T) {
	dbURL, _, _ := testDBURL(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	cfg := testCfg(t, dbURL, map[string]string{"HTTP_ADDR": ln.Addr().String()})
	err = Run(context.Background(), cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err == nil || !strings.Contains(err.Error(), "listen") {
		t.Fatalf("err = %v", err)
	}
}

func TestServe_DBDownFailsFast(t *testing.T) {
	dbURL, _, _ := testDBURL(t)
	u, _ := url.Parse(dbURL)
	u.Host = "127.0.0.1:1"
	cfg := testCfg(t, u.String(), nil)
	begin := time.Now()
	err := Run(context.Background(), cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err == nil {
		t.Fatal("err = nil")
	}
	if time.Since(begin) > 7*time.Second {
		t.Errorf("terlalu lama: %v", time.Since(begin))
	}
	if strings.Contains(err.Error(), "pw") && strings.Contains(err.Error(), "://") {
		t.Errorf("error memuat URL: %v", err)
	}
}

func TestMigrateUp_PrintsApplied(t *testing.T) {
	dbURL, _, _ := testDBURL(t)
	cfg := testCfg(t, dbURL, nil)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	for i, want := range []string{"diterapkan: core/20261021090000000_create_audit_log.sql\nditerapkan: core/20261021100000000_create_auth_tables.sql\nditerapkan: project/20261022090000000_create_projects.sql\nditerapkan: project/20261022095716034_create_api_keys.sql\nditerapkan: project/20261022105258455_create_api_specs.sql\nditerapkan: project/20261022105305340_create_knowledge_files.sql\n", "tidak ada migrasi baru\n"} {
		var out bytes.Buffer
		if err := MigrateUp(context.Background(), cfg, log, &out); err != nil {
			t.Fatal(err)
		}
		if out.String() != want {
			t.Errorf("run %d: out = %q, want %q", i, out.String(), want)
		}
	}
}
