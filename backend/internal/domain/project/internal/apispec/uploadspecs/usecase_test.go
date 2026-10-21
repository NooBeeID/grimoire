package uploadspecs

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"grimoire/internal/domain/project/internal/apispec/base"
	"grimoire/internal/domain/project/internal/apispec/base/audit"
	"grimoire/internal/domain/project/internal/apispec/base/store"
	"grimoire/internal/domain/project/internal/apispec/base/upload"
	"grimoire/lib/common/errs"
	"grimoire/lib/common/reqctx"
	"grimoire/lib/contracts/core/audit/record"
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

type fakeGuard struct {
	err   error
	calls int
}

func (g *fakeGuard) RequireMember(context.Context, string) error { g.calls++; return g.err }

type fakeAudit struct {
	events    []audit.Event
	ctxCancel []bool
	repo      *fakeRepo
	afterTx   []bool
}

func (a *fakeAudit) Record(ctx context.Context, ev audit.Event) {
	a.events = append(a.events, ev)
	a.ctxCancel = append(a.ctxCancel, ctx.Err() != nil)
	if a.repo != nil {
		a.afterTx = append(a.afterTx, !a.repo.inTx)
	}
}

type fakeRepo struct {
	specs       map[string]base.Spec
	count       int // override bila >= 0
	calls       []string
	inTx        bool
	txCalls     int
	inserted    []base.Spec
	updated     []base.Spec
	replaced    [][]base.Endpoint
	insertErrs  []error
	failOnTx    int // tx ke-N (1-based) gagal infra
	infraErr    error
	nextID      int
	cancelAfter func()
}

func newFakeRepo() *fakeRepo { return &fakeRepo{specs: map[string]base.Spec{}, count: -1} }

type fakeTx struct{ r *fakeRepo }

func (r *fakeRepo) InTx(ctx context.Context, fn func(Tx) error) error {
	r.txCalls++
	if r.failOnTx > 0 && r.txCalls == r.failOnTx {
		return r.infraErr
	}
	r.inTx = true
	defer func() { r.inTx = false }()
	err := fn(&fakeTx{r})
	if r.cancelAfter != nil && err == nil {
		r.cancelAfter()
	}
	return err
}
func (t *fakeTx) LockProjectSpecs(context.Context, string) error {
	t.r.calls = append(t.r.calls, "lockproject")
	return nil
}
func (t *fakeTx) LockSpec(_ context.Context, _, name string) (base.Spec, bool, error) {
	t.r.calls = append(t.r.calls, "lockspec")
	s, ok := t.r.specs[name]
	return s, ok, nil
}
func (t *fakeTx) CountSpecs(context.Context, string) (int, error) {
	t.r.calls = append(t.r.calls, "count")
	if t.r.count >= 0 {
		return t.r.count, nil
	}
	return len(t.r.specs), nil
}
func (t *fakeTx) InsertSpec(_ context.Context, s *base.Spec) error {
	t.r.calls = append(t.r.calls, "insert")
	if len(t.r.insertErrs) > 0 {
		e := t.r.insertErrs[0]
		t.r.insertErrs = t.r.insertErrs[1:]
		if e != nil {
			return e
		}
	}
	t.r.nextID++
	s.ID = "id-" + s.Name
	t.r.specs[s.Name] = *s
	t.r.inserted = append(t.r.inserted, *s)
	return nil
}
func (t *fakeTx) UpdateSpecContent(_ context.Context, s base.Spec) error {
	t.r.calls = append(t.r.calls, "update")
	t.r.specs[s.Name] = s
	t.r.updated = append(t.r.updated, s)
	return nil
}
func (t *fakeTx) ReplaceEndpoints(_ context.Context, _ string, es []base.Endpoint) error {
	t.r.calls = append(t.r.calls, "replace")
	t.r.replaced = append(t.r.replaced, es)
	return nil
}

type env struct {
	uc    *Usecase
	repo  *fakeRepo
	guard *fakeGuard
	aud   *fakeAudit
}

func newEnv(maxSpecs int) *env {
	r := newFakeRepo()
	g := &fakeGuard{}
	a := &fakeAudit{repo: r}
	return &env{uc: NewUsecase(r, g, a, func() time.Time { return fixedNow }, maxSpecs, 5242880), repo: r, guard: g, aud: a}
}

func authCtx() context.Context {
	return reqctx.WithIdentity(context.Background(), reqctx.Identity{UserID: "u1", Email: "b@x.id", Name: "Budi", Role: "developer"})
}

