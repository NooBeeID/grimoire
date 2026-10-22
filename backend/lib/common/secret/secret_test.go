package secret

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"
)

const canary = "ghp_CANARY0123456789abcdefCANARY"

type holder struct {
	Name  string
	Token String
}

func TestNoLeak(t *testing.T) {
	s := New(canary)
	h := holder{Name: "x", Token: s}
	var outs []string
	for _, verb := range []string{"%v", "%+v", "%#v", "%s", "%q", "%x", "%d"} {
		outs = append(outs, fmt.Sprintf(verb, s), fmt.Sprintf(verb, h), fmt.Sprintf(verb, &h))
	}
	outs = append(outs, fmt.Sprint(s), errors.New(fmt.Sprint(s)).Error())
	b, err := json.Marshal(h)
	if err != nil {
		t.Fatal(err)
	}
	outs = append(outs, string(b))
	var jb, tb bytes.Buffer
	slog.New(slog.NewJSONHandler(&jb, nil)).Info("m", "tok", s, "h", h)
	slog.New(slog.NewTextHandler(&tb, nil)).Info("m", "tok", s)
	outs = append(outs, jb.String(), tb.String())
	for i, o := range outs {
		if strings.Contains(o, canary) || strings.Contains(o, "CANARY") {
			t.Errorf("output %d leaks: %s", i, o)
		}
	}
	if got := fmt.Sprintf("%v", s); got != "[REDACTED]" {
		t.Errorf("got %q", got)
	}
}

func TestRevealLast4(t *testing.T) {
	if New(canary).Reveal() != canary {
		t.Fatal("reveal")
	}
	cases := map[string]string{"abc": "abc", "": "", "abcd": "abcd", "xxabcd": "abcd", "日本語のテキスト": "テキスト"}
	for in, want := range cases {
		if got := New(in).Last4(); got != want {
			t.Errorf("Last4(%q)=%q want %q", in, got, want)
		}
	}
	if !New("").IsZero() || New("a").IsZero() {
		t.Fatal("IsZero")
	}
}
