package authenticate

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/time/rate"

	"grimoire/internal/domain/discovery/internal/mcpgateway/base"
	"grimoire/internal/infra/bus"
	"grimoire/internal/infra/middleware"
	"grimoire/lib/constants"
	"grimoire/lib/contracts/core/audit/record"
	"grimoire/lib/contracts/project/apikey/touch"
	"grimoire/lib/contracts/project/apikey/verify"
	"grimoire/lib/contracts/project/management/getproject"
	"net/http"
)

type fakeAudit struct {
	mu   sync.Mutex
	recs []record.Request
}

func (f *fakeAudit) Write(_ context.Context, r record.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.recs = append(f.recs, r)
}
func (f *fakeAudit) list() []record.Request {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]record.Request(nil), f.recs...)
}

type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fakeClock) now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *fakeClock) add(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

type env struct {
	uc          *Usecase
	audit       *fakeAudit
	logBuf      *bytes.Buffer
	verifyCalls atomic.Int32
	projCalls   atomic.Int32
	touches     chan string
	verifyFn    func(verify.Request) (verify.Response, error)
	projErr     error
	projFound   bool
	clock       *fakeClock
	b           *bus.Registry
}

const keyA = "gr_aaaaaaaa_secret"

func okVerify(id string) verify.Response {
	return verify.Response{Found: true, KeyID: id, KeyName: "laptop", Prefix: "aaaaaaaa", ProjectID: "p1"}
}

func newEnv(t *testing.T, ipBurst, keyBurst int, withTouch bool) *env {
	t.Helper()
	e := &env{audit: &fakeAudit{}, logBuf: &bytes.Buffer{}, touches: make(chan string, 16), projFound: true,
		clock: &fakeClock{t: time.Unix(1000, 0)}}
	log := slog.New(slog.NewTextHandler(e.logBuf, nil))
	e.b = bus.NewRegistry(log)
	e.verifyFn = func(r verify.Request) (verify.Response, error) {
		if r.RawKey == keyA {
			return okVerify("k1"), nil
		}
		return verify.Response{Reason: verify.ReasonUnknown, Prefix: "abcd1234"}, nil
	}
	e.b.Register(constants.BUS_PROJECT_APIKEY_VERIFY, func(_ context.Context, req any) (any, error) {
		e.verifyCalls.Add(1)
		return e.verifyFn(req.(verify.Request))
	})
	e.b.Register(constants.BUS_PROJECT_MANAGEMENT_GET_PROJECT, func(_ context.Context, req any) (any, error) {
		e.projCalls.Add(1)
		if e.projErr != nil {
			return nil, e.projErr
		}
		return getproject.Response{Found: e.projFound, Name: "Payments"}, nil
	})
	if withTouch {
		e.b.Register(constants.BUS_PROJECT_APIKEY_TOUCH, func(_ context.Context, req any) (any, error) {
			e.touches <- req.(touch.Request).KeyID
			return nil, nil
		})
	}
	mk := func(burst int) *middleware.Limiter {
		return middleware.NewLimiter(context.Background(), middleware.Rule{Name: "t", Rate: rate.Limit(0.001), Burst: burst,
			KeyFunc: func(*http.Request) string { return "" }})
	}
	e.uc = NewUsecase(e.b, mk(ipBurst), mk(keyBurst), base.NewAuditThrottle(time.Minute, e.clock.now), e.audit, log)
	return e
}

func (e *env) touchCount(t *testing.T, want int) {
	t.Helper()
	time.Sleep(50 * time.Millisecond)
	if got := len(e.touches); got != want {
		t.Fatalf("touch=%d want %d", got, want)
	}
}

func in(key string) Input { return Input{RawKey: key, IP: "10.0.0.7", UserAgent: "ua"} }

func TestT07_AC2_RejectedAudit(t *testing.T) {
	cases := []struct {
		name, key, reason, prefix string
		fn                        func(verify.Request) (verify.Response, error)
		verifyCalls               int32
	}{
		{"missing", "", "missing", "", nil, 0},
		{"unknown", "gr_zzzz", "unknown", "abcd1234", nil, 1},
		{"revoked", "gr_rev", "revoked", "abcd1234", func(verify.Request) (verify.Response, error) {
			return verify.Response{Reason: verify.ReasonRevoked, Prefix: "abcd1234"}, nil
		}, 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := newEnv(t, 10, 10, true)
			if c.fn != nil {
				e.verifyFn = c.fn
			}
			d := e.uc.Execute(context.Background(), in(c.key))
			if d.Kind != Unauthorized {
				t.Fatalf("kind %v", d.Kind)
			}
			if e.verifyCalls.Load() != c.verifyCalls {
				t.Fatalf("verify calls %d", e.verifyCalls.Load())
			}
			recs := e.audit.list()
			if len(recs) != 1 {
				t.Fatalf("audit %d", len(recs))
			}
			r := recs[0]
			if r.Action != record.ActionMCPAuthRejected || r.ActorType != record.ActorAnonymous || r.ActorID != "" ||
				r.ActorLabel != c.prefix || r.Status != record.StatusDenied || r.IP != "10.0.0.7" || r.UserAgent != "ua" ||
				r.Args["reason"] != c.reason || r.Args["key_prefix"] != c.prefix {
				t.Fatalf("%+v", r)
			}
			e.touchCount(t, 0)
		})
	}
}

