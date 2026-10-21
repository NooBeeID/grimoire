package base

import (
	"encoding/json"
	"strings"
	"testing"
)

type tArgs struct {
	Spec  *string `json:"spec"`
	Limit *int    `json:"limit"`
}

func TestDecodeStrict(t *testing.T) {
	allowed := []string{"spec", "limit"}
	tests := []struct {
		name    string
		raw     string
		allowed []string
		field   string // "" = tanpa error
		contain string
	}{
		{"empty", "", allowed, "", ""},
		{"spaces", "  ", allowed, "", ""},
		{"null", "null", allowed, "", ""},
		{"object", `{"spec":"a"}`, allowed, "", ""},
		{"array", "[]", allowed, "arguments", "harus object JSON"},
		{"number", "1", allowed, "arguments", "harus object JSON"},
		{"broken", "{", allowed, "arguments", "bukan JSON valid"},
		{"trailing", `{"spec":"a"}{"b":2}`, allowed, "arguments", "harus satu object JSON"},
		{"project_id", `{"project_id":"x"}`, allowed, "project_id", "ditentukan oleh API key"},
		{"projectId", `{"projectId":"x"}`, allowed, "projectId", "ditentukan oleh API key"},
		{"project", `{"project":"x"}`, allowed, "project", "ditentukan oleh API key"},
		{"unknown", `{"foo":1}`, allowed, "foo", "foo tidak dikenal. Argumen yang didukung: spec, limit."},
		{"unknown-noargs", `{"foo":1}`, nil, "foo", "foo tidak dikenal. Tool ini tidak menerima argumen."},
		{"unknown-empty-allowed", `{"foo":1}`, []string{}, "foo", "Tool ini tidak menerima argumen."},
		{"null-field-unknown", `{"a":null}`, nil, "a", "tidak dikenal"},
		{"limit-string", `{"limit":"x"}`, allowed, "limit", "harus integer"},
		{"limit-float", `{"limit":1.5}`, allowed, "limit", "harus integer"},
		{"spec-int", `{"spec":1}`, allowed, "spec", "harus string"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var a tArgs
			e := DecodeStrict(json.RawMessage(tc.raw), &a, tc.allowed)
			if tc.field == "" {
				if e != nil {
					t.Fatalf("unexpected error: %v", e.Message())
				}
				return
			}
			if e == nil {
				t.Fatal("want error")
			}
			if e.Field != tc.field || !strings.Contains(e.Message(), tc.contain) {
				t.Fatalf("got field=%q msg=%q", e.Field, e.Message())
			}
		})
	}
}

func TestDecodeStrict_NilRaw(t *testing.T) {
	var a tArgs
	if e := DecodeStrict(nil, &a, nil); e != nil {
		t.Fatal(e.Message())
	}
}

func TestIntArg(t *testing.T) {
	v := func(i int) *int { return &i }
	if got, e := IntArg("limit", nil, 100, 1, 300, ""); e != nil || got != 100 {
		t.Fatalf("nil: %d %v", got, e)
	}
	for _, in := range []int{1, 300} {
		if got, e := IntArg("limit", v(in), 100, 1, 300, ""); e != nil || got != in {
			t.Fatalf("%d: %d %v", in, got, e)
		}
	}
	for _, in := range []int{0, 301} {
		_, e := IntArg("limit", v(in), 100, 1, 300, "Gunakan offset.")
		if e == nil {
			t.Fatalf("%d: want error", in)
		}
	}
	_, e := IntArg("limit", v(500), 100, 1, 300, "Gunakan offset untuk halaman berikutnya.")
	want := "Argumen tidak valid: limit harus 1-300 (diterima: 500). Gunakan offset untuk halaman berikutnya."
	if e.Message() != want {
		t.Fatalf("got %q", e.Message())
	}
}

func TestStrArg(t *testing.T) {
	s := func(x string) *string { return &x }
	if got, e := StrArg("spec", nil, 80, ""); e != nil || got != "" {
		t.Fatal("nil")
	}
	if got, e := StrArg("spec", s("  pet  "), 80, ""); e != nil || got != "pet" {
		t.Fatalf("trim: %q %v", got, e)
	}
	if _, e := StrArg("spec", s("a\tb"), 80, ""); e == nil || !strings.Contains(e.Reason, "karakter kontrol") {
		t.Fatal("control")
	}
	if _, e := StrArg("spec", s("a\x7fb"), 80, ""); e == nil {
		t.Fatal("del")
	}
	if _, e := StrArg("spec", s(strings.Repeat("é", 80)), 80, ""); e != nil {
		t.Fatal("80 rune ok")
	}
	if _, e := StrArg("spec", s(strings.Repeat("é", 81)), 80, ""); e == nil || e.Reason != "maksimal 80 karakter" {
		t.Fatal("81 rune ditolak")
	}
	if _, e := StrArg("spec", s("a\xffb"), 80, ""); e == nil {
		t.Fatal("invalid utf8")
	}
}

func TestMessage(t *testing.T) {
	cases := []struct {
		e    ArgError
		want string
	}{
		{ArgError{Field: "f", Reason: "wajib diisi"}, "Argumen tidak valid: f wajib diisi."},
		{ArgError{Field: "f", Reason: "r", Got: "9"}, "Argumen tidak valid: f r (diterima: 9)."},
		{ArgError{Field: "f", Reason: "r", Hint: "Coba."}, "Argumen tidak valid: f r. Coba."},
		{ArgError{Field: "f", Custom: "apa adanya"}, "apa adanya"},
	}
	for _, c := range cases {
		if got := c.e.Message(); got != c.want {
			t.Errorf("got %q want %q", got, c.want)
		}
	}
}

func TestArgResult(t *testing.T) {
	r := ArgResult(&ArgError{Field: "limit", Reason: "x"}, TargetTypeSpec)
	if !r.IsError || r.TargetType != TargetTypeSpec || r.Summary["error_code"] != "invalid_argument" || r.Summary["field"] != "limit" {
		t.Fatalf("%+v", r)
	}
}

func TestBrief(t *testing.T) {
	if got := Brief(strings.Repeat("x", 100)); got != strings.Repeat("x", 40)+"..." {
		t.Fatalf("got %q", got)
	}
	if got := Brief("a\tb"); got != `a\u0009b` {
		t.Fatalf("got %q", got)
	}
}
