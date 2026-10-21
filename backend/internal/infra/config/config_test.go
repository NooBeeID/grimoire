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
		MaxUploadFiles: 500, MaxProjectFiles: 5000, MCPMaxResponseBytes: 102400, DBMaxOpenConns: 20, DBMaxIdleConns: 5,
		DBConnMaxLifetime: 30 * time.Minute, HTTPDefaultBodyBytes: 1048576,
		HTTPReadHeaderTimeout: 10 * time.Second, HTTPDefaultTimeout: 30 * time.Second,
		ShutdownTimeout: 15 * time.Second, BusShutdownTimeout: 5 * time.Second,
		LogLevel: "info", LogFormat: "json",
		SessionIdleTimeout: 12 * time.Hour, SessionAbsoluteTimeout: 168 * time.Hour, PasswordMinLength: 10,
		Argon2MemoryKiB: 65536, Argon2Iterations: 3, Argon2Parallelism: 2, LoginMaxFailures: 5,
		LoginFailureWindow: 15 * time.Minute, LoginLockDuration: 15 * time.Minute, LoginRatePerMinute: 20, APIKeyMaxActivePerProject: 20,
		APISpecMaxFileBytes: 5242880, APISpecMaxSpecs: 100,
	}
	if c != want {
		t.Fatalf("got %+v\nwant %+v", c, want)
	}
}

