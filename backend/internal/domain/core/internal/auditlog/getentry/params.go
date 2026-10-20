package getentry

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"grimoire/lib/common/errs"
)

// Params adalah input usecase Get.
type Params struct{ ID string }

func notFound() error {
	return errs.Wrap(errs.CodeNotFound, "resource tidak ditemukan", errs.ErrNotFound)
}

// parseParams mengambil URLParam "id". Hanya uuid kanonis (36 karakter) yang
// lolos; selain itu diperlakukan sama dengan tidak ada (404). ID dinormalisasi
// ke lowercase. Pesan error tidak memuat input.
func parseParams(r *http.Request) (Params, error) {
	raw := chi.URLParam(r, "id")
	if len(raw) != 36 {
		return Params{}, notFound()
	}
	u, err := uuid.Parse(raw)
	if err != nil {
		return Params{}, notFound()
	}
	return Params{ID: u.String()}, nil
}
