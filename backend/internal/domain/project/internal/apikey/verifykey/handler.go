package verifykey

import (
	"context"

	"grimoire/internal/infra/bus"
	"grimoire/lib/contracts/project/apikey/verify"
)

type handler struct{ uc *Usecase }

func newHandler(uc *Usecase) *handler { return &handler{uc: uc} }

// Handle mengembalikan verify.Response sebagai NILAI.
func (h *handler) Handle(ctx context.Context, req any) (any, error) {
	in, err := bus.Expect[verify.Request](req)
	if err != nil {
		return nil, err
	}
	res, err := h.uc.Execute(ctx, in)
	if err != nil {
		return verify.Response{}, err
	}
	return res, nil
}
