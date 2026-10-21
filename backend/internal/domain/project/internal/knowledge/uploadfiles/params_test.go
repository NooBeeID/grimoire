package uploadfiles

import (
	"bytes"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"testing"

	"grimoire/internal/domain/project/internal/knowledge/base"
	"grimoire/lib/common/errs"
)

type tp struct {
	field, name string
	data        string
	noName      bool
}

func buildBody(t *testing.T, parts []tp) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for _, p := range parts {
		cd := `form-data; name="` + p.field + `"`
		if !p.noName {
			cd += `; filename="` + p.name + `"`
		}
		w, err := mw.CreatePart(textproto.MIMEHeader{"Content-Disposition": {cd}})
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte(p.data))
	}
	_ = mw.Close()
	return &buf, mw.FormDataContentType()
}

func req(body *bytes.Buffer, ct string) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/x", body)
	r.Header.Set("Content-Type", ct)
	return r
}

var lim = base.Limits{MaxFileBytes: 10, MaxUploadBytes: 1000, MaxUploadFiles: 3, MaxProjectFiles: 100}

func errInfo(t *testing.T, err error) (errs.Code, any) {
	t.Helper()
	var e *errs.Error
	if !errors.As(err, &e) {
		t.Fatalf("bukan *errs.Error: %v", err)
	}
	m, _ := e.Details.(map[string]any)
	return e.Code, m["reason"]
}

func TestReadPartsKeepsFolders(t *testing.T) {
	names := []string{"docs/auth/login.md", "a/b/c/d.md", "catatan/é spasi.md", "../x.md"}
	var ps []tp
	for _, n := range names {
		ps = append(ps, tp{field: "files", name: n, data: "x"})
	}
	b, ct := buildBody(t, ps)
	got, err := readParts(req(b, ct), base.Limits{MaxFileBytes: 10, MaxUploadBytes: 1000, MaxUploadFiles: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Parts) != len(names) {
		t.Fatalf("%d parts", len(got.Parts))
	}
	for i, n := range names {
		if got.Parts[i].Name != n || got.Parts[i].Index != i {
			t.Fatalf("part %d = %+v, want %q", i, got.Parts[i], n)
		}
	}
}

func TestReadPartsIgnoresOtherFields(t *testing.T) {
	b, ct := buildBody(t, []tp{
		{field: "meta", name: "m.md", data: "zzzz"},
		{field: "files", noName: true, data: "yyyy"},
		{field: "files", name: "a.md", data: "abc"},
	})
	got, err := readParts(req(b, ct), lim)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Parts) != 1 || got.TotalBytes != 3 || string(got.Parts[0].Data) != "abc" {
		t.Fatalf("%+v", got)
	}
}

func TestReadPartsPerFileLimit(t *testing.T) {
	ok := strings.Repeat("a", 13) // MaxFileBytes+3
	big := strings.Repeat("a", 14)
	b, ct := buildBody(t, []tp{
		{field: "files", name: "ok.md", data: ok},
		{field: "files", name: "big.md", data: big},
		{field: "files", name: "next.md", data: "next"},
	})
	got, err := readParts(req(b, ct), lim)
	if err != nil {
		t.Fatal(err)
	}
	if got.Parts[0].TooLarge || len(got.Parts[0].Data) != 13 {
		t.Fatalf("%+v", got.Parts[0])
	}
	if !got.Parts[1].TooLarge || got.Parts[1].Data != nil {
		t.Fatalf("%+v", got.Parts[1])
	}
	if string(got.Parts[2].Data) != "next" {
		t.Fatalf("part berikutnya rusak: %+v", got.Parts[2])
	}
	if got.TotalBytes != 13+14+4 {
		t.Fatalf("total %d", got.TotalBytes)
	}
}

