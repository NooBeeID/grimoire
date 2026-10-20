//go:build integration

package purgesessions

import (
	"io"
	"log/slog"
	"testing"
	"time"

	"grimoire/internal/domain/core/internal/auth/base"
	"grimoire/internal/infra/app"
	"grimoire/internal/infra/db"
	"grimoire/internal/infra/jobs"
)

func TestInit_RegistersJob(t *testing.T) {
	d := db.TestDB(t)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	deps := app.Deps{DB: d, Log: log, Jobs: jobs.New(d, log)}
	s := base.Shared{
		Clock:    base.SystemClock,
		Session:  base.SessionConfig{IdleTimeout: time.Hour},
		Throttle: base.ThrottlePolicy{Window: time.Minute},
	}
	Init(deps, s)
	defer func() {
		if recover() == nil {
			t.Fatal("Init kedua harus panic (nama duplikat)")
		}
	}()
	Init(deps, s)
}
