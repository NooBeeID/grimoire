package createfile

// CreateRequest adalah body POST /projects/{projectID}/knowledge/files. Hanya bentuk; validasi ada di entity.
type CreateRequest struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}
