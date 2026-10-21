//go:build integration

package scanentries

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"grimoire/internal/infra/app"
	"grimoire/internal/infra/bus"
	"grimoire/lib/common/errs"
	"grimoire/lib/constants"
	"grimoire/lib/contracts/core/audit/scanentries"
)

func TestInit_BusRoundTrip(t *testing.T) {
	d := setupDB(t)
	log := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	reg := bus.NewRegistry(log)
	deps := app.Deps{DB: d, Bus: reg, Log: log}
	Init(deps)

	p := uuid.NewString()
	seed(t, d, seedRow{Project: p, At: time.Now().Add(-time.Hour).UTC().Truncate(time.Microsecond)})
	req := scanentries.Request{ProjectID: p, Actions: []string{"mcp.search_knowledge"}, From: time.Now().Add(-48 * time.Hour)}

	resp, err := bus.CallTyped[scanentries.Response](context.Background(), reg, constants.BUS_CORE_AUDIT_SCAN_ENTRIES, req)
	if err != nil || len(resp.Entries) != 1 {
		t.Fatalf("resp=%+v err=%v", resp, err)
	}

	_, err = bus.CallTyped[scanentries.Response](context.Background(), reg, constants.BUS_CORE_AUDIT_SCAN_ENTRIES, &scanentries.Request{})
	if !errors.Is(err, bus.ErrBadRequestType) {
		t.Fatalf("err=%v", err)
	}

	_, err = bus.CallTyped[scanentries.Response](context.Background(), reg, constants.BUS_CORE_AUDIT_SCAN_ENTRIES, scanentries.Request{})
	if !errors.Is(err, errs.ErrInvalid) {
		t.Fatalf("err=%v", err)
	}

	defer func() {
		r := recover()
		if r == nil || !strings.Contains(r.(string), "registered twice") {
			t.Fatalf("recover=%v", r)
		}
	}()
	Init(deps)
}
