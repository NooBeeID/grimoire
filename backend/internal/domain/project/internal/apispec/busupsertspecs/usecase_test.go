package busupsertspecs

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"grimoire/internal/domain/project/internal/apispec/base"
	"grimoire/internal/domain/project/internal/apispec/base/audit"
	"grimoire/internal/domain/project/internal/apispec/base/ingest"
	"grimoire/lib/common/errs"
	"grimoire/lib/common/reqctx"
	"grimoire/lib/contracts/project/apispec/upsertspecs"
)

const pid = "0b6c3e8a-6f0d-4a3b-9c1e-2d7f5a1b8c90"

var fixedNow = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

func specYAML(paths ...string) string {
	var b strings.Builder
	b.WriteString("openapi: 3.0.3\ninfo: { title: Pay, version: 1.0.0 }\npaths:\n")
	if len(paths) == 0 {
		paths = []string{"/payments"}
	}
	for _, p := range paths {
		b.WriteString("  " + p + ":\n    get:\n      responses: { \"200\": { description: OK } }\n")
	}
	return b.String()
}

const extRefYAML = `openapi: 3.0.3
info: { title: Pay, version: 1.0.0 }
paths:
  /charges:
    get:
      responses:
        "200": { $ref: "other.yaml#/x" }
`

type fakeAudit struct{ events []audit.Event }

func (a *fakeAudit) Record(_ context.Context, ev audit.Event) { a.events = append(a.events, ev) }

type fakeRepo struct {
	specs    map[string]base.Spec
	txCalls  int
	failOnTx int
	infraErr error
	inserted []base.Spec
	updated  []base.Spec
	replaced [][]base.Endpoint
	writes   int
	cancel   func()
}

type fakeTx struct{ r *fakeRepo }

func (r *fakeRepo) InTx(_ context.Context, fn func(ingest.Tx) error) error {
	r.txCalls++
	if r.failOnTx > 0 && r.txCalls == r.failOnTx {
		return r.infraErr
	}
	err := fn(&fakeTx{r})
	if err == nil && r.cancel != nil {
		r.cancel()
	}
	return err
}
func (t *fakeTx) LockProjectSpecs(context.Context, string) error { return nil }
func (t *fakeTx) LockSpec(_ context.Context, _, name string) (base.Spec, bool, error) {
	s, ok := t.r.specs[name]
	return s, ok, nil
}
func (t *fakeTx) CountSpecs(context.Context, string) (int, error) { return len(t.r.specs), nil }
func (t *fakeTx) InsertSpec(_ context.Context, s *base.Spec) error {
	s.ID = "id-" + s.Name
	t.r.specs[s.Name] = *s
	t.r.inserted = append(t.r.inserted, *s)
	t.r.writes++
	return nil
}
func (t *fakeTx) UpdateSpecContent(_ context.Context, s base.Spec) error {
	t.r.specs[s.Name] = s
	t.r.updated = append(t.r.updated, s)
	t.r.writes++
	return nil
}
func (t *fakeTx) ReplaceEndpoints(_ context.Context, _ string, es []base.Endpoint) error {
	t.r.replaced = append(t.r.replaced, es)
	t.r.writes++
	return nil
}

type env struct {
	uc   *Usecase
	repo *fakeRepo
	aud  *fakeAudit
}

func newEnv(maxSpecs int) *env {
	r := &fakeRepo{specs: map[string]base.Spec{}}
	a := &fakeAudit{}
	return &env{uc: NewUsecase(r, a, func() time.Time { return fixedNow }, maxSpecs, 1024*1024), repo: r, aud: a}
}

func req(items ...upsertspecs.Item) upsertspecs.Request {
	return upsertspecs.Request{
		ProjectID: pid,
		Actor:     upsertspecs.Actor{UserID: "u-actor", Role: "developer", Label: "Rina"},
		Source:    upsertspecs.SourceGitSync,
		Items:     items,
	}
}

func item(i int, p, content string) upsertspecs.Item {
	return upsertspecs.Item{Index: i, SourcePath: p, Content: []byte(content)}
}

func run(t *testing.T, e *env, r upsertspecs.Request) upsertspecs.Response {
	t.Helper()
	res, err := e.uc.Execute(context.Background(), r)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	return res
}

