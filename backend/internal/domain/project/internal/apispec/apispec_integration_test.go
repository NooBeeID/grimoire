//go:build integration

package apispec

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/go-chi/chi/v5"

	"grimoire/internal/infra/app"
	"grimoire/internal/infra/bus"
	"grimoire/internal/infra/config"
	"grimoire/internal/infra/db"
	"grimoire/lib/common/reqctx"
	"grimoire/lib/constants"
	"grimoire/lib/contracts/core/audit/record"
	"grimoire/lib/contracts/project/apispec/listendpoints"
	"grimoire/lib/contracts/project/management/checkaccess"
)

const (
	itUser    = "0b6c0000-0000-4000-8000-000000000001"
	testdata  = "base/openapi/testdata/"
	itMissing = "0b6c0000-0000-4000-8000-0000000000ff"
)

type itEnv struct {
	t     *testing.T
	d     *db.DB
	reg   *bus.Registry
	h     http.Handler
	mu    sync.Mutex
	level map[string]checkaccess.Level // projectID -> level; tidak ada -> project tidak ada
	audit []record.Request
	// auditFail membuat handler core.audit.record mengembalikan error.
	auditFail bool
}

func newITEnv(t *testing.T, maxSpecs int) *itEnv {
	t.Helper()
	d := db.TestDB(t)
	if _, err := db.Up(context.Background(), d, nil); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	e := &itEnv{t: t, d: d, level: map[string]checkaccess.Level{}}
	e.reg = bus.NewRegistry(slog.New(slog.NewTextHandler(io.Discard, nil)))
	e.reg.Register(constants.BUS_PROJECT_MANAGEMENT_CHECK_ACCESS, func(_ context.Context, req any) (any, error) {
		r := req.(checkaccess.Request)
		e.mu.Lock()
		defer e.mu.Unlock()
		lv, ok := e.level[r.ProjectID]
		return checkaccess.Response{ProjectExists: ok, Level: lv}, nil
	})
	e.reg.Register(constants.BUS_CORE_AUDIT_RECORD, func(_ context.Context, req any) (any, error) {
		e.mu.Lock()
		defer e.mu.Unlock()
		if e.auditFail {
			return nil, errors.New("audit down")
		}
		e.audit = append(e.audit, req.(record.Request))
		return record.Response{Recorded: true}, nil
	})
	api := chi.NewRouter()
	api.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("X-No-Identity") == "" {
				ctx := reqctx.WithIdentity(r.Context(), reqctx.Identity{UserID: itUser, Email: "dev@example.com", Name: "Dev", Role: "developer"})
				r = r.WithContext(ctx)
			}
			next.ServeHTTP(w, r)
		})
	})
	Register(app.Deps{
		Config: config.Config{MaxUploadBytes: 20 << 20, MaxUploadFiles: 50, APISpecMaxFileBytes: 5 << 20, APISpecMaxSpecs: maxSpecs},
		Log:    slog.New(slog.NewTextHandler(io.Discard, nil)),
		DB:     d, Bus: e.reg, API: api,
	})
	root := chi.NewRouter()
	root.Mount("/api", api)
	e.h = root
	return e
}

func (e *itEnv) project(name string) string {
	e.t.Helper()
	var id string
	if err := e.d.GetContext(context.Background(), &id, `INSERT INTO projects (name) VALUES ($1) RETURNING id::text`, name); err != nil {
		e.t.Fatal(err)
	}
	e.mu.Lock()
	e.level[id] = checkaccess.LevelMember
	e.mu.Unlock()
	return id
}

func (e *itEnv) audits(action string) []record.Request {
	e.mu.Lock()
	defer e.mu.Unlock()
	var out []record.Request
	for _, r := range e.audit {
		if action == "" || r.Action == action {
			out = append(out, r)
		}
	}
	return out
}

type itFile struct{ Path, Content string }

type itResult struct {
	File       string  `json:"file"`
	Name       *string `json:"name"`
	Status     string  `json:"status"`
	ReasonCode *string `json:"reason_code"`
	Issues     []struct {
		Code string `json:"code"`
	} `json:"issues"`
}

type itUploadResp struct {
	Results []itResult `json:"results"`
	Summary struct {
		Created, Updated, Unchanged, Skipped, Rejected int
	} `json:"summary"`
}

func (e *itEnv) do(method, url string, body io.Reader, ctype string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, url, body)
	if ctype != "" {
		req.Header.Set("Content-Type", ctype)
	}
	w := httptest.NewRecorder()
	e.h.ServeHTTP(w, req)
	return w
}