func TestReadPartsOverCount(t *testing.T) {
	var ps []tp
	for i := 0; i < 4; i++ {
		ps = append(ps, tp{field: "files", name: "f.md", data: "abcde"})
	}
	b, ct := buildBody(t, ps)
	got, err := readParts(req(b, ct), lim)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Parts) != 4 || !got.Parts[3].OverCount || got.Parts[3].Data != nil || got.Parts[2].OverCount {
		t.Fatalf("%+v", got.Parts)
	}
	if got.TotalBytes != 20 {
		t.Fatalf("total %d", got.TotalBytes)
	}
}

func TestReadPartsTotalTooLarge(t *testing.T) {
	l := base.Limits{MaxFileBytes: 100, MaxUploadBytes: 20, MaxUploadFiles: 10}
	b, ct := buildBody(t, []tp{
		{field: "files", name: "a.md", data: strings.Repeat("a", 15)},
		{field: "files", name: "b.md", data: strings.Repeat("b", 15)},
	})
	_, err := readParts(req(b, ct), l)
	code, reason := errInfo(t, err)
	if code != errs.CodePayloadTooLarge || reason != base.ReasonUploadTooLarge {
		t.Fatalf("%v %v", code, reason)
	}
	var e *errs.Error
	errors.As(err, &e)
	if e.Details.(map[string]any)["max_bytes"] != int64(20) {
		t.Fatalf("%v", e.Details)
	}
}

func TestReadPartsDrainedBytesCountTowardTotal(t *testing.T) {
	l := base.Limits{MaxFileBytes: 5, MaxUploadBytes: 50, MaxUploadFiles: 10}
	b, ct := buildBody(t, []tp{{field: "files", name: "a.md", data: strings.Repeat("a", 200)}})
	_, err := readParts(req(b, ct), l)
	if _, reason := errInfo(t, err); reason != base.ReasonUploadTooLarge {
		t.Fatalf("%v", err)
	}
}

func TestReadPartsNotMultipart(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(`{}`))
	r.Header.Set("Content-Type", "application/json")
	_, err := readParts(r, lim)
	if code, reason := errInfo(t, err); code != errs.CodeInvalid || reason != base.ReasonNotMultipart {
		t.Fatalf("%v %v", code, reason)
	}
	r = httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(`x`))
	r.Header.Set("Content-Type", "multipart/form-data")
	if _, err = readParts(r, lim); err == nil {
		t.Fatal("want error")
	} else if _, reason := errInfo(t, err); reason != base.ReasonNotMultipart {
		t.Fatal(reason)
	}
}

func TestReadPartsTruncatedBody(t *testing.T) {
	b, ct := buildBody(t, []tp{{field: "files", name: "a.md", data: "abcdef"}})
	cut := bytes.NewBuffer(b.Bytes()[:b.Len()-30])
	_, err := readParts(req(cut, ct), lim)
	if _, reason := errInfo(t, err); reason != base.ReasonNotMultipart {
		t.Fatalf("%v", err)
	}
}

func TestReadPartsBrokenDisposition(t *testing.T) {
	body := "--b\r\nContent-Disposition: form-data; name=\"files\"; filename=\"unterminated\r\n\r\nabc\r\n--b--\r\n"
	r := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(body))
	r.Header.Set("Content-Type", "multipart/form-data; boundary=b")
	got, err := readParts(r, lim)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Parts) != 1 || got.Parts[0].Name != "" {
		t.Fatalf("%+v", got.Parts)
	}
}

func TestReadPartsMaxBytesReader(t *testing.T) {
	b, ct := buildBody(t, []tp{{field: "files", name: "a.md", data: "abcdefgh"}})
	r := req(b, ct)
	w := httptest.NewRecorder()
	r.Body = http.MaxBytesReader(w, r.Body, 100)
	l := base.Limits{MaxFileBytes: 1000, MaxUploadBytes: 100000, MaxUploadFiles: 10}
	// body < 100 byte tidak memicu; kecilkan limit.
	r.Body = http.MaxBytesReader(w, req(b, ct).Body, 60)
	_, err := readParts(r, l)
	if _, reason := errInfo(t, err); reason != base.ReasonUploadTooLarge {
		t.Fatalf("%v", err)
	}
}
