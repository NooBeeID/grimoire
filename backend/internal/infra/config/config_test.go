package config

import (
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"
)

const secret32 = "0123456789abcdef0123456789abcdef"

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func base(extra map[string]string) map[string]string {
	m := map[string]string{"DATABASE_URL": "postgres://u:p@h:5432/d", "SESSION_SECRET": secret32}
	for k, v := range extra {
		m[k] = v
	}
	return m
}

func TestLoad_Defaults(t *testing.T) {
	c, err := Load(env(base(nil)))
	if err != nil {
		t.Fatal(err)
	}
	want := Config{
		DatabaseURL: "postgres://u:p@h:5432/d", HTTPAddr: ":8080", AppEnv: "production", SessionSecret: secret32,
		CookieSecure: true, AuditRetentionDays: 90, MaxFileBytes: 1048576, MaxUploadBytes: 52428800,
		MaxUploadFiles: 500, MCPMaxResponseBytes: 102400, DBMaxOpenConns: 20, DBMaxIdleConns: 5,
		DBConnMaxLifetime: 30 * time.Minute, HTTPDefaultBodyBytes: 1048576,
		HTTPReadHeaderTimeout: 10 * time.Second, HTTPDefaultTimeout: 30 * time.Second,
		ShutdownTimeout: 15 * time.Second, BusShutdownTimeout: 5 * time.Second,
		LogLevel: "info", LogFormat: "json",
	}
	if c != want {
		t.Fatalf("got %+v\nwant %+v", c, want)
	}
}

func TestLoad_OverridesApplied(t *testing.T) {
	c, err := Load(env(base(map[string]string{
		"HTTP_ADDR": "127.0.0.1:9000", "APP_ENV": "development", "COOKIE_SECURE": "false",
		"AUDIT_RETENTION_DAYS": "7", "MAX_FILE_BYTES": "10", "MAX_UPLOAD_BYTES": "20", "MAX_UPLOAD_FILES": "3",
		"MCP_MAX_RESPONSE_BYTES": "2048", "RUN_MIGRATIONS_ON_START": "true", "DB_MAX_OPEN_CONNS": "10",
		"DB_MAX_IDLE_CONNS": "2", "DB_CONN_MAX_LIFETIME": "1h", "HTTP_DEFAULT_BODY_BYTES": "99",
		"HTTP_READ_HEADER_TIMEOUT": "1s", "HTTP_DEFAULT_TIMEOUT": "2s", "SHUTDOWN_TIMEOUT": "3s",
		"BUS_SHUTDOWN_TIMEOUT": "4s", "LOG_LEVEL": "debug", "LOG_FORMAT": "text", "DEV_VITE_URL": "http://localhost:5173",
	})))
	if err != nil {
		t.Fatal(err)
	}
	if c.HTTPAddr != "127.0.0.1:9000" || c.CookieSecure || c.AuditRetentionDays != 7 || !c.RunMigrationsOnStart ||
		c.DBConnMaxLifetime != time.Hour || c.BusShutdownTimeout != 4*time.Second || c.LogFormat != "text" ||
		c.DevViteURL != "http://localhost:5173" || c.MaxUploadBytes != 20 {
		t.Fatalf("overrides not applied: %+v", c)
	}
}

func problems(t *testing.T, err error) []string {
	t.Helper()
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected *ValidationError, got %v", err)
	}
	return ve.Problems
}

func TestLoad_MissingRequired(t *testing.T) {
	_, err := Load(env(nil))
	p := strings.Join(problems(t, err), "\n")
	if !strings.Contains(p, "DATABASE_URL: wajib diisi") || !strings.Contains(p, "SESSION_SECRET: wajib diisi") {
		t.Fatalf("got %s", p)
	}
}

func TestLoad_ReportsAllErrors(t *testing.T) {
	_, err := Load(env(map[string]string{"SESSION_SECRET": "short-12-chr", "LOG_LEVEL": "verbose"}))
	p := problems(t, err)
	if len(p) != 3 {
		t.Fatalf("got %v", p)
	}
	if !strings.HasPrefix(p[0], "DATABASE_URL") || !strings.HasPrefix(p[1], "SESSION_SECRET") || !strings.HasPrefix(p[2], "LOG_LEVEL") {
		t.Fatalf("unstable order: %v", p)
	}
}

