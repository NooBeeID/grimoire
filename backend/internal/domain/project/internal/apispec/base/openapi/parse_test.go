package openapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestParse_ValidDocuments(t *testing.T) {
	cases := []struct {
		file, ver, title, apiVer string
		endpoints                int
	}{
		{"petstore-3.0.yaml", "3.0.3", "Petstore", "1.0.0", 3},
		{"petstore-3.1.json", "3.1.0", "Petstore 31", "2.0.0", 1},
		{"webhooks-only-3.1.yaml", "3.1.0", "Hooks", "1", 0},
		{"merge-keys.yaml", "3.0.3", "Merge", "1", 1},
		{"ref-in-example.yaml", "3.0.3", "X", "1", 1},
		{"nested-circular.yaml", "3.0.3", "Circ", "1", 1},
	}
	for _, c := range cases {
		t.Run(c.file, func(t *testing.T) {
			doc := mustParse(t, c.file)
			if doc.OpenAPIVersion != c.ver || doc.Title != c.title || doc.APIVersion != c.apiVer {
				t.Fatalf("%+v", doc)
			}
			if n := len(ExtractEndpoints(doc)); n != c.endpoints {
				t.Fatalf("endpoints=%d", n)
			}
		})
	}
}

func TestParse_Rejections(t *testing.T) {
	cases := []struct {
		file, code string
	}{
		{"components-only-3.0.yaml", CodeInvalidStructure},
		{"package.json", CodeNotOpenAPI},
		{"broken.yaml", CodeSyntax},
		{"broken.json", CodeSyntax},
		{"swagger-1.2.yaml", CodeUnsupportedVersion},
		{"openapi-4.0.yaml", CodeUnsupportedVersion},
		{"openapi-number.yaml", CodeUnsupportedVersion},
		{"ref-unresolved.yaml", CodeUnresolvedRef},
		{"dup-operationid.yaml", CodeDuplicateOpID},
		{"billion-laughs.yaml", CodeYAMLLimit},
		{"deep.yaml", CodeYAMLLimit},
		{"alias-cycle.yaml", CodeYAMLLimit},
		{"multi-doc.yaml", CodeMultiDocument},
		{"invalid-utf8.yaml", CodeInvalidEncoding},
		{"nul.json", CodeInvalidEncoding},
	}
	for _, c := range cases {
		t.Run(c.file, func(t *testing.T) {
			_, is := parseFile(t, c.file)
			if len(is) == 0 || is[0].Code != c.code {
				t.Fatalf("want %s, got %+v", c.code, is)
			}
		})
	}
}

func TestParse_SyntaxIssuesHaveLines(t *testing.T) {
	for _, f := range []string{"broken.yaml", "broken.json"} {
		_, is := parseFile(t, f)
		if len(is) != 1 || is[0].Line <= 0 || !strings.Contains(is[0].Message, "baris") {
			t.Fatalf("%s: %+v", f, is)
		}
	}
	// baris JSON dari offset: kesalahan ada di baris 3.
	if _, is := parseFile(t, "broken.json"); is[0].Line != 3 {
		t.Fatalf("line=%d", is[0].Line)
	}
}

func TestParse_EmptyAndWhitespace(t *testing.T) {
	for _, in := range []string{"", "  \n\t ", "# hanya komentar\n"} {
		_, is := Parse([]byte(in), FormatYAML, DefaultLimits)
		if len(is) != 1 || is[0].Code != CodeEmpty {
			t.Errorf("%q: %+v", in, is)
		}
	}
	if _, is := Parse([]byte(" \n"), FormatJSON, DefaultLimits); len(is) != 1 || is[0].Code != CodeEmpty {
		t.Fatalf("%+v", is)
	}
}

func TestParse_JSONTrailingGarbage(t *testing.T) {
	_, is := Parse([]byte(`{"openapi":"3.0.3"} xyz`), FormatJSON, DefaultLimits)
	if len(is) != 1 || is[0].Code != CodeSyntax {
		t.Fatalf("%+v", is)
	}
}

func TestParse_BOM(t *testing.T) {
	raw := load(t, "bom.json")
	if _, is := Parse(raw, FormatJSON, DefaultLimits); len(is) == 0 {
		t.Fatal("BOM mentah seharusnya ditolak oleh parser JSON")
	}
	stripped := StripBOM(raw)
	if bytes.HasPrefix(stripped, []byte{0xEF}) {
		t.Fatal("BOM tidak dibuang")
	}
	if _, is := Parse(stripped, FormatJSON, DefaultLimits); len(is) != 0 {
		t.Fatalf("%+v", is)
	}
	if !bytes.Equal(StripBOM([]byte("abc")), []byte("abc")) {
		t.Fatal("tanpa BOM harus tetap")
	}
}

