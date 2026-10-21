package getspec

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"grimoire/internal/domain/project/internal/apispec/base"
	"grimoire/lib/common/errs"
	"grimoire/lib/common/reqctx"
)

type fakeGuard struct {
	err   error
	calls int
}

func (g *fakeGuard) RequireMember(context.Context, string) error { g.calls++; return g.err }

type fakeRepo struct {
	spec                 base.Spec
	found                bool
	eps                  []base.Endpoint
	findErr, listErr     error
	findCalls, listCalls int
}

func (r *fakeRepo) FindSpec(context.Context, string, string) (base.Spec, bool, error) {
	r.findCalls++
	return r.spec, r.found, r.findErr
}

func (r *fakeRepo) ListSpecEndpoints(context.Context, string) ([]base.Endpoint, error) {
	r.listCalls++
	return r.eps, r.listErr
}

func authCtx() context.Context {
	return reqctx.WithIdentity(context.Background(), reqctx.Identity{UserID: "u1", Role: "developer", Email: "a@tim.id"})
}

func specOf(content, format string) base.Spec {
	return base.Spec{ID: "s1", ProjectID: "p1", Name: "payment", Title: "T", APIVersion: "1", OpenAPIVersion: "3.0.3",
		Content: content, Format: format, SizeBytes: len(content)}
}

const yaml30 = `openapi: 3.0.3
info: {title: T, version: "1"}
paths:
  /zebra:
    get:
      responses:
        200: {description: OK}
        404: {description: NF}
  /alpha:
    get:
      responses:
        "200": {description: OK}
components:
  schemas:
    X: {type: string, nullable: true}
`

const json31 = `{"openapi":"3.1.0","info":{"title":"T","version":"1"},"webhooks":{"ev":{"post":{"responses":{"200":{"description":"ok"}}}}},"components":{"schemas":{"S":{"type":["string","null"]}}}}`

func run(t *testing.T, r *fakeRepo, g *fakeGuard, log *slog.Logger) (Result, error) {
	t.Helper()
	return NewUsecase(r, g, log).Execute(authCtx(), "p1", "payment")
}

func TestGetSpec_NonMemberNotFoundBeforeSpecLookup(t *testing.T) {
	r := &fakeRepo{found: true}
	_, err := run(t, r, &fakeGuard{err: base.ErrProjectNotFound}, nil)
	if !errors.Is(err, base.ErrProjectNotFound) || r.findCalls != 0 || r.listCalls != 0 {
		t.Fatalf("err=%v find=%d list=%d", err, r.findCalls, r.listCalls)
	}
}

func TestGetSpec_GuardInfraError(t *testing.T) {
	r := &fakeRepo{}
	boom := errors.New("bus down")
	_, err := run(t, r, &fakeGuard{err: boom}, nil)
	if !errors.Is(err, boom) || r.findCalls != 0 {
		t.Fatalf("err=%v", err)
	}
}

func TestGetSpec_NoIdentity(t *testing.T) {
	g := &fakeGuard{}
	_, err := NewUsecase(&fakeRepo{}, g, nil).Execute(context.Background(), "p1", "payment")
	var e *errs.Error
	if !errors.As(err, &e) || e.Code != errs.CodeUnauthorized || g.calls != 0 {
		t.Fatalf("err=%v", err)
	}
}

func TestGetSpec_SpecNotFound(t *testing.T) {
	r := &fakeRepo{}
	_, err := run(t, r, &fakeGuard{}, nil)
	if !errors.Is(err, base.ErrSpecNotFound) || r.listCalls != 0 {
		t.Fatalf("err=%v", err)
	}
}

func TestGetSpec_RepoErrors(t *testing.T) {
	for name, r := range map[string]*fakeRepo{
		"find": {findErr: errors.New("db")},
		"list": {found: true, spec: specOf(yaml30, "yaml"), listErr: errors.New("db")},
	} {
		_, err := run(t, r, &fakeGuard{}, nil)
		var e *errs.Error
		if err == nil || errors.As(err, &e) || !strings.HasPrefix(err.Error(), "getspec:") {
			t.Fatalf("%s: err=%v", name, err)
		}
	}
}

func TestGetSpec_OK30(t *testing.T) {
	r := &fakeRepo{found: true, spec: specOf(yaml30, "yaml"), eps: []base.Endpoint{{Method: "GET", Path: "/alpha"}, {Method: "GET", Path: "/zebra"}}}
	res, err := run(t, r, &fakeGuard{}, nil)
	if err != nil || res.Spec.EndpointCount != 2 || len(res.Endpoints) != 2 {
		t.Fatalf("res=%+v err=%v", res.Spec, err)
	}
	var doc map[string]any
	if err := json.Unmarshal(res.Document, &doc); err != nil || doc["openapi"] != "3.0.3" {
		t.Fatalf("doc=%s err=%v", res.Document, err)
	}
}

func TestGetSpec_OK31(t *testing.T) {
	r := &fakeRepo{found: true, spec: specOf(json31, "json")}
	res, err := run(t, r, &fakeGuard{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(res.Document), `"type":["string","null"]`) || !strings.Contains(string(res.Document), `"webhooks"`) {
		t.Fatalf("doc=%s", res.Document)
	}
	if res.Spec.EndpointCount != 0 {
		t.Fatalf("count=%d", res.Spec.EndpointCount)
	}
}

func TestGetSpec_NonStringKeysBecomeStrings(t *testing.T) {
	res, err := run(t, &fakeRepo{found: true, spec: specOf(yaml30, "yaml")}, &fakeGuard{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Paths map[string]struct {
			Get struct {
				Responses map[string]any `json:"responses"`
			} `json:"get"`
		} `json:"paths"`
	}
	if err := json.Unmarshal(res.Document, &doc); err != nil {
		t.Fatal(err)
	}
	rs := doc.Paths["/zebra"].Get.Responses
	if _, ok := rs["200"]; !ok {
		t.Fatalf("responses=%v", rs)
	}
	if _, ok := rs["404"]; !ok {
		t.Fatalf("responses=%v", rs)
	}
}

func TestGetSpec_DocumentKeyOrderPreserved(t *testing.T) {
	res, err := run(t, &fakeRepo{found: true, spec: specOf(yaml30, "yaml")}, &fakeGuard{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Index(res.Document, []byte("/zebra")) > bytes.Index(res.Document, []byte("/alpha")) {
		t.Fatalf("urutan berubah: %s", res.Document)
	}
}

func TestGetSpec_ReparseFailure(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, nil))
	_, err := run(t, &fakeRepo{found: true, spec: specOf("not: [valid SECRETSNIPPET", "yaml")}, &fakeGuard{}, log)
	var e *errs.Error
	if err == nil || errors.As(err, &e) {
		t.Fatalf("err=%v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "feature=getspec") || strings.Contains(out, "SECRETSNIPPET") || strings.Contains(err.Error(), "SECRETSNIPPET") {
		t.Fatalf("log=%s err=%v", out, err)
	}
}
