package base

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestT02_AC4_PasswordRedacted(t *testing.T) {
	in := map[string]any{
		"password":      "x",
		"Password":      "x",
		"access_token":  "x",
		"client_secret": "x",
		"X-API-Key":     "x",
		"apiKey":        "x",
		"Authorization": "x",
		"auth":          map[string]any{"token": "t"},
		"list":          []any{map[string]any{"secret": "s"}},
		"path":          "a.md",
	}
	out := Sanitize(in)
	for _, k := range []string{"password", "Password", "access_token", "client_secret", "X-API-Key", "apiKey", "Authorization"} {
		if out[k] != Redacted {
			t.Errorf("%s not redacted: %v", k, out[k])
		}
	}
	if out["auth"].(map[string]any)["token"] != Redacted {
		t.Error("nested token not redacted")
	}
	if out["list"].([]any)[0].(map[string]any)["secret"] != Redacted {
		t.Error("slice secret not redacted")
	}
	if out["path"] != "a.md" {
		t.Error("path must be kept")
	}
	b, _ := json.Marshal(out)
	for _, v := range []string{`"x"`, `"t"`, `"s"`} {
		if strings.Contains(string(b), v) {
			t.Errorf("leaked %s in %s", v, b)
		}
	}
}

func TestT02_AC4_ContentOmitted(t *testing.T) {
	out := Sanitize(map[string]any{"content": "isi file rahasia", "body": "b", "contents": "ok", "path": "a.md", "bytes": 12})
	if out["content"] != Omitted || out["body"] != Omitted {
		t.Fatalf("got %v", out)
	}
	if out["contents"] != "ok" || out["path"] != "a.md" || out["bytes"] != 12 {
		t.Fatalf("got %v", out)
	}
}

func TestSanitize_LongString(t *testing.T) {
	out := Sanitize(map[string]any{"s": strings.Repeat("é", 1500)})
	s := out["s"].(string)
	if !strings.HasSuffix(s, TruncatedSuffix) || !utf8.ValidString(s) {
		t.Fatal("bad truncation")
	}
	if len(strings.TrimSuffix(s, TruncatedSuffix)) > MaxStringBytes {
		t.Fatalf("too long: %d", len(s))
	}
}

func TestSanitize_Depth(t *testing.T) {
	var cur any = "leaf"
	for i := 0; i < 6; i++ {
		cur = map[string]any{"k": cur}
	}
	out := Sanitize(cur.(map[string]any)) // 7 level map
	lvl := any(out)
	for i := 1; i <= 5; i++ {
		lvl = lvl.(map[string]any)["k"]
	}
	if lvl != TooDeep {
		t.Fatalf("level 6 = %v", lvl)
	}
}

func TestSanitize_TotalSizeLimit(t *testing.T) {
	in := map[string]any{}
	for i := 0; i < 10; i++ {
		in[string(rune('a'+i))] = strings.Repeat("z", 1500)
	}
	out := Sanitize(in)
	if out["_truncated"] != true {
		t.Fatalf("got %v", out)
	}
	if n := out["_original_bytes"].(int); n <= MaxJSONBytes {
		t.Fatalf("n=%d", n)
	}
	keys := out["_keys"].([]string)
	if len(keys) != 10 || keys[0] != "a" || keys[9] != "j" {
		t.Fatalf("keys %v", keys)
	}

	big := map[string]any{}
	for i := 0; i < 80; i++ {
		big[strings.Repeat("k", 1)+string(rune('A'+i))] = strings.Repeat("v", 200)
	}
	if got := Sanitize(big)["_keys"].([]string); len(got) != MaxSummaryKeys {
		t.Fatalf("keys %d", len(got))
	}
}

func TestSanitize_NoMutationAndTypes(t *testing.T) {
	in := map[string]any{
		"password": "x",
		"n":        3,
		"f":        1.5,
		"b":        true,
		"nil":      nil,
		"strs":     []string{"a", "b"},
		"st":       struct{ A int }{A: 1},
		"nested":   map[string]any{"token": "t"},
	}
	out := Sanitize(in)
	if in["password"] != "x" || in["nested"].(map[string]any)["token"] != "t" {
		t.Fatal("input mutated")
	}
	if out["n"] != 3 || out["f"] != 1.5 || out["b"] != true || out["nil"] != nil {
		t.Fatalf("primitives changed: %v", out)
	}
	if s := out["strs"].([]any); len(s) != 2 || s[0] != "a" {
		t.Fatalf("strs %v", out["strs"])
	}
	if m := out["st"].(map[string]any); m["A"] != float64(1) {
		t.Fatalf("struct %v", out["st"])
	}
}

func TestSanitize_Nil(t *testing.T) {
	if out := Sanitize(nil); out == nil || len(out) != 0 {
		t.Fatalf("got %v", out)
	}
}

func TestTruncateUTF8(t *testing.T) {
	if got := TruncateUTF8("éé", 3); got != "é" {
		t.Fatalf("got %q", got)
	}
	if TruncateUTF8("abc", 0) != "" || TruncateUTF8("abc", 10) != "abc" {
		t.Fatal("edge cases")
	}
}
