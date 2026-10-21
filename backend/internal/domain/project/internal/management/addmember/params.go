package addmember

import (
	"strings"

	"grimoire/internal/domain/project/internal/management/base"
)

// AddMemberRequest adalah body POST. Role opsional ("" -> member).
type AddMemberRequest struct {
	UserID string `json:"user_id"`
	Role   string `json:"role"`
}

// Validate hanya memeriksa bentuk. Isi (uuid, role, kelayakan) bukan urusan di sini.
func (r *AddMemberRequest) Validate() error {
	r.UserID = strings.TrimSpace(r.UserID)
	if r.UserID == "" {
		return base.InvalidFields(base.FieldErrors{"user_id": "wajib diisi"})
	}
	return nil
}
