// Package base berisi entity dan rule bisnis module project/api-key. Tanpa I/O dan tanpa context.
package base

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"strings"
)

const (
	KeyScheme        = "gr"
	PrefixLen        = 8
	PrefixAlphabet   = "abcdefghijklmnopqrstuvwxyz0123456789"
	SecretBytes      = 32
	SecretEncodedLen = 43                                                    // base64.RawURLEncoding(32 byte)
	RawKeyLen        = len(KeyScheme) + 1 + PrefixLen + 1 + SecretEncodedLen // 55
	MaxRawKeyInput   = 256                                                   // input lebih panjang langsung ditolak
)

// RawKey adalah key plaintext. Full HANYA untuk respons createkey.
type RawKey struct {
	Full   string // "gr_<prefix>_<secret>"
	Prefix string
	Secret string
}

// GenerateRawKey membaca byte acak dari r (produksi: crypto/rand.Reader).
// Prefix memakai rejection sampling (b < 252) agar tanpa bias.
func GenerateRawKey(r io.Reader) (RawKey, error) {
	const limit = 252 // 36*7
	prefix := make([]byte, 0, PrefixLen)
	one := make([]byte, 1)
	for len(prefix) < PrefixLen {
		if _, err := io.ReadFull(r, one); err != nil {
			return RawKey{}, fmt.Errorf("rawkey: read random: %w", err)
		}
		if one[0] >= limit {
			continue
		}
		prefix = append(prefix, PrefixAlphabet[int(one[0])%len(PrefixAlphabet)])
	}
	sec := make([]byte, SecretBytes)
	if _, err := io.ReadFull(r, sec); err != nil {
		return RawKey{}, fmt.Errorf("rawkey: read random: %w", err)
	}
	secret := base64.RawURLEncoding.EncodeToString(sec)
	p := string(prefix)
	return RawKey{Full: KeyScheme + "_" + p + "_" + secret, Prefix: p, Secret: secret}, nil
}

// ParseRawKey memakai posisi tetap (secret base64url boleh memuat '_' dan '-').
func ParseRawKey(s string) (prefix, secret string, ok bool) {
	s = strings.TrimSpace(s)
	if len(s) > MaxRawKeyInput || len(s) != RawKeyLen {
		return "", "", false
	}
	if s[:3] != KeyScheme+"_" || s[3+PrefixLen] != '_' {
		return "", "", false
	}
	prefix = s[3 : 3+PrefixLen]
	for i := 0; i < len(prefix); i++ {
		if strings.IndexByte(PrefixAlphabet, prefix[i]) < 0 {
			return "", "", false
		}
	}
	secret = s[3+PrefixLen+1:]
	dec, err := base64.RawURLEncoding.Strict().DecodeString(secret)
	if err != nil || len(dec) != SecretBytes {
		return "", "", false
	}
	return prefix, secret, true
}

// HashSecret = SHA-256 atas string secret ter-encode (32 byte).
func HashSecret(secret string) []byte {
	h := sha256.Sum256([]byte(secret))
	return h[:]
}

// DummyHash dipakai VerifyOutcome saat prefix tidak dikenal agar waktu jalur setara.
var DummyHash = HashSecret("grimoire-dummy-secret")
