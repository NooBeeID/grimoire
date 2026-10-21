package searchknowledge

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"

	"grimoire/internal/infra/app"
	"grimoire/internal/infra/bus"
	"grimoire/lib/common/errs"
	"grimoire/lib/constants"
	"grimoire/lib/contracts/discovery/mcpgateway/toolkit"
	"grimoire/lib/contracts/project/knowledge/search"
)

type fakeBus struct {
	reg   *bus.Registry
	reqs  []search.Request
	resp  any
	err   error
	calls int
}

func newFakeBus(register bool) *fakeBus {
	f := &fakeBus{reg: bus.NewRegistry(slog.New(slog.NewTextHandler(io.Discard, nil)))}
	f.resp = search.Response{Items: []search.Item{{Path: "docs/guides/b.md", Score: 0.6079, Snippet: "**rate** **limiting**"}}, Total: 1}
	if register {
		f.reg.Register(constants.BUS_PROJECT_KNOWLEDGE_SEARCH, func(_ context.Context, req any) (any, error) {
			f.calls++
			if r, ok := req.(search.Request); ok {
				f.reqs = append(f.reqs, r)
			}
			return f.resp, f.err
		})
	}
	return f
}

func (f *fakeBus) call(projectID, args string) (toolkit.Result, error) {
	h := Init(app.Deps{Bus: f.reg}).Handler
	return h(context.Background(), toolkit.Call{ProjectID: projectID, Arguments: json.RawMessage(args)})
}

func TestHandler_Success(t *testing.T) {
	f := newFakeBus(true)
	r, err := f.call("p1", `{"query":"rate limit","path":"/docs/","limit":5}`)
	want := search.Request{ProjectID: "p1", Query: "rate limit", Prefix: "docs", Limit: 5}
	if err != nil || r.IsError || len(f.reqs) != 1 || f.reqs[0] != want {
		t.Fatalf("%+v %v %+v", r, err, f.reqs)
	}
	if !strings.Contains(r.Text, "docs/guides/b.md") || !strings.Contains(r.Text, "(skor 0.608)") || r.Structured["total"] != 1 {
		t.Fatalf("%+v", r)
	}
	if r.TargetType != "folder" || r.Target != "docs" {
		t.Fatalf("audit target: %s %s", r.TargetType, r.Target)
	}
	if len(r.Summary) != 2 || r.Summary["count"] != 1 || r.Summary["total"] != 1 {
		t.Fatalf("summary: %v", r.Summary)
	}
	sb, _ := json.Marshal(r.Summary)
	if strings.Contains(string(sb), "rate") {
		t.Fatalf("summary bocor: %s", sb)
	}
}

func TestHandler_DefaultLimitSentExplicit(t *testing.T) {
	f := newFakeBus(true)
	if _, err := f.call("p1", `{"query":"a"}`); err != nil || f.reqs[0].Limit != 10 {
		t.Fatalf("%+v %v", f.reqs, err)
	}
}

func TestHandler_NoResults(t *testing.T) {
	f := newFakeBus(true)
	f.resp = search.Response{}
	r, err := f.call("p1", `{"query":"a"}`)
	if err != nil || r.IsError || !strings.Contains(r.Text, "Tidak ada hasil") || !strings.Contains(r.Text, "list_files") {
		t.Fatalf("%+v %v", r, err)
	}
}

func TestHandler_InvalidArgsDoNotCallBus(t *testing.T) {
	for _, args := range []string{`{}`, `{"query":"  "}`, `{"query":"a","limit":0}`, `{"query":"a","project_id":"x"}`, `{"query":"a","extra":1}`} {
		f := newFakeBus(true)
		r, err := f.call("A", args)
		if err != nil || !r.IsError || r.Summary["error_code"] != "invalid_argument" || f.calls != 0 {
			t.Fatalf("args=%s %+v %v calls=%d", args, r, err, f.calls)
		}
	}
}

func TestHandler_ProjectIDArgIgnored(t *testing.T) {
	f := newFakeBus(true)
	r, err := f.call("A", `{"query":"a","project_id":"B"}`)
	if err != nil || !r.IsError || f.calls != 0 || !strings.Contains(r.Text, "ditentukan oleh API key") {
		t.Fatalf("%+v %v", r, err)
	}
}

func TestHandler_ErrInvalidBecomesToolError(t *testing.T) {
	f := newFakeBus(true)
	f.err = errors.Join(errors.New("prefix tidak valid"), errs.ErrInvalid)
	r, err := f.call("p", `{"query":"a"}`)
	if err != nil || !r.IsError || !strings.HasPrefix(r.Text, "Permintaan ditolak:") ||
		!strings.HasSuffix(r.Text, "Periksa path atau hapus filter path.") || r.Summary["error_code"] != "invalid_request" {
		t.Fatalf("%+v %v", r, err)
	}
}

func TestHandler_GoErrors(t *testing.T) {
	boom := errors.New("boom")
	for name, tc := range map[string]struct {
		err  error
		want error
	}{
		"other":    {boom, boom},
		"deadline": {context.DeadlineExceeded, context.DeadlineExceeded},
		"badtype":  {bus.ErrBadRequestType, bus.ErrBadRequestType},
	} {
		f := newFakeBus(true)
		f.err = tc.err
		r, err := f.call("p", `{"query":"a"}`)
		if !errors.Is(err, tc.want) || r.Text != "" || r.IsError {
			t.Fatalf("%s: %+v %v", name, r, err)
		}
	}
}

func TestHandler_NotRegisteredIsGoError(t *testing.T) {
	f := newFakeBus(false)
	_, err := f.call("p", `{"query":"a"}`)
	if !errors.Is(err, bus.ErrNotRegistered) {
		t.Fatalf("%v", err)
	}
}
