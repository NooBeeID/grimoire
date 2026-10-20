// Package finddevelopers adalah kontrak bus core.auth.find-developers.
// Hanya role developer yang aktif. Query kosong menghasilkan Items kosong.
// Limit <= 0 menjadi DefaultLimit; Limit > MaxLimit di-clamp.
package finddevelopers

const (
	DefaultLimit = 10
	MaxLimit     = 50
)

type Request struct {
	Query string
	Limit int
}

type Developer struct {
	ID, Name, Email string
}

// Response.Items tidak pernah nil; kosong = []Developer{}.
type Response struct {
	Items []Developer
}