func TestT07_DecisionHasNoReason(t *testing.T) {
	typ := reflect.TypeFor[Decision]()
	for i := 0; i < typ.NumField(); i++ {
		if strings.Contains(strings.ToLower(typ.Field(i).Name), "reason") {
			t.Fatal("Decision tidak boleh memuat Reason")
		}
	}
}

func TestT07_AC5_RevokeTakesEffectNextRequest(t *testing.T) {
	e := newEnv(t, 10, 10, true)
	var n atomic.Int32
	e.verifyFn = func(verify.Request) (verify.Response, error) {
		if n.Add(1) == 1 {
			return okVerify("k1"), nil
		}
		return verify.Response{Reason: verify.ReasonRevoked, Prefix: "aaaaaaaa"}, nil
	}
	if d := e.uc.Execute(context.Background(), in(keyA)); d.Kind != Admitted {
		t.Fatalf("1: %v", d.Kind)
	}
	if d := e.uc.Execute(context.Background(), in(keyA)); d.Kind != Unauthorized {
		t.Fatalf("2: %v", d.Kind)
	}
	if e.verifyCalls.Load() != 2 {
		t.Fatalf("verify calls %d", e.verifyCalls.Load())
	}
}

func TestT07_AC6_KeyRateLimit(t *testing.T) {
	e := newEnv(t, 10, 2, true)
	e.verifyFn = func(r verify.Request) (verify.Response, error) {
		if r.RawKey == "keyB" {
			return okVerify("k2"), nil
		}
		return okVerify("k1"), nil
	}
	for i := 0; i < 2; i++ {
		if d := e.uc.Execute(context.Background(), in(keyA)); d.Kind != Admitted {
			t.Fatalf("%d: %v", i, d.Kind)
		}
	}
	for i := 0; i < 4; i++ {
		d := e.uc.Execute(context.Background(), in(keyA))
		if d.Kind != RateLimited || d.RetryAfter < 1 {
			t.Fatalf("%d: %+v", i, d)
		}
	}
	recs := e.audit.list()
	if len(recs) != 1 {
		t.Fatalf("audit %d", len(recs))
	}
	r := recs[0]
	if r.Action != record.ActionMCPRateLimited || r.ActorType != record.ActorAPIKey || r.ActorID != "k1" ||
		r.Args["scope"] != "key" || r.Status != record.StatusDenied || r.ProjectLabel != "Payments" || r.Args["retry_after_seconds"] == nil {
		t.Fatalf("%+v", r)
	}
	e.clock.add(61 * time.Second)
	e.uc.Execute(context.Background(), in(keyA))
	if n := len(e.audit.list()); n != 2 {
		t.Fatalf("setelah 61s audit=%d", n)
	}
	if d := e.uc.Execute(context.Background(), in("keyB")); d.Kind != Admitted {
		t.Fatalf("key B: %v", d.Kind)
	}
}

func TestT07_IPPrecheckBlocksEvenWithValidKey(t *testing.T) {
	e := newEnv(t, 1, 10, true)
	if d := e.uc.Execute(context.Background(), in("bad")); d.Kind != Unauthorized {
		t.Fatalf("1: %v", d.Kind)
	}
	for i := 0; i < 5; i++ {
		d := e.uc.Execute(context.Background(), in(keyA))
		if d.Kind != RateLimited || d.RetryAfter < 1 {
			t.Fatalf("%d: %+v", i, d)
		}
	}
	if e.verifyCalls.Load() != 1 {
		t.Fatalf("verify %d", e.verifyCalls.Load())
	}
	recs := e.audit.list()
	if len(recs) != 2 { // 1 rejected + 1 rate_limited
		t.Fatalf("audit %d", len(recs))
	}
	r := recs[1]
	if r.Action != record.ActionMCPRateLimited || r.ActorType != record.ActorAnonymous || r.TargetType != "ip" || r.Args["scope"] != "auth_ip" {
		t.Fatalf("%+v", r)
	}
	if !strings.Contains(e.logBuf.String(), "audit throttled") {
		t.Fatal("log warn throttled tidak ada")
	}
	e.touchCount(t, 0)
}

