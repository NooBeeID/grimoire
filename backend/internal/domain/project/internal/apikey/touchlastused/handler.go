package touchlastused

import (
	"context"

	"grimoire/internal/infra/bus"
	"grimoire/lib/contracts/project/apikey/touch"
)

// Handler adalah handler bus project.apikey.touch.
type Handler struct{ uc *Usecase }

func newHandler(uc *Usecase) *Handler { return &Handler{uc: uc} }

// Handle hanya mengembalikan error bila tipe request salah.
func (h *Handler) Handle(ctx context.Context, req any) (any, error) {
	r, err := bus.Expect[touch.Request](req)
	if err != nil {
		return nil, err
	}
	h.uc.Touch(ctx, r)
	return nil, nil
}
