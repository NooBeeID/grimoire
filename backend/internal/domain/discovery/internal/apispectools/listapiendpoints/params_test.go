package listapiendpoints

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"

	"grimoire/lib/contracts/project/apispec/listendpoints"
)

func TestParse_Valid(t *testing.T) {
	cases := []struct {
		in   string
		want Params
	}{
		{``, Params{Limit: 100}},
		{`null`, Params{Limit: 100}},
		{`{}`, Params{Limit: 100}},
		{` `, Params{Limit: 100}},
		{`{"spec":"  petstore "}`, Params{Spec: "petstore", Limit: 100}},
		{`{"tag":"pets"}`, Params{Tag: "pets", Limit: 100}},
		{`{"spec":"` + strings.Repeat("é", 80) + `"}`, Params{Spec: strings.Repeat("é", 80), Limit: 100}},
		{`{"query":"` + strings.Repeat("q", 200) + `"}`, Params{Query: strings.Repeat("q", 200), Limit: 100}},
		{`{"limit":1}`, Params{Limit: 1}},
		{`{"limit":300}`, Params{Limit: 300}},
		{`{"limit":null}`, Params{Limit: 100}},
		{`{"offset":0}`, Params{Limit: 100}},
		{`{"offset":1000000}`, Params{Limit: 100, Offset: 1000000}},
	}
	for _, c := range cases {
		got, err := Parse(json.RawMessage(c.in))
		if err != nil || got != c.want {
			t.Errorf("%q: got %+v %v want %+v", c.in, got, err, c.want)
			continue
		}
		if got.Limit < 1 || got.Limit > 300 || got.Offset < 0 || utf8.RuneCountInString(got.Query) > 200 {
			t.Errorf("%q: nilai di luar rentang provider", c.in)
		}
	}
}

func TestParse_Invalid(t *testing.T) {
	cases := []struct {
		in, field, contains string
	}{
		{`{"spec":"` + strings.Repeat("a", 81) + `"}`, "spec", "spec maksimal 80 karakter"},
		{`{"spec":"a\tb"}`, "spec", "spec memuat karakter kontrol"},
		{`{"tag":"` + strings.Repeat("a", 201) + `"}`, "tag", "tag maksimal 200 karakter"},
		{`{"query":"` + strings.Repeat("a", 201) + `"}`, "query", "query maksimal 200 karakter"},
		{`{"limit":0}`, "limit", "limit harus 1-300"},
		{`{"limit":301}`, "limit", "limit harus 1-300"},
		{`{"limit":-5}`, "limit", "limit harus 1-300"},
		{`{"limit":500}`, "limit", "Argumen tidak valid: limit harus 1-300 (diterima: 500). Gunakan offset untuk halaman berikutnya."},
		{`{"limit":"x"}`, "limit", "limit harus integer"},
		{`{"limit":1.5}`, "limit", "limit harus integer"},
		{`{"offset":-1}`, "offset", "offset harus 0-1000000"},
		{`{"offset":1000001}`, "offset", "offset harus 0-1000000"},
		{`{"spec":1}`, "spec", "spec harus string"},
		{`[]`, "arguments", "arguments harus object JSON"},
		{`1`, "arguments", "arguments harus object JSON"},
		{`"x"`, "arguments", "arguments harus object JSON"},
		{`{"limit":1}{"x":1}`, "arguments", "harus satu object JSON"},
		{`{"foo":1}`, "foo", "Argumen tidak valid: foo tidak dikenal. Argumen yang didukung: spec, tag, query, limit, offset."},
		{`{"spec":"a","limit":0,"offset":-1}`, "limit", "limit harus 1-300"},
	}
	for _, c := range cases {
		_, err := Parse(json.RawMessage(c.in))
		if err == nil {
			t.Errorf("%q: harus error", c.in)
			continue
		}
		if err.Field != c.field || !strings.Contains(err.Message(), c.contains) {
			t.Errorf("%q: field=%s msg=%q", c.in, err.Field, err.Message())
		}
	}
}

func TestT09_AC6_ProjectArgRejected(t *testing.T) {
	for _, k := range []string{"project_id", "projectId", "project"} {
		_, err := Parse(json.RawMessage(`{"` + k + `":"x"}`))
		if err == nil || !strings.Contains(err.Message(), "ditentukan oleh API key") {
			t.Errorf("%s: %v", k, err)
		}
	}
}

func TestLimitsMatchContract(t *testing.T) {
	if maxLimit != listendpoints.MaxLimit || defaultLimit != listendpoints.DefaultLimit || maxQueryRunes != listendpoints.MaxQueryRunes {
		t.Fatal("konstanta tidak sama dengan kontrak T06")
	}
}

func TestSchema_Valid(t *testing.T) {
	var s struct {
		AdditionalProperties bool `json:"additionalProperties"`
		Properties           map[string]struct {
			Minimum *int `json:"minimum"`
			Maximum *int `json:"maximum"`
			Default *int `json:"default"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(schema(), &s); err != nil {
		t.Fatal(err)
	}
	if s.AdditionalProperties {
		t.Fatal("additionalProperties harus false")
	}
	l, o := s.Properties["limit"], s.Properties["offset"]
	if *l.Minimum != 1 || *l.Maximum != maxLimit || *l.Default != defaultLimit {
		t.Fatalf("limit: %+v", l)
	}
	if *o.Minimum != 0 || *o.Maximum != maxOffset || *o.Default != 0 {
		t.Fatalf("offset: %+v", o)
	}
}
