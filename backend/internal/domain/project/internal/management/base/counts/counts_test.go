package counts

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"grimoire/internal/infra/bus"
	"grimoire/lib/constants"
	"grimoire/lib/contracts/project/management/contentcount"
)

func setup() (*bus.Registry, *slog.Logger, *bytes.Buffer) {
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	return bus.NewRegistry(log), log, &buf
}

func TestForProjects_NoProviders(t *testing.T) {
	b, log, buf := setup()
	out := New(b, log).ForProjects(context.Background(), []string{"a"})
	c := out["a"]
	if c.Files != nil || c.Specs != nil || c.Keys != nil {
		t.Fatalf("%+v", c)
	}
	if !strings.Contains(buf.String(), "belum tersedia") {
		t.Fatal(buf.String())
	}
}

func TestForProjects_PartialFailure(t *testing.T) {
	b, log, buf := setup()
	b.Register(constants.BUS_PROJECT_KNOWLEDGE_COUNT_FILES, func(context.Context, any) (any, error) {
		return contentcount.Response{Counts: map[string]int{"a": 3}}, nil
	})
	b.Register(constants.BUS_PROJECT_APISPEC_COUNT_SPECS, func(context.Context, any) (any, error) {
		return nil, errors.New("db down")
	})
	out := New(b, log).ForProjects(context.Background(), []string{"a", "b"})
	if *out["a"].Files != 3 || *out["b"].Files != 0 || out["a"].Specs != nil || out["a"].Keys != nil {
		t.Fatalf("%+v", out)
	}
	if !strings.Contains(buf.String(), "content count gagal") {
		t.Fatal(buf.String())
	}
}

func TestForProjects_EmptyAndDedupe(t *testing.T) {
	b, log, _ := setup()
	calls := 0
	for _, ep := range []string{
		constants.BUS_PROJECT_KNOWLEDGE_COUNT_FILES, constants.BUS_PROJECT_APISPEC_COUNT_SPECS, constants.BUS_PROJECT_APIKEY_COUNT_KEYS,
	} {
		b.Register(ep, func(_ context.Context, req any) (any, error) {
			calls++
			if n := len(req.(contentcount.Request).ProjectIDs); n != 20 {
				return nil, fmt.Errorf("ids = %d", n)
			}
			return contentcount.Response{}, nil
		})
	}
	c := New(b, log)
	if out := c.ForProjects(context.Background(), nil); len(out) != 0 || calls != 0 {
		t.Fatal("kosong tanpa Call")
	}
	ids := make([]string, 0, 40)
	for i := range 20 {
		ids = append(ids, fmt.Sprint("p", i), fmt.Sprint("p", i))
	}
	out := c.ForProjects(context.Background(), ids)
	if calls != 3 || len(out) != 20 || out["p0"].Keys == nil {
		t.Fatalf("calls=%d len=%d", calls, len(out))
	}
}
