package base

import (
	"io"
	"time"
)

// SessionConfig adalah kebijakan sesi dari config.
type SessionConfig struct {
	Secret          []byte
	IdleTimeout     time.Duration
	AbsoluteTimeout time.Duration
	CookieSecure    bool
}

// Shared adalah dependensi bersama (tanpa I/O stateful) yang diteruskan ke Init tiap feature.
type Shared struct {
	Hasher             Hasher
	DummyHash          string
	Clock              Clock
	Rand               io.Reader
	Session            SessionConfig
	Password           PasswordPolicy
	Throttle           ThrottlePolicy
	LoginRatePerMinute int
	// Invitation adalah kebijakan undangan email (T12).
	Invitation InvitationPolicy
	// LoginURL adalah URL halaman login untuk email undangan: PUBLIC_URL + "/login", atau "/login" bila kosong.
	LoginURL string
}
