package openapi

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func parseSwagger2(t testing.TB, src string) Document {
	t.Helper()
	doc, is := Parse([]byte(src), FormatYAML, DefaultLimits)
	if len(is) != 0 {
		t.Fatalf("%+v", is)
	}
	return doc
}

func resolve2(t *testing.T, doc Document, method, path string, depth, max int) (Resolved, map[string]any) {
	t.Helper()
	r, err := ResolveOperation(doc, method, path, depth, max)
	if err != nil {
		t.Fatal(err)
	}
	var op map[string]any
	if err := json.Unmarshal(r.Operation, &op); err != nil {
		t.Fatal(err)
	}
	return r, op
}

func TestResolveOperation_3xFieldsEmpty(t *testing.T) {
	r, _ := resolve(t, "petstore-3.0.yaml", "GET", "/pets", 4, 0)
	if r.OpenAPIVersion != "" || r.Consumes != nil || r.Produces != nil {
		t.Fatalf("%+v", r)
	}
}

func TestResolveOperation_Swagger2_InlinesDefinitions(t *testing.T) {
	doc := mustParse(t, "swagger-2.0-petstore.yaml")
	r, op := resolve2(t, doc, "POST", "/pets", 4, 100000)
	s := string(r.Operation)
	if !strings.Contains(s, `"properties":{"id":{"type":"integer"},"name":{"type":"string"}}`) {
		t.Fatalf("Pet tidak ter-inline: %s", s)
	}
	if strings.Contains(s, "definitions") || strings.Contains(s, "Error") {
		t.Fatalf("%s", s)
	}
	p := op["parameters"].([]any)[0].(map[string]any)
	if p["in"] != "body" || p["schema"] == nil {
		t.Fatalf("%v", p)
	}
	if r.OpenAPIVersion != "2.0" || r.Truncated || r.EffectiveDepth != 4 {
		t.Fatalf("%+v", r)
	}
	if string(r.Consumes) != `["application/json"]` || string(r.Produces) != `["application/json"]` {
		t.Fatalf("%s %s", r.Consumes, r.Produces)
	}
	if string(r.Servers) != `[{"url":"https://petstore.example.com/v1"}]` || string(r.SecuritySchemes) != `{}` {
		t.Fatalf("%s %s", r.Servers, r.SecuritySchemes)
	}
}

func TestResolveOperation_Swagger2_InlinesParametersAndResponses(t *testing.T) {
	doc := mustParse(t, "swagger-2.0-full.yaml")
	r, _ := resolve2(t, doc, "GET", "/pets", 4, 0)
	s := string(r.Operation)
	if !strings.Contains(s, `"name":"limit","in":"query","type":"integer","collectionFormat":"csv"`) {
		t.Fatalf("parameter $ref tidak ter-inline: %s", s)
	}
	if !strings.Contains(s, `"404":{"description":"tidak ada","schema":{"type":"object","properties":{"message":{"type":"string"}}}}`) {
		t.Fatalf("response $ref tidak ter-inline: %s", s)
	}
	if !strings.Contains(s, `"discriminator":"kind"`) || !strings.Contains(s, `"x-nullable":true`) {
		t.Fatalf("allOf/discriminator/x-nullable hilang: %s", s)
	}
}

func TestResolveOperation_Swagger2_PathParamsMerged(t *testing.T) {
	src := s2(`paths:
  /a/{id}:
    parameters:
      - {name: id, in: path, required: true, type: string}
      - {name: v, in: query, type: boolean}
      - $ref: '#/parameters/T'
    get:
      parameters:
        - {name: v, in: query, type: string, description: override}
        - {name: X-T, in: header, description: op}
` + okOp + `parameters:
  T: {name: X-T, in: header, type: string}
`)
	_, op := resolve2(t, parseSwagger2(t, src), "GET", "/a/{id}", 4, 0)
	params := op["parameters"].([]any)
	if len(params) != 3 {
		t.Fatalf("%v", params)
	}
	names := []string{}
	for _, p := range params {
		names = append(names, p.(map[string]any)["name"].(string))
	}
	if strings.Join(names, ",") != "id,v,X-T" || params[1].(map[string]any)["description"] != "override" ||
		params[2].(map[string]any)["description"] != "op" {
		t.Fatalf("%v", params)
	}
}

