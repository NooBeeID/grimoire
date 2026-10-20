package base

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"time"
)

const (
	TokenBytes    = 32
	TouchInterval = time.Minute
)

// NewSessionToken membuat token acak 32 byte (base64url tanpa padding, 43 karakter).
func NewSessionToken(r io.Reader) (string, error) {
	b := make([]byte, TokenBytes)
	if _, err := io.ReadFull(r, b); err != nil {
		return "", fmt.Errorf("auth: baca acak token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// HashToken menghitung HMAC-SHA256(secret, token); hanya hash ini yang disimpan.
func HashToken(secret []byte, token string) []byte {
	m := hmac.New(sha256.New, secret)
	m.Write([]byte(token))
	return m.Sum(nil)
}

// Session adalah sesi login. ID diisi DB.
type Session struct {
	ID, UserID string
	TokenHash  []byte
	CreatedAt  time.Time
	LastSeenAt time.Time
	ExpiresAt  time.Time
	IP         *string
	UserAgent  *string
}

// NewSession membentuk sesi baru yang siap disimpan.
func NewSession(userID string, tokenHash []byte, now time.Time, cfg SessionConfig, meta RequestMeta) Session {
	s := Session{
		UserID: userID, TokenHash: tokenHash,
		CreatedAt: now, LastSeenAt: now, ExpiresAt: now.Add(cfg.AbsoluteTimeout),
	}
	if meta.IP != "" {
		ip := meta.IP
		s.IP = &ip
	}
	if meta.UserAgent != "" {
		ua := TruncateUA(meta.UserAgent)
		s.UserAgent = &ua
	}
	return s
}

// IsValid: now < ExpiresAt dan now < LastSeenAt + idle.
func (s Session) IsValid(now time.Time, idle time.Duration) bool {
	return now.Before(s.ExpiresAt) && now.Before(s.LastSeenAt.Add(idle))
}

// NeedsTouch: true bila last_seen_at sudah lebih tua dari TouchInterval.
func (s Session) NeedsTouch(now time.Time) bool {
	return now.Sub(s.LastSeenAt) >= TouchInterval
}

// MaxAgeSeconds adalah Max-Age cookie sesi.
func (s Session) MaxAgeSeconds(cfg SessionConfig) int {
	return int(cfg.AbsoluteTimeout.Seconds())
}
