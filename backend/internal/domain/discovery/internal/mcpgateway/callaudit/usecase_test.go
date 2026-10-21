package callaudit

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"grimoire/internal/domain/discovery/internal/mcpgateway/base"
	"grimoire/internal/domain/discovery/internal/mcpgateway/base/auditwriter"
	"grimoire/internal/infra/bus"
	"grimoire/lib/common/reqctx"
	"grimoire/lib/constants"
	"grimoire/lib/contracts/core/audit/record"
	"grimoire/lib/contracts/discovery/mcpgateway/toolkit"
)

const secret = "SECRET-CONTENT-123"

type fakeWriter struct {
	mu      sync.Mutex
	entries []record.Request
}

func (f *fakeWriter) Write(_ context.Context, r record.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.entries = append(f.entries, r)
}

func (f *fakeWriter) all() []record.Request {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]record.Request(nil), f.entries...)
}

var principal = base.Principal{
	KeyID: "k-1", KeyName: "claude-laptop", KeyPrefix: "a1b2c3d4",
	ProjectID: "P-A", ProjectName: "Payments", IP: "10.0.0.7", UserAgent: "claude-code/2.1.0",
}

var schema = json.RawMessage(`{"type":"object"}`)

func tool(name string, h toolkit.Handler) toolkit.Tool {
	return toolkit.Tool{Name: name, InputSchema: schema, Handler: h}
}

func newUC(t *testing.T, tools []toolkit.Tool, opt Options) (*Usecase, *fakeWriter, *bytes.Buffer) {
	t.Helper()
	cat, err := base.NewCatalog(tools)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	fw := &fakeWriter{}
	return NewUsecase(cat, fw, slog.New(slog.NewTextHandler(&buf, nil)), opt), fw, &buf
}

func okH(res toolkit.Result) toolkit.Handler {
	return func(context.Context, toolkit.Call) (toolkit.Result, error) { return res, nil }
}

func one(t *testing.T, fw *fakeWriter) record.Request {
	t.Helper()
	e := fw.all()
	if len(e) != 1 {
		t.Fatalf("entri audit = %d, want 1", len(e))
	}
	return e[0]
}

func TestT07_AC4_ExactlyOneAuditPerCall(t *testing.T) {
	block := make(chan struct{})
	defer close(block)
	tests := []struct {
		name       string
		h          toolkit.Handler
		callName   string
		wantAction string
		wantStatus string
		wantCode   string
		wantUnk    bool
	}{
		{"sukses", okH(toolkit.Result{Text: secret, Target: "docs/a.md", TargetType: "file", Summary: map[string]any{"count": 3}}), "read_file", "mcp.read_file", record.StatusOK, "", false},
		{"tool iserror", okH(toolkit.Result{Text: "nope", IsError: true}), "read_file", "mcp.read_file", record.StatusError, "", false},
		{"error go", func(context.Context, toolkit.Call) (toolkit.Result, error) {
			return toolkit.Result{Text: secret, Target: secret}, errors.New(secret)
		}, "read_file", "mcp.read_file", record.StatusError, "internal", false},
		{"timeout buta", func(context.Context, toolkit.Call) (toolkit.Result, error) {
			<-block
			return toolkit.Result{}, nil
		}, "read_file", "mcp.read_file", record.StatusError, "timeout", false},
		{"timeout kooperatif", func(ctx context.Context, _ toolkit.Call) (toolkit.Result, error) {
			<-ctx.Done()
			return toolkit.Result{}, fmt.Errorf("wrap: %w", ctx.Err())
		}, "read_file", "mcp.read_file", record.StatusError, "timeout", false},
		{"panic", func(context.Context, toolkit.Call) (toolkit.Result, error) { panic(secret) }, "read_file", "mcp.read_file", record.StatusError, "panic", false},
		{"unknown", okH(toolkit.Result{}), "write_file", "mcp.unknown_tool", record.StatusError, "", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			uc, fw, logs := newUC(t, []toolkit.Tool{tool("read_file", tc.h)}, Options{Timeout: 50 * time.Millisecond})
			begin := time.Now()
			out := uc.Invoke(context.Background(), principal, tc.callName, json.RawMessage(`{"path":"x"}`))
			if time.Since(begin) > time.Second {
				t.Fatal("terlalu lama")
			}
			e := one(t, fw)
			if e.Action != tc.wantAction || e.Status != tc.wantStatus {
				t.Fatalf("action/status = %s/%s", e.Action, e.Status)
			}
			if out.UnknownTool != tc.wantUnk || out.ErrorCode != tc.wantCode {
				t.Fatalf("outcome = %+v", out)
			}
			if tc.wantCode != "" && e.Result["error_code"] != tc.wantCode {
				t.Fatalf("audit error_code = %v", e.Result["error_code"])
			}
			if strings.Contains(fmt.Sprintf("%#v", e), secret) {
				t.Fatal("secret bocor ke audit")
			}
			if tc.wantCode != "" {
				if strings.Contains(out.Result.Text, secret) {
					t.Fatal("secret bocor ke respons")
				}
				if !out.Result.IsError {
					t.Fatal("harus IsError")
				}
			}
			if tc.wantCode == "internal" && !strings.Contains(out.Result.Text, "trace_id=") {
				t.Fatal(out.Result.Text)
			}
			if tc.wantCode == "panic" && (!strings.Contains(logs.String(), "mcp tool panic") || !strings.Contains(logs.String(), "stack")) {
				t.Fatal(logs.String())
			}
			if tc.wantUnk {
				if e.Args["name"] != "write_file" || e.Result["error_code"] != "unknown_tool" {
					t.Fatalf("%+v", e)
				}
			}
			if tc.name == "sukses" {
				if e.Target != "docs/a.md" || e.TargetType != "file" || e.Result["count"] != 3 ||
					e.Result["bytes"] != len(secret) || e.Result["truncated"] != false {
					t.Fatalf("%+v", e)
				}
			}
		})
	}
}

