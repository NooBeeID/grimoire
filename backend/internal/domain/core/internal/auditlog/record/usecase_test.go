package record

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"grimoire/internal/domain/core/internal/auditlog/base"
	"grimoire/lib/common/errs"
	"grimoire/lib/common/reqctx"
	"grimoire/lib/contracts/core/audit/record"
)

const (
	uidActor   = "0192f0c4-7a1e-7c3a-9b1d-3c4e5f6a7b8c"
	uidProject = "0192f0c4-0000-7000-8000-000000000001"
	fixedID    = "0192f0c4-9999-7000-8000-000000000009"
)

var fixedNow = time.Date(2026, 10, 21, 9, 0, 0, 0, time.FixedZone("X", 7*3600))

type fakeRepo struct {
	calls int
	last  base.Entry
	err   error
	block bool
	dl    time.Time
	hasDL bool
}

func (f *fakeRepo) Insert(ctx context.Context, e base.Entry) error {
	f.calls++
	f.last = e
	f.dl, f.hasDL = ctx.Deadline()
	if f.block {
		<-ctx.Done()
		return ctx.Err()
	}
	return f.err
}

func newUC(r Repository) *Usecase {
	return NewUsecase(r, func() time.Time { return fixedNow }, func() string { return fixedID })
}

func validReq() record.Request {
	return record.Request{
		ActorType: record.ActorAPIKey, ActorID: uidActor, ActorLabel: "claude-code-budi (gk_ab12)",
		ProjectID: uidProject, ProjectLabel: "Payments", Action: record.ActionMCPReadFile,
		TargetType: "file", Target: "docs/auth/login.md",
		Args:   map[string]any{"path": "docs/auth/login.md"},
		Result: map[string]any{"bytes": 4812},
		Status: record.StatusOK, IP: "203.0.113.5", UserAgent: "claude-code/2.1",
	}
}

func TestRecord_Success(t *testing.T) {
	r := &fakeRepo{}
	resp, err := newUC(r).Execute(context.Background(), validReq())
	if err != nil || !resp.Recorded {
		t.Fatalf("resp=%v err=%v", resp, err)
	}
	if r.calls != 1 || r.last.ID != fixedID || !r.last.OccurredAt.Equal(fixedNow) || r.last.OccurredAt.Location() != time.UTC {
		t.Fatalf("entry: %+v", r.last)
	}
	if r.last.Action != "mcp.read_file" || r.last.Target != "docs/auth/login.md" || r.last.ActorID != uidActor {
		t.Fatalf("entry: %+v", r.last)
	}
}

func TestRecord_TraceIDFromContext(t *testing.T) {
	ctx := reqctx.WithTraceID(context.Background(), "t-123")
	r := &fakeRepo{}
	if _, err := newUC(r).Execute(ctx, validReq()); err != nil || r.last.TraceID != "t-123" {
		t.Fatalf("trace=%q err=%v", r.last.TraceID, err)
	}
	req := validReq()
	req.TraceID = "explicit"
	if _, err := newUC(r).Execute(ctx, req); err != nil || r.last.TraceID != "explicit" {
		t.Fatalf("trace=%q err=%v", r.last.TraceID, err)
	}
}