func file(path, content string) upload.File {
	return upload.File{FileName: path[strings.LastIndex(path, "/")+1:], RawPath: path, Content: []byte(content)}
}

func run(t *testing.T, e *env, files ...upload.File) Output {
	t.Helper()
	out, err := e.uc.Execute(authCtx(), Input{ProjectID: pid, Files: files})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	return out
}

func TestUploadSpecs_NoIdentity(t *testing.T) {
	e := newEnv(10)
	_, err := e.uc.Execute(context.Background(), Input{ProjectID: pid, Files: []upload.File{file("a.yaml", specYAML())}})
	var ee *errs.Error
	if !errors.As(err, &ee) || ee.Code != errs.CodeUnauthorized {
		t.Fatalf("err = %v", err)
	}
	if e.guard.calls != 0 {
		t.Fatal("guard dipanggil")
	}
}

func TestUploadSpecs_GuardDenied(t *testing.T) {
	e := newEnv(10)
	e.guard.err = base.ErrProjectNotFound
	_, err := e.uc.Execute(authCtx(), Input{ProjectID: pid, Files: []upload.File{file("a.yaml", specYAML())}})
	if !errors.Is(err, base.ErrProjectNotFound) {
		t.Fatalf("err = %v", err)
	}
	if e.repo.txCalls != 0 || len(e.aud.events) != 0 {
		t.Fatal("repo/audit tersentuh")
	}
}

func TestUploadSpecs_GuardInfraError(t *testing.T) {
	e := newEnv(10)
	e.guard.err = errors.New("bus down")
	if _, err := e.uc.Execute(authCtx(), Input{ProjectID: pid, Files: []upload.File{file("a.yaml", specYAML())}}); err == nil {
		t.Fatal("harus error")
	}
	if e.repo.txCalls != 0 {
		t.Fatal("repo tersentuh")
	}
}

func TestUploadSpecs_ExplicitName(t *testing.T) {
	e := newEnv(10)
	bad := "Bad Name"
	_, err := e.uc.Execute(authCtx(), Input{ProjectID: pid, Name: &bad, Files: []upload.File{file("payment.yaml", specYAML())}})
	if !errors.Is(err, base.ErrInvalidName) || e.repo.txCalls != 0 {
		t.Fatalf("err=%v tx=%d", err, e.repo.txCalls)
	}
	blank := "  "
	out, err := e.uc.Execute(authCtx(), Input{ProjectID: pid, Name: &blank, Files: []upload.File{file("payment.yaml", specYAML())}})
	if err != nil || out.Results[0].Name != "payment" {
		t.Fatalf("blank: %v %+v", err, out.Results)
	}
	e2 := newEnv(10)
	ok := " payment-v2 "
	out, err = e2.uc.Execute(authCtx(), Input{ProjectID: pid, Name: &ok, Files: []upload.File{file("payment.yaml", specYAML())}})
	if err != nil || out.Results[0].Name != "payment-v2" || out.Results[0].Status != base.StatusCreated {
		t.Fatalf("explicit: %v %+v", err, out.Results)
	}
}

func TestUploadSpecs_NonPersistFiles(t *testing.T) {
	cases := []struct {
		name   string
		f      upload.File
		status base.FileStatus
		code   string
	}{
		{"txt", file("notes.txt", "x"), base.StatusSkipped, base.ReasonNotSpecFile},
		{"noext", file("README", "x"), base.StatusSkipped, base.ReasonNotSpecFile},
		{"too many", upload.File{FileName: "a.yaml", RawPath: "a.yaml", Rejected: base.ReasonTooManyFiles}, base.StatusRejected, base.ReasonTooManyFiles},
		{"traversal", file("../x.yaml", specYAML()), base.StatusRejected, base.ReasonInvalidPath},
		{"abs", file("/abs.yaml", specYAML()), base.StatusRejected, base.ReasonInvalidPath},
		{"drive", file(`C:\x.yaml`, specYAML()), base.StatusRejected, base.ReasonInvalidPath},
		{"too large", upload.File{FileName: "big.yaml", RawPath: "big.yaml", TooLarge: true}, base.StatusRejected, base.ReasonFileTooLarge},
		{"empty name", file(".yaml", specYAML()), base.StatusRejected, base.ReasonInvalidName},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := newEnv(10)
			out := run(t, e, c.f)
			r := out.Results[0]
			if r.Status != c.status || r.ReasonCode != c.code {
				t.Fatalf("got %s/%s", r.Status, r.ReasonCode)
			}
			if r.Issues == nil || e.repo.txCalls != 0 || len(e.aud.events) != 0 {
				t.Fatal("issues nil / repo / audit")
			}
			if c.code == base.ReasonFileTooLarge && !strings.Contains(r.Reason, "5242880") {
				t.Fatalf("reason %q", r.Reason)
			}
		})
	}
}

