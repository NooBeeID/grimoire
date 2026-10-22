package base

import (
	"time"

	"github.com/google/uuid"
)

// Credential adalah satu PAT tersimpan. TokenEnc terenkripsi; plaintext tidak pernah ada di entity.
type Credential struct {
	UserID    string
	Provider  Provider
	Host      string
	TokenEnc  []byte
	Last4     string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Saved adalah hasil upsert credential.
type Saved struct {
	CreatedAt, UpdatedAt time.Time
	Replaced             bool
}

// CredentialDTO adalah bentuk JSON ke klien. Sengaja tanpa field token.
type CredentialDTO struct {
	Provider  string    `json:"provider"`
	Host      string    `json:"host"`
	Last4     string    `json:"last4"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ToDTO memetakan ke DTO (UTC).
func (c Credential) ToDTO() CredentialDTO {
	return CredentialDTO{
		Provider: string(c.Provider), Host: c.Host, Last4: c.Last4,
		CreatedAt: c.CreatedAt.UTC(), UpdatedAt: c.UpdatedAt.UTC(),
	}
}

// ValidUUID true bila semua id adalah uuid valid.
func ValidUUID(ids ...string) bool {
	for _, id := range ids {
		if _, err := uuid.Parse(id); err != nil {
			return false
		}
	}
	return true
}
