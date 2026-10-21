package createkey

// CreateRequest adalah body POST /projects/{projectID}/api-keys.
type CreateRequest struct {
	Name string `json:"name"`
}
