package purgeexpired

import (
	"context"
	"time"

	"grimoire/internal/infra/app"
	"grimoire/internal/infra/jobs"
)

const (
	JobName = "core.auditlog.purge-expired"
	DailyAt = "03:15"
)

// newJob merakit definisi job (dipisah agar tes membaca Name/DailyAt tanpa scheduler).
func newJob(run func(context.Context) error) jobs.Job {
	return jobs.Job{Name: JobName, DailyAt: DailyAt, Run: run}
}

func build(d app.Deps) *Usecase {
	repo := NewRepository(d.DB)
	return New(repo, Options{Retention: d.Config.AuditRetentionDays, BatchSize: BatchSize, Pause: Pause}, time.Now, d.Log)
}

// Init mendaftarkan job purge ke scheduler.
func Init(d app.Deps) {
	d.Jobs.Register(newJob(Handler(build(d))))
}
