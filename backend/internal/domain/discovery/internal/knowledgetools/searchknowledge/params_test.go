package searchknowledge

import (
	"encoding/json"
	"strings"
	"testing"
)

const wantQueryRequired = `Argumen tidak valid: query wajib diisi 1-200 karakter. Contoh: query="rate limit".`

func jq(s string) string { b, _ := json.Marshal(s); return string(b) }

func TestParse_QueryRequired(t *testing.T) {
	for _, in := range []string{`{}`, `{"query":""}`, `{"query":"   "}`, `{"query":"\t\n"}`, `null`, ``, ` `} {
		_, e := Parse(json.RawMessage(in))
		if e == nil || e.Field != "query" || e.Message() != wantQueryRequired {
			t.Fatalf("in=%q e=%+v", in, e)
		}
	}
}

func TestParse_QueryWrongType(t *testing.T) {
	_, e := Parse(json.RawMessage(`{"query":5}`))
	if e == nil || e.Field != "query" || e.Reason != "harus string" {
		t.Fatalf("%+v", e)
	}
}

func TestParse_QueryTrimmed(t *testing.T) {
	p, e := Parse(json.RawMessage(`{"query":"  rate limit  "}`))
	if e != nil || p.Query != "rate limit" {
		t.Fatalf("%+v %+v", p, e)
	}
}

func TestParse_QueryLengths(t *testing.T) {
	ok := []string{strings.Repeat("é", 200), strings.Repeat("日", 200), strings.Repeat("a", 200),
		strings.Repeat(" ", 5) + strings.Repeat("a", 200) + strings.Repeat(" ", 5)}
	for _, q := range ok {
		p, e := Parse(json.RawMessage(`{"query":` + jq(q) + `}`))
		if e != nil || p.Query != strings.TrimSpace(q) {
			t.Fatalf("ditolak len=%d: %+v", len(q), e)
		}
	}
	for _, q := range []string{strings.Repeat("é", 201), strings.Repeat("a", 201)} {
		_, e := Parse(json.RawMessage(`{"query":` + jq(q) + `}`))
		if e == nil || e.Field != "query" || !strings.Contains(e.Reason, "maksimal 200") || !strings.Contains(e.Hint, "Ringkas") {
			t.Fatalf("diterima/salah: %+v", e)
		}
	}
}

func TestT08_AC4_SpecialCharsAccepted(t *testing.T) {
	for _, q := range []string{`a & b | !c:* (x) 'y' "z" <tag>`, `"`, `'"`, `*`, `:*`, `\`, `%`, `_`, `--`, `;`, `<>`, "😀", `)(`, `<->`} {
		p, e := Parse(json.RawMessage(`{"query":` + jq(q) + `}`))
		if e != nil || p.Query != strings.TrimSpace(q) {
			t.Fatalf("q=%q p=%+v e=%+v", q, p, e)
		}
	}
}

func TestParse_Limit(t *testing.T) {
	p, e := Parse(json.RawMessage(`{"query":"a"}`))
	if e != nil || p.Limit != 10 {
		t.Fatalf("%+v %+v", p, e)
	}
	for _, l := range []string{"1", "20"} {
		if _, e := Parse(json.RawMessage(`{"query":"a","limit":` + l + `}`)); e != nil {
			t.Fatalf("limit %s: %+v", l, e)
		}
	}
	for _, l := range []string{"0", "-1", "21", "100"} {
		_, e := Parse(json.RawMessage(`{"query":"a","limit":` + l + `}`))
		if e == nil || e.Field != "limit" || !strings.Contains(e.Message(), "harus 1-20") || !strings.Contains(e.Message(), "(diterima: "+l+")") {
			t.Fatalf("limit %s: %+v", l, e)
		}
	}
	for _, l := range []string{`"5"`, `1.5`} {
		_, e := Parse(json.RawMessage(`{"query":"a","limit":` + l + `}`))
		if e == nil || e.Field != "limit" || e.Reason != "harus integer" {
			t.Fatalf("limit %s: %+v", l, e)
		}
	}
}

func TestParse_PathNormalized(t *testing.T) {
	cases := map[string]string{
		`{"query":"a","path":"/docs/guides/"}`:                       "docs/guides",
		`{"query":"a","path":"  docs/guides  "}`:                     "docs/guides",
		`{"query":"a","path":"docs"}`:                                "docs",
		`{"query":"a","path":"/"}`:                                   "",
		`{"query":"a","path":""}`:                                    "",
		`{"query":"a"}`:                                              "",
		`{"query":"a","path":` + jq(strings.Repeat("a", 1024)) + `}`: strings.Repeat("a", 1024),
	}
	for in, want := range cases {
		p, e := Parse(json.RawMessage(in))
		if e != nil || p.Prefix != want {
			t.Fatalf("in=%.60s p=%+v e=%+v", in, p, e)
		}
	}
}

func TestParse_PathInvalid(t *testing.T) {
	for _, path := range []string{"../etc", "a/../b", "a\u0000b", strings.Repeat("a", 1025)} {
		_, e := Parse(json.RawMessage(`{"query":"a","path":` + jq(path) + `}`))
		if e == nil || e.Field != "path" {
			t.Fatalf("path %.20q: %+v", path, e)
		}
	}
}

func TestT08_AC5_ProjectIDRejected(t *testing.T) {
	for _, k := range []string{"project_id", "projectId", "project"} {
		p, e := Parse(json.RawMessage(`{"query":"a","` + k + `":"x"}`))
		if e == nil || !strings.Contains(e.Message(), "ditentukan oleh API key") || p != (Params{}) {
			t.Fatalf("%s: %+v %+v", k, p, e)
		}
	}
}

func TestParse_UnknownField(t *testing.T) {
	_, e := Parse(json.RawMessage(`{"query":"a","foo":1}`))
	if e == nil || !strings.Contains(e.Message(), "foo tidak dikenal") || !strings.Contains(e.Message(), "query, path, limit") {
		t.Fatalf("%+v", e)
	}
}

func TestParse_NotObject(t *testing.T) {
	for _, in := range []string{`[]`, `"x"`, `1`, `{"query":"a"}{"x":1}`, `{`} {
		_, e := Parse(json.RawMessage(in))
		if e == nil || e.Field != "arguments" {
			t.Fatalf("in=%q %+v", in, e)
		}
	}
}

func TestParse_ErrorOrder(t *testing.T) {
	_, e := Parse(json.RawMessage(`{"query":"","limit":99}`))
	if e == nil || e.Field != "query" {
		t.Fatalf("%+v", e)
	}
}

func TestSchema_MatchesConstants(t *testing.T) {
	var s map[string]any
	if err := json.Unmarshal(schema(), &s); err != nil {
		t.Fatal(err)
	}
	props := s["properties"].(map[string]any)
	if s["additionalProperties"] != false || len(s["required"].([]any)) != 1 || s["required"].([]any)[0] != "query" {
		t.Fatalf("%v", s)
	}
	if props["query"].(map[string]any)["maxLength"] != float64(200) {
		t.Fatal("query.maxLength")
	}
	l := props["limit"].(map[string]any)
	if l["default"] != float64(10) || l["maximum"] != float64(20) {
		t.Fatal("limit")
	}
	if len(props) != 3 {
		t.Fatalf("props: %v", props)
	}
}