func (e *itEnv) upload(pid string, files ...itFile) itUploadResp {
	e.t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for _, f := range files {
		w, err := mw.CreateFormFile("files", f.Path)
		if err != nil {
			e.t.Fatal(err)
		}
		_, _ = io.WriteString(w, f.Content)
	}
	_ = mw.Close()
	w := e.do(http.MethodPost, "/api/projects/"+pid+"/api-specs/upload", &buf, mw.FormDataContentType())
	if w.Code != http.StatusOK {
		e.t.Fatalf("upload: %d %s", w.Code, w.Body.String())
	}
	var out itUploadResp
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		e.t.Fatal(err)
	}
	return out
}

func (e *itEnv) endpointPaths(pid, spec string) []string {
	e.t.Helper()
	res, err := bus.CallTyped[listendpoints.Response](context.Background(), e.reg, constants.BUS_PROJECT_APISPEC_LIST_ENDPOINTS,
		listendpoints.Request{ProjectID: pid, Spec: spec, Limit: 100})
	if err != nil {
		e.t.Fatal(err)
	}
	out := []string{}
	for _, it := range res.Items {
		out = append(out, it.Path)
	}
	return out
}

func (e *itEnv) count(q string, args ...any) int {
	e.t.Helper()
	var n int
	if err := e.d.GetContext(context.Background(), &n, q, args...); err != nil {
		e.t.Fatal(err)
	}
	return n
}

func specYAML(title string, paths ...string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "openapi: 3.0.3\ninfo: {title: %s, version: \"1\"}\npaths:\n", title)
	for _, p := range paths {
		fmt.Fprintf(&b, "  %s:\n    get:\n      responses:\n        '200': {description: OK}\n", p)
	}
	return b.String()
}

func readTestdata(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(testdata + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func has(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func TestT06_AC1_UploadThreeThenOverwrite(t *testing.T) {
	e := newITEnv(t, 50)
	pid := e.project("Proj")
	files := []itFile{
		{"specs/auth.yaml", specYAML("Auth", "/login", "/logout")},
		{"specs/payment.yaml", specYAML("Payment", "/pay", "/refund")},
		{"specs/notification.yaml", specYAML("Notif", "/notify")},
	}
	out := e.upload(pid, files...)
	if out.Summary.Created != 3 || len(out.Results) != 3 {
		t.Fatalf("want 3 created: %+v", out.Summary)
	}
	if got := e.endpointPaths(pid, "payment"); !has(got, "/refund") {
		t.Fatalf("payment endpoints: %v", got)
	}
	if n := len(e.audits(record.ActionAPISpecUploaded)); n != 3 {
		t.Fatalf("audit uploaded = %d, want 3", n)
	}

	// Timpa payment dengan endpoint berbeda.
	files[1].Content = specYAML("Payment", "/pay", "/invoice")
	out = e.upload(pid, files[1])
	if out.Summary.Updated != 1 || out.Results[0].Status != "updated" {
		t.Fatalf("want updated: %+v", out)
	}
	got := e.endpointPaths(pid, "payment")
	if has(got, "/refund") || !has(got, "/invoice") || !has(got, "/pay") {
		t.Fatalf("endpoint lama harus hilang: %v", got)
	}
	ups := e.audits(record.ActionAPISpecUploaded)
	if len(ups) != 4 || ups[3].Args["overwritten"] != true {
		t.Fatalf("audit overwrite: %+v", ups)
	}

	// Identik: unchanged tanpa audit baru.
	out = e.upload(pid, files[1])
	if out.Summary.Unchanged != 1 || out.Results[0].Status != "unchanged" {
		t.Fatalf("want unchanged: %+v", out)
	}
	if n := len(e.audits(record.ActionAPISpecUploaded)); n != 4 {
		t.Fatalf("unchanged tidak boleh menulis audit: %d", n)
	}
}

func TestT06_AC2_RejectedWithReasons(t *testing.T) {
	e := newITEnv(t, 50)
	pid := e.project("Proj")

	var hits int
	var hmu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hmu.Lock()
		hits++
		hmu.Unlock()
	}))
	defer srv.Close()
	port := srv.URL[strings.LastIndex(srv.URL, ":")+1:]

	out := e.upload(pid,
		itFile{"package.json", readTestdata(t, "package.json")},
		itFile{"broken.yaml", readTestdata(t, "broken.yaml")},
		itFile{"old.yaml", readTestdata(t, "swagger-2.0.yaml")},
		itFile{"ext.yaml", strings.ReplaceAll(readTestdata(t, "ref-http.yaml"), "__PORT__", port)},
	)
	if out.Summary.Rejected != 4 || len(out.Results) != 4 {
		t.Fatalf("want 4 rejected: %+v", out)
	}
	want := map[string]string{
		"package.json": "NOT_OPENAPI", "broken.yaml": "SYNTAX", "old.yaml": "NOT_OPENAPI_3", "ext.yaml": "EXTERNAL_REF",
	}
	for _, r := range out.Results {
		code := want[r.File]
		if r.Status != "rejected" || r.ReasonCode == nil || *r.ReasonCode != "INVALID_SPEC" {
			t.Errorf("%s: %+v", r.File, r)
			continue
		}
		if len(r.Issues) == 0 || r.Issues[0].Code != code {
			t.Errorf("%s: issues %+v, want %s", r.File, r.Issues, code)
		}
	}
	hmu.Lock()
	defer hmu.Unlock()
	if hits != 0 {
		t.Fatalf("listener menerima %d request, want 0", hits)
	}
	if n := e.count(`SELECT count(*) FROM api_specs WHERE project_id = $1`, pid); n != 0 {
		t.Fatalf("baris tersimpan = %d, want 0", n)
	}
	if n := len(e.audits("")); n != 0 {
		t.Fatalf("audit = %d, want 0", n)
	}
}

