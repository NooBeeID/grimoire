package savecredential

import (
	"bytes"
	"encoding/json"
	"time"

	"grimoire/lib/common/errs"
	"grimoire/lib/common/secret"
)

// SaveRequest adalah body PUT /me/git-credentials/{provider}.
// Token bertipe secret.String: String()/Format/slog/json.Marshal selalu "[REDACTED]".
type SaveRequest struct {
	Host  string
	Token secret.String
}

// UnmarshalJSON ada di sini (bukan di lib/common/secret) agar tipe lib tetap tidak bisa dibuat dari JSON.
// Semua kegagalan decode dikembalikan tanpa membungkus error asli (pesan json bisa memantulkan input).
func (r *SaveRequest) UnmarshalJSON(b []byte) error {
	var aux struct {
		Host  *string `json:"host"`
		Token *string `json:"token"`
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&aux); err != nil {
		return errs.New(errs.CodeInvalid, "permintaan tidak valid")
	}
	if aux.Token == nil {
		return errs.WithDetails(errs.CodeInvalid, "permintaan tidak valid",
			map[string]any{"fields": map[string]string{"token": "token wajib diisi"}})
	}
	r.Token = secret.New(*aux.Token)
	r.Host = ""
	if aux.Host != nil {
		r.Host = *aux.Host
	}
	return nil
}

// response adalah bentuk JSON sukses; sengaja tanpa field token.
type response struct {
	Provider  string    `json:"provider"`
	Host      string    `json:"host"`
	Last4     string    `json:"last4"`
	UpdatedAt time.Time `json:"updated_at"`
	Replaced  bool      `json:"replaced"`
}

func newResponse(r Result) response {
	return response{Provider: r.Provider, Host: r.Host, Last4: r.Last4, UpdatedAt: r.UpdatedAt.UTC(), Replaced: r.Replaced}
}
