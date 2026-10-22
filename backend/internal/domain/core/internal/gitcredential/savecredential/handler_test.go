package savecredential

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"context"
	"grimoire/internal/domain/core/internal/gitcredential/base"
	"grimoire/internal/infra/gitprovider"
	httpx "grimoire/internal/infra/http"
	"grimoire/internal/infra/middleware"
	"grimoire/internal/infra/safehttp"
	"grimoire/lib/common/errs"
	"grimoire/lib/common/reqctx"
)

var codesOnce sync.Once

func registerCodes() {
	codesOnce.Do(func() { httpx.RegisterCodeStatus(base.CodeTokenInvalid, http.StatusUnprocessableEntity) })
}

type errBody struct {
	Error struct {
		Code    string         `json:"code"`
		Message string         `json:"message"`
		Details map[string]any `json:"details"`
	} `json:"error"`
	TraceID string `json:"trace_id"`
}

func serve(t *testing.T, e *env, withUser bool, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	registerCodes()
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, rq *http.Request) {
			if withUser {
				rq = rq.WithContext(reqctx.WithIdentity(rq.Context(), reqctx.Identity{UserID: userID, Name: "A"}))
			}
			next.ServeHTTP(w, rq)
		})
	})
	r.With(middleware.RequireAuth(), middleware.BodyLimit(8<<10)).
		Put("/me/git-credentials/{provider}", newHandler(e.uc).ServeHTTP)
	rq := httptest.NewRequest(method, path, strings.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, rq)
	return w
}

func put(t *testing.T, e *env, provider, body string) *httptest.ResponseRecorder {
	return serve(t, e, true, http.MethodPut, "/me/git-credentials/"+provider, body)
}

func decodeErr(t *testing.T, w *httptest.ResponseRecorder) errBody {
	t.Helper()
	var b errBody
	if err := json.Unmarshal(w.Body.Bytes(), &b); err != nil {
		t.Fatalf("%v: %s", err, w.Body)
	}
	return b
}

func TestHandler_OK_GitHub(t *testing.T) {
	e := newEnv(t, true)
	w := put(t, e, "github", `{"token":"`+canary+`"}`)
	if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal(w.Code, w.Body)
	}
	var m map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &m)
	if len(m) != 5 || m["provider"] != "github" || m["host"] != "github.com" || m["last4"] != "NARY" || m["replaced"] != false {
		t.Fatalf("%v", m)
	}
	if strings.Contains(w.Body.String(), canary) {
		t.Fatal("body memuat canary")
	}
}

func TestHandler_OK_GitLabReplaced(t *testing.T) {
	e := newEnv(t, true)
	e.rep.saved.Replaced = true
	w := put(t, e, "gitlab", `{"host":"git.noobee.id","token":"glpat-abcdefgh1234"}`)
	var m map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &m)
	if w.Code != 200 || m["replaced"] != true || m["host"] != "git.noobee.id" {
		t.Fatal(w.Code, w.Body)
	}
}

func TestHandler_BadBodies(t *testing.T) {
	cases := map[string]string{
		"empty":     `{}`,
		"null":      `{"token":null}`,
		"wrongtype": `{"token":123456789012}`,
		"unknown":   `{"token":"` + canary + `","extra":1}`,
		"truncated": `{"token":"` + canary + `"`,
		"notjson":   `halo`,
		"trailing":  `{"token":"` + canary + `"}{"token":"` + canary + `"}`,
		"nobody":    ``,
	}
	for name, body := range cases {
		e := newEnv(t, true)
		w := put(t, e, "github", body)
		if w.Code != 400 {
			t.Fatalf("%s: %d %s", name, w.Code, w.Body)
		}
		if strings.Contains(w.Body.String(), canary) || strings.Contains(w.Body.String(), "123456789012") {
			t.Fatalf("%s: memantulkan input", name)
		}
		e.noSideEffects(t)
	}
	e := newEnv(t, true)
	b := decodeErr(t, put(t, e, "github", `{}`))
	if f, _ := b.Error.Details["fields"].(map[string]any); f["token"] == nil {
		t.Fatalf("%+v", b)
	}
}

func TestHandler_BodyTooLarge(t *testing.T) {
	e := newEnv(t, true)
	w := put(t, e, "github", `{"token":"`+strings.Repeat("a", 9<<10)+`"}`)
	if w.Code != 413 || decodeErr(t, w).Error.Code != string(errs.CodePayloadTooLarge) {
		t.Fatal(w.Code, w.Body)
	}
}

