package listfiles

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"grimoire/internal/infra/bus"
	"grimoire/lib/common/errs"
	"grimoire/lib/constants"
	"grimoire/lib/contracts/discovery/mcpgateway/toolkit"
	"grimoire/lib/contracts/project/knowledge/listfiles"
)

type busRec struct {
	calls int
	req   listfiles.Request
}

func newTestHandler(t *testing.T, fn func(listfiles.Request) (listfiles.Response, error)) (toolkit.Handler, *busRec) {
	t.Helper()
	reg := bus.NewRegistry(nil)
	rec := &busRec{}
	if fn != nil {
		reg.Register(constants.BUS_PROJECT_KNOWLEDGE_LIST_FILES, func(_ context.Context, req any) (any, error) {
			r, err := bus.Expect[listfiles.Request](req)
			if err != nil {
				return nil, err
			}
			rec.calls++
			rec.req = r
			return fn(r)
		})
	}
	return newHandler(newUsecase(newRepository(reg))), rec
}

func okResp(listfiles.Request) (listfiles.Response, error) {
	return listfiles.Response{
		Items:   []listfiles.Item{{Path: "readme.md", SizeBytes: 100, UpdatedAt: time.Date(2026, 10, 20, 8, 15, 0, 0, time.UTC)}},
		Folders: []string{"docs"},
		Total:   1,
	}, nil
}

func TestT08_AC5_ProjectFromCallOnly(t *testing.T) {
	h, rec := newTestHandler(t, okResp)
	res, err := h(context.Background(), toolkit.Call{ProjectID: "A", Arguments: json.RawMessage(`{"project_id":"B"}`)})
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError || !strings.Contains(res.Text, "ditentukan oleh API key") || res.Summary["error_code"] != "invalid_argument" || rec.calls != 0 {
		t.Fatalf("res=%+v calls=%d", res, rec.calls)
	}
	res, err = h(context.Background(), toolkit.Call{ProjectID: "A", Arguments: json.RawMessage(`{}`)})
	if err != nil || res.IsError {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	if rec.calls != 1 || rec.req.ProjectID != "A" {
		t.Fatalf("calls=%d req=%+v", rec.calls, rec.req)
	}
}

func TestHandler_ArgErrorSkipsBus(t *testing.T) {
	h, rec := newTestHandler(t, okResp)
	res, err := h(context.Background(), toolkit.Call{ProjectID: "A", Arguments: json.RawMessage(`{"limit":900}`)})
	if err != nil || !res.IsError || res.Summary["field"] != "limit" || rec.calls != 0 {
		t.Fatalf("res=%+v err=%v calls=%d", res, err, rec.calls)
	}
}

func TestHandler_Success(t *testing.T) {
	h, rec := newTestHandler(t, okResp)
	res, err := h(context.Background(), toolkit.Call{ProjectID: "A", Arguments: json.RawMessage(`{"path":"/x/"}`)})
	if err != nil || res.IsError {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	if !strings.Contains(res.Text, "readme.md") || !strings.Contains(res.Text, "[folder] docs/") {
		t.Fatalf("text: %s", res.Text)
	}
	if res.Structured["total"] != 1 || res.TargetType != "folder" || res.Target != "x" {
		t.Fatalf("res=%+v", res)
	}
	if rec.req.Prefix != "x" || rec.req.Limit != 200 || rec.req.Offset != 0 || rec.req.Recursive {
		t.Fatalf("req=%+v", rec.req)
	}
}

func TestHandler_ErrInvalidBecomesIsError(t *testing.T) {
	h, _ := newTestHandler(t, func(listfiles.Request) (listfiles.Response, error) {
		return listfiles.Response{}, fmt.Errorf("x: %w", errs.ErrInvalid)
	})
	res, err := h(context.Background(), toolkit.Call{ProjectID: "A", Arguments: json.RawMessage(`{}`)})
	if err != nil || !res.IsError {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	if !strings.Contains(res.Text, "Permintaan ditolak:") || !strings.Contains(res.Text, hintInvalid) || res.Summary["error_code"] != "invalid_request" {
		t.Fatalf("res=%+v", res)
	}
}

func TestHandler_OtherErrorBecomesGoError(t *testing.T) {
	cause := errors.New("db down")
	h, _ := newTestHandler(t, func(listfiles.Request) (listfiles.Response, error) { return listfiles.Response{}, cause })
	_, err := h(context.Background(), toolkit.Call{ProjectID: "A", Arguments: json.RawMessage(`{}`)})
	if !errors.Is(err, cause) {
		t.Fatalf("err=%v", err)
	}
}

func TestHandler_BusNotRegisteredIsGoError(t *testing.T) {
	h, _ := newTestHandler(t, nil)
	_, err := h(context.Background(), toolkit.Call{ProjectID: "A", Arguments: json.RawMessage(`{}`)})
	if !errors.Is(err, bus.ErrNotRegistered) {
		t.Fatalf("err=%v", err)
	}
}

func TestHandler_ContextDeadlinePreserved(t *testing.T) {
	h, _ := newTestHandler(t, func(listfiles.Request) (listfiles.Response, error) {
		return listfiles.Response{}, context.DeadlineExceeded
	})
	_, err := h(context.Background(), toolkit.Call{ProjectID: "A", Arguments: json.RawMessage(`{}`)})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err=%v", err)
	}
}

func TestHandler_ResultMetadataOnly(t *testing.T) {
	h, _ := newTestHandler(t, func(listfiles.Request) (listfiles.Response, error) {
		return listfiles.Response{
			Items: []listfiles.Item{{Path: "SECRET-PATH-XYZ", SizeBytes: 1, UpdatedAt: time.Now()}},
			Total: 1,
		}, nil
	})
	res, err := h(context.Background(), toolkit.Call{ProjectID: "A", Arguments: json.RawMessage(`{}`)})
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range res.Summary {
		switch v.(type) {
		case int, bool:
		default:
			t.Fatalf("summary[%s] bertipe %T", k, v)
		}
	}
	if b, _ := json.Marshal(res.Summary); strings.Contains(string(b), "SECRET-PATH-XYZ") {
		t.Fatal("summary membocorkan path")
	}
	for k := range res.Structured {
		switch k {
		case "items", "folders", "total", "next_offset", "recursive":
		default:
			t.Fatalf("kunci structured tak dikenal: %s", k)
		}
	}
}

func TestHandler_OffsetOutOfRange(t *testing.T) {
	h, rec := newTestHandler(t, func(listfiles.Request) (listfiles.Response, error) {
		return listfiles.Response{Total: 3}, nil
	})
	res, err := h(context.Background(), toolkit.Call{ProjectID: "A", Arguments: json.RawMessage(`{"offset":5}`)})
	if err != nil || !res.IsError || res.Summary["error_code"] != "offset_out_of_range" || rec.calls != 1 {
		t.Fatalf("res=%+v err=%v calls=%d", res, err, rec.calls)
	}
}