func TestParse_ExternalRefs(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits.Add(1) }))
	defer srv.Close()
	port := srv.Listener.Addr().String()[strings.LastIndex(srv.Listener.Addr().String(), ":")+1:]
	content := strings.ReplaceAll(string(load(t, "ref-http.yaml")), "__PORT__", port)
	_, is := Parse([]byte(content), FormatYAML, DefaultLimits)
	if len(is) != 1 || is[0].Code != CodeExternalRef || is[0].Line == 0 || !strings.HasPrefix(is[0].Pointer, "#/paths/~1pets/get") {
		t.Fatalf("%+v", is)
	}
	if hits.Load() != 0 {
		t.Fatalf("parser melakukan %d request keluar", hits.Load())
	}
	_, is = parseFile(t, "ref-file.yaml")
	if len(is) != 2 || is[0].Code != CodeExternalRef || is[1].Code != CodeExternalRef {
		t.Fatalf("%+v", is)
	}
}

func TestParse_UnresolvedRefHasPointerAndLine(t *testing.T) {
	_, is := parseFile(t, "ref-unresolved.yaml")
	if len(is) != 1 || is[0].Line == 0 || !strings.Contains(is[0].Pointer, "schema") ||
		!strings.Contains(is[0].Message, "#/components/schemas/Missing") {
		t.Fatalf("%+v", is)
	}
}

func TestParse_DuplicateOperationIDMentionsBoth(t *testing.T) {
	_, is := parseFile(t, "dup-operationid.yaml")
	if len(is) != 1 {
		t.Fatalf("%+v", is)
	}
	if is[0].Pointer != "#/paths/~1b/get/operationId" || !strings.Contains(is[0].Message, "#/paths/~1a/get/operationId") {
		t.Fatalf("%+v", is[0])
	}
}

func TestParse_ManyErrorsCappedAndSorted(t *testing.T) {
	_, is := parseFile(t, "many-errors.yaml")
	if len(is) != MaxIssues {
		t.Fatalf("got %d issues", len(is))
	}
	for i := 1; i < len(is); i++ {
		if is[i].Line < is[i-1].Line {
			t.Fatalf("tidak urut: %+v", is)
		}
	}
}

func TestParse_MergeKeysExplicitWins(t *testing.T) {
	doc := mustParse(t, "merge-keys.yaml")
	m, ok := ResolvePointer(doc.Root, "#/x-merged")
	if !ok {
		t.Fatal("x-merged hilang")
	}
	mm := m.(*Map)
	if v, _ := mm.Get("summary"); v != "eksplisit" {
		t.Fatalf("summary=%v", v)
	}
	if v, _ := mm.Get("description"); v != "dari base" {
		t.Fatalf("description=%v", v)
	}
}

func TestParse_NumbersAndKeys(t *testing.T) {
	doc := mustParse(t, "petstore-3.1.json")
	b, err := ToJSON(doc)
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if !strings.Contains(s, `"200":{"description":"OK"`) || !strings.Contains(s, `"type":["string","null"]`) {
		t.Fatal(s)
	}
	// YAML: kunci integer menjadi string, nilai angka dinormalkan, nullable dibiarkan.
	y := "openapi: 3.0.3\ninfo: {title: X, version: '1'}\npaths: {}\nx-n: {200: a, 0x10: b, hex: 0x1F, under: 1_000, f: 1.5, inf: .inf, t: true, n: ~}\n"
	d2, is := Parse([]byte(y), FormatYAML, DefaultLimits)
	if len(is) != 0 {
		t.Fatalf("%+v", is)
	}
	b, _ = ToJSON(d2)
	for _, want := range []string{`"200":"a"`, `"0x10":"b"`, `"hex":31`, `"under":1000`, `"f":1.5`, `"inf":".inf"`, `"t":true`, `"n":null`} {
		if !strings.Contains(string(b), want) {
			t.Errorf("tidak memuat %s dalam %s", want, b)
		}
	}
	if !strings.Contains(string(mustJSON(t, mustParse(t, "petstore-3.0.yaml"))), `"nullable":true`) {
		t.Fatal("nullable harus dibiarkan")
	}
}

