package listfiles

import (
	"encoding/json"
	"strings"
	"testing"

	"grimoire/lib/contracts/project/knowledge/listfiles"
)

func TestParse_Valid(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want Params
	}{
		{"empty object", `{}`, Params{"", false, 200, 0}},
		{"null", `null`, Params{"", false, 200, 0}},
		{"empty raw", ``, Params{"", false, 200, 0}},
		{"path trimmed", `{"path":"  /docs/  "}`, Params{"docs", false, 200, 0}},
		{"path slash only", `{"path":"/"}`, Params{"", false, 200, 0}},
		{"recursive", `{"recursive":true}`, Params{"", true, 200, 0}},
		{"limit min", `{"limit":1}`, Params{"", false, 1, 0}},
		{"limit max", `{"limit":500}`, Params{"", false, 500, 0}},
		{"offset max", `{"offset":1000000}`, Params{"", false, 200, 1000000}},
		{"dots in name", `{"path":"a..b"}`, Params{"a..b", false, 200, 0}},
		{"path 1024", `{"path":"` + strings.Repeat("a", 1024) + `"}`, Params{strings.Repeat("a", 1024), false, 200, 0}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := Parse(json.RawMessage(c.in))
			if err != nil {
				t.Fatalf("err: %s", err.Message())
			}
			if got != c.want {
				t.Fatalf("got %+v want %+v", got, c.want)
			}
		})
	}
}

func TestParse_Invalid(t *testing.T) {
	cases := []struct {
		name, in, field, contains string
	}{
		{"limit 0", `{"limit":0}`, "limit", "harus 1-500"},
		{"limit 501", `{"limit":501}`, "limit", "(diterima: 501)"},
		{"limit string", `{"limit":"x"}`, "limit", "harus integer"},
		{"limit float", `{"limit":1.5}`, "limit", "harus integer"},
		{"offset negative", `{"offset":-1}`, "offset", "harus 0-1000000"},
		{"offset too big", `{"offset":1000001}`, "offset", "harus 0-1000000"},
		{"recursive string", `{"recursive":"true"}`, "recursive", "harus boolean"},
		{"project_id", `{"project_id":"x"}`, "project_id", "ditentukan oleh API key"},
		{"projectId", `{"projectId":"x"}`, "projectId", "ditentukan oleh API key"},
		{"project", `{"project":"x"}`, "project", "ditentukan oleh API key"},
		{"unknown", `{"foo":1}`, "foo", "foo tidak dikenal"},
		{"path too long", `{"path":"` + strings.Repeat("a", 1025) + `"}`, "path", "maksimal 1024 byte"},
		{"dotdot", `{"path":"../a"}`, "path", ".."},
		{"dotdot middle", `{"path":"a/../b"}`, "path", ".."},
		{"nul", `{"path":"a\u0000b"}`, "path", "karakter kontrol"},
		{"array", `[]`, "arguments", ""},
		{"number", `1`, "arguments", ""},
		{"string", `"x"`, "arguments", ""},
		{"broken", `{`, "arguments", ""},
		{"first error wins", `{"limit":900,"offset":-1}`, "limit", "harus 1-500"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := Parse(json.RawMessage(c.in))
			if err == nil {
				t.Fatal("harus error")
			}
			if err.Field != c.field {
				t.Fatalf("field %q want %q (%s)", err.Field, c.field, err.Message())
			}
			if !strings.Contains(err.Message(), c.contains) {
				t.Fatalf("pesan %q tidak memuat %q", err.Message(), c.contains)
			}
		})
	}
}

func TestParse_LimitMessageExact(t *testing.T) {
	_, err := Parse(json.RawMessage(`{"limit":900}`))
	want := "Argumen tidak valid: limit harus 1-500 (diterima: 900). Gunakan offset untuk halaman berikutnya."
	if err == nil || err.Message() != want {
		t.Fatalf("got %v", err)
	}
}

func TestParse_UnknownListsSupported(t *testing.T) {
	_, err := Parse(json.RawMessage(`{"foo":1}`))
	if err == nil || !strings.Contains(err.Message(), "path, recursive, limit, offset") {
		t.Fatalf("got %v", err)
	}
}

func TestMaxLimitMatchesContract(t *testing.T) {
	if maxLimit != listfiles.MaxLimit {
		t.Fatalf("maxLimit %d != contract %d", maxLimit, listfiles.MaxLimit)
	}
}

func TestSchema_Valid(t *testing.T) {
	var s map[string]any
	if err := json.Unmarshal(schema(), &s); err != nil {
		t.Fatal(err)
	}
	if s["type"] != "object" || s["additionalProperties"] != false {
		t.Fatalf("schema: %v", s)
	}
	if _, ok := s["required"]; ok {
		t.Fatal("tidak boleh ada required")
	}
	props := s["properties"].(map[string]any)
	for _, bad := range []string{"project_id", "projectId", "project"} {
		if _, ok := props[bad]; ok {
			t.Fatalf("properti %s terlarang", bad)
		}
	}
	if props["limit"].(map[string]any)["default"] != float64(200) ||
		props["offset"].(map[string]any)["default"] != float64(0) ||
		props["recursive"].(map[string]any)["default"] != false {
		t.Fatalf("default salah: %v", props)
	}
}