func TestT07_VerifyErrorIs503(t *testing.T) {
	e := newEnv(t, 10, 10, true)
	e.verifyFn = func(verify.Request) (verify.Response, error) { return verify.Response{}, errors.New("db down") }
	d := e.uc.Execute(context.Background(), in(keyA))
	if d.Kind != Unavailable {
		t.Fatalf("%v", d.Kind)
	}
	if len(e.audit.list()) != 0 || e.projCalls.Load() != 0 {
		t.Fatal("tidak boleh ada audit / get-project")
	}
	out := e.logBuf.String()
	if !strings.Contains(out, "mcp verify failed") || !strings.Contains(out, "level=ERROR") || strings.Contains(out, keyA) {
		t.Fatal(out)
	}
	// token IP tidak terkonsumsi: burst 1 tetap lolos Peek berkali-kali
	e2 := newEnv(t, 1, 10, true)
	e2.verifyFn = e.verifyFn
	for i := 0; i < 3; i++ {
		if d := e2.uc.Execute(context.Background(), in(keyA)); d.Kind != Unavailable {
			t.Fatalf("%d: %v", i, d.Kind)
		}
	}
	e.touchCount(t, 0)
}

func TestT07_GetProjectFailureStillAdmits(t *testing.T) {
	for _, name := range []string{"error", "notfound"} {
		t.Run(name, func(t *testing.T) {
			e := newEnv(t, 10, 10, true)
			if name == "error" {
				e.projErr = errors.New("boom")
			} else {
				e.projFound = false
			}
			d := e.uc.Execute(context.Background(), in(keyA))
			if d.Kind != Admitted || d.Principal.ProjectName != "" {
				t.Fatalf("%+v", d)
			}
			if !strings.Contains(e.logBuf.String(), "level=WARN") {
				t.Fatal("log warn tidak ada")
			}
			e.touchCount(t, 1)
		})
	}
}

func TestT07_TouchOnlyOnSuccess(t *testing.T) {
	e := newEnv(t, 10, 1, true)
	if d := e.uc.Execute(context.Background(), in(keyA)); d.Kind != Admitted {
		t.Fatal(d.Kind)
	}
	if id := <-e.touches; id != "k1" {
		t.Fatalf("touch %q", id)
	}
	e.uc.Execute(context.Background(), in(keyA)) // key limit
	e.uc.Execute(context.Background(), in("bad"))
	e.uc.Execute(context.Background(), in(""))
	e.touchCount(t, 0)
}

func TestT07_TouchPublishErrorStillAdmits(t *testing.T) {
	e := newEnv(t, 10, 10, false)
	if d := e.uc.Execute(context.Background(), in(keyA)); d.Kind != Admitted {
		t.Fatal(d.Kind)
	}
	if !strings.Contains(e.logBuf.String(), "mcp touch publish failed") {
		t.Fatal(e.logBuf.String())
	}
}

func TestT07_PrincipalComplete(t *testing.T) {
	e := newEnv(t, 10, 10, true)
	d := e.uc.Execute(context.Background(), in(keyA))
	want := base.Principal{KeyID: "k1", KeyName: "laptop", KeyPrefix: "aaaaaaaa", ProjectID: "p1", ProjectName: "Payments", IP: "10.0.0.7", UserAgent: "ua"}
	if d.Principal != want {
		t.Fatalf("%+v", d.Principal)
	}
}

func TestT07_MissingConsumesIPToken(t *testing.T) {
	e := newEnv(t, 3, 10, true)
	for i := 0; i < 3; i++ {
		if d := e.uc.Execute(context.Background(), in("")); d.Kind != Unauthorized {
			t.Fatalf("%d: %v", i, d.Kind)
		}
	}
	if d := e.uc.Execute(context.Background(), in("")); d.Kind != RateLimited {
		t.Fatalf("%v", d.Kind)
	}
}

func TestT07_AuditFailureDoesNotChangeDecision(t *testing.T) {
	e := newEnv(t, 10, 10, true)
	// writer asli ke bus tanpa handler audit -> gagal, fail-open
	e.uc.audit = silentWriter{}
	if d := e.uc.Execute(context.Background(), in("")); d.Kind != Unauthorized {
		t.Fatal(d.Kind)
	}
}

type silentWriter struct{}

func (silentWriter) Write(context.Context, record.Request) {}
