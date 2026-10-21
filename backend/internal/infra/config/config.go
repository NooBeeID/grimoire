// Package config memuat dan memvalidasi konfigurasi dari environment.
package config

import (
	"fmt"
	"log/slog"
	"net"
	"net/mail"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config adalah seluruh konfigurasi proses. Setiap field wajib ber-tag env.
type Config struct {
	DatabaseURL           string        `env:"DATABASE_URL"`
	HTTPAddr              string        `env:"HTTP_ADDR"`
	AppEnv                string        `env:"APP_ENV"`
	SessionSecret         string        `env:"SESSION_SECRET"`
	CookieSecure          bool          `env:"COOKIE_SECURE"`
	AuditRetentionDays    int           `env:"AUDIT_RETENTION_DAYS"`
	MaxFileBytes          int64         `env:"MAX_FILE_BYTES"`
	MaxUploadBytes        int64         `env:"MAX_UPLOAD_BYTES"`
	MaxUploadFiles        int           `env:"MAX_UPLOAD_FILES"`
	MaxProjectFiles       int           `env:"MAX_PROJECT_FILES"`
	MCPMaxResponseBytes   int           `env:"MCP_MAX_RESPONSE_BYTES"`
	RunMigration          bool          `env:"RUN_MIGRATION"`
	DBMaxOpenConns        int           `env:"DB_MAX_OPEN_CONNS"`
	DBMaxIdleConns        int           `env:"DB_MAX_IDLE_CONNS"`
	DBConnMaxLifetime     time.Duration `env:"DB_CONN_MAX_LIFETIME"`
	HTTPDefaultBodyBytes  int64         `env:"HTTP_DEFAULT_BODY_BYTES"`
	HTTPReadHeaderTimeout time.Duration `env:"HTTP_READ_HEADER_TIMEOUT"`
	HTTPDefaultTimeout    time.Duration `env:"HTTP_DEFAULT_TIMEOUT"`
	ShutdownTimeout       time.Duration `env:"SHUTDOWN_TIMEOUT"`
	BusShutdownTimeout    time.Duration `env:"BUS_SHUTDOWN_TIMEOUT"`
	LogLevel              string        `env:"LOG_LEVEL"`
	LogFormat             string        `env:"LOG_FORMAT"`
	DevViteURL            string        `env:"DEV_VITE_URL"`

	SessionIdleTimeout     time.Duration `env:"SESSION_IDLE_TIMEOUT"`
	SessionAbsoluteTimeout time.Duration `env:"SESSION_ABSOLUTE_TIMEOUT"`
	PasswordMinLength      int           `env:"PASSWORD_MIN_LENGTH"`
	Argon2MemoryKiB        uint32        `env:"ARGON2_MEMORY_KIB"`
	Argon2Iterations       uint32        `env:"ARGON2_ITERATIONS"`
	Argon2Parallelism      uint8         `env:"ARGON2_PARALLELISM"`
	LoginMaxFailures       int           `env:"LOGIN_MAX_FAILURES"`
	LoginFailureWindow     time.Duration `env:"LOGIN_FAILURE_WINDOW"`
	LoginLockDuration      time.Duration `env:"LOGIN_LOCK_DURATION"`
	LoginRatePerMinute     int           `env:"LOGIN_RATE_PER_MINUTE"`

	APIKeyMaxActivePerProject int    `env:"API_KEY_MAX_ACTIVE_PER_PROJECT"`
	PublicURL                 string `env:"PUBLIC_URL"`

	APISpecMaxFileBytes int64 `env:"APISPEC_MAX_FILE_BYTES"`
	APISpecMaxSpecs     int   `env:"APISPEC_MAX_SPECS"`
	// MCP gateway (T07).
	MCPRatePerMinute     int           `env:"MCP_RATE_PER_MINUTE"`
	MCPAuthFailPerMinute int           `env:"MCP_AUTH_FAIL_PER_MINUTE"`
	MCPMaxRequestBytes   int64         `env:"MCP_MAX_REQUEST_BYTES"`
	MCPToolTimeout       time.Duration `env:"MCP_TOOL_TIMEOUT"`
	MCPAllowedOrigins    []string      `env:"MCP_ALLOWED_ORIGINS"`

	// Mailer dan undangan email (T12).
	MailDriver                 string        `env:"MAIL_DRIVER"`
	SMTPHost                   string        `env:"SMTP_HOST"`
	SMTPPort                   int           `env:"SMTP_PORT"`
	SMTPUsername               string        `env:"SMTP_USERNAME"`
	SMTPPassword               string        `env:"SMTP_PASSWORD"`
	SMTPTLS                    string        `env:"SMTP_TLS"`
	MailFrom                   string        `env:"MAIL_FROM"`
	MailTimeout                time.Duration `env:"MAIL_TIMEOUT"`
	InvitationTTL              time.Duration `env:"INVITATION_TTL"`
	InvitationMaxPerHour       int           `env:"INVITATION_MAX_PER_HOUR"`
	InvitationResendMaxPerHour int           `env:"INVITATION_RESEND_MAX_PER_HOUR"`
	InvitationPurgeAfter       time.Duration `env:"INVITATION_PURGE_AFTER"`
}

// ValidationError mengumpulkan semua masalah konfigurasi sekaligus.
type ValidationError struct {
	Problems []string
}

func (e *ValidationError) Error() string {
	var b strings.Builder
	b.WriteString("konfigurasi tidak valid:")
	for _, p := range e.Problems {
		b.WriteString("\n- ")
		b.WriteString(p)
	}
	return b.String()
}

// LoadFromEnv memuat konfigurasi dari environment proses.
func LoadFromEnv() (Config, error) { return Load(os.Getenv) }

type loader struct {
	get      func(string) string
	problems []string
}

func (l *loader) fail(env, format string, a ...any) {
	l.problems = append(l.problems, env+": "+fmt.Sprintf(format, a...))
}

// raw mengembalikan nilai ter-trim; ok=false bila kosong.
func (l *loader) raw(env string) (string, bool) {
	v := strings.TrimSpace(l.get(env))
	return v, v != ""
}

func (l *loader) str(env, def string) string {
	if v, ok := l.raw(env); ok {
		return v
	}
	return def
}

func (l *loader) enum(env, def string, allowed ...string) (string, bool) {
	v := l.str(env, def)
	for _, a := range allowed {
		if v == a {
			return v, true
		}
	}
	l.fail(env, "nilai %q tidak dikenal (%s)", v, strings.Join(allowed, "|"))
	return v, false
}

func (l *loader) boolean(env string, def bool) bool {
	v, ok := l.raw(env)
	if !ok {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		l.fail(env, "nilai %q bukan boolean", v)
		return def
	}
	return b
}

func (l *loader) int64(env string, def, min int64) (int64, bool) {
	v, ok := l.raw(env)
	if !ok {
		return def, true
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		l.fail(env, "nilai %q bukan bilangan bulat", v)
		return def, false
	}
	if n < min {
		l.fail(env, "minimal %d (sekarang %d)", min, n)
		return def, false
	}
	return n, true
}

func (l *loader) intv(env string, def, min int) (int, bool) {
	n, ok := l.int64(env, int64(def), int64(min))
	return int(n), ok
}

// intr membaca int dengan batas bawah dan atas (inklusif).
func (l *loader) intr(env string, def, min, max int) (int, bool) {
	n, ok := l.intv(env, def, min)
	if ok && n > max {
		l.fail(env, "maksimal %d (sekarang %d)", max, n)
		return def, false
	}
	return n, ok
}

func (l *loader) dur(env string, def time.Duration) time.Duration {
	v, ok := l.raw(env)
	if !ok {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		l.fail(env, "nilai %q bukan durasi valid", v)
		return def
	}
	if d <= 0 {
		l.fail(env, "harus lebih dari 0 (sekarang %s)", v)
		return def
	}
	return d
}

// Load memuat konfigurasi dari getenv, mengumpulkan semua error sekaligus.
func Load(getenv func(string) string) (Config, error) {
	l := &loader{get: getenv}
	var c Config

	// DATABASE_URL
	if v, ok := l.raw("DATABASE_URL"); !ok {
		l.fail("DATABASE_URL", "wajib diisi")
	} else {
		u, err := url.Parse(v)
		switch {
		case err != nil:
			l.fail("DATABASE_URL", "bukan URL valid") // nilai tidak disertakan (bisa berisi password)
		case u.Scheme != "postgres" && u.Scheme != "postgresql":
			l.fail("DATABASE_URL", "scheme harus postgres atau postgresql")
		case u.Host == "":
			l.fail("DATABASE_URL", "host tidak boleh kosong")
		default:
			c.DatabaseURL = v
		}
	}

	// HTTP_ADDR
	c.HTTPAddr = l.str("HTTP_ADDR", ":8080")
	if _, port, err := net.SplitHostPort(c.HTTPAddr); err != nil {
		l.fail("HTTP_ADDR", "format harus host:port (nilai %q)", c.HTTPAddr)
	} else if p, err := strconv.Atoi(port); err != nil || p < 0 || p > 65535 {
		l.fail("HTTP_ADDR", "port tidak valid (nilai %q)", c.HTTPAddr)
	}

	appEnv, _ := l.enum("APP_ENV", "production", "production", "development")
	c.AppEnv = appEnv

	// SESSION_SECRET (nilai tidak pernah disertakan di pesan)
	if v, ok := l.raw("SESSION_SECRET"); !ok {
		l.fail("SESSION_SECRET", "wajib diisi")
	} else if len(v) < 32 {
		l.fail("SESSION_SECRET", "minimal 32 byte (sekarang %d)", len(v))
	} else {
		c.SessionSecret = v
	}

	c.CookieSecure = l.boolean("COOKIE_SECURE", true)
	c.AuditRetentionDays, _ = l.intv("AUDIT_RETENTION_DAYS", 90, 1)

	var fileOK, uploadOK bool
	c.MaxFileBytes, fileOK = l.int64("MAX_FILE_BYTES", 1048576, 1)
	c.MaxUploadBytes, uploadOK = l.int64("MAX_UPLOAD_BYTES", 52428800, 1)
	if fileOK && uploadOK && c.MaxUploadBytes < c.MaxFileBytes {
		l.fail("MAX_UPLOAD_BYTES", "harus >= MAX_FILE_BYTES (%d < %d)", c.MaxUploadBytes, c.MaxFileBytes)
	}
	c.MaxUploadFiles, _ = l.intv("MAX_UPLOAD_FILES", 500, 1)
	c.MaxProjectFiles, _ = l.intv("MAX_PROJECT_FILES", 5000, 1)
	c.MCPMaxResponseBytes, _ = l.intv("MCP_MAX_RESPONSE_BYTES", 102400, 1024)
	c.RunMigration = l.boolean("RUN_MIGRATION", false)

	var openOK, idleOK bool
	c.DBMaxOpenConns, openOK = l.intv("DB_MAX_OPEN_CONNS", 20, 1)
	c.DBMaxIdleConns, idleOK = l.intv("DB_MAX_IDLE_CONNS", 5, 0)
	if openOK && idleOK && c.DBMaxIdleConns > c.DBMaxOpenConns {
		l.fail("DB_MAX_IDLE_CONNS", "tidak boleh melebihi DB_MAX_OPEN_CONNS (%d > %d)", c.DBMaxIdleConns, c.DBMaxOpenConns)
	}
	c.DBConnMaxLifetime = l.dur("DB_CONN_MAX_LIFETIME", 30*time.Minute)
	c.HTTPDefaultBodyBytes, _ = l.int64("HTTP_DEFAULT_BODY_BYTES", 1048576, 1)
	c.HTTPReadHeaderTimeout = l.dur("HTTP_READ_HEADER_TIMEOUT", 10*time.Second)
	c.HTTPDefaultTimeout = l.dur("HTTP_DEFAULT_TIMEOUT", 30*time.Second)
	c.ShutdownTimeout = l.dur("SHUTDOWN_TIMEOUT", 15*time.Second)
	c.BusShutdownTimeout = l.dur("BUS_SHUTDOWN_TIMEOUT", 5*time.Second)
	c.LogLevel, _ = l.enum("LOG_LEVEL", "info", "debug", "info", "warn", "error")
	c.LogFormat, _ = l.enum("LOG_FORMAT", "json", "json", "text")

	// Auth (T01).
	c.SessionIdleTimeout = l.dur("SESSION_IDLE_TIMEOUT", 12*time.Hour)
	c.SessionAbsoluteTimeout = l.dur("SESSION_ABSOLUTE_TIMEOUT", 168*time.Hour)
	if c.SessionIdleTimeout > c.SessionAbsoluteTimeout {
		l.fail("SESSION_IDLE_TIMEOUT", "tidak boleh melebihi SESSION_ABSOLUTE_TIMEOUT (%s > %s)", c.SessionIdleTimeout, c.SessionAbsoluteTimeout)
	}
	c.PasswordMinLength, _ = l.intr("PASSWORD_MIN_LENGTH", 10, 8, 128)
	mem, _ := l.intr("ARGON2_MEMORY_KIB", 65536, 8192, 4194304)
	c.Argon2MemoryKiB = uint32(mem)
	it, _ := l.intr("ARGON2_ITERATIONS", 3, 1, 20)
	c.Argon2Iterations = uint32(it)
	par, _ := l.intr("ARGON2_PARALLELISM", 2, 1, 16)
	c.Argon2Parallelism = uint8(par)
	c.LoginMaxFailures, _ = l.intr("LOGIN_MAX_FAILURES", 5, 1, 100)
	c.LoginFailureWindow = l.dur("LOGIN_FAILURE_WINDOW", 15*time.Minute)
	c.LoginLockDuration = l.dur("LOGIN_LOCK_DURATION", 15*time.Minute)
	c.LoginRatePerMinute, _ = l.intr("LOGIN_RATE_PER_MINUTE", 20, 1, 1000)

	// API key (T04).
	c.APIKeyMaxActivePerProject, _ = l.intv("API_KEY_MAX_ACTIVE_PER_PROJECT", 20, 1)
	if v, ok := l.raw("PUBLIC_URL"); ok {
		u, err := url.Parse(v)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" ||
			u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.User != nil || strings.Contains(v, "#") {
			l.fail("PUBLIC_URL", "harus URL absolut http(s) dengan host, tanpa query/fragment/userinfo (nilai %q)", v)
		} else {
			c.PublicURL = strings.TrimRight(v, "/")
		}
	}

	// API spec (T06).
	specFile, specFileOK := l.int64("APISPEC_MAX_FILE_BYTES", 5242880, 1)
	c.APISpecMaxFileBytes = specFile
	if specFileOK && uploadOK && specFile > c.MaxUploadBytes {
		l.fail("APISPEC_MAX_FILE_BYTES", "harus <= MAX_UPLOAD_BYTES (%d > %d)", specFile, c.MaxUploadBytes)
	}
	c.APISpecMaxSpecs, _ = l.intv("APISPEC_MAX_SPECS", 100, 1)
	// MCP gateway (T07).
	c.MCPRatePerMinute, _ = l.intr("MCP_RATE_PER_MINUTE", 120, 1, 10000)
	c.MCPAuthFailPerMinute, _ = l.intr("MCP_AUTH_FAIL_PER_MINUTE", 20, 1, 1000)
	c.MCPMaxRequestBytes, _ = l.int64("MCP_MAX_REQUEST_BYTES", 65536, 1024)
	c.MCPToolTimeout = l.dur("MCP_TOOL_TIMEOUT", 10*time.Second)
	if c.MCPToolTimeout > 60*time.Second {
		l.fail("MCP_TOOL_TIMEOUT", "maksimal 60s (sekarang %s)", c.MCPToolTimeout)
	}
	if v, ok := l.raw("MCP_ALLOWED_ORIGINS"); ok {
		for _, e := range strings.Split(v, ",") {
			e = strings.TrimSpace(e)
			if e == "" {
				continue
			}
			u, err := url.Parse(e)
			if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" ||
				u.Path != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.User != nil || strings.Contains(e, "#") {
				l.fail("MCP_ALLOWED_ORIGINS", "entri %q harus berbentuk scheme://host[:port]", e)
				continue
			}
			c.MCPAllowedOrigins = append(c.MCPAllowedOrigins, e)
		}
	}

	// Mailer dan undangan (T12). SMTP_PASSWORD tidak pernah disebut di pesan error.
	c.MailDriver, _ = l.enum("MAIL_DRIVER", "", "", "smtp", "log")
	c.SMTPHost = l.str("SMTP_HOST", "")
	c.SMTPPort, _ = l.intr("SMTP_PORT", 587, 1, 65535)
	c.SMTPUsername = l.str("SMTP_USERNAME", "")
	c.SMTPPassword = l.str("SMTP_PASSWORD", "")
	c.SMTPTLS, _ = l.enum("SMTP_TLS", "starttls", "starttls", "tls", "none")
	c.MailFrom = l.str("MAIL_FROM", "")
	if c.MailFrom != "" {
		if _, err := mail.ParseAddress(c.MailFrom); err != nil {
			l.fail("MAIL_FROM", "bukan alamat email valid (contoh: Nama <a@b.id>)")
		}
	}
	c.MailTimeout = l.dur("MAIL_TIMEOUT", 10*time.Second)
	if c.MailTimeout > 60*time.Second {
		l.fail("MAIL_TIMEOUT", "maksimal 60s (sekarang %s)", c.MailTimeout)
	}
	c.InvitationTTL = l.dur("INVITATION_TTL", 168*time.Hour)
	if c.InvitationTTL < time.Hour || c.InvitationTTL > 720*time.Hour {
		l.fail("INVITATION_TTL", "harus antara 1h dan 720h (sekarang %s)", c.InvitationTTL)
	}
	c.InvitationMaxPerHour, _ = l.intr("INVITATION_MAX_PER_HOUR", 20, 1, 1000)
	c.InvitationResendMaxPerHour, _ = l.intr("INVITATION_RESEND_MAX_PER_HOUR", 3, 1, 100)
	c.InvitationPurgeAfter = l.dur("INVITATION_PURGE_AFTER", 720*time.Hour)
	if c.InvitationPurgeAfter < 24*time.Hour || c.InvitationPurgeAfter > 8760*time.Hour {
		l.fail("INVITATION_PURGE_AFTER", "harus antara 24h dan 8760h (sekarang %s)", c.InvitationPurgeAfter)
	}
	switch c.MailDriver {
	case "log":
		if appEnv == "production" {
			l.fail("MAIL_DRIVER", "driver log tidak boleh dipakai saat APP_ENV=production")
		}
	case "smtp":
		if c.SMTPHost == "" {
			l.fail("SMTP_HOST", "wajib diisi bila MAIL_DRIVER=smtp")
		}
		if c.MailFrom == "" {
			l.fail("MAIL_FROM", "wajib diisi bila MAIL_DRIVER=smtp")
		}
		if c.PublicURL == "" {
			l.fail("PUBLIC_URL", "wajib diisi bila MAIL_DRIVER=smtp (dipakai membentuk URL login di email)")
		}
	}

	// DEV_VITE_URL
	if v, ok := l.raw("DEV_VITE_URL"); ok {
		u, err := url.Parse(v)
		switch {
		case err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "":
			l.fail("DEV_VITE_URL", "harus URL http(s) dengan host (nilai %q)", v)
		case appEnv != "development":
			l.fail("DEV_VITE_URL", "hanya boleh diisi saat APP_ENV=development")
		default:
			c.DevViteURL = v
		}
	}

	if len(l.problems) > 0 {
		return Config{}, &ValidationError{Problems: l.problems}
	}
	return c, nil
}

// maskURL mengganti password pada userinfo dengan ***. Gagal parse -> ***.
func maskURL(s string) string {
	u, err := url.Parse(s)
	if err != nil {
		return "***"
	}
	if u.User == nil {
		return s
	}
	if _, has := u.User.Password(); has {
		u.User = url.UserPassword(u.User.Username(), "***")
		return strings.Replace(u.String(), url.QueryEscape("***"), "***", 1)
	}
	return u.String()
}

func (c Config) masked() Config {
	if c.DatabaseURL != "" {
		c.DatabaseURL = maskURL(c.DatabaseURL)
	}
	if c.SessionSecret != "" {
		c.SessionSecret = "***"
	}
	if c.SMTPPassword != "" {
		c.SMTPPassword = "***"
	}
	return c
}

type plain Config // tanpa method agar %+v tidak rekursif

// String menampilkan config dengan secret disamarkan.
func (c Config) String() string { return fmt.Sprintf("%+v", plain(c.masked())) }

// GoString mencegah %#v membocorkan secret.
func (c Config) GoString() string { return c.String() }

// LogValue mengimplementasikan slog.LogValuer dengan secret disamarkan.
func (c Config) LogValue() slog.Value {
	m := c.masked()
	return slog.GroupValue(
		slog.String("database_url", m.DatabaseURL),
		slog.String("http_addr", m.HTTPAddr),
		slog.String("app_env", m.AppEnv),
		slog.String("session_secret", m.SessionSecret),
		slog.Bool("cookie_secure", m.CookieSecure),
		slog.Int("audit_retention_days", m.AuditRetentionDays),
		slog.Int64("max_file_bytes", m.MaxFileBytes),
		slog.Int64("max_upload_bytes", m.MaxUploadBytes),
		slog.Int("max_upload_files", m.MaxUploadFiles),
		slog.Int("max_project_files", m.MaxProjectFiles),
		slog.Int("mcp_max_response_bytes", m.MCPMaxResponseBytes),
		slog.Bool("run_migration", m.RunMigration),
		slog.Int("db_max_open_conns", m.DBMaxOpenConns),
		slog.Int("db_max_idle_conns", m.DBMaxIdleConns),
		slog.Duration("db_conn_max_lifetime", m.DBConnMaxLifetime),
		slog.Int64("http_default_body_bytes", m.HTTPDefaultBodyBytes),
		slog.Duration("http_read_header_timeout", m.HTTPReadHeaderTimeout),
		slog.Duration("http_default_timeout", m.HTTPDefaultTimeout),
		slog.Duration("shutdown_timeout", m.ShutdownTimeout),
		slog.Duration("bus_shutdown_timeout", m.BusShutdownTimeout),
		slog.String("log_level", m.LogLevel),
		slog.String("log_format", m.LogFormat),
		slog.String("dev_vite_url", m.DevViteURL),
		slog.Duration("session_idle_timeout", m.SessionIdleTimeout),
		slog.Duration("session_absolute_timeout", m.SessionAbsoluteTimeout),
		slog.Int("password_min_length", m.PasswordMinLength),
		slog.Int("argon2_memory_kib", int(m.Argon2MemoryKiB)),
		slog.Int("argon2_iterations", int(m.Argon2Iterations)),
		slog.Int("argon2_parallelism", int(m.Argon2Parallelism)),
		slog.Int("login_max_failures", m.LoginMaxFailures),
		slog.Duration("login_failure_window", m.LoginFailureWindow),
		slog.Duration("login_lock_duration", m.LoginLockDuration),
		slog.Int("login_rate_per_minute", m.LoginRatePerMinute),
		slog.String("public_url", m.PublicURL),
		slog.Int("api_key_max_active_per_project", m.APIKeyMaxActivePerProject),
		slog.Int64("apispec_max_file_bytes", m.APISpecMaxFileBytes),
		slog.Int("apispec_max_specs", m.APISpecMaxSpecs),
		slog.Int("mcp_rate_per_minute", m.MCPRatePerMinute),
		slog.Int("mcp_auth_fail_per_minute", m.MCPAuthFailPerMinute),
		slog.Int64("mcp_max_request_bytes", m.MCPMaxRequestBytes),
		slog.Duration("mcp_tool_timeout", m.MCPToolTimeout),
		slog.Any("mcp_allowed_origins", m.MCPAllowedOrigins),
		slog.String("mail_driver", m.MailDriver),
		slog.String("smtp_host", m.SMTPHost),
		slog.Int("smtp_port", m.SMTPPort),
		slog.String("smtp_username", m.SMTPUsername),
		slog.String("smtp_password", m.SMTPPassword),
		slog.String("smtp_tls", m.SMTPTLS),
		slog.String("mail_from", m.MailFrom),
		slog.Duration("mail_timeout", m.MailTimeout),
		slog.Duration("invitation_ttl", m.InvitationTTL),
		slog.Int("invitation_max_per_hour", m.InvitationMaxPerHour),
		slog.Int("invitation_resend_max_per_hour", m.InvitationResendMaxPerHour),
		slog.Duration("invitation_purge_after", m.InvitationPurgeAfter),
	)
}
