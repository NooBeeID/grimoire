package changememberrole

import "grimoire/internal/domain/project/internal/management/base"

// ChangeRoleRequest adalah body PATCH. Pointer membedakan "tidak dikirim" dari "".
type ChangeRoleRequest struct {
	Role *string `json:"role"`
}

// Validate memeriksa bentuk saja (wajib ada). Nilai valid/tidaknya adalah rule entity.
func (r ChangeRoleRequest) Validate() error {
	if r.Role == nil || *r.Role == "" {
		return base.InvalidFields(base.FieldErrors{"role": "wajib diisi"})
	}
	return nil
}

// Value mengembalikan isi role atau "".
func (r ChangeRoleRequest) Value() string {
	if r.Role == nil {
		return ""
	}
	return *r.Role
}
