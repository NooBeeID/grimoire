package getfileoutline

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParse_Valid(t *testing.T) {
	got, e := Parse(json.RawMessage(`{"path":"/docs/a.md"}`))
	if e != nil || got.Path != "docs/a.md" {
		t.Fatalf("got %+v err=%v", got, e)
	}
}

func TestParse_Invalid(t *testing.T) {
	cases := []struct {
		name, in string
		contains []string
	}{
		{"path wajib", `{}`, []string{"path wajib diisi", "list_files"}},
		{"folder", `{"path":"docs/"}`, []string{"menunjuk folder"}},
		{"dotdot", `{"path":"a/../b"}`, []string{".."}},
		{"project_id", `{"path":"a.md","project_id":"x"}`, []string{"ditentukan oleh API key"}},
		{"projectId", `{"path":"a.md","projectId":"x"}`, []string{"ditentukan oleh API key"}},
		{"project", `{"path":"a.md","project":"x"}`, []string{"ditentukan oleh API key"}},
		{"field asing", `{"path":"a.md","foo":1}`, []string{"foo tidak dikenal", "path"}},
		{"array", `[]`, []string{"harus object JSON"}},
		{"path angka", `{"path":1}`, []string{"harus string"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, e := Parse(json.RawMessage(c.in))
			if e == nil {
				t.Fatal("harus error")
			}
			for _, w := range c.contains {
				if !strings.Contains(e.Message(), w) {
					t.Errorf("pesan %q tanpa %q", e.Message(), w)
				}
			}
		})
	}
}
