package validatespec

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-chi/chi/v5"

	"grimoire/internal/domain/project/internal/apispec/base"
	"grimoire/internal/infra/middleware"
	"grimoire/lib/common/errs"
	"grimoire/lib/common/reqctx"
)

type hpart struct{ field, filename, content string }

func multipartBody(t *testing.T, parts ...hpart) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	for _, p := range parts {
		if p.filename == "" {
			_ = w.WriteField(p.field, p.content)
			continue
		}
		pw, _ := w.CreatePart(map[string][]string{
			"Content-Disposition": {`form-data; name="` + p.field + `"; filename="` + p.filename + `"`},
		})
		_, _ = pw.Write([]byte(p.content))
	}
	_ = w.Close()
	return &buf, w.FormDataContentType()
}

func fixture(t *testing.T, n string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "base", "openapi", "testdata", n))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// serve merakit router seperti Init tetapi dengan guard palsu; auth=true menaruh identitas.
func serve(t *testing.T, guardErr error, bodyLimit int64, auth bool, body *bytes.Buffer, ct string) *httptest.ResponseRecorder {
	t.Helper()
	h := newHandler(newUC(guardErr), 1024, 10)
	r := chi.NewRouter()
	r.With(middleware.RequireAuth(), middleware.BodyLimit(bodyLimit)).
		Post("/projects/{projectID}/api-specs/validate", h.ServeHTTP)
	req := httptest.NewRequest(http.MethodPost, "/projects/p1/api-specs/validate", body)
	req.Header.Set("Content-Type", ct)
	if auth {
		req = req.WithContext(reqctx.WithIdentity(context.Background(), reqctx.Identity{UserID: "u", Role: "developer", Email: "a@b"}))
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func errBody(t *testing.T, rec *httptest.ResponseRecorder) (code, reason string) {
	t.Helper()
	var e struct {
		Error struct {
			Code    string         `json:"code"`
			Details map[string]any `json:"details"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &e); err != nil {
		t.Fatal(err)
	}
	r, _ := e.Error.Details["reason"].(string)
	return e.Error.Code, r
}

func TestValidateHandler_OK(t *testing.T) {
	b, ct := multipartBody(t,
		hpart{"files", "specs/ok.yaml", "openapi: 3.0.0\ninfo: {title: T, version: '1'}\npaths: {}\n"},
		hpart{"files", "specs/bad.yaml", "foo: bar"},
		hpart{"files", "specs/README.md", "# x"})
	rec := serve2(t, b, ct, 1<<20)
	if rec.Code != 200 {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	var out struct {
		Results []map[string]any `json:"results"`
		Summary map[string]int   `json:"summary"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Summary["valid"] != 1 || out.Summary["skipped"] != 1 || out.Summary["rejected"] != 1 {
		t.Fatalf("%v", out.Summary)
	}
	if out.Results[0]["status"] != "valid" || out.Results[0]["reason_code"] != nil || out.Results[2]["name"] != nil {
		t.Fatalf("%v", out.Results)
	}
	if is, ok := out.Results[2]["issues"].([]any); !ok || len(is) != 0 {
		t.Fatalf("issues %v", out.Results[2]["issues"])
	}
}

func serve2(t *testing.T, b *bytes.Buffer, ct string, limit int64) *httptest.ResponseRecorder {
	return serve(t, nil, limit, true, b, ct)
}

func TestValidateHandler_Errors(t *testing.T) {
	t.Run("not multipart", func(t *testing.T) {
		rec := serve2(t, bytes.NewBufferString("{}"), "application/json", 1<<20)
		if c, r := errBody(t, rec); rec.Code != 400 || c != "INVALID_ARGUMENT" || r != "NOT_MULTIPART" {
			t.Fatalf("%d %s", rec.Code, rec.Body)
		}
	})
	t.Run("no files", func(t *testing.T) {
		b, ct := multipartBody(t, hpart{"name", "", "x"})
		rec := serve2(t, b, ct, 1<<20)
		if _, r := errBody(t, rec); rec.Code != 400 || r != "NO_FILES" {
			t.Fatalf("%d %s", rec.Code, rec.Body)
		}
	})
	t.Run("name with multiple files", func(t *testing.T) {
		b, ct := multipartBody(t, hpart{"name", "", "x"}, hpart{"files", "a.yaml", "1"}, hpart{"files", "b.yaml", "2"})
		rec := serve2(t, b, ct, 1<<20)
		if _, r := errBody(t, rec); rec.Code != 400 || r != "NAME_WITH_MULTIPLE_FILES" {
			t.Fatalf("%d %s", rec.Code, rec.Body)
		}
	})
	t.Run("invalid explicit name", func(t *testing.T) {
		b, ct := multipartBody(t, hpart{"name", "", "Bad Name"}, hpart{"files", "a.yaml", "1"})
		rec := serve2(t, b, ct, 1<<20)
		if _, r := errBody(t, rec); rec.Code != 400 || r != "INVALID_NAME" {
			t.Fatalf("%d %s", rec.Code, rec.Body)
		}
	})
	t.Run("empty name treated as absent", func(t *testing.T) {
		b, ct := multipartBody(t, hpart{"name", "", "  "}, hpart{"files", "my-api.yaml", "foo: bar"})
		rec := serve2(t, b, ct, 1<<20)
		var out struct {
			Results []struct {
				Name string `json:"name"`
			} `json:"results"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
		if rec.Code != 200 || out.Results[0].Name != "my-api" {
			t.Fatalf("%d %s", rec.Code, rec.Body)
		}
	})
	t.Run("body too large", func(t *testing.T) {
		b, ct := multipartBody(t, hpart{"files", "a.yaml", string(bytes.Repeat([]byte("a"), 4096))})
		rec := serve2(t, b, ct, 512)
		if c, _ := errBody(t, rec); rec.Code != 413 || c != "PAYLOAD_TOO_LARGE" {
			t.Fatalf("%d %s", rec.Code, rec.Body)
		}
	})
	t.Run("non member 404", func(t *testing.T) {
		b, ct := multipartBody(t, hpart{"files", "a.yaml", "foo: bar"})
		rec := serve(t, base.ErrProjectNotFound, 1<<20, true, b, ct)
		if c, _ := errBody(t, rec); rec.Code != 404 || c != "NOT_FOUND" {
			t.Fatalf("%d %s", rec.Code, rec.Body)
		}
	})
	t.Run("no session 401", func(t *testing.T) {
		b, ct := multipartBody(t, hpart{"files", "a.yaml", "foo: bar"})
		rec := serve(t, nil, 1<<20, false, b, ct)
		if rec.Code != 401 {
			t.Fatalf("%d %s", rec.Code, rec.Body)
		}
	})
	t.Run("infra error 500", func(t *testing.T) {
		b, ct := multipartBody(t, hpart{"files", "a.yaml", "foo: bar"})
		rec := serve(t, errs.New(errs.CodeInternal, "boom"), 1<<20, true, b, ct)
		if rec.Code != 500 {
			t.Fatalf("%d", rec.Code)
		}
	})
}

func TestValidateHandler_FileTooLargeKeepsOthers(t *testing.T) {
	big := string(bytes.Repeat([]byte("a"), 2048)) // batas file 1024 di serve()
	b, ct := multipartBody(t,
		hpart{"files", "big.yaml", big},
		hpart{"files", "ok.yaml", "openapi: 3.0.0\ninfo: {title: T, version: '1'}\npaths: {}\n"})
	rec := serve2(t, b, ct, 1<<20)
	var out struct {
		Results []struct {
			Status     string `json:"status"`
			ReasonCode string `json:"reason_code"`
		} `json:"results"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if rec.Code != 200 || out.Results[0].ReasonCode != "FILE_TOO_LARGE" || out.Results[1].Status != "valid" {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
}
