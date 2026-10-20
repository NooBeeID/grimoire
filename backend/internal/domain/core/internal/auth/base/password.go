package base

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"
)

// MaxPasswordBytes adalah panjang maksimum password (byte).
const MaxPasswordBytes = 256

// PasswordPolicy adalah kebijakan password.
type PasswordPolicy struct{ MinLength int }

// ValidateField memvalidasi pw untuk field tertentu; satu pesan per field.
// Urutan: kosong, panjang minimum (rune), maksimum (byte), sama dengan email.
func (p PasswordPolicy) ValidateField(field, pw, email string) FieldErrors {
	switch {
	case pw == "":
		return FieldErrors{field: msgRequired}
	case utf8.RuneCountInString(pw) < p.MinLength:
		return FieldErrors{field: fmt.Sprintf("minimal %d karakter", p.MinLength)}
	case len(pw) > MaxPasswordBytes:
		return FieldErrors{field: "maksimal 256 byte"}
	case email != "" && strings.EqualFold(pw, email):
		return FieldErrors{field: "password tidak boleh sama dengan email"}
	}
	return nil
}

// Validate adalah ValidateField untuk field "password"; nil bila lolos.
func (p PasswordPolicy) Validate(pw, email string) error {
	if fe := p.ValidateField("password", pw, email); len(fe) > 0 {
		return fe
	}
	return nil
}

// ValidateChange memvalidasi pergantian password: current wajib dan <= 256 byte,
// next mengikuti kebijakan. Mengembalikan nil bila lolos.
func (p PasswordPolicy) ValidateChange(current, next Secret, email string) FieldErrors {
	var fe FieldErrors
	switch {
	case current == "":
		fe = fe.Merge(FieldErrors{"current_password": msgRequired})
	case len(current) > MaxPasswordBytes:
		fe = fe.Merge(FieldErrors{"current_password": "maksimal 256 byte"})
	}
	return fe.Merge(p.ValidateField("new_password", next.Reveal(), email))
}

// DifferentPassword memastikan password baru tidak sama dengan yang lama (constant time).
// Dipanggil setelah password lama terverifikasi.
func DifferentPassword(current, next Secret) error {
	if subtle.ConstantTimeCompare([]byte(current), []byte(next)) == 1 {
		return fieldErr("new_password", "password baru harus berbeda dari password lama")
	}
	return nil
}

// Secret adalah string rahasia yang tidak pernah tercetak/ter-log/ter-marshal.
type Secret string

func (Secret) String() string               { return "***" }
func (Secret) GoString() string             { return `"***"` }
func (Secret) LogValue() slog.Value         { return slog.StringValue("***") }
func (Secret) MarshalJSON() ([]byte, error) { return []byte(`"***"`), nil }
func (Secret) MarshalText() ([]byte, error) { return []byte("***"), nil }
func (s Secret) Reveal() string             { return string(s) }

// Hasher meng-hash dan memverifikasi password.
type Hasher interface {
	Hash(pw string) (string, error)
	// Verify: error hanya untuk hash rusak; mismatch -> (false, nil).
	Verify(encoded, pw string) (bool, error)
}

// Argon2Params adalah parameter argon2id.
type Argon2Params struct {
	MemoryKiB   uint32
	Iterations  uint32
	Parallelism uint8
}

const (
	argonSaltLen = 16
	argonKeyLen  = 32
	// Batas atas parameter yang dibaca dari hash tersimpan (cegah DoS dari data rusak).
	maxStoredMemoryKiB   = 4194304
	maxStoredIterations  = 100
	maxStoredParallelism = 255
)

// Argon2Hasher mengimplementasikan Hasher dengan argon2id, format PHC.
type Argon2Hasher struct {
	Params Argon2Params
	Rand   io.Reader // nil -> crypto/rand
}

var errBadHash = errors.New("auth: format hash password tidak valid")

func (h Argon2Hasher) Hash(pw string) (string, error) {
	r := h.Rand
	if r == nil {
		r = rand.Reader
	}
	salt := make([]byte, argonSaltLen)
	if _, err := io.ReadFull(r, salt); err != nil {
		return "", fmt.Errorf("auth: baca acak salt: %w", err)
	}
	key := argon2.IDKey([]byte(pw), salt, h.Params.Iterations, h.Params.MemoryKiB, h.Params.Parallelism, argonKeyLen)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s", argon2.Version,
		h.Params.MemoryKiB, h.Params.Iterations, h.Params.Parallelism,
		base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key)), nil
}

func (h Argon2Hasher) Verify(encoded, pw string) (bool, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" {
		return false, errBadHash
	}
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return false, errBadHash
	}
	var m, t uint32
	var p uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &m, &t, &p); err != nil {
		return false, errBadHash
	}
	if m == 0 || t == 0 || p == 0 || m > maxStoredMemoryKiB || t > maxStoredIterations || int(p) > maxStoredParallelism {
		return false, errBadHash
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil || len(salt) == 0 {
		return false, errBadHash
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(want) < 16 {
		return false, errBadHash
	}
	got := argon2.IDKey([]byte(pw), salt, t, m, p, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}

const generatedAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz23456789" // tanpa 0 O 1 l I

// GeneratedPasswordLen adalah panjang password sementara.
const GeneratedPasswordLen = 16

// GeneratePassword membuat password acak 16 karakter dengan rejection sampling (tanpa bias).
func GeneratePassword(r io.Reader) (string, error) {
	n := len(generatedAlphabet)
	limit := 256 - 256%n
	out := make([]byte, 0, GeneratedPasswordLen)
	buf := make([]byte, 2*GeneratedPasswordLen)
	for len(out) < GeneratedPasswordLen {
		if _, err := io.ReadFull(r, buf); err != nil {
			return "", fmt.Errorf("auth: baca acak password: %w", err)
		}
		for _, b := range buf {
			if int(b) >= limit {
				continue
			}
			out = append(out, generatedAlphabet[int(b)%n])
			if len(out) == GeneratedPasswordLen {
				break
			}
		}
	}
	return string(out), nil
}
