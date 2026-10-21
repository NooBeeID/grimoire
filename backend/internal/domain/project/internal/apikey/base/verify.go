package base

import "crypto/subtle"

// Nilai string sama dengan konstanta kontrak verify (diuji di feature verifykey).
const (
	ReasonMalformed = "malformed"
	ReasonUnknown   = "unknown"
	ReasonRevoked   = "revoked"
	ReasonMismatch  = "mismatch"
)

// MatchSecret membandingkan hash tersimpan dengan hash secret dalam waktu konstan.
func MatchSecret(stored []byte, secret string) bool {
	return subtle.ConstantTimeCompare(stored, HashSecret(secret)) == 1
}

// VerifyOutcome SELALU menghitung hash + compare lebih dulu (DummyHash bila !found),
// lalu menentukan hasil: unknown, mismatch, revoked.
func VerifyOutcome(k Key, found bool, secret string) (ok bool, reason string) {
	stored := k.SecretHash
	if !found {
		stored = DummyHash
	}
	match := MatchSecret(stored, secret)
	switch {
	case !found:
		return false, ReasonUnknown
	case !match:
		return false, ReasonMismatch
	case !k.Active():
		return false, ReasonRevoked
	}
	return true, ""
}