func TestUploadSpecs_InvalidSpecs(t *testing.T) {
	cases := []struct {
		name, path, content, code string
	}{
		{"not openapi", "package.json", `{"name":"x","version":"1.0.0"}`, "NOT_OPENAPI"},
		{"swagger2", "s.yaml", "swagger: '2.0'\ninfo: {title: a, version: '1'}\npaths: {}\n", "NOT_OPENAPI_3"},
		{"syntax", "b.yaml", "openapi: 3.0.3\ninfo: [\n  bad: : x\n", "SYNTAX"},
		{"external ref", "r.yaml", "openapi: 3.0.3\ninfo: {title: a, version: '1'}\npaths:\n  /p:\n    get:\n      responses:\n        '200':\n          description: ok\n          content:\n            application/json:\n              schema:\n                $ref: http://127.0.0.1:1/x.yaml\n", "EXTERNAL_REF"},
		{"unresolved", "u.yaml", "openapi: 3.0.3\ninfo: {title: a, version: '1'}\npaths:\n  /p:\n    get:\n      responses:\n        '200':\n          description: ok\n          content:\n            application/json:\n              schema:\n                $ref: '#/components/schemas/Nope'\n", "UNRESOLVED_REF"},
		{"version4", "v.yaml", "openapi: 4.0.0\ninfo: {title: a, version: '1'}\npaths: {}\n", "UNSUPPORTED_VERSION"},
		{"billion laughs", "bl.yaml", billionLaughs(), "YAML_LIMIT"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := newEnv(10)
			out := run(t, e, file(c.path, c.content))
			r := out.Results[0]
			if r.Status != base.StatusRejected || r.ReasonCode != base.ReasonInvalidSpec {
				t.Fatalf("got %s/%s", r.Status, r.ReasonCode)
			}
			if len(r.Issues) == 0 || r.Issues[0].Code != c.code {
				t.Fatalf("issues = %+v", r.Issues)
			}
			if r.Reason != r.Issues[0].Message {
				t.Fatalf("reason %q", r.Reason)
			}
			if c.code == "SYNTAX" && r.Issues[0].Line <= 0 {
				t.Fatalf("line = %d", r.Issues[0].Line)
			}
			if r.Name == "" || r.EndpointCount != 0 || e.repo.txCalls != 0 || len(e.aud.events) != 0 {
				t.Fatal("name/endpoint/repo/audit")
			}
		})
	}
}

func billionLaughs() string {
	var b strings.Builder
	b.WriteString("a0: &a0 [x,x,x,x,x,x,x,x,x]\n")
	prev := "a0"
	for i := 1; i < 12; i++ {
		cur := "a" + string(rune('0'+i%10)) + string(rune('a'+i))
		b.WriteString(cur + ": &" + cur + " [" + strings.Repeat("*"+prev+",", 9) + "]\n")
		prev = cur
	}
	return b.String()
}

func TestUploadSpecs_BOMStripped(t *testing.T) {
	e := newEnv(10)
	raw := "\xEF\xBB\xBF" + specYAML()
	out := run(t, e, file("pay.yaml", raw))
	if out.Results[0].Status != base.StatusCreated {
		t.Fatalf("%+v", out.Results[0])
	}
	s := e.repo.inserted[0]
	if s.Content != specYAML() || s.Hash != base.HashContent(specYAML()) {
		t.Fatal("BOM tidak dibuang")
	}
}

func TestUploadSpecs_DuplicateNameInUpload(t *testing.T) {
	e := newEnv(10)
	out := run(t, e, file("a/payment.yaml", specYAML()), file("b/payment.yaml", specYAML("/other")))
	if out.Results[0].Status != base.StatusCreated || out.Results[1].Status != base.StatusRejected ||
		out.Results[1].ReasonCode != base.ReasonDuplicateNameInUpload {
		t.Fatalf("%+v", out.Results)
	}
	e = newEnv(10)
	out = run(t, e, file("a/payment.yaml", "{"), file("b/payment.yaml", specYAML()))
	if out.Results[0].ReasonCode != base.ReasonInvalidSpec || out.Results[1].Status != base.StatusCreated {
		t.Fatalf("%+v", out.Results)
	}
}