func mustJSON(t testing.TB, d Document) []byte {
	t.Helper()
	b, err := ToJSON(d)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestParse_DuplicateKeysRejected(t *testing.T) {
	y := "openapi: 3.0.3\ninfo: {title: X, version: '1'}\npaths: {}\npaths: {}\n"
	if _, is := Parse([]byte(y), FormatYAML, DefaultLimits); len(is) != 1 || is[0].Code != CodeSyntax {
		t.Fatalf("%+v", is)
	}
	j := `{"openapi":"3.0.3","info":{"title":"X","version":"1"},"paths":{},"paths":{}}`
	if _, is := Parse([]byte(j), FormatJSON, DefaultLimits); len(is) != 1 || is[0].Code != CodeSyntax {
		t.Fatalf("%+v", is)
	}
}

func TestParse_JSONSurrogateEscapes(t *testing.T) {
	j := `{"openapi":"3.0.3","info":{"title":"emoji \ud83d\ude00","version":"1"},"paths":{}}`
	doc, is := Parse([]byte(j), FormatJSON, DefaultLimits)
	if len(is) != 0 || doc.Title != "emoji \U0001F600" {
		t.Fatalf("%+v %q", is, doc.Title)
	}
}

func TestParse_JSONDepthLimit(t *testing.T) {
	j := `{"openapi":"3.0.3","info":{"title":"X","version":"1"},"paths":{},"x":` + strings.Repeat("[", 200) + strings.Repeat("]", 200) + `}`
	if _, is := Parse([]byte(j), FormatJSON, DefaultLimits); len(is) != 1 || is[0].Code != CodeYAMLLimit {
		t.Fatalf("%+v", is)
	}
}

func TestParse_TopLevelNotMap(t *testing.T) {
	for _, in := range []string{"- a\n- b\n", "plain scalar\n", "42\n"} {
		if _, is := Parse([]byte(in), FormatYAML, DefaultLimits); len(is) != 1 || is[0].Code != CodeNotOpenAPI {
			t.Errorf("%q: %+v", in, is)
		}
	}
	if _, is := Parse([]byte(`[1,2]`), FormatJSON, DefaultLimits); len(is) != 1 || is[0].Code != CodeNotOpenAPI {
		t.Fatalf("%+v", is)
	}
}

func TestParse_StructureRules(t *testing.T) {
	base := func(paths string) string {
		return "openapi: 3.0.3\ninfo: {title: X, version: '1'}\n" + paths
	}
	bad := map[string]string{
		"path tanpa slash":    base("paths:\n  pets: {}\n"),
		"response code":       base("paths:\n  /a:\n    get:\n      responses:\n        '99': {description: x}\n"),
		"param in":            base("paths:\n  /a:\n    get:\n      parameters: [{name: x, in: body}]\n      responses: {'200': {description: x}}\n"),
		"path param opsional": base("paths:\n  /a/{id}:\n    get:\n      parameters: [{name: id, in: path}]\n      responses: {'200': {description: x}}\n"),
		"deprecated":          base("paths:\n  /a:\n    get:\n      deprecated: yes-no\n      responses: {'200': {description: x}}\n"),
		"3.0 tanpa responses": base("paths:\n  /a:\n    get: {}\n"),
		"servers":             base("paths: {}\nservers: [{name: x}]\n"),
		"components":          base("paths: {}\ncomponents: {schemas: [1]}\n"),
		"info version":        "openapi: 3.0.3\ninfo: {title: X, version: 1.0}\npaths: {}\n",
		"ref non-string":      base("paths: {}\ncomponents: {schemas: {A: {$ref: 5}}}\n"),
	}
	for name, y := range bad {
		if _, is := Parse([]byte(y), FormatYAML, DefaultLimits); len(is) == 0 || is[0].Code != CodeInvalidStructure {
			t.Errorf("%s: %+v", name, is)
		}
	}
	good := map[string]string{
		"3.1 tanpa responses":    "openapi: 3.1.0\ninfo: {title: X, version: '1'}\npaths:\n  /a:\n    get: {}\n",
		"x- pada paths":          base("paths:\n  x-foo: 1\n"),
		"path item ref internal": base("paths:\n  /a:\n    $ref: '#/components/pathItems/A'\ncomponents:\n  pathItems:\n    A: {}\n"),
		"field tak dikenal":      base("paths: {}\nfoo: bar\nx-bar: {}\n"),
		"percent ref":            base("paths: {}\ncomponents:\n  schemas:\n    'A B': {type: string}\n    C: {$ref: '#/components/schemas/A%20B'}\n"),
	}
	for name, y := range good {
		if _, is := Parse([]byte(y), FormatYAML, DefaultLimits); len(is) != 0 {
			t.Errorf("%s: %+v", name, is)
		}
	}
}

func TestParse_ResponsesDefaultRefChecked(t *testing.T) {
	y := "openapi: 3.0.3\ninfo: {title: X, version: '1'}\npaths:\n  /a:\n    get:\n      responses:\n        default:\n          $ref: '#/components/responses/Nope'\n"
	if _, is := Parse([]byte(y), FormatYAML, DefaultLimits); len(is) != 1 || is[0].Code != CodeUnresolvedRef {
		t.Fatalf("%+v", is)
	}
}

func TestT06_AC3_BillionLaughsCheap(t *testing.T) {
	content := load(t, "billion-laughs.yaml")
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	start := time.Now()
	_, is := Parse(content, FormatYAML, DefaultLimits)
	elapsed := time.Since(start)
	runtime.ReadMemStats(&after)
	if len(is) != 1 || is[0].Code != CodeYAMLLimit {
		t.Fatalf("%+v", is)
	}
	if elapsed > time.Second {
		t.Fatalf("terlalu lambat: %s", elapsed)
	}
	if alloc := after.TotalAlloc - before.TotalAlloc; alloc > 50<<20 {
		t.Fatalf("alokasi %d byte", alloc)
	}
}

func TestT06_AC3_LargeJSONBaseline(t *testing.T) {
	var b strings.Builder
	b.WriteString(`{"openapi":"3.0.3","info":{"title":"Big","version":"1"},"paths":{`)
	n := 0
	for b.Len() < 5<<20 {
		if n > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, `"/p%d":{"get":{"operationId":"op%d","summary":"%s","responses":{"200":{"description":"OK"}}}}`, n, n, strings.Repeat("x", 100))
		n++
	}
	b.WriteString(`}}`)
	if !json.Valid([]byte(b.String())) {
		t.Fatal("fixture rusak")
	}
	start := time.Now()
	doc, is := Parse([]byte(b.String()), FormatJSON, DefaultLimits)
	if len(is) != 0 || len(ExtractEndpoints(doc)) != n {
		t.Fatalf("issues=%d", len(is))
	}
	if time.Since(start) > 2*time.Second*raceSlowdown {
		t.Fatalf("lambat: %s", time.Since(start))
	}
}

