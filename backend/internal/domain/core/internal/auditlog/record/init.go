package record

import (
	"time"

	"github.com/google/uuid"

	"grimoire/internal/infra/app"
	"grimoire/lib/constants"
)

// Init merakit repo -> usecase -> handler bus (satu registrasi untuk Call dan Publish).
func Init(d app.Deps) {
	uc := NewUsecase(NewRepo(d.DB), time.Now, newUUIDv7)
	d.Bus.Register(constants.BUS_CORE_AUDIT_RECORD, handler{uc: uc}.Handle)
}

func newUUIDv7() string { return uuid.Must(uuid.NewV7()).String() }
