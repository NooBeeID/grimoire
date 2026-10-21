package listapispecs

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestParse_AcceptsEmpty(t *testing.T) {
	for _, in := range []string{"", "  ", "null", "{}", "{ }"} {
		if _, e := Parse(json.RawMessage(in)); e != nil {
			t.Errorf("%q: %v", in, e.Message())
		}
	}
	if _, e := Parse(nil); e != nil {
		t.Fatal(e.Message())
	}
}

func TestParse_RejectsAnyArgument(t *testing.T) {
	for _, in := range []string{`{"foo":1}`, `{"spec":"petstore"}`, `{"limit":5}`, `{"a":null}`} {
		_, e := Parse(json.RawMessage(in))
		if e == nil {
			t.Fatalf("%s diterima", in)
		}
		m := e.Message()
		if !strings.Contains(m, "tidak dikenal") || !strings.Contains(m, "Tool ini tidak menerima argumen.") {
			t.Errorf("%s: %s", in, m)
		}
		if e.Summary()["error_code"] != "invalid_argument" {
			t.Errorf("summary %v", e.Summary())
		}
	}
}

func TestParse_ProjectAliases(t *testing.T) {
	for _, k := range []string{"project_id", "projectId", "project"} {
		_, e := Parse(json.RawMessage(`{"` + k + `":"x"}`))
		if e == nil {
			t.Fatal(k)
		}
		m := e.Message()
		if !strings.Contains(m, "ditentukan oleh API key") || !strings.Contains(m, "Hapus argumen ini") || !strings.Contains(m, k) {
			t.Errorf("%s: %s", k, m)
		}
	}
}

func TestParse_NotObject(t *testing.T) {
	for _, in := range []string{`[]`, `1`, `"x"`, `true`} {
		_, e := Parse(json.RawMessage(in))
		if e == nil || e.Field != "arguments" || !strings.Contains(e.Message(), "harus object JSON") {
			t.Errorf("%s: %v", in, e)
		}
	}
}

func TestParse_TrailingTokens(t *testing.T) {
	for _, in := range []string{`{}{"a":1}`, `{} x`} {
		_, e := Parse(json.RawMessage(in))
		if e == nil || e.Field != "arguments" {
			t.Errorf("%s: %v", in, e)
		}
	}
	_, e := Parse(json.RawMessage(`{}{"a":1}`))
	if !strings.Contains(e.Message(), "harus satu object JSON") {
		t.Error(e.Message())
	}
}

func TestParse_BrokenJSON(t *testing.T) {
	for _, in := range []string{`{`, `{"a":`} {
		_, e := Parse(json.RawMessage(in))
		if e == nil || !strings.Contains(e.Message(), "bukan JSON valid") {
			t.Errorf("%s: %v", in, e)
		}
	}
}

func TestSchema_Exact(t *testing.T) {
	var m map[string]any
	if err := json.Unmarshal(schema(), &m); err != nil {
		t.Fatal(err)
	}
	if len(m) != 3 || m["type"] != "object" || m["additionalProperties"] != false {
		t.Fatalf("%v", m)
	}
	props, ok := m["properties"].(map[string]any)
	if !ok || len(props) != 0 {
		t.Fatalf("%v", m["properties"])
	}
	if _, ok := m["required"]; ok {
		t.Fatal("required")
	}
}

func TestParams_NoFields(t *testing.T) {
	if reflect.TypeOf(Params{}).NumField() != 0 {
		t.Fatal("Params harus kosong")
	}
}
