package uploadspecs

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"grimoire/internal/domain/project/internal/apispec/base"
	"grimoire/internal/infra/middleware"
	"grimoire/lib/common/errs"
)

type fakeExec struct {
	in    Input
	calls int
	out   Output
	err   error
}

func (f *fakeExec) Execute(_ context.Context, in Input) (Output, error) {
	f.calls++
	f.in = in
	return f.out, f.err
}

type part struct{ field, filename, content string }

func multipartBody(t *testing.T, parts ...part) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for _, p := range parts {
		if p.filename == "" {
			if err := mw.WriteField(p.field, p.content); err != nil {
				t.Fatal(err)
			}
			continue
		}
		h := make(map[string][]string)
		h["Content-Disposition"] = []string{`form-data; name="` + p.field + `"; filename="` + p.filename + `"`}
		w, err := mw.CreatePart(h)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte(p.content))
	}
	_ = mw.Close()
	return &buf, mw.FormDataContentType()
}

func newRouter(h *Handler, bodyLimit int64) http.Handler {
	r := chi.NewRouter()
	r.With(middleware.BodyLimit(bodyLimit)).Post("/projects/{projectID}/api-specs/upload", h.ServeHTTP)
	return r
}

func post(t *testing.T, h http.Handler, ct string, body *bytes.Buffer, ua string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/projects/p1/api-specs/upload", body)
	req.Header.Set("Content-Type", ct)
	if ua != "" {
		req.Header.Set("User-Agent", ua)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func reasonOf(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var env struct {
		Error struct {
			Code    string         `json:"code"`
			Details map[string]any `json:"details"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	r, _ := env.Error.Details["reason"].(string)
	return r
}

func TestUploadSpecs_HandlerSuccess(t *testing.T) {
	sp := "specs/payment.yaml"
	ex := &fakeExec{out: Output{
		Results: []base.FileResult{
			{File: "payment.yaml", SourcePath: &sp, Name: "payment", Status: base.StatusCreated, Issues: []base.Issue{}, OpenAPIVersion: "3.0.3", Title: "Pay", APIVersion: "1", EndpointCount: 0},
			{File: "n.txt", Status: base.StatusSkipped, ReasonCode: base.ReasonNotSpecFile, Reason: "x", Issues: []base.Issue{}},
		},
		Summary: base.UploadSummary{Created: 1, Skipped: 1},
	}}
	h := newRouter(&Handler{uc: ex, maxFileBytes: 1000, maxFiles: 10}, 1<<20)
	body, ct := multipartBody(t, part{"files", sp, "a"}, part{"files", "specs/n.txt", "b"}, part{"zzz", "", "ignored"})
	rec := post(t, h, ct, body, strings.Repeat("x", 600))
	if rec.Code != 200 || !strings.HasPrefix(rec.Header().Get("Content-Type"), "application/json") {
		t.Fatalf("%d %s", rec.Code, rec.Header().Get("Content-Type"))
	}
	if ex.in.ProjectID != "p1" || len(ex.in.Files) != 2 || ex.in.Files[0].RawPath != sp || len(ex.in.Meta.UserAgent) != 512 {
		t.Fatalf("%+v", ex.in)
	}
	var got struct {
		Results []map[string]any `json:"results"`
		Summary map[string]int   `json:"summary"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Summary["created"] != 1 || got.Summary["skipped"] != 1 || len(got.Results) != 2 {
		t.Fatalf("%s", rec.Body.String())
	}
	if got.Results[0]["endpoint_count"] != float64(0) || got.Results[1]["endpoint_count"] != nil || got.Results[1]["name"] != nil {
		t.Fatalf("%v", got.Results)
	}
}

func TestUploadSpecs_HandlerRequestErrors(t *testing.T) {
	ex := &fakeExec{}
	h := newRouter(&Handler{uc: ex, maxFileBytes: 1000, maxFiles: 10}, 1<<20)

	rec := post(t, h, "application/json", bytes.NewBufferString("{}"), "")
	if rec.Code != 400 || reasonOf(t, rec) != base.ReasonNotMultipart {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	body, ct := multipartBody(t, part{"other", "", "x"})
	rec = post(t, h, ct, body, "")
	if rec.Code != 400 || reasonOf(t, rec) != base.ReasonNoFiles {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	body, ct = multipartBody(t, part{"files", "a.yaml", "a"}, part{"files", "b.yaml", "b"}, part{"name", "", "x"})
	rec = post(t, h, ct, body, "")
	if rec.Code != 400 || reasonOf(t, rec) != base.ReasonNameWithMultipleFiles {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	if ex.calls != 0 {
		t.Fatal("executor dipanggil")
	}
}

func TestUploadSpecs_HandlerBodyLimit(t *testing.T) {
	ex := &fakeExec{}
	h := newRouter(&Handler{uc: ex, maxFileBytes: 1 << 20, maxFiles: 10}, 200)
	body, ct := multipartBody(t, part{"files", "a.yaml", strings.Repeat("a", 5000)})
	rec := post(t, h, ct, body, "")
	if rec.Code != 413 || ex.calls != 0 {
		t.Fatalf("%d calls=%d", rec.Code, ex.calls)
	}
	if !strings.Contains(rec.Body.String(), "PAYLOAD_TOO_LARGE") {
		t.Fatal(rec.Body.String())
	}
}

func TestUploadSpecs_HandlerPerFileLimits(t *testing.T) {
	ex := &fakeExec{}
	h := newRouter(&Handler{uc: ex, maxFileBytes: 10, maxFiles: 2}, 1<<20)
	body, ct := multipartBody(t,
		part{"files", "big.yaml", strings.Repeat("a", 50)},
		part{"files", "ok.yaml", "tiny"},
		part{"files", "extra.yaml", "x"})
	rec := post(t, h, ct, body, "")
	if rec.Code != 200 || len(ex.in.Files) != 3 {
		t.Fatalf("%d %d", rec.Code, len(ex.in.Files))
	}
	if !ex.in.Files[0].TooLarge || string(ex.in.Files[1].Content) != "tiny" || ex.in.Files[2].Rejected != base.ReasonTooManyFiles {
		t.Fatalf("%+v", ex.in.Files)
	}
}

func TestUploadSpecs_HandlerExecutorErrors(t *testing.T) {
	ex := &fakeExec{err: base.ErrProjectNotFound}
	h := newRouter(&Handler{uc: ex, maxFileBytes: 1000, maxFiles: 10}, 1<<20)
	body, ct := multipartBody(t, part{"files", "a.yaml", "a"})
	rec := post(t, h, ct, body, "")
	if rec.Code != 404 || !strings.Contains(rec.Body.String(), "project tidak ditemukan") {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	var ee *errs.Error
	if !errors.As(base.ErrProjectNotFound, &ee) {
		t.Fatal("sentinel")
	}
	ex.err = errors.New("pq: secret detail")
	body, ct = multipartBody(t, part{"files", "a.yaml", "a"})
	rec = post(t, h, ct, body, "")
	if rec.Code != 500 || strings.Contains(rec.Body.String(), "secret") {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
}
