package login

import "grimoire/internal/domain/core/internal/auth/base"

// LoginRequest adalah body POST /api/auth/login.
type LoginRequest struct {
	Email    string      `json:"email"`
	Password base.Secret `json:"password"` // log/marshal -> "***"
}

// LoginResponse adalah body sukses.
type LoginResponse struct {
	User base.MeDTO `json:"user"`
}
