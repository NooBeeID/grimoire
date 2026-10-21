package base

import (
	"encoding/json"
	"strings"
	"testing"
)

type testArgs struct {
	Limit     *int    `json:"limit"`
	Path      *string `json:"path"`
	Recursive *bool   `json:"recursive"`
}

var testAllowed = []string{"path", "recursive", "limit"}

func TestDecodeStrict(t *testing.T) {
	cases := []struct {
		name, in string
		wantNil  bool
		field    string
		contains []string
	}{
		{"kosong", "", true, "", nil},
		{"null", "null", true, "", nil},
		{"spasi", " ", true, "", nil},
		{"object kosong", "{}", true, "", nil},
		{"array", "[]", false, "arguments", []string{"harus object JSON"}},
		{"angka", "1", false, "arguments", []string{"harus object JSON"}},
		{"token sisa", `{"limit":1}{"path":"x"}`, false, "arguments", []string{"harus satu object JSON"}},
		{"sintaks", `{"limit":`, false, "arguments", []string{"bukan JSON valid"}},
		{"project_id", `{"project_id":"x"}`, false, "project_id", []string{"ditentukan oleh API key"}},
		{"projectId", `{"projectId":"x"}`, false, "projectId", []string{"ditentukan oleh API key"}},
		{"project", `{"project":"x"}`, false, "project", []string{"ditentukan oleh API key"}},
		{"tak dikenal", `{"foo":1}`, false, "foo", []string{"foo tidak dikenal", "path, recursive, limit"}},
		{"limit string", `{"limit":"x"}`, false, "limit", []string{"limit harus integer", "diterima: string"}},
		{"limit pecahan", `{"limit":1.5}`, false, "limit", []string{"harus integer"}},
		{"recursive string", `{"recursive":"true"}`, false, "recursive", []string{"harus boolean"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var a testArgs
			err := DecodeStrict(json.RawMessage(tc.in), &a, testAllowed)
			if tc.wantNil {
				if err != nil {
					t.Fatalf("err = %v", err.Message())
				}
				return
			}
			if err == nil {
				t.Fatal("want error")
			}
			if err.Field != tc.field {
				t.Fatalf("field = %q", err.Field)
			}
			for _, c := range tc.contains {
				if !strings.Contains(err.Message(), c) {
					t.Fatalf("%q tidak memuat %q", err.Message(), c)
				}
			}
		})
	}
}

func TestDecodeStrictValues(t *testing.T) {
	var a testArgs
	if err := DecodeStrict(json.RawMessage(`{"limit":5,"path":"d","recursive":true}`), &a, testAllowed); err != nil {
		t.Fatal(err.Message())
	}
	if *a.Limit != 5 || *a.Path != "d" || !*a.Recursive {
		t.Fatalf("%+v", a)
	}
}

func TestIntArg(t *testing.T) {
	v := func(n int) *int { return &n }
	if got, err := IntArg("limit", nil, 7, 1, 10, ""); err != nil || got != 7 {
		t.Fatal(got, err)
	}
	for _, n := range []int{1, 10} {
		if got, err := IntArg("limit", v(n), 7, 1, 10, ""); err != nil || got != n {
			t.Fatal(got, err)
		}
	}
	for _, n := range []int{0, 11} {
		_, err := IntArg("limit", v(n), 7, 1, 10, "petunjuk")
		if err == nil || err.Reason != "harus 1-10" || err.Hint != "petunjuk" {
			t.Fatalf("%v", err)
		}
	}
	_, err := IntArg("offset", v(-1), 0, 0, 2147483647, "")
	if err == nil || err.Reason != "harus >= 0" {
		t.Fatalf("%v", err)
	}
}

func TestBoolArg(t *testing.T) {
	tr := true
	if !BoolArg(nil, true) || BoolArg(nil, false) || !BoolArg(&tr, false) {
		t.Fatal("BoolArg")
	}
}

func TestArgErrorMessage(t *testing.T) {
	e := &ArgError{Field: "limit", Reason: "harus 1-500", Got: "900", Hint: "Gunakan offset."}
	if got := e.Message(); got != "Argumen tidak valid: limit harus 1-500 (diterima: 900). Gunakan offset." {
		t.Fatal(got)
	}
	e = &ArgError{Field: "path", Reason: "wajib diisi"}
	if got := e.Message(); got != "Argumen tidak valid: path wajib diisi." {
		t.Fatal(got)
	}
	if s := e.Summary(); s["error_code"] != "invalid_argument" || s["field"] != "path" {
		t.Fatal(s)
	}
}

func TestBrief(t *testing.T) {
	got := Brief(strings.Repeat("é", 100))
	if got != strings.Repeat("é", 40)+"..." {
		t.Fatal(got)
	}
	if got := Brief("a\x00b\n"); got != `a\u0000b\u000a` {
		t.Fatal(got)
	}
}
