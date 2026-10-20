package createuser

import "grimoire/internal/domain/core/internal/auth/base"

// CreateUserRequest adalah body POST /api/users.
type CreateUserRequest struct {
	Email    string      `json:"email"`
	Name     string      `json:"name"`
	Role     string      `json:"role"`
	Password base.Secret `json:"password"` // opsional; kosong = dibangkitkan server
}

// Input adalah input usecase (tanpa *http.Request).
type Input struct {
	Email, Name, Role string
	Password          base.Secret
	Meta              base.RequestMeta
}

// Result adalah hasil usecase. Tidak boleh dilog atau masuk audit.
type Result struct {
	User              base.UserDTO `json:"user"`
	TemporaryPassword *string      `json:"temporary_password"` // null bila password diberikan admin
}
