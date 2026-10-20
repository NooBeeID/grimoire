package base

import (
	"bytes"
	"math/rand"
	"regexp"
	"testing"
	"time"
)

func TestNewSessionToken(t *testing.T) {
	tok, err := NewSessionToken(rand.New(rand.NewSource(1)))
	if err != nil || len(tok) != 43 || !regexp.MustCompile(`^[A-Za-z0-9_-]{43}$`).MatchString(tok) {
		t.Fatalf("%q %v", tok, err)
	}
	if _, err := NewSessionToken(bytes.NewReader([]byte("pendek"))); err == nil {
		t.Fatal("reader pendek harus error")
	}
}

func TestHashToken(t *testing.T) {
	a := HashToken([]byte("s1"), "tok")
	if len(a) != 32 || !bytes.Equal(a, HashToken([]byte("s1"), "tok")) {
		t.Fatal("harus deterministik 32 byte")
	}
	if bytes.Equal(a, HashToken([]byte("s2"), "tok")) || bytes.Equal(a, HashToken([]byte("s1"), "tok2")) {
		t.Fatal("beda input harus beda hash")
	}
}

func TestNewSession(t *testing.T) {
	now := time.Unix(1000, 0)
	cfg := SessionConfig{AbsoluteTimeout: 168 * time.Hour}
	s := NewSession("u1", []byte{1}, now, cfg, RequestMeta{IP: "10.0.0.1", UserAgent: "ua"})
	if s.UserID != "u1" || !s.CreatedAt.Equal(now) || !s.LastSeenAt.Equal(now) || !s.ExpiresAt.Equal(now.Add(168*time.Hour)) ||
		s.IP == nil || *s.IP != "10.0.0.1" || *s.UserAgent != "ua" {
		t.Fatalf("%+v", s)
	}
	if s.MaxAgeSeconds(cfg) != 604800 {
		t.Fatal("max age")
	}
	if e := NewSession("u", nil, now, cfg, RequestMeta{}); e.IP != nil || e.UserAgent != nil {
		t.Fatal("meta kosong harus nil")
	}
}

func TestSession_IsValid(t *testing.T) {
	t0 := time.Unix(0, 0)
	s := Session{LastSeenAt: t0, ExpiresAt: t0.Add(10 * time.Hour)}
	idle := time.Hour
	cases := []struct {
		at   time.Duration
		want bool
	}{{0, true}, {59 * time.Minute, true}, {time.Hour, false}, {2 * time.Hour, false}}
	for _, c := range cases {
		if got := s.IsValid(t0.Add(c.at), idle); got != c.want {
			t.Errorf("idle at %v: got %v", c.at, got)
		}
	}
	s2 := Session{LastSeenAt: t0.Add(9*time.Hour + 30*time.Minute), ExpiresAt: t0.Add(10 * time.Hour)}
	if !s2.IsValid(t0.Add(10*time.Hour-time.Second), idle) || s2.IsValid(t0.Add(10*time.Hour), idle) {
		t.Fatal("batas absolut")
	}
}

func TestSession_NeedsTouch(t *testing.T) {
	t0 := time.Unix(0, 0)
	s := Session{LastSeenAt: t0}
	if s.NeedsTouch(t0.Add(59*time.Second)) || !s.NeedsTouch(t0.Add(60*time.Second)) {
		t.Fatal("NeedsTouch")
	}
}
