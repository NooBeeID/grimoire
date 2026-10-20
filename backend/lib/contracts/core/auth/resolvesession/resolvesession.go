// Package resolvesession adalah kontrak bus core.auth.resolve-session.
// Sesi tidak valid/kedaluwarsa/user nonaktif dikembalikan sebagai Found=false (bukan error).
package resolvesession

type Request struct {
	Token string // nilai mentah cookie grimoire_session
}

type Response struct {
	Found              bool
	UserID             string
	Email              string
	Name               string
	Role               string // RoleSuperAdmin | RoleDeveloper
	MustChangePassword bool
}

const (
	RoleSuperAdmin = "super_admin"
	RoleDeveloper  = "developer"
)
