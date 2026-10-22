package bustoken

import (
	"context"

	"grimoire/internal/infra/bus"
	"grimoire/lib/contracts/core/gitcredential/gettoken"
)

type Handler struct{ uc *Usecase }

func NewHandler(uc *Usecase) *Handler { return &Handler{uc: uc} }

// Handle mengembalikan NILAI gettoken.Response agar bus.CallTyped bekerja.
func (h *Handler) Handle(ctx context.Context, req any) (any, error) {
	in, err := bus.Expect[gettoken.Request](req)
	if err != nil {
		return nil, err
	}
	return h.uc.Execute(ctx, in)
}
