package getme

import (
	"context"

	"grimoire/internal/domain/core/internal/auth/base"
	"grimoire/lib/common/errs"
	"grimoire/lib/common/reqctx"
)

// Usecase mengembalikan profil sesi saat ini tanpa dependensi.
type Usecase struct{}

// NewUsecase membuat Usecase.
func NewUsecase() *Usecase { return &Usecase{} }

// Execute mengembalikan profil dari identitas sesi pada context.
// Tanpa identitas -> errs.CodeUnauthorized.
func (u *Usecase) Execute(ctx context.Context) (base.MeDTO, error) {
	id, ok := reqctx.IdentityFrom(ctx)
	if !ok {
		return base.MeDTO{}, errs.New(errs.CodeUnauthorized, "perlu login")
	}
	return base.MeFromIdentity(id), nil
}
