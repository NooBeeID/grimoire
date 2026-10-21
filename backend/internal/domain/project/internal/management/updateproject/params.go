package updateproject

import "grimoire/internal/domain/project/internal/management/base"

// UpdateRequest adalah body PATCH. Pointer membedakan "tidak dikirim" (nil) dari "" (dikirim kosong).
type UpdateRequest struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
}

// Patch mengubah request menjadi patch entity; validasi ada di entity.
func (r UpdateRequest) Patch() base.ProjectPatch {
	return base.ProjectPatch{Name: r.Name, Description: r.Description}
}