func TestMap_MarshalJSONKeepsOrderAndEscapes(t *testing.T) {
	m := NewMap()
	m.Set("z", "a<b>&\u2028")
	m.Set("a", json.Number("12"))
	m.Set("m", []any{nil, true, 1.5})
	got, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	want, _ := json.Marshal(map[string]any{"z": "a<b>&\u2028"})
	if !strings.HasPrefix(string(got), `{"z":`+strings.TrimSuffix(strings.TrimPrefix(string(want), `{"z":`), `}`)+`,"a":12,"m":[null,true,1.5]}`) {
		t.Fatalf("%s", got)
	}
	if m.Len() != 3 || m.Keys()[0] != "z" {
		t.Fatal("keys")
	}
	var nilMap *Map
	if nilMap.Len() != 0 || nilMap.Keys() != nil {
		t.Fatal("nil map")
	}
	if _, ok := nilMap.Get("x"); ok {
		t.Fatal("nil get")
	}
}

func TestResolvePointer(t *testing.T) {
	inner := NewMap()
	inner.Set("a/b", "slash")
	inner.Set("c~d", "tilde")
	inner.Set("e f", "space")
	root := NewMap()
	root.Set("m", inner)
	root.Set("arr", []any{"zero", "one"})
	cases := map[string]any{
		"#/m/a~1b": "slash", "#/m/c~0d": "tilde", "#/m/e%20f": "space", "#/arr/1": "one", "#": root,
	}
	for ref, want := range cases {
		got, ok := ResolvePointer(root, ref)
		if !ok || got != want {
			t.Errorf("%s: %v %v", ref, got, ok)
		}
	}
	for _, ref := range []string{"#/nope", "#/arr/2", "#/arr/x", "#/arr/-1", "#/arr/", "x", "#a", "#/m/a/b"} {
		if _, ok := ResolvePointer(root, ref); ok {
			t.Errorf("%s harus gagal", ref)
		}
	}
	if EscapePointerToken("a/b~c") != "a~1b~0c" {
		t.Fatal("escape")
	}
}

