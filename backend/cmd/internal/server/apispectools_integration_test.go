//go:build integration

package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"sort"
	"strings"
	"testing"
	"time"
)

// Tes integrasi T09 (overview 10.3). Memakai helper T07/T08 di package yang sama:
// setupMCP, mcpEnv.mcpCall, mcpEnv.newProject, mcpEnv.countAudit.

const asPetstoreYAML = `openapi: 3.0.3
info:
  title: Swagger Petstore
  version: 1.0.0
servers:
  - url: https://petstore.example.com/v1
paths:
  /pets:
    get:
      operationId: listPets
      summary: List all pets
      tags: [pets]
      responses:
        "200":
          description: ok
    post:
      operationId: createPet
      summary: Create a pet
      tags: [pets]
      deprecated: true
      responses:
        "201":
          description: created
  /pets/{petId}:
    get:
      operationId: showPetById
      summary: Info for a specific pet
      tags: [pets]
      security:
        - apiKey: []
      parameters:
        - name: petId
          in: path
          required: true
          schema:
            type: string
      responses:
        "200":
          description: ok
          content:
            application/json:
              schema:
                $ref: "#/components/schemas/Pet"
components:
  securitySchemes:
    apiKey:
      type: apiKey
      in: header
      name: X-API-Key
  schemas:
    Pet:
      type: object
      properties:
        id:
          type: string
        owner:
          $ref: "#/components/schemas/Owner"
        children:
          type: array
          items:
            $ref: "#/components/schemas/Pet"
    Owner:
      type: object
      properties:
        name:
          type: string
        address:
          $ref: "#/components/schemas/Address"
    Address:
      type: object
      properties:
        city:
          type: string
    Unused:
      type: object
      properties:
        never:
          type: string
`

const asBillingYAML = `openapi: 3.0.3
info:
  title: Billing API
  version: 2.1.0
paths:
  /invoices:
    get:
      operationId: listInvoices
      summary: List invoices
      tags: [invoices]
      responses:
        "200":
          description: ok
  /invoices/{id}/pay:
    post:
      operationId: payInvoice
      summary: Pay an invoice
      tags: [invoices]
      parameters:
        - name: id
          in: path
          required: true
          schema:
            type: string
      responses:
        "200":
          description: ok
`

const asSoloYAML = `openapi: 3.0.3
info:
  title: Solo
  version: 1.0.0
paths:
  /health:
    get:
      operationId: healthCheck
      summary: Health check
      responses:
        "200":
          description: ok
`

// asHugeYAML membangun skema bersarang 8 tingkat dengan 4 properti per tingkat sehingga
// hasil kedalaman 8 jauh melewati batas respons.
func asHugeYAML() string {
	var sb strings.Builder
	sb.WriteString("openapi: 3.0.3\ninfo:\n  title: Huge Report\n  version: 1.0.0\npaths:\n  /report:\n    get:\n      operationId: getReport\n      summary: Report\n      responses:\n        \"200\":\n          description: ok\n          content:\n            application/json:\n              schema:\n                $ref: \"#/components/schemas/L1\"\ncomponents:\n  schemas:\n")
	for i := 1; i <= 8; i++ {
		fmt.Fprintf(&sb, "    L%d:\n      type: object\n      description: tingkat %d dari skema laporan bersarang untuk menguji batas ukuran respons\n      properties:\n", i, i)
		for _, p := range []string{"alpha", "beta", "gamma", "delta"} {
			if i < 8 {
				fmt.Fprintf(&sb, "        %s:\n          $ref: \"#/components/schemas/L%d\"\n", p, i+1)
			} else {
				fmt.Fprintf(&sb, "        %s:\n          type: string\n", p)
			}
		}
	}
	return sb.String()
}

// asUpload mengunggah spec lewat REST T06; nama spec = nama file tanpa ekstensi.
func (e *mcpEnv) asUpload(projID string, files map[string]string) {
	e.t.Helper()
	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	sort.Strings(names)
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for _, n := range names {
		w, err := mw.CreateFormFile("files", n)
		if err != nil {
			e.t.Fatal(err)
		}
		_, _ = w.Write([]byte(files[n]))
	}
	_ = mw.Close()
	req, _ := http.NewRequest(http.MethodPost, e.base+"/api/projects/"+projID+"/api-specs/upload", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("X-Requested-With", "grimoire")
	resp, err := e.client.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		var b bytes.Buffer
		_, _ = b.ReadFrom(resp.Body)
		e.t.Fatalf("upload spec: %d %s", resp.StatusCode, b.String())
	}
}

