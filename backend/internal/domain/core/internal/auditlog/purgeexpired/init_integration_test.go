//go:build integration

package purgeexpired

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"grimoire/internal/infra/app"
	"grimoire/internal/infra/config"
	"grimoire/internal/infra/jobs"
)

func TestInit_SchedulerAcceptsJob(t *testing.T) {
	d := setupDB(t)
	log := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	s := jobs.New(d, log)
	deps := app.Deps{DB: d, Jobs: s, Log: log, Config: config.Config{AuditRetentionDays: 90}}
	Init(deps)
	defer func() {
		r := recover()
		if r == nil || !strings.Contains(r.(string), "registered twice") {
			t.Fatalf("recover=%v", r)
		}
	}()
	Init(deps)
}

func TestInit_UsesConfiguredRetention(t *testing.T) {
	d := setupDB(t)
	seed(t, d, day(8), 1)
	seed(t, d, day(6), 1)
	uc := build(app.Deps{DB: d, Log: slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)), Config: config.Config{AuditRetentionDays: 7}})
	if err := Handler(uc)(context.Background()); err != nil {
		t.Fatal(err)
	}
	if n := count(t, d, ""); n != 1 {
		t.Fatalf("remaining=%d", n)
	}
}
