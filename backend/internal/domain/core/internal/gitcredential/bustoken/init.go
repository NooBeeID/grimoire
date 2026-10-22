package bustoken

import (
	"grimoire/internal/domain/core/internal/gitcredential/base"
	"grimoire/internal/infra/app"
	"grimoire/lib/constants"
)

// Init mendaftarkan handler bus core.gitcredential.get-token.
func Init(d app.Deps, sh base.Shared) {
	var o opener // harus nil interface murni bila Box nil
	if sh.Box != nil {
		o = sh.Box
	}
	h := NewHandler(NewUsecase(NewRepository(d.DB), o, d.Log))
	d.Bus.Register(constants.BUS_CORE_GITCREDENTIAL_GET_TOKEN, h.Handle)
}
