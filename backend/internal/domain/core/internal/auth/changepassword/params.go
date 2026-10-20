// Package changepassword menangani POST /api/auth/change-password: user mengganti
// passwordnya sendiri dan semua sesi lain dicabut.
package changepassword

import "grimoire/internal/domain/core/internal/auth/base"

type request struct {
	CurrentPassword base.Secret `json:"current_password"`
	NewPassword     base.Secret `json:"new_password"`
}