func TestParse_Swagger2Accepted(t *testing.T) {
	for _, f := range []string{"swagger-2.0.yaml", "swagger-2.0-petstore.yaml", "swagger-2.0-petstore.json"} {
		t.Run(f, func(t *testing.T) {
			doc := mustParse(t, f)
			if doc.OpenAPIVersion != "2.0" || !doc.IsSwagger2() || doc.Title != "Swagger Petstore" || doc.APIVersion != "1.0.0" {
				t.Fatalf("%+v", doc)
			}
			if n := len(ExtractEndpoints(doc)); n != 4 {
				t.Fatalf("endpoints=%d", n)
			}
		})
	}
	if doc := mustParse(t, "swagger-2.0-numeric.yaml"); doc.OpenAPIVersion != Swagger2Version {
		t.Fatalf("%+v", doc)
	}
}

func TestParse_Swagger2VersionForms(t *testing.T) {
	ok := []struct {
		name, src string
		f         Format
	}{
		{"yaml string", "swagger: \"2.0\"\ninfo: {title: X, version: '1'}\npaths: {}\n", FormatYAML},
		{"yaml float", "swagger: 2.0\ninfo: {title: X, version: '1'}\npaths: {}\n", FormatYAML},
		{"yaml int", "swagger: 2\ninfo: {title: X, version: '1'}\npaths: {}\n", FormatYAML},
		{"json int", `{"swagger":2,"info":{"title":"X","version":"1"},"paths":{}}`, FormatJSON},
		{"json float", `{"swagger":2.0,"info":{"title":"X","version":"1"},"paths":{}}`, FormatJSON},
	}
	for _, c := range ok {
		t.Run(c.name, func(t *testing.T) {
			doc, is := Parse([]byte(c.src), c.f, DefaultLimits)
			if len(is) != 0 || doc.OpenAPIVersion != "2.0" {
				t.Fatalf("%+v %+v", doc, is)
			}
		})
	}
	bad := []struct{ name, val, msg string }{
		{"patch", `"2.0.0"`, `versi Swagger "2.0.0" tidak didukung (hanya 2.0)`},
		{"1.2", `"1.2"`, `versi Swagger "1.2" tidak didukung (hanya 2.0)`},
		{"3.0", `"3.0"`, `versi Swagger "3.0" tidak didukung (hanya 2.0)`},
		{"array", `[2]`, `versi Swagger "?" tidak didukung (hanya 2.0)`},
		{"map", `{a: 1}`, `versi Swagger "?" tidak didukung (hanya 2.0)`},
		{"float 1.2", `1.2`, `versi Swagger "1.2" tidak didukung (hanya 2.0)`},
	}
	for _, c := range bad {
		t.Run(c.name, func(t *testing.T) {
			src := "swagger: " + c.val + "\ninfo: {title: X, version: '1'}\npaths: {}\n"
			_, is := Parse([]byte(src), FormatYAML, DefaultLimits)
			if len(is) != 1 || is[0].Code != CodeUnsupportedVersion || is[0].Pointer != "#/swagger" || is[0].Line <= 0 ||
				!strings.Contains(is[0].Message, c.msg) {
				t.Fatalf("%+v", is)
			}
		})
	}
	_, is := parseFile(t, "swagger-null.yaml")
	if len(is) != 1 || is[0].Code != CodeNotOpenAPI || is[0].Pointer != "#" {
		t.Fatalf("%+v", is)
	}
	_, is = Parse([]byte(`{"swagger":null}`), FormatJSON, DefaultLimits)
	if len(is) != 1 || is[0].Code != CodeNotOpenAPI {
		t.Fatalf("%+v", is)
	}
	// JSON: baris dihitung lewat jsonLineNode.
	_, is = Parse([]byte("{\n\"swagger\": \"1.2\"\n}"), FormatJSON, DefaultLimits)
	if len(is) != 1 || is[0].Code != CodeUnsupportedVersion || is[0].Line != 2 {
		t.Fatalf("%+v", is)
	}
}

func TestParse_BothKeysIs3x(t *testing.T) {
	doc := mustParse(t, "both-keys.yaml")
	if doc.OpenAPIVersion != "3.0.3" || doc.IsSwagger2() {
		t.Fatalf("%+v", doc)
	}
}

func TestParse_SwaggerKeyInsideInfoIs3x(t *testing.T) {
	doc := mustParse(t, "swagger-in-info.yaml")
	if doc.OpenAPIVersion != "3.0.3" || doc.IsSwagger2() {
		t.Fatalf("%+v", doc)
	}
}

func TestParse_NotOpenAPI3NeverEmitted(t *testing.T) {
	entries, err := os.ReadDir("testdata")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		_, is := parseFile(t, e.Name())
		for _, i := range is {
			if i.Code == CodeNotOpenAPI3 {
				t.Errorf("%s menghasilkan %s", e.Name(), CodeNotOpenAPI3)
			}
		}
	}
}
