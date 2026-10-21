// Package createproject adalah feature POST /api/projects.
package createproject

// CreateRequest adalah body request; hanya bentuk, validasi isi ada di base.NewProject.
type CreateRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}
