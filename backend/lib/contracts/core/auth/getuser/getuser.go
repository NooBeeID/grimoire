// Package getuser adalah kontrak bus core.auth.get-user.
// UserID bukan uuid atau tidak ada: Found=false tanpa error. User nonaktif:
// Found=true, IsActive=false (konsumen yang memutuskan).
package getuser

type Request struct {
	UserID string
}

type Response struct {
	Found    bool
	ID       string
	Name     string
	Email    string
	Role     string
	IsActive bool
}