func TestCreate(t *testing.T) {
	e := newEnv(10)
	res := run(t, e, req(item(0, "api/openapi/billing.yaml", specYAML())))
	it := res.Items[0]
	if it.Status != "created" || it.Name != "billing" || it.SourcePath != "api/openapi/billing.yaml" ||
		it.OpenAPIVersion != "3.0.3" || it.Title != "Pay" || it.EndpointCount != 1 || it.Issues == nil {
		t.Fatalf("item = %+v", it)
	}
	if res.Summary.Created != 1 {
		t.Fatalf("summary = %+v", res.Summary)
	}
	if len(e.aud.events) != 1 {
		t.Fatalf("audit = %d", len(e.aud.events))
	}
	ev := e.aud.events[0]
	if ev.Args["source"] != "git_sync" || ev.Actor == nil || ev.Actor.ID != "u-actor" || ev.ProjectID != pid {
		t.Fatalf("event = %+v", ev)
	}
	for k := range ev.Args {
		if k == "content" {
			t.Fatal("args memuat konten")
		}
	}
	if e.repo.inserted[0].CreatedBy != "u-actor" || sp(e.repo.inserted[0].SourcePath) != "api/openapi/billing.yaml" {
		t.Fatalf("spec = %+v", e.repo.inserted[0])
	}
}

func TestUpdateAndUnchanged(t *testing.T) {
	e := newEnv(10)
	run(t, e, req(item(0, "a/billing.yaml", specYAML("/a", "/b"))))
	res := run(t, e, req(item(0, "new/billing.yaml", specYAML("/c"))))
	if res.Items[0].Status != "updated" || res.Summary.Updated != 1 {
		t.Fatalf("res = %+v", res)
	}
	if got := e.repo.replaced[len(e.repo.replaced)-1]; len(got) != 1 {
		t.Fatalf("endpoint = %d", len(got))
	}
	if sp(e.repo.specs["billing"].SourcePath) != "new/billing.yaml" {
		t.Fatal("source_path tidak diperbarui")
	}
	before := e.repo.writes
	nAudit := len(e.aud.events)
	res = run(t, e, req(item(0, "new/billing.yaml", specYAML("/c"))))
	if res.Items[0].Status != "unchanged" || e.repo.writes != before || len(e.aud.events) != nAudit {
		t.Fatalf("res = %+v", res)
	}
}

func TestInvalidAndExternalRef(t *testing.T) {
	e := newEnv(10)
	res := run(t, e, req(item(0, "a/bad.yaml", "openapi: [unclosed"), item(1, "a/pay.yaml", extRefYAML)))
	for _, it := range res.Items {
		if it.Status != "rejected" || it.ReasonCode != "INVALID_SPEC" || len(it.Issues) == 0 {
			t.Fatalf("item = %+v", it)
		}
	}
	if e.repo.txCalls != 0 || len(e.aud.events) != 0 || res.Summary.Rejected != 2 {
		t.Fatal("rejected tidak boleh menulis")
	}
}

func TestDuplicateNameInBatch(t *testing.T) {
	e := newEnv(10)
	res := run(t, e, req(item(0, "a/billing.yaml", specYAML()), item(1, "b/billing.yaml", specYAML("/z"))))
	if res.Items[0].Status != "created" || res.Items[1].Status != "rejected" || res.Items[1].ReasonCode != "DUPLICATE_NAME_IN_UPLOAD" {
		t.Fatalf("res = %+v", res.Items)
	}
	if len(e.aud.events) != 1 {
		t.Fatal("audit harus satu")
	}
}

func TestSkippedAndTooLarge(t *testing.T) {
	e := newEnv(10)
	big := upsertspecs.Item{Index: 1, SourcePath: "a/huge.yaml", TooLarge: true}
	res := run(t, e, req(item(0, "docs/readme.md", "# x"), big, item(2, "a/ok.yaml", specYAML())))
	if res.Items[0].Status != "skipped" || res.Items[0].ReasonCode != "NOT_SPEC_FILE" {
		t.Fatalf("0 = %+v", res.Items[0])
	}
	if res.Items[1].Status != "rejected" || res.Items[1].ReasonCode != "FILE_TOO_LARGE" || !strings.Contains(res.Items[1].Reason, "1048576") {
		t.Fatalf("1 = %+v", res.Items[1])
	}
	if res.Items[2].Status != "created" {
		t.Fatalf("2 = %+v", res.Items[2])
	}
	if res.Summary.Skipped != 1 || res.Summary.Rejected != 1 || res.Summary.Created != 1 {
		t.Fatalf("summary = %+v", res.Summary)
	}
}

