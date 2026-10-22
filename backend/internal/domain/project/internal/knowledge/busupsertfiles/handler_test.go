package busupsertfiles

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"grimoire/internal/infra/bus"
	"grimoire/lib/common/errs"
	"grimoire/lib/constants"
	"grimoire/lib/contracts/project/knowledge/upsertfiles"
)

func newReg(uc *Usecase) *bus.Registry {
	reg := bus.NewRegistry(slog.New(slog.NewTextHandler(io.Discard, nil)))
	h := &Handler{uc: uc}
	reg.Register(constants.BUS_PROJECT_KNOWLEDGE_UPSERT_FILES, h.Handle)
	return reg
}

// Request harus berupa nilai upsertfiles.Request; pointer/tipe lain ditolak tanpa menyentuh repo.
func TestHandler_BadType(t *testing.T) {
	u, r, a, _ := setup(lim)
	reg := newReg(u)
	for _, rq := range []any{struct{}{}, "x", &upsertfiles.Request{}} {
		_, err := bus.CallTyped[upsertfiles.Response](context.Background(), reg, constants.BUS_PROJECT_KNOWLEDGE_UPSERT_FILES, rq)
		if !errors.Is(err, bus.ErrBadRequestType) {
			t.Fatalf("%T: %v", rq, err)
		}
	}
	if r.inTxN != 0 || len(a.events) != 0 {
		t.Fatal("repo/audit tidak boleh tersentuh")
	}
}

func TestHandler_Success(t *testing.T) {
	u, r, a, _ := setup(lim)
	reg := newReg(u)
	res, err := bus.CallTyped[upsertfiles.Response](context.Background(), reg, constants.BUS_PROJECT_KNOWLEDGE_UPSERT_FILES,
		req("guides", item(0, "a.md", "# A")))
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Items) != 1 || res.Items[0].Path != "guides/a.md" || res.Items[0].Status != "created" {
		t.Fatalf("%+v", res)
	}
	if len(r.inserted) != 1 || len(a.events) != 1 {
		t.Fatalf("inserted=%d audit=%d", len(r.inserted), len(a.events))
	}
}

func TestHandler_ForwardsValidationError(t *testing.T) {
	u, _, _, _ := setup(lim)
	reg := newReg(u)
	rq := req("", item(0, "a.md", "# A"))
	rq.ProjectID = "bukan-uuid"
	_, err := bus.CallTyped[upsertfiles.Response](context.Background(), reg, constants.BUS_PROJECT_KNOWLEDGE_UPSERT_FILES, rq)
	if !errors.Is(err, errs.ErrInvalid) {
		t.Fatalf("got %v", err)
	}
}

func TestHandler_NotRegisteredWithoutInit(t *testing.T) {
	reg := bus.NewRegistry(slog.New(slog.NewTextHandler(io.Discard, nil)))
	_, err := reg.Call(context.Background(), constants.BUS_PROJECT_KNOWLEDGE_UPSERT_FILES, upsertfiles.Request{})
	if !errors.Is(err, bus.ErrNotRegistered) {
		t.Fatalf("got %v", err)
	}
}
