package buslistgroups

import (
	"grimoire/internal/domain/project/internal/searchsynonym/base/store"
	"grimoire/internal/infra/app"
	"grimoire/lib/constants"
)

// Init mendaftarkan handler bus project.searchsynonym.list-groups. Panggil sekali saja.
func Init(d app.Deps) {
	h := &Handler{uc: New(NewRepo(store.New(d.DB)))}
	d.Bus.Register(constants.BUS_PROJECT_SEARCHSYNONYM_LIST_GROUPS, h.Handle)
}