func TestUploadSpecs_OrderAndSummary(t *testing.T) {
	e := newEnv(10)
	out := run(t, e, file("a.yaml", specYAML()), file("n.txt", "x"), file("b.json", "{}"), file("c.yaml", specYAML()))
	want := []string{"a.yaml", "n.txt", "b.json", "c.yaml"}
	for i, w := range want {
		if out.Results[i].File != w {
			t.Fatalf("order %d: %s", i, out.Results[i].File)
		}
	}
	if out.Summary != base.Summarize(out.Results) || out.Summary.Created != 2 || out.Summary.Skipped != 1 || out.Summary.Rejected != 1 {
		t.Fatalf("%+v", out.Summary)
	}
}

func TestUploadSpecs_Create(t *testing.T) {
	e := newEnv(10)
	out := run(t, e, file("specs/payment.yaml", specYAML("/a", "/b")))
	r := out.Results[0]
	if r.Status != base.StatusCreated || r.EndpointCount != 2 || r.Title != "Pay" || r.OpenAPIVersion != "3.0.3" || r.APIVersion != "1.0.0" {
		t.Fatalf("%+v", r)
	}
	if r.SourcePath == nil || *r.SourcePath != "specs/payment.yaml" || r.ReasonCode != "" {
		t.Fatalf("%+v", r)
	}
	if len(e.repo.inserted) != 1 || len(e.repo.replaced) != 1 || len(e.repo.replaced[0]) != 2 {
		t.Fatal("insert/replace")
	}
	s := e.repo.inserted[0]
	if s.CreatedBy != "u1" || s.UpdatedBy != "u1" || s.CreatedByLabel != "Budi" || !s.CreatedAt.Equal(fixedNow) {
		t.Fatalf("%+v", s)
	}
}

func TestUploadSpecs_ActorLabelFallbackEmail(t *testing.T) {
	e := newEnv(10)
	ctx := reqctx.WithIdentity(context.Background(), reqctx.Identity{UserID: "u2", Email: "e@x.id", Role: "developer"})
	if _, err := e.uc.Execute(ctx, Input{ProjectID: pid, Files: []upload.File{file("a.yaml", specYAML())}}); err != nil {
		t.Fatal(err)
	}
	if e.repo.inserted[0].CreatedByLabel != "e@x.id" {
		t.Fatal(e.repo.inserted[0].CreatedByLabel)
	}
}

func TestUploadSpecs_UpdateAndUnchanged(t *testing.T) {
	e := newEnv(10)
	run(t, e, file("payment.yaml", specYAML("/old")))
	created := e.repo.specs["payment"]
	e.repo.replaced = nil
	e.aud.events = nil

	later := fixedNow.Add(time.Hour)
	e.uc.clock = func() time.Time { return later }
	out := run(t, e, file("payment.yaml", specYAML("/new")))
	if out.Results[0].Status != base.StatusUpdated || len(e.repo.updated) != 1 || len(e.repo.inserted) != 1 {
		t.Fatalf("%+v", out.Results[0])
	}
	u := e.repo.updated[0]
	if !u.CreatedAt.Equal(created.CreatedAt) || !u.UpdatedAt.Equal(later) || u.ID != created.ID {
		t.Fatalf("%+v", u)
	}
	if len(e.repo.replaced) != 1 || e.repo.replaced[0][0].Path != "/new" || len(e.repo.replaced[0]) != 1 {
		t.Fatalf("%+v", e.repo.replaced)
	}

	e.repo.replaced = nil
	e.aud.events = nil
	out = run(t, e, file("payment.yaml", specYAML("/new")))
	if out.Results[0].Status != base.StatusUnchanged || len(e.repo.replaced) != 0 || len(e.aud.events) != 0 {
		t.Fatalf("%+v", out.Results[0])
	}
	if out.Results[0].EndpointCount != 1 {
		t.Fatal("endpoint count unchanged")
	}
}

func TestUploadSpecs_Limit(t *testing.T) {
	e := newEnv(1)
	run(t, e, file("a.yaml", specYAML()))
	e.aud.events = nil
	out := run(t, e, file("b.yaml", specYAML()), file("b2/b.yaml", specYAML("/x")))
	if out.Results[0].Status != base.StatusRejected || out.Results[0].ReasonCode != base.ReasonSpecLimitReached ||
		!strings.Contains(out.Results[0].Reason, "1 spec") || len(e.aud.events) != 0 {
		t.Fatalf("%+v", out.Results[0])
	}
	// nama tidak dipesan: file kedua ber-nama sama juga kena limit, bukan duplikat
	if out.Results[1].ReasonCode != base.ReasonSpecLimitReached {
		t.Fatalf("%+v", out.Results[1])
	}
	// timpa tetap boleh
	out = run(t, e, file("a.yaml", specYAML("/changed")))
	if out.Results[0].Status != base.StatusUpdated {
		t.Fatalf("%+v", out.Results[0])
	}
	out = run(t, e, file("a.yaml", specYAML("/changed")))
	if out.Results[0].Status != base.StatusUnchanged {
		t.Fatalf("%+v", out.Results[0])
	}
}

