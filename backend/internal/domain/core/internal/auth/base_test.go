package auth

import (
	"net/http"
	"testing"
	"time"

	"grimoire/internal/domain/core/internal/auth/base"
	"grimoire/internal/infra/config"
	httpx "grimoire/internal/infra/http"
	"grimoire/lib/common/errs"
)

func testConfig() config.Config {
	return config.Config{
		SessionSecret: "0123456789abcdef0123456789abcdef", CookieSecure: true,
		SessionIdleTimeout: time.Hour, SessionAbsoluteTimeout: 2 * time.Hour, PasswordMinLength: 10,
		Argon2MemoryKiB: 64, Argon2Iterations: 1, Argon2Parallelism: 1,
		LoginMaxFailures: 5, LoginFailureWindow: 15 * time.Minute, LoginLockDuration: 15 * time.Minute, LoginRatePerMinute: 20,
	}
}

func TestBuildShared(t *testing.T) {
	cfg := testConfig()
	s, err := buildShared(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := s.Hasher.Verify(s.DummyHash, dummyPassword); !ok || err != nil {
		t.Fatalf("dummy hash tidak valid: %v %v", ok, err)
	}
	if s.Clock == nil || s.Rand == nil || string(s.Session.Secret) != cfg.SessionSecret || s.Session.IdleTimeout != time.Hour ||
		s.Session.AbsoluteTimeout != 2*time.Hour || !s.Session.CookieSecure || s.Password.MinLength != 10 ||
		s.Throttle != (base.ThrottlePolicy{MaxFailures: 5, Window: 15 * time.Minute, LockDuration: 15 * time.Minute}) ||
		s.LoginRatePerMinute != 20 {
		t.Fatalf("%+v", s)
	}
}

func TestRegister_CodesRegisteredOnceAndIdempotent(t *testing.T) {
	registerCodes()
	registerCodes() // tidak boleh panic (sync.Once)
	if httpx.StatusFor(base.CodeInvalidCredentials) != http.StatusUnauthorized ||
		httpx.StatusFor(base.CodeAccountLocked) != http.StatusTooManyRequests {
		t.Fatal("status kode auth salah")
	}
	if httpx.StatusFor(errs.CodePasswordChangeRequired) != http.StatusForbidden {
		t.Fatal("PASSWORD_CHANGE_REQUIRED harus 403 bawaan")
	}
}
