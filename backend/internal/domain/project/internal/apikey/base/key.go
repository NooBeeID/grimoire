package base

import (
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	NameMinRunes = 2
	NameMaxRunes = 60
	TargetType   = "api_key" // TargetType audit
)

type Key struct {
	ID             string
	ProjectID      string
	Name           string
	Prefix         string
	SecretHash     []byte
	CreatedBy      *string
	CreatedByLabel *string
	CreatedAt      time.Time
	LastUsedAt     *time.Time
	RevokedAt      *time.Time
	RevokedBy      *string
	RevokedByLabel *string
}

// ValidateName men-trim lalu memeriksa panjang 2..60 rune dan karakter kontrol.
func ValidateName(s string) (string, FieldErrors) {
	s = strings.TrimSpace(s)
	n := utf8.RuneCountInString(s)
	if n < NameMinRunes || n > NameMaxRunes {
		return s, FieldErrors{"name": "nama key wajib 2-60 karakter"}
	}
	for _, r := range s {
		if unicode.IsControl(r) {
			return s, FieldErrors{"name": "nama key tidak boleh mengandung karakter kontrol"}
		}
	}
	return s, nil
}

// NewKey membangun key baru; ID diisi DB. Secret plaintext tidak disimpan.
func NewKey(projectID, name string, raw RawKey, actor Actor, now time.Time) (Key, error) {
	n, fe := ValidateName(name)
	if len(fe) > 0 {
		return Key{}, InvalidFields(fe)
	}
	id := actor.ID
	k := Key{
		ProjectID:  projectID,
		Name:       n,
		Prefix:     raw.Prefix,
		SecretHash: HashSecret(raw.Secret),
		CreatedBy:  &id,
		CreatedAt:  now.UTC(),
	}
	if actor.Label != "" {
		l := actor.Label
		k.CreatedByLabel = &l
	}
	return k, nil
}

// CheckActiveLimit menolak bila jumlah key aktif sudah mencapai limit.
func CheckActiveLimit(activeCount, limit int) error {
	if activeCount >= limit {
		return ErrKeyLimitReached(limit)
	}
	return nil
}

func (k Key) Active() bool { return k.RevokedAt == nil }

// Revoke mencabut key; sudah dicabut -> ErrAlreadyRevoked tanpa mengubah struct.
func (k *Key) Revoke(actor Actor, now time.Time) error {
	if !k.Active() {
		return ErrAlreadyRevoked
	}
	t := now.UTC()
	id := actor.ID
	k.RevokedAt = &t
	k.RevokedBy = &id
	if actor.Label != "" {
		l := actor.Label
		k.RevokedByLabel = &l
	}
	return nil
}
