// Package validatespec memeriksa file spec OpenAPI seperti upload, tanpa menyimpan apa pun (tanpa DB, tanpa audit).
package validatespec

import "grimoire/internal/domain/project/internal/apispec/base/upload"

// Params adalah input usecase (bukan body JSON).
type Params struct {
	ProjectID string
	Name      *string       // field "name" (sudah di-trim; nil bila kosong)
	Files     []upload.File // urutan part
}