func TestT07_AC4_NoTextInAudit(t *testing.T) {
	uc, fw, _ := newUC(t, []toolkit.Tool{tool("read_file", okH(toolkit.Result{Text: "abcde", Summary: map[string]any{"bytes": 1, "count": 2}}))}, Options{})
	uc.Invoke(context.Background(), principal, "read_file", nil)
	e := one(t, fw)
	if e.Result["bytes"] != 5 || e.Result["count"] != 2 {
		t.Fatalf("%v", e.Result)
	}
}

func bigTool(text string) toolkit.Tool {
	return tool("read_file", okH(toolkit.Result{Text: text}))
}

func TestT07_AC7_Truncate500KB(t *testing.T) {
	t.Run("ascii", func(t *testing.T) {
		uc, fw, _ := newUC(t, []toolkit.Tool{bigTool(strings.Repeat("a", 512000))}, Options{MaxBytes: 102400})
		out := uc.Invoke(context.Background(), principal, "read_file", nil)
		if len(out.Result.Text) > 102400 || !strings.Contains(out.Result.Text, "[truncated:") || !out.Truncated || out.OriginalBytes != 512000 {
			t.Fatalf("len=%d %v %d", len(out.Result.Text), out.Truncated, out.OriginalBytes)
		}
		e := one(t, fw)
		if e.Result["bytes"] != len(out.Result.Text) || e.Result["original_bytes"] != 512000 || e.Result["truncated"] != true {
			t.Fatalf("%v", e.Result)
		}
	})
	t.Run("multibyte", func(t *testing.T) {
		txt := strings.Repeat("😀ก", 500*1024/7)
		uc, _, _ := newUC(t, []toolkit.Tool{bigTool(txt)}, Options{MaxBytes: 102401})
		out := uc.Invoke(context.Background(), principal, "read_file", nil)
		if !utf8.ValidString(out.Result.Text) || len(out.Result.Text) > 102401 || !out.Truncated {
			t.Fatal("tidak valid")
		}
	})
}

func TestT07_AC7_NotTruncatedWithinLimit(t *testing.T) {
	uc, _, _ := newUC(t, []toolkit.Tool{bigTool(strings.Repeat("a", 1024))}, Options{MaxBytes: 1024})
	out := uc.Invoke(context.Background(), principal, "read_file", nil)
	if out.Truncated || len(out.Result.Text) != 1024 || out.OriginalBytes != 1024 {
		t.Fatalf("%+v", out.Truncated)
	}
}