func TestResolveOperation_Swagger2_DepthLeavesRef(t *testing.T) {
	doc := mustParse(t, "swagger-2.0-full.yaml")
	r, _ := resolve2(t, doc, "GET", "/pets", 1, 0)
	if !strings.Contains(string(r.Operation), `{"$ref":"#/definitions/Animal"}`) {
		t.Fatalf("level 2 harus tetap $ref: %s", r.Operation)
	}
	r0, _ := resolve2(t, mustParse(t, "swagger-2.0-petstore.yaml"), "GET", "/pets/{petId}", 0, 0)
	if !strings.Contains(string(r0.Operation), `"$ref":"#/definitions/Pet"`) {
		t.Fatalf("%s", r0.Operation)
	}
}

func TestResolveOperation_Swagger2_Circular(t *testing.T) {
	r, _ := resolve2(t, mustParse(t, "swagger-2.0-circular.yaml"), "GET", "/nodes/{id}", 8, 100000)
	s := string(r.Operation)
	if !strings.Contains(s, `{"$circular":"#/definitions/Node"}`) || strings.Contains(s, "Unused") {
		t.Fatalf("%s", s)
	}
}

func TestResolveOperation_Swagger2_Servers(t *testing.T) {
	cases := []struct{ name, head, want string }{
		{"dua scheme", "host: h\nbasePath: /v1\nschemes: [http, https]\n", `[{"url":"http://h/v1"},{"url":"https://h/v1"}]`},
		{"tanpa schemes", "host: h\nbasePath: /v1\n", `[{"url":"https://h/v1"}]`},
		{"tanpa host", "basePath: /v1\nschemes: [http]\n", `[{"url":"/v1"}]`},
		{"tanpa basePath", "host: h\n", `[{"url":"https://h/"}]`},
		{"tanpa apa pun", "", `[{"url":"/"}]`},
		{"schemes duplikat", "host: h\nschemes: [https, https]\n", `[{"url":"https://h/"}]`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			doc := parseSwagger2(t, s2(c.head+"paths:\n  /a:\n    get:\n"+okOp))
			r, _ := resolve2(t, doc, "GET", "/a", 4, 0)
			if string(r.Servers) != c.want {
				t.Fatalf("got %s want %s", r.Servers, c.want)
			}
		})
	}
}

func TestResolveOperation_Swagger2_Security(t *testing.T) {
	doc := mustParse(t, "swagger-2.0-full.yaml")
	// root security: basic
	r, _ := resolve2(t, doc, "GET", "/pets", 4, 0)
	if string(r.SecuritySchemes) != `{"basic":{"type":"basic"}}` {
		t.Fatalf("%s", r.SecuritySchemes)
	}
	// operasi security: [] -> {}
	r, _ = resolve2(t, doc, "POST", "/pets/{petId}/photo", 4, 0)
	if string(r.SecuritySchemes) != `{}` {
		t.Fatalf("%s", r.SecuritySchemes)
	}
	// operasi menimpa root, oauth2 membawa scopes
	r, _ = resolve2(t, doc, "POST", "/pets", 4, 0)
	if !strings.HasPrefix(string(r.SecuritySchemes), `{"oauth":{"type":"oauth2"`) || !strings.Contains(string(r.SecuritySchemes), `"scopes":{"read":"Baca","write":"Tulis"}`) ||
		strings.Contains(string(r.SecuritySchemes), "basic") {
		t.Fatalf("%s", r.SecuritySchemes)
	}
	// dua alternatif
	r, _ = resolve2(t, doc, "DELETE", "/pets/{petId}", 4, 0)
	if !strings.Contains(string(r.SecuritySchemes), `"api_key"`) || !strings.Contains(string(r.SecuritySchemes), `"basic"`) || strings.Contains(string(r.SecuritySchemes), "oauth") {
		t.Fatalf("%s", r.SecuritySchemes)
	}
	// nama tak terdefinisi dilewati
	src := s2("security: [{ghost: []}]\npaths:\n  /a:\n    get:\n" + okOp)
	r, _ = resolve2(t, parseSwagger2(t, src), "GET", "/a", 4, 0)
	if string(r.SecuritySchemes) != `{}` {
		t.Fatalf("%s", r.SecuritySchemes)
	}
}