func TestSpecLimit(t *testing.T) {
	e := newEnv(1)
	res := run(t, e, req(item(0, "a/one.yaml", specYAML()), item(1, "a/two.yaml", specYAML())))
	if res.Items[1].Status != "rejected" || res.Items[1].ReasonCode != "SPEC_LIMIT_REACHED" {
		t.Fatalf("res = %+v", res.Items)
	}
	res = run(t, e, req(item(0, "a/one.yaml", specYAML("/other"))))
	if res.Items[0].Status != "updated" {
		t.Fatalf("saat penuh harus tetap bisa update: %+v", res.Items[0])
	}
}

func TestRequestValidation(t *testing.T) {
	e := newEnv(10)
	r := req(item(0, "a/x.yaml", specYAML()))
	r.ProjectID = "bukan-uuid"
	r2 := req(item(0, "a/x.yaml", specYAML()))
	r2.Actor.UserID = "  "
	for _, bad := range []upsertspecs.Request{r, r2} {
		_, err := e.uc.Execute(context.Background(), bad)
		var ee *errs.Error
		if !errors.As(err, &ee) || ee.Code != errs.CodeInvalid {
			t.Fatalf("err = %v", err)
		}
	}
	if e.repo.txCalls != 0 || len(e.aud.events) != 0 {
		t.Fatal("tidak boleh menulis")
	}
}

func TestInvalidPath(t *testing.T) {
	e := newEnv(10)
	res := run(t, e, req(item(0, "", specYAML()), item(1, "../../x.yaml", specYAML())))
	for i, it := range res.Items {
		if it.Status != "rejected" || it.ReasonCode != "INVALID_PATH" {
			t.Fatalf("item = %+v", it)
		}
		if want := []string{"", "../../x.yaml"}[i]; it.SourcePath != want {
			t.Fatalf("source_path = %q", it.SourcePath)
		}
	}
}

func TestOrderAndIndex(t *testing.T) {
	e := newEnv(10)
	res := run(t, e, req(item(5, "a/a.yaml", specYAML()), item(2, "a/b.yaml", specYAML()), item(9, "a/c.md", "x")))
	got := []int{res.Items[0].Index, res.Items[1].Index, res.Items[2].Index}
	if got[0] != 5 || got[1] != 2 || got[2] != 9 {
		t.Fatalf("index = %v", got)
	}
}

func TestInfraErrorStopsBatch(t *testing.T) {
	e := newEnv(10)
	e.repo.failOnTx = 2
	e.repo.infraErr = errors.New("db down")
	_, err := e.uc.Execute(context.Background(), req(
		item(0, "a/a.yaml", specYAML()), item(1, "a/b.yaml", specYAML()), item(2, "a/c.yaml", specYAML())))
	if !errors.Is(err, e.repo.infraErr) {
		t.Fatalf("err = %v", err)
	}
	if _, ok := e.repo.specs["a"]; !ok || e.repo.txCalls != 2 {
		t.Fatalf("file pertama harus ter-commit, sisa dihentikan (tx=%d)", e.repo.txCalls)
	}
}

func TestContextCanceledBetweenFiles(t *testing.T) {
	e := newEnv(10)
	ctx, cancel := context.WithCancel(context.Background())
	e.repo.cancel = cancel
	_, err := e.uc.Execute(ctx, req(item(0, "a/a.yaml", specYAML()), item(1, "a/b.yaml", specYAML())))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
	if e.repo.txCalls != 1 {
		t.Fatalf("tx = %d", e.repo.txCalls)
	}
}

func TestActorFromRequestNotCtx(t *testing.T) {
	e := newEnv(10)
	ctx := reqctx.WithIdentity(context.Background(), reqctx.Identity{UserID: "other", Name: "Lain"})
	if _, err := e.uc.Execute(ctx, req(item(0, "a/a.yaml", specYAML()))); err != nil {
		t.Fatal(err)
	}
	if e.aud.events[0].Actor.ID != "u-actor" {
		t.Fatalf("actor = %+v", e.aud.events[0].Actor)
	}
	run(t, newEnv(10), req(item(0, "a/a.yaml", specYAML()))) // ctx tanpa identity
}

func TestEmptyItems(t *testing.T) {
	res := run(t, newEnv(10), req())
	if res.Items == nil || len(res.Items) != 0 || res.Summary != (upsertspecs.Summary{}) {
		t.Fatalf("res = %+v", res)
	}
}

func TestDefaultSource(t *testing.T) {
	e := newEnv(10)
	r := req(item(0, "a/a.yaml", specYAML()))
	r.Source = ""
	run(t, e, r)
	if e.aud.events[0].Args["source"] != "git_sync" {
		t.Fatalf("args = %v", e.aud.events[0].Args)
	}
}

func sp(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
