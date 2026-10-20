//go:build integration

package record

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"grimoire/internal/infra/app"
	"grimoire/internal/infra/bus"
	"grimoire/lib/constants"
	"grimoire/lib/contracts/core/audit/record"
)

type syncBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuf) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuf) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

func publishSetup(t *testing.T) (*bus.Registry, *syncBuf, func() int) {
	t.Helper()
	d := setup(t)
	buf := &syncBuf{}
	reg := bus.NewRegistry(slog.New(slog.NewTextHandler(buf, nil)))
	Init(app.Deps{DB: d, Bus: reg})
	return reg, buf, func() int { return countRows(t, d) }
}

func shutdown(t *testing.T, reg *bus.Registry) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := reg.Shutdown(ctx); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
}

func TestPublish_WritesEntry(t *testing.T) {
	reg, _, rows := publishSetup(t)
	if err := reg.Publish(context.Background(), constants.BUS_CORE_AUDIT_RECORD, validReq()); err != nil {
		t.Fatal(err)
	}
	shutdown(t, reg)
	if n := rows(); n != 1 {
		t.Fatalf("rows=%d", n)
	}
}

func TestPublish_InvalidRequestLoggedByBus(t *testing.T) {
	reg, buf, rows := publishSetup(t)
	req := validReq()
	req.Action = ""
	if err := reg.Publish(context.Background(), constants.BUS_CORE_AUDIT_RECORD, req); err != nil {
		t.Fatal(err)
	}
	shutdown(t, reg)
	if rows() != 0 || !strings.Contains(buf.String(), "bus publish failed") || !strings.Contains(buf.String(), "level=ERROR") {
		t.Fatalf("rows=%d log=%s", rows(), buf.String())
	}
}

func TestPublish_CanceledParentStillWrites(t *testing.T) {
	reg, _, rows := publishSetup(t)
	ctx, cancel := context.WithCancel(context.Background())
	if err := reg.Publish(ctx, constants.BUS_CORE_AUDIT_RECORD, validReq()); err != nil {
		t.Fatal(err)
	}
	cancel()
	shutdown(t, reg)
	if n := rows(); n != 1 {
		t.Fatalf("rows=%d", n)
	}
}

func TestPublish_PointerRequestRejected(t *testing.T) {
	reg, buf, rows := publishSetup(t)
	if err := reg.Publish(context.Background(), constants.BUS_CORE_AUDIT_RECORD, &record.Request{}); err != nil {
		t.Fatal(err)
	}
	shutdown(t, reg)
	if rows() != 0 || !strings.Contains(buf.String(), "bus publish failed") || !strings.Contains(buf.String(), "unexpected request type") {
		t.Fatalf("rows=%d log=%s", rows(), buf.String())
	}
}