func TestResolveOperation_Swagger2_ConsumesProduces(t *testing.T) {
	doc := mustParse(t, "swagger-2.0-full.yaml")
	r, _ := resolve2(t, doc, "POST", "/pets/{petId}/photo", 4, 0)
	if string(r.Consumes) != `["multipart/form-data"]` || string(r.Produces) != `["application/json"]` {
		t.Fatalf("%s %s", r.Consumes, r.Produces)
	}
	r, _ = resolve2(t, doc, "GET", "/pets", 4, 0)
	if string(r.Consumes) != `["application/json"]` {
		t.Fatalf("%s", r.Consumes)
	}
	src := s2("consumes: [a/b]\nproduces: [c/d]\npaths:\n  /a:\n    get:\n      consumes: []\n" + okOp + "  /b:\n    get:\n" + okOp)
	d := parseSwagger2(t, src)
	r, _ = resolve2(t, d, "GET", "/a", 4, 0)
	if string(r.Consumes) != `[]` || string(r.Produces) != `["c/d"]` {
		t.Fatalf("%s %s", r.Consumes, r.Produces)
	}
	none := parseSwagger2(t, s2("paths:\n  /a:\n    get:\n"+okOp))
	r, _ = resolve2(t, none, "GET", "/a", 4, 0)
	if string(r.Consumes) != `[]` || string(r.Produces) != `[]` {
		t.Fatalf("%s %s", r.Consumes, r.Produces)
	}
}

func TestResolveOperation_Swagger2_TraceNotFound(t *testing.T) {
	doc := mustParse(t, "swagger-2.0-full.yaml")
	if _, err := ResolveOperation(doc, "TRACE", "/pets/{petId}/photo", 4, 0); err != ErrOperationNotFound {
		t.Fatalf("%v", err)
	}
	// kontrol 3.x: TRACE bisa di-resolve
	d3, is := Parse([]byte(golden3xServers), FormatYAML, DefaultLimits)
	if len(is) != 0 {
		t.Fatal(is)
	}
	if _, err := ResolveOperation(d3, "TRACE", "/a", 4, 0); err != nil {
		t.Fatalf("%v", err)
	}
}

func TestResolveOperation_Swagger2_TruncationStepsDown(t *testing.T) {
	doc := mustParse(t, "swagger-2.0-circular.yaml")
	full, _ := resolve2(t, doc, "GET", "/nodes/{id}", 8, 0)
	r, _ := resolve2(t, doc, "GET", "/nodes/{id}", 8, len(full.Operation)-30)
	if !r.Truncated || r.EffectiveDepth >= 8 {
		t.Fatalf("%+v", r)
	}
	tiny, op := resolve2(t, mustParse(t, "swagger-2.0-full.yaml"), "POST", "/pets", 8, 10)
	if !tiny.Truncated || tiny.EffectiveDepth != 0 || op["$truncated"] != true || op["operationId"] != "createPet" {
		t.Fatalf("%+v %v", tiny, op)
	}
	if tiny.OpenAPIVersion != "2.0" || len(tiny.Consumes) == 0 || len(tiny.Produces) == 0 {
		t.Fatalf("%+v", tiny)
	}
}

func TestResolveOperation_Swagger2_InlineBudget(t *testing.T) {
	// fan-out: tiap definisi merujuk 3 definisi berikutnya -> ledakan eksponensial; budget harus memotong.
	var b strings.Builder
	b.WriteString(s2("paths:\n  /a:\n    get:\n      responses:\n        '200':\n          description: ok\n          schema: {$ref: '#/definitions/D0'}\ndefinitions:\n"))
	const n = 30
	for i := 0; i < n; i++ {
		b.WriteString("  D" + itoa(i) + ":\n    type: object\n    properties:\n")
		for j := 1; j <= 3 && i+j < n; j++ {
			b.WriteString("      p" + itoa(j) + ": {$ref: '#/definitions/D" + itoa(i+j) + "'}\n")
		}
	}
	doc := parseSwagger2(t, b.String())
	done := make(chan struct{})
	go func() {
		defer close(done)
		r, err := ResolveOperation(doc, "GET", "/a", 8, 0)
		if err != nil || !strings.Contains(string(r.Operation), `"$ref"`) {
			t.Errorf("%v %.200s", err, r.Operation)
		}
	}()
	select {
	case <-done:
	case <-time.After(20 * time.Second):
		t.Fatal("ResolveOperation tidak selesai (budget tidak bekerja)")
	}
}

func TestResolveOperation_Swagger2_NotFound(t *testing.T) {
	doc := mustParse(t, "swagger-2.0-petstore.yaml")
	for _, c := range [][2]string{{"GET", "/nope"}, {"PUT", "/pets"}} {
		if _, err := ResolveOperation(doc, c[0], c[1], 4, 0); err != ErrOperationNotFound {
			t.Errorf("%v: %v", c, err)
		}
	}
	empty := parseSwagger2(t, s2("paths: {}\n"))
	if _, err := ResolveOperation(empty, "GET", "/a", 4, 0); err != ErrOperationNotFound {
		t.Fatal(err)
	}
}