func TestT06_AC6_OpenAPI30And31(t *testing.T) {
	e := newITEnv(t, 50)
	pid := e.project("Proj")
	out := e.upload(pid,
		itFile{"petstore-3.0.yaml", readTestdata(t, "petstore-3.0.yaml")},
		itFile{"petstore-3.1.json", readTestdata(t, "petstore-3.1.json")},
	)
	if out.Summary.Created != 2 {
		t.Fatalf("%+v", out)
	}
	for _, r := range out.Results {
		w := e.do(http.MethodGet, "/api/projects/"+pid+"/api-specs/"+*r.Name, nil, "")
		if w.Code != http.StatusOK {
			t.Fatalf("getspec %s: %d %s", *r.Name, w.Code, w.Body.String())
		}
	}
}

func TestT06_SpecLimit(t *testing.T) {
	e := newITEnv(t, 2)
	pid := e.project("Proj")
	out := e.upload(pid, itFile{"a.yaml", specYAML("A", "/a")}, itFile{"b.yaml", specYAML("B", "/b")})
	if out.Summary.Created != 2 {
		t.Fatalf("%+v", out)
	}
	out = e.upload(pid, itFile{"c.yaml", specYAML("C", "/c")})
	r := out.Results[0]
	if r.Status != "rejected" || r.ReasonCode == nil || *r.ReasonCode != "SPEC_LIMIT_REACHED" {
		t.Fatalf("want SPEC_LIMIT_REACHED: %+v", r)
	}
	// Menimpa spec yang ada tetap boleh.
	out = e.upload(pid, itFile{"a.yaml", specYAML("A", "/a", "/a2")})
	if out.Summary.Updated != 1 {
		t.Fatalf("overwrite pada batas: %+v", out)
	}
	if n := e.count(`SELECT count(*) FROM api_specs WHERE project_id = $1`, pid); n != 2 {
		t.Fatalf("spec = %d, want 2", n)
	}
}

func TestT06_RenameThenUploadOldName(t *testing.T) {
	e := newITEnv(t, 50)
	pid := e.project("Proj")
	e.upload(pid, itFile{"payment.yaml", specYAML("Pay", "/pay")})

	w := e.do(http.MethodPatch, "/api/projects/"+pid+"/api-specs/payment", strings.NewReader(`{"name":"billing"}`), "application/json")
	if w.Code != http.StatusOK {
		t.Fatalf("rename: %d %s", w.Code, w.Body.String())
	}
	ren := e.audits(record.ActionAPISpecUpdated)
	if len(ren) != 1 || ren[0].Args["from"] != "payment" || ren[0].Args["to"] != "billing" || ren[0].Target != "billing" {
		t.Fatalf("audit rename: %+v", ren)
	}

	out := e.upload(pid, itFile{"payment.yaml", specYAML("Pay2", "/pay2")})
	if out.Summary.Created != 1 || out.Results[0].Status != "created" {
		t.Fatalf("nama lama harus created: %+v", out)
	}
	if got := e.endpointPaths(pid, "billing"); !has(got, "/pay") {
		t.Fatalf("billing: %v", got)
	}
	if got := e.endpointPaths(pid, "payment"); !has(got, "/pay2") || has(got, "/pay") {
		t.Fatalf("payment baru: %v", got)
	}
}

