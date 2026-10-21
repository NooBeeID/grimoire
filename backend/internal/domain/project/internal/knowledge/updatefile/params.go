package updatefile

// UpdateRequest adalah body PUT. Content pointer agar "tidak dikirim" (nil) dibedakan dari "" (kosong sengaja).
type UpdateRequest struct {
	Content *string `json:"content"`
	Version string  `json:"version"`
}
