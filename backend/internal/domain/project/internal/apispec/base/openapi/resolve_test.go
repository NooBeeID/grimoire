package openapi

import (
	"encoding/json"
	"strings"
	"testing"

	"grimoire/internal/domain/project/internal/apispec/base"
)

func TestExtractEndpoints_PetstoreOrderAndFields(t *testing.T) {
	eps := ExtractEndpoints(mustParse(t, "petstore-3.0.yaml"))
	got := []string{}
	for _, e := range eps {
		got = append(got, e.Method+" "+e.Path)
	}
	want := "GET /pets|POST /pets|GET /pets/{petId}"
	if strings.Join(got, "|") != want {
		t.Fatalf("%v", got)
	}
	if eps[0].OperationID == nil || *eps[0].OperationID != "listPets" || eps[0].Summary != "Daftar hewan" {
		t.Fatalf("%+v", eps[0])
	}
	if strings.Join(eps[0].Tags, ",") != "pets,read" {
		t.Fatalf("tags tidak dedup: %v", eps[0].Tags)
	}
	if !eps[2].Deprecated || eps[2].Summary != "" || eps[2].Tags == nil {
		t.Fatalf("%+v", eps[2])
	}
	if !base.ValidMethod(eps[1].Method) {
		t.Fatal("method")
	}
}

func TestExtractEndpoints_SummaryTruncatedAndPathItemRef(t *testing.T) {
	y := "openapi: 3.0.3\ninfo: {title: X, version: '1'}\npaths:\n  /a:\n    $ref: '#/components/pathItems/A'\n  /b:\n    get:\n      summary: " +
		strings.Repeat("é", 400) + "\n      responses: {'200': {description: x}}\ncomponents:\n  pathItems:\n    A:\n      post:\n        responses: {'200': {description: x}}\n"
	doc, is := Parse([]byte(y), FormatYAML, DefaultLimits)
	if len(is) != 0 {
		t.Fatalf("%+v", is)
	}
	eps := ExtractEndpoints(doc)
	if len(eps) != 2 || eps[0].Method != "POST" || eps[0].Path != "/a" || len([]rune(eps[1].Summary)) != 300 {
		t.Fatalf("%+v", eps)
	}
}

func TestFindOperation(t *testing.T) {
	doc := mustParse(t, "petstore-3.0.yaml")
	if m, p, ok := FindOperation(doc, "get", "/pets/{petId}", ""); !ok || m != "GET" || p != "/pets/{petId}" {
		t.Fatalf("%s %s %v", m, p, ok)
	}
	if m, p, ok := FindOperation(doc, "", "", "createPet"); !ok || m != "POST" || p != "/pets" {
		t.Fatalf("%s %s %v", m, p, ok)
	}
	for _, c := range [][3]string{{"GET", "/pets/{id}", ""}, {"DELETE", "/pets", ""}, {"", "", "nope"}} {
		if _, _, ok := FindOperation(doc, c[0], c[1], c[2]); ok {
			t.Errorf("%v harus tidak ketemu", c)
		}
	}
}

func resolve(t *testing.T, file, method, path string, depth, max int) (Resolved, map[string]any) {
	t.Helper()
	r, err := ResolveOperation(mustParse(t, file), method, path, depth, max)
	if err != nil {
		t.Fatal(err)
	}
	var op map[string]any
	if err := json.Unmarshal(r.Operation, &op); err != nil {
		t.Fatalf("operation bukan JSON valid: %v", err)
	}
	return r, op
}

func TestT06_AC5_InlinesNestedSchemas(t *testing.T) {
	r, _ := resolve(t, "petstore-3.0.yaml", "GET", "/pets", 4, 100000)
	s := string(r.Operation)
	if !strings.Contains(s, `"name":{"nullable":true`) && !strings.Contains(s, `"nullable":true`) {
		t.Fatalf("Pet tidak ter-inline: %s", s)
	}
	if !strings.Contains(s, `"owner":{"type":"object","properties":{"name"`) {
		t.Fatalf("Owner tidak ter-inline: %s", s)
	}
	if strings.Contains(s, "Unused") || strings.Contains(s, `"components"`) {
		t.Fatalf("skema tak dirujuk muncul: %s", s)
	}
	if r.Truncated || r.EffectiveDepth != 4 {
		t.Fatalf("%+v", r)
	}
}

func TestT06_AC5_DepthLeavesRef(t *testing.T) {
	r, _ := resolve(t, "petstore-3.0.yaml", "GET", "/pets", 1, 100000)
	s := string(r.Operation)
	if !strings.Contains(s, `"owner":{"$ref":"#/components/schemas/Owner"}`) {
		t.Fatalf("level 2 harus tetap $ref: %s", s)
	}
	r0, _ := resolve(t, "petstore-3.0.yaml", "GET", "/pets", 0, 100000)
	if !strings.Contains(string(r0.Operation), `"$ref":"#/components/schemas/Pet"`) {
		t.Fatalf("depth 0: %s", r0.Operation)
	}
}

func TestT06_AC5_Circular(t *testing.T) {
	r, _ := resolve(t, "nested-circular.yaml", "GET", "/nodes/{id}", 8, 100000)
	s := string(r.Operation)
	if !strings.Contains(s, `{"$circular":"#/components/schemas/Node"}`) {
		t.Fatalf("%s", s)
	}
	if strings.Contains(s, "Unused") {
		t.Fatal("Unused muncul")
	}
}

