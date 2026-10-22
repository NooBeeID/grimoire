//go:build integration

package main

import (
	"bytes"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

const secret = "0123456789abcdef0123456789abcdef-secret"

var binPath string

func buildBinary(t *testing.T) string {
	t.Helper()
	if binPath != "" {
		return binPath
	}
	_, file, _, _ := runtime.Caller(0)
	backend := filepath.Join(filepath.Dir(file), "..", "..")
	out := filepath.Join(os.TempDir(), "grimoire-smoke-"+hex8())
	pkg := "grimoire/cmd/internal/buildinfo"
	cmd := exec.Command("go", "build", "-ldflags", "-X "+pkg+".Version=test-ver -X "+pkg+".Commit=abc1234 -X "+pkg+".BuildDate=2026-10-20", "-o", out, "./cmd/cli")
	cmd.Dir = backend
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, b)
	}
	binPath = out
	return out
}

func TestMain(m *testing.M) {
	code := m.Run()
	if binPath != "" {
		_ = os.Remove(binPath)
	}
	os.Exit(code)
}

func hex8() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func runBin(t *testing.T, env []string, stdin string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	cmd := exec.Command(buildBinary(t), args...)
	cmd.Dir = t.TempDir()
	cmd.Env = env
	cmd.Stdin = strings.NewReader(stdin)
	var o, e bytes.Buffer
	cmd.Stdout, cmd.Stderr = &o, &e
	err := cmd.Run()
	code = 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatal(err)
	}
	return o.String(), e.String(), code
}

func TestBuiltBinary_VersionAndHelp(t *testing.T) {
	out, _, code := runBin(t, []string{}, "", "version")
	if code != 0 || out != "grimoire test-ver (commit abc1234, built 2026-10-20)\n" {
		t.Fatalf("code=%d out=%q", code, out)
	}
	out, _, code = runBin(t, []string{}, "", "help")
	if code != 0 || !strings.Contains(out, "Pemakaian:") {
		t.Fatalf("help: code=%d out=%q", code, out)
	}
}

func TestBuiltBinary_MissingEnvExit1(t *testing.T) {
	_, errOut, code := runBin(t, []string{}, "")
	if code != 1 || !strings.Contains(errOut, "DATABASE_URL") || !strings.Contains(errOut, "SESSION_SECRET") {
		t.Fatalf("code=%d stderr=%q", code, errOut)
	}
	_, errOut, code = runBin(t, []string{"SESSION_SECRET=abc"}, "")
	if code != 1 || strings.Contains(errOut, "SESSION_SECRET=abc") {
		t.Fatalf("code=%d stderr=%q", code, errOut)
	}
}

func TestBuiltBinary_CreateAdminInputValidation(t *testing.T) {
	_, errOut, code := runBin(t, []string{"CREATE_ADMIN_PASSWORD=rahasia123"}, "", "create-admin", "--email", "a@b.co", "--name", "Admin")
	if code != 1 || !strings.Contains(errOut, "DATABASE_URL") || strings.Contains(errOut, "rahasia123") {
		t.Fatalf("code=%d stderr=%q", code, errOut)
	}
	_, errOut, code = runBin(t, []string{}, "", "create-admin", "--email", "a@b.co", "--name", "Admin")
	if code != 2 || !strings.Contains(errOut, "password tidak boleh kosong") {
		t.Fatalf("stdin kosong: code=%d stderr=%q", code, errOut)
	}
	_, errOut, code = runBin(t, []string{}, "", "create-admin")
	if code != 2 || !strings.Contains(errOut, "Pemakaian:") {
		t.Fatalf("tanpa flag: code=%d stderr=%q", code, errOut)
	}
	for _, a := range []string{"frobnicate", "migrate"} {
		if _, _, code := runBin(t, []string{}, "", a); code != 2 {
			t.Errorf("%s: code = %d", a, code)
		}
	}
}

func testSchemaURL(t *testing.T) string {
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
	// Database sendiri per test: tabel sisa di schema public (mis. hasil make seed)
	// tidak boleh terlihat lewat search_path.
	dbName := "t_" + hex8()
	if _, err := admin.Exec("CREATE DATABASE " + dbName); err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(base)
	u.Path = "/" + dbName
	t.Cleanup(func() {
		_, _ = admin.Exec("DROP DATABASE " + dbName + " WITH (FORCE)")
		_ = admin.Close()
	})
	own, err := sql.Open("pgx", u.String())
	if err != nil {
		t.Fatal(err)
	}
	defer own.Close()
	if _, err := own.Exec("CREATE EXTENSION IF NOT EXISTS citext"); err != nil {
		t.Fatal(err)
	}
	return u.String()
}

func TestT00_AC2_BuiltBinaryServesWithOnlyRequiredEnv(t *testing.T) {
	dbURL := testSchemaURL(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()

	cmd := exec.Command(buildBinary(t))
	cmd.Dir = t.TempDir()
	cmd.Env = []string{"DATABASE_URL=" + dbURL, "SESSION_SECRET=" + secret, "HTTP_ADDR=" + addr}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()
	t.Cleanup(func() { _ = cmd.Process.Kill() })

	base := "http://" + addr
	deadline := time.Now().Add(10 * time.Second)
	for {
		resp, err := http.Get(base + "/healthz")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == 200 {
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatal("healthz tidak siap")
		}
		time.Sleep(100 * time.Millisecond)
	}
	resp, err := http.Get(base + "/")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 || !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/html") {
		t.Errorf("GET /: %d %s", resp.StatusCode, resp.Header.Get("Content-Type"))
	}

	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-exited:
		if err != nil {
			t.Errorf("exit tidak 0: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("proses tidak berhenti setelah SIGTERM")
	}
}

func TestBuiltBinary_MigrateUpIdempotent(t *testing.T) {
	dbURL := testSchemaURL(t)
	env := []string{"DATABASE_URL=" + dbURL, "SESSION_SECRET=" + secret}
	for i, want := range []string{"diterapkan: core/20261021090000000_create_audit_log.sql\nditerapkan: core/20261021100000000_create_auth_tables.sql\nditerapkan: core/20261021233706157_create_user_invitations.sql\nditerapkan: project/20261022090000000_create_projects.sql\nditerapkan: project/20261022095716034_create_api_keys.sql\nditerapkan: project/20261022105258455_create_api_specs.sql\nditerapkan: project/20261022105305340_create_knowledge_files.sql\nditerapkan: project/20261022160000000_api_keys_owner_scope.sql\nditerapkan: core/20261022163418271_create_git_credentials.sql\nditerapkan: project/20261022163418846_create_git_sync_configs.sql\n", "tidak ada migrasi baru\n"} {
		out, errOut, code := runBin(t, env, "", "migrate", "up")
		if code != 0 || !strings.HasSuffix(out, want) { // log JSON ikut tercetak di stdout
			t.Fatalf("run %d: code=%d out=%q err=%q", i, code, out, errOut)
		}
	}
}
