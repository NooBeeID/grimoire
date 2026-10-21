package getapiendpoint

import (
	"encoding/json"
	"strings"
	"testing"
)

func parse(s string) (Params, string, string) {
	p, e := Parse(json.RawMessage(s))
	if e != nil {
		return p, e.Field, e.Message()
	}
	return p, "", ""
}

func TestParse_Defaults(t *testing.T) {
	for _, in := range []string{``, `null`, `{}`} {
		_, f, m := parse(in)
		if f != "method" || !strings.Contains(m, "wajib diisi bersama path, atau isi operation_id") {
			t.Fatalf("%q: %s %s", in, f, m)
		}
	}
	p, f, m := parse(`{"method":"GET","path":"/pets"}`)
	if f != "" || p.SchemaDepth != 4 || p.Ask.Method != "GET" || p.Ask.Path != "/pets" {
		t.Fatalf("%+v %s %s", p, f, m)
	}
}

func TestParse_Exclusivity(t *testing.T) {
	_, f, m := parse(`{"method":"GET","path":"/a","operation_id":"x"}`)
	if f != "operation_id" || !strings.Contains(m, "pilih salah satu") {
		t.Fatal(f, m)
	}
	if _, f, _ := parse(`{"method":"GET"}`); f != "path" {
		t.Fatal(f)
	}
	if _, f, _ := parse(`{"path":"/a"}`); f != "method" {
		t.Fatal(f)
	}
	if p, f, m := parse(`{"operation_id":"x"}`); f != "" || p.Ask.OperationID != "x" {
		t.Fatal(f, m)
	}
	if _, f, m := parse(`{"method":"  ","operation_id":"x"}`); f != "" {
		t.Fatal(f, m)
	}
}

func TestParse_Method(t *testing.T) {
	for in, want := range map[string]string{"get": "GET", " Patch ": "PATCH", "tRaCe": "TRACE"} {
		p, f, m := parse(`{"method":"` + in + `","path":"/a"}`)
		if f != "" || p.Ask.Method != want {
			t.Fatal(in, f, m)
		}
	}
	for _, in := range []string{"FETCH", "CONNECT", "GETS"} {
		_, f, m := parse(`{"method":"` + in + `","path":"/a"}`)
		if f != "method" || !strings.Contains(m, "GET, PUT, POST, DELETE, OPTIONS, HEAD, PATCH, TRACE") {
			t.Fatal(in, f, m)
		}
	}
	if _, f, m := parse(`{"method":"GE\tT","path":"/a"}`); f != "method" || !strings.Contains(m, "karakter kontrol") {
		t.Fatal(f, m)
	}
	if _, f, m := parse(`{"method":"` + strings.Repeat("a", 40) + `","path":"/a"}`); f != "method" || !strings.Contains(m, "maksimal 32 karakter") {
		t.Fatal(f, m)
	}
}

func TestParse_Path(t *testing.T) {
	if _, f, m := parse(`{"method":"GET","path":"pets"}`); f != "path" || !strings.Contains(m, "harus diawali /") {
		t.Fatal(f, m)
	}
	for in, want := range map[string]string{" /pets/{petId} ": "/pets/{petId}", "/pets/": "/pets/", "/Pets/{ID}": "/Pets/{ID}", "/": "/"} {
		p, f, m := parse(`{"method":"GET","path":"` + in + `"}`)
		if f != "" || p.Ask.Path != want {
			t.Fatal(in, f, m)
		}
	}
	if _, f, _ := parse(`{"method":"GET","path":"/` + strings.Repeat("a", 2047) + `"}`); f != "" {
		t.Fatal("2048 harus diterima")
	}
	if _, f, _ := parse(`{"method":"GET","path":"/` + strings.Repeat("a", 2048) + `"}`); f != "path" {
		t.Fatal("2049 harus ditolak")
	}
	if _, f, _ := parse(`{"method":"GET","path":"/a\u0001"}`); f != "path" {
		t.Fatal("kontrol")
	}
}

func TestParse_SpecAndOperationID(t *testing.T) {
	if p, f, _ := parse(`{"spec":"  petstore ","operation_id":"x"}`); f != "" || p.Ask.Spec != "petstore" {
		t.Fatal(p, f)
	}
	if _, f, _ := parse(`{"spec":"` + strings.Repeat("a", 80) + `","operation_id":"x"}`); f != "" {
		t.Fatal("80")
	}
	if _, f, _ := parse(`{"spec":"` + strings.Repeat("a", 81) + `","operation_id":"x"}`); f != "spec" {
		t.Fatal("81")
	}
	if _, f, m := parse(`{"spec":1,"operation_id":"x"}`); f != "spec" || !strings.Contains(m, "harus string") {
		t.Fatal(f, m)
	}
	if _, f, _ := parse(`{"operation_id":"` + strings.Repeat("a", 201) + `"}`); f != "operation_id" {
		t.Fatal("201")
	}
	if _, f, _ := parse(`{"operation_id":"` + strings.Repeat("a", 200) + `"}`); f != "" {
		t.Fatal("200")
	}
}

