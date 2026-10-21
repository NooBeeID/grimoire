package deleteproject

import "grimoire/internal/domain/project/internal/management/base"

// Request adalah body DELETE. Pointer agar "field hilang" dibedakan dari "string kosong".
type Request struct {
	ConfirmName *string `json:"confirm_name"`
}

// Validate hanya memeriksa bentuk. String kosong lolos ke entity (CONFIRMATION_MISMATCH).
func (r Request) Validate() error {
	if r.ConfirmName == nil {
		return base.InvalidFields(base.FieldErrors{"confirm_name": "wajib diisi"})
	}
	return nil
}