func TestHandler_ErrorMapping(t *testing.T) {
	tests := []struct {
		name   string
		verify error
		status int
		code   string
		retry  string
	}{
		{"unauthorized", gitprovider.ErrUnauthorized, 422, "TOKEN_INVALID", ""},
		{"forbidden", gitprovider.ErrForbidden, 422, "TOKEN_INVALID", ""},
		{"rl60", &gitprovider.RateLimitedError{RetryAfter: 60 * time.Second}, 429, "RATE_LIMITED", "60"},
		{"rl0", &gitprovider.RateLimitedError{}, 429, "RATE_LIMITED", ""},
		{"rlclamp", &gitprovider.RateLimitedError{RetryAfter: 90 * time.Minute}, 429, "RATE_LIMITED", "3600"},
		{"upstream", gitprovider.ErrUpstream, 502, "GIT_UPSTREAM_ERROR", ""},
		{"net", errors.New("tcp reset"), 502, "GIT_UPSTREAM_ERROR", ""},
		{"deadline", context.DeadlineExceeded, 502, "GIT_UPSTREAM_ERROR", ""},
		{"blocked", safehttp.ErrBlockedAddress, 502, "GIT_UPSTREAM_ERROR", ""},
		{"notfound", gitprovider.ErrNotFound, 502, "GIT_UPSTREAM_ERROR", ""},
	}
	for _, tc := range tests {
		e := newEnv(t, true)
		e.ver.err = tc.verify
		w := put(t, e, "github", `{"token":"`+canary+`"}`)
		b := decodeErr(t, w)
		if w.Code != tc.status || b.Error.Code != tc.code || w.Header().Get("Retry-After") != tc.retry {
			t.Fatalf("%s: %d %s retry=%q", tc.name, w.Code, w.Body, w.Header().Get("Retry-After"))
		}
		if w.Header().Get("Cache-Control") != "no-store" || strings.Contains(w.Body.String(), canary) {
			t.Fatalf("%s: header/body", tc.name)
		}
		if tc.name == "unauthorized" && (b.Error.Message != "token tidak valid atau tidak punya akses" ||
			b.Error.Details["provider"] != "github" || b.Error.Details["host"] != "github.com") {
			t.Fatalf("%+v", b)
		}
		if tc.name == "rl60" && b.Error.Details["retry_after_seconds"] != float64(60) {
			t.Fatalf("%+v", b)
		}
		if tc.name == "rl0" && b.Error.Details != nil {
			t.Fatalf("%+v", b)
		}
	}
}

func TestHandler_UsecaseErrors(t *testing.T) {
	e := newEnv(t, true)
	w := put(t, e, "gitlab", `{"host":"git.contoh.com","token":"`+canary+`"}`)
	b := decodeErr(t, w)
	if w.Code != 422 || b.Error.Code != "HOST_NOT_ALLOWED" || b.Error.Details["host"] != "git.contoh.com" {
		t.Fatal(w.Code, w.Body)
	}
	w = serve(t, e, false, http.MethodPut, "/me/git-credentials/github", `{"token":"`+canary+`"}`)
	if w.Code != 401 {
		t.Fatal(w.Code)
	}
	d := newEnv(t, true)
	d.uc.enabled = false
	w = put(t, d, "github", `{"token":"`+canary+`"}`)
	if w.Code != 503 || decodeErr(t, w).Error.Code != "GIT_FEATURE_DISABLED" {
		t.Fatal(w.Code, w.Body)
	}
	w = put(t, e, "Bitbucket", `{"token":"`+canary+`"}`)
	if w.Code != 400 || !strings.Contains(w.Body.String(), "provider") {
		t.Fatal(w.Code, w.Body)
	}
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("no-store pada error")
	}
}

func TestHandler_CanceledContext(t *testing.T) {
	e := newEnv(t, true)
	e.ver.err = context.Canceled
	w := put(t, e, "github", `{"token":"`+canary+`"}`)
	if strings.Contains(w.Body.String(), "GIT_UPSTREAM_ERROR") {
		t.Fatal(w.Body)
	}
}

func TestSaveRequest_UnmarshalRedacted(t *testing.T) {
	var req SaveRequest
	if err := json.Unmarshal([]byte(`{"host":"h","token":"`+canary+`"}`), &req); err != nil {
		t.Fatal(err)
	}
	if req.Token.Reveal() != canary || req.Host != "h" {
		t.Fatal("decode")
	}
	var buf bytes.Buffer
	slog.New(slog.NewJSONHandler(&buf, nil)).Info("x", "req", req, "tok", req.Token)
	mj, _ := json.Marshal(req)
	out := fmt.Sprintf("%v %+v %#v %q %s %s", req, req, req, req, buf.String(), mj)
	if strings.Contains(out, canary) {
		t.Fatal(out)
	}
}
