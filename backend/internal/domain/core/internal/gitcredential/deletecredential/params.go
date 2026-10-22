package deletecredential

import "grimoire/internal/domain/core/internal/gitcredential/base"

// Input adalah masukan usecase. Tanpa body: semua dari path/query/context.
type Input struct {
	Provider string // path {provider}, mentah
	Host     string // query ?host=, mentah, boleh kosong
	Meta     base.RequestMeta
}
