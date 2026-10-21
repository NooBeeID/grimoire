package base

import "time"

const (
	StatusActive  = "active"
	StatusRevoked = "revoked"
)

type KeyDTO struct {
	ID             string     `json:"id"`
	Name           string     `json:"name"`
	Prefix         string     `json:"prefix"`
	Status         string     `json:"status"`
	CreatedBy      *string    `json:"created_by"`
	CreatedByLabel *string    `json:"created_by_label"`
	CreatedAt      time.Time  `json:"created_at"`
	LastUsedAt     *time.Time `json:"last_used_at"`
	RevokedAt      *time.Time `json:"revoked_at"`
	RevokedBy      *string    `json:"revoked_by"`
	RevokedByLabel *string    `json:"revoked_by_label"`
}

func utcPtr(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	u := t.UTC()
	return &u
}

// ToDTO tidak pernah mengeluarkan hash/secret.
func (k Key) ToDTO() KeyDTO {
	st := StatusActive
	if !k.Active() {
		st = StatusRevoked
	}
	return KeyDTO{
		ID: k.ID, Name: k.Name, Prefix: k.Prefix, Status: st,
		CreatedBy: k.CreatedBy, CreatedByLabel: k.CreatedByLabel, CreatedAt: k.CreatedAt.UTC(),
		LastUsedAt: utcPtr(k.LastUsedAt), RevokedAt: utcPtr(k.RevokedAt),
		RevokedBy: k.RevokedBy, RevokedByLabel: k.RevokedByLabel,
	}
}

type CreateKeyResult struct {
	Key    KeyDTO  `json:"key"`
	RawKey string  `json:"raw_key"`
	MCPURL *string `json:"mcp_url"`
}

type KeyList struct {
	Items []KeyDTO `json:"items"`
}

type KeyEnvelope struct {
	Key KeyDTO `json:"key"`
}

// MCPURL: publicURL kosong -> nil; selain itu publicURL + "/mcp".
func MCPURL(publicURL string) *string {
	if publicURL == "" {
		return nil
	}
	u := publicURL + "/mcp"
	return &u
}
