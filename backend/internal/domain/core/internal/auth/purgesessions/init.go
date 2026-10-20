package purgesessions

import (
	"context"

	"grimoire/internal/domain/core/internal/auth/base"
	"grimoire/internal/infra/app"
	"grimoire/internal/infra/jobs"
)

const (
	JobName = "core.auth.purge-sessions"
	DailyAt = "03:00"
)

// newJob dipisah agar test membaca Name/DailyAt tanpa scheduler.
func newJob(run func(context.Context) error) jobs.Job {
	return jobs.Job{Name: JobName, DailyAt: DailyAt, Run: run}
}

// Init mendaftarkan job terjadwal. Tidak ada route HTTP maupun handler bus.
func Init(d app.Deps, s base.Shared) {
	uc := New(NewRepository(d.DB), s.Clock, s.Session.IdleTimeout, s.Throttle.Window, d.Log)
	d.Jobs.Register(newJob(uc.Run))
}
