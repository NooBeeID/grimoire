package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"grimoire/cmd/internal/buildinfo"
)

type harness struct {
	c              *cli
	stdout, stderr *bytes.Buffer
	serveN, migN   int
	readSecretN    int
	secrets        []string
	secretErr      error
}

func newTestCLI(t *testing.T, env map[string]string, stdin string, tty bool) *harness {
	t.Helper()
	h := &harness{stdout: &bytes.Buffer{}, stderr: &bytes.Buffer{}}
	h.c = newCLI(strings.NewReader(stdin), h.stdout, h.stderr, func(k string) string { return env[k] })
	h.c.isTTY = func() bool { return tty }
	h.c.serve = func(context.Context) int { h.serveN++; return 0 }
	h.c.migrateUp = func(context.Context) int { h.migN++; return 0 }
	h.c.readSecret = func(string) (string, error) {
		i := h.readSecretN
		h.readSecretN++
		if h.secretErr != nil {
			return "", h.secretErr
		}
		return h.secrets[i], nil
	}
	return h
}

func (h *harness) run(args ...string) int { return h.c.run(context.Background(), args) }

func TestCLI_DefaultIsServe(t *testing.T) {
	h := newTestCLI(t, nil, "", false)
	if code := h.run(); code != 0 || h.serveN != 1 || h.migN != 0 {
		t.Fatalf("code=%d serve=%d mig=%d", code, h.serveN, h.migN)
	}
}

func TestCLI_ServeExplicit(t *testing.T) {
	h := newTestCLI(t, nil, "", false)
	if h.run("serve") != 0 || h.serveN != 1 {
		t.Fatal("serve tidak terpanggil")
	}
	if h.run("serve", "x") != 2 || h.serveN != 1 {
		t.Fatal("serve x harus 2")
	}
}

func TestCLI_MigrateUp(t *testing.T) {
	h := newTestCLI(t, nil, "", false)
	if h.run("migrate", "up") != 0 || h.migN != 1 {
		t.Fatal("migrate up")
	}
	for _, a := range [][]string{{"migrate"}, {"migrate", "down"}, {"migrate", "up", "x"}} {
		if h.run(a...) != 2 {
			t.Errorf("%v harus 2", a)
		}
	}
	if h.migN != 1 {
		t.Error("stub migrate terpanggil untuk argumen salah")
	}
}

func TestCLI_UnknownSubcommandExit2(t *testing.T) {
	for _, a := range []string{"frobnicate", "--foo"} {
		h := newTestCLI(t, nil, "", false)
		if h.run(a) != 2 {
			t.Errorf("%s harus 2", a)
		}
		if !strings.Contains(h.stderr.String(), "perintah tidak dikenal") || !strings.Contains(h.stderr.String(), "Pemakaian:") || h.stdout.Len() != 0 {
			t.Errorf("%s: stderr=%q stdout=%q", a, h.stderr, h.stdout)
		}
	}
}

func TestCLI_HelpExit0(t *testing.T) {
	for _, a := range []string{"help", "-h", "--help"} {
		h := newTestCLI(t, nil, "", false)
		if h.run(a) != 0 || !strings.Contains(h.stdout.String(), "Pemakaian:") {
			t.Errorf("%s: stdout=%q", a, h.stdout)
		}
	}
}

func setBuild(t *testing.T, v, c, d string) {
	ov, oc, od := buildinfo.Version, buildinfo.Commit, buildinfo.BuildDate
	t.Cleanup(func() { buildinfo.Version, buildinfo.Commit, buildinfo.BuildDate = ov, oc, od })
	buildinfo.Version, buildinfo.Commit, buildinfo.BuildDate = v, c, d
}

func TestCLI_Version(t *testing.T) {
	setBuild(t, "v1.2.3", "abc1234", "2026-10-20T00:00:00Z")
	h := newTestCLI(t, nil, "", false)
	h.c.getenv = func(string) string { panic("getenv tidak boleh dipanggil") }
	if h.run("version") != 0 {
		t.Fatal("exit != 0")
	}
	if got, want := h.stdout.String(), "grimoire v1.2.3 (commit abc1234, built 2026-10-20T00:00:00Z)\n"; got != want {
		t.Fatalf("got %q", got)
	}
}

func TestCLI_VersionDefaults(t *testing.T) {
	setBuild(t, "dev", "none", "unknown")
	h := newTestCLI(t, nil, "", false)
	h.run("version")
	if got := h.stdout.String(); got != "grimoire dev (commit none, built unknown)\n" {
		t.Fatalf("got %q", got)
	}
}

func TestCreateAdmin_RequiresFlags(t *testing.T) {
	cases := map[string][]string{
		"none":         {"create-admin"},
		"only email":   {"create-admin", "--email", "a@b.co"},
		"only name":    {"create-admin", "--name", "Admin"},
		"blank name":   {"create-admin", "--email", "a@b.co", "--name", "   "},
		"no at":        {"create-admin", "--email", "abc", "--name", "X"},
		"display name": {"create-admin", "--email", "Nama <a@b.co>", "--name", "X"},
		"extra arg":    {"create-admin", "--email", "a@b.co", "--name", "X", "extra"},
	}
	for name, args := range cases {
		h := newTestCLI(t, nil, "pw-tidak-dibaca\n", false)
		if code := h.run(args...); code != 2 {
			t.Errorf("%s: code = %d", name, code)
		}
		if !strings.Contains(h.stderr.String(), "Pemakaian:") {
			t.Errorf("%s: stderr tanpa usage: %q", name, h.stderr)
		}
		if h.c.stdin.(*strings.Reader).Len() != len("pw-tidak-dibaca\n") {
			t.Errorf("%s: stdin dikonsumsi", name)
		}
	}
	h := newTestCLI(t, nil, "", false)
	h.run("create-admin")
	if !strings.Contains(h.stderr.String(), "--email wajib dan harus alamat email valid") {
		t.Errorf("pesan: %q", h.stderr)
	}
	h = newTestCLI(t, nil, "", false)
	h.run("create-admin", "--email", "a@b.co")
	if !strings.Contains(h.stderr.String(), "--name wajib diisi") {
		t.Errorf("pesan: %q", h.stderr)
	}
}

func TestCreateAdmin_UnknownFlag(t *testing.T) {
	h := newTestCLI(t, nil, "", false)
	if h.run("create-admin", "--bogus") != 2 {
		t.Fatal("harus 2")
	}
}

func TestCreateAdmin_NotAvailableUntilT01(t *testing.T) {
	h := newTestCLI(t, map[string]string{"CREATE_ADMIN_PASSWORD": "rahasia123"}, "", false)
	if code := h.run("create-admin", "--email", "a@b.co", "--name", "Admin"); code != 1 {
		t.Fatalf("code = %d", code)
	}
	if !strings.Contains(h.stderr.String(), "create-admin belum tersedia (butuh T01)") {
		t.Errorf("stderr = %q", h.stderr)
	}
	if strings.Contains(h.stdout.String()+h.stderr.String(), "rahasia123") {
		t.Error("password bocor")
	}
	if h.serveN+h.migN != 0 {
		t.Error("stub terpanggil")
	}
}
