package main

import (
	"errors"
	"strings"
	"testing"
)

func TestPassword_FromEnvWins(t *testing.T) {
	h := newTestCLI(t, map[string]string{"CREATE_ADMIN_PASSWORD": "dari-env\n"}, "dari-stdin\n", false)
	pw, err := h.c.resolvePassword()
	if err != nil || pw != "dari-env" {
		t.Fatalf("pw=%q err=%v", pw, err)
	}
	if h.c.stdin.(*strings.Reader).Len() != len("dari-stdin\n") {
		t.Error("stdin dibaca")
	}
}

func TestPassword_FromStdinNonTTY(t *testing.T) {
	for _, in := range []string{"pw-dari-stdin\n", "pw-dari-stdin\r\n", "pw-dari-stdin"} {
		h := newTestCLI(t, nil, in, false)
		pw, err := h.c.resolvePassword()
		if err != nil || pw != "pw-dari-stdin" {
			t.Errorf("%q: pw=%q err=%v", in, pw, err)
		}
	}
}

func TestPassword_StdinOnlyFirstLine(t *testing.T) {
	h := newTestCLI(t, nil, "a\nb\n", false)
	if pw, _ := h.c.resolvePassword(); pw != "a" {
		t.Fatalf("pw = %q", pw)
	}
}

func TestPassword_EmptyRejected(t *testing.T) {
	for _, in := range []string{"", "\n"} {
		h := newTestCLI(t, map[string]string{"CREATE_ADMIN_PASSWORD": ""}, in, false)
		if _, err := h.c.resolvePassword(); err == nil || err.Error() != "password tidak boleh kosong" {
			t.Errorf("%q: err = %v", in, err)
		}
	}
}

func TestPassword_TTYPromptsTwice(t *testing.T) {
	h := newTestCLI(t, nil, "tidak-dibaca", true)
	var prompts []string
	h.c.readSecret = func(p string) (string, error) { prompts = append(prompts, p); return "x", nil }
	pw, err := h.c.resolvePassword()
	if err != nil || pw != "x" || len(prompts) != 2 || prompts[0] == prompts[1] {
		t.Fatalf("pw=%q err=%v prompts=%v", pw, err, prompts)
	}
	if h.c.stdin.(*strings.Reader).Len() != len("tidak-dibaca") {
		t.Error("stdin dibaca")
	}
}

func TestPassword_TTYMismatch(t *testing.T) {
	h := newTestCLI(t, nil, "", true)
	h.secrets = []string{"x", "y"}
	if code := h.run("create-admin", "--email", "a@b.co", "--name", "A"); code != 2 {
		t.Fatalf("code = %d", code)
	}
	if !strings.Contains(h.stderr.String(), "password tidak sama") {
		t.Errorf("stderr = %q", h.stderr)
	}
}

func TestPassword_TTYReadError(t *testing.T) {
	h := newTestCLI(t, nil, "", true)
	h.secretErr = errors.New("boom")
	if code := h.run("create-admin", "--email", "a@b.co", "--name", "A"); code != 2 {
		t.Fatalf("code = %d", code)
	}
}

func TestPassword_NeverEchoed(t *testing.T) {
	const marker = "S3CR3T-MARKER"
	args := []string{"create-admin", "--email", "a@b.co", "--name", "A"}
	cases := []*harness{
		newTestCLI(t, map[string]string{"CREATE_ADMIN_PASSWORD": marker}, "", false),
		newTestCLI(t, nil, marker+"\n", false),
		newTestCLI(t, nil, "", true),
	}
	cases[2].secrets = []string{marker, marker}
	mismatch := newTestCLI(t, nil, "", true)
	mismatch.secrets = []string{marker, marker + "x"}
	cases = append(cases, mismatch)
	for i, h := range cases {
		h.run(args...)
		if strings.Contains(h.stdout.String()+h.stderr.String(), marker) {
			t.Errorf("kasus %d: password tercetak", i)
		}
	}
}