// asEnv: project A (e.projID, 3 spec) dan project B (satu spec solo) dengan key masing-masing.
type asEnv struct {
	*mcpEnv
	projB string
	keyB  string
}

func setupAPISpecTools(t *testing.T) *asEnv {
	t.Helper()
	e := setupMCP(t)
	e.asUpload(e.projID, map[string]string{
		"petstore.yaml": asPetstoreYAML,
		"billing.yaml":  asBillingYAML,
		"huge.yaml":     asHugeYAML(),
	})
	b := e.newProject("Other")
	e.asUpload(b, map[string]string{"solo.yaml": asSoloYAML})
	resp, body := e.rest(http.MethodPost, "/api/projects/"+b+"/api-keys", map[string]string{"name": "key-b"})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create key B: %d %s", resp.StatusCode, body)
	}
	var kr struct {
		RawKey string `json:"raw_key"`
	}
	if err := json.Unmarshal(body, &kr); err != nil {
		t.Fatal(err)
	}
	return &asEnv{mcpEnv: e, projB: b, keyB: kr.RawKey}
}

// asJSON mengurai teks hasil get_api_endpoint sebagai object JSON.
func asJSON(t *testing.T, text string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(text), &m); err != nil {
		t.Fatalf("teks bukan JSON valid: %v\n%.500s", err, text)
	}
	return m
}

func asMustOK(t *testing.T, r ktResult) {
	t.Helper()
	if r.IsError {
		t.Fatalf("isError tak terduga: %s", r.Text)
	}
}

func asMustContain(t *testing.T, text string, subs ...string) {
	t.Helper()
	for _, s := range subs {
		if !strings.Contains(text, s) {
			t.Errorf("teks tidak memuat %q:\n%.1500s", s, text)
		}
	}
}