func TestUploadSpecs_TxCallOrder(t *testing.T) {
	e := newEnv(10)
	run(t, e, file("a.yaml", specYAML()))
	want := []string{"lockproject", "lockspec", "count", "insert", "replace"}
	if strings.Join(e.repo.calls, ",") != strings.Join(want, ",") {
		t.Fatalf("calls = %v", e.repo.calls)
	}
}

func TestUploadSpecs_NameTakenRetry(t *testing.T) {
	e := newEnv(10)
	e.repo.insertErrs = []error{store.ErrNameTaken}
	out := run(t, e, file("a.yaml", specYAML()))
	if out.Results[0].Status != base.StatusCreated || e.repo.txCalls != 2 {
		t.Fatalf("%+v tx=%d", out.Results[0], e.repo.txCalls)
	}
	e = newEnv(10)
	e.repo.insertErrs = []error{store.ErrNameTaken, store.ErrNameTaken}
	if _, err := e.uc.Execute(authCtx(), Input{ProjectID: pid, Files: []upload.File{file("a.yaml", specYAML())}}); err == nil {
		t.Fatal("harus error")
	}
}

func TestUploadSpecs_InfraErrorStopsRest(t *testing.T) {
	e := newEnv(10)
	e.repo.failOnTx = 2
	e.repo.infraErr = errors.New("db down")
	_, err := e.uc.Execute(authCtx(), Input{ProjectID: pid, Files: []upload.File{
		file("a.yaml", specYAML()), file("b.yaml", specYAML()), file("c.yaml", specYAML())}})
	if err == nil || !errors.Is(err, e.repo.infraErr) {
		t.Fatalf("err = %v", err)
	}
	if len(e.aud.events) != 1 || e.aud.events[0].Target != "a" || e.repo.txCalls != 2 {
		t.Fatalf("events=%d tx=%d", len(e.aud.events), e.repo.txCalls)
	}
}

func TestUploadSpecs_ContextCanceledBetweenFiles(t *testing.T) {
	e := newEnv(10)
	ctx, cancel := context.WithCancel(authCtx())
	e.repo.cancelAfter = cancel
	_, err := e.uc.Execute(ctx, Input{ProjectID: pid, Files: []upload.File{file("a.yaml", specYAML()), file("b.yaml", specYAML())}})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
	if e.repo.txCalls != 1 {
		t.Fatal(e.repo.txCalls)
	}
}

func TestUploadSpecs_Audit(t *testing.T) {
	e := newEnv(10)
	run(t, e, file("a.yaml", specYAML()), file("b.yaml", specYAML()), file("c.yaml", specYAML()))
	if len(e.aud.events) != 3 {
		t.Fatalf("events = %d", len(e.aud.events))
	}
	ev := e.aud.events[0]
	if ev.Action != record.ActionAPISpecUploaded || ev.Target != "a" || ev.ProjectID != pid || ev.Args["overwritten"] != false {
		t.Fatalf("%+v", ev)
	}
	if _, has := ev.Args["content"]; has {
		t.Fatal("args memuat content")
	}
	for i := range e.aud.events {
		if !e.aud.afterTx[i] {
			t.Fatal("audit di dalam tx")
		}
	}
	e.aud.events = nil
	run(t, e, file("a.yaml", specYAML("/z")))
	if len(e.aud.events) != 1 || e.aud.events[0].Args["overwritten"] != true {
		t.Fatalf("%+v", e.aud.events)
	}
}

func TestUploadSpecs_AuditSurvivesCancel(t *testing.T) {
	e := newEnv(10)
	ctx, cancel := context.WithCancel(authCtx())
	e.repo.cancelAfter = cancel
	_, _ = e.uc.Execute(ctx, Input{ProjectID: pid, Files: []upload.File{file("a.yaml", specYAML())}})
	if len(e.aud.events) != 1 || e.aud.ctxCancel[0] {
		t.Fatalf("events=%d cancel=%v", len(e.aud.events), e.aud.ctxCancel)
	}
}
