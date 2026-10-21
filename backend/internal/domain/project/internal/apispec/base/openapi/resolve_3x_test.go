package openapi

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"testing"
)

var update = flag.Bool("update", false, "tulis ulang testdata/resolve_3x.golden.json")

const (
	golden3xServers = "openapi: 3.0.3\ninfo: {title: X, version: '1'}\nservers: [{url: 'https://root'}]\nsecurity: [{b: []}]\npaths:\n  /a:\n    servers: [{url: 'https://path'}]\n    get:\n      security: []\n      responses: {'200': {description: x}}\n    post:\n      servers: [{url: 'https://op'}]\n      responses: {'200': {description: x}}\n    trace:\n      responses: {'200': {description: x}}\ncomponents:\n  securitySchemes:\n    b: {type: http, scheme: basic}\n"
	golden3xNoSrv   = "openapi: 3.0.3\ninfo: {title: X, version: '1'}\npaths:\n  /a:\n    get:\n      responses: {'200': {description: x}}\n"
)

// TestResolveOperation_3xUnchanged mengunci keluaran ResolveOperation untuk OpenAPI 3.x (golden
// dibuat dari kode sebelum dukungan Swagger 2.0).
func TestResolveOperation_3xUnchanged(t *testing.T) {
	type tc struct {
		name, file, src, method, path string
		depth, max                    int
	}
	cases := []tc{
		{"petstore d4", "petstore-3.0.yaml", "", "GET", "/pets", 4, 100000},
		{"petstore d1", "petstore-3.0.yaml", "", "GET", "/pets", 1, 100000},
		{"petstore d0", "petstore-3.0.yaml", "", "GET", "/pets", 0, 100000},
		{"circular full", "nested-circular.yaml", "", "GET", "/nodes/{id}", 8, 0},
		{"circular truncated", "nested-circular.yaml", "", "GET", "/nodes/{id}", 8, 420},
		{"circular summary", "nested-circular.yaml", "", "GET", "/nodes/{id}", 8, 10},
		{"petstore31", "petstore-3.1.json", "", "GET", "/pets", 4, 100000},
		{"servers get", "", golden3xServers, "GET", "/a", 4, 0},
		{"servers post", "", golden3xServers, "POST", "/a", 4, 0},
		{"servers trace", "", golden3xServers, "TRACE", "/a", 4, 0},
		{"no servers", "", golden3xNoSrv, "GET", "/a", 4, 0},
	}
	type entry struct {
		Case            string          `json:"case"`
		Operation       json.RawMessage `json:"operation"`
		Servers         json.RawMessage `json:"servers"`
		SecuritySchemes json.RawMessage `json:"securitySchemes"`
		Truncated       bool            `json:"truncated"`
		EffectiveDepth  int             `json:"effectiveDepth"`
	}
	var out []entry
	for _, c := range cases {
		var doc Document
		if c.file != "" {
			doc = mustParse(t, c.file)
		} else {
			var is []Issue
			doc, is = Parse([]byte(c.src), FormatYAML, DefaultLimits)
			if len(is) != 0 {
				t.Fatalf("%s: %+v", c.name, is)
			}
		}
		r, err := ResolveOperation(doc, c.method, c.path, c.depth, c.max)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		out = append(out, entry{c.name, r.Operation, r.Servers, r.SecuritySchemes, r.Truncated, r.EffectiveDepth})
	}
	got, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	got = append(got, '\n')
	const path = "testdata/resolve_3x.golden.json"
	if *update {
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("keluaran ResolveOperation 3.x berubah dari golden\n got: %s\nwant: %s", got, want)
	}
}
