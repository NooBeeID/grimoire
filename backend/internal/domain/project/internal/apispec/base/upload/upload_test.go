package upload

import (
	"bytes"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"grimoire/internal/domain/project/internal/apispec/base"
	"grimoire/internal/infra/middleware"
)

type part struct{ field, filename, content string }

func build(t *testing.T, parts ...part) (*http.Request, string) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	for _, p := range parts {
		if p.filename == "" {
			if err := w.WriteField(p.field, p.content); err != nil {
				t.Fatal(err)
			}
			continue
		}
		pw, err := w.CreatePart(map[string][]string{
			"Content-Disposition": {`form-data; name="` + p.field + `"; filename="` + p.filename + `"`},
			"Content-Type":        {"application/yaml"},
		})
		if err != nil {
			t.Fatal(err)
		}
		_, _ = pw.Write([]byte(p.content))
	}
	_ = w.Close()
	r := httptest.NewRequest(http.MethodPost, "/x", &buf)
	r.Header.Set("Content-Type", w.FormDataContentType())
	return r, w.FormDataContentType()
}

func TestRead_DirectoriesKeptInRawPath(t *testing.T) {
	r, _ := build(t, part{"files", "v1/payment.yaml", "a"}, part{"files", `win\dir\auth.json`, "b"})
	f, err := Read(r, 100, 10)
	if err != nil || len(f.Files) != 2 {
		t.Fatalf("%+v %v", f, err)
	}
	if f.Files[0].RawPath != "v1/payment.yaml" || f.Files[0].FileName != "payment.yaml" || string(f.Files[0].Content) != "a" {
		t.Fatalf("%+v", f.Files[0])
	}
	if f.Files[1].RawPath != `win\dir\auth.json` || f.Files[1].FileName != "auth.json" {
		t.Fatalf("%+v", f.Files[1])
	}
}

func TestRead_TooLargeThenNextPartStillRead(t *testing.T) {
	r, _ := build(t, part{"files", "big.yaml", strings.Repeat("x", 50)}, part{"files", "ok.yaml", "fine"})
	f, err := Read(r, 10, 10)
	if err != nil || len(f.Files) != 2 {
		t.Fatalf("%+v %v", f, err)
	}
	if !f.Files[0].TooLarge || f.Files[0].Content != nil || f.Files[1].TooLarge || string(f.Files[1].Content) != "fine" {
		t.Fatalf("%+v", f.Files)
	}
}

func TestRead_ExactlyMaxIsAllowed(t *testing.T) {
	r, _ := build(t, part{"files", "a.yaml", strings.Repeat("x", 10)})
	f, err := Read(r, 10, 10)
	if err != nil || f.Files[0].TooLarge {
		t.Fatalf("%+v %v", f, err)
	}
}

func TestRead_TooManyFiles(t *testing.T) {
	r, _ := build(t, part{"files", "a.yaml", "1"}, part{"files", "b.yaml", "2"}, part{"files", "c.yaml", "3"})
	f, err := Read(r, 10, 2)
	if err != nil || len(f.Files) != 3 {
		t.Fatalf("%+v %v", f, err)
	}
	if f.Files[1].Rejected != "" || f.Files[2].Rejected != base.ReasonTooManyFiles || f.Files[2].Content != nil {
		t.Fatalf("%+v", f.Files)
	}
}

func TestRead_BodyLimitIsPayloadTooLarge(t *testing.T) {
	r, _ := build(t, part{"files", "a.yaml", strings.Repeat("x", 5000)})
	rec := httptest.NewRecorder()
	var gotErr error
	h := middleware.BodyLimit(500)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, gotErr = Read(r, 1<<20, 10)
	}))
	h.ServeHTTP(rec, r)
	if !errors.Is(gotErr, base.ErrPayloadTooLarge) {
		t.Fatalf("err=%v", gotErr)
	}
}

func TestRead_NotMultipart(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader("{}"))
	r.Header.Set("Content-Type", "application/json")
	if _, err := Read(r, 10, 10); !errors.Is(err, base.ErrNotMultipart) {
		t.Fatalf("%v", err)
	}
	r2 := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(""))
	if _, err := Read(r2, 10, 10); !errors.Is(err, base.ErrNotMultipart) {
		t.Fatalf("%v", err)
	}
}

func TestRead_NoFiles(t *testing.T) {
	r, _ := build(t, part{"other", "", "x"})
	if _, err := Read(r, 10, 10); !errors.Is(err, base.ErrNoFiles) {
		t.Fatalf("%v", err)
	}
}

func TestRead_NameRules(t *testing.T) {
	r, _ := build(t, part{"name", "", "payment-v2"}, part{"files", "a.yaml", "1"})
	f, err := Read(r, 10, 10)
	if err != nil || f.Name == nil || *f.Name != "payment-v2" {
		t.Fatalf("%+v %v", f, err)
	}
	r2, _ := build(t, part{"name", "", "x"}, part{"files", "a.yaml", "1"}, part{"files", "b.yaml", "2"})
	if _, err := Read(r2, 10, 10); !errors.Is(err, base.ErrNameWithMultipleFiles) {
		t.Fatalf("%v", err)
	}
}

func TestRead_UnknownFieldsIgnored(t *testing.T) {
	r, _ := build(t, part{"junk", "", "zzz"}, part{"extra", "x.bin", "bin"}, part{"files", "a.yaml", "1"})
	f, err := Read(r, 10, 10)
	if err != nil || len(f.Files) != 1 || f.Name != nil {
		t.Fatalf("%+v %v", f, err)
	}
}