func TestT07_AC3_ProjectIDFromPrincipal(t *testing.T) {
	var got toolkit.Call
	h := func(_ context.Context, c toolkit.Call) (toolkit.Result, error) {
		got = c
		return toolkit.Result{Text: "ok"}, nil
	}
	uc, fw, _ := newUC(t, []toolkit.Tool{tool("read_file", h)}, Options{})
	in := `{"path":"x","project_id":"P-B"}`
	uc.Invoke(context.Background(), principal, "read_file", json.RawMessage(in))
	if got.ProjectID != "P-A" || string(got.Arguments) != in {
		t.Fatalf("%+v", got)
	}
	e := one(t, fw)
	if e.ProjectID != "P-A" || e.ProjectLabel != "Payments" {
		t.Fatalf("%+v", e)
	}
	for _, raw := range []string{"", "null", "[1]", "  "} {
		uc.Invoke(context.Background(), principal, "read_file", json.RawMessage(raw))
		if string(got.Arguments) != "{}" {
			t.Fatalf("raw %q -> %s", raw, got.Arguments)
		}
	}
	uc.Invoke(context.Background(), principal, "read_file", json.RawMessage(`[1]`))
	es := fw.all()
	if es[len(es)-1].Args["_invalid"] != true {
		t.Fatalf("%v", es[len(es)-1].Args)
	}
}

func TestT07_ToolTimeoutUsesConfiguredDuration(t *testing.T) {
	sleep := func(d time.Duration) toolkit.Handler {
		return func(context.Context, toolkit.Call) (toolkit.Result, error) {
			time.Sleep(d)
			return toolkit.Result{Text: "done"}, nil
		}
	}
	uc, _, _ := newUC(t, []toolkit.Tool{tool("read_file", sleep(200*time.Millisecond))}, Options{Timeout: 30 * time.Millisecond})
	out := uc.Invoke(context.Background(), principal, "read_file", nil)
	if out.ErrorCode != "timeout" || !strings.Contains(out.Result.Text, "30ms") {
		t.Fatalf("%+v", out)
	}
	uc, _, _ = newUC(t, []toolkit.Tool{tool("read_file", sleep(20*time.Millisecond))}, Options{Timeout: 500 * time.Millisecond})
	out = uc.Invoke(context.Background(), principal, "read_file", nil)
	if out.ErrorCode != "" || out.Result.Text != "done" {
		t.Fatalf("%+v", out)
	}
}

func TestT07_PanicRecovered_ServerSurvives(t *testing.T) {
	uc, fw, _ := newUC(t, []toolkit.Tool{
		tool("read_file", func(context.Context, toolkit.Call) (toolkit.Result, error) { panic("boom") }),
		tool("list_files", okH(toolkit.Result{Text: "ok"})),
	}, Options{})
	if out := uc.Invoke(context.Background(), principal, "read_file", nil); out.ErrorCode != "panic" {
		t.Fatal(out.ErrorCode)
	}
	if out := uc.Invoke(context.Background(), principal, "list_files", nil); out.Result.Text != "ok" {
		t.Fatal(out)
	}
	if len(fw.all()) != 2 {
		t.Fatal("audit")
	}
}

func TestT07_LateToolDoesNotBlockOrLeak(t *testing.T) {
	release := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	h := func(context.Context, toolkit.Call) (toolkit.Result, error) {
		defer wg.Done()
		<-release
		return toolkit.Result{Text: "late"}, nil
	}
	uc, _, _ := newUC(t, []toolkit.Tool{tool("read_file", h)}, Options{Timeout: 20 * time.Millisecond})
	if out := uc.Invoke(context.Background(), principal, "read_file", nil); out.ErrorCode != "timeout" {
		t.Fatal(out.ErrorCode)
	}
	close(release)
	finished := make(chan struct{})
	go func() { wg.Wait(); close(finished) }()
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("goroutine tool tidak selesai")
	}
}

func TestT07_AuditFailureFailOpen(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, nil))
	b := bus.NewRegistry(log)
	calls := 0
	b.Register(constants.BUS_CORE_AUDIT_RECORD, func(context.Context, any) (any, error) {
		calls++
		return nil, errors.New("db down")
	})
	cat, _ := base.NewCatalog([]toolkit.Tool{tool("read_file", okH(toolkit.Result{Text: secret}))})
	uc := NewUsecase(cat, auditwriter.New(b, log), log, Options{})
	out := uc.Invoke(context.Background(), principal, "read_file", json.RawMessage(`{"path":"secret-path"}`))
	if out.Result.Text != secret || out.Result.IsError {
		t.Fatalf("%+v", out)
	}
	logs := buf.String()
	if calls != 1 || !strings.Contains(logs, "audit write failed") || !strings.Contains(logs, "mcp.read_file") {
		t.Fatal(calls, logs)
	}
	if strings.Contains(logs, secret) || strings.Contains(logs, "secret-path") {
		t.Fatal(logs)
	}
}

