package listkeys

import (
	"context"
	"fmt"

	"grimoire/internal/domain/project/internal/apikey/base"
	"grimoire/internal/domain/project/internal/apikey/base/access"
	"grimoire/lib/common/errs"
	"grimoire/lib/common/reqctx"
)

// KeyLister membaca key sebuah project sesuai scope pemilik (urutan ditentukan repository).
type KeyLister interface {
	ListByProject(ctx context.Context, projectID string, scope base.OwnerScope) ([]base.Key, error)
}

type Usecase struct {
	access access.Checker
	repo   KeyLister
}

func NewUsecase(a access.Checker, r KeyLister) *Usecase { return &Usecase{access: a, repo: r} }

// Execute mengembalikan key project (aktif dan dicabut) tanpa hash/secret:
// key milik pemanggil; super_admin semua. Penyaringan dilakukan di SQL oleh repository.
func (u *Usecase) Execute(ctx context.Context, projectID string) (base.KeyList, error) {
	actor, ok := base.ActorFrom(reqctx.IdentityFrom(ctx))
	if !ok {
		return base.KeyList{}, errs.New(errs.CodeUnauthorized, "perlu login")
	}
	if err := u.access.Require(ctx, projectID, actor); err != nil {
		return base.KeyList{}, err
	}
	scope := actor.OwnerScope()
	keys, err := u.repo.ListByProject(ctx, projectID, scope)
	if err != nil {
		return base.KeyList{}, fmt.Errorf("listkeys: list: %w", err)
	}
	items := make([]base.KeyDTO, 0, len(keys))
	for _, k := range keys {
		items = append(items, k.ToDTO())
	}
	return base.KeyList{Items: items}, nil
}
