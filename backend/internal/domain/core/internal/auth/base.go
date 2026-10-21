// Package auth adalah module core/auth: akun user, login berbasis sesi, dan manajemen user.
package auth

import (
	"crypto/rand"
	"fmt"
	"net/http"
	"sync"

	"grimoire/internal/domain/core/internal/auth/base"
	"grimoire/internal/domain/core/internal/auth/changepassword"
	"grimoire/internal/domain/core/internal/auth/createadmin"
	"grimoire/internal/domain/core/internal/auth/createuser"
	"grimoire/internal/domain/core/internal/auth/finddevelopers"
	"grimoire/internal/domain/core/internal/auth/getme"
	"grimoire/internal/domain/core/internal/auth/getuser"
	"grimoire/internal/domain/core/internal/auth/getuserbus"
	"grimoire/internal/domain/core/internal/auth/inviteuser"
	"grimoire/internal/domain/core/internal/auth/listusers"
	"grimoire/internal/domain/core/internal/auth/login"
	"grimoire/internal/domain/core/internal/auth/logout"
	"grimoire/internal/domain/core/internal/auth/purgeinvitations"
	"grimoire/internal/domain/core/internal/auth/purgesessions"
	"grimoire/internal/domain/core/internal/auth/resendinvitation"
	"grimoire/internal/domain/core/internal/auth/resetpassword"
	"grimoire/internal/domain/core/internal/auth/resolvesession"
	"grimoire/internal/domain/core/internal/auth/setuserstatus"
	"grimoire/internal/domain/core/internal/auth/updateuser"
	"grimoire/internal/infra/app"
	"grimoire/internal/infra/config"
	httpx "grimoire/internal/infra/http"
)

const dummyPassword = "grimoire-dummy-password"

var codesOnce sync.Once

// registerCodes mendaftarkan status HTTP kode error milik auth. RegisterCodeStatus panic
// bila duplikat, jadi dibungkus sync.Once (Register bisa dipanggil lebih dari sekali per proses).
func registerCodes() {
	codesOnce.Do(func() {
		httpx.RegisterCodeStatus(base.CodeInvalidCredentials, http.StatusUnauthorized)
		httpx.RegisterCodeStatus(base.CodeAccountLocked, http.StatusTooManyRequests)
	})
}

// buildShared merakit dependensi bersama dari config. Dihitung sekali per Register.
func buildShared(cfg config.Config) (base.Shared, error) {
	hasher := base.Argon2Hasher{
		Params: base.Argon2Params{
			MemoryKiB:   cfg.Argon2MemoryKiB,
			Iterations:  cfg.Argon2Iterations,
			Parallelism: cfg.Argon2Parallelism,
		},
		Rand: rand.Reader,
	}
	dummy, err := hasher.Hash(dummyPassword)
	if err != nil {
		return base.Shared{}, fmt.Errorf("auth: hitung dummy hash: %w", err)
	}
	return base.Shared{
		Hasher:    hasher,
		DummyHash: dummy,
		Clock:     base.SystemClock,
		Rand:      rand.Reader,
		Session: base.SessionConfig{
			Secret:          []byte(cfg.SessionSecret),
			IdleTimeout:     cfg.SessionIdleTimeout,
			AbsoluteTimeout: cfg.SessionAbsoluteTimeout,
			CookieSecure:    cfg.CookieSecure,
		},
		Password: base.PasswordPolicy{MinLength: cfg.PasswordMinLength},
		Throttle: base.ThrottlePolicy{
			MaxFailures:  cfg.LoginMaxFailures,
			Window:       cfg.LoginFailureWindow,
			LockDuration: cfg.LoginLockDuration,
		},
		LoginRatePerMinute: cfg.LoginRatePerMinute,
		Invitation: base.InvitationPolicy{
			TTL:                  cfg.InvitationTTL,
			MaxPerInviterPerHour: cfg.InvitationMaxPerHour,
			ResendMaxPerHour:     cfg.InvitationResendMaxPerHour,
			PurgeAfter:           cfg.InvitationPurgeAfter,
		},
		LoginURL: loginURL(cfg.PublicURL),
	}, nil
}

// loginURL membentuk URL login untuk email: PUBLIC_URL + "/login", atau "/login" bila kosong.
func loginURL(publicURL string) string {
	if publicURL == "" {
		return "/login"
	}
	return publicURL + "/login"
}

// Register merakit module auth. Panic bila wiring gagal (rule 5.9).
func Register(d app.Deps) {
	registerCodes()
	shared, err := buildShared(d.Config)
	if err != nil {
		panic(err)
	}
	resolvesession.Init(d, shared)
	getuserbus.Init(d, shared)
	finddevelopers.Init(d, shared)
	createadmin.Init(d, shared)
	purgesessions.Init(d, shared)
	login.Init(d, shared)
	logout.Init(d, shared)
	getme.Init(d, shared)
	changepassword.Init(d, shared)
	createuser.Init(d, shared)
	listusers.Init(d, shared)
	getuser.Init(d, shared)
	updateuser.Init(d, shared)
	setuserstatus.Init(d, shared)
	resetpassword.Init(d, shared)
	inviteuser.Init(d, shared)
	resendinvitation.Init(d, shared)
	purgeinvitations.Init(d, shared)
}
