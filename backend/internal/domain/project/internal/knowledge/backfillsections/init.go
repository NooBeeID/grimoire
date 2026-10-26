package backfillsections

import (
	"context"
	"time"

	"grimoire/internal/domain/project/internal/knowledge/base/store"
	"grimoire/internal/infra/app"
	"grimoire/internal/infra/jobs"
)

const (
	JobName = "knowledge.backfill-sections"
	Every   = time.Minute
)

// newJob dipisah agar test membaca Name/Every tanpa scheduler.
func newJob(run func(context.Context) error) jobs.Job {
	return jobs.Job{Name: JobName, Every: Every, Run: run}
}

// Init mendaftarkan job terjadwal. Tidak ada route HTTP maupun handler bus.
func Init(d app.Deps) {
	if d.Jobs == nil { // dependensi tes tidak membawa scheduler
		return
	}
	uc := NewUsecase(NewRepository(store.New(d.DB)))
	d.Jobs.Register(newJob(uc.Run))
}
