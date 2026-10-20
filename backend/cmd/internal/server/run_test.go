package server

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestLoadConfig_PrintsAllProblemsAndFails(t *testing.T) {
	var stderr bytes.Buffer
	env := map[string]string{"SESSION_SECRET": "abc"}
	_, _, ok := LoadConfig(func(k string) string { return env[k] }, &stderr)
	if ok {
		t.Fatal("ok = true")
	}
	out := stderr.String()
	for _, want := range []string{"konfigurasi tidak valid:", "DATABASE_URL", "SESSION_SECRET"} {
		if !strings.Contains(out, want) {
			t.Errorf("stderr tidak memuat %q: %s", want, out)
		}
	}
}

func TestServeFromEnv_InvalidConfigReturns1(t *testing.T) {
	var stderr bytes.Buffer
	code := ServeFromEnv(context.Background(), func(string) string { return "" }, &stderr)
	if code != 1 {
		t.Fatalf("code = %d", code)
	}
	if !strings.Contains(stderr.String(), "konfigurasi tidak valid:") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestMigrateFromEnv_InvalidConfigReturns1(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := MigrateFromEnv(context.Background(), func(string) string { return "" }, &stdout, &stderr)
	if code != 1 || stdout.Len() != 0 {
		t.Fatalf("code=%d stdout=%q", code, stdout.String())
	}
}
