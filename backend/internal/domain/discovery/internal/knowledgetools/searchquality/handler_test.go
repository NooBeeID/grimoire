package searchquality

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"grimoire/internal/infra/bus"
	"grimoire/lib/common/errs"
	"grimoire/lib/constants"
	"grimoire/lib/contracts/discovery/knowledgetools/searchquality"
)

type fakeReporter struct {
	calls int
	resp  searchquality.Response
}

func (f *fakeReporter) Report(context.Context, searchquality.Request) (searchquality.Response, error) {
	f.calls++
	return f.resp, nil
}

func newBus(uc reporter) *bus.Registry {
	b := bus.NewRegistry(slog.New(slog.NewTextHandler(io.Discard, nil)))
	b.Register(constants.BUS_DISCOVERY_KNOWLEDGETOOLS_SEARCH_QUALITY_REPORT, (&handler{uc: uc}).handle)
	return b
}

func TestHandler_PassesResponse(t *testing.T) {
	fr := &fakeReporter{resp: searchquality.Response{Report: searchquality.Report{ProjectID: pid, Searches: 7}}}
	res, err := bus.CallTyped[searchquality.Response](context.Background(), newBus(fr),
		constants.BUS_DISCOVERY_KNOWLEDGETOOLS_SEARCH_QUALITY_REPORT, okReq())
	if err != nil || res.Report.Searches != 7 || fr.calls != 1 {
		t.Fatalf("res=%+v err=%v", res, err)
	}
}

func TestHandler_WrongType(t *testing.T) {
	fr := &fakeReporter{}
	b := newBus(fr)
	for _, in := range []any{"string-salah", &searchquality.Request{}} {
		if _, err := b.Call(context.Background(), constants.BUS_DISCOVERY_KNOWLEDGETOOLS_SEARCH_QUALITY_REPORT, in); !errors.Is(err, bus.ErrBadRequestType) {
			t.Fatalf("in=%T err=%v", in, err)
		}
	}
	if fr.calls != 0 {
		t.Fatal("usecase terpanggil")
	}
}

func TestHandler_InvalidRequestViaBus(t *testing.T) {
	b := newBus(newUC(&fakeScanner{}))
	_, err := bus.CallTyped[searchquality.Response](context.Background(), b,
		constants.BUS_DISCOVERY_KNOWLEDGETOOLS_SEARCH_QUALITY_REPORT, searchquality.Request{ProjectID: "x"})
	var e *errs.Error
	if !errors.As(err, &e) || e.Code != errs.CodeInvalid {
		t.Fatalf("err=%v", err)
	}
}
