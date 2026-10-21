package getprojectbus

import (
	"context"

	"grimoire/internal/infra/bus"
	"grimoire/lib/contracts/project/management/getproject"
)

// Handler adalah handler bus untuk project.management.get-project.
type Handler struct{ uc *Usecase }

func newHandler(uc *Usecase) *Handler { return &Handler{uc: uc} }

// Handle adalah bus.Handler; mengembalikan getproject.Response sebagai nilai.
func (h *Handler) Handle(ctx context.Context, req any) (any, error) {
	in, err := bus.Expect[getproject.Request](req)
	if err != nil {
		return nil, err
	}
	return h.uc.Execute(ctx, in.ProjectID)
}