func TestT07_AuditWrittenEvenIfClientCanceled(t *testing.T) {
	h := func(ctx context.Context, _ toolkit.Call) (toolkit.Result, error) {
		<-ctx.Done()
		return toolkit.Result{}, ctx.Err()
	}
	uc, fw, _ := newUC(t, []toolkit.Tool{tool("read_file", h)}, Options{})
	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(20 * time.Millisecond); cancel() }()
	out := uc.Invoke(ctx, principal, "read_file", nil)
	if out.ErrorCode != "internal" {
		t.Fatal(out.ErrorCode)
	}
	one(t, fw)
}

func TestT07_DurationMeasured(t *testing.T) {
	var n int
	base0 := time.Unix(1000, 0)
	clock := func() time.Time { n++; return base0.Add(time.Duration(n-1) * 1500 * time.Millisecond) }
	uc, fw, _ := newUC(t, []toolkit.Tool{tool("read_file", okH(toolkit.Result{Text: "x"}))}, Options{Now: clock})
	uc.Invoke(context.Background(), principal, "read_file", nil)
	if one(t, fw).Result["duration_ms"] != int64(1500) {
		t.Fatalf("%v", fw.all()[0].Result)
	}
	if n != 2 {
		t.Fatalf("Now dipanggil %d kali", n)
	}
	n = 0
	uc.Invoke(context.Background(), principal, "nope", nil)
	if fw.all()[1].Result["duration_ms"] != int64(1500) || n != 2 {
		t.Fatalf("%v n=%d", fw.all()[1].Result, n)
	}
}

func TestT07_UnknownToolNameTruncated(t *testing.T) {
	uc, fw, _ := newUC(t, nil, Options{})
	name := strings.Repeat("é世", 100)
	out := uc.Invoke(context.Background(), principal, name, nil)
	e := one(t, fw)
	got := e.Args["name"].(string)
	if !out.UnknownTool || len(got) > 128 || !utf8.ValidString(got) {
		t.Fatalf("%d", len(got))
	}
}

func TestT07_ActorFieldsFromPrincipal(t *testing.T) {
	uc, fw, _ := newUC(t, []toolkit.Tool{tool("read_file", okH(toolkit.Result{}))}, Options{})
	uc.Invoke(context.Background(), principal, "read_file", nil)
	e := one(t, fw)
	if e.ActorType != record.ActorAPIKey || e.ActorID != "k-1" || e.ActorLabel != "claude-laptop (a1b2c3d4)" ||
		e.IP != "10.0.0.7" || e.UserAgent != "claude-code/2.1.0" {
		t.Fatalf("%+v", e)
	}
}

func TestInternalMessageUsesTraceID(t *testing.T) {
	h := func(context.Context, toolkit.Call) (toolkit.Result, error) { return toolkit.Result{}, errors.New("x") }
	uc, _, _ := newUC(t, []toolkit.Tool{tool("read_file", h)}, Options{})
	out := uc.Invoke(reqctx.WithTraceID(context.Background(), "T123"), principal, "read_file", nil)
	if !strings.HasSuffix(out.Result.Text, "trace_id=T123") {
		t.Fatal(out.Result.Text)
	}
	out = uc.Invoke(context.Background(), principal, "read_file", nil)
	if !strings.HasSuffix(out.Result.Text, "trace_id=-") {
		t.Fatal(out.Result.Text)
	}
}

func TestNewUsecaseDefaults(t *testing.T) {
	uc := NewUsecase(base.Catalog{}, &fakeWriter{}, nil, Options{})
	if uc.opt.Timeout != 10*time.Second || uc.opt.MaxBytes != 102400 || uc.opt.Now == nil || uc.log == nil {
		t.Fatalf("%+v", uc.opt)
	}
	uc.Invoke(context.Background(), principal, "x", nil)
}

func TestT08_D1_StructuredForwardedNotAudited(t *testing.T) {
	st := map[string]any{"items": []any{map[string]any{"path": "docs/a.md"}}}
	uc, fw, _ := newUC(t, []toolkit.Tool{tool("read_file", okH(toolkit.Result{Text: "x", Structured: st, Summary: map[string]any{"count": 1}}))}, Options{})
	out := uc.Invoke(context.Background(), principal, "read_file", nil)
	if !reflect.DeepEqual(out.Result.Structured, st) {
		t.Fatalf("Structured tidak diteruskan: %v", out.Result.Structured)
	}
	e := one(t, fw)
	if _, ok := e.Result["items"]; ok {
		t.Fatalf("items masuk audit: %v", e.Result)
	}
	if _, ok := e.Result["structured"]; ok {
		t.Fatalf("structured masuk audit: %v", e.Result)
	}
	if strings.Contains(fmt.Sprint(e), "docs/a.md") {
		t.Fatalf("isi Structured bocor ke audit: %v", e)
	}
}