func TestT06_AC5_PathParamsMergedOperationWins(t *testing.T) {
	_, op := resolve(t, "nested-circular.yaml", "GET", "/nodes/{id}", 4, 100000)
	params := op["parameters"].([]any)
	if len(params) != 2 {
		t.Fatalf("%v", params)
	}
	first := params[0].(map[string]any)
	second := params[1].(map[string]any)
	if first["name"] != "id" || second["name"] != "verbose" || second["description"] != "override" {
		t.Fatalf("%v", params)
	}
}

func TestResolveOperation_ParamRefIdentity(t *testing.T) {
	// parameter path-level via $ref diselesaikan untuk perbandingan name+in dengan parameter operasi.
	y := "openapi: 3.0.3\ninfo: {title: X, version: '1'}\npaths:\n  /a:\n    parameters:\n      - $ref: '#/components/parameters/T'\n    get:\n      parameters:\n        - {name: X-T, in: header, description: op}\n      responses: {'200': {description: x}}\ncomponents:\n  parameters:\n    T: {name: X-T, in: header}\n"
	doc, is := Parse([]byte(y), FormatYAML, DefaultLimits)
	if len(is) != 0 {
		t.Fatal(is)
	}
	r, err := ResolveOperation(doc, "GET", "/a", 4, 0)
	if err != nil || strings.Count(string(r.Operation), "X-T") != 1 || !strings.Contains(string(r.Operation), `"op"`) {
		t.Fatalf("%v %s", err, r.Operation)
	}
}

func TestT06_AC5_ServersAndSecurity(t *testing.T) {
	r, _ := resolve(t, "nested-circular.yaml", "GET", "/nodes/{id}", 4, 0)
	if string(r.Servers) != `[{"url":"https://root.example"}]` {
		t.Fatalf("servers=%s", r.Servers)
	}
	if string(r.SecuritySchemes) != `{"bearer":{"type":"http","scheme":"bearer"}}` {
		t.Fatalf("security=%s", r.SecuritySchemes)
	}
	// operasi security: [] -> {}, tanpa servers sama sekali -> [].
	y := "openapi: 3.0.3\ninfo: {title: X, version: '1'}\nsecurity: [{b: []}]\npaths:\n  /a:\n    get:\n      security: []\n      responses: {'200': {description: x}}\ncomponents:\n  securitySchemes:\n    b: {type: http, scheme: basic}\n"
	doc, _ := Parse([]byte(y), FormatYAML, DefaultLimits)
	r2, err := ResolveOperation(doc, "GET", "/a", 4, 0)
	if err != nil || string(r2.SecuritySchemes) != `{}` || string(r2.Servers) != `[]` {
		t.Fatalf("%v %s %s", err, r2.SecuritySchemes, r2.Servers)
	}
	// servers: operasi > path > root.
	y2 := "openapi: 3.0.3\ninfo: {title: X, version: '1'}\nservers: [{url: 'https://root'}]\npaths:\n  /a:\n    servers: [{url: 'https://path'}]\n    get:\n      responses: {'200': {description: x}}\n    post:\n      servers: [{url: 'https://op'}]\n      responses: {'200': {description: x}}\n"
	d2, _ := Parse([]byte(y2), FormatYAML, DefaultLimits)
	g, _ := ResolveOperation(d2, "GET", "/a", 4, 0)
	p, _ := ResolveOperation(d2, "POST", "/a", 4, 0)
	if string(g.Servers) != `[{"url":"https://path"}]` || string(p.Servers) != `[{"url":"https://op"}]` {
		t.Fatalf("%s %s", g.Servers, p.Servers)
	}
}

func TestT06_AC5_TruncationStepsDown(t *testing.T) {
	full, _ := resolve(t, "nested-circular.yaml", "GET", "/nodes/{id}", 8, 0)
	r, _ := resolve(t, "nested-circular.yaml", "GET", "/nodes/{id}", 8, len(full.Operation)-1+len(full.Servers)+len(full.SecuritySchemes)-60)
	if !r.Truncated || r.EffectiveDepth >= 8 {
		t.Fatalf("%+v", r)
	}
	tiny, op := resolve(t, "nested-circular.yaml", "GET", "/nodes/{id}", 8, 10)
	if !tiny.Truncated || tiny.EffectiveDepth != 0 || op["$truncated"] != true || op["operationId"] != "getNode" || op["summary"] != "Ambil node" {
		t.Fatalf("%+v %v", tiny, op)
	}
}

func TestResolveOperation_NotFound(t *testing.T) {
	doc := mustParse(t, "petstore-3.0.yaml")
	for _, c := range [][2]string{{"GET", "/nope"}, {"DELETE", "/pets"}} {
		if _, err := ResolveOperation(doc, c[0], c[1], 4, 0); err != ErrOperationNotFound {
			t.Errorf("%v: %v", c, err)
		}
	}
}

func TestSimilar(t *testing.T) {
	cands := []Candidate{
		{Method: "GET", Path: "/orders", OperationID: "listOrders"},
		{Method: "GET", Path: "/pets/{petId}", OperationID: "getPet"},
		{Method: "POST", Path: "/pets", OperationID: "createPet"},
	}
	got := Similar(cands, "GET", "/pet/{id}", "", 2)
	if len(got) != 2 || got[0].Path != "/pets/{petId}" {
		t.Fatalf("%+v", got)
	}
	byID := Similar(cands, "", "", "createpet", 1)
	if len(byID) != 1 || byID[0].OperationID != "createPet" {
		t.Fatalf("%+v", byID)
	}
	if got := Similar(nil, "GET", "/x", "", 5); got == nil || len(got) != 0 {
		t.Fatal("harus slice kosong non-nil")
	}
	many := make([]Candidate, 20)
	if len(Similar(many, "GET", "/x", "", 5)) != 5 {
		t.Fatal("maks n")
	}
}
