package base

import (
	"testing"
	"time"
)

func TestVerifyOutcome(t *testing.T) {
	secret := "right-secret"
	active := Key{SecretHash: HashSecret(secret)}
	now := time.Now()
	revoked := Key{SecretHash: HashSecret(secret), RevokedAt: &now}
	cases := []struct {
		name   string
		k      Key
		found  bool
		secret string
		ok     bool
		reason string
	}{
		{"ok", active, true, secret, true, ""},
		{"unknown", Key{}, false, secret, false, ReasonUnknown},
		{"mismatch", active, true, "wrong", false, ReasonMismatch},
		{"revoked", revoked, true, secret, false, ReasonRevoked},
		{"revoked-wrong-secret", revoked, true, "wrong", false, ReasonMismatch},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ok, reason := VerifyOutcome(tc.k, tc.found, tc.secret)
			if ok != tc.ok || reason != tc.reason {
				t.Fatalf("got %v %q", ok, reason)
			}
		})
	}
}

func TestMatchSecret(t *testing.T) {
	h := HashSecret("a")
	if !MatchSecret(h, "a") || MatchSecret(h, "b") || MatchSecret(nil, "a") {
		t.Fatal("match")
	}
}