func TestT02_AC4_PasswordRedacted(t *testing.T) {
	r := &fakeRepo{}
	req := validReq()
	req.Args = map[string]any{"password": "hunter2", "path": "a.md"}
	if _, err := newUC(r).Execute(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	if r.last.Args["password"] != "[REDACTED]" || r.last.Args["path"] != "a.md" {
		t.Fatalf("args: %v", r.last.Args)
	}
	b, _ := json.Marshal(r.last.Args)
	if strings.Contains(string(b), "hunter2") {
		t.Fatal("password leaked")
	}
}

func TestT02_AC4_ContentOmitted(t *testing.T) {
	r := &fakeRepo{}
	req := validReq()
	req.Result = map[string]any{"content": "isi file rahasia", "bytes": 12}
	req.Args = map[string]any{"content": "isi file rahasia"}
	if _, err := newUC(r).Execute(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	if r.last.Result["content"] != "[OMITTED]" || r.last.Args["content"] != "[OMITTED]" || r.last.Result["bytes"] != 12 {
		t.Fatalf("result=%v args=%v", r.last.Result, r.last.Args)
	}
	b, _ := json.Marshal([]any{r.last.Args, r.last.Result})
	if strings.Contains(string(b), "isi file rahasia") {
		t.Fatal("content leaked")
	}
}

func TestRecord_LongStringTruncated(t *testing.T) {
	r := &fakeRepo{}
	req := validReq()
	req.Args = map[string]any{"q": strings.Repeat("é", 1500)} // 3000 byte
	if _, err := newUC(r).Execute(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	s, _ := r.last.Args["q"].(string)
	if !strings.HasSuffix(s, "…[truncated]") || !utf8.ValidString(s) || len(s) >= 3000 {
		t.Fatalf("len=%d", len(s))
	}
}

func TestRecord_ArgsOver8KBReplacedByMarker(t *testing.T) {
	r := &fakeRepo{}
	req := validReq()
	args := map[string]any{}
	for i := 0; i < 10; i++ {
		args[string(rune('a'+i))] = strings.Repeat("x", 1500)
	}
	req.Args = args
	if _, err := newUC(r).Execute(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	if r.last.Args["_truncated"] != true {
		t.Fatalf("args: %v", r.last.Args)
	}
	if n, ok := r.last.Args["_original_bytes"].(int); !ok || n <= 8192 {
		t.Fatalf("original_bytes: %v", r.last.Args["_original_bytes"])
	}
	if _, ok := r.last.Args["_keys"]; !ok {
		t.Fatal("no _keys")
	}
}

func TestT02_AC2_AnonymousWithKeyPrefixAccepted(t *testing.T) {
	r := &fakeRepo{}
	req := record.Request{ActorType: record.ActorAnonymous, ActorLabel: "gk_ab12", Action: record.ActionMCPAuthRejected, Status: record.StatusDenied}
	resp, err := newUC(r).Execute(context.Background(), req)
	if err != nil || !resp.Recorded || r.last.ActorLabel != "gk_ab12" || r.last.ActorID != "" {
		t.Fatalf("resp=%v err=%v entry=%+v", resp, err, r.last)
	}
}

func TestT02_AC2_AnonymousWithActorIDRejected(t *testing.T) {
	r := &fakeRepo{}
	req := record.Request{ActorType: record.ActorAnonymous, ActorID: uidActor, ActorLabel: "gk_ab12", Action: record.ActionMCPAuthRejected, Status: record.StatusDenied}
	resp, err := newUC(r).Execute(context.Background(), req)
	if !errors.Is(err, errs.ErrInvalid) || resp.Recorded || r.calls != 0 {
		t.Fatalf("resp=%v err=%v calls=%d", resp, err, r.calls)
	}
}

func TestRecord_InvalidRejected(t *testing.T) {
	mut := map[string]func(*record.Request){
		"empty action":       func(q *record.Request) { q.Action = "" },
		"uppercase action":   func(q *record.Request) { q.Action = "MCP.read" },
		"no dot action":      func(q *record.Request) { q.Action = "read_file" },
		"bad actor type":     func(q *record.Request) { q.ActorType = "admin" },
		"user without actor": func(q *record.Request) { q.ActorType = record.ActorUser; q.ActorID = "" },
		"bad status":         func(q *record.Request) { q.Status = "failed" },
		"bad project id":     func(q *record.Request) { q.ProjectID = "x" },
		"bad id":             func(q *record.Request) { q.ID = "abc" },
	}
	for name, f := range mut {
		t.Run(name, func(t *testing.T) {
			r := &fakeRepo{}
			req := validReq()
			req.Args = map[string]any{"password": "SECRET-VALUE"}
			f(&req)
			resp, err := newUC(r).Execute(context.Background(), req)
			if !errors.Is(err, errs.ErrInvalid) || resp.Recorded || r.calls != 0 {
				t.Fatalf("resp=%v err=%v calls=%d", resp, err, r.calls)
			}
			if strings.Contains(err.Error(), "SECRET-VALUE") {
				t.Fatal("error leaks input")
			}
		})
	}
}

func TestT02_AC8_RepoErrorReturned(t *testing.T) {
	sentinel := errors.New("conn refused")
	r := &fakeRepo{err: sentinel}
	var resp record.Response
	var err error
	func() {
		defer func() {
			if p := recover(); p != nil {
				t.Fatalf("panic: %v", p)
			}
		}()
		resp, err = newUC(r).Execute(context.Background(), validReq())
	}()
	if resp.Recorded || !errors.Is(err, sentinel) || !strings.HasPrefix(err.Error(), "audit record: ") {
		t.Fatalf("resp=%v err=%v", resp, err)
	}
}

func TestT02_AC8_RepoTimeout(t *testing.T) {
	old := insertTimeout
	insertTimeout = 20 * time.Millisecond
	defer func() { insertTimeout = old }()
	r := &fakeRepo{block: true}
	start := time.Now()
	resp, err := newUC(r).Execute(context.Background(), validReq())
	if !errors.Is(err, context.DeadlineExceeded) || resp.Recorded {
		t.Fatalf("resp=%v err=%v", resp, err)
	}
	if time.Since(start) > 2*time.Second || !r.hasDL {
		t.Fatalf("elapsed=%v hasDeadline=%v", time.Since(start), r.hasDL)
	}
}

func TestInsertTimeout_Default(t *testing.T) {
	if insertTimeout != 2*time.Second {
		t.Fatalf("insertTimeout=%v", insertTimeout)
	}
	r := &fakeRepo{}
	if _, err := newUC(r).Execute(context.Background(), validReq()); err != nil {
		t.Fatal(err)
	}
	if !r.hasDL || time.Until(r.dl) > 2*time.Second {
		t.Fatalf("deadline: %v %v", r.hasDL, time.Until(r.dl))
	}
}

func TestRecord_NoLogOnFailure(t *testing.T) {
	for _, f := range reflect.VisibleFields(reflect.TypeFor[Usecase]()) {
		if f.Name == "log" || f.Name == "logger" {
			t.Fatalf("Usecase must not hold a logger: %s", f.Name)
		}
	}
}
