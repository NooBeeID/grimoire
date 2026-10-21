package readfile

import (
	"encoding/json"
	"strings"
	"testing"

	"grimoire/internal/domain/discovery/internal/knowledgetools/base"
)

func TestParse_Valid(t *testing.T) {
	max := base.MaxReadLimit(102400)
	cases := []struct {
		name, in string
		max      int
		want     Params
	}{
		{"slash depan", `{"path":"/docs/a.md"}`, max, Params{"docs/a.md", 0, 40000}},
		{"trim", `{"path":"  docs/a.md  "}`, max, Params{"docs/a.md", 0, 40000}},
		{"default", `{"path":"a"}`, max, Params{"a", 0, 40000}},
		{"limit maks", `{"path":"a","offset":5,"limit":101888}`, max, Params{"a", 5, 101888}},
		{"config kecil", `{"path":"a"}`, base.MaxReadLimit(1024), Params{"a", 0, 1024}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, e := Parse(json.RawMessage(c.in), c.max)
			if e != nil || got != c.want {
				t.Fatalf("got %+v err=%v, want %+v", got, e, c.want)
			}
		})
	}
}

func TestParse_Invalid(t *testing.T) {
	max := base.MaxReadLimit(102400)
	pathReq := "Argumen tidak valid: path wajib diisi. Dapatkan path dengan list_files atau search_knowledge."
	cases := []struct {
		name, in, field string
		max             int
		contains        []string
		exact           string
	}{
		{"path wajib", `{}`, "path", max, nil, pathReq},
		{"path kosong", `{"path":""}`, "path", max, nil, pathReq},
		{"path spasi", `{"path":"   "}`, "path", max, nil, pathReq},
		{"null", `null`, "path", max, nil, pathReq},
		{"raw kosong", ``, "path", max, nil, pathReq},
		{"folder", `{"path":"docs/"}`, "path", max, []string{"menunjuk folder", "list_files"}, ""},
		{"dotdot", `{"path":"a/../b"}`, "path", max, []string{".."}, ""},
		{"terlalu panjang", `{"path":"` + strings.Repeat("a", 1025) + `"}`, "path", max, []string{"1024"}, ""},
		{"offset negatif", `{"path":"a","offset":-1}`, "offset", max, []string{"(diterima: -1)"}, ""},
		{"offset string", `{"path":"a","offset":"1"}`, "offset", max, []string{"harus integer"}, ""},
		{"offset desimal", `{"path":"a","offset":1.5}`, "offset", max, []string{"harus integer"}, ""},
		{"limit nol", `{"path":"a","limit":0}`, "limit", max, nil, ""},
		{"limit lewat maks", `{"path":"a","limit":101889}`, "limit", max, []string{"101888", "101889"}, ""},
		{"limit config kecil", `{"path":"a","limit":1025}`, "limit", base.MaxReadLimit(1024), []string{"1024"}, ""},
		{"project_id", `{"path":"a","project_id":"x"}`, "", max, []string{"ditentukan oleh API key"}, ""},
		{"projectId", `{"path":"a","projectId":"x"}`, "", max, []string{"ditentukan oleh API key"}, ""},
		{"project", `{"path":"a","project":"x"}`, "", max, []string{"ditentukan oleh API key"}, ""},
		{"field asing", `{"path":"a","foo":1}`, "", max, []string{"foo tidak dikenal", "path, offset, limit"}, ""},
		{"array", `[]`, "arguments", max, nil, ""},
		{"angka", `1`, "arguments", max, nil, ""},
		{"token sisa", `{"path":"a"}{"x":1}`, "arguments", max, nil, ""},
		{"urutan error", `{"offset":-1,"limit":0}`, "path", max, nil, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, e := Parse(json.RawMessage(c.in), c.max)
			if e == nil {
				t.Fatalf("want error, got %+v", got)
			}
			if got != (Params{}) {
				t.Fatalf("Params harus kosong: %+v", got)
			}
			if c.field != "" && e.Field != c.field {
				t.Fatalf("field=%q want %q (%s)", e.Field, c.field, e.Message())
			}
			if c.exact != "" && e.Message() != c.exact {
				t.Fatalf("msg=%q", e.Message())
			}
			for _, s := range c.contains {
				if !strings.Contains(e.Message(), s) {
					t.Fatalf("msg %q tidak memuat %q", e.Message(), s)
				}
			}
		})
	}
}

func schemaMap(t *testing.T, max int) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(schema(max), &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestSchema(t *testing.T) {
	m := schemaMap(t, 101888)
	props := m["properties"].(map[string]any)
	limit := props["limit"].(map[string]any)
	if limit["maximum"] != float64(101888) || limit["default"] != float64(40000) {
		t.Fatalf("limit: %v", limit)
	}
	if m["additionalProperties"] != false || m["type"] != "object" {
		t.Fatalf("schema: %v", m)
	}
	req := m["required"].([]any)
	if len(req) != 1 || req[0] != "path" {
		t.Fatalf("required: %v", req)
	}
	for k := range props {
		if strings.Contains(k, "project") {
			t.Fatalf("properti project: %s", k)
		}
	}
	small := schemaMap(t, 1024)["properties"].(map[string]any)["limit"].(map[string]any)
	if small["maximum"] != float64(1024) || small["default"] != float64(1024) {
		t.Fatalf("limit kecil: %v", small)
	}
}