func TestT09_Int_ToolsList(t *testing.T) {
	e := setupAPISpecTools(t)
	_, body := e.mcp(e.rawKey, listBody, nil)
	var out struct {
		Result struct {
			Tools []struct {
				Name        string `json:"name"`
				Annotations struct {
					ReadOnlyHint bool `json:"readOnlyHint"`
				} `json:"annotations"`
			} `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatal(err)
	}
	found := map[string]bool{}
	for _, tl := range out.Result.Tools {
		found[tl.Name] = tl.Annotations.ReadOnlyHint
	}
	for _, n := range []string{"get_api_endpoint", "list_api_endpoints", "list_api_specs"} {
		ro, ok := found[n]
		if !ok {
			t.Errorf("tool %s tidak ada: %s", n, body)
		} else if !ro {
			t.Errorf("tool %s tanpa readOnlyHint", n)
		}
	}
}

func TestT09_Int_AC1_ListSpecs(t *testing.T) {
	e := setupAPISpecTools(t)
	r := e.mcpCall(e.rawKey, "list_api_specs", map[string]any{})
	asMustOK(t, r)
	asMustContain(t, r.Text, "3 API spec", "billing", "huge", "petstore", "Swagger Petstore", "2.1.0", "OpenAPI 3.0.3", "3 endpoint", "2 endpoint")
}

func TestT09_Int_AC1_ListEndpointsGrouped(t *testing.T) {
	e := setupAPISpecTools(t)
	r := e.mcpCall(e.rawKey, "list_api_endpoints", map[string]any{})
	asMustOK(t, r)
	asMustContain(t, r.Text, "== billing ==", "== huge ==", "== petstore ==", "POST /pets — Create a pet [createPet] (pets) [deprecated]")
	b, h, p := strings.Index(r.Text, "== billing =="), strings.Index(r.Text, "== huge =="), strings.Index(r.Text, "== petstore ==")
	if !(b < h && h < p) {
		t.Errorf("grup tidak berurutan billing<huge<petstore: %d %d %d", b, h, p)
	}
}

func TestT09_Int_AC1_TagAndQuery(t *testing.T) {
	e := setupAPISpecTools(t)

	r := e.mcpCall(e.rawKey, "list_api_endpoints", map[string]any{"tag": "pets"})
	asMustOK(t, r)
	asMustContain(t, r.Text, "GET /pets", "createPet")
	if strings.Contains(r.Text, "listInvoices") {
		t.Errorf("tag pets memuat invoice: %s", r.Text)
	}

	r = e.mcpCall(e.rawKey, "list_api_endpoints", map[string]any{"query": "invoice"})
	asMustOK(t, r)
	asMustContain(t, r.Text, "listInvoices", "payInvoice")
	if strings.Contains(r.Text, "listPets") {
		t.Errorf("query invoice memuat pets: %s", r.Text)
	}

	r = e.mcpCall(e.rawKey, "list_api_endpoints", map[string]any{"spec": "petstore", "query": "petId"})
	asMustOK(t, r)
	asMustContain(t, r.Text, "Endpoint 1-1 dari 1", "showPetById")
}

func TestT09_Int_ListEndpointsPaged(t *testing.T) {
	e := setupAPISpecTools(t)
	p1 := e.mcpCall(e.rawKey, "list_api_endpoints", map[string]any{"limit": 2})
	asMustOK(t, p1)
	asMustContain(t, p1.Text, "offset=2")

	seen := map[string]int{}
	offset := 0
	for page := 0; page < 10; page++ {
		r := e.mcpCall(e.rawKey, "list_api_endpoints", map[string]any{"limit": 2, "offset": offset})
		asMustOK(t, r)
		for _, ln := range strings.Split(r.Text, "\n") {
			for _, m := range []string{"GET ", "POST ", "PUT ", "DELETE ", "PATCH "} {
				if strings.HasPrefix(ln, m) {
					seen[ln]++
				}
			}
		}
		if !strings.Contains(r.Text, "[ada ") {
			break
		}
		offset += 2
	}
	// petstore 3 + billing 2 + huge 1
	if len(seen) != 6 {
		t.Fatalf("endpoint unik = %d, want 6: %v", len(seen), seen)
	}
	for ln, n := range seen {
		if n != 1 {
			t.Errorf("duplikat %q x%d", ln, n)
		}
	}
}

func TestT09_Int_UnknownSpecList(t *testing.T) {
	e := setupAPISpecTools(t)
	r := e.mcpCall(e.rawKey, "list_api_endpoints", map[string]any{"spec": "petstor"})
	if !r.IsError {
		t.Fatalf("harus isError: %s", r.Text)
	}
	asMustContain(t, r.Text, "Spec tidak ditemukan: petstor", "billing, huge, petstore")
}

func TestT09_Int_AC2_NestedCircular(t *testing.T) {
	e := setupAPISpecTools(t)
	r := e.mcpCall(e.rawKey, "get_api_endpoint", map[string]any{"spec": "petstore", "method": "get", "path": "/pets/{petId}"})
	asMustOK(t, r)
	m := asJSON(t, r.Text)
	if m["method"] != "GET" || m["path"] != "/pets/{petId}" || m["spec"] != "petstore" {
		t.Errorf("identitas: %v %v %v", m["spec"], m["method"], m["path"])
	}
	asMustContain(t, r.Text, `"owner"`, `"address"`, `"city"`, `"$circular":"#/components/schemas/Pet"`)
	if strings.Contains(r.Text, "Unused") || strings.Contains(r.Text, `"never"`) {
		t.Errorf("skema tak dirujuk ikut: %.800s", r.Text)
	}
	markers, _ := m["markers"].(map[string]any)
	circ, _ := markers["circular"].([]any)
	if len(circ) == 0 || circ[0] != "#/components/schemas/Pet" {
		t.Errorf("markers.circular = %v", markers)
	}
	servers, _ := m["servers"].([]any)
	if len(servers) == 0 {
		t.Errorf("servers kosong")
	}
	ss, _ := m["securitySchemes"].(map[string]any)
	if _, ok := ss["apiKey"]; !ok {
		t.Errorf("securitySchemes.apiKey tidak ada: %v", ss)
	}
}

func TestT09_Int_AC2_ByOperationID(t *testing.T) {
	e := setupAPISpecTools(t)
	r := e.mcpCall(e.rawKey, "get_api_endpoint", map[string]any{"spec": "petstore", "operation_id": "showPetById"})
	asMustOK(t, r)
	m := asJSON(t, r.Text)
	if m["method"] != "GET" || m["path"] != "/pets/{petId}" {
		t.Errorf("method/path = %v %v", m["method"], m["path"])
	}
	byPath := e.mcpCall(e.rawKey, "get_api_endpoint", map[string]any{"spec": "petstore", "method": "GET", "path": "/pets/{petId}"})
	asMustOK(t, byPath)
	if byPath.Text != r.Text {
		t.Errorf("operation_id dan method+path harus identik:\n%.400s\n%.400s", r.Text, byPath.Text)
	}
}

func TestT09_Int_AC3_SpecRequired(t *testing.T) {
	e := setupAPISpecTools(t)
	r := e.mcpCall(e.rawKey, "get_api_endpoint", map[string]any{"method": "GET", "path": "/pets"})
	if !r.IsError {
		t.Fatalf("harus isError: %s", r.Text)
	}
	asMustContain(t, r.Text, "punya 3 API spec", "billing", "huge", "petstore")
}

func TestT09_Int_AC3_SingleSpecOptional(t *testing.T) {
	e := setupAPISpecTools(t)
	r := e.mcpCall(e.keyB, "get_api_endpoint", map[string]any{"method": "GET", "path": "/health"})
	asMustOK(t, r)
	if asJSON(t, r.Text)["spec"] != "solo" {
		t.Errorf("spec != solo: %.300s", r.Text)
	}
}

func TestT09_Int_AC4_Similar(t *testing.T) {
	e := setupAPISpecTools(t)
	r := e.mcpCall(e.rawKey, "get_api_endpoint", map[string]any{"spec": "petstore", "method": "GET", "path": "/pet/{petId}"})
	if !r.IsError {
		t.Fatalf("harus isError: %s", r.Text)
	}
	asMustContain(t, r.Text, "Endpoint tidak ditemukan", "Similar:", "GET /pets/{petId}", "list_api_endpoints")
}

func TestT09_Int_AC5_TruncatedDepth(t *testing.T) {
	e := setupAPISpecTools(t)
	r := e.mcpCall(e.rawKey, "get_api_endpoint", map[string]any{"spec": "huge", "method": "GET", "path": "/report", "schema_depth": 8})
	asMustOK(t, r)
	if strings.Contains(r.Text, "[truncated: respons dipotong") {
		t.Fatalf("dipotong gateway, seharusnya diturunkan provider")
	}
	m := asJSON(t, r.Text)
	if m["truncated"] != true {
		t.Errorf("truncated = %v", m["truncated"])
	}
	if d, _ := m["schema_depth"].(float64); d >= 8 {
		t.Errorf("schema_depth = %v, want < 8", m["schema_depth"])
	}
	if h, _ := m["hint"].(string); !strings.Contains(h, "schema_depth") {
		t.Errorf("hint = %q", h)
	}
	if len(r.Text) > 102400 {
		t.Errorf("len(text) = %d > 102400", len(r.Text))
	}
}

func TestT09_Int_AC6_ProjectArgIgnoredForData(t *testing.T) {
	e := setupAPISpecTools(t)
	cases := map[string]map[string]any{
		"list_api_specs":     {"project_id": e.projB},
		"list_api_endpoints": {"project_id": e.projB},
		"get_api_endpoint":   {"project_id": e.projB, "method": "GET", "path": "/health"},
	}
	for name, args := range cases {
		r := e.mcpCall(e.rawKey, name, args)
		if !r.IsError {
			t.Errorf("%s: harus isError: %s", name, r.Text)
		}
		asMustContain(t, r.Text, "ditentukan oleh API key")
		if strings.Contains(r.Text, "solo") {
			t.Errorf("%s: bocor data project B: %s", name, r.Text)
		}
	}
	r := e.mcpCall(e.rawKey, "get_api_endpoint", map[string]any{"spec": "solo", "method": "GET", "path": "/health"})
	if !r.IsError {
		t.Fatalf("spec solo milik project B harus tak ditemukan: %s", r.Text)
	}
	asMustContain(t, r.Text, "Spec tidak ditemukan")
}

func TestT09_Int_AuditOneEntryPerCall(t *testing.T) {
	e := setupAPISpecTools(t)
	r := e.mcpCall(e.rawKey, "get_api_endpoint", map[string]any{"spec": "petstore", "method": "GET", "path": "/pets/{petId}"})
	asMustOK(t, r)
	deadline := time.Now().Add(3 * time.Second)
	for e.countAudit("mcp.get_api_endpoint") == 0 && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if n := e.countAudit("mcp.get_api_endpoint"); n != 1 {
		t.Fatalf("audit mcp.get_api_endpoint = %d, want 1", n)
	}
	var targetType, target string
	var result []byte
	q := "SELECT coalesce(target_type,''), coalesce(target,''), result FROM " + e.schema + ".audit_log WHERE action = 'mcp.get_api_endpoint'"
	if err := e.admin.QueryRow(q).Scan(&targetType, &target, &result); err != nil {
		t.Fatal(err)
	}
	if targetType != "api_endpoint" || target != "petstore GET /pets/{petId}" {
		t.Errorf("target = %q %q", targetType, target)
	}
	var m map[string]any
	if err := json.Unmarshal(result, &m); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"bytes", "schema_depth"} {
		if _, ok := m[k]; !ok {
			t.Errorf("result tanpa %s: %s", k, result)
		}
	}
	if strings.Contains(string(result), "never") || strings.Contains(string(result), "city") {
		t.Errorf("result memuat isi skema: %s", result)
	}
}
