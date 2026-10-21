package validatespec

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"grimoire/internal/domain/project/internal/apispec/base"
	"grimoire/internal/domain/project/internal/apispec/base/upload"
)

type fakeGuard struct{ err error }

func (f fakeGuard) RequireMember(context.Context, string) error { return f.err }

func newUC(err error) *Usecase {
	return NewUsecase(fakeGuard{err}, base.UploadLimits{MaxFileBytes: 5242880, MaxFiles: 500})
}

func loadFixture(t *testing.T, name string) upload.File {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "base", "openapi", "testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return upload.File{FileName: name, RawPath: name, Content: b}
}

func exec(t *testing.T, files ...upload.File) Result {
	t.Helper()
	res, err := newUC(nil).Execute(context.Background(), Params{ProjectID: "p", Files: files})
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestValidate_NonMember_404(t *testing.T) {
	start := time.Now()
	res, err := newUC(base.ErrProjectNotFound).Execute(context.Background(),
		Params{ProjectID: "p", Files: []upload.File{loadFixture(t, "billion-laughs.yaml")}})
	if !errors.Is(err, base.ErrProjectNotFound) || len(res.Results) != 0 {
		t.Fatalf("%v %+v", err, res)
	}
	if time.Since(start) > time.Second {
		t.Fatal("tidak boleh memproses file")
	}
}

func TestValidate_CheckerError_FailClosed(t *testing.T) {
	boom := errors.New("boom")
	res, err := newUC(boom).Execute(context.Background(), Params{ProjectID: "p"})
	if !errors.Is(err, boom) || len(res.Results) != 0 {
		t.Fatalf("%v", err)
	}
}

func TestValidate_ExplicitNameInvalid_400(t *testing.T) {
	bad := "Bad Name"
	res, err := newUC(nil).Execute(context.Background(),
		Params{ProjectID: "p", Name: &bad, Files: []upload.File{loadFixture(t, "petstore-3.0.yaml")}})
	if !errors.Is(err, base.ErrInvalidName) || len(res.Results) != 0 {
		t.Fatalf("%v", err)
	}
}

func TestValidate_PerFileCases(t *testing.T) {
	cases := []struct {
		fixture   string
		status    base.FileStatus
		reason    string
		issueCode string
	}{
		{"petstore-3.0.yaml", base.StatusValid, "", ""},
		{"petstore-3.1.json", base.StatusValid, "", ""},
		{"webhooks-only-3.1.yaml", base.StatusValid, "", ""},
		{"bom.json", base.StatusValid, "", ""},
		{"merge-keys.yaml", base.StatusValid, "", ""},
		{"ref-in-example.yaml", base.StatusValid, "", ""},
		{"package.json", base.StatusRejected, "INVALID_SPEC", "NOT_OPENAPI"},
		{"broken.yaml", base.StatusRejected, "INVALID_SPEC", "SYNTAX"},
		{"broken.json", base.StatusRejected, "INVALID_SPEC", "SYNTAX"},
		{"swagger-2.0.yaml", base.StatusRejected, "INVALID_SPEC", "NOT_OPENAPI_3"},
		{"ref-http.yaml", base.StatusRejected, "INVALID_SPEC", "EXTERNAL_REF"},
		{"ref-file.yaml", base.StatusRejected, "INVALID_SPEC", "EXTERNAL_REF"},
		{"ref-unresolved.yaml", base.StatusRejected, "INVALID_SPEC", "UNRESOLVED_REF"},
		{"dup-operationid.yaml", base.StatusRejected, "INVALID_SPEC", "DUPLICATE_OPERATION_ID"},
		{"billion-laughs.yaml", base.StatusRejected, "INVALID_SPEC", "YAML_LIMIT"},
		{"deep.yaml", base.StatusRejected, "INVALID_SPEC", "YAML_LIMIT"},
		{"alias-cycle.yaml", base.StatusRejected, "INVALID_SPEC", "YAML_LIMIT"},
		{"invalid-utf8.yaml", base.StatusRejected, "INVALID_SPEC", "INVALID_ENCODING"},
		{"nul.json", base.StatusRejected, "INVALID_SPEC", "INVALID_ENCODING"},
		{"multi-doc.yaml", base.StatusRejected, "INVALID_SPEC", "MULTI_DOCUMENT"},
		{"many-errors.yaml", base.StatusRejected, "INVALID_SPEC", ""},
	}
	for _, c := range cases {
		t.Run(c.fixture, func(t *testing.T) {
			r := exec(t, loadFixture(t, c.fixture)).Results[0]
			if r.Status != c.status || r.ReasonCode != c.reason {
				t.Fatalf("%s/%s: %+v", r.Status, r.ReasonCode, r)
			}
			if r.Issues == nil {
				t.Fatal("Issues nil")
			}
			if c.status == base.StatusValid {
				if r.OpenAPIVersion == "" || len(r.Issues) != 0 || r.Reason != "" {
					t.Fatalf("%+v", r)
				}
				return
			}
			if len(r.Issues) == 0 || r.Reason != r.Issues[0].Message {
				t.Fatalf("%+v", r)
			}
			if c.issueCode != "" && r.Issues[0].Code != c.issueCode {
				t.Fatalf("issue %s want %s", r.Issues[0].Code, c.issueCode)
			}
		})
	}
}

func TestValidate_VersionsAndEndpoints(t *testing.T) {
	r30 := exec(t, loadFixture(t, "petstore-3.0.yaml")).Results[0]
	r31 := exec(t, loadFixture(t, "petstore-3.1.json")).Results[0]
	wh := exec(t, loadFixture(t, "webhooks-only-3.1.yaml")).Results[0]
	if !strings.HasPrefix(r30.OpenAPIVersion, "3.0") || !strings.HasPrefix(r31.OpenAPIVersion, "3.1") {
		t.Fatalf("%s %s", r30.OpenAPIVersion, r31.OpenAPIVersion)
	}
	if r30.EndpointCount == 0 || r31.EndpointCount == 0 || wh.EndpointCount != 0 {
		t.Fatalf("%d %d %d", r30.EndpointCount, r31.EndpointCount, wh.EndpointCount)
	}
}

func TestValidate_ManyErrorsCapped(t *testing.T) {
	r := exec(t, loadFixture(t, "many-errors.yaml")).Results[0]
	if len(r.Issues) != 20 {
		t.Fatalf("issues %d", len(r.Issues))
	}
	for i := 1; i < len(r.Issues); i++ {
		if r.Issues[i].Line < r.Issues[i-1].Line {
			t.Fatal("tidak urut baris")
		}
	}
}

func TestValidate_Skipped(t *testing.T) {
	for _, n := range []string{"README.md", "notes.txt", "spec.yaml.bak", "noext"} {
		r := exec(t, upload.File{FileName: n, RawPath: "docs/" + n, Content: []byte("x")}).Results[0]
		if r.Status != base.StatusSkipped || r.ReasonCode != "NOT_SPEC_FILE" || r.Name != "" {
			t.Fatalf("%s: %+v", n, r)
		}
		if r.SourcePath == nil || *r.SourcePath != "docs/"+n {
			t.Fatalf("source path %v", r.SourcePath)
		}
	}
}

func TestValidate_UploadFlags(t *testing.T) {
	big := exec(t, upload.File{FileName: "a.yaml", RawPath: "a.yaml", TooLarge: true}).Results[0]
	if big.Status != base.StatusRejected || big.ReasonCode != "FILE_TOO_LARGE" || !strings.Contains(big.Reason, "5242880") {
		t.Fatalf("%+v", big)
	}
	many := exec(t, upload.File{FileName: "a.yaml", RawPath: "a.yaml", Rejected: base.ReasonTooManyFiles}).Results[0]
	if many.ReasonCode != "TOO_MANY_FILES" || many.Status != base.StatusRejected {
		t.Fatalf("%+v", many)
	}
}

func TestValidate_InvalidPathAndName(t *testing.T) {
	for _, p := range []string{"/etc/x.yaml", "a/../b.yaml"} {
		r := exec(t, upload.File{FileName: "x.yaml", RawPath: p, Content: []byte("x")}).Results[0]
		if r.ReasonCode != "INVALID_PATH" || r.SourcePath != nil {
			t.Fatalf("%s: %+v", p, r)
		}
	}
	r := exec(t, upload.File{FileName: ".yaml", RawPath: ".yaml", Content: []byte("x")}).Results[0]
	if r.ReasonCode != "INVALID_NAME" || r.Name != "" {
		t.Fatalf("%+v", r)
	}
}

func TestValidate_DuplicateNameInUpload(t *testing.T) {
	good := loadFixture(t, "petstore-3.0.yaml").Content
	bad := []byte("not: openapi")
	rs := exec(t,
		upload.File{FileName: "payment.yaml", RawPath: "v1/payment.yaml", Content: good},
		upload.File{FileName: "payment.yaml", RawPath: "v2/payment.yaml", Content: good}).Results
	if rs[0].Status != base.StatusValid || rs[1].ReasonCode != "DUPLICATE_NAME_IN_UPLOAD" {
		t.Fatalf("%+v", rs)
	}
	rs = exec(t,
		upload.File{FileName: "payment.yaml", RawPath: "v1/payment.yaml", Content: bad},
		upload.File{FileName: "payment.yaml", RawPath: "v2/payment.yaml", Content: good}).Results
	if rs[0].ReasonCode != "INVALID_SPEC" || rs[1].ReasonCode != "DUPLICATE_NAME_IN_UPLOAD" {
		t.Fatalf("%+v", rs)
	}
}

func TestValidate_ExplicitName(t *testing.T) {
	n := "payment-v2"
	res, err := newUC(nil).Execute(context.Background(), Params{ProjectID: "p", Name: &n,
		Files: []upload.File{loadFixture(t, "petstore-3.0.yaml")}})
	if err != nil || res.Results[0].Name != "payment-v2" {
		t.Fatalf("%v %+v", err, res)
	}
}

func TestValidate_SourcePathNormalized(t *testing.T) {
	r := exec(t, upload.File{FileName: "c.yaml", RawPath: `a\b\c.yaml`, Content: loadFixture(t, "petstore-3.0.yaml").Content}).Results[0]
	if r.SourcePath == nil || *r.SourcePath != "a/b/c.yaml" {
		t.Fatalf("%v", r.SourcePath)
	}
}

func TestValidate_MixedKeepsOrderAndSummary(t *testing.T) {
	res := exec(t,
		loadFixture(t, "petstore-3.0.yaml"),
		upload.File{FileName: "README.md", RawPath: "README.md", Content: []byte("#")},
		loadFixture(t, "swagger-2.0.yaml"),
		loadFixture(t, "broken.yaml"))
	want := []base.FileStatus{base.StatusValid, base.StatusSkipped, base.StatusRejected, base.StatusRejected}
	for i, w := range want {
		if res.Results[i].Status != w {
			t.Fatalf("%d: %s", i, res.Results[i].Status)
		}
	}
	if res.Summary != (base.ValidateSummaryDTO{Valid: 1, Skipped: 1, Rejected: 2}) {
		t.Fatalf("%+v", res.Summary)
	}
}

func TestValidate_NoFilesResultsNotNil(t *testing.T) {
	res := exec(t)
	if res.Results == nil {
		t.Fatal("nil")
	}
}

func TestT06_AC3_BillionLaughsCheap(t *testing.T) {
	f := loadFixture(t, "billion-laughs.yaml")
	var m0, m1 runtime.MemStats
	runtime.ReadMemStats(&m0)
	start := time.Now()
	r := exec(t, f).Results[0]
	runtime.ReadMemStats(&m1)
	if r.Status != base.StatusRejected || time.Since(start) > time.Second || m1.TotalAlloc-m0.TotalAlloc > 50<<20 {
		t.Fatalf("%+v %v %d", r.Status, time.Since(start), m1.TotalAlloc-m0.TotalAlloc)
	}
}

func TestValidate_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := newUC(nil).Execute(ctx, Params{ProjectID: "p", Files: []upload.File{loadFixture(t, "petstore-3.0.yaml")}})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("%v", err)
	}
}

func TestValidate_NoNameExistsField(t *testing.T) {
	res := exec(t, loadFixture(t, "petstore-3.0.yaml"))
	dtos := make([]base.FileResultDTO, len(res.Results))
	for i, r := range res.Results {
		dtos[i] = base.ToFileResultDTO(r)
	}
	b, _ := json.Marshal(response{Results: dtos, Summary: res.Summary})
	if strings.Contains(string(b), "name_exists") {
		t.Fatal(string(b))
	}
}