func TestLoad_InvalidValues(t *testing.T) {
	cases := []struct {
		name, want string
		env        map[string]string
	}{
		{"secret31", "SESSION_SECRET", map[string]string{"SESSION_SECRET": secret32[:31]}},
		{"appenv", "APP_ENV", map[string]string{"APP_ENV": "staging"}},
		{"lifetime-nan", "DB_CONN_MAX_LIFETIME", map[string]string{"DB_CONN_MAX_LIFETIME": "abc"}},
		{"lifetime-zero", "DB_CONN_MAX_LIFETIME", map[string]string{"DB_CONN_MAX_LIFETIME": "0s"}},
		{"idle>open", "DB_MAX_IDLE_CONNS", map[string]string{"DB_MAX_IDLE_CONNS": "30"}},
		{"upload<file", "MAX_UPLOAD_BYTES", map[string]string{"MAX_UPLOAD_BYTES": "10"}},
		{"vite-prod", "DEV_VITE_URL", map[string]string{"DEV_VITE_URL": "http://localhost:5173"}},
		{"vite-ftp", "DEV_VITE_URL", map[string]string{"APP_ENV": "development", "DEV_VITE_URL": "ftp://x"}},
		{"addr", "HTTP_ADDR", map[string]string{"HTTP_ADDR": "nonsense"}},
		{"dburl", "DATABASE_URL", map[string]string{"DATABASE_URL": "mysql://x/y"}},
		{"cookie", "COOKIE_SECURE", map[string]string{"COOKIE_SECURE": "maybe"}},
		{"retention", "AUDIT_RETENTION_DAYS", map[string]string{"AUDIT_RETENTION_DAYS": "0"}},
		{"mcp", "MCP_MAX_RESPONSE_BYTES", map[string]string{"MCP_MAX_RESPONSE_BYTES": "100"}},
		{"logformat", "LOG_FORMAT", map[string]string{"LOG_FORMAT": "xml"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Load(env(base(tc.env)))
			if err == nil || !strings.Contains(err.Error(), tc.want+":") {
				t.Fatalf("want error for %s, got %v", tc.want, err)
			}
		})
	}
}

func TestLoad_DevViteAllowedInDevelopment(t *testing.T) {
	_, err := Load(env(base(map[string]string{"APP_ENV": "development", "DEV_VITE_URL": "http://localhost:5173"})))
	if err != nil {
		t.Fatal(err)
	}
}

func TestLoad_ErrorDoesNotLeakSecrets(t *testing.T) {
	_, err := Load(env(map[string]string{"DATABASE_URL": "mysql://u:topsecretpw@h/d", "SESSION_SECRET": "shortsecret"}))
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), "topsecretpw") || strings.Contains(err.Error(), "shortsecret") {
		t.Fatalf("leak: %v", err)
	}
}

func TestConfig_StringMasksSecrets(t *testing.T) {
	c, err := Load(env(base(map[string]string{"DATABASE_URL": "postgres://u:dbpass123@dbhost:5432/d"})))
	if err != nil {
		t.Fatal(err)
	}
	out := fmt.Sprintf("%v %+v %#v %s", c, c, c, c)
	var sb strings.Builder
	slog.New(slog.NewJSONHandler(&sb, nil)).Info("cfg", "config", c)
	out += sb.String()
	if strings.Contains(out, "dbpass123") || strings.Contains(out, secret32) {
		t.Fatalf("leak: %s", out)
	}
	if !strings.Contains(out, "***") || !strings.Contains(out, "dbhost") {
		t.Fatalf("expected mask and host: %s", out)
	}
}

func TestMaskURL(t *testing.T) {
	cases := map[string]string{
		"postgres://h/d":          "postgres://h/d",
		"postgres://u@h/d":        "postgres://u@h/d",
		"postgres://u:p@h:5432/d": "postgres://u:***@h:5432/d",
		"://bad":                  "***",
	}
	for in, want := range cases {
		if got := maskURL(in); got != want {
			t.Errorf("maskURL(%q)=%q want %q", in, got, want)
		}
	}
}
