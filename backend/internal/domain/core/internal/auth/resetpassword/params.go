package resetpassword

import "grimoire/internal/domain/core/internal/auth/base"

type request struct {
	Password base.Secret `json:"password"` // opsional; "" atau tidak dikirim = bangkitkan
}

// Input adalah masukan usecase.
type Input struct {
	TargetID string
	Password base.Secret
	Meta     base.RequestMeta
}

// Result hanya keluar lewat WriteJSON di handler. TIDAK dilog, TIDAK masuk audit.Event.
type Result struct {
	User              base.UserDTO `json:"user"`
	TemporaryPassword *string      `json:"temporary_password"` // null bila admin yang mengisi
}