func TestLoad_OverridesApplied(t *testing.T) {
	c, err := Load(env(base(map[string]string{
		"HTTP_ADDR": "127.0.0.1:9000", "APP_ENV": "development", "COOKIE_SECURE": "false",
		"AUDIT_RETENTION_DAYS": "7", "MAX_FILE_BYTES": "10", "MAX_UPLOAD_BYTES": "20", "APISPEC_MAX_FILE_BYTES": "20", "MAX_UPLOAD_FILES": "3",
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

func TestLoad_MaxProjectFiles(t *testing.T) {
	c, err := Load(env(base(map[string]string{"MAX_PROJECT_FILES": "3"})))
	if err != nil || c.MaxProjectFiles != 3 {
		t.Fatalf("got %d, %v", c.MaxProjectFiles, err)
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
		{"projectfiles-zero", "MAX_PROJECT_FILES", map[string]string{"MAX_PROJECT_FILES": "0"}},
		{"projectfiles-nan", "MAX_PROJECT_FILES", map[string]string{"MAX_PROJECT_FILES": "abc"}},
		{"logformat", "LOG_FORMAT", map[string]string{"LOG_FORMAT": "xml"}},
		{"apikey-max-zero", "API_KEY_MAX_ACTIVE_PER_PROJECT", map[string]string{"API_KEY_MAX_ACTIVE_PER_PROJECT": "0"}},
		{"apikey-max-nan", "API_KEY_MAX_ACTIVE_PER_PROJECT", map[string]string{"API_KEY_MAX_ACTIVE_PER_PROJECT": "abc"}},
		{"apispec-file-zero", "APISPEC_MAX_FILE_BYTES", map[string]string{"APISPEC_MAX_FILE_BYTES": "0"}},
		{"apispec-file-nan", "APISPEC_MAX_FILE_BYTES", map[string]string{"APISPEC_MAX_FILE_BYTES": "abc"}},
		{"apispec-file>upload", "APISPEC_MAX_FILE_BYTES", map[string]string{"APISPEC_MAX_FILE_BYTES": "52428801"}},
		{"apispec-specs-zero", "APISPEC_MAX_SPECS", map[string]string{"APISPEC_MAX_SPECS": "0"}},
		{"public-ftp", "PUBLIC_URL", map[string]string{"PUBLIC_URL": "ftp://x"}},
		{"public-nohost", "PUBLIC_URL", map[string]string{"PUBLIC_URL": "g.tim.id"}},
		{"public-query", "PUBLIC_URL", map[string]string{"PUBLIC_URL": "https://g.tim.id?a=1"}},
		{"public-fragment", "PUBLIC_URL", map[string]string{"PUBLIC_URL": "https://g.tim.id#x"}},
		{"public-userinfo", "PUBLIC_URL", map[string]string{"PUBLIC_URL": "https://u:p@g.tim.id"}},
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

func TestLoad_AuthDefaults(t *testing.T) {
	c, err := Load(env(base(nil)))
	if err != nil {
		t.Fatal(err)
	}
	if c.SessionIdleTimeout != 12*time.Hour || c.SessionAbsoluteTimeout != 168*time.Hour || c.PasswordMinLength != 10 ||
		c.Argon2MemoryKiB != 65536 || c.Argon2Iterations != 3 || c.Argon2Parallelism != 2 ||
		c.LoginMaxFailures != 5 || c.LoginFailureWindow != 15*time.Minute || c.LoginLockDuration != 15*time.Minute ||
		c.LoginRatePerMinute != 20 {
		t.Fatalf("auth defaults salah: %+v", c)
	}
}

func TestLoad_AuthOverrides(t *testing.T) {
	c, err := Load(env(base(map[string]string{
		"SESSION_IDLE_TIMEOUT": "1h", "SESSION_ABSOLUTE_TIMEOUT": "2h", "PASSWORD_MIN_LENGTH": "12",
		"ARGON2_MEMORY_KIB": "8192", "ARGON2_ITERATIONS": "1", "ARGON2_PARALLELISM": "1",
		"LOGIN_MAX_FAILURES": "3", "LOGIN_FAILURE_WINDOW": "5m", "LOGIN_LOCK_DURATION": "10m", "LOGIN_RATE_PER_MINUTE": "50",
	})))
	if err != nil {
		t.Fatal(err)
	}
	if c.SessionIdleTimeout != time.Hour || c.PasswordMinLength != 12 || c.Argon2MemoryKiB != 8192 ||
		c.LoginMaxFailures != 3 || c.LoginLockDuration != 10*time.Minute || c.LoginRatePerMinute != 50 {
		t.Fatalf("override salah: %+v", c)
	}
}

func TestLoad_AuthInvalid(t *testing.T) {
	_, err := Load(env(base(map[string]string{
		"SESSION_IDLE_TIMEOUT": "10h", "SESSION_ABSOLUTE_TIMEOUT": "1h", "PASSWORD_MIN_LENGTH": "7",
		"ARGON2_PARALLELISM": "0", "ARGON2_MEMORY_KIB": "100", "ARGON2_ITERATIONS": "21",
		"LOGIN_MAX_FAILURES": "101", "LOGIN_RATE_PER_MINUTE": "0", "LOGIN_LOCK_DURATION": "0s",
	})))
	p := strings.Join(problems(t, err), "\n")
	for _, want := range []string{
		"SESSION_IDLE_TIMEOUT: tidak boleh melebihi", "PASSWORD_MIN_LENGTH: minimal 8", "ARGON2_PARALLELISM: minimal 1",
		"ARGON2_MEMORY_KIB: minimal 8192", "ARGON2_ITERATIONS: maksimal 20", "LOGIN_MAX_FAILURES: maksimal 100",
		"LOGIN_RATE_PER_MINUTE: minimal 1", "LOGIN_LOCK_DURATION: harus lebih dari 0",
	} {
		if !strings.Contains(p, want) {
			t.Errorf("tidak memuat %q dalam:\n%s", want, p)
		}
	}
}

func TestLoad_APIKeyConfig(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"empty", "", ""},
		{"plain", "https://g.tim.id", "https://g.tim.id"},
		{"trailing-slash", "https://g.tim.id/", "https://g.tim.id"},
		{"path", "https://tim.id/grimoire/", "https://tim.id/grimoire"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, err := Load(env(base(map[string]string{"PUBLIC_URL": tc.in, "API_KEY_MAX_ACTIVE_PER_PROJECT": "3"})))
			if err != nil {
				t.Fatal(err)
			}
			if c.PublicURL != tc.want || c.APIKeyMaxActivePerProject != 3 {
				t.Fatalf("got %q / %d", c.PublicURL, c.APIKeyMaxActivePerProject)
			}
		})
	}
}

func TestLoad_APISpecConfig(t *testing.T) {
	c, err := Load(env(base(map[string]string{"APISPEC_MAX_FILE_BYTES": "1024", "APISPEC_MAX_SPECS": "7"})))
	if err != nil {
		t.Fatal(err)
	}
	if c.APISpecMaxFileBytes != 1024 || c.APISpecMaxSpecs != 7 {
		t.Fatalf("got %d / %d", c.APISpecMaxFileBytes, c.APISpecMaxSpecs)
	}
	// Batas atas = MAX_UPLOAD_BYTES (inklusif).
	if _, err := Load(env(base(map[string]string{"APISPEC_MAX_FILE_BYTES": "52428800"}))); err != nil {
		t.Fatalf("== MAX_UPLOAD_BYTES harus lolos: %v", err)
	}
}
