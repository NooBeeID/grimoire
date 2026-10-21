package base

import (
	"encoding/json"
	"testing"
	"unicode/utf8"
)

func TestDecodeArgs(t *testing.T) {
	if m := DecodeArgs(nil); len(m) != 0 {
		t.Fatal(m)
	}
	if m := DecodeArgs(json.RawMessage(`null`)); len(m) != 0 {
		t.Fatal(m)
	}
	if m := DecodeArgs(json.RawMessage(`{"path":"a"}`)); m["path"] != "a" {
		t.Fatal(m)
	}
	for _, in := range []string{`[1]`, `"x"`, `{`} {
		if m := DecodeArgs(json.RawMessage(in)); m["_invalid"] != true {
			t.Fatalf("%s: %v", in, m)
		}
	}
}

func TestTruncateName(t *testing.T) {
	if TruncateName("abc", 5) != "abc" {
		t.Fatal()
	}
	got := TruncateName("あいう", 4)
	if got != "あ" || !utf8.ValidString(got) {
		t.Fatalf("%q", got)
	}
}
