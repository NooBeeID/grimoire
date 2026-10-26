package buslistchanges

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"grimoire/internal/infra/app"
	"grimoire/internal/infra/bus"
	"grimoire/lib/constants"
	"grimoire/lib/contracts/project/knowledge/listchanges"
)

func TestInit_RegistersEndpoint(t *testing.T) {
	reg := bus.NewRegistry(slog.New(slog.NewTextHandler(io.Discard, nil)))
	Init(app.Deps{Bus: reg})
	_, err := bus.CallTyped[listchanges.Response](context.Background(), reg, constants.BUS_PROJECT_KNOWLEDGE_LIST_CHANGES, "bukan-request")
	if !errors.Is(err, bus.ErrBadRequestType) {
		t.Fatalf("endpoint tidak terdaftar: %v", err)
	}
}
