package countmemberships

import (
	"grimoire/internal/infra/app"
	"grimoire/lib/constants"
)

// Init mendaftarkan handler bus project.management.count-memberships.
func Init(d app.Deps) {
	uc := NewUsecase(newRepo(d.DB))
	d.Bus.Register(constants.BUS_PROJECT_MANAGEMENT_COUNT_MEMBERSHIPS, newHandler(uc).Handle)
}