func TestParse_Depth(t *testing.T) {
	for _, d := range []string{"1", "8"} {
		if _, f, m := parse(`{"operation_id":"x","schema_depth":` + d + `}`); f != "" {
			t.Fatal(d, f, m)
		}
	}
	for _, d := range []string{"0", "9", "-1"} {
		_, f, m := parse(`{"operation_id":"x","schema_depth":` + d + `}`)
		if f != "schema_depth" || !strings.Contains(m, "schema_depth harus 1-8 (diterima: "+d+")") {
			t.Fatal(d, f, m)
		}
	}
	for _, d := range []string{`"x"`, `1.5`} {
		if _, f, m := parse(`{"operation_id":"x","schema_depth":` + d + `}`); f != "schema_depth" || !strings.Contains(m, "harus integer") {
			t.Fatal(d, f, m)
		}
	}
}

func TestParse_ProjectAliasesAndUnknown(t *testing.T) {
	for _, a := range []string{"project_id", "projectId", "project"} {
		_, _, m := parse(`{"` + a + `":"x","operation_id":"y"}`)
		if !strings.Contains(m, "ditentukan oleh API key") {
			t.Fatal(a, m)
		}
	}
	if _, _, m := parse(`{"foo":1}`); !strings.Contains(m, "foo tidak dikenal") || !strings.Contains(m, "spec, method, path, operation_id, schema_depth") {
		t.Fatal(m)
	}
	if _, f, _ := parse(`{}{}`); f != "arguments" {
		t.Fatal(f)
	}
	if _, f, _ := parse(`[]`); f != "arguments" {
		t.Fatal(f)
	}
}

func TestParse_ErrorOrder(t *testing.T) {
	_, f, _ := parse(`{"spec":"` + strings.Repeat("a", 81) + `","schema_depth":9}`)
	if f != "spec" {
		t.Fatal(f)
	}
}

func TestParse_SupersetOfProvider(t *testing.T) {
	for _, in := range []string{`{}`, `{"method":"GET"}`, `{"path":"/pets"}`,
		`{"method":"GET","path":"/pets","operation_id":"x"}`, `{"method":"FETCH","path":"/pets"}`, `{"method":"GET","path":"pets"}`} {
		if _, f, _ := parse(in); f == "" {
			t.Fatalf("%s harus ditolak", in)
		}
	}
	methods := []string{"GET", "PUT", "POST", "DELETE", "OPTIONS", "HEAD", "PATCH", "TRACE", "FETCH"}
	valid := map[string]bool{"GET": true, "PUT": true, "POST": true, "DELETE": true, "OPTIONS": true, "HEAD": true, "PATCH": true, "TRACE": true}
	for _, m := range methods {
		for _, path := range []string{"", "/p"} {
			for _, op := range []string{"", "x"} {
				args := map[string]string{"method": m, "path": path, "operation_id": op}
				b, _ := json.Marshal(args)
				_, f, _ := parse(string(b))
				mp := path != ""
				o := op != ""
				want := valid[m] && (mp && !o)
				// operation_id saja (method kosong) ditangani di bawah.
				if f == "" && !want {
					t.Fatalf("lolos tak sah: %s", b)
				}
				if f != "" && want {
					t.Fatalf("ditolak tak semestinya: %s", b)
				}
			}
		}
	}
}

func TestSchema(t *testing.T) {
	var s map[string]any
	if err := json.Unmarshal(schema(), &s); err != nil {
		t.Fatal(err)
	}
	props := s["properties"].(map[string]any)
	if len(props) != 5 {
		t.Fatal(props)
	}
	for _, k := range allowedArgs {
		if _, ok := props[k]; !ok {
			t.Fatal(k)
		}
	}
	if s["additionalProperties"] != false {
		t.Fatal("additionalProperties")
	}
	if _, ok := s["required"]; ok {
		t.Fatal("required")
	}
	sd := props["schema_depth"].(map[string]any)
	if sd["minimum"] != float64(1) || sd["maximum"] != float64(8) || sd["default"] != float64(4) {
		t.Fatal(sd)
	}
}
