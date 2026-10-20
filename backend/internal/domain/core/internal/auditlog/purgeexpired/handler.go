package purgeexpired

import "context"

type runner interface {
	Run(ctx context.Context) error
}

// Handler adalah adaptor Run(ctx) error untuk jobs.Job.
func Handler(uc runner) func(ctx context.Context) error { return uc.Run }