func TestT06_DeleteRemovesEndpointIndexAndAudits(t *testing.T) {
	e := newITEnv(t, 50)
	pid := e.project("Proj")
	e.upload(pid, itFile{"payment.yaml", specYAML("Pay", "/pay", "/refund")})
	if n := e.count(`SELECT count(*) FROM api_spec_endpoints e JOIN api_specs s ON s.id = e.spec_id WHERE s.project_id = $1`, pid); n != 2 {
		t.Fatalf("endpoint = %d, want 2", n)
	}

	w := e.do(http.MethodDelete, "/api/projects/"+pid+"/api-specs/payment", nil, "")
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete: %d %s", w.Code, w.Body.String())
	}
	if n := e.count(`SELECT count(*) FROM api_spec_endpoints`); n != 0 {
		t.Fatalf("indeks endpoint tersisa: %d", n)
	}
	if n := e.count(`SELECT count(*) FROM api_specs WHERE project_id = $1`, pid); n != 0 {
		t.Fatalf("spec tersisa: %d", n)
	}
	del := e.audits(record.ActionAPISpecDeleted)
	if len(del) != 1 || del[0].Args["name"] != "payment" || fmt.Sprint(del[0].Args["endpoint_count"]) != "2" {
		t.Fatalf("audit deleted: %+v", del)
	}
	for _, a := range e.audits("") {
		if a.TargetType != "api_spec" || a.ActorID != itUser || a.ProjectID != pid {
			t.Fatalf("audit field: %+v", a)
		}
		if _, leaked := a.Args["content"]; leaked {
			t.Fatalf("audit memuat content")
		}
	}
	if w := e.do(http.MethodGet, "/api/projects/"+pid+"/api-specs/payment", nil, ""); w.Code != http.StatusNotFound {
		t.Fatalf("get setelah delete: %d", w.Code)
	}
}

func TestT06_AuditFailureStillSucceeds(t *testing.T) {
	e := newITEnv(t, 50)
	pid := e.project("Proj")
	e.mu.Lock()
	e.auditFail = true
	e.mu.Unlock()

	out := e.upload(pid, itFile{"payment.yaml", specYAML("Pay", "/pay")})
	if out.Summary.Created != 1 {
		t.Fatalf("upload dengan audit gagal: %+v", out)
	}
	w := e.do(http.MethodPatch, "/api/projects/"+pid+"/api-specs/payment", strings.NewReader(`{"name":"billing"}`), "application/json")
	if w.Code != http.StatusOK {
		t.Fatalf("rename dengan audit gagal: %d %s", w.Code, w.Body.String())
	}
	w = e.do(http.MethodDelete, "/api/projects/"+pid+"/api-specs/billing", nil, "")
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete dengan audit gagal: %d %s", w.Code, w.Body.String())
	}
	if n := e.count(`SELECT count(*) FROM api_specs WHERE project_id = $1`, pid); n != 0 {
		t.Fatalf("spec tersisa: %d", n)
	}
}

func TestT06_AC7_ProjectIsolation(t *testing.T) {
	e := newITEnv(t, 50)
	a := e.project("Proj A")
	b := e.project("Proj B")
	e.upload(a, itFile{"payment.yaml", specYAML("Pay", "/pay")})

	// Non-anggota: project ada tetapi level none -> 404 di semua route.
	e.mu.Lock()
	e.level[b] = checkaccess.LevelNone
	e.mu.Unlock()
	for _, pid := range []string{b, itMissing, "bukan-uuid"} {
		base := "/api/projects/" + pid + "/api-specs"
		cases := []struct{ method, url string }{
			{http.MethodGet, base}, {http.MethodGet, base + "/payment"}, {http.MethodGet, base + "/payment/raw"},
			{http.MethodPatch, base + "/payment"}, {http.MethodDelete, base + "/payment"},
		}
		for _, c := range cases {
			body := io.Reader(nil)
			ct := ""
			if c.method == http.MethodPatch {
				body, ct = strings.NewReader(`{"name":"x"}`), "application/json"
			}
			if w := e.do(c.method, c.url, body, ct); w.Code != http.StatusNotFound {
				t.Errorf("%s %s: %d", c.method, c.url, w.Code)
			}
		}
		for _, suffix := range []string{"/upload", "/validate"} {
			var buf bytes.Buffer
			mw := multipart.NewWriter(&buf)
			fw, _ := mw.CreateFormFile("files", "x.yaml")
			_, _ = io.WriteString(fw, specYAML("X", "/x"))
			_ = mw.Close()
			if w := e.do(http.MethodPost, base+suffix, &buf, mw.FormDataContentType()); w.Code != http.StatusNotFound {
				t.Errorf("POST %s: %d", base+suffix, w.Code)
			}
		}
	}
	if n := e.count(`SELECT count(*) FROM api_specs WHERE project_id = $1`, a); n != 1 {
		t.Fatalf("spec project A berubah: %d", n)
	}

	// Bus dengan ProjectID B tidak melihat data A.
	if got := e.endpointPaths(b, "payment"); len(got) != 0 {
		t.Fatalf("bus bocor: %v", got)
	}
}
